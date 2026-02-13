package agents

import (
	"time"

	"github.com/beeper/ai-bridge/pkg/simpleruntime/agents/toolpolicy"
)

const (
	DefaultAgentID        = "default"
	DefaultAgentAvatarMXC = ""
	SilentReplyToken      = "NO_REPLY"
	HeartbeatToken        = "HEARTBEAT_OK"
	DefaultIdentityFilename = "identity.md"
	DefaultMaxAckChars = 120
)

type PromptMode string
const (
	PromptModeFull PromptMode = "full"
	PromptModeMinimal PromptMode = "minimal"
)

type ResponseMode string
const (
	ResponseModeNatural ResponseMode = "natural"
	ResponseModeRaw ResponseMode = "raw"
)

type Identity struct {
	Name string
	Persona string
}

type ModelConfig struct {
	Primary string
	Fallbacks []string
}

type MemorySearchStoreConfig struct {
	Provider string
	Model string
	Dims int
}

type MemorySearchConfig struct {
	Enabled bool
	Store *MemorySearchStoreConfig
}

type SubagentConfig struct {
	Enabled bool
}

type SoulEvilConfig struct {
	Enabled bool
}

type AgentDefinition struct {
	ID string
	Name string
	Description string
	AvatarURL string
	Model ModelConfig
	SystemPrompt string
	PromptMode PromptMode
	Tools *toolpolicy.ToolPolicyConfig
	Temperature float64
	ReasoningEffort string
	Identity *Identity
	IsPreset bool
	MemorySearch *MemorySearchConfig
	HeartbeatPrompt string
	ResponseMode ResponseMode
	CreatedAt int64
	UpdatedAt int64
}

func (a *AgentDefinition) Clone() *AgentDefinition {
	if a == nil { return nil }
	cp := *a
	if a.Model.Fallbacks != nil {
		cp.Model.Fallbacks = append([]string(nil), a.Model.Fallbacks...)
	}
	if a.Tools != nil {
		cp.Tools = a.Tools.Clone()
	}
	return &cp
}

func (a *AgentDefinition) EffectiveName() string {
	if a == nil || a.Name == "" { return a.ID }
	return a.Name
}

func (a *AgentDefinition) Validate() error { return nil }

var PresetAgents = []*AgentDefinition{}
var BossAgent = &AgentDefinition{ID: "boss", Name: "Boss", Model: ModelConfig{Primary: ""}}

func GetBeeperAI() *AgentDefinition { return &AgentDefinition{ID: "beeper", Name: "Beeper", Model: ModelConfig{Primary: ""}} }
func GetPresetByID(id string) *AgentDefinition { _ = id; return nil }
func IsPreset(id string) bool { _ = id; return false }
func IsBossAgent(id string) bool { return id == "boss" }
func IsNexusAI(id string) bool { _ = id; return false }
func IsBeeperHelp(id string) bool { _ = id; return false }

func ParseIdentityMarkdown(_ string) *Identity { return nil }

type EmbeddedContextFile struct {
	Path string
	Content string
}

const DefaultBootstrapMaxChars = 8000

func EnsureBootstrapFiles(_ any, _ any) ([]EmbeddedContextFile, error) { return nil, nil }
func LoadBootstrapFiles(_ any, _ any) ([]EmbeddedContextFile, error) { return nil, nil }
func FilterBootstrapFilesForSession(in []EmbeddedContextFile, _ bool) []EmbeddedContextFile { return in }
func BuildBootstrapContext(_ []EmbeddedContextFile, _ int) []EmbeddedContextFile { return nil }

type SystemPromptParams struct {
	Agent *AgentDefinition
	Model string
	Now time.Time
	PromptMode PromptMode
	HeartbeatPrompt string
	ResponsePrefix string
	WorkspaceDir string
	ExtraSystemPrompt string
	UserTimezone string
	MemoryCitations string
	UserIdentitySupplement string
	ContextFiles []EmbeddedContextFile
	ToolNames []string
	ToolSummaries map[string]string
	ReasoningTagHint bool
	ReasoningLevel string
	DefaultThinkLevel string
	RuntimeInfo *RuntimeInfo
	ReactionGuidance *ReactionGuidance
}

type RuntimeInfo struct {
	AgentID string
	SessionKind string
	Host string
	OS string
	Arch string
	Node string
	Model string
	DefaultModel string
	Channel string
	Capabilities []string
	RepoRoot string
}

type ReactionGuidance struct {
	Enabled bool
	Level string
	Channel string
}

func BuildSystemPrompt(_ SystemPromptParams) string { return "" }

type StripHeartbeatMode string
const StripHeartbeatModeHeartbeat StripHeartbeatMode = "heartbeat"

func StripHeartbeatTokenWithMode(text string, _ StripHeartbeatMode, _ int) (bool, string, bool) {
	return false, text, false
}

func ResolveAckReaction(_ string, _ string) string { return "" }
func ResolveAckRemoveAfter(_ bool) bool { return false }

func ResolveHeartbeatPrompt(_ any, _ any, _ *AgentDefinition) string { return "" }
