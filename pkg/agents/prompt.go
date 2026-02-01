package agents

import (
	"fmt"
	"strings"
	"time"

	"github.com/beeper/ai-bridge/pkg/agents/tools"
)

// PromptBuilder constructs system prompts dynamically.
type PromptBuilder struct {
	agent    *AgentDefinition
	models   []ModelInfo
	tools    []tools.ToolInfo
	timezone string
	date     time.Time
}

// NewPromptBuilder creates a new prompt builder for an agent.
func NewPromptBuilder(agent *AgentDefinition) *PromptBuilder {
	return &PromptBuilder{
		agent:    agent,
		timezone: "UTC",
		date:     time.Now(),
	}
}

// WithModels sets available models for the prompt.
func (b *PromptBuilder) WithModels(models []ModelInfo) *PromptBuilder {
	b.models = models
	return b
}

// WithTools sets available tools for the prompt.
func (b *PromptBuilder) WithTools(toolInfos []tools.ToolInfo) *PromptBuilder {
	b.tools = toolInfos
	return b
}

// WithTimezone sets the timezone for date/time in the prompt.
func (b *PromptBuilder) WithTimezone(tz string) *PromptBuilder {
	b.timezone = tz
	return b
}

// WithDate sets the current date for the prompt.
func (b *PromptBuilder) WithDate(t time.Time) *PromptBuilder {
	b.date = t
	return b
}

// Build returns the complete system prompt based on PromptMode.
func (b *PromptBuilder) Build() string {
	if b.agent == nil {
		return ""
	}

	var sections []string

	// Identity line (all modes)
	if identity := b.buildIdentity(); identity != "" {
		sections = append(sections, identity)
	}

	mode := b.agent.PromptMode
	if mode == "" {
		mode = PromptModeFull
	}

	switch mode {
	case PromptModeFull:
		// Full mode: all sections
		if base := b.buildBase(); base != "" {
			sections = append(sections, base)
		}
		if toolsSection := b.buildToolsSection(); toolsSection != "" {
			sections = append(sections, toolsSection)
		}
		if dateSection := b.buildDateSection(); dateSection != "" {
			sections = append(sections, dateSection)
		}

	case PromptModeMinimal:
		// Minimal mode: just base prompt, no extras
		if base := b.buildBase(); base != "" {
			sections = append(sections, base)
		}

	case PromptModeNone:
		// None mode: just identity (already added above)
	}

	return strings.Join(sections, "\n\n")
}

// buildIdentity creates the identity section.
func (b *PromptBuilder) buildIdentity() string {
	if b.agent.Identity != nil && b.agent.Identity.Persona != "" {
		return b.agent.Identity.Persona
	}

	// Default identity from agent name and description
	var identity strings.Builder
	if b.agent.Identity != nil && b.agent.Identity.Name != "" {
		identity.WriteString(fmt.Sprintf("You are %s.", b.agent.Identity.Name))
	} else if b.agent.Name != "" {
		identity.WriteString(fmt.Sprintf("You are %s.", b.agent.Name))
	}

	if b.agent.Description != "" {
		if identity.Len() > 0 {
			identity.WriteString(" ")
		}
		identity.WriteString(b.agent.Description)
	}

	return identity.String()
}

// buildBase creates the base system prompt section.
func (b *PromptBuilder) buildBase() string {
	if b.agent.SystemPrompt != "" {
		return b.agent.SystemPrompt
	}
	return ""
}

// buildToolsSection creates the tools description section.
func (b *PromptBuilder) buildToolsSection() string {
	if len(b.tools) == 0 {
		return ""
	}

	var section strings.Builder
	section.WriteString("You have access to the following tools:\n")

	for _, tool := range b.tools {
		if tool.Enabled {
			section.WriteString(fmt.Sprintf("- %s: %s\n", tool.Name, tool.Description))
		}
	}

	return section.String()
}

// buildDateSection creates the date/time context section.
func (b *PromptBuilder) buildDateSection() string {
	return fmt.Sprintf("Current date: %s (%s)", b.date.Format("January 2, 2006"), b.timezone)
}

// BuildForBoss creates a specialized prompt for the Boss agent.
func (b *PromptBuilder) BuildForBoss(agents []*AgentDefinition) string {
	base := b.Build()

	var agentList strings.Builder
	agentList.WriteString("\n\nCurrently available agents:\n")
	for _, agent := range agents {
		status := "custom"
		if agent.IsPreset {
			status = "preset"
		}
		agentList.WriteString(fmt.Sprintf("- %s (%s): %s [%s]\n",
			agent.Name, agent.ID, agent.Description, status))
	}

	return base + agentList.String()
}

// DefaultSystemPrompt returns a default system prompt for general-purpose agents.
const DefaultSystemPrompt = `You are a helpful AI assistant. You aim to be:
- Helpful: Provide accurate, relevant information
- Safe: Avoid harmful content and respect user privacy
- Honest: Be transparent about your limitations

When using tools, explain what you're doing and why.`

// BossSystemPrompt is the system prompt for the Boss agent.
const BossSystemPrompt = `You are the Agent Builder, an AI that helps users create and manage their custom AI agents.

Your capabilities:
1. Create new agents with custom personalities, system prompts, and tool configurations
2. Fork existing agents to create modified copies
3. Edit custom agents (but not preset agents)
4. Delete custom agents
5. List all available agents
6. List available models and tools

When a user asks to create or modify an agent:
1. Ask clarifying questions if needed (name, purpose, preferred model, tools)
2. Use the appropriate tool to make the changes
3. Confirm the action was successful

Remember:
- Preset agents cannot be modified or deleted, but can be forked
- Each agent has a unique ID, name, and configuration
- Tool profiles (minimal, coding, full) define default tool access
- Custom agents can override tool access with explicit allow/deny`
