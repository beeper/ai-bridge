package agents

import (
	"context"
	"errors"

	policypkg "github.com/beeper/ai-bridge/pkg/simpleruntime/simpleagent/toolpolicy"
	toolspkg "github.com/beeper/ai-bridge/pkg/simpleruntime/simpleagent/tools"
)

type ModelInfo struct {
	ID          string   `json:"id,omitempty"`
	Name        string   `json:"name,omitempty"`
	Provider    string   `json:"provider,omitempty"`
	Description string   `json:"description,omitempty"`
	Primary     string   `json:"primary,omitempty"`
	Fallbacks   []string `json:"fallbacks,omitempty"`
}

type Identity struct {
	Name            string   `json:"name,omitempty"`
	DisplayName     string   `json:"display_name,omitempty"`
	Persona         string   `json:"persona,omitempty"`
	MentionPatterns []string `json:"mention_patterns,omitempty"`
}

type AgentDefinition struct {
	ID              string                      `json:"id,omitempty"`
	Name            string                      `json:"name,omitempty"`
	Description     string                      `json:"description,omitempty"`
	AvatarURL       string                      `json:"avatar_url,omitempty"`
	Model           ModelInfo                   `json:"model,omitempty"`
	Identity        *Identity                   `json:"identity,omitempty"`
	SystemPrompt    string                      `json:"system_prompt,omitempty"`
	Tools           *policypkg.ToolPolicyConfig `json:"tools,omitempty"`
	Subagents       *SubagentConfig             `json:"subagents,omitempty"`
	IsPreset        bool                        `json:"is_preset,omitempty"`
	PromptMode      PromptMode                  `json:"prompt_mode,omitempty"`
	ResponseMode    ResponseMode                `json:"response_mode,omitempty"`
	Temperature     float64                     `json:"temperature,omitempty"`
	ReasoningEffort string                      `json:"reasoning_effort,omitempty"`
	HeartbeatPrompt string                      `json:"heartbeat_prompt,omitempty"`
	MemorySearch    *MemorySearchConfig         `json:"memory_search,omitempty"`
	CreatedAt       int64                       `json:"created_at,omitempty"`
	UpdatedAt       int64                       `json:"updated_at,omitempty"`
}

type EmbeddedContextFile struct {
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
}

type SubagentConfig struct {
	Model       string   `json:"model,omitempty"`
	Thinking    string   `json:"thinking,omitempty"`
	AllowAgents []string `json:"allowAgents,omitempty"`
}

const DefaultAgentID = "default"

func (a *AgentDefinition) Clone() *AgentDefinition {
	if a == nil {
		return nil
	}
	out := *a
	return &out
}

func (a *AgentDefinition) EffectiveName() string {
	if a == nil {
		return "Agent"
	}
	if a.Name != "" {
		return a.Name
	}
	if a.Identity != nil {
		if a.Identity.DisplayName != "" {
			return a.Identity.DisplayName
		}
		if a.Identity.Name != "" {
			return a.Identity.Name
		}
	}
	if a.ID != "" {
		return a.ID
	}
	return "Agent"
}

func IsBossAgent(agentID string) bool { return agentID == "boss" }
func IsNexusAI(agentID string) bool   { return agentID == "nexus" }

func BuildSubagentSystemPrompt(SubagentPromptParams) string { return "" }

type SubagentPromptParams struct {
	RequesterSessionKey string
	RequesterChannel    string
	ChildSessionKey     string
	Label               string
	Task                string
}

type SoulEvilConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

type MemorySearchConfig struct {
	Enabled      *bool                     `json:"enabled,omitempty"`
	Provider     string                    `json:"provider,omitempty"`
	Model        string                    `json:"model,omitempty"`
	BaseURL      string                    `json:"base_url,omitempty"`
	APIKey       string                    `json:"api_key,omitempty"`
	Citations    string                    `json:"citations,omitempty"`
	Fallback     string                    `json:"fallback,omitempty"`
	Sources      []string                  `json:"sources,omitempty"`
	ExtraPaths   []string                  `json:"extra_paths,omitempty"`
	Experimental *MemorySearchExperimental `json:"experimental,omitempty"`
	Store        *MemorySearchStore        `json:"store,omitempty"`
	Chunking     *MemorySearchChunking     `json:"chunking,omitempty"`
	Sync         *MemorySearchSync         `json:"sync,omitempty"`
	Query        *MemorySearchQuery        `json:"query,omitempty"`
	Cache        *MemorySearchCache        `json:"cache,omitempty"`
	Remote       *MemorySearchRemote       `json:"remote,omitempty"`
}

type MemorySearchExperimental struct {
	SessionMemory *bool `json:"session_memory,omitempty"`
}

type MemorySearchStore struct {
	Vector *MemorySearchStoreVector `json:"vector,omitempty"`
}

type MemorySearchStoreVector struct {
	Enabled       *bool  `json:"enabled,omitempty"`
	ExtensionPath string `json:"extension_path,omitempty"`
}

type MemorySearchChunking struct {
	Tokens  int `json:"tokens,omitempty"`
	Overlap int `json:"overlap,omitempty"`
}

type MemorySearchSync struct {
	OnSessionStart  *bool                     `json:"on_session_start,omitempty"`
	OnSearch        *bool                     `json:"on_search,omitempty"`
	Watch           *bool                     `json:"watch,omitempty"`
	WatchDebounceMs int                       `json:"watch_debounce_ms,omitempty"`
	IntervalMinutes int                       `json:"interval_minutes,omitempty"`
	Sessions        *MemorySearchSyncSessions `json:"sessions,omitempty"`
}

type MemorySearchSyncSessions struct {
	DeltaBytes    int `json:"delta_bytes,omitempty"`
	DeltaMessages int `json:"delta_messages,omitempty"`
	RetentionDays int `json:"retention_days,omitempty"`
}

type MemorySearchQuery struct {
	MaxResults       int                      `json:"max_results,omitempty"`
	MinScore         float64                  `json:"min_score,omitempty"`
	MaxInjectedChars int                      `json:"max_injected_chars,omitempty"`
	Hybrid           *MemorySearchQueryHybrid `json:"hybrid,omitempty"`
}

type MemorySearchQueryHybrid struct {
	Enabled             *bool   `json:"enabled,omitempty"`
	VectorWeight        float64 `json:"vector_weight,omitempty"`
	TextWeight          float64 `json:"text_weight,omitempty"`
	CandidateMultiplier int     `json:"candidate_multiplier,omitempty"`
}

type MemorySearchCache struct {
	Enabled    *bool `json:"enabled,omitempty"`
	MaxEntries int   `json:"max_entries,omitempty"`
}

type MemorySearchRemote struct {
	BaseURL string                   `json:"base_url,omitempty"`
	APIKey  string                   `json:"api_key,omitempty"`
	Headers map[string]string        `json:"headers,omitempty"`
	Batch   *MemorySearchRemoteBatch `json:"batch,omitempty"`
}

type MemorySearchRemoteBatch struct {
	Enabled        *bool `json:"enabled,omitempty"`
	Wait           *bool `json:"wait,omitempty"`
	Concurrency    int   `json:"concurrency,omitempty"`
	PollIntervalMs int   `json:"poll_interval_ms,omitempty"`
	TimeoutMinutes int   `json:"timeout_minutes,omitempty"`
}

type AgentStore interface {
	LoadAgents(context.Context) (map[string]*AgentDefinition, error)
	SaveAgent(context.Context, *AgentDefinition) error
	DeleteAgent(context.Context, string) error
	ListModels(context.Context) ([]ModelInfo, error)
	ListAvailableTools(context.Context) ([]toolspkg.ToolInfo, error)
}

type ResponseMode string
type PromptMode string

type ModelConfig = ModelInfo

const (
	PromptModeFull    PromptMode = "full"
	PromptModeMinimal PromptMode = "minimal"
	PromptModeNone    PromptMode = "none"
)

const (
	ResponseModeNatural ResponseMode = "natural"
	ResponseModeRaw     ResponseMode = "raw"
)

type ReactionGuidance struct {
	Level   string `json:"level,omitempty"`
	Channel string `json:"channel,omitempty"`
}

type RuntimeInfo struct {
	AgentID      string   `json:"agent_id,omitempty"`
	Host         string   `json:"host,omitempty"`
	OS           string   `json:"os,omitempty"`
	Arch         string   `json:"arch,omitempty"`
	Node         string   `json:"node,omitempty"`
	Model        string   `json:"model,omitempty"`
	DefaultModel string   `json:"default_model,omitempty"`
	Channel      string   `json:"channel,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	RepoRoot     string   `json:"repo_root,omitempty"`
}

type SystemPromptParams struct {
	WorkspaceDir           string                `json:"workspace_dir,omitempty"`
	ExtraSystemPrompt      string                `json:"extra_system_prompt,omitempty"`
	UserTimezone           string                `json:"user_timezone,omitempty"`
	PromptMode             PromptMode            `json:"prompt_mode,omitempty"`
	HeartbeatPrompt        string                `json:"heartbeat_prompt,omitempty"`
	MemoryCitations        string                `json:"memory_citations,omitempty"`
	UserIdentitySupplement string                `json:"user_identity_supplement,omitempty"`
	ContextFiles           []EmbeddedContextFile `json:"context_files,omitempty"`
	ToolNames              []string              `json:"tool_names,omitempty"`
	ToolSummaries          map[string]string     `json:"tool_summaries,omitempty"`
	RuntimeInfo            *RuntimeInfo          `json:"runtime_info,omitempty"`
	ReactionGuidance       *ReactionGuidance     `json:"reaction_guidance,omitempty"`
	ReasoningTagHint       bool                  `json:"reasoning_tag_hint,omitempty"`
	ReasoningLevel         string                `json:"reasoning_level,omitempty"`
	DefaultThinkLevel      string                `json:"default_think_level,omitempty"`
}

const (
	SilentReplyToken         = "NO_REPLY"
	HeartbeatToken           = "HEARTBEAT_OK"
	DefaultIdentityFilename  = "IDENTITY.md"
	DefaultAgentAvatarMXC    = ""
	BossSystemPrompt         = ""
	DefaultHeartbeatEvery    = "5m"
	DefaultMaxAckChars       = 512
	DefaultHeartbeatFilename = "HEARTBEAT.md"
)

var ErrAgentIsPreset = errors.New("preset agent")
var ErrAgentNotFound = errors.New("agent not found")

var BossAgent = &AgentDefinition{ID: "boss", Name: "Boss", IsPreset: true}
var BeeperAI = &AgentDefinition{ID: DefaultAgentID, Name: "AI", IsPreset: true}

var PresetAgents = map[string]*AgentDefinition{
	"boss":    BossAgent,
	"default": BeeperAI,
}

func IsPreset(agentID string) bool {
	_, ok := PresetAgents[agentID]
	return ok
}

func IsBeeperHelp(agentID string) bool { return agentID == "beeper-help" }

func ParseIdentityMarkdown(_ string) *Identity { return nil }

func (a *AgentDefinition) Validate() error { return nil }

func GetBossAgent() *AgentDefinition { return BossAgent.Clone() }

func GetPresetByID(agentID string) *AgentDefinition {
	if a, ok := PresetAgents[agentID]; ok {
		return a.Clone()
	}
	return nil
}

func GetBeeperAI() *AgentDefinition { return BeeperAI.Clone() }

func BuildSystemPrompt(SystemPromptParams) string { return "" }

type StripHeartbeatMode string

const StripHeartbeatModeHeartbeat StripHeartbeatMode = "heartbeat"

func StripHeartbeatTokenWithMode(text string, _ StripHeartbeatMode, _ int) (bool, string, bool) {
	return false, text, false
}

func ResolveHeartbeatPrompt(raw string) string { return raw }

func IsHeartbeatContentEffectivelyEmpty(raw string) bool { return len(raw) == 0 }
