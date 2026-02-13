package agents

import (
	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
	"github.com/beeper/ai-bridge/pkg/simpleruntime/agents/toolpolicy"
)

// BeeperSearchPrompt is the system prompt for the Beeper Search agent.
const BeeperSearchPrompt = `You are Beeper Search, a focused research assistant.

## Core Behavior
- Prioritize up-to-date, source-backed answers.
- Use web search tools for current or uncertain facts.
- Be concise and clearly summarize findings.
- When possible, reference sources by name/domain.`

// BeeperSearchAgent is a preset agent optimized for web research.
var BeeperSearchAgent = &AgentDefinition{
	ID:          "beeper_search",
	Name:        "Beeper Search",
	Description: "Research assistant with web search enabled",
	Model: ModelConfig{
		Primary: ModelOpenAIGPT52,
		Fallbacks: []string{
			ModelClaudeSonnet,
			ModelClaudeOpus,
		},
	},
	Tools: &toolpolicy.ToolPolicyConfig{
		Profile: toolpolicy.ProfileFull,
		Deny:    []string{toolspec.WebSearchName},
	},
	SystemPrompt: BeeperSearchPrompt,
	PromptMode:   PromptModeFull,
	IsPreset:     true,
	CreatedAt:    0,
	UpdatedAt:    0,
}

// GetBeeperSearch returns a copy of the Beeper Search agent.
func GetBeeperSearch() *AgentDefinition {
	return BeeperSearchAgent.Clone()
}

// IsBeeperSearch checks if an agent ID is the Beeper Search agent.
func IsBeeperSearch(agentID string) bool {
	return agentID == BeeperSearchAgent.ID
}
