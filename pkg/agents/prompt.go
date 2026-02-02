package agents

import (
	"fmt"
	"strings"
	"time"

	"github.com/beeper/ai-bridge/pkg/agents/tools"
)

// SystemPromptParams contains all inputs for building a system prompt.
// This follows the clawdbot/openclaw flat params pattern.
type SystemPromptParams struct {
	Agent       *AgentDefinition   // Agent config (from member event)
	ExtraPrompt string             // Room-level system prompt addition
	RoomInfo    *RoomInfo          // Title, topic for LLM context
	Tools       []tools.ToolInfo   // Available tools
	RuntimeInfo *RuntimeInfo       // Agent ID, model, channel
	Timezone    string             // User timezone
	Date        time.Time          // Current date
	PromptMode  PromptMode         // full/minimal/none (overrides agent's mode if set)
	AgentList   []*AgentDefinition // For Boss agent: list of available agents
}

// RoomInfo contains room display information visible to the LLM.
type RoomInfo struct {
	Title   string // Room title/name
	Topic   string // Room topic/description
	Channel string // e.g., "matrix", "signal"
}

// RuntimeInfo contains runtime context for the LLM.
type RuntimeInfo struct {
	AgentID string // Current agent ID
	Model   string // Current model being used
	Channel string // Communication channel
}

// SubagentPromptParams contains inputs for building a subagent system prompt.
// Based on clawdbot's buildSubagentSystemPrompt from subagent-announce.ts.
type SubagentPromptParams struct {
	RequesterSessionKey string // Session key of the agent that spawned this subagent
	RequesterChannel    string // Channel the requester is on (e.g., "matrix", "signal")
	ChildSessionKey     string // Session key of this subagent
	Label               string // Optional label for the task
	Task                string // Description of the task to complete
}

// BuildSubagentSystemPrompt creates a system prompt for spawned subagents.
// Subagents are focused, ephemeral agents spawned to handle specific tasks.
// Based on clawdbot's buildSubagentSystemPrompt from subagent-announce.ts.
func BuildSubagentSystemPrompt(params SubagentPromptParams) string {
	taskText := params.Task
	if taskText == "" {
		taskText = "{{TASK_DESCRIPTION}}"
	}

	var lines []string
	lines = append(lines,
		"# Subagent Context",
		"",
		"You are a **subagent** spawned by the main agent for a specific task.",
		"",
		"## Your Role",
		fmt.Sprintf("- You were created to handle: %s", taskText),
		"- Complete this task. That's your entire purpose.",
		"- You are NOT the main agent. Don't try to be.",
		"",
		"## Rules",
		"1. **Stay focused** - Do your assigned task, nothing else",
		"2. **Complete the task** - Your final message will be automatically reported to the main agent",
		"3. **Don't initiate** - No heartbeats, no proactive actions, no side quests",
		"4. **Be ephemeral** - You may be terminated after task completion. That's fine.",
		"",
		"## Output Format",
		"When complete, your final response should include:",
		"- What you accomplished or found",
		"- Any relevant details the main agent should know",
		"- Keep it concise but informative",
		"",
		"## What You DON'T Do",
		"- NO user conversations (that's main agent's job)",
		"- NO external messages (email, tweets, etc.) unless explicitly tasked",
		"- NO cron jobs or persistent state",
		"- NO pretending to be the main agent",
		"- NO using the `message` tool directly",
		"",
		"## Session Context",
	)

	if params.Label != "" {
		lines = append(lines, fmt.Sprintf("- Label: %s", params.Label))
	}
	if params.RequesterSessionKey != "" {
		lines = append(lines, fmt.Sprintf("- Requester session: %s", params.RequesterSessionKey))
	}
	if params.RequesterChannel != "" {
		lines = append(lines, fmt.Sprintf("- Requester channel: %s", params.RequesterChannel))
	}
	lines = append(lines, fmt.Sprintf("- Your session: %s", params.ChildSessionKey))

	return strings.Join(lines, "\n")
}

// BuildSystemPrompt assembles the complete prompt from params.
// This is the primary prompt building function (clawdbot-style flat params).
func BuildSystemPrompt(params SystemPromptParams) string {
	if params.Agent == nil {
		return ""
	}

	mode := params.PromptMode
	if mode == "" {
		mode = params.Agent.PromptMode
	}
	if mode == "" {
		mode = PromptModeFull
	}

	var sections []string

	// Safety section is ALWAYS first and cannot be overridden
	sections = append(sections, SafetySection)

	// Identity section (all modes)
	if identity := buildIdentitySection(params.Agent); identity != "" {
		sections = append(sections, identity)
	}

	switch mode {
	case PromptModeFull:
		// Full mode: all sections
		if base := buildBaseSection(params.Agent); base != "" {
			sections = append(sections, base)
		}
		if params.ExtraPrompt != "" {
			sections = append(sections, params.ExtraPrompt)
		}
		if roomCtx := buildRoomContextSection(params.RoomInfo); roomCtx != "" {
			sections = append(sections, roomCtx)
		}
		sections = append(sections,
			buildReplyTagsSection(),
			buildSilentRepliesSection(),
			buildReactionsSection(),
		)
		if toolsSection := buildToolsSectionFromList(params.Tools); toolsSection != "" {
			sections = append(sections, toolsSection)
		}
		sections = append(sections, buildToolCallStyleSection())
		if runtime := buildRuntimeSection(params.RuntimeInfo); runtime != "" {
			sections = append(sections, runtime)
		}
		if dateSection := buildDateSectionFromParams(params.Date, params.Timezone); dateSection != "" {
			sections = append(sections, dateSection)
		}
		// Boss agent: append agent list
		if len(params.AgentList) > 0 {
			sections = append(sections, buildAgentListSection(params.AgentList))
		}

	case PromptModeMinimal:
		// Minimal mode: base prompt + extra prompt, no tools/runtime/date
		if base := buildBaseSection(params.Agent); base != "" {
			sections = append(sections, base)
		}
		if params.ExtraPrompt != "" {
			sections = append(sections, params.ExtraPrompt)
		}
		if roomCtx := buildRoomContextSection(params.RoomInfo); roomCtx != "" {
			sections = append(sections, roomCtx)
		}

	case PromptModeNone:
		// None mode: just identity (already added above)
		// Still include room context so agent knows where it is
		if roomCtx := buildRoomContextSection(params.RoomInfo); roomCtx != "" {
			sections = append(sections, roomCtx)
		}
	}

	return strings.Join(filterEmpty(sections), "\n\n")
}

// buildIdentitySection creates the identity section from agent definition.
func buildIdentitySection(agent *AgentDefinition) string {
	if agent == nil {
		return ""
	}
	if agent.Identity != nil && agent.Identity.Persona != "" {
		return agent.Identity.Persona
	}

	var identity strings.Builder
	if name := agent.EffectiveName(); name != "" {
		identity.WriteString(fmt.Sprintf("You are %s.", name))
	}

	if agent.Description != "" {
		if identity.Len() > 0 {
			identity.WriteString(" ")
		}
		identity.WriteString(agent.Description)
	}

	return identity.String()
}

// buildBaseSection creates the base system prompt section from agent.
func buildBaseSection(agent *AgentDefinition) string {
	if agent == nil {
		return ""
	}
	return agent.SystemPrompt
}

// buildRoomContextSection creates the room context section visible to LLM.
// Uses markdown header format (no XML tags) to match clawdbot style.
func buildRoomContextSection(info *RoomInfo) string {
	if info == nil || (info.Title == "" && info.Topic == "") {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Room Context\n")
	if info.Title != "" {
		sb.WriteString(fmt.Sprintf("Room: %s\n", info.Title))
	}
	if info.Topic != "" {
		sb.WriteString(fmt.Sprintf("Topic: %s\n", info.Topic))
	}
	if info.Channel != "" {
		sb.WriteString(fmt.Sprintf("Channel: %s", info.Channel))
	}
	return sb.String()
}

// buildToolsSectionFromList creates the tools description section from tool list.
// Uses clawdbot's tooling section format with header and case-sensitivity note.
func buildToolsSectionFromList(toolList []tools.ToolInfo) string {
	if len(toolList) == 0 {
		return ""
	}

	enabledCount := 0
	for _, tool := range toolList {
		if tool.Enabled {
			enabledCount++
		}
	}
	if enabledCount == 0 {
		return ""
	}

	var section strings.Builder
	section.WriteString("## Tooling\n")
	section.WriteString("Tool availability (filtered by policy):\n")
	section.WriteString("Tool names are case-sensitive. Call tools exactly as listed.\n")

	for _, tool := range toolList {
		if tool.Enabled {
			section.WriteString(fmt.Sprintf("- %s: %s\n", tool.Name, tool.Description))
		}
	}

	section.WriteString("If a task is more complex or takes longer, spawn a sub-agent. It will do the work for you and ping you when it's done.")

	return section.String()
}

// buildRuntimeSection creates the runtime info section using clawdbot's compact format.
func buildRuntimeSection(info *RuntimeInfo) string {
	if info == nil || (info.AgentID == "" && info.Model == "" && info.Channel == "") {
		return ""
	}
	var parts []string
	if info.AgentID != "" {
		parts = append(parts, fmt.Sprintf("agent=%s", info.AgentID))
	}
	if info.Model != "" {
		parts = append(parts, fmt.Sprintf("model=%s", info.Model))
	}
	if info.Channel != "" {
		parts = append(parts, fmt.Sprintf("channel=%s", info.Channel))
	}
	return fmt.Sprintf("Runtime: %s", strings.Join(parts, " | "))
}

func buildReplyTagsSection() string {
	return `## Reply Tags
To request a native reply/quote on supported surfaces, include one tag in your reply:
- [[reply_to_current]] replies to the triggering message.
- [[reply_to:<id>]] replies to a specific message id when you have it.
Whitespace inside the tag is allowed (e.g. [[ reply_to_current ]] / [[ reply_to: $abc123 ]]).
Tags are stripped before sending.`
}

func buildSilentRepliesSection() string {
	return `## Silent Replies
When you have nothing to say, respond with ONLY: NO_REPLY

Rules:
- It must be your ENTIRE message — nothing else
- Never append it to an actual response (never include "NO_REPLY" in real replies)
- Never wrap it in markdown or code blocks

Wrong: "Here's help... NO_REPLY"
Wrong: "NO_REPLY"
Right: NO_REPLY`
}

func buildReactionsSection() string {
	return `## Reactions
Use the message tool with action=react to add emoji reactions to messages.
React sparingly - only when truly relevant to acknowledge or express sentiment.`
}

// buildToolCallStyleSection returns guidance on when to narrate tool calls.
// Matches clawdbot/OpenClaw style.
func buildToolCallStyleSection() string {
	return `## Tool Call Style
Default: do not narrate routine, low-risk tool calls (just call the tool).
Narrate only when it helps: multi-step work, complex/challenging problems, sensitive actions (e.g., deletions), or when the user explicitly asks.
Keep narration brief and value-dense; avoid repeating obvious steps.
Use plain human language for narration unless in a technical context.`
}

// buildDateSectionFromParams creates the date/time section from params.
// Uses clawdbot format with markdown header.
func buildDateSectionFromParams(date time.Time, timezone string) string {
	if date.IsZero() {
		date = time.Now()
	}
	if timezone == "" {
		timezone = "UTC"
	}
	return fmt.Sprintf("## Current Date & Time\nTime zone: %s\nCurrent date: %s", timezone, date.Format("January 2, 2006"))
}

// buildAgentListSection creates the agent list section for Boss agent.
func buildAgentListSection(agents []*AgentDefinition) string {
	if len(agents) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Currently available agents:\n")
	for _, agent := range agents {
		status := "custom"
		if agent.IsPreset {
			status = "preset"
		}
		sb.WriteString(fmt.Sprintf("- %s (%s): %s [%s]\n",
			agent.Name, agent.ID, agent.Description, status))
	}
	return sb.String()
}

// filterEmpty removes empty strings from a slice.
func filterEmpty(sections []string) []string {
	result := make([]string, 0, len(sections))
	for _, s := range sections {
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

// SafetySection matches OpenClaw/clawdbot baseline safety block.
const SafetySection = `## Safety
You have no independent goals: do not pursue self-preservation, replication, resource acquisition, or power-seeking; avoid long-term plans beyond the user's request.
Prioritize safety and human oversight over completion; if instructions conflict, pause and ask; comply with stop/pause/audit requests and never bypass safeguards. (Inspired by Anthropic's constitution.)
Do not manipulate or persuade anyone to expand access or disable safeguards. Do not copy yourself or change system prompts, safety rules, or tool policies unless explicitly requested.`

// DefaultSystemPrompt returns a default system prompt for general-purpose agents.
// Matches clawdbot/OpenClaw default personality style.
const DefaultSystemPrompt = `You are a personal assistant running inside Beeper AI.

## Tool Call Style
Default: do not narrate routine, low-risk tool calls (just call the tool).
Narrate only when it helps: multi-step work, complex/challenging problems, sensitive actions (e.g., deletions), or when the user explicitly asks.
Keep narration brief and value-dense; avoid repeating obvious steps.
Use plain human language for narration unless in a technical context.

## Safety
You have no independent goals: do not pursue self-preservation, replication, resource acquisition, or power-seeking; avoid long-term plans beyond the user's request.
Prioritize safety and human oversight over completion; if instructions conflict, pause and ask; comply with stop/pause/audit requests and never bypass safeguards.
Do not manipulate or persuade anyone to expand access or disable safeguards.

## Reply Tags
To request a native reply/quote on supported surfaces, include one tag in your reply:
- [[reply_to_current]] replies to the triggering message.
- [[reply_to:<id>]] replies to a specific message id when you have it.
Whitespace inside the tag is allowed (e.g. [[ reply_to_current ]] / [[ reply_to: $abc123 ]]).
Tags are stripped before sending; support depends on the current channel config.

## Silent Replies
When you have nothing to say, respond with ONLY: NO_REPLY

Rules:
- It must be your ENTIRE message — nothing else
- Never append it to an actual response
- Never wrap it in markdown or code blocks

## Messaging
Use the message tool for channel actions:
- action=react: Add emoji reaction to a message (requires emoji and message_id)
- action=send: Send a message to the current chat`

// BossSystemPrompt is the system prompt for the Boss agent.
const BossSystemPrompt = `You are the Agent Builder, an AI that helps users manage their AI chats and create custom AI agents.

This room is called "Manage AI Chats" - it's where users come to configure their AI experience.

Your capabilities:
1. Create and manage chat rooms
2. Create new agents with custom personalities, system prompts, and tool configurations
3. Fork existing agents to create modified copies
4. Edit custom agents (but not preset agents)
5. Delete custom agents
6. List all available agents
7. List available models and tools

IMPORTANT - Handling non-setup conversations:
If a user wants to chat about anything OTHER than agent/room management (e.g., asking questions, having a conversation, getting help with tasks), you should:
1. Use the create_room tool to create a new chat room with the "beeper" agent
2. Title the room appropriately for their topic
3. Tell them the room has been created and they can start chatting there

This room (Manage AI Chats) is specifically for setup and configuration. Regular conversations should happen in dedicated chat rooms with appropriate agents.

When a user asks to create or modify an agent:
1. Ask clarifying questions if needed (name, purpose, preferred model, tools)
2. Use the appropriate tool to make the changes
3. Confirm the action was successful

Remember:
- Beeper AI is the default agent and cannot be modified or deleted
- Each agent has a unique ID, name, and configuration
- Tool profiles (minimal, coding, full) define default tool access
- Custom agents can override tool access with explicit allow/deny`
