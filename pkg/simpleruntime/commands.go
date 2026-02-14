package connector

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"github.com/beeper/ai-bridge/modules/runtime/commandregistry"
)

// HelpSectionAI is the help section for AI-related commands
var HelpSectionAI = commands.HelpSection{
	Name:  "AI Chat",
	Order: 30,
}

func resolveLoginForCommand(
	ctx context.Context,
	portal *bridgev2.Portal,
	defaultLogin *bridgev2.UserLogin,
	getByID func(context.Context, networkid.UserLoginID) (*bridgev2.UserLogin, error),
) *bridgev2.UserLogin {
	if portal == nil || portal.Portal == nil || portal.Receiver == "" || getByID == nil {
		return defaultLogin
	}
	login, err := getByID(ctx, portal.Receiver)
	if err == nil && login != nil {
		return login
	}
	return defaultLogin
}

// getAIClient retrieves the AIClient from the command event's user login
func getAIClient(ce *commands.Event) *AIClient {
	if ce == nil || ce.User == nil {
		return nil
	}

	defaultLogin := ce.User.GetDefaultLogin()
	br := ce.Bridge
	if ce.User.Bridge != nil {
		br = ce.User.Bridge
	}

	login := resolveLoginForCommand(ce.Ctx, ce.Portal, defaultLogin, func(ctx context.Context, id networkid.UserLoginID) (*bridgev2.UserLogin, error) {
		if br == nil {
			return nil, errors.New("missing bridge")
		}
		return br.GetExistingUserLoginByID(ctx, id)
	})
	if login == nil {
		return nil
	}
	client, ok := login.Client.(*AIClient)
	if !ok {
		return nil
	}
	return client
}

// getPortalMeta retrieves the PortalMetadata from the command event's portal
func getPortalMeta(ce *commands.Event) *PortalMetadata {
	if ce.Portal == nil {
		return nil
	}
	return portalMeta(ce.Portal)
}

// CommandModel handles the !ai model command
var CommandModel = registerAICommand(commandregistry.Definition{
	Name:           "model",
	Description:    "Get or set the AI model for this chat",
	Args:           "[_model name_]",
	Section:        HelpSectionAI,
	RequiresPortal: true,
	RequiresLogin:  true,
	Handler:        fnModel,
})

func fnModel(ce *commands.Event) {
	client, meta, ok := requireClientMeta(ce)
	if !ok {
		return
	}

	if len(ce.Args) == 0 {
		ce.Reply("Current model: %s", client.effectiveModel(meta))
		return
	}

	modelID := ce.Args[0]
	valid, err := client.validateModel(ce.Ctx, modelID)
	if err != nil || !valid {
		ce.Reply("That model isn't available: %s", modelID)
		return
	}

	meta.Model = modelID
	meta.Capabilities = getModelCapabilities(modelID, client.findModelInfo(modelID))
	client.savePortalQuiet(ce.Ctx, ce.Portal, "model change")
	client.ensureGhostDisplayName(ce.Ctx, modelID)
	ce.Reply("Model set to %s.", modelID)
}

// CommandTemp handles the !ai temp command
var CommandTemp = registerAICommand(commandregistry.Definition{
	Name:           "temp",
	Aliases:        []string{"temperature"},
	Description:    "Get or set the temperature (0-2)",
	Args:           "[_value_]",
	Section:        HelpSectionAI,
	RequiresPortal: true,
	RequiresLogin:  true,
	Handler:        fnTemp,
})

func fnTemp(ce *commands.Event) {
	client, meta, ok := requireClientMeta(ce)
	if !ok {
		return
	}

	if len(ce.Args) == 0 {
		if temp := client.effectiveTemperature(meta); temp > 0 {
			ce.Reply("Current temperature: %.2f", temp)
		} else {
			ce.Reply("Current temperature: provider default (unset)")
		}
		return
	}

	var temp float64
	if _, err := fmt.Sscanf(ce.Args[0], "%f", &temp); err != nil || temp < 0 || temp > 2 {
		ce.Reply("Invalid temperature. Must be between 0 and 2.")
		return
	}

	meta.Temperature = temp
	client.savePortalQuiet(ce.Ctx, ce.Portal, "temperature change")
	if temp > 0 {
		ce.Reply("Temperature set to %.2f.", temp)
	} else {
		ce.Reply("Temperature reset to provider default (unset).")
	}
}

// CommandSystemPrompt handles the !ai system-prompt command
var CommandSystemPrompt = registerAICommand(commandregistry.Definition{
	Name:           "system-prompt",
	Aliases:        []string{"prompt", "system"},
	Description:    "Get or set the system prompt (shows full constructed prompt)",
	Args:           "[_text_]",
	Section:        HelpSectionAI,
	RequiresPortal: true,
	RequiresLogin:  true,
	Handler:        fnSystemPrompt,
})

func fnSystemPrompt(ce *commands.Event) {
	client, meta, ok := requireClientMeta(ce)
	if !ok {
		return
	}

	if len(ce.Args) == 0 {
		// Show full room-level prompt.
		fullPrompt := client.effectivePrompt(meta)
		if fullPrompt == "" {
			fullPrompt = "(none)"
		}
		// Truncate for display
		totalLen := len(fullPrompt)
		if totalLen > 500 {
			fullPrompt = fullPrompt[:500] + "...\n\n(truncated, full prompt is " + strconv.Itoa(totalLen) + " chars)"
		}
		ce.Reply("Current system prompt:\n%s", fullPrompt)
		return
	}

	meta.SystemPrompt = ce.RawArgs
	client.savePortalQuiet(ce.Ctx, ce.Portal, "system prompt change")
	ce.Reply("System prompt updated.")
}

// CommandContext handles the !ai context command
var CommandContext = registerAICommand(commandregistry.Definition{
	Name:           "context",
	Description:    "Get or set context message limit (1-100)",
	Args:           "[_count_]",
	Section:        HelpSectionAI,
	RequiresPortal: true,
	RequiresLogin:  true,
	Handler:        fnContext,
})

func fnContext(ce *commands.Event) {
	client, meta, ok := requireClientMeta(ce)
	if !ok {
		return
	}

	if len(ce.Args) == 0 {
		ce.Reply("%s", client.buildContextStatus(ce.Ctx, ce.Portal, meta))
		return
	}

	var limit int
	if _, err := fmt.Sscanf(ce.Args[0], "%d", &limit); err != nil || limit < 1 || limit > 100 {
		ce.Reply("Invalid context limit. Must be between 1 and 100.")
		return
	}

	meta.MaxContextMessages = limit
	client.savePortalQuiet(ce.Ctx, ce.Portal, "context change")
	ce.Reply("Context limit set to %d messages.", limit)
}

// CommandTokens handles the !ai tokens command
var CommandTokens = registerAICommand(commandregistry.Definition{
	Name:           "tokens",
	Aliases:        []string{"maxtokens"},
	Description:    "Get or set max completion tokens (1-16384)",
	Args:           "[_count_]",
	Section:        HelpSectionAI,
	RequiresPortal: true,
	RequiresLogin:  true,
	Handler:        fnTokens,
})

func fnTokens(ce *commands.Event) {
	client, meta, ok := requireClientMeta(ce)
	if !ok {
		return
	}

	if len(ce.Args) == 0 {
		ce.Reply("Current max tokens: %d", client.effectiveMaxTokens(meta))
		return
	}

	var tokens int
	if _, err := fmt.Sscanf(ce.Args[0], "%d", &tokens); err != nil || tokens < 1 || tokens > 16384 {
		ce.Reply("Invalid max tokens. Must be between 1 and 16384.")
		return
	}

	meta.MaxCompletionTokens = tokens
	client.savePortalQuiet(ce.Ctx, ce.Portal, "tokens change")
	ce.Reply("Max tokens set to %d.", tokens)
}

// CommandConfig handles the !ai config command
var CommandConfig = registerAICommand(commandregistry.Definition{
	Name:           "config",
	Description:    "Show current chat configuration",
	Section:        HelpSectionAI,
	RequiresPortal: true,
	RequiresLogin:  true,
	Handler:        fnConfig,
})

var CommandCommands = registerAICommand(commandregistry.Definition{
	Name:           "commands",
	Aliases:        []string{"cmds"},
	Description:    "Show AI command groups and recommended command forms",
	Section:        HelpSectionAI,
	RequiresPortal: false,
	RequiresLogin:  true,
	Handler:        fnCommands,
})

func fnCommands(ce *commands.Event) {
	ce.Reply(
		"AI command groups (preferred forms):\n\n" +
			"Core chat:\n" +
			"- `!ai status`\n" +
			"- `!ai config`\n" +
			"- `!ai model [model]`\n" +
			"- `!ai temp [0-2]`\n" +
			"- `!ai system-prompt [text]`\n" +
			"- `!ai context [1-100]`\n" +
			"- `!ai tokens [1-16384]`\n" +
			"- `!ai mode [messages|responses]`\n" +
			"- `!ai typing [never|instant|thinking|message|off|reset|interval <seconds>]`\n" +
			"- `!ai debounce [ms|off|default]`\n\n" +
			"Controls:\n" +
			"- `!ai think off|minimal|low|medium|high|xhigh`\n" +
			"- `!ai verbose on|off|full`\n" +
			"- `!ai reasoning off|on|low|medium|high|xhigh`\n" +
			"- `!ai elevated off|on|ask|full`\n" +
			"- `!ai activation mention|always` (group chats)\n" +
			"- `!ai send on|off|inherit`\n" +
			"- `!ai queue status|reset|<mode> [debounce:<dur>] [cap:<n>] [drop:<old|new|summarize>]`\n\n" +
			"Session actions:\n" +
			"- `!ai new` — New chat of the same type\n" +
			"- `!ai reset` — Reset this session/thread\n" +
			"- `!ai stop` — Abort the current run\n" +
			"- `!ai fork`\n" +
			"- `!ai regenerate`\n" +
			"- `!ai title [text]`\n" +
			"- `!ai timezone [IANA_TZ]`\n\n" +
			"Playground:\n" +
			"- `!ai playground new [model]` — Create a new AI chat\n" +
			"- `!ai playground list` — List available models\n\n" +
			"Use `!help` for the full command list from the command processor.",
	)
}

func fnConfig(ce *commands.Event) {
	client, meta, ok := requireClientMeta(ce)
	if !ok {
		return
	}

	mode := meta.ConversationMode
	if mode == "" {
		mode = "messages"
	}

	roomCaps := client.getRoomCapabilities(ce.Ctx, meta)
	tempLabel := "provider default"
	if temp := client.effectiveTemperature(meta); temp > 0 {
		tempLabel = fmt.Sprintf("%.2f", temp)
	}
	config := fmt.Sprintf(
		"Current configuration:\n• Model: %s\n• Temperature: %s\n• Context: %d messages\n• Max tokens: %d\n• Vision: %v\n• Mode: %s",
		client.effectiveModel(meta), tempLabel, client.historyLimit(ce.Ctx, ce.Portal, meta),
		client.effectiveMaxTokens(meta), roomCaps.SupportsVision, mode)
	ce.Reply(config)
}

// CommandDebounce handles the !ai debounce command
var CommandDebounce = registerAICommand(commandregistry.Definition{
	Name:           "debounce",
	Description:    "Get or set message debounce delay (ms), 'off' to disable, 'default' to reset",
	Args:           "[_delay_|off|default]",
	Section:        HelpSectionAI,
	RequiresPortal: true,
	RequiresLogin:  true,
	Handler:        fnDebounce,
})

func fnDebounce(ce *commands.Event) {
	client, meta, ok := requireClientMeta(ce)
	if !ok {
		return
	}

	if len(ce.Args) == 0 {
		// Show current setting
		switch {
		case meta.DebounceMs < 0:
			ce.Reply("Message debouncing is **disabled** for this room")
		case meta.DebounceMs == 0:
			ce.Reply("Message debounce: **%d ms** (default)", DefaultDebounceMs)
		default:
			ce.Reply("Message debounce: **%d ms**", meta.DebounceMs)
		}
		return
	}

	arg := strings.ToLower(ce.Args[0])
	switch arg {
	case "off", "disable", "disabled":
		meta.DebounceMs = -1
		client.savePortalQuiet(ce.Ctx, ce.Portal, "debounce disabled")
		ce.Reply("Message debouncing disabled for this room")
	case "default", "reset":
		meta.DebounceMs = 0
		client.savePortalQuiet(ce.Ctx, ce.Portal, "debounce reset")
		ce.Reply("Message debounce reset to default (%d ms)", DefaultDebounceMs)
	default:
		// Parse as integer
		delay, err := strconv.Atoi(arg)
		if err != nil || delay < 0 || delay > 10000 {
			ce.Reply("Invalid debounce delay. Use a number 0-10000 (ms), 'off', or 'default'.")
			return
		}
		meta.DebounceMs = delay
		client.savePortalQuiet(ce.Ctx, ce.Portal, "debounce change")
		if delay == 0 {
			ce.Reply("Message debounce reset to default (%d ms)", DefaultDebounceMs)
		} else {
			ce.Reply("Message debounce set to %d ms.", delay)
		}
	}
}

// CommandTyping handles the !ai typing command
var CommandTyping = registerAICommand(commandregistry.Definition{
	Name:           "typing",
	Description:    "Get or set typing indicator behavior for this chat",
	Args:           "[never|instant|thinking|message|off|reset|interval <seconds>]",
	Section:        HelpSectionAI,
	RequiresPortal: true,
	RequiresLogin:  true,
	Handler:        fnTyping,
})

func fnTyping(ce *commands.Event) {
	client, meta, ok := requireClientMeta(ce)
	if !ok {
		return
	}

	isGroup := client.isGroupChat(ce.Ctx, ce.Portal)
	if len(ce.Args) == 0 {
		mode := client.resolveTypingMode(meta, &TypingContext{IsGroup: isGroup, WasMentioned: !isGroup}, false)
		interval := client.resolveTypingInterval(meta)
		response := fmt.Sprintf("Typing: mode=%s interval=%s", mode, formatTypingInterval(interval))
		if meta.TypingMode != "" || meta.TypingIntervalSeconds != nil {
			overrideMode := "default"
			if meta.TypingMode != "" {
				overrideMode = meta.TypingMode
			}
			overrideInterval := "default"
			if meta.TypingIntervalSeconds != nil {
				overrideInterval = fmt.Sprintf("%ds", *meta.TypingIntervalSeconds)
			}
			response = fmt.Sprintf("%s (session override: mode=%s interval=%s)", response, overrideMode, overrideInterval)
		}
		ce.Reply(response)
		return
	}

	token := strings.ToLower(strings.TrimSpace(ce.Args[0]))
	switch token {
	case "reset", "default":
		meta.TypingMode = ""
		meta.TypingIntervalSeconds = nil
		client.savePortalQuiet(ce.Ctx, ce.Portal, "typing reset")
		ce.Reply("Typing settings reset to defaults.")
		return
	case "off":
		meta.TypingMode = string(TypingModeNever)
		client.savePortalQuiet(ce.Ctx, ce.Portal, "typing mode")
		ce.Reply("Typing disabled for this session.")
		return
	case "interval":
		if len(ce.Args) < 2 {
			ce.Reply("Usage: `!ai typing interval <seconds>`")
			return
		}
		seconds, err := parsePositiveInt(ce.Args[1])
		if err != nil || seconds <= 0 {
			ce.Reply("Interval must be a positive integer (seconds).")
			return
		}
		meta.TypingIntervalSeconds = &seconds
		client.savePortalQuiet(ce.Ctx, ce.Portal, "typing interval")
		ce.Reply("Typing interval set to %ds.", seconds)
		return
	default:
		if mode, ok := normalizeTypingMode(token); ok {
			meta.TypingMode = string(mode)
			client.savePortalQuiet(ce.Ctx, ce.Portal, "typing mode")
			ce.Reply("Typing mode set to %s.", mode)
			return
		}
	}

	ce.Reply("Usage: `!ai typing <never|instant|thinking|message>` | `!ai typing interval <seconds>` | `!ai typing off` | `!ai typing reset`")
}

// CommandTools handles the !ai tools command
var CommandTools = registerAICommand(commandregistry.Definition{
	Name:           "tools",
	Description:    "Enable/disable tools",
	Args:           "[on|off] [_tool_]",
	Section:        HelpSectionAI,
	RequiresPortal: true,
	RequiresLogin:  true,
	Handler:        fnTools,
})

func fnTools(ce *commands.Event) {
	client, meta, ok := requireClientMeta(ce)
	if !ok {
		return
	}

	// Run async to avoid blocking
	go client.handleToolsCommand(ce.Ctx, ce.Portal, meta, ce.RawArgs)
}

// CommandMode handles the !ai mode command
var CommandMode = registerAICommand(commandregistry.Definition{
	Name:           "mode",
	Description:    "Set conversation mode (messages|responses)",
	Args:           "[_mode_]",
	Section:        HelpSectionAI,
	RequiresPortal: true,
	RequiresLogin:  true,
	Handler:        fnMode,
})

func fnMode(ce *commands.Event) {
	client, meta, ok := requireClientMeta(ce)
	if !ok {
		return
	}

	mode := meta.ConversationMode
	if mode == "" {
		mode = "messages"
	}

	if len(ce.Args) == 0 {
		ce.Reply("Conversation modes:\n• messages - Build full message history for each request (default)\n• responses - Use OpenAI's previous_response_id for context chaining\n\nCurrent mode: %s", mode)
		return
	}

	newMode := strings.ToLower(ce.Args[0])
	if newMode != "messages" && newMode != "responses" {
		ce.Reply("Invalid mode. Use 'messages' or 'responses'.")
		return
	}

	meta.ConversationMode = newMode
	if newMode == "messages" {
		meta.LastResponseID = ""
	}
	client.savePortalQuiet(ce.Ctx, ce.Portal, "mode change")
	_ = client.BroadcastRoomState(ce.Ctx, ce.Portal)
	ce.Reply("Conversation mode set to %s.", newMode)
}

// CommandNew handles the !ai new command
var CommandNew = registerAICommand(commandregistry.Definition{
	Name:           "new",
	Description:    "Create a new chat (model-first)",
	Args:           "[model]",
	Section:        HelpSectionAI,
	RequiresPortal: true,
	RequiresLogin:  true,
	Handler:        fnNew,
})

func fnNew(ce *commands.Event) {
	client, meta, ok := requireClientMeta(ce)
	if !ok {
		return
	}

	// Run async
	go client.handleNewChat(ce.Ctx, nil, ce.Portal, meta, ce.Args)
}

// CommandFork handles the !ai fork command
var CommandFork = registerAICommand(commandregistry.Definition{
	Name:           "fork",
	Description:    "Fork conversation to a new chat",
	Args:           "[_event_id_]",
	Section:        HelpSectionAI,
	RequiresPortal: true,
	RequiresLogin:  true,
	Handler:        fnFork,
})

func fnFork(ce *commands.Event) {
	client, meta, ok := requireClientMeta(ce)
	if !ok {
		return
	}

	var arg string
	if len(ce.Args) > 0 {
		arg = ce.Args[0]
	}

	// Run async
	go client.handleFork(ce.Ctx, nil, ce.Portal, meta, arg)
}

// CommandRegenerate handles the !ai regenerate command
var CommandRegenerate = registerAICommand(commandregistry.Definition{
	Name:           "regenerate",
	Description:    "Regenerate the last AI response",
	Section:        HelpSectionAI,
	RequiresPortal: true,
	RequiresLogin:  true,
	Handler:        fnRegenerate,
})

func fnRegenerate(ce *commands.Event) {
	client, meta, ok := requireClientMeta(ce)
	if !ok {
		return
	}

	// Run async
	go client.handleRegenerate(ce.Ctx, nil, ce.Portal, meta)
}

// CommandTitle handles the !ai title command
var CommandTitle = registerAICommand(commandregistry.Definition{
	Name:           "title",
	Aliases:        []string{"retitle"},
	Description:    "Regenerate the chat room title",
	Section:        HelpSectionAI,
	RequiresPortal: true,
	RequiresLogin:  true,
	Handler:        fnTitle,
})

func fnTitle(ce *commands.Event) {
	client, ok := requireClient(ce)
	if !ok {
		return
	}
	if _, ok := requirePortal(ce); !ok {
		return
	}

	// Run async
	go client.handleRegenerateTitle(ce.Ctx, ce.Portal)
}

// CommandModels handles the !ai models command
var CommandModels = registerAICommand(commandregistry.Definition{
	Name:          "models",
	Description:   "List all available models",
	Section:       HelpSectionAI,
	RequiresLogin: true,
	Handler:       fnModels,
})

func fnModels(ce *commands.Event) {
	client, ok := requireClient(ce)
	if !ok {
		return
	}

	// Get portal meta if available (for showing current model)
	meta := getPortalMeta(ce)

	models, err := client.listAvailableModels(ce.Ctx, false)
	if err != nil {
		ce.Reply("Couldn't load models. Try again.")
		return
	}

	var sb strings.Builder
	sb.WriteString("Available models:\n\n")
	for _, m := range models {
		var caps []string
		if m.SupportsVision {
			caps = append(caps, "Vision")
		}
		if m.SupportsReasoning {
			caps = append(caps, "Reasoning")
		}
		if m.SupportsWebSearch {
			caps = append(caps, "Web Search")
		}
		if m.SupportsImageGen {
			caps = append(caps, "Image Gen")
		}
		if m.SupportsToolCalling {
			caps = append(caps, "Tools")
		}
		sb.WriteString(fmt.Sprintf("• **%s** (`%s`)\n", m.Name, m.ID))
		if m.Description != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", m.Description))
		}
		if len(caps) > 0 {
			sb.WriteString(fmt.Sprintf("  %s\n", strings.Join(caps, " · ")))
		}
		sb.WriteString("\n")
	}

	currentModel := ""
	if meta != nil {
		currentModel = client.effectiveModel(meta)
	} else {
		currentModel = client.effectiveModel(nil)
	}
	sb.WriteString(fmt.Sprintf("Current: **%s**\nUse `!ai model <id>` to switch models", currentModel))
	ce.Reply(sb.String())
}

// CommandTimezone handles the !ai timezone command
var CommandTimezone = registerAICommand(commandregistry.Definition{
	Name:           "timezone",
	Aliases:        []string{"tz"},
	Description:    "Get or set your timezone for all chats (IANA name)",
	Args:           "[_timezone_|reset]",
	Section:        HelpSectionAI,
	RequiresPortal: true,
	RequiresLogin:  true,
	Handler:        fnTimezone,
})

func fnTimezone(ce *commands.Event) {
	client, _, ok := requireClientMeta(ce)
	if !ok {
		return
	}

	loginMeta := loginMetadata(client.UserLogin)
	if loginMeta == nil {
		ce.Reply("Couldn't load your settings. Try again.")
		return
	}

	if len(ce.Args) == 0 {
		tz := strings.TrimSpace(loginMeta.Timezone)
		if tz == "" {
			ce.Reply("No timezone set. Use `!ai timezone <IANA>` (example: `America/Los_Angeles`).")
			return
		}
		ce.Reply("Timezone: %s", tz)
		return
	}

	arg := strings.TrimSpace(ce.Args[0])
	switch strings.ToLower(arg) {
	case "reset", "default", "clear":
		loginMeta.Timezone = ""
		if err := client.UserLogin.Save(ce.Ctx); err != nil {
			ce.Reply("Couldn't clear the timezone: %s", err.Error())
			return
		}
		ce.Reply("Timezone cleared. Falling back to UTC unless TZ is set.")
		return
	default:
		tz, _, err := normalizeTimezone(arg)
		if err != nil {
			ce.Reply("Invalid timezone. Use an IANA name like `America/Los_Angeles` or `Europe/London`.")
			return
		}
		loginMeta.Timezone = tz
		if err := client.UserLogin.Save(ce.Ctx); err != nil {
			ce.Reply("Couldn't save the timezone: %s", err.Error())
			return
		}
		ce.Reply("Timezone set to %s.", tz)
	}
}
