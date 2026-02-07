package openclawws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

const protocolVersion = 3

type ErrorShape struct {
	Code        string          `json:"code"`
	Message     string          `json:"message"`
	Details     json.RawMessage `json:"details,omitempty"`
	Retryable   *bool           `json:"retryable,omitempty"`
	RetryAfterMs *int64         `json:"retryAfterMs,omitempty"`
}

type Frame struct {
	Type  string          `json:"type"`
	ID    string          `json:"id,omitempty"`
	Ok    *bool           `json:"ok,omitempty"`
	Event string          `json:"event,omitempty"`
	Method string         `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error  *ErrorShape     `json:"error,omitempty"`
}

type connectParams struct {
	MinProtocol int `json:"minProtocol"`
	MaxProtocol int `json:"maxProtocol"`
	Client      struct {
		ID       string `json:"id"`
		Version  string `json:"version"`
		Platform string `json:"platform"`
		Mode     string `json:"mode"`
	} `json:"client"`
	Role   string   `json:"role,omitempty"`
	Scopes []string `json:"scopes,omitempty"`
	Auth   struct {
		Token string `json:"token,omitempty"`
	} `json:"auth,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	Locale    string `json:"locale,omitempty"`
}

type reqFrame struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

// Client is a minimal OpenClaw Gateway WebSocket client intended for sequential calls.
// It authenticates using the shared gateway token (connect.params.auth.token), which allows skipping device identity.
type Client struct {
	conn *websocket.Conn
}

func NormalizeGatewayWSURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("gateway url is required")
	}
	if !strings.Contains(s, "://") {
		s = "ws://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(u.Scheme) {
	case "ws", "wss":
		// ok
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported gateway url scheme: %s", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("invalid gateway url: missing host")
	}
	u.Fragment = ""
	return u.String(), nil
}

func DialAndConnect(ctx context.Context, gatewayURL string, token string, timeout time.Duration) (*Client, error) {
	wsURL, err := NormalizeGatewayWSURL(gatewayURL)
	if err != nil {
		return nil, err
	}
	dialCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		dialCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	if cancel != nil {
		defer cancel()
	}

	conn, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{})
	if err != nil {
		return nil, fmt.Errorf("openclaw ws dial failed: %w", err)
	}
	c := &Client{conn: conn}

	// Gateway sends connect.challenge immediately after upgrade; ignore it (and any other events)
	// before we send connect. The server requires the first *client* frame to be connect.
	_ = c.drainOneEventBestEffort(ctx)

	reqID := uuid.NewString()
	var params connectParams
	params.MinProtocol = protocolVersion
	params.MaxProtocol = protocolVersion
	params.Client.ID = "ai-bridge"
	params.Client.Version = "0.1.0"
	params.Client.Platform = "go"
	params.Client.Mode = "operator"
	params.Role = "operator"
	params.Scopes = []string{"operator.read", "operator.write"}
	params.Auth.Token = strings.TrimSpace(token)
	params.UserAgent = "ai-bridge/openclawws"
	params.Locale = "en-US"

	if err := c.writeJSON(ctx, reqFrame{
		Type:   "req",
		ID:     reqID,
		Method: "connect",
		Params: params,
	}); err != nil {
		_ = conn.Close(websocket.StatusProtocolError, "connect write failed")
		return nil, err
	}

	res, err := c.readResponse(ctx, reqID)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "connect failed")
		return nil, err
	}
	if res.Ok == nil || !*res.Ok {
		msg := "connect failed"
		if res.Error != nil && strings.TrimSpace(res.Error.Message) != "" {
			msg = res.Error.Message
		}
		_ = conn.Close(websocket.StatusPolicyViolation, msg)
		return nil, fmt.Errorf("openclaw connect rejected: %s", msg)
	}

	return c, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close(websocket.StatusNormalClosure, "bye")
}

func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if c == nil || c.conn == nil {
		return nil, fmt.Errorf("openclaw ws not connected")
	}
	id := uuid.NewString()
	if err := c.writeJSON(ctx, reqFrame{
		Type:   "req",
		ID:     id,
		Method: strings.TrimSpace(method),
		Params: params,
	}); err != nil {
		return nil, err
	}
	res, err := c.readResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	if res.Ok == nil || !*res.Ok {
		if res.Error != nil {
			return nil, fmt.Errorf("openclaw %s error: %s (%s)", method, res.Error.Message, res.Error.Code)
		}
		return nil, fmt.Errorf("openclaw %s error", method)
	}
	return res.Payload, nil
}

func (c *Client) drainOneEventBestEffort(ctx context.Context) error {
	if c == nil || c.conn == nil {
		return nil
	}
	// Non-fatal. If the server doesn't send an early event for some reason, this will time out or block;
	// caller should provide a timeout ctx.
	_, data, err := c.conn.Read(ctx)
	if err != nil {
		return err
	}
	var frame Frame
	if err := json.Unmarshal(data, &frame); err != nil {
		return nil
	}
	// Only drain events; if we somehow read a res frame here, ignore (connect will read its own response).
	return nil
}

func (c *Client) readResponse(ctx context.Context, id string) (*Frame, error) {
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return nil, err
		}
		var frame Frame
		if err := json.Unmarshal(data, &frame); err != nil {
			continue
		}
		if frame.Type != "res" {
			continue
		}
		if frame.ID != id {
			continue
		}
		return &frame, nil
	}
}

func (c *Client) writeJSON(ctx context.Context, v any) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("openclaw ws not connected")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.conn.Write(ctx, websocket.MessageText, b)
}

