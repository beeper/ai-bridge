package opencode

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/beeper/agentremote/bridges/opencode/api"
)

type managedOpenCodeProcess struct {
	cmd *exec.Cmd
	url string
}

func (p *managedOpenCodeProcess) Close() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	_ = p.cmd.Process.Kill()
	_, _ = p.cmd.Process.Wait()
	return nil
}

func allocateLoopbackHTTPURL() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("allocate loopback http listener: %w", err)
	}
	addr, ok := l.Addr().(*net.TCPAddr)
	_ = l.Close()
	if !ok || addr == nil || addr.Port == 0 {
		return "", errors.New("allocate loopback http listener: missing TCP port")
	}
	return fmt.Sprintf("http://127.0.0.1:%d", addr.Port), nil
}

func (m *OpenCodeManager) spawnManagedProcess(ctx context.Context, cfg *OpenCodeInstance, workingDir string) (*managedOpenCodeProcess, error) {
	if cfg == nil {
		return nil, errors.New("managed opencode config is required")
	}
	binaryPath := strings.TrimSpace(cfg.BinaryPath)
	if binaryPath == "" {
		return nil, errors.New("managed opencode binary path is missing")
	}
	workingDir = strings.TrimSpace(workingDir)
	if workingDir == "" {
		return nil, errors.New("managed opencode working directory is missing")
	}
	baseURL, err := allocateLoopbackHTTPURL()
	if err != nil {
		return nil, err
	}
	client, err := api.NewClient(baseURL, "", "")
	if err != nil {
		return nil, err
	}
	port := strings.TrimPrefix(baseURL, "http://127.0.0.1:")
	cmd := exec.CommandContext(ctx, binaryPath, "serve", "--hostname", "127.0.0.1", "--port", port)
	cmd.Dir = workingDir
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			m.log().Debug().
				Str("instance", cfg.ID).
				Str("workdir", workingDir).
				Msg(scanner.Text())
		}
	}()
	dead := make(chan error, 1)
	go func() {
		dead <- cmd.Wait()
	}()
	readyCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err = client.ListSessions(readyCtx); err == nil {
			return &managedOpenCodeProcess{cmd: cmd, url: baseURL}, nil
		}
		select {
		case waitErr := <-dead:
			if waitErr == nil {
				waitErr = errors.New("managed opencode process exited before becoming ready")
			}
			return nil, waitErr
		case <-readyCtx.Done():
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("managed opencode did not become ready: %w", readyCtx.Err())
		case <-ticker.C:
		}
	}
}
