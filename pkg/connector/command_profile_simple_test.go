package connector

import "testing"

func TestSimpleProfileCommandSurface(t *testing.T) {
	oc := &OpenAIConnector{}
	oc.SetPolicy(BridgePolicy{
		NetworkID: "ai-simple",
	})

	allowed := []string{"model", "status", "send"}
	for _, name := range allowed {
		if !oc.shouldRegisterCommand(name) {
			t.Fatalf("expected command %q to be enabled for simple profile", name)
		}
	}

	blocked := []string{"memory", "agent", "cron", "mcp", "clay", "manage"}
	for _, name := range blocked {
		if oc.shouldRegisterCommand(name) {
			t.Fatalf("expected command %q to be disabled for simple profile", name)
		}
	}
}
