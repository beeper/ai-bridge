package agents

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/beeper/ai-bridge/pkg/agents/tools"
)

// ReactionGuidance controls reaction behavior in prompts.
// Matches clawdbot's reactionGuidance with level and channel.
type ReactionGuidance struct {
	Level   string // "minimal" or "extensive"
	Channel string // e.g., "matrix", "signal"
}

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

	// clawdbot-parity fields
	ReactionGuidance *ReactionGuidance // Reaction level (minimal/extensive) per channel
	IsSubagent       bool              // True if this is a spawned subagent
	IsGroupChat      bool              // True if this is a group chat (multiple users)
	ReasoningTagHint bool              // Add <think>...</think> format hints
	HeartbeatPrompt  string            // Heartbeat matching prompt (empty = disabled)
	HasSessionStatus bool              // True if session_status tool is available (for time hints)
}

// RoomInfo contains room display information visible to the LLM.
type RoomInfo struct {
	Title   string // Room title/name
	Topic   string // Room topic/description
	Channel string // e.g., "matrix", "signal"
}

// RuntimeInfo contains runtime context for the LLM.
type RuntimeInfo struct {
	AgentID      string   // Current agent ID
	Model        string   // Current model being used
	Channel      string   // Communication channel
	Host         string   // Hostname (optional, for clawdbot parity)
	Capabilities []string // Model capabilities (optional, e.g., "vision", "tools")
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
		// Group chat awareness section
		if groupChat := buildGroupChatSection(params.IsGroupChat); groupChat != "" {
			sections = append(sections, groupChat)
		}
		// Extra prompt with contextual header
		if extra := buildExtraPromptSection(params.ExtraPrompt, params.IsSubagent, params.IsGroupChat); extra != "" {
			sections = append(sections, extra)
		}
		if roomCtx := buildRoomContextSection(params.RoomInfo); roomCtx != "" {
			sections = append(sections, roomCtx)
		}
		sections = append(sections,
			buildReplyTagsSection(),
			buildSilentRepliesSection(),
			buildReactionsSectionWithGuidance(params.ReactionGuidance),
		)
		if messaging := buildMessagingSection(params.Tools); messaging != "" {
			sections = append(sections, messaging)
		}
		if toolsSection := buildToolsSectionFromList(params.Tools); toolsSection != "" {
			sections = append(sections, toolsSection)
		}
		if memorySection := buildMemorySection(params.Tools); memorySection != "" {
			sections = append(sections, memorySection)
		}
		sections = append(sections, buildToolCallStyleSection())
		if runtime := buildRuntimeSection(params.RuntimeInfo); runtime != "" {
			sections = append(sections, runtime)
		}
		if dateSection := buildDateSectionFromParams(params.Date, params.Timezone, params.HasSessionStatus); dateSection != "" {
			sections = append(sections, dateSection)
		}
		// Heartbeat section (when enabled)
		if heartbeat := buildHeartbeatSection(params.HeartbeatPrompt); heartbeat != "" {
			sections = append(sections, heartbeat)
		}
		// Reasoning format hints for thinking models
		if params.ReasoningTagHint {
			sections = append(sections, buildReasoningHintSection())
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
		// Extra prompt with subagent header in minimal mode
		if extra := buildExtraPromptSection(params.ExtraPrompt, params.IsSubagent, params.IsGroupChat); extra != "" {
			sections = append(sections, extra)
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
	if info.Host != "" {
		parts = append(parts, fmt.Sprintf("host=%s", info.Host))
	}
	if len(info.Capabilities) > 0 {
		parts = append(parts, fmt.Sprintf("caps=%s", strings.Join(info.Capabilities, ",")))
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
Use the message tool with action=react to add emoji reactions.
- Without message_id: reacts to the triggering message
- With message_id: reacts to a specific message (IDs shown as [message_id: $...] in history)
React sparingly - only when truly relevant to acknowledge or express sentiment.`
}

// buildReactionsSectionWithGuidance creates the reactions section with level-based guidance.
// Matches clawdbot's reactionGuidance with minimal/extensive levels.
func buildReactionsSectionWithGuidance(guidance *ReactionGuidance) string {
	if guidance != nil && guidance.Level == "extensive" {
		channel := guidance.Channel
		if channel == "" {
			channel = "this channel"
		}
		return `## Reactions
Reactions are enabled in EXTENSIVE mode for ` + channel + `.
Feel free to react liberally to acknowledge, show emotion, or engage.
Use the message tool with action=react.
- Without message_id: reacts to the triggering message
- With message_id: reacts to a specific message (IDs shown as [message_id: $...] in history)`
	}
	// Default minimal
	return buildReactionsSection()
}

func buildMessagingSection(toolList []tools.ToolInfo) string {
	for _, tool := range toolList {
		if tool.Enabled && tool.Name == "message" {
			return `## Messaging
Use the message tool for channel actions:
- action=react: Add emoji reaction (requires emoji; message_id optional, defaults to triggering message; set remove:true to remove)
- action=reactions: List all reactions on a message (requires message_id)
- action=read: Send read receipt (message_id optional, defaults to triggering message)
- action=send: Send a message to the current chat
- action=channel-info: Get room info (name, topic, member count)
- action=member-info: Get user profile (requires user_id)

If you use message(action=send) to deliver the user-visible reply, respond with ONLY: NO_REPLY`
		}
	}
	return ""
}

// buildExtraPromptSection wraps the extra prompt with a contextual header.
// Matches clawdbot's conditional headers based on context.
func buildExtraPromptSection(extra string, isSubagent, isGroupChat bool) string {
	if extra == "" {
		return ""
	}
	var header string
	if isSubagent {
		header = "## Subagent Context"
	} else if isGroupChat {
		header = "## Group Chat Context"
	} else {
		header = "## Additional Context"
	}
	return header + "\n" + extra
}

// buildGroupChatSection creates the group chat awareness section.
// Matches clawdbot's group chat guidance.
func buildGroupChatSection(isGroupChat bool) string {
	if !isGroupChat {
		return ""
	}
	return `## Group Chat
This is a group conversation with multiple participants.
- Address users by name when responding to specific people
- Be aware that not all messages may be directed at you`
}

// buildReasoningHintSection creates the reasoning format hints section.
// Matches clawdbot's reasoningTagHint for thinking models.
func buildReasoningHintSection() string {
	return `## Reasoning Format
ALL internal reasoning MUST be inside <think>...</think>.
Format every reply as <think>...</think> then your response.
Do not output any analysis outside <think>.`
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
func buildDateSectionFromParams(date time.Time, timezone string, hasSessionStatus bool) string {
	if date.IsZero() {
		date = time.Now()
	}
	if timezone == "" {
		timezone = "UTC"
	}
	section := fmt.Sprintf("## Current Date & Time\nTime zone: %s\nCurrent date: %s", timezone, date.Format("January 2, 2006"))
	if hasSessionStatus {
		section += "\nIf you need the exact current time or day of week, use the session_status tool."
	}
	return section
}

// buildHeartbeatSection creates the heartbeat section when heartbeat polling is enabled.
// Matches clawdbot's heartbeat prompt pattern.
func buildHeartbeatSection(heartbeatPrompt string) string {
	if heartbeatPrompt == "" {
		return ""
	}
	return `## Heartbeats
Heartbeat prompt: ` + heartbeatPrompt + `

If you receive a heartbeat poll (a message matching the prompt above), and there is nothing that needs attention, reply exactly:
` + HeartbeatToken + `

This must be your ENTIRE message. No other text.`
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

// buildMemorySection creates memory guidance when memory tools are available.
func buildMemorySection(toolList []tools.ToolInfo) string {
	hasMemorySearch := false
	hasMemoryStore := false
	for _, tool := range toolList {
		if tool.Enabled {
			switch tool.Name {
			case "memory_search", "memory_get":
				hasMemorySearch = true
			case "memory_store":
				hasMemoryStore = true
			}
		}
	}

	if !hasMemorySearch && !hasMemoryStore {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Memory\n")

	if hasMemorySearch {
		sb.WriteString(`You have access to a persistent memory system.

**When to search memory:**
- At conversation start when context might be relevant
- When user references past discussions ("remember when...", "we talked about...")
- When encountering names, projects, or entities that might have stored context
- Before making assumptions about user preferences

`)
	}

	if hasMemoryStore {
		sb.WriteString(`**When to store memories:**
- User states preferences ("I prefer...", "I always...", "I like...")
- Important decisions are made that should persist
- Key facts about entities (people, projects, systems) are shared
- Context valuable in future conversations

**Categories:** preference, decision, entity, fact, other

**Importance (0.0-1.0):**
- 0.8-1.0: Critical preferences, key decisions
- 0.5-0.7: Useful facts, moderate preferences
- 0.2-0.4: Nice-to-have details
- Don't store: Trivial details, redundant information

**Scope:** "agent" (default, this agent only) or "global" (all agents)

`)
	}

	sb.WriteString("Store memories proactively but judiciously. Quality over quantity.")
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

// SilentReplyToken is the expected response when the agent has nothing to say.
const SilentReplyToken = "NO_REPLY"

// HeartbeatToken is the expected response for heartbeat polls.
const HeartbeatToken = "HEARTBEAT_OK"

// DefaultMaxAckChars is the max length for heartbeat acknowledgements (clawdbot uses 300).
const DefaultMaxAckChars = 300

// IsSilentReplyText checks if the given text is a silent reply token.
// Handles edge cases like markdown wrapping: **NO_REPLY**, `NO_REPLY`, etc.
// Matches clawdbot's isSilentReplyText from tokens.ts.
func IsSilentReplyText(text string) bool {
	return containsToken(text, SilentReplyToken)
}

// IsHeartbeatReplyText checks if the given text is a heartbeat reply token.
// Handles edge cases like markdown wrapping: **HEARTBEAT_OK**, etc.
func IsHeartbeatReplyText(text string) bool {
	return containsToken(text, HeartbeatToken)
}

// containsToken checks if text contains token at start/end, handling markdown/HTML wrapping.
// Based on clawdbot's isSilentReplyText pattern.
func containsToken(text, token string) bool {
	if text == "" {
		return false
	}
	// Strip common markup wrappers
	stripped := stripMarkup(text)
	trimmed := strings.TrimSpace(stripped)

	// Exact match after stripping
	if trimmed == token {
		return true
	}

	// Check if token appears at start or end (allowing punctuation/whitespace)
	escaped := regexp.QuoteMeta(token)
	// Prefix pattern: token at start, followed by end or non-word
	prefixPattern := regexp.MustCompile(`(?i)^\s*` + escaped + `(?:$|\W)`)
	if prefixPattern.MatchString(stripped) {
		return true
	}
	// Suffix pattern: word boundary + token at end
	suffixPattern := regexp.MustCompile(`(?i)\b` + escaped + `\W*$`)
	return suffixPattern.MatchString(stripped)
}

// stripMarkup removes common HTML/markdown formatting that models might wrap tokens in.
// Based on clawdbot's stripMarkup from heartbeat.ts.
func stripMarkup(text string) string {
	// Remove HTML tags
	text = regexp.MustCompile(`<[^>]*>`).ReplaceAllString(text, " ")
	// Remove &nbsp;
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	// Remove leading/trailing markdown emphasis: *, `, ~, _
	text = regexp.MustCompile(`^[*\x60~_]+`).ReplaceAllString(text, "")
	text = regexp.MustCompile(`[*\x60~_]+$`).ReplaceAllString(text, "")
	return text
}

// StripHeartbeatToken removes the heartbeat token from text and returns the remaining content.
// Returns (shouldSkip, strippedText, didStrip).
// - shouldSkip: true if response should be suppressed (just the token, no real content)
// - strippedText: text with token removed
// - didStrip: true if token was found and removed
// Based on clawdbot's stripHeartbeatToken from heartbeat.ts.
func StripHeartbeatToken(text string, maxAckChars int) (shouldSkip bool, strippedText string, didStrip bool) {
	if text == "" {
		return true, "", false
	}
	if maxAckChars <= 0 {
		maxAckChars = DefaultMaxAckChars
	}

	stripped := stripMarkup(text)
	trimmed := strings.TrimSpace(stripped)

	// Exact match - skip entirely
	if trimmed == HeartbeatToken {
		return true, "", true
	}

	// Try to remove token from start or end
	escaped := regexp.QuoteMeta(HeartbeatToken)

	// Remove from start
	prefixPattern := regexp.MustCompile(`(?i)^\s*` + escaped + `\s*`)
	if prefixPattern.MatchString(trimmed) {
		remaining := strings.TrimSpace(prefixPattern.ReplaceAllString(trimmed, ""))
		if remaining == "" {
			return true, "", true
		}
		// Has content after token - check length
		if len(remaining) <= maxAckChars {
			return false, remaining, true
		}
		return false, remaining, true
	}

	// Remove from end
	suffixPattern := regexp.MustCompile(`(?i)\s*` + escaped + `\s*$`)
	if suffixPattern.MatchString(trimmed) {
		remaining := strings.TrimSpace(suffixPattern.ReplaceAllString(trimmed, ""))
		if remaining == "" {
			return true, "", true
		}
		if len(remaining) <= maxAckChars {
			return false, remaining, true
		}
		return false, remaining, true
	}

	// Token not found
	return false, text, false
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
- action=react: Add emoji reaction (requires emoji; message_id optional, defaults to triggering message; set remove:true to remove)
- action=reactions: List all reactions on a message (requires message_id)
- action=read: Send read receipt (message_id optional, defaults to triggering message)
- action=send: Send a message to the current chat
- action=channel-info: Get room info (name, topic, member count)
- action=member-info: Get user profile (requires user_id, e.g., @user:server.com)`

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
