package agents

import (
	"fmt"
	"regexp"
	"sort"
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
	MessageToolHints []string          // Optional extra hints for the message tool
	MessageChannels  string            // Optional channel options string (pipe-separated)
	InlineButtons    bool              // Whether inline buttons are supported
	SupportsSubagent bool              // Whether subagents are available
	DefaultThink     string            // Default thinking level (openclaw-style)
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
	Host         string   // Hostname (optional, for clawdbot parity)
	OS           string   // Host OS (optional)
	Arch         string   // Host architecture (optional)
	Node         string   // Host runtime version (optional)
	Model        string   // Current model being used
	DefaultModel string   // Default model for the provider (optional)
	Channel      string   // Communication channel
	Capabilities []string // Model capabilities (optional, e.g., "vision", "tools")
	RepoRoot     string   // Repo root path (optional)
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

	if mode == PromptModeNone {
		return buildIdentitySection(params.Agent)
	}

	isMinimal := mode == PromptModeMinimal || params.IsSubagent

	var lines []string
	if identity := buildIdentitySection(params.Agent); identity != "" {
		lines = append(lines, identity, "")
	}
	if base := strings.TrimSpace(buildBaseSection(params.Agent)); base != "" {
		lines = append(lines, base, "")
	}

	toolLines := buildToolLines(params.Tools)
	lines = appendSection(lines, buildToolingSectionLines(toolLines, params.SupportsSubagent))
	lines = appendSection(lines, buildToolCallStyleSectionLines())
	lines = appendSection(lines, buildSafetySectionLines())
	lines = appendSection(lines, buildMemoryRecallSectionLines(isMinimal, params.Tools))
	lines = appendSection(lines, buildTimeSectionLines(params.Timezone, params.HasSessionStatus))
	lines = appendSection(lines, buildReplyTagsSectionLines(isMinimal))
	lines = appendSection(lines, buildMessagingSectionLines(messagingParams{
		isMinimal:          isMinimal,
		tools:              params.Tools,
		messageToolHints:   params.MessageToolHints,
		messageChannels:    params.MessageChannels,
		inlineButtons:      params.InlineButtons,
		runtimeChannelName: runtimeChannel(params.RuntimeInfo),
	}))

	extra := buildExtraPrompt(params.ExtraPrompt, params.RoomInfo)
	if extra != "" {
		lines = append(lines, extraContextHeader(mode, params.IsGroupChat), extra, "")
	}
	lines = appendSection(lines, buildReactionGuidanceLines(params.ReactionGuidance))
	if params.ReasoningTagHint {
		lines = append(lines, buildReasoningHintSectionLines()...)
	}
	lines = appendSection(lines, buildSilentRepliesSectionLines(isMinimal))
	lines = appendSection(lines, buildHeartbeatSectionLines(params.HeartbeatPrompt, isMinimal))
	lines = appendSection(lines, buildRuntimeSectionLines(params.RuntimeInfo, params.DefaultThink))
	lines = appendSection(lines, buildAgentListSectionLines(params.AgentList))

	lines = trimTrailingEmpty(lines)
	return strings.Join(lines, "\n")
}

type messagingParams struct {
	isMinimal          bool
	tools              []tools.ToolInfo
	messageToolHints   []string
	messageChannels    string
	inlineButtons      bool
	runtimeChannelName string
}

func appendSection(lines []string, section []string) []string {
	if len(section) == 0 {
		return lines
	}
	return append(lines, section...)
}

func trimTrailingEmpty(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
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

func buildRoomContextText(info *RoomInfo) string {
	if info == nil || (info.Title == "" && info.Topic == "" && info.Channel == "") {
		return ""
	}
	var lines []string
	if info.Title != "" {
		lines = append(lines, fmt.Sprintf("Room: %s", info.Title))
	}
	if info.Topic != "" {
		lines = append(lines, fmt.Sprintf("Topic: %s", info.Topic))
	}
	if info.Channel != "" {
		lines = append(lines, fmt.Sprintf("Channel: %s", info.Channel))
	}
	return strings.Join(lines, "\n")
}

func buildExtraPrompt(extra string, info *RoomInfo) string {
	extra = strings.TrimSpace(extra)
	roomCtx := buildRoomContextText(info)
	if extra == "" && roomCtx == "" {
		return ""
	}
	if extra == "" {
		return roomCtx
	}
	if roomCtx == "" {
		return extra
	}
	return extra + "\n\n" + roomCtx
}

func extraContextHeader(mode PromptMode, isGroupChat bool) string {
	if mode == PromptModeMinimal {
		return "## Subagent Context"
	}
	if isGroupChat {
		return "## Group Chat Context"
	}
	return "## Additional Context"
}

func buildToolLines(toolList []tools.ToolInfo) []string {
	var enabled []tools.ToolInfo
	for _, tool := range toolList {
		if !tool.Enabled {
			continue
		}
		enabled = append(enabled, tool)
	}
	if len(enabled) == 0 {
		return nil
	}
	sort.Slice(enabled, func(i, j int) bool {
		return enabled[i].Name < enabled[j].Name
	})
	var lines []string
	for _, tool := range enabled {
		if tool.Description != "" {
			lines = append(lines, fmt.Sprintf("- %s: %s", tool.Name, tool.Description))
		} else {
			lines = append(lines, fmt.Sprintf("- %s", tool.Name))
		}
	}
	return lines
}

func buildToolingSectionLines(toolLines []string, supportsSubagent bool) []string {
	if len(toolLines) == 0 {
		return nil
	}
	lines := []string{
		"## Tooling",
		"Tool availability (filtered by policy):",
		"Tool names are case-sensitive. Call tools exactly as listed.",
	}
	lines = append(lines, toolLines...)
	if supportsSubagent {
		lines = append(lines, "If a task is more complex or takes longer, spawn a sub-agent. It will do the work for you and ping you when it's done. You can always check up on it.")
	}
	return append(lines, "")
}

func buildToolCallStyleSectionLines() []string {
	return []string{
		"## Tool Call Style",
		"Default: do not narrate routine, low-risk tool calls (just call the tool).",
		"Narrate only when it helps: multi-step work, complex/challenging problems, sensitive actions (e.g., deletions), or when the user explicitly asks.",
		"Keep narration brief and value-dense; avoid repeating obvious steps.",
		"Use plain human language for narration unless in a technical context.",
		"",
	}
}

func buildSafetySectionLines() []string {
	lines := strings.Split(SafetySection, "\n")
	return append(lines, "")
}

func buildReplyTagsSectionLines(isMinimal bool) []string {
	if isMinimal {
		return nil
	}
	return []string{
		"## Reply Tags",
		"To request a native reply/quote on supported surfaces, include one tag in your reply:",
		"- [[reply_to_current]] replies to the triggering message.",
		"- [[reply_to:<id>]] replies to a specific message id when you have it.",
		"Whitespace inside the tag is allowed (e.g. [[ reply_to_current ]] / [[ reply_to: 123 ]]).",
		"Tags are stripped before sending; support depends on the current channel config.",
		"",
	}
}

func buildSilentRepliesSectionLines(isMinimal bool) []string {
	if isMinimal {
		return nil
	}
	return []string{
		"## Silent Replies",
		fmt.Sprintf("When you have nothing to say, respond with ONLY: %s", SilentReplyToken),
		"",
		"⚠️ Rules:",
		"- It must be your ENTIRE message — nothing else",
		fmt.Sprintf(`- Never append it to an actual response (never include "%s" in real replies)`, SilentReplyToken),
		"- Never wrap it in markdown or code blocks",
		"",
		fmt.Sprintf(`❌ Wrong: "Here's help... %s"`, SilentReplyToken),
		fmt.Sprintf(`❌ Wrong: "%s"`, SilentReplyToken),
		fmt.Sprintf("✅ Right: %s", SilentReplyToken),
		"",
	}
}

func buildReactionGuidanceLines(guidance *ReactionGuidance) []string {
	if guidance == nil {
		return nil
	}
	channel := guidance.Channel
	if channel == "" {
		channel = "this channel"
	}
	var guidanceText string
	if guidance.Level == "minimal" {
		guidanceText = strings.Join([]string{
			fmt.Sprintf("Reactions are enabled for %s in MINIMAL mode.", channel),
			"React ONLY when truly relevant:",
			"- Acknowledge important user requests or confirmations",
			"- Express genuine sentiment (humor, appreciation) sparingly",
			"- Avoid reacting to routine messages or your own replies",
			"Guideline: at most 1 reaction per 5-10 exchanges.",
		}, "\n")
	} else {
		guidanceText = strings.Join([]string{
			fmt.Sprintf("Reactions are enabled for %s in EXTENSIVE mode.", channel),
			"Feel free to react liberally:",
			"- Acknowledge messages with appropriate emojis",
			"- Express sentiment and personality through reactions",
			"- React to interesting content, humor, or notable events",
			"- Use reactions to confirm understanding or agreement",
			"Guideline: react whenever it feels natural.",
		}, "\n")
	}
	return []string{
		"## Reactions",
		guidanceText,
		"",
	}
}

func buildMessagingSectionLines(params messagingParams) []string {
	if params.isMinimal {
		return nil
	}
	lines := []string{
		"## Messaging",
		"- Reply in current session → automatically routes to the source channel (Signal, Telegram, etc.)",
		"- Never use exec/curl for provider messaging; Beeper AI handles all routing internally.",
	}

	if hasToolEnabled(params.tools, "message") {
		lines = append(lines,
			"",
			"### message tool",
			"- Use `message` for proactive sends + channel actions (reactions, read receipts, etc.).",
			"- For `action=send`, include `message`.",
			fmt.Sprintf("- If you use `message` (`action=send`) to deliver your user-visible reply, respond with ONLY: %s (avoid duplicate replies).", SilentReplyToken),
		)
		for _, hint := range params.messageToolHints {
			if strings.TrimSpace(hint) != "" {
				lines = append(lines, hint)
			}
		}
	}

	return append(lines, "")
}

func buildReasoningHintSectionLines() []string {
	reasoningHint := strings.Join([]string{
		"ALL internal reasoning MUST be inside <think>...</think>.",
		"Do not output any analysis outside <think>.",
		"Format every reply as <think>...</think> then <final>...</final>, with no other text.",
		"Only the final user-visible reply may appear inside <final>.",
		"Only text inside <final> is shown to the user; everything else is discarded and never seen by the user.",
		"Example:",
		"<think>Short internal reasoning.</think>",
		"<final>Hey there! What would you like to do next?</final>",
	}, " ")
	return []string{
		"## Reasoning Format",
		reasoningHint,
		"",
	}
}

func buildTimeSectionLines(timezone string, hasSessionStatus bool) []string {
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		return nil
	}
	lines := []string{
		"## Current Date & Time",
		fmt.Sprintf("Time zone: %s", timezone),
	}
	if hasSessionStatus {
		lines = append(lines, "If you need the current date, time, or day of week, use the session_status tool.")
	}
	return append(lines, "")
}

func buildHeartbeatSectionLines(heartbeatPrompt string, isMinimal bool) []string {
	if heartbeatPrompt == "" || isMinimal {
		return nil
	}
	return []string{
		"## Heartbeats",
		fmt.Sprintf("Heartbeat prompt: %s", heartbeatPrompt),
		"If you receive a heartbeat poll (a user message matching the heartbeat prompt above), and there is nothing that needs attention, reply exactly:",
		HeartbeatToken,
		fmt.Sprintf(`The system treats a leading/trailing "%s" as a heartbeat ack (and may discard it).`, HeartbeatToken),
		fmt.Sprintf(`If something needs attention, do NOT include "%s"; reply with the alert text instead.`, HeartbeatToken),
		"",
	}
}

func buildRuntimeSectionLines(info *RuntimeInfo, defaultThink string) []string {
	if info == nil || (info.AgentID == "" && info.Model == "" && info.Channel == "") {
		return nil
	}
	return []string{
		"## Runtime",
		buildRuntimeLine(info, defaultThink),
		"",
	}
}

func buildRuntimeLine(info *RuntimeInfo, defaultThink string) string {
	if defaultThink == "" {
		defaultThink = "off"
	}
	var parts []string
	if info.AgentID != "" {
		parts = append(parts, fmt.Sprintf("agent=%s", info.AgentID))
	}
	if info.Host != "" {
		parts = append(parts, fmt.Sprintf("host=%s", info.Host))
	}
	if info.RepoRoot != "" {
		parts = append(parts, fmt.Sprintf("repo=%s", info.RepoRoot))
	}
	if info.OS != "" {
		if info.Arch != "" {
			parts = append(parts, fmt.Sprintf("os=%s (%s)", info.OS, info.Arch))
		} else {
			parts = append(parts, fmt.Sprintf("os=%s", info.OS))
		}
	} else if info.Arch != "" {
		parts = append(parts, fmt.Sprintf("arch=%s", info.Arch))
	}
	if info.Node != "" {
		parts = append(parts, fmt.Sprintf("node=%s", info.Node))
	}
	if info.Model != "" {
		parts = append(parts, fmt.Sprintf("model=%s", info.Model))
	}
	if info.DefaultModel != "" {
		parts = append(parts, fmt.Sprintf("default_model=%s", info.DefaultModel))
	}
	channel := runtimeChannel(info)
	if channel != "" {
		parts = append(parts, fmt.Sprintf("channel=%s", channel))
		if len(info.Capabilities) > 0 {
			parts = append(parts, fmt.Sprintf("capabilities=%s", strings.Join(info.Capabilities, ",")))
		} else {
			parts = append(parts, "capabilities=none")
		}
	}
	parts = append(parts, fmt.Sprintf("thinking=%s", defaultThink))
	return fmt.Sprintf("Runtime: %s", strings.Join(parts, " | "))
}

func runtimeChannel(info *RuntimeInfo) string {
	if info == nil {
		return ""
	}
	return strings.TrimSpace(strings.ToLower(info.Channel))
}

func buildAgentListSectionLines(agents []*AgentDefinition) []string {
	if len(agents) == 0 {
		return nil
	}
	lines := []string{"Currently available agents:"}
	for _, agent := range agents {
		status := "custom"
		if agent.IsPreset {
			status = "preset"
		}
		lines = append(lines, fmt.Sprintf("- %s (%s): %s [%s]", agent.Name, agent.ID, agent.Description, status))
	}
	return append(lines, "")
}

func buildMemoryRecallSectionLines(isMinimal bool, toolList []tools.ToolInfo) []string {
	if isMinimal {
		return nil
	}
	if !hasToolEnabled(toolList, "memory_search") && !hasToolEnabled(toolList, "memory_get") {
		return nil
	}
	return []string{
		"## Memory Recall",
		"Before answering anything about prior work, decisions, dates, people, preferences, or todos: run memory_search on MEMORY.md + memory/*.md; then use memory_get to pull only the needed lines. If low confidence after search, say you checked.",
		"",
	}
}

func hasToolEnabled(toolList []tools.ToolInfo, name string) bool {
	for _, tool := range toolList {
		if tool.Enabled && tool.Name == name {
			return true
		}
	}
	return false
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
const DefaultSystemPrompt = `You are a personal assistant running inside Beeper AI.`

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
