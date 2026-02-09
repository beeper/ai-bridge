package connector

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/beeper/ai-bridge/pkg/opencode"
	"github.com/beeper/ai-bridge/pkg/opencodebridge"
)

type openCodeLocalServer struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	baseURL  string
	port     int
	username string
	password string
	dir      string // resolved absolute project path ("" for config-driven default)
}

func (oc *AIClient) bootstrapOpenCode(ctx context.Context) {
	if oc == nil || oc.UserLogin == nil {
		return
	}
	// Autostart must happen before restore, so RestoreConnections can connect to the local instance.
	_ = oc.ensureOpenCodeLocalServer(ctx)
	if oc.opencodeBridge != nil {
		if err := oc.opencodeBridge.RestoreConnections(ctx); err != nil {
			oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to restore OpenCode connections")
		}
	}
}

func (oc *AIClient) stopOpenCodeLocalServers() {
	if oc == nil {
		return
	}
	oc.opencodeLocalsMu.Lock()
	servers := oc.opencodeLocals
	oc.opencodeLocals = nil
	oc.opencodeLocalsMu.Unlock()
	for _, srv := range servers {
		srv.stop()
	}
}

func (oc *AIClient) ensureOpenCodeLocalServer(ctx context.Context) error {
	if oc == nil || oc.connector == nil || oc.UserLogin == nil {
		return nil
	}
	cfg := oc.connector.Config.OpenCode
	if cfg == nil {
		return nil
	}
	if cfg.Enabled != nil && !*cfg.Enabled {
		return nil
	}
	if cfg.AutoStart == nil || !*cfg.AutoStart {
		return nil
	}

	oc.opencodeLocalsMu.Lock()
	srv := oc.opencodeLocals[""]
	oc.opencodeLocalsMu.Unlock()
	if srv != nil && strings.TrimSpace(srv.baseURL) != "" {
		return nil
	}

	cmdName := strings.TrimSpace(cfg.Command)
	if cmdName == "" {
		cmdName = "opencode"
	}
	if _, err := exec.LookPath(cmdName); err != nil {
		return err
	}

	host := strings.TrimSpace(cfg.Hostname)
	if host == "" {
		host = "127.0.0.1"
	}

	meta := loginMetadata(oc.UserLogin)
	if meta == nil {
		return nil
	}

	username := strings.TrimSpace(cfg.Username)
	if username == "" {
		username = strings.TrimSpace(meta.OpenCodeLocalUsername)
	}
	if username == "" {
		username = "opencode"
	}

	password := strings.TrimSpace(cfg.Password)
	if password == "" {
		password = strings.TrimSpace(meta.OpenCodeLocalPassword)
	}
	if password == "" {
		pw, err := randomToken(32)
		if err != nil {
			return err
		}
		password = pw
	}

	port := 0
	if cfg.Port > 0 {
		port = cfg.Port
	} else if meta.OpenCodeLocalPort > 0 {
		port = meta.OpenCodeLocalPort
	} else {
		picked, err := pickFreeTCPPort(host)
		if err != nil {
			return err
		}
		port = picked
	}

	// Persist the local server settings so the instance ID remains stable across restarts.
	changed := false
	if meta.OpenCodeLocalPort != port {
		meta.OpenCodeLocalPort = port
		changed = true
	}
	if meta.OpenCodeLocalUsername != username {
		meta.OpenCodeLocalUsername = username
		changed = true
	}
	if meta.OpenCodeLocalPassword != password {
		meta.OpenCodeLocalPassword = password
		changed = true
	}
	if changed {
		saveCtx, cancel := context.WithTimeout(oc.backgroundContext(ctx), 10*time.Second)
		_ = oc.UserLogin.Save(saveCtx)
		cancel()
	}

	baseURL := fmt.Sprintf("http://%s:%d", host, port)

	// If there is already a server at this URL with our password, just persist config and let RestoreConnections connect.
	if err := waitForOpenCodeServer(oc.backgroundContext(ctx), baseURL, username, password, 2*time.Second); err == nil {
		return oc.upsertOpenCodeInstanceConfig(oc.backgroundContext(ctx), baseURL, username, password)
	}

	// Spawn a managed local server.
	bg := oc.UserLogin.Bridge.BackgroundCtx
	srvCtx, cancel := context.WithCancel(bg)
	args := []string{"serve", "--hostname", host, "--port", strconv.Itoa(port), "--log-level", "WARN"}
	cmd := exec.CommandContext(srvCtx, cmdName, args...)
	cmd.Env = append(os.Environ(), "OPENCODE_SERVER_PASSWORD="+password)

	if cfg.IsolateXDG != nil && *cfg.IsolateXDG {
		base := strings.TrimSpace(cfg.HomeBaseDir)
		if base == "" {
			if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
				base = filepath.Join(home, ".local", "share", "ai-bridge", "opencode")
			} else {
				base = filepath.Join(os.TempDir(), "ai-bridge-opencode")
			}
		}
		loginDir := openCodeLocalLoginDir(string(oc.UserLogin.ID))
		root := filepath.Join(base, loginDir)
		_ = os.MkdirAll(root, 0o700)
		dataHome := filepath.Join(root, "data")
		configHome := filepath.Join(root, "config")
		cacheHome := filepath.Join(root, "cache")
		stateHome := filepath.Join(root, "state")
		_ = os.MkdirAll(dataHome, 0o700)
		_ = os.MkdirAll(configHome, 0o700)
		_ = os.MkdirAll(cacheHome, 0o700)
		_ = os.MkdirAll(stateHome, 0o700)
		cmd.Env = append(cmd.Env,
			"XDG_DATA_HOME="+dataHome,
			"XDG_CONFIG_HOME="+configHome,
			"XDG_CACHE_HOME="+cacheHome,
			"XDG_STATE_HOME="+stateHome,
		)
	}

	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}

	local := &openCodeLocalServer{
		cmd:      cmd,
		cancel:   cancel,
		baseURL:  baseURL,
		port:     port,
		username: username,
		password: password,
	}
	oc.opencodeLocalsMu.Lock()
	if oc.opencodeLocals == nil {
		oc.opencodeLocals = make(map[string]*openCodeLocalServer)
	}
	oc.opencodeLocals[""] = local
	oc.opencodeLocalsMu.Unlock()

	if err := waitForOpenCodeServer(oc.backgroundContext(ctx), baseURL, username, password, 20*time.Second); err != nil {
		local.stop()
		return err
	}

	return oc.upsertOpenCodeInstanceConfig(oc.backgroundContext(ctx), baseURL, username, password)
}

func (oc *AIClient) upsertOpenCodeInstanceConfig(ctx context.Context, baseURL, username, password string) error {
	if oc == nil || oc.UserLogin == nil {
		return nil
	}
	meta := loginMetadata(oc.UserLogin)
	if meta == nil {
		return nil
	}
	instID := opencodebridge.OpenCodeInstanceID(baseURL, username)
	if meta.OpenCodeInstances == nil {
		meta.OpenCodeInstances = make(map[string]*opencodebridge.OpenCodeInstance)
	}
	meta.OpenCodeInstances[instID] = &opencodebridge.OpenCodeInstance{
		ID:       instID,
		URL:      strings.TrimSpace(baseURL),
		Username: strings.TrimSpace(username),
		Password: strings.TrimSpace(password),
	}
	saveCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := oc.UserLogin.Save(saveCtx)
	cancel()
	return err
}

func (s *openCodeLocalServer) stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancel := s.cancel
	cmd := s.cmd
	s.cancel = nil
	s.cmd = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if cmd == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
	}
}

func waitForOpenCodeServer(ctx context.Context, baseURL, username, password string, timeout time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return errors.New("opencode server did not become ready")
		}
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		client, err := opencode.NewClient(baseURL, username, password)
		if err == nil {
			_, err = client.ListSessions(callCtx)
		}
		cancel()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func pickFreeTCPPort(host string) (int, error) {
	addr := net.JoinHostPort(host, "0")
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok || tcp == nil || tcp.Port == 0 {
		return 0, errors.New("failed to allocate port")
	}
	return tcp.Port, nil
}

func randomToken(n int) (string, error) {
	if n <= 0 {
		n = 32
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// spawnOpenCodeForDir starts (or reuses) a local OpenCode server rooted in dir.
// It returns the instance ID of the connected server.
func (oc *AIClient) spawnOpenCodeForDir(ctx context.Context, dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(absDir)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", absDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", absDir)
	}

	oc.opencodeLocalsMu.Lock()
	if oc.opencodeLocals == nil {
		oc.opencodeLocals = make(map[string]*openCodeLocalServer)
	}
	existing := oc.opencodeLocals[absDir]
	oc.opencodeLocalsMu.Unlock()

	// Reuse a running server if it's still healthy.
	if existing != nil && strings.TrimSpace(existing.baseURL) != "" {
		if err := waitForOpenCodeServer(ctx, existing.baseURL, existing.username, existing.password, 2*time.Second); err == nil {
			instID := opencodebridge.OpenCodeInstanceID(existing.baseURL, existing.username)
			return instID, nil
		}
		// Dead — remove it and fall through to spawn a new one.
		oc.opencodeLocalsMu.Lock()
		if oc.opencodeLocals[absDir] == existing {
			delete(oc.opencodeLocals, absDir)
		}
		oc.opencodeLocalsMu.Unlock()
		existing.stop()
	}

	// Resolve command name from config, default to "opencode".
	cmdName := "opencode"
	if oc.connector != nil && oc.connector.Config.OpenCode != nil {
		if c := strings.TrimSpace(oc.connector.Config.OpenCode.Command); c != "" {
			cmdName = c
		}
	}
	if _, err := exec.LookPath(cmdName); err != nil {
		return "", fmt.Errorf("opencode binary not found: %w", err)
	}

	host := "127.0.0.1"
	port, err := pickFreeTCPPort(host)
	if err != nil {
		return "", fmt.Errorf("pick port: %w", err)
	}

	password, err := randomToken(32)
	if err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	username := "opencode"
	baseURL := fmt.Sprintf("http://%s:%d", host, port)

	bg := oc.UserLogin.Bridge.BackgroundCtx
	srvCtx, cancel := context.WithCancel(bg)
	args := []string{"serve", "--hostname", host, "--port", strconv.Itoa(port), "--log-level", "WARN"}
	cmd := exec.CommandContext(srvCtx, cmdName, args...)
	cmd.Dir = absDir
	cmd.Env = append(os.Environ(), "OPENCODE_SERVER_PASSWORD="+password)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		cancel()
		return "", fmt.Errorf("start opencode: %w", err)
	}

	local := &openCodeLocalServer{
		cmd:      cmd,
		cancel:   cancel,
		baseURL:  baseURL,
		port:     port,
		username: username,
		password: password,
		dir:      absDir,
	}
	oc.opencodeLocalsMu.Lock()
	oc.opencodeLocals[absDir] = local
	oc.opencodeLocalsMu.Unlock()

	if err := waitForOpenCodeServer(ctx, baseURL, username, password, 20*time.Second); err != nil {
		local.stop()
		oc.opencodeLocalsMu.Lock()
		if oc.opencodeLocals[absDir] == local {
			delete(oc.opencodeLocals, absDir)
		}
		oc.opencodeLocalsMu.Unlock()
		return "", fmt.Errorf("server not ready: %w", err)
	}

	// Ensure bridge exists and connect.
	if oc.opencodeBridge == nil {
		oc.opencodeBridge = opencodebridge.NewBridge(oc)
	}
	inst, _, err := oc.opencodeBridge.Connect(ctx, baseURL, password, username)
	if err != nil {
		local.stop()
		oc.opencodeLocalsMu.Lock()
		if oc.opencodeLocals[absDir] == local {
			delete(oc.opencodeLocals, absDir)
		}
		oc.opencodeLocalsMu.Unlock()
		return "", fmt.Errorf("connect to opencode: %w", err)
	}

	return inst.ID, nil
}

// looksLikeFilesystemPath returns true if s is unambiguously a filesystem path
// (starts with /, ~/, ./, ../, or is exactly ".").
func looksLikeFilesystemPath(s string) bool {
	return s == "." || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~/") ||
		strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../")
}

func openCodeLocalLoginDir(loginID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(loginID)))
	return hex.EncodeToString(sum[:8])
}
