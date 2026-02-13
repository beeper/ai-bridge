package core

import (
	"github.com/beeper/ai-bridge/modules/simple"
	"github.com/beeper/ai-bridge/pkg/connector"
)

func SimplePolicy() connector.BridgePolicy {
	p := simple.Profile()
	return connector.BridgePolicy{
		Name:                p.Name,
		NetworkID:           p.NetworkID,
		BeeperBridgeType:    p.BeeperBridgeType,
		ProvisioningEnabled: p.ProvisioningEnabled,
		ResolveIdentifier:   p.ResolveIdentifier,
	}
}
