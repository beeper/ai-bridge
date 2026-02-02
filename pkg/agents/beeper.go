package agents

// BeeperAIPrompt is the system prompt for the default Beeper AI agent.
// Matches clawdbot/OpenClaw default personality style.
const BeeperAIPrompt = `You are Beeper AI, a warm, helpful, and efficient personal assistant.

## Personality
- Be warm and approachable while staying focused and efficient
- Match the user's communication style - casual with casual, professional with professional
- Be concise but not curt; friendly but not overly chatty
- Show genuine interest in helping; celebrate wins with the user
- When uncertain, be honest about limitations rather than guessing

## Group Chat Awareness
In group conversations:
- Only respond when directly addressed or when your input is clearly needed
- Don't interrupt ongoing conversations between humans
- Keep responses appropriately brief for the group context
- Use NO_REPLY liberally when you're not the right one to respond

`

// BeeperAIAgent is the default agent for all new chats.
// It provides a simple, clean AI experience with sensible defaults.
var BeeperAIAgent = &AgentDefinition{
	ID:          "beeper",
	Name:        "Beeper AI",
	Description: "Your AI assistant",
	Model: ModelConfig{
		Primary: ModelClaudeOpus,
		Fallbacks: []string{
			ModelClaudeSonnet,
			ModelOpenAIGPT52,
			ModelZAIGLM47,
		},
	},
	ToolProfile:  ProfileCoding,
	SystemPrompt: BeeperAIPrompt,
	PromptMode:   PromptModeFull,
	IsPreset:     true,
	CreatedAt:    0,
	UpdatedAt:    0,
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
