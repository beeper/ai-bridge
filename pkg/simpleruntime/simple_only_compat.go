package connector

import (
	"context"
	"errors"
	"maunium.net/go/mautrix/bridgev2"
	"strings"
)

// Simple bridge keeps model-chat behavior as source of truth.
// Agent-specific hooks are retained as safe no-op compatibility paths while legacy
// callsites are being removed.
func (oc *AIClient) resolveAgentDisplayName(_ context.Context, agent *AgentDefinition) string {
	if agent == nil {
		return ""
	}
	if name := strings.TrimSpace(agent.Name); name != "" {
		return name
	}
	return strings.TrimSpace(agent.ID)
}

type AgentStoreAdapter struct{}

func NewAgentStoreAdapter(*AIClient) *AgentStoreAdapter { return &AgentStoreAdapter{} }

func (s *AgentStoreAdapter) LoadAgents(context.Context) (map[string]*AgentDefinition, error) {
	return map[string]*AgentDefinition{}, nil
}

func (s *AgentStoreAdapter) GetAgentByID(context.Context, string) (*AgentDefinition, error) {
	return nil, errors.New("agent not found")
}

func (oc *AIClient) agentDefaultModel(*AgentDefinition) string {
	if oc == nil {
		return ""
	}
	return oc.effectiveModel(nil)
}

func (oc *AIClient) toolNamesForPortal(*PortalMetadata) []string {
	return []string{ToolNameWebSearch}
}

func (oc *AIClient) isToolAvailable(*PortalMetadata, string) (bool, SettingSource, string) {
	return true, SourceGlobalDefault, ""
}

func (oc *AIClient) isToolAllowedByPolicy(*PortalMetadata, string) bool {
	return true
}

func purgeLoginDataBestEffort(context.Context, *bridgev2.UserLogin) {}

func readStringArgAny(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	raw, ok := args[key]
	if !ok {
		return ""
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func normalizeAgentID(value string) string { return strings.TrimSpace(strings.ToLower(value)) }

func formatCronTime(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return value
}

func (oc *AIClient) enabledBuiltinToolsForModel(context.Context, *PortalMetadata) []ToolDefinition {
	return []ToolDefinition{}
}

func (oc *AIClient) isToolEnabled(*PortalMetadata, string) bool { return false }

func notifyWorkspaceFileChanged(context.Context, string) {}
