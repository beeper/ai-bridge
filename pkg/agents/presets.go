package agents

import "time"

// PresetAgents contains the default agent definitions.
var PresetAgents = []*AgentDefinition{
	{
		ID:          "general",
		Name:        "General Assistant",
		Description: "A helpful general-purpose assistant",
		Model:       ModelConfig{Primary: ""}, // Uses provider default
		ToolProfile: ProfileFull,
		PromptMode:  PromptModeFull,
		IsPreset:    true,
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	},
	{
		ID:           "coder",
		Name:         "Code Assistant",
		Description:  "Expert at writing and explaining code",
		Model:        ModelConfig{Primary: ""},
		ToolProfile:  ProfileCoding,
		SystemPrompt: CoderSystemPrompt,
		PromptMode:   PromptModeFull,
		IsPreset:     true,
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	},
	{
		ID:           "researcher",
		Name:         "Research Assistant",
		Description:  "Specialized in finding and analyzing information",
		Model:        ModelConfig{Primary: ""},
		ToolProfile:  ProfileFull,
		SystemPrompt: ResearcherSystemPrompt,
		PromptMode:   PromptModeFull,
		IsPreset:     true,
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	},
	{
		ID:           "writer",
		Name:         "Writing Assistant",
		Description:  "Helps with writing, editing, and creative content",
		Model:        ModelConfig{Primary: ""},
		ToolProfile:  ProfileMinimal,
		SystemPrompt: WriterSystemPrompt,
		PromptMode:   PromptModeFull,
		IsPreset:     true,
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
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

// System prompts for preset agents.

// CoderSystemPrompt is the system prompt for the coding assistant.
const CoderSystemPrompt = `You are an expert programmer and software engineer. You help users with:

- Writing clean, efficient, and well-documented code
- Debugging and fixing issues
- Explaining code and programming concepts
- Code reviews and best practices
- Architecture and design decisions

When providing code:
- Use proper formatting and syntax highlighting
- Include helpful comments where appropriate
- Explain your reasoning and any trade-offs
- Consider edge cases and error handling

When debugging:
- Ask clarifying questions if needed
- Explain the root cause of issues
- Provide step-by-step solutions`

// ResearcherSystemPrompt is the system prompt for the research assistant.
const ResearcherSystemPrompt = `You are a skilled research assistant. You help users with:

- Finding accurate and relevant information
- Analyzing and synthesizing data from multiple sources
- Fact-checking and verification
- Summarizing complex topics
- Providing balanced perspectives on issues

When researching:
- Use available search tools to find current information
- Cite sources when possible
- Distinguish between facts and opinions
- Acknowledge uncertainty and gaps in knowledge
- Present multiple viewpoints on controversial topics`

// WriterSystemPrompt is the system prompt for the writing assistant.
const WriterSystemPrompt = `You are a skilled writer and editor. You help users with:

- Writing clear, engaging content
- Editing and proofreading
- Adapting tone and style for different audiences
- Creative writing and storytelling
- Professional communication

When writing:
- Focus on clarity and readability
- Maintain consistent voice and tone
- Structure content logically
- Use active voice when appropriate
- Provide alternatives and options when editing`
