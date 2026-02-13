package connector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/beeper/ai-bridge/pkg/simpleruntime/agents"
	"github.com/beeper/ai-bridge/pkg/simpleruntime/agents/toolpolicy"
	agenttools "github.com/beeper/ai-bridge/pkg/simpleruntime/agents/tools"
	"github.com/beeper/ai-bridge/pkg/simpleruntime/cron"
	"github.com/beeper/ai-bridge/pkg/simpleruntime/memory"
	"github.com/openai/openai-go/v3"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

type sessionStoreRef struct {
	AgentID string
	Path    string
}

type sessionListEntry struct {
	SessionKey      string `json:"session_key,omitempty"`
	StorePath       string `json:"store_path,omitempty"`
	UpdatedAt       int64  `json:"updated_at,omitempty"`
	updatedAt       int64
	data            map[string]any
	QueueMode       string
	QueueDebounceMs int
	QueueCap        int
	QueueDrop       string
}

type toolExecutor func(context.Context, map[string]any, *BridgeToolContext) (string, error)

type ToolAvailabilitySource = SettingSource

type AgentStoreAdapter struct{}

func NewAgentStoreAdapter(_ *AIClient) *AgentStoreAdapter { return &AgentStoreAdapter{} }

func (s *AgentStoreAdapter) LoadAgents(context.Context) (map[string]*agents.AgentDefinition, error) {
	_ = s
	return map[string]*agents.AgentDefinition{}, nil
}

func (s *AgentStoreAdapter) GetAgentByID(_ context.Context, _ string) (*agents.AgentDefinition, error) {
	return nil, errors.New("agent store disabled in simple runtime")
}

func (s *AgentStoreAdapter) SaveAgent(_ context.Context, _ *agents.AgentDefinition) error { return nil }

func (s *AgentStoreAdapter) DeleteAgent(_ context.Context, _ string) error { return nil }

func (oc *AIClient) stopSubagentRuns(id.RoomID) int { return 0 }

func (oc *AIClient) resolveAgentDisplayName(_ context.Context, agent *agents.AgentDefinition) string {
	if agent == nil {
		return "Agent"
	}
	return agent.EffectiveName()
}

func (oc *AIClient) agentDefaultModel(agent *agents.AgentDefinition) string {
	if agent == nil {
		return oc.effectiveModel(nil)
	}
	if agent.Model.Primary != "" {
		return ResolveAlias(agent.Model.Primary)
	}
	return oc.effectiveModel(nil)
}

func (oc *AIClient) toolNamesForPortal(*PortalMetadata) []string { return nil }

func (oc *AIClient) isToolAvailable(*PortalMetadata, string) (bool, ToolAvailabilitySource, string) {
	return false, SourceAgentPolicy, "disabled"
}

func (oc *AIClient) isToolAllowedByPolicy(*PortalMetadata, string) bool { return false }

func (oc *AIClient) setToolApproval(_ string, _ *pendingToolApproval)      {}
func (oc *AIClient) getToolApproval(_ string) (*pendingToolApproval, bool) { return nil, false }
func (oc *AIClient) clearToolApproval(_ string)                            {}

func (oc *AIClient) defaultToolPolicyConfig() *toolpolicy.ToolPolicyConfig { return nil }

func (oc *AIClient) availableAgentTools(_ *PortalMetadata) []*agenttools.Tool { return nil }

func (oc *AIClient) notifySessionMemoryChange(context.Context, *bridgev2.Portal, *PortalMetadata, bool) {
}

func restoreSystemEventsFromDisk(any, any) {}

type bridgeStateStore interface {
	Read(context.Context, string) ([]byte, bool, error)
	Write(context.Context, string, []byte) error
	Delete(context.Context, string) error
	List(context.Context, string) ([]stateEntry, error)
}

type stateEntry struct {
	Key  string
	Data []byte
}

type noopBridgeStateStore struct{}

func (noopBridgeStateStore) Read(context.Context, string) ([]byte, bool, error) {
	return nil, false, nil
}
func (noopBridgeStateStore) Write(context.Context, string, []byte) error        { return nil }
func (noopBridgeStateStore) Delete(context.Context, string) error               { return nil }
func (noopBridgeStateStore) List(context.Context, string) ([]stateEntry, error) { return nil, nil }

func (oc *AIClient) bridgeStateBackend() bridgeStateStore { return noopBridgeStateStore{} }

func (oc *AIClient) buildCronService() *cron.CronService { return nil }

func seedLastHeartbeatEvent(networkid.UserLoginID, *HeartbeatEventPayload) {}

func stopMemoryManagersForLogin(string, string) {}

func purgeMemoryManagersForLogin(string, string) {}

func (oc *AIClient) recordAgentActivity(context.Context, *bridgev2.Portal, *PortalMetadata) {}

func resolveHeartbeatConfig(*Config, string) *HeartbeatConfig { return nil }

func resolveHeartbeatPrompt(*Config, *HeartbeatConfig, *agents.AgentDefinition) string { return "" }

func (oc *AIClient) buildBootstrapContextFiles(context.Context, string, *PortalMetadata) []agents.EmbeddedContextFile {
	return nil
}

func resolveMemorySearchConfig(*AIClient, string) (*memory.ResolvedConfig, error) {
	return &memory.ResolvedConfig{Enabled: false}, nil
}

type extensionEnabler struct{}

func loadExtensionEnabler(string) (*extensionEnabler, error) { return nil, nil }

func purgeLoginDataBestEffort(context.Context, *bridgev2.UserLogin) {}

const mcpDiscoveryTimeout = 2 * time.Second
const memorySearchTimeout = 5 * time.Second

func (oc *AIClient) lookupMCPToolDefinition(context.Context, string) (*ToolDefinition, bool) {
	return nil, false
}

func (oc *AIClient) buildDesktopAccountHintPrompt(context.Context) string { return "" }

func normalizeAgentID(agentID string) string { return strings.TrimSpace(strings.ToLower(agentID)) }

func sanitizeDesktopInstanceKey(instance string) string {
	trimmed := strings.TrimSpace(strings.ToLower(instance))
	if trimmed == "" {
		return desktopDefaultInstance
	}
	var b strings.Builder
	wasUnderscore := false
	for _, r := range trimmed {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			wasUnderscore = false
			continue
		}
		if !wasUnderscore {
			b.WriteByte('_')
			wasUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return desktopDefaultInstance
	}
	return out
}

func normalizeDesktopBridgeType(network string) string {
	if out := canonicalDesktopNetwork(network); out != "" {
		return out
	}
	if out := normalizeDesktopNetworkToken(network); out != "" {
		return out
	}
	return "unknown"
}

func formatDesktopAccountID(areThereMultipleDesktopInstances bool, instanceKey, bridgeType, rawAccountID string) string {
	accountID := strings.TrimSpace(rawAccountID)
	if accountID == "" {
		return ""
	}
	bridge := normalizeDesktopBridgeType(bridgeType)
	if areThereMultipleDesktopInstances {
		instance := sanitizeDesktopInstanceKey(instanceKey)
		return fmt.Sprintf("%s_%s_%s", instance, bridge, accountID)
	}
	return fmt.Sprintf("%s_%s", bridge, accountID)
}

func (oc *AIClient) resolveSessionStoreRef(agentID string) sessionStoreRef {
	return sessionStoreRef{AgentID: normalizeAgentID(agentID)}
}

func (oc *AIClient) getSessionEntry(context.Context, sessionStoreRef, string) (sessionEntry, bool) {
	return sessionEntry{}, false
}

func formatCronTime(any) string { return "" }

func (oc *AIClient) injectMemoryContext(_ context.Context, _ *bridgev2.Portal, _ *PortalMetadata, messages []openai.ChatCompletionMessageParamUnion) []openai.ChatCompletionMessageParamUnion {
	return messages
}

func (oc *AIClient) restoreHeartbeatUpdatedAt(sessionStoreRef, string, int64) {}

func (oc *AIClient) isDuplicateHeartbeat(sessionStoreRef, string, string, int64) bool { return false }

func (oc *AIClient) recordHeartbeatText(sessionStoreRef, string, string, int64) {}

func (oc *AIClient) emitHeartbeatEvent(*HeartbeatEventPayload) {}

func resolveIndicatorType(string) *HeartbeatIndicatorType { return nil }

func (oc *AIClient) resolveAgentIdentityName(context.Context, string) string { return "" }

func (oc *AIClient) maybeRunMemoryFlush(context.Context, *bridgev2.Portal, *PortalMetadata, []openai.ChatCompletionMessageParamUnion) {
}

func getLastHeartbeatEventForLogin(*bridgev2.UserLogin) *HeartbeatEventPayload { return nil }

type heartbeatRunContext struct {
	Config   *HeartbeatRunConfig
	ResultCh chan HeartbeatRunOutcome
}

func heartbeatRunFromContext(context.Context) *heartbeatRunContext { return nil }

func (oc *AIClient) enabledBuiltinToolsForModel(context.Context, *PortalMetadata) []ToolDefinition {
	return nil
}

func (oc *AIClient) isBuilderRoom(*bridgev2.Portal) bool { return false }

func (oc *AIClient) isToolEnabled(*PortalMetadata, string) bool { return false }

func (oc *AIClient) executeBuiltinTool(context.Context, *bridgev2.Portal, string, string) (string, error) {
	return "", errors.New("tool execution unavailable in simple runtime")
}

func parseToolInputPayload(string) map[string]any { return nil }

func normalizeToolArgsJSON(string) string { return "{}" }

func buildBuiltinToolDefinitions() []ToolDefinition { return nil }

func notifyMemoryFileChanged(context.Context, string) {}

func maybeRefreshAgentIdentity(context.Context, string) {}

func getMemorySearchManager(*AIClient, string) (*memory.SearchManager, string) {
	return nil, "disabled"
}
