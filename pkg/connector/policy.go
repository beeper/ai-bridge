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

	AllowedCommands   map[string]struct{}
	AllowedLoginFlows map[string]struct{}
}

func normalizeAllowSet(in map[string]struct{}) map[string]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(in))
	for value := range in {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		out[trimmed] = struct{}{}
	}
	return out
}

func BuildAllowSet(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		out[trimmed] = struct{}{}
	}
	return out
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
		policy.ResolveIdentifier == (bridgev2.ResolveIdentifierCapabilities{}) &&
		len(policy.AllowedCommands) == 0 &&
		len(policy.AllowedLoginFlows) == 0
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
	policy.AllowedCommands = normalizeAllowSet(policy.AllowedCommands)
	policy.AllowedLoginFlows = normalizeAllowSet(policy.AllowedLoginFlows)
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

func (oc *OpenAIConnector) commandAllowed(commandName string) bool {
	allowed := oc.bridgePolicy().AllowedCommands
	if len(allowed) == 0 {
		return true
	}
	_, ok := allowed[strings.ToLower(strings.TrimSpace(commandName))]
	return ok
}

func (oc *OpenAIConnector) loginFlowAllowed(flowID string) bool {
	allowed := oc.bridgePolicy().AllowedLoginFlows
	if len(allowed) == 0 {
		return true
	}
	_, ok := allowed[strings.ToLower(strings.TrimSpace(flowID))]
	return ok
}

func (oc *OpenAIConnector) shouldBootstrapChats() bool {
	return oc.bridgePolicy().ResolveIdentifier.CreateDM
}

func providerToFlowID(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case ProviderBeeper:
		return ProviderBeeper
	case ProviderMagicProxy:
		return ProviderMagicProxy
	default:
		return FlowCustom
	}
}
