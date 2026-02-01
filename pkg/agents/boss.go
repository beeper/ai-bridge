package agents

import "time"

// BossAgent is the special agent that manages other agents and rooms.
// This is the "Meta Chatter" - uses Claude Opus with medium thinking.
var BossAgent = &AgentDefinition{
	ID:              "boss",
	Name:            "Meta Chatter",
	Description:     "Manages agents, rooms, and system configuration",
	Model:           ModelConfig{Primary: ModelClaudeOpus},
	ToolProfile:     ProfileBoss,
	ReasoningEffort: ReasoningMedium,
	SystemPrompt:    BossSystemPrompt,
	PromptMode:      PromptModeFull,
	IsPreset:        true,
	CreatedAt:       time.Now().Unix(),
	UpdatedAt:       time.Now().Unix(),
}

// GetBossAgent returns a copy of the Boss agent definition.
func GetBossAgent() *AgentDefinition {
	return BossAgent.Clone()
}

// IsBossAgent checks if an agent ID is the Boss agent.
func IsBossAgent(agentID string) bool {
	return agentID == "boss"
}
