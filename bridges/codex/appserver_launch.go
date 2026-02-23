package codex

import (
	"fmt"
	"strings"
)

type appServerLaunch struct {
	Args         []string
	WebSocketURL string
}

func (cc *CodexConnector) resolveAppServerLaunch() (appServerLaunch, error) {
	listen := "stdio://"
	if cc != nil && cc.Config.Codex != nil && strings.TrimSpace(cc.Config.Codex.Listen) != "" {
		listen = strings.TrimSpace(cc.Config.Codex.Listen)
	}
	switch {
	case strings.EqualFold(listen, "stdio"), strings.EqualFold(listen, "stdio://"):
		return appServerLaunch{Args: []string{"app-server"}}, nil
	case strings.HasPrefix(strings.ToLower(listen), "ws://"), strings.HasPrefix(strings.ToLower(listen), "wss://"):
		return appServerLaunch{
			Args:         []string{"app-server", "--listen", listen},
			WebSocketURL: listen,
		}, nil
	default:
		return appServerLaunch{}, fmt.Errorf("unsupported codex.listen value %q (expected stdio:// or ws://...)", listen)
	}
}
