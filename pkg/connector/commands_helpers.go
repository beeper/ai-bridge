package connector

import (
	"maunium.net/go/mautrix/bridgev2/commands"

	"github.com/beeper/ai-bridge/pkg/agents"
)

func requireClientMeta(ce *commands.Event) (*AIClient, *PortalMetadata, bool) {
	client := getAIClient(ce)
	meta := getPortalMeta(ce)
	if client == nil || meta == nil {
		ce.Reply("Failed to access AI configuration")
		return nil, nil, false
	}
	return client, meta, true
}

func requireClient(ce *commands.Event) (*AIClient, bool) {
	client := getAIClient(ce)
	if client == nil {
		ce.Reply("Failed to access AI configuration")
		return nil, false
	}
	return client, true
}

func rejectBossOverrides(ce *commands.Event, meta *PortalMetadata, message string) bool {
	if agents.IsBossAgent(meta.AgentID) || agents.IsBossAgent(meta.DefaultAgentID) {
		ce.Reply(message)
		return true
	}
	return false
}
