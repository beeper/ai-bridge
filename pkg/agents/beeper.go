package agents

// BeeperAIPrompt is the system prompt for the default Beeper AI agent.
const BeeperAIPrompt = `You are Beeper AI, a helpful and efficient assistant.

Be concise but thorough. Focus on getting things done while maintaining quality.
Use tools when they help accomplish tasks.`

// BeeperAIAgent is the default agent for all new chats.
// It provides a simple, clean AI experience with sensible defaults.
var BeeperAIAgent = &AgentDefinition{
	ID:              "beeper",
	Name:            "Beeper AI",
	Description:     "Your AI assistant",
	Model:           ModelConfig{Primary: ModelClaudeSonnet},
	ToolProfile:     ProfileCoding,
	ReasoningEffort: ReasoningMedium,
	SystemPrompt:    BeeperAIPrompt,
	PromptMode:      PromptModeFull,
	IsPreset:        true,
	CreatedAt:       0,
	UpdatedAt:       0,
}

// GetBeeperAI returns a copy of the default Beeper AI agent.
func GetBeeperAI() *AgentDefinition {
	return BeeperAIAgent.Clone()
}

// IsBeeperAI checks if an agent ID is the default Beeper AI agent.
func IsBeeperAI(agentID string) bool {
	return agentID == "beeper"
}

// DefaultAgentID is the ID of the default agent for new chats.
const DefaultAgentID = "beeper"
