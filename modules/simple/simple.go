package simple

import (
	"github.com/beeper/ai-bridge/modules/runtime"
	"github.com/beeper/ai-bridge/pkg/simpleconnector"
	"maunium.net/go/mautrix/bridgev2"
)

func Profile() runtime.BridgeProfile {
	return runtime.BridgeProfile{
		Name:                "Beeper AI (Simple)",
		NetworkID:           "ai-simple",
		BeeperBridgeType:    "ai-simple",
		ProvisioningEnabled: true,
		ResolveIdentifier: bridgev2.ResolveIdentifierCapabilities{
			CreateDM:       true,
			LookupUsername: true,
			ContactList:    true,
			Search:         true,
		},
	}
}

func NewConnector() *simpleconnector.OpenAIConnector {
	return simpleconnector.New(Profile())
}
