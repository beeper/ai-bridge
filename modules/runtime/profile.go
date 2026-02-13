package runtime

import "maunium.net/go/mautrix/bridgev2"

type BridgeProfile struct {
	Name                string
	NetworkID           string
	BeeperBridgeType    string
	ProvisioningEnabled bool
	ResolveIdentifier   bridgev2.ResolveIdentifierCapabilities
}
