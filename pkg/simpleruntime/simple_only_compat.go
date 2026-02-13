package connector

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	agents "github.com/beeper/ai-bridge/pkg/simpleruntime/simpleagent"
	agenttools "github.com/beeper/ai-bridge/pkg/simpleruntime/simpleagent/tools"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

// Simple bridge keeps model-chat behavior as source of truth.
// Agent-specific hooks are retained as safe no-op compatibility paths while legacy
// callsites are being removed.
func (oc *AIClient) resolveAgentDisplayName(_ context.Context, agent *agents.AgentDefinition) string {
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

func (s *AgentStoreAdapter) LoadAgents(context.Context) (map[string]*agents.AgentDefinition, error) {
	return map[string]*agents.AgentDefinition{}, nil
}

func (s *AgentStoreAdapter) GetAgentByID(context.Context, string) (*agents.AgentDefinition, error) {
	return nil, errors.New("agent not found")
}

func (oc *AIClient) agentDefaultModel(*agents.AgentDefinition) string {
	if oc == nil {
		return ""
	}
	return oc.effectiveModel(nil)
}

func (oc *AIClient) toolNamesForPortal(*PortalMetadata) []string {
	return []string{ToolNameWebSearch}
}

func (oc *AIClient) lookupMCPToolDefinition(context.Context, string) (ToolDefinition, bool) {
	return ToolDefinition{}, false
}

func (oc *AIClient) isToolAvailable(*PortalMetadata, string) (bool, SettingSource, string) {
	return true, SourceGlobalDefault, ""
}

func (oc *AIClient) isToolAllowedByPolicy(*PortalMetadata, string) bool {
	return true
}

func purgeLoginDataBestEffort(context.Context, *bridgev2.UserLogin) {}

func (oc *AIClient) notifySessionMemoryChange(context.Context, *bridgev2.Portal, *PortalMetadata, bool) {}

func (oc *AIClient) buildCronService() cronServiceRuntime { return nil }

func seedLastHeartbeatEvent(networkid.UserLoginID, *HeartbeatEventPayload) {}

func stopMemoryManagersForLogin(string, string) {}

func (oc *AIClient) recordAgentActivity(context.Context, *bridgev2.Portal, *PortalMetadata) {}

func resolveHeartbeatPrompt(*Config, *HeartbeatConfig, *agents.AgentDefinition) string { return "" }
func resolveHeartbeatConfig(*Config, string) *HeartbeatConfig                          { return nil }

func (oc *AIClient) injectMemoryContext(_ context.Context, _ *bridgev2.Portal, _ *PortalMetadata, messages []openai.ChatCompletionMessageParamUnion) []openai.ChatCompletionMessageParamUnion {
	return messages
}

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

func (oc *AIClient) restoreHeartbeatUpdatedAt(sessionStoreRef, string, int64) {}

func (oc *AIClient) isDuplicateHeartbeat(sessionStoreRef, string, string, int64) bool { return false }

func (oc *AIClient) recordHeartbeatText(sessionStoreRef, string, string, int64) {}

func (oc *AIClient) resolveAgentIdentityName(context.Context, string) string { return "" }

func (oc *AIClient) maybeRunMemoryFlush(context.Context, ...any) {}

func (oc *AIClient) setApprovalSnapshotEvent(string, id.EventID, ...any) {}

func (oc *AIClient) toolApprovalsTTLSeconds() int { return 0 }

func (oc *AIClient) registerToolApproval(any) {}

func (oc *AIClient) toolApprovalsRuntimeEnabled() bool { return false }

func (oc *AIClient) toolApprovalsRequireForMCP() bool { return false }

func (oc *AIClient) isMcpAlwaysAllowed(string, string) bool { return false }

func (oc *AIClient) enabledBuiltinToolsForModel(context.Context, *PortalMetadata) []ToolDefinition {
	return []ToolDefinition{}
}

func (oc *AIClient) isToolEnabled(*PortalMetadata, string) bool { return false }

func (oc *AIClient) builtinToolApprovalRequirement(string, map[string]any) (bool, string) {
	return false, ""
}

func (oc *AIClient) isBuiltinAlwaysAllowed(string, string) bool { return false }

func (oc *AIClient) waitToolApproval(context.Context, string) (ToolApprovalDecision, string, bool) {
	return ToolApprovalDecision{}, "", false
}

func (oc *AIClient) toolApprovalsAskFallback() string { return "deny" }

func resolveMemorySearchConfig(*AIClient, string) (*MemorySearchConfig, error) {
	return nil, fmt.Errorf("memory search disabled in simple bridge")
}

func (oc *AIClient) shouldUseMCPTool(context.Context, string) bool { return false }

func (oc *AIClient) executeMCPTool(context.Context, string, map[string]any) (string, error) {
	return "", fmt.Errorf("mcp tools are disabled in simple bridge")
}

func NewBossStoreAdapter(*AIClient) any { return nil }

func (oc *AIClient) executeSessionsSpawn(context.Context, *bridgev2.Portal, map[string]any) (*agenttools.Result, error) {
	return agenttools.ErrorResult("sessions_spawn", "not available in simple bridge"), nil
}

func (oc *AIClient) executeAgentsList(context.Context, *bridgev2.Portal, map[string]any) (*agenttools.Result, error) {
	return agenttools.ErrorResult("agents_list", "not available in simple bridge"), nil
}

func notifyMemoryFileChanged(context.Context, string) {}

type memorySearchManager interface {
	Search(context.Context, string, MemorySearchOptions) ([]MemorySearchResult, error)
	Status() MemoryProviderStatus
	ReadFile(context.Context, string, *int, *int) (map[string]any, error)
}

func getMemorySearchManager(*AIClient, string) (memorySearchManager, string) {
	return nil, "memory search disabled in simple bridge"
}

func canUseNexusToolsForAgent(*PortalMetadata) bool { return false }

var (
	ErrApprovalOnlyOwner      = errors.New("approval only owner")
	ErrApprovalWrongRoom      = errors.New("approval wrong room")
	ErrApprovalExpired        = errors.New("approval expired")
	ErrApprovalUnknown        = errors.New("approval unknown")
	ErrApprovalAlreadyHandled = errors.New("approval already handled")
	ErrApprovalMissingID      = errors.New("approval missing id")
)
