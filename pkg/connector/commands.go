package connector

import (
	"fmt"
	"strings"

	"maunium.net/go/mautrix/bridgev2/commands"
)

// HelpSectionAI is the help section for AI-related commands
var HelpSectionAI = commands.HelpSection{
	Name:  "AI Chat",
	Order: 30,
}

// getAIClient retrieves the AIClient from the command event's user login
func getAIClient(ce *commands.Event) *AIClient {
	login := ce.User.GetDefaultLogin()
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
var CommandModel = &commands.FullHandler{
	Func: fnModel,
	Name: "model",
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Get or set the AI model for this chat",
		Args:        "[_model name_]",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnModel(ce *commands.Event) {
	client := getAIClient(ce)
	meta := getPortalMeta(ce)
	if client == nil || meta == nil {
		ce.Reply("Failed to access AI configuration")
		return
	}

	if len(ce.Args) == 0 {
		ce.Reply("Current model: %s", client.effectiveModel(meta))
		return
	}

	modelID := ce.Args[0]
	valid, err := client.validateModel(ce.Ctx, modelID)
	if err != nil || !valid {
		ce.Reply("Invalid model: %s", modelID)
		return
	}

	meta.Model = modelID
	meta.Capabilities = getModelCapabilities(modelID, client.findModelInfo(modelID))
	client.savePortalQuiet(ce.Ctx, ce.Portal, "model change")
	client.ensureGhostDisplayName(ce.Ctx, modelID)
	ce.Reply("Model changed to: %s", modelID)
}

// CommandTemp handles the !ai temp command
var CommandTemp = &commands.FullHandler{
	Func:    fnTemp,
	Name:    "temp",
	Aliases: []string{"temperature"},
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Get or set the temperature (0-2)",
		Args:        "[_value_]",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnTemp(ce *commands.Event) {
	client := getAIClient(ce)
	meta := getPortalMeta(ce)
	if client == nil || meta == nil {
		ce.Reply("Failed to access AI configuration")
		return
	}

	if len(ce.Args) == 0 {
		ce.Reply("Current temperature: %.2f", client.effectiveTemperature(meta))
		return
	}

	var temp float64
	if _, err := fmt.Sscanf(ce.Args[0], "%f", &temp); err != nil || temp < 0 || temp > 2 {
		ce.Reply("Invalid temperature. Must be between 0 and 2.")
		return
	}

	meta.Temperature = temp
	client.savePortalQuiet(ce.Ctx, ce.Portal, "temperature change")
	ce.Reply("Temperature set to: %.2f", temp)
}

// CommandPrompt handles the !ai prompt command
var CommandPrompt = &commands.FullHandler{
	Func:    fnPrompt,
	Name:    "prompt",
	Aliases: []string{"system"},
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Get or set the system prompt",
		Args:        "[_text_]",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnPrompt(ce *commands.Event) {
	client := getAIClient(ce)
	meta := getPortalMeta(ce)
	if client == nil || meta == nil {
		ce.Reply("Failed to access AI configuration")
		return
	}

	if len(ce.Args) == 0 {
		current := client.effectivePrompt(meta)
		if current == "" {
			current = "(none)"
		} else if len(current) > 100 {
			current = current[:100] + "..."
		}
		ce.Reply("Current system prompt: %s", current)
		return
	}

	meta.SystemPrompt = ce.RawArgs
	client.savePortalQuiet(ce.Ctx, ce.Portal, "prompt change")
	ce.Reply("System prompt updated.")
}

// CommandContext handles the !ai context command
var CommandContext = &commands.FullHandler{
	Func: fnContext,
	Name: "context",
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Get or set context message limit (1-100)",
		Args:        "[_count_]",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnContext(ce *commands.Event) {
	client := getAIClient(ce)
	meta := getPortalMeta(ce)
	if client == nil || meta == nil {
		ce.Reply("Failed to access AI configuration")
		return
	}

	if len(ce.Args) == 0 {
		ce.Reply("Current context limit: %d messages", client.historyLimit(meta))
		return
	}

	var limit int
	if _, err := fmt.Sscanf(ce.Args[0], "%d", &limit); err != nil || limit < 1 || limit > 100 {
		ce.Reply("Invalid context limit. Must be between 1 and 100.")
		return
	}

	meta.MaxContextMessages = limit
	client.savePortalQuiet(ce.Ctx, ce.Portal, "context change")
	ce.Reply("Context limit set to: %d messages", limit)
}

// CommandTokens handles the !ai tokens command
var CommandTokens = &commands.FullHandler{
	Func:    fnTokens,
	Name:    "tokens",
	Aliases: []string{"maxtokens"},
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Get or set max completion tokens (1-16384)",
		Args:        "[_count_]",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnTokens(ce *commands.Event) {
	client := getAIClient(ce)
	meta := getPortalMeta(ce)
	if client == nil || meta == nil {
		ce.Reply("Failed to access AI configuration")
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
	ce.Reply("Max tokens set to: %d", tokens)
}

// CommandConfig handles the !ai config command
var CommandConfig = &commands.FullHandler{
	Func: fnConfig,
	Name: "config",
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Show current chat configuration",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnConfig(ce *commands.Event) {
	client := getAIClient(ce)
	meta := getPortalMeta(ce)
	if client == nil || meta == nil {
		ce.Reply("Failed to access AI configuration")
		return
	}

	mode := meta.ConversationMode
	if mode == "" {
		mode = "messages"
	}

	config := fmt.Sprintf(
		"Current configuration:\n• Model: %s\n• Temperature: %.2f\n• Context: %d messages\n• Max tokens: %d\n• Vision: %v\n• Mode: %s",
		client.effectiveModel(meta), client.effectiveTemperature(meta), client.historyLimit(meta),
		client.effectiveMaxTokens(meta), meta.Capabilities.SupportsVision, mode)
	ce.Reply(config)
}

// CommandTools handles the !ai tools command
var CommandTools = &commands.FullHandler{
	Func: fnTools,
	Name: "tools",
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Enable/disable tools",
		Args:        "[on|off] [_tool_]",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnTools(ce *commands.Event) {
	client := getAIClient(ce)
	meta := getPortalMeta(ce)
	if client == nil || meta == nil {
		ce.Reply("Failed to access AI configuration")
		return
	}

	// Run async to avoid blocking
	go client.handleToolsCommand(ce.Ctx, ce.Portal, meta, ce.RawArgs)
}

// CommandMode handles the !ai mode command
var CommandMode = &commands.FullHandler{
	Func: fnMode,
	Name: "mode",
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Set conversation mode (messages|responses)",
		Args:        "[_mode_]",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnMode(ce *commands.Event) {
	client := getAIClient(ce)
	meta := getPortalMeta(ce)
	if client == nil || meta == nil {
		ce.Reply("Failed to access AI configuration")
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
	ce.Reply("Conversation mode set to: %s", newMode)
}

// CommandNew handles the !ai new command
var CommandNew = &commands.FullHandler{
	Func: fnNew,
	Name: "new",
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Create a new chat (uses current model if none specified)",
		Args:        "[_model_]",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnNew(ce *commands.Event) {
	client := getAIClient(ce)
	meta := getPortalMeta(ce)
	if client == nil || meta == nil {
		ce.Reply("Failed to access AI configuration")
		return
	}

	var arg string
	if len(ce.Args) > 0 {
		arg = ce.Args[0]
	}

	// Run async
	go client.handleNewChat(ce.Ctx, nil, ce.Portal, meta, arg)
}

// CommandFork handles the !ai fork command
var CommandFork = &commands.FullHandler{
	Func: fnFork,
	Name: "fork",
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Fork conversation to a new chat",
		Args:        "[_event_id_]",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnFork(ce *commands.Event) {
	client := getAIClient(ce)
	meta := getPortalMeta(ce)
	if client == nil || meta == nil {
		ce.Reply("Failed to access AI configuration")
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
var CommandRegenerate = &commands.FullHandler{
	Func: fnRegenerate,
	Name: "regenerate",
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Regenerate the last AI response",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnRegenerate(ce *commands.Event) {
	client := getAIClient(ce)
	meta := getPortalMeta(ce)
	if client == nil || meta == nil {
		ce.Reply("Failed to access AI configuration")
		return
	}

	// Run async
	go client.handleRegenerate(ce.Ctx, nil, ce.Portal, meta)
}

// CommandModels handles the !ai models command
var CommandModels = &commands.FullHandler{
	Func: fnModels,
	Name: "models",
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "List all available models",
	},
	RequiresLogin: true,
}

func fnModels(ce *commands.Event) {
	client := getAIClient(ce)
	if client == nil {
		ce.Reply("Failed to access AI configuration")
		return
	}

	// Get portal meta if available (for showing current model)
	meta := getPortalMeta(ce)

	models, err := client.listAvailableModels(ce.Ctx, false)
	if err != nil {
		ce.Reply("Failed to fetch models")
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

// registerCommands registers all AI commands with the command processor
func (oc *OpenAIConnector) registerCommands(proc *commands.Processor) {
	proc.AddHandlers(
		CommandModel,
		CommandTemp,
		CommandPrompt,
		CommandContext,
		CommandTokens,
		CommandConfig,
		CommandTools,
		CommandMode,
		CommandNew,
		CommandFork,
		CommandRegenerate,
		CommandModels,
	)
	oc.br.Log.Info().
		Str("section", HelpSectionAI.Name).
		Int("section_order", HelpSectionAI.Order).
		Msg("Registered AI commands: model, temp, prompt, context, tokens, config, tools, mode, new, fork, regenerate, models")
}
