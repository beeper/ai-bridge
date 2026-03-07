package codex

import (
	"strings"
	"testing"
)

func TestResolveAppServerLaunch_DefaultsToLoopbackWebSocket(t *testing.T) {
	cc := &CodexConnector{}

	launch, err := cc.resolveAppServerLaunch()
	if err != nil {
		t.Fatalf("resolveAppServerLaunch returned error: %v", err)
	}

	if len(launch.Args) != 3 {
		t.Fatalf("expected websocket launch args, got %v", launch.Args)
	}
	if launch.Args[0] != "app-server" || launch.Args[1] != "--listen" {
		t.Fatalf("unexpected args: %v", launch.Args)
	}
	if !strings.HasPrefix(launch.Args[2], "ws://127.0.0.1:") {
		t.Fatalf("expected loopback websocket URL, got %q", launch.Args[2])
	}
	if launch.WebSocketURL != launch.Args[2] {
		t.Fatalf("expected websocket dial URL to match listen URL, got %q vs %q", launch.WebSocketURL, launch.Args[2])
	}
}

func TestResolveAppServerLaunch_UsesConfiguredWebSocket(t *testing.T) {
	cc := &CodexConnector{Config: Config{Codex: &CodexConfig{Listen: "ws://127.0.0.1:4545"}}}

	launch, err := cc.resolveAppServerLaunch()
	if err != nil {
		t.Fatalf("resolveAppServerLaunch returned error: %v", err)
	}

	if got := strings.Join(launch.Args, " "); got != "app-server --listen ws://127.0.0.1:4545" {
		t.Fatalf("unexpected args: %q", got)
	}
	if launch.WebSocketURL != "ws://127.0.0.1:4545" {
		t.Fatalf("unexpected websocket URL: %q", launch.WebSocketURL)
	}
}

func TestResolveAppServerLaunch_RejectsNonWebSocketListenURL(t *testing.T) {
	cc := &CodexConnector{Config: Config{Codex: &CodexConfig{Listen: "stdio://"}}}

	_, err := cc.resolveAppServerLaunch()
	if err == nil {
		t.Fatal("expected stdio listen URL to be rejected")
	}
}
