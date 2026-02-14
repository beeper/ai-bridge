package connector

import (
	"context"
	"errors"
	"strings"
)

// AgentResolver abstracts agent-related lookups and tool-gating decisions.
// The simple bridge uses SimpleAgentResolver (web_search only, no agents).
// Beep provides a full implementation backed by its agent store.
type AgentResolver interface {
	// IsBossAgent reports whether agentID refers to the "boss" agent.
	IsBossAgent(agentID string) bool

	// GetAgent retrieves an agent definition by ID.
	GetAgent(ctx context.Context, agentID string) (*AgentDefinition, error)

	// LoadAgents returns all available agent definitions.
	LoadAgents(ctx context.Context) (map[string]*AgentDefinition, error)

	// ResolveAgentDisplayName returns the human-readable display name for an agent.
	ResolveAgentDisplayName(ctx context.Context, agent *AgentDefinition) string

	// AgentDefaultModel returns the default model ID for the given agent.
	AgentDefaultModel(agent *AgentDefinition) string

	// ToolNamesForPortal returns the tool names enabled for the given portal.
	ToolNamesForPortal(meta *PortalMetadata) []string

	// IsToolEnabled reports whether a specific tool is enabled for the portal.
	IsToolEnabled(meta *PortalMetadata, toolName string) bool

	// IsToolAvailable reports whether a tool is available, along with the
	// setting source and reason (for diagnostics).
	IsToolAvailable(meta *PortalMetadata, toolName string) (bool, SettingSource, string)

	// IsToolAllowedByPolicy reports whether a tool is allowed by policy.
	IsToolAllowedByPolicy(meta *PortalMetadata, toolName string) bool

	// EnabledBuiltinToolsForModel returns the builtin tool definitions that
	// should be active for the current model and portal configuration.
	EnabledBuiltinToolsForModel(ctx context.Context, meta *PortalMetadata) []ToolDefinition
}

var errAgentNotFound = errors.New("agent not found")

// SimpleAgentResolver is the default AgentResolver for the simple bridge.
// It exposes only web_search and returns no-op results for agent queries.
type SimpleAgentResolver struct {
	// client is set during AIClient init so that AgentDefaultModel
	// can delegate to effectiveModel.
	client *AIClient
}

var _ AgentResolver = (*SimpleAgentResolver)(nil)

func (SimpleAgentResolver) IsBossAgent(agentID string) bool {
	return isBossAgent(agentID)
}

func (SimpleAgentResolver) GetAgent(context.Context, string) (*AgentDefinition, error) {
	return nil, errAgentNotFound
}

func (SimpleAgentResolver) LoadAgents(context.Context) (map[string]*AgentDefinition, error) {
	return map[string]*AgentDefinition{}, nil
}

func (SimpleAgentResolver) ResolveAgentDisplayName(_ context.Context, agent *AgentDefinition) string {
	if agent == nil {
		return ""
	}
	if name := strings.TrimSpace(agent.Name); name != "" {
		return name
	}
	return strings.TrimSpace(agent.ID)
}

func (r *SimpleAgentResolver) AgentDefaultModel(_ *AgentDefinition) string {
	if r.client == nil {
		return ""
	}
	return r.client.effectiveModel(nil)
}

func (SimpleAgentResolver) ToolNamesForPortal(*PortalMetadata) []string {
	return []string{ToolNameWebSearch}
}

func (SimpleAgentResolver) IsToolEnabled(*PortalMetadata, string) bool {
	return false
}

func (SimpleAgentResolver) IsToolAvailable(*PortalMetadata, string) (bool, SettingSource, string) {
	return true, SourceGlobalDefault, ""
}

func (SimpleAgentResolver) IsToolAllowedByPolicy(*PortalMetadata, string) bool {
	return true
}

func (SimpleAgentResolver) EnabledBuiltinToolsForModel(context.Context, *PortalMetadata) []ToolDefinition {
	return []ToolDefinition{}
}
