package core

import (
	"github.com/beeper/ai-bridge/pkg/connector"
	"maunium.net/go/mautrix/bridgev2"
)

func allow(names ...string) map[string]struct{} {
	return connector.BuildAllowSet(names...)
}

func PolicyFor(kind BridgeKind) connector.BridgePolicy {
	switch kind {
	case BridgeSimple:
		return connector.BridgePolicy{
			Name:                "Beeper AI (Simple)",
			NetworkID:           "ai-simple",
			BeeperBridgeType:    "ai-simple",
			ProvisioningEnabled: true,
			ResolveIdentifier: bridgev2.ResolveIdentifierCapabilities{
				CreateDM:       true,
				LookupUsername: true,
				ContactList:    true,
				Search:         false,
			},
			AllowedLoginFlows: allow(connector.ProviderMagicProxy, connector.FlowCustom),
			AllowedCommands: allow(
				"status", "approve", "reset", "stop", "whoami", "commands",
				"model", "temp", "system-prompt", "context", "tokens", "config", "debounce", "typing", "mode", "new", "fork", "regenerate", "title", "models", "timezone", "gravatar", "playground", "queue", "think", "verbose", "reasoning", "elevated", "activation", "send",
			),
		}
	case BridgeAgentic:
		return connector.BridgePolicy{
			Name:                "Beeper AI (Agentic)",
			NetworkID:           "ai-agentic",
			BeeperBridgeType:    "ai-agentic",
			ProvisioningEnabled: true,
			ResolveIdentifier: bridgev2.ResolveIdentifierCapabilities{
				CreateDM:       true,
				LookupUsername: true,
				ContactList:    true,
				Search:         true,
			},
			AllowedLoginFlows: allow(connector.ProviderMagicProxy, connector.FlowCustom),
			AllowedCommands: allow(
				"status", "last-heartbeat", "approve", "reset", "stop", "whoami", "commands",
				"model", "temp", "system-prompt", "context", "tokens", "config", "debounce", "typing", "mode", "new", "fork", "regenerate", "title", "models", "timezone", "gravatar", "playground", "queue", "think", "verbose", "reasoning", "elevated", "activation", "send",
				"tools", "cron", "agent", "agents", "create-agent", "delete-agent", "manage", "memory", "mcp", "clay",
			),
		}
	default:
		return connector.BridgePolicy{}
	}
}
