package connector

import "strings"

type AgentModelInfo struct {
	ID          string   `json:"id,omitempty"`
	Name        string   `json:"name,omitempty"`
	Provider    string   `json:"provider,omitempty"`
	Description string   `json:"description,omitempty"`
	Primary     string   `json:"primary,omitempty"`
	Fallbacks   []string `json:"fallbacks,omitempty"`
}

type AgentIdentity struct {
	Name            string   `json:"name,omitempty"`
	DisplayName     string   `json:"display_name,omitempty"`
	Persona         string   `json:"persona,omitempty"`
	MentionPatterns []string `json:"mention_patterns,omitempty"`
}

type AgentSubagentConfig struct {
	Model       string   `json:"model,omitempty"`
	Thinking    string   `json:"thinking,omitempty"`
	AllowAgents []string `json:"allowAgents,omitempty"`
}

type AgentDefinition struct {
	ID              string               `json:"id,omitempty"`
	Name            string               `json:"name,omitempty"`
	Description     string               `json:"description,omitempty"`
	AvatarURL       string               `json:"avatar_url,omitempty"`
	Model           AgentModelInfo       `json:"model,omitempty"`
	Identity        *AgentIdentity       `json:"identity,omitempty"`
	SystemPrompt    string               `json:"system_prompt,omitempty"`
	Tools           map[string]any       `json:"tools,omitempty"`
	Subagents       *AgentSubagentConfig `json:"subagents,omitempty"`
	PromptMode      string               `json:"prompt_mode,omitempty"`
	ResponseMode    string               `json:"response_mode,omitempty"`
	Temperature     float64              `json:"temperature,omitempty"`
	ReasoningEffort string               `json:"reasoning_effort,omitempty"`
	HeartbeatPrompt string               `json:"heartbeat_prompt,omitempty"`
	CreatedAt       int64                `json:"created_at,omitempty"`
	UpdatedAt       int64                `json:"updated_at,omitempty"`
}

func (a *AgentDefinition) Clone() *AgentDefinition {
	if a == nil {
		return nil
	}
	out := *a
	return &out
}

const (
	defaultAgentID = "default"
	heartbeatToken = "HEARTBEAT_OK"
)

func isBossAgent(agentID string) bool {
	return strings.EqualFold(strings.TrimSpace(agentID), "boss")
}

func getBossAgent() *AgentDefinition {
	return &AgentDefinition{ID: "boss", Name: "Boss"}
}

type AgentEmbeddedContextFile struct {
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
}
