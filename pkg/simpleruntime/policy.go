//lint:file-ignore U1000 Hard-cut compatibility: pending full dead-code deletion.
package connector

import (
	"strings"

	"maunium.net/go/mautrix/bridgev2"
)

// BridgePolicy configures connector composition for a dedicated bridge binary.
// This keeps feature selection out of runtime config flags.
type BridgePolicy struct {
	Name             string
	NetworkID        string
	BeeperBridgeType string

	ProvisioningEnabled bool
	ResolveIdentifier   bridgev2.ResolveIdentifierCapabilities
}

func defaultBridgePolicy() BridgePolicy {
	return BridgePolicy{
		Name:                "Beeper AI",
		NetworkID:           "ai",
		BeeperBridgeType:    "ai",
		ProvisioningEnabled: true,
		ResolveIdentifier: bridgev2.ResolveIdentifierCapabilities{
			CreateDM:       true,
			LookupUsername: true,
			ContactList:    true,
			Search:         true,
		},
	}
}

func normalizeBridgePolicy(policy BridgePolicy) BridgePolicy {
	def := defaultBridgePolicy()
	isZero := strings.TrimSpace(policy.Name) == "" &&
		strings.TrimSpace(policy.NetworkID) == "" &&
		strings.TrimSpace(policy.BeeperBridgeType) == "" &&
		!policy.ProvisioningEnabled &&
		policy.ResolveIdentifier == (bridgev2.ResolveIdentifierCapabilities{})
	if isZero {
		return def
	}

	if strings.TrimSpace(policy.Name) == "" {
		policy.Name = def.Name
	}
	if strings.TrimSpace(policy.NetworkID) == "" {
		policy.NetworkID = def.NetworkID
	}
	if strings.TrimSpace(policy.BeeperBridgeType) == "" {
		policy.BeeperBridgeType = policy.NetworkID
	}
	if policy.ResolveIdentifier == (bridgev2.ResolveIdentifierCapabilities{}) {
		policy.ResolveIdentifier = def.ResolveIdentifier
	}
	return policy
}

func (oc *OpenAIConnector) SetPolicy(policy BridgePolicy) {
	if oc == nil {
		return
	}
	oc.policy = normalizeBridgePolicy(policy)
}

func (oc *OpenAIConnector) bridgePolicy() BridgePolicy {
	if oc == nil {
		return defaultBridgePolicy()
	}
	return normalizeBridgePolicy(oc.policy)
}

func (oc *OpenAIConnector) shouldBootstrapChats() bool {
	return oc.bridgePolicy().ResolveIdentifier.CreateDM
}

func providerToFlowID(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case ProviderBeeper:
		return ProviderMagicProxy
	case ProviderMagicProxy:
		return ProviderMagicProxy
	default:
		return FlowCustom
	}
}
