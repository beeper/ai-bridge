package agents

// PlaygroundAgent is a sandbox for direct model access with minimal tools.
// This is for advanced users who want raw model interaction without agent personality.
var PlaygroundAgent = &AgentDefinition{
	ID:          "playground",
	Name:        "Model Playground",
	Description: "Direct model access with minimal tools, no agent personality",
	Model: ModelConfig{
		Primary: ModelClaudeSonnet, // Default, but typically overridden by user
		Fallbacks: []string{
			ModelOpenAIGPT52,
			ModelZAIGLM47,
		},
	},
	ToolProfile:  ProfileMinimal,  // web search, calculator, chat info
	PromptMode:   PromptModeNone,  // no system prompt sections
	ResponseMode: ResponseModeRaw, // no directive processing
	IsPreset:     true,
	CreatedAt:    0,
	UpdatedAt:    0,
}

// GetPlaygroundAgent returns a copy of the Playground agent definition.
func GetPlaygroundAgent() *AgentDefinition {
	return PlaygroundAgent.Clone()
}

// IsPlaygroundAgent checks if an agent ID is the Playground agent.
func IsPlaygroundAgent(agentID string) bool {
	return agentID == "playground"
}
