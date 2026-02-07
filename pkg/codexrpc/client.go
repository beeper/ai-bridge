package codexrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type ClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

type InitializeCapabilities struct {
	ExperimentalAPI bool `json:"experimentalApi,omitempty"`
}

type InitializeParams struct {
	ClientInfo    ClientInfo              `json:"clientInfo"`
	Capabilities  *InitializeCapabilities  `json:"capabilities,omitempty"`
	Extra         map[string]any          `json:"-"`
	RawExtraJSON  json.RawMessage          `json:"-"`
}

type initializeParamsWire struct {
	ClientInfo   ClientInfo             `json:"clientInfo"`
	Capabilities *InitializeCapabilities `json:"capabilities,omitempty"`
}

type ProcessConfig struct {
	Command string
	Args    []string
	Env     []string
}

type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	writeMu sync.Mutex

	nextID  atomic.Int64
	pending sync.Map // idKey -> chan Response

	notifMu   sync.RWMutex
	notifSubs []func(method string, params json.RawMessage)

	reqMu         sync.RWMutex
	requestRoutes map[string]func(ctx context.Context, req Request) (any, *RPCError)

	closed atomic.Bool
}

func StartProcess(ctx context.Context, cfg ProcessConfig) (*Client, error) {
	if strings.TrimSpace(cfg.Command) == "" {
		return nil, fmt.Errorf("missing command")
	}
	args := append([]string{}, cfg.Args...)
	cmd := exec.CommandContext(ctx, cfg.Command, args...)
	if len(cfg.Env) > 0 {
		cmd.Env = append(os.Environ(), cfg.Env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	c := &Client{
		cmd:           cmd,
		stdin:         stdin,
		stdout:        stdout,
		stderr:        stderr,
		requestRoutes: make(map[string]func(ctx context.Context, req Request) (any, *RPCError)),
	}
	c.nextID.Store(1)
	go c.readLoop()
	if c.stderr != nil {
		go c.drainStderr()
	}
	return c, nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	if c.closed.Swap(true) {
		return nil
	}
	_ = c.stdin.Close()
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	if c.cmd != nil {
		_ = c.cmd.Wait()
	}
	return nil
}

func (c *Client) OnNotification(fn func(method string, params json.RawMessage)) {
	if c == nil || fn == nil {
		return
	}
	c.notifMu.Lock()
	c.notifSubs = append(c.notifSubs, fn)
	c.notifMu.Unlock()
}

func (c *Client) HandleRequest(method string, fn func(ctx context.Context, req Request) (any, *RPCError)) {
	if c == nil || strings.TrimSpace(method) == "" || fn == nil {
		return
	}
	method = strings.TrimSpace(method)
	c.reqMu.Lock()
	c.requestRoutes[method] = fn
	c.reqMu.Unlock()
}

func (c *Client) Initialize(ctx context.Context, info ClientInfo, experimental bool) (string, error) {
	params := initializeParamsWire{
		ClientInfo: info,
	}
	if experimental {
		params.Capabilities = &InitializeCapabilities{ExperimentalAPI: true}
	}
	var result struct {
		UserAgent string `json:"userAgent"`
	}
	if err := c.Call(ctx, "initialize", params, &result); err != nil {
		return "", err
	}
	// Followed by initialized notification.
	if err := c.Notify(ctx, "initialized", map[string]any{}); err != nil {
		return "", err
	}
	return result.UserAgent, nil
}

func (c *Client) Notify(ctx context.Context, method string, params any) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}
	msg := map[string]any{
		"method": method,
	}
	if params != nil {
		msg["params"] = params
	}
	return c.writeJSONL(ctx, msg)
}

func (c *Client) Call(ctx context.Context, method string, params any, out any) error {
	if c == nil {
		return fmt.Errorf("client is nil")
	}
	idNum := c.nextID.Add(1)
	idRaw, _ := json.Marshal(idNum)
	ch := make(chan Response, 1)
	c.pending.Store(idKey(idRaw), ch)
	defer c.pending.Delete(idKey(idRaw))

	req := map[string]any{
		"id":     idNum,
		"method": method,
	}
	if params != nil {
		req["params"] = params
	}
	if err := c.writeJSONL(ctx, req); err != nil {
		return err
	}

	var resp Response
	select {
	case resp = <-ch:
	case <-ctx.Done():
		return ctx.Err()
	}
	if resp.Error != nil {
		return fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	if out == nil {
		return nil
	}
	if len(resp.Result) == 0 {
		return errors.New("missing rpc result")
	}
	return json.Unmarshal(resp.Result, out)
}

func (c *Client) writeJSONL(ctx context.Context, v any) error {
	if c.closed.Load() {
		return errors.New("rpc client closed")
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	_ = ctx
	_, err = c.stdin.Write(data)
	return err
}

func (c *Client) readLoop() {
	sc := bufio.NewScanner(c.stdout)
	// Default token limit is 64K; Codex items/diffs can be bigger.
	buf := make([]byte, 0, 1024*1024)
	sc.Buffer(buf, 32*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		line = bytesTrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// Probe JSON shape.
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(line, &probe); err != nil {
			continue
		}
		if _, hasMethod := probe["method"]; hasMethod {
			// Notification or server-initiated request.
			if _, hasID := probe["id"]; hasID {
				var req Request
				if err := json.Unmarshal(line, &req); err != nil {
					continue
				}
				go c.handleServerRequest(req)
				continue
			}
			var n Notification
			if err := json.Unmarshal(line, &n); err != nil {
				continue
			}
			c.dispatchNotification(n.Method, n.Params)
			continue
		}
		if _, hasID := probe["id"]; hasID {
			var resp Response
			if err := json.Unmarshal(line, &resp); err != nil {
				continue
			}
			if chAny, ok := c.pending.Load(idKey(resp.ID)); ok {
				ch := chAny.(chan Response)
				select {
				case ch <- resp:
				default:
				}
			}
			continue
		}
	}
	_ = c.Close()
}

func (c *Client) handleServerRequest(req Request) {
	method := strings.TrimSpace(req.Method)
	if method == "" {
		return
	}
	c.reqMu.RLock()
	handler := c.requestRoutes[method]
	c.reqMu.RUnlock()

	if handler == nil {
		// Best-effort: respond with decline for approval-style requests,
		// otherwise return a generic error.
		result := map[string]any{"decision": "decline"}
		_ = c.writeResponse(context.Background(), req.ID, result, nil)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	result, rpcErr := handler(ctx, req)
	_ = c.writeResponse(context.Background(), req.ID, result, rpcErr)
}

func (c *Client) writeResponse(ctx context.Context, id json.RawMessage, result any, rpcErr *RPCError) error {
	if len(id) == 0 {
		return nil
	}
	msg := map[string]any{
		"id": json.RawMessage(id),
	}
	if rpcErr != nil {
		msg["error"] = rpcErr
	} else {
		msg["result"] = result
	}
	return c.writeJSONL(ctx, msg)
}

func (c *Client) dispatchNotification(method string, params json.RawMessage) {
	c.notifMu.RLock()
	subs := append([]func(string, json.RawMessage){}, c.notifSubs...)
	c.notifMu.RUnlock()
	for _, fn := range subs {
		fn(method, params)
	}
}

func (c *Client) drainStderr() {
	r := bufio.NewReader(c.stderr)
	for {
		_, err := r.ReadString('\n')
		if err != nil {
			return
		}
	}
}

func bytesTrimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\n' || b[0] == '\r' || b[0] == '\t') {
		b = b[1:]
	}
	for len(b) > 0 {
		last := b[len(b)-1]
		if last == ' ' || last == '\n' || last == '\r' || last == '\t' {
			b = b[:len(b)-1]
			continue
		}
		break
	}
	return b
}
