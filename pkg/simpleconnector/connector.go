package simpleconnector

import (
	"github.com/beeper/ai-bridge/modules/runtime"
	base "github.com/beeper/ai-bridge/pkg/connector"
)

type OpenAIConnector = base.OpenAIConnector

// New builds the simple bridge connector from the shared base connector.
func New(profile runtime.BridgeProfile) *base.OpenAIConnector {
	oc := &base.OpenAIConnector{}
	oc.SetPolicy(base.BridgePolicy{
		Name:                profile.Name,
		NetworkID:           profile.NetworkID,
		BeeperBridgeType:    profile.BeeperBridgeType,
		ProvisioningEnabled: profile.ProvisioningEnabled,
		ResolveIdentifier:   profile.ResolveIdentifier,
	})
	return oc
}
