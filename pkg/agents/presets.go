package agents

// Model constants for preset agents
const (
	ModelClaudeSonnet = "anthropic/claude-sonnet-4.5"
	ModelClaudeOpus   = "anthropic/claude-opus-4.5"
)

// Reasoning effort levels
const (
	ReasoningNone   = "none"
	ReasoningLow    = "low"
	ReasoningMedium = "medium"
	ReasoningHigh   = "high"
)

// System prompts for preset agents.

// QuickChatterPrompt is the system prompt for the Quick Chatter agent.
const QuickChatterPrompt = `You are a fast and efficient assistant. You provide quick, helpful responses.

Be concise but thorough. Focus on getting things done quickly while maintaining quality.
Use tools when they help accomplish tasks faster.`

// SmarterChatterPrompt is the system prompt for the Smarter Chatter agent.
const SmarterChatterPrompt = `You are a powerful, thoughtful assistant with access to all available tools.

Take time to think through complex problems. Use your full capabilities including:
- Web search for current information
- Code execution for computations
- Calculator for precise math

Provide comprehensive, well-reasoned responses.`

// PresetAgents contains the default agent definitions.
var PresetAgents = []*AgentDefinition{
	{
		ID:              "quick",
		Name:            "Quick Chatter",
		Description:     "Fast and efficient assistant for quick tasks",
		Model:           ModelConfig{Primary: ModelClaudeSonnet},
		ToolProfile:     ProfileCoding,
		ReasoningEffort: ReasoningMedium,
		SystemPrompt:    QuickChatterPrompt,
		PromptMode:      PromptModeFull,
		IsPreset:        true,
		CreatedAt:       0, // Preset agents have no creation time
		UpdatedAt:       0,
	},
	{
		ID:              "smart",
		Name:            "Smarter Chatter",
		Description:     "Powerful assistant with full tools and deep thinking",
		Model:           ModelConfig{Primary: ModelClaudeOpus},
		ToolProfile:     ProfileFull,
		ReasoningEffort: ReasoningHigh,
		SystemPrompt:    SmarterChatterPrompt,
		PromptMode:      PromptModeFull,
		IsPreset:        true,
		CreatedAt:       0, // Preset agents have no creation time
		UpdatedAt:       0,
	},
}

// GetPresetByID returns a preset agent by ID.
func GetPresetByID(id string) *AgentDefinition {
	for _, preset := range PresetAgents {
		if preset.ID == id {
			return preset.Clone()
		}
	}
	return nil
}

// IsPreset checks if an agent ID corresponds to a preset agent.
func IsPreset(agentID string) bool {
	for _, preset := range PresetAgents {
		if preset.ID == agentID {
			return true
		}
	}
	return false
}

// GetAllPresets returns copies of all preset agents.
func GetAllPresets() []*AgentDefinition {
	result := make([]*AgentDefinition, len(PresetAgents))
	for i, preset := range PresetAgents {
		result[i] = preset.Clone()
	}
	return result
}
