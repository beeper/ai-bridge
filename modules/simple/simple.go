package simple

import (
	base "github.com/beeper/ai-bridge/pkg/simpleruntime"
	"maunium.net/go/mautrix/bridgev2"
)

func NewConnector() *base.OpenAIConnector {
	oc := &base.OpenAIConnector{}
	oc.SetPolicy(base.BridgePolicy{
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
	})
	return oc
}
