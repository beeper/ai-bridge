package agents

import "time"

// BossAgent is the special agent that manages other agents.
var BossAgent = &AgentDefinition{
	ID:           "boss",
	Name:         "Agent Builder",
	Description:  "I help you create and manage your AI agents",
	Model:        ModelConfig{Primary: ""}, // Uses provider default
	ToolProfile:  ProfileBoss,
	SystemPrompt: BossSystemPrompt,
	PromptMode:   PromptModeFull,
	IsPreset:     true,
	CreatedAt:    time.Now().Unix(),
	UpdatedAt:    time.Now().Unix(),
}

// GetBossAgent returns a copy of the Boss agent definition.
func GetBossAgent() *AgentDefinition {
	return BossAgent.Clone()
}

// IsBossAgent checks if an agent ID is the Boss agent.
func IsBossAgent(agentID string) bool {
	return agentID == "boss"
}
