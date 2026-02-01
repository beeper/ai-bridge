package connector

import (
	"fmt"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2/commands"

	"github.com/beeper/ai-bridge/pkg/agents"
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

	// Protect rooms with Boss agent from overrides
	if agents.IsBossAgent(meta.AgentID) || agents.IsBossAgent(meta.DefaultAgentID) {
		ce.Reply("Cannot change model in a room managed by the Boss agent")
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

	// Protect rooms with Boss agent from overrides
	if agents.IsBossAgent(meta.AgentID) || agents.IsBossAgent(meta.DefaultAgentID) {
		ce.Reply("Cannot change temperature in a room managed by the Boss agent")
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

	// Protect rooms with Boss agent from overrides
	if agents.IsBossAgent(meta.AgentID) || agents.IsBossAgent(meta.DefaultAgentID) {
		ce.Reply("Cannot change system prompt in a room managed by the Boss agent")
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

// CommandTitle handles the !ai title command
var CommandTitle = &commands.FullHandler{
	Func:    fnTitle,
	Name:    "title",
	Aliases: []string{"retitle"},
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Regenerate the chat room title",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnTitle(ce *commands.Event) {
	client := getAIClient(ce)
	if client == nil || ce.Portal == nil {
		ce.Reply("Failed to access AI configuration")
		return
	}

	// Run async
	go client.handleRegenerateTitle(ce.Ctx, ce.Portal)
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
		CommandTitle,
		CommandModels,
		CommandAgent,
		CommandAgents,
		CommandCreateAgent,
		CommandDeleteAgent,
		CommandManage,
		CommandModelRoom,
	)
	oc.br.Log.Info().
		Str("section", HelpSectionAI.Name).
		Int("section_order", HelpSectionAI.Order).
		Msg("Registered AI commands: model, temp, prompt, context, tokens, config, tools, mode, new, fork, regenerate, title, models, agent, agents, create-agent, delete-agent, manage, model-room")
}

// CommandAgent handles the !ai agent command
var CommandAgent = &commands.FullHandler{
	Func: fnAgent,
	Name: "agent",
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Get or set the agent for this chat",
		Args:        "[_agent id_]",
	},
	RequiresPortal: true,
	RequiresLogin:  true,
}

func fnAgent(ce *commands.Event) {
	client := getAIClient(ce)
	meta := getPortalMeta(ce)
	if client == nil || meta == nil {
		ce.Reply("Failed to access AI configuration")
		return
	}

	store := NewAgentStoreAdapter(client)

	if len(ce.Args) == 0 {
		// Show current agent
		agentID := meta.AgentID
		if agentID == "" {
			agentID = meta.DefaultAgentID
		}
		if agentID == "" {
			ce.Reply("No agent configured. Using default model: %s", client.effectiveModel(meta))
			return
		}
		agent, err := store.GetAgentByID(ce.Ctx, agentID)
		if err != nil {
			ce.Reply("Current agent ID: %s (not found)", agentID)
			return
		}
		ce.Reply("Current agent: **%s** (`%s`)\n%s", agent.Name, agent.ID, agent.Description)
		return
	}

	// Protect rooms with Boss agent from overrides
	if agents.IsBossAgent(meta.AgentID) || agents.IsBossAgent(meta.DefaultAgentID) {
		ce.Reply("Cannot change agent in a room managed by the Boss agent")
		return
	}

	// Set agent
	agentID := ce.Args[0]

	// Special case: "none" clears the agent
	if agentID == "none" || agentID == "clear" {
		meta.AgentID = ""
		meta.DefaultAgentID = ""
		client.savePortalQuiet(ce.Ctx, ce.Portal, "agent cleared")
		ce.Reply("Agent cleared. Using default model.")
		return
	}

	agent, err := store.GetAgentByID(ce.Ctx, agentID)
	if err != nil {
		ce.Reply("Agent not found: %s", agentID)
		return
	}

	meta.AgentID = agent.ID
	meta.DefaultAgentID = agent.ID
	if agent.SystemPrompt != "" {
		meta.SystemPrompt = agent.SystemPrompt
	}
	ce.Portal.OtherUserID = agentUserID(agent.ID)
	client.savePortalQuiet(ce.Ctx, ce.Portal, "agent change")
	client.ensureAgentGhostDisplayName(ce.Ctx, agent.ID, agent.Name)
	ce.Reply("Agent set to: **%s** (`%s`)", agent.Name, agent.ID)
}

// CommandAgents handles the !ai agents command
var CommandAgents = &commands.FullHandler{
	Func: fnAgents,
	Name: "agents",
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "List available agents",
	},
	RequiresLogin: true,
}

func fnAgents(ce *commands.Event) {
	client := getAIClient(ce)
	if client == nil {
		ce.Reply("Failed to access AI configuration")
		return
	}

	store := NewAgentStoreAdapter(client)
	agentsMap, err := store.LoadAgents(ce.Ctx)
	if err != nil {
		ce.Reply("Failed to load agents: %v", err)
		return
	}

	var sb strings.Builder
	sb.WriteString("## Available Agents\n\n")

	// Group by preset vs custom
	var presets, custom []string
	for id, agent := range agentsMap {
		line := fmt.Sprintf("• **%s** (`%s`)", agent.Name, id)
		if agent.Description != "" {
			line += fmt.Sprintf(" - %s", agent.Description)
		}
		if agent.IsPreset {
			presets = append(presets, line)
		} else {
			custom = append(custom, line)
		}
	}

	if len(presets) > 0 {
		sb.WriteString("**Presets:**\n")
		for _, line := range presets {
			sb.WriteString(line + "\n")
		}
		sb.WriteString("\n")
	}

	if len(custom) > 0 {
		sb.WriteString("**Custom:**\n")
		for _, line := range custom {
			sb.WriteString(line + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Use `!ai agent <id>` to switch agents")
	ce.Reply(sb.String())
}

// CommandCreateAgent handles the !ai create-agent command
var CommandCreateAgent = &commands.FullHandler{
	Func: fnCreateAgent,
	Name: "create-agent",
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Create a new custom agent",
		Args:        "<id> <name> [model] [system prompt...]",
	},
	RequiresLogin: true,
}

func fnCreateAgent(ce *commands.Event) {
	client := getAIClient(ce)
	if client == nil {
		ce.Reply("Failed to access AI configuration")
		return
	}

	if len(ce.Args) < 2 {
		ce.Reply("Usage: !ai create-agent <id> <name> [model] [system prompt...]\nExample: !ai create-agent my-helper \"My Helper\" gpt-4o You are a helpful assistant.")
		return
	}

	agentID := ce.Args[0]
	agentName := ce.Args[1]

	// Parse optional model and system prompt
	var model, systemPrompt string
	if len(ce.Args) > 2 {
		model = ce.Args[2]
	}
	if len(ce.Args) > 3 {
		systemPrompt = strings.Join(ce.Args[3:], " ")
	}

	store := NewAgentStoreAdapter(client)

	// Check if agent already exists
	if _, err := store.GetAgentByID(ce.Ctx, agentID); err == nil {
		ce.Reply("Agent with ID '%s' already exists", agentID)
		return
	}

	// Create new agent
	newAgent := &agents.AgentDefinition{
		ID:           agentID,
		Name:         agentName,
		SystemPrompt: systemPrompt,
		ToolProfile:  agents.ProfileFull,
		IsPreset:     false,
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
	}
	if model != "" {
		newAgent.Model = agents.ModelConfig{Primary: model}
	}

	if err := store.SaveAgent(ce.Ctx, newAgent); err != nil {
		ce.Reply("Failed to create agent: %v", err)
		return
	}

	ce.Reply("Created agent: **%s** (`%s`)\nUse `!ai agent %s` to use it", agentName, agentID, agentID)
}

// CommandDeleteAgent handles the !ai delete-agent command
var CommandDeleteAgent = &commands.FullHandler{
	Func: fnDeleteAgent,
	Name: "delete-agent",
	Help: commands.HelpMeta{
		Section:     HelpSectionAI,
		Description: "Delete a custom agent",
		Args:        "<id>",
	},
	RequiresLogin: true,
}

func fnDeleteAgent(ce *commands.Event) {
	client := getAIClient(ce)
	if client == nil {
		ce.Reply("Failed to access AI configuration")
		return
	}

	if len(ce.Args) < 1 {
		ce.Reply("Usage: !ai delete-agent <id>")
		return
	}

	agentID := ce.Args[0]
	store := NewAgentStoreAdapter(client)

	// Check if it's a preset
	if agents.IsPreset(agentID) || agents.IsBossAgent(agentID) {
		ce.Reply("Cannot delete preset agent: %s", agentID)
		return
	}

	if err := store.DeleteAgent(ce.Ctx, agentID); err != nil {
		ce.Reply("Failed to delete agent: %v", err)
		return
	}

	ce.Reply("Deleted agent: %s", agentID)
}
