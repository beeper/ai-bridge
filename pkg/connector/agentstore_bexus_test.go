package connector

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

func newAgentStoreTestClient(tokens *ServiceTokens) *AIClient {
	meta := &UserLoginMetadata{ServiceTokens: tokens}
	login := &database.UserLogin{ID: networkid.UserLoginID("login"), Metadata: meta}
	userLogin := &bridgev2.UserLogin{UserLogin: login, Log: zerolog.Nop()}
	return &AIClient{UserLogin: userLogin}
}

func TestLoadAgentsHidesBexusWithoutConnectedClay(t *testing.T) {
	client := newAgentStoreTestClient(nil)
	store := NewAgentStoreAdapter(client)

	agentsMap, err := store.LoadAgents(context.Background())
	if err != nil {
		t.Fatalf("LoadAgents failed: %v", err)
	}
	if _, ok := agentsMap["nexus"]; ok {
		t.Fatalf("expected Bexus preset to be hidden without connected Clay MCP")
	}
}

func TestLoadAgentsShowsBexusWithConnectedClay(t *testing.T) {
	client := newAgentStoreTestClient(&ServiceTokens{MCPServers: map[string]MCPServerConfig{
		"nexus": {
			Endpoint:  "https://nexum.clay.earth/mcp",
			AuthType:  "none",
			Connected: true,
			Kind:      mcpServerKindNexus,
		},
	}})
	store := NewAgentStoreAdapter(client)

	agentsMap, err := store.LoadAgents(context.Background())
	if err != nil {
		t.Fatalf("LoadAgents failed: %v", err)
	}
	agent, ok := agentsMap["nexus"]
	if !ok {
		t.Fatalf("expected Bexus preset to be visible with connected Clay MCP")
	}
	if agent.Name != "Bexus" {
		t.Fatalf("expected visible agent name Bexus, got %q", agent.Name)
	}
}
