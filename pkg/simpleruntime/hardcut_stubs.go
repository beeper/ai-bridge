package connector

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/rs/zerolog"

	agents "github.com/beeper/ai-bridge/pkg/simpleruntime/simpleagent"
	agenttools "github.com/beeper/ai-bridge/pkg/simpleruntime/simpleagent/tools"
	cron "github.com/beeper/ai-bridge/pkg/simpleruntime/simplecron"
	memory "github.com/beeper/ai-bridge/pkg/simpleruntime/simplememory"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

type subagentRun struct{}

type pendingToolApproval struct{}

type HeartbeatWake struct {
	log zerolog.Logger
}

func (h *HeartbeatWake) Request(string, int64) {}

type HeartbeatRunner struct{}

func NewHeartbeatRunner(*AIClient) *HeartbeatRunner { return &HeartbeatRunner{} }
func (h *HeartbeatRunner) Start()                   {}
func (h *HeartbeatRunner) Stop()                    {}
func (h *HeartbeatRunner) run(string) cron.HeartbeatRunResult {
	return cron.HeartbeatRunResult{Status: "skipped", Reason: "disabled"}
}

type HeartbeatRunConfig struct {
	StoreAgentID  string
	StorePath     string
	SessionKey    string
	TargetRoom    id.RoomID
	TargetReason  string
	Reason        string
	Channel       string
	ResponsePrefix string
	ShowOk        bool
	ShowAlerts    bool
	UseIndicator  bool
	IncludeReasoning bool
	ExecEvent     bool
	SuppressSave  bool
	SuppressSend  bool
	PrevUpdatedAt int64
	AckMaxChars   int
}

type HeartbeatRunOutcome struct {
	Status  string
	Reason  string
	Text    string
	Sent    bool
	Skipped bool
	Silent  bool
}

type heartbeatRunContext struct {
	Config   *HeartbeatRunConfig
	ResultCh chan HeartbeatRunOutcome
}

type HeartbeatIndicatorType string

const (
	HeartbeatIndicatorSent    HeartbeatIndicatorType = "sent"
	HeartbeatIndicatorSkipped HeartbeatIndicatorType = "skipped"
)

type HeartbeatEventPayload struct {
	TS            int64                  `json:"ts,omitempty"`
	Status        string                 `json:"status,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
	To            string                 `json:"to,omitempty"`
	Preview       string                 `json:"preview,omitempty"`
	Channel       string                 `json:"channel,omitempty"`
	Silent        bool                   `json:"silent,omitempty"`
	HasMedia      bool                   `json:"has_media,omitempty"`
	DurationMs    int64                  `json:"duration_ms,omitempty"`
	IndicatorType *HeartbeatIndicatorType `json:"indicator_type,omitempty"`
}

func resolveIndicatorType(string) *HeartbeatIndicatorType {
	v := HeartbeatIndicatorSkipped
	return &v
}

func heartbeatRunFromContext(context.Context) *heartbeatRunContext { return nil }

func (oc *AIClient) emitHeartbeatEvent(evt *HeartbeatEventPayload) {
	if oc == nil || oc.UserLogin == nil || evt == nil {
		return
	}
	meta := loginMetadata(oc.UserLogin)
	meta.LastHeartbeatEvent = evt
	_ = oc.UserLogin.Save(context.Background())
}

func getLastHeartbeatEventForLogin(login *bridgev2.UserLogin) *HeartbeatEventPayload {
	if login == nil {
		return nil
	}
	meta := loginMetadata(login)
	return meta.LastHeartbeatEvent
}

func (oc *AIClient) stopSubagentRuns(id.RoomID) int {
	return 0
}

func purgeLoginDataBestEffort(context.Context, *bridgev2.UserLogin) {}

func (oc *AIClient) resolveAgentDisplayName(_ context.Context, agent *agents.AgentDefinition) string {
	if agent == nil {
		return ""
	}
	if strings.TrimSpace(agent.Name) != "" {
		return strings.TrimSpace(agent.Name)
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

func canUseNexusToolsForAgent(*PortalMetadata) bool { return false }

const mcpDiscoveryTimeout = 2 * time.Second

var (
	ErrApprovalOnlyOwner      = errors.New("approval only owner")
	ErrApprovalWrongRoom      = errors.New("approval wrong room")
	ErrApprovalExpired        = errors.New("approval expired")
	ErrApprovalUnknown        = errors.New("approval unknown")
	ErrApprovalAlreadyHandled = errors.New("approval already handled")
	ErrApprovalMissingID      = errors.New("approval missing id")
)

func resolveHeartbeatConfig(*Config, string) *HeartbeatConfig { return nil }

func resolveHeartbeatPrompt(*Config, *HeartbeatConfig, *agents.AgentDefinition) string { return "" }

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

func formatCronTime(string) string { return "" }

func (oc *AIClient) restoreHeartbeatUpdatedAt(sessionStoreRef, string, int64) {}

func (oc *AIClient) isDuplicateHeartbeat(sessionStoreRef, string, string, int64) bool { return false }

func (oc *AIClient) recordHeartbeatText(sessionStoreRef, string, string, int64) {}

func (oc *AIClient) resolveAgentIdentityName(context.Context, string) string {
	return ""
}

func (oc *AIClient) maybeRunMemoryFlush(context.Context, ...any) {}

func (oc *AIClient) setApprovalSnapshotEvent(string, id.EventID, ...any) {}

func (oc *AIClient) toolApprovalsTTLSeconds() int { return 0 }

type ToolApprovalKind string

const (
	ToolApprovalKindMCP     ToolApprovalKind = "mcp"
	ToolApprovalKindBuiltin ToolApprovalKind = "builtin"
)

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
	return nil, errors.New("memory search disabled in simple bridge")
}

func (oc *AIClient) shouldUseMCPTool(context.Context, string) bool { return false }

func (oc *AIClient) executeMCPTool(context.Context, string, map[string]any) (string, error) {
	return "", errors.New("mcp tools are disabled in simple bridge")
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
	Search(context.Context, string, memory.SearchOptions) ([]memory.SearchResult, error)
	Status() memory.ProviderStatus
	ReadFile(context.Context, string, *int, *int) (map[string]any, error)
}

func getMemorySearchManager(*AIClient, string) (memorySearchManager, string) {
	return nil, "memory search disabled in simple bridge"
}

const memorySearchTimeout = 3 * time.Second

func (oc *AIClient) notifySessionMemoryChange(context.Context, *bridgev2.Portal, *PortalMetadata, bool) {}

func (oc *AIClient) buildCronService() *cron.CronService { return nil }

func seedLastHeartbeatEvent(networkid.UserLoginID, *HeartbeatEventPayload) {}

func stopMemoryManagersForLogin(string, string) {}

func (oc *AIClient) recordAgentActivity(context.Context, *bridgev2.Portal, *PortalMetadata) {}

type ToolApprovalDecision struct {
	Approve   bool
	Always    bool
	Reason    string
	DecidedAt time.Time
	DecidedBy id.UserID
}

func (oc *AIClient) resolveToolApproval(id.RoomID, string, ToolApprovalDecision) error { return nil }
