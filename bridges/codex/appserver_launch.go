package codex

import (
	"fmt"
	"net"
	"strings"
)

type appServerLaunch struct {
	Args         []string
	WebSocketURL string
}

func (cc *CodexConnector) resolveAppServerLaunch() (appServerLaunch, error) {
	listen := ""
	if cc != nil && cc.Config.Codex != nil {
		listen = strings.TrimSpace(cc.Config.Codex.Listen)
	}
	if listen == "" {
		wsURL, err := allocateLoopbackWebSocketURL()
		if err != nil {
			return appServerLaunch{}, err
		}
		return appServerLaunch{
			Args:         []string{"app-server", "--listen", wsURL},
			WebSocketURL: wsURL,
		}, nil
	}

	switch {
	case strings.HasPrefix(strings.ToLower(listen), "ws://"):
		return appServerLaunch{
			Args:         []string{"app-server", "--listen", listen},
			WebSocketURL: listen,
		}, nil
	default:
		return appServerLaunch{}, fmt.Errorf("unsupported codex.listen value %q (expected ws://IP:PORT, or blank for auto loopback websocket)", listen)
	}
}

func allocateLoopbackWebSocketURL() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("allocate loopback websocket listener: %w", err)
	}
	addr, ok := l.Addr().(*net.TCPAddr)
	_ = l.Close()
	if !ok || addr == nil || addr.Port == 0 {
		return "", fmt.Errorf("allocate loopback websocket listener: missing TCP port")
	}
	return fmt.Sprintf("ws://127.0.0.1:%d", addr.Port), nil
}
