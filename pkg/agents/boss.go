package agents

// BossAgent is the special agent that manages other agents and rooms.
// This is the "Meta Chatter" - uses Claude Opus with medium thinking.
var BossAgent = &AgentDefinition{
	ID:          "boss",
	Name:        "Meta Chatter",
	Description: "Manages agents, rooms, and system configuration",
	Model: ModelConfig{
		Primary: ModelClaudeOpus,
		Fallbacks: []string{
			ModelClaudeSonnet,
			ModelOpenAIGPT52,
			ModelZAIGLM47,
		},
	},
	ToolProfile:  ProfileBoss,
	SystemPrompt: BossSystemPrompt,
	PromptMode:   PromptModeFull,
	IsPreset:     true,
	CreatedAt:    0, // Preset agents have no creation time
	UpdatedAt:    0,
}

// GetBossAgent returns a copy of the Boss agent definition.
func GetBossAgent() *AgentDefinition {
	return BossAgent.Clone()
}

// IsBossAgent checks if an agent ID is the Boss agent.
func IsBossAgent(agentID string) bool {
	return agentID == "boss"
}
