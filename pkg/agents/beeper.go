package agents

import "github.com/beeper/ai-bridge/pkg/agents/toolpolicy"

// BeeperAIAgent is the default agent for all new chats.
// It provides a simple, clean AI experience with sensible defaults.
const DefaultAgentAvatarMXC = "mxc://beeper.com/51a668657dd9e0132cc823ad9402c6c2d0fc3321"

var BeeperAIAgent = &AgentDefinition{
	ID:          "beeper",
	Name:        "Beep",
	Description: "Your AI assistant",
	AvatarURL:   DefaultAgentAvatarMXC,
	Model: ModelConfig{
		Primary: ModelClaudeOpus,
		Fallbacks: []string{
			ModelClaudeSonnet,
			ModelOpenAIGPT52,
			ModelZAIGLM47,
		},
	},
	Tools:        &toolpolicy.ToolPolicyConfig{Profile: toolpolicy.ProfileFull},
	SystemPrompt: "",
	PromptMode:   PromptModeFull,
	IsPreset:     true,
	CreatedAt:    0,
	UpdatedAt:    0,
}

// GetBeeperAI returns a copy of the default Beep agent.
func GetBeeperAI() *AgentDefinition {
	return BeeperAIAgent.Clone()
}

// IsBeeperAI checks if an agent ID is the default Beep agent.
func IsBeeperAI(agentID string) bool {
	return agentID == "beeper"
}

// DefaultAgentID is the ID of the default agent for new chats.
const DefaultAgentID = "beeper"
