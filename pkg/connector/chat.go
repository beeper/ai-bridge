package connector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"

	"github.com/beeper/ai-bridge/pkg/agents"
	"github.com/beeper/ai-bridge/pkg/agents/tools"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/event"
)

// Tool name constants
const (
	ToolNameCalculator  = "calculator"
	ToolNameWebSearch   = "web_search"
	ToolNameSetChatInfo = "set_chat_info"
)

func hasAssignedAgent(meta *PortalMetadata) bool {
	if meta == nil {
		return false
	}
	return meta.AgentID != "" || meta.DefaultAgentID != ""
}

// getDefaultToolsConfig returns the default tools configuration for a new room.
func getDefaultToolsConfig(_ string) ToolsConfig {
	config := ToolsConfig{
		Tools: make(map[string]*ToolEntry),
	}

	// Calculator - arithmetic and math
	registerTool(&config, mcp.Tool{
		Name:        ToolNameCalculator,
		Description: "Perform arithmetic calculations",
		Annotations: &mcp.ToolAnnotations{Title: "Calculator"},
	}, "builtin")

	// Web search - uses DuckDuckGo
	registerTool(&config, mcp.Tool{
		Name:        ToolNameWebSearch,
		Description: "Search the web for information",
		Annotations: &mcp.ToolAnnotations{Title: "Web Search"},
	}, "builtin")

	// Chat info tool (title/description)
	registerTool(&config, mcp.Tool{
		Name:        ToolNameSetChatInfo,
		Description: "Set the chat title and/or description (patches existing values)",
		Annotations: &mcp.ToolAnnotations{Title: "Set Chat Info"},
	}, "builtin")

	return config
}

// registerTool registers a tool in the ToolsConfig using MCP SDK types
func registerTool(config *ToolsConfig, tool mcp.Tool, toolType string) {
	if config.Tools == nil {
		config.Tools = make(map[string]*ToolEntry)
	}
	config.Tools[tool.Name] = &ToolEntry{
		Tool: tool,
		Type: toolType,
		// Enabled stays nil = auto (uses defaults based on context)
	}
}

// ensureToolsConfig ensures the portal has a valid ToolsConfig, initializing if empty
func ensureToolsConfig(meta *PortalMetadata, provider string) bool {
	changed := false
	if meta.ToolsConfig.Tools == nil {
		meta.ToolsConfig.Tools = make(map[string]*ToolEntry)
		changed = true
	}
	if len(meta.ToolsConfig.Tools) == 0 {
		meta.ToolsConfig = getDefaultToolsConfig(provider)
		changed = true
	}

	// Ensure web search tool exists (enabled by default unless explicitly disabled)
	if _, ok := meta.ToolsConfig.Tools[ToolNameWebSearch]; !ok {
		registerTool(&meta.ToolsConfig, mcp.Tool{
			Name:        ToolNameWebSearch,
			Description: "Search the web for information",
			Annotations: &mcp.ToolAnnotations{Title: "Web Search"},
		}, "builtin")
		changed = true
	}

	// Ensure chat info tool exists
	if _, ok := meta.ToolsConfig.Tools[ToolNameSetChatInfo]; !ok {
		registerTool(&meta.ToolsConfig, mcp.Tool{
			Name:        ToolNameSetChatInfo,
			Description: "Set the chat title and/or description (patches existing values)",
			Annotations: &mcp.ToolAnnotations{Title: "Set Chat Info"},
		}, "builtin")
		changed = true
	}

	// Only expose message tool when an agent is assigned to the room.
	if hasAssignedAgent(meta) {
		if _, ok := meta.ToolsConfig.Tools[ToolNameMessage]; !ok {
			registerTool(&meta.ToolsConfig, mcp.Tool{
				Name:        ToolNameMessage,
				Description: "Send messages and perform channel actions in the current chat",
				Annotations: &mcp.ToolAnnotations{Title: "Message"},
			}, "builtin")
			changed = true
		}
	} else if _, ok := meta.ToolsConfig.Tools[ToolNameMessage]; ok {
		delete(meta.ToolsConfig.Tools, ToolNameMessage)
		changed = true
	}

	return changed
}

// buildAvailableTools returns a list of ToolInfo for all tools based on current config
func (oc *AIClient) buildAvailableTools(meta *PortalMetadata) []ToolInfo {
	loginMeta := loginMetadata(oc.UserLogin)
	provider := loginMeta.Provider
	isOpenRouter := provider == ProviderOpenRouter || provider == ProviderBeeper

	// Ensure tools config is initialized
	ensureToolsConfig(meta, provider)

	// Check if model supports tool calling
	supportsTools := meta.Capabilities.SupportsToolCalling

	// Get agent policy ONCE before the loop to avoid redundant lookups
	var agentPolicy *tools.Policy
	var agent *agents.AgentDefinition
	var err error
	store := NewAgentStoreAdapter(oc)
	if hasAssignedAgent(meta) {
		agent, err = store.GetAgentForRoom(context.Background(), meta)
		if err == nil && agent != nil {
			agentPolicy = agents.CreatePolicyFromProfile(agent, tools.DefaultRegistry())
		}
	}

	var toolsList []ToolInfo

	for name, entry := range meta.ToolsConfig.Tools {
		if entry == nil {
			continue
		}
		if name == ToolNameMessage && !hasAssignedAgent(meta) {
			continue
		}

		// Get display name from MCP annotations or fall back to tool name
		displayName := entry.Tool.Name
		if entry.Tool.Annotations != nil && entry.Tool.Annotations.Title != "" {
			displayName = entry.Tool.Annotations.Title
		}

		// Determine availability based on tool type
		available := supportsTools
		if entry.Type == "plugin" || entry.Type == "provider" {
			available = true // Provider decides actual support
		}

		// Check agent policy first (if we have an agent)
		var enabled bool
		var source SettingSource
		var reason string

		if agentPolicy != nil {
			// Boss agent with boss tools - skip policy check
			isBossWithBossTool := agent != nil && agents.IsBossAgent(agent.ID) && tools.IsBossTool(name)
			if !isBossWithBossTool && !agentPolicy.IsAllowed(name) {
				enabled = false
				source = SourceAgentPolicy
				reason = "Disabled by agent policy"
			} else {
				enabled, source, reason = oc.getToolStateWithSource(meta, name, entry, isOpenRouter)
			}
		} else {
			enabled, source, reason = oc.getToolStateWithSource(meta, name, entry, isOpenRouter)
		}

		toolsList = append(toolsList, ToolInfo{
			Name:        name,
			DisplayName: displayName,
			Description: entry.Tool.Description,
			Type:        entry.Type,
			Enabled:     enabled,
			Available:   available,
			Source:      source,
			Reason:      reason,
		})
	}

	return toolsList
}

// getToolStateWithSource returns enabled state plus source and reason
func (oc *AIClient) getToolStateWithSource(meta *PortalMetadata, toolName string, entry *ToolEntry, _ bool) (bool, SettingSource, string) {
	// 1. Check room-level explicit setting
	if entry.Enabled != nil {
		return *entry.Enabled, SourceRoomOverride, ""
	}

	// 2. Check user-level defaults
	loginMeta := loginMetadata(oc.UserLogin)
	if loginMeta.Defaults != nil && loginMeta.Defaults.Tools != nil {
		if enabled, ok := loginMeta.Defaults.Tools[toolName]; ok {
			return enabled, SourceUserDefault, ""
		}
	}

	// 3. Fall back to global default
	return oc.getDefaultToolState(meta, toolName), SourceGlobalDefault, ""
}

// isToolEnabled checks if a specific tool is enabled
// Priority: Agent Policy → Room → User → Provider/Model defaults
func (oc *AIClient) isToolEnabled(meta *PortalMetadata, toolName string) bool {
	if toolName == ToolNameMessage && !hasAssignedAgent(meta) {
		return false
	}

	// 0. Check agent policy first (if room has an agent assigned)
	if hasAssignedAgent(meta) {
		store := NewAgentStoreAdapter(oc)
		agent, err := store.GetAgentForRoom(context.Background(), meta)
		if err == nil && agent != nil {
			// Boss agent has its own tools - always allow Boss tools for Boss agent
			if agents.IsBossAgent(agent.ID) && tools.IsBossTool(toolName) {
				return true
			}
			// Use agent policy to check if tool is allowed
			policy := agents.CreatePolicyFromProfile(agent, tools.DefaultRegistry())
			if !policy.IsAllowed(toolName) {
				return false
			}
		}
	}

	// 1. Check room-level explicit setting (can enable tools the agent allows)
	if entry, ok := meta.ToolsConfig.Tools[toolName]; ok && entry != nil {
		if entry.Enabled != nil {
			return *entry.Enabled
		}
	}

	// 2. Check user-level defaults
	loginMeta := loginMetadata(oc.UserLogin)
	if loginMeta.Defaults != nil && loginMeta.Defaults.Tools != nil {
		if enabled, ok := loginMeta.Defaults.Tools[toolName]; ok {
			return enabled
		}
	}

	// 3. Fall back to provider/model defaults
	return oc.getDefaultToolState(meta, toolName)
}

// getDefaultToolState returns the default enabled state for a tool
// Most tools are enabled by default when the model supports them
func (oc *AIClient) getDefaultToolState(meta *PortalMetadata, toolName string) bool {
	if toolName == ToolNameWebSearch {
		return true
	}
	return meta.Capabilities.SupportsToolCalling
}

// applyToolToggle applies a tool toggle from client
func (oc *AIClient) applyToolToggle(meta *PortalMetadata, toggle ToolToggle, provider string) {
	// Ensure tools config is initialized
	ensureToolsConfig(meta, provider)

	// Normalize tool name aliases
	toolName := normalizeToolName(toggle.Name)

	// Check if tool exists
	entry, ok := meta.ToolsConfig.Tools[toolName]
	if !ok || entry == nil {
		oc.log.Warn().Str("tool", toggle.Name).Msg("Unknown tool in toggle request")
		return
	}

	// Apply toggle
	entry.Enabled = &toggle.Enabled

	oc.log.Info().Str("tool", toolName).Bool("enabled", toggle.Enabled).Msg("Applied tool toggle from client")
}

// normalizeToolName converts common aliases to canonical tool names
func normalizeToolName(name string) string {
	switch name {
	case "calc":
		return ToolNameCalculator
	case "websearch", "search":
		return ToolNameWebSearch
	case "chatinfo", "chat_info", "setchatinfo":
		return ToolNameSetChatInfo
	default:
		return name
	}
}

// SearchUsers searches available AI agents by name/ID
func (oc *AIClient) SearchUsers(ctx context.Context, query string) ([]*bridgev2.ResolveIdentifierResponse, error) {
	oc.log.Debug().Str("query", query).Msg("Agent search requested")

	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, nil
	}

	// Load agents
	store := NewAgentStoreAdapter(oc)
	agentsMap, err := store.LoadAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load agents: %w", err)
	}

	// Filter agents by query (match ID, name, or description)
	var results []*bridgev2.ResolveIdentifierResponse
	for _, agent := range agentsMap {
		// Check if query matches agent ID, name, or description (case-insensitive)
		if !strings.Contains(strings.ToLower(agent.ID), query) &&
			!strings.Contains(strings.ToLower(agent.Name), query) &&
			!strings.Contains(strings.ToLower(agent.Description), query) {
			continue
		}

		modelID := oc.agentDefaultModel(agent)
		userID := agentModelUserID(agent.ID, modelID)
		ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, userID)
		if err != nil {
			oc.log.Warn().Err(err).Str("agent", agent.ID).Msg("Failed to get ghost for search result")
			continue
		}

		displayName := oc.agentModelDisplayName(agent.Name, modelID)
		oc.ensureAgentModelGhostDisplayName(ctx, agent.ID, modelID, agent.Name)

		results = append(results, &bridgev2.ResolveIdentifierResponse{
			UserID: userID,
			UserInfo: &bridgev2.UserInfo{
				Name:        ptr.Ptr(displayName),
				IsBot:       ptr.Ptr(true),
				Identifiers: []string{agent.ID},
			},
			Ghost: ghost,
		})
	}

	oc.log.Info().Str("query", query).Int("results", len(results)).Msg("Agent search completed")
	return results, nil
}

// GetContactList returns a list of available AI agents and models as contacts
func (oc *AIClient) GetContactList(ctx context.Context) ([]*bridgev2.ResolveIdentifierResponse, error) {
	oc.log.Debug().Msg("Contact list requested")

	// Load agents
	store := NewAgentStoreAdapter(oc)
	agentsMap, err := store.LoadAgents(ctx)
	if err != nil {
		oc.log.Error().Err(err).Msg("Failed to load agents")
		return nil, fmt.Errorf("failed to load agents: %w", err)
	}

	// Create a contact for each agent
	contacts := make([]*bridgev2.ResolveIdentifierResponse, 0, len(agentsMap))

	for _, agent := range agentsMap {
		// Get or create ghost for this agent
		modelID := oc.agentDefaultModel(agent)
		userID := agentModelUserID(agent.ID, modelID)
		ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, userID)
		if err != nil {
			oc.log.Warn().Err(err).Str("agent", agent.ID).Msg("Failed to get ghost for agent")
			continue
		}

		// Update ghost display name
		displayName := oc.agentModelDisplayName(agent.Name, modelID)
		oc.ensureAgentModelGhostDisplayName(ctx, agent.ID, modelID, agent.Name)

		contacts = append(contacts, &bridgev2.ResolveIdentifierResponse{
			UserID: userID,
			UserInfo: &bridgev2.UserInfo{
				Name:        ptr.Ptr(displayName),
				IsBot:       ptr.Ptr(true),
				Identifiers: []string{agent.ID},
			},
			Ghost: ghost,
		})
	}

	// Add contacts for available models
	models, err := oc.listAvailableModels(ctx, false)
	if err != nil {
		oc.log.Warn().Err(err).Msg("Failed to load model contact list")
	} else {
		for i := range models {
			model := &models[i]
			if model.ID == "" {
				continue
			}
			userID := modelUserID(model.ID)
			ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, userID)
			if err != nil {
				oc.log.Warn().Err(err).Str("model", model.ID).Msg("Failed to get ghost for model")
				continue
			}

			// Ensure ghost display name is set before returning
			oc.ensureGhostDisplayNameWithGhost(ctx, ghost, model.ID, model)

			contacts = append(contacts, &bridgev2.ResolveIdentifierResponse{
				UserID: userID,
				UserInfo: &bridgev2.UserInfo{
					Name:        ptr.Ptr(modelContactName(model.ID, model)),
					IsBot:       ptr.Ptr(false),
					Identifiers: modelContactIdentifiers(model.ID, model),
				},
				Ghost: ghost,
			})
		}
	}

	oc.log.Info().Int("count", len(contacts)).Msg("Returning contact list")
	return contacts, nil
}

// ResolveIdentifier resolves an agent ID to a ghost and optionally creates a chat
func (oc *AIClient) ResolveIdentifier(ctx context.Context, identifier string, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	// Identifier can be an agent ID (e.g., "beeper", "boss") or model ID for backwards compatibility
	id := strings.TrimSpace(identifier)
	if id == "" {
		return nil, fmt.Errorf("identifier is required")
	}

	store := NewAgentStoreAdapter(oc)

	// Check if identifier is an agent+model ghost ID (agent-{id}:model-{id})
	if agentID, modelID, ok := parseAgentModelFromGhostID(id); ok {
		agent, err := store.GetAgentByID(ctx, agentID)
		if err != nil || agent == nil {
			return nil, fmt.Errorf("agent '%s' not found", agentID)
		}
		return oc.resolveAgentIdentifierWithModel(ctx, agent, modelID, createChat)
	}

	// Try to find as agent first (bare agent ID like "beeper", "boss")
	agent, err := store.GetAgentByID(ctx, id)
	if err == nil && agent != nil {
		return oc.resolveAgentIdentifier(ctx, agent, createChat)
	}

	// Fallback: try as model ID for backwards compatibility
	resolved, valid, err := oc.resolveModelID(ctx, id)
	if err != nil {
		return nil, err
	}
	if valid && resolved != "" {
		return oc.resolveModelIdentifier(ctx, resolved, createChat)
	}
	return oc.resolveModelIdentifier(ctx, id, createChat)
}

// resolveAgentIdentifier resolves an agent to a ghost and optionally creates a chat
func (oc *AIClient) resolveAgentIdentifier(ctx context.Context, agent *agents.AgentDefinition, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	return oc.resolveAgentIdentifierWithModel(ctx, agent, "", createChat)
}

func (oc *AIClient) resolveAgentIdentifierWithModel(ctx context.Context, agent *agents.AgentDefinition, modelID string, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	if modelID == "" {
		modelID = oc.agentDefaultModel(agent)
	}
	userID := agentModelUserID(agent.ID, modelID)
	ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ghost: %w", err)
	}

	displayName := oc.agentModelDisplayName(agent.Name, modelID)
	oc.ensureAgentModelGhostDisplayName(ctx, agent.ID, modelID, agent.Name)

	var chatResp *bridgev2.CreateChatResponse
	if createChat {
		oc.log.Info().Str("agent", agent.ID).Msg("Creating new chat for agent")
		chatResp, err = oc.createAgentChatWithModel(ctx, agent, modelID)
		if err != nil {
			return nil, fmt.Errorf("failed to create chat: %w", err)
		}
	}

	return &bridgev2.ResolveIdentifierResponse{
		UserID: userID,
		UserInfo: &bridgev2.UserInfo{
			Name:        ptr.Ptr(displayName),
			IsBot:       ptr.Ptr(true),
			Identifiers: []string{agent.ID},
		},
		Ghost: ghost,
		Chat:  chatResp,
	}, nil
}

// resolveModelIdentifier resolves a model ID to a ghost (backwards compatibility)
func (oc *AIClient) resolveModelIdentifier(ctx context.Context, modelID string, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	// Get or create ghost
	userID := modelUserID(modelID)
	ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ghost: %w", err)
	}

	// Ensure ghost display name is set before returning
	oc.ensureGhostDisplayName(ctx, modelID)

	var chatResp *bridgev2.CreateChatResponse
	if createChat {
		oc.log.Info().Str("model", modelID).Msg("Creating new chat for model")
		chatResp, err = oc.createNewChat(ctx, modelID)
		if err != nil {
			return nil, fmt.Errorf("failed to create chat: %w", err)
		}
	}

	info := oc.findModelInfo(modelID)
	return &bridgev2.ResolveIdentifierResponse{
		UserID: userID,
		UserInfo: &bridgev2.UserInfo{
			Name:        ptr.Ptr(modelContactName(modelID, info)),
			IsBot:       ptr.Ptr(false),
			Identifiers: modelContactIdentifiers(modelID, info),
		},
		Ghost: ghost,
		Chat:  chatResp,
	}, nil
}

// createAgentChat creates a new chat room for an agent
func (oc *AIClient) createAgentChat(ctx context.Context, agent *agents.AgentDefinition) (*bridgev2.CreateChatResponse, error) {
	return oc.createAgentChatWithModel(ctx, agent, "")
}

func (oc *AIClient) createAgentChatWithModel(ctx context.Context, agent *agents.AgentDefinition, modelID string) (*bridgev2.CreateChatResponse, error) {
	if modelID == "" {
		modelID = oc.agentDefaultModel(agent)
	}

	portal, chatInfo, err := oc.initPortalForChat(ctx, PortalInitOpts{
		ModelID:      modelID,
		Title:        fmt.Sprintf("Chat with %s", agent.Name),
		SystemPrompt: agent.SystemPrompt,
	})
	if err != nil {
		return nil, err
	}

	// Set agent-specific metadata
	pm := portalMeta(portal)
	pm.AgentID = agent.ID
	pm.DefaultAgentID = agent.ID
	if agent.SystemPrompt != "" {
		pm.SystemPrompt = agent.SystemPrompt
	}
	if agent.ReasoningEffort != "" {
		pm.ReasoningEffort = agent.ReasoningEffort
	}

	agentGhostID := agentModelUserID(agent.ID, modelID)
	agentDisplayName := oc.agentModelDisplayName(agent.Name, modelID)

	// Update the OtherUserID to be the agent+model ghost
	portal.OtherUserID = agentGhostID

	if err := portal.Save(ctx); err != nil {
		return nil, fmt.Errorf("failed to save portal with agent config: %w", err)
	}

	// Update chat info members to use agent+model ghost
	members := chatInfo.Members
	if members == nil {
		members = &bridgev2.ChatMemberList{}
	}
	if members.MemberMap == nil {
		members.MemberMap = make(bridgev2.ChatMemberMap)
	}
	members.OtherUserID = agentGhostID

	humanID := humanUserID(oc.UserLogin.ID)

	humanMember := members.MemberMap[humanID]
	humanMember.EventSender = bridgev2.EventSender{
		IsFromMe:    true,
		SenderLogin: oc.UserLogin.ID,
	}

	agentMember := members.MemberMap[agentGhostID]
	agentMember.EventSender = bridgev2.EventSender{
		Sender:      agentGhostID,
		SenderLogin: oc.UserLogin.ID,
	}
	agentMember.UserInfo = &bridgev2.UserInfo{
		Name:  ptr.Ptr(agentDisplayName),
		IsBot: ptr.Ptr(true),
	}
	agentMember.MemberEventExtra = map[string]any{
		"displayname":         agentDisplayName,
		"com.beeper.ai.model": modelID,
		"com.beeper.ai.agent": agent.ID,
	}

	members.MemberMap = bridgev2.ChatMemberMap{
		humanID:      humanMember,
		agentGhostID: agentMember,
	}
	chatInfo.Members = members

	return &bridgev2.CreateChatResponse{
		PortalKey: portal.PortalKey,
		PortalInfo: &bridgev2.ChatInfo{
			Name:    chatInfo.Name,
			Members: chatInfo.Members,
		},
	}, nil
}

// createNewChat creates a new portal for a specific model
func (oc *AIClient) createNewChat(ctx context.Context, modelID string) (*bridgev2.CreateChatResponse, error) {
	portal, chatInfo, err := oc.initPortalForChat(ctx, PortalInitOpts{
		ModelID: modelID,
	})
	if err != nil {
		return nil, err
	}

	return &bridgev2.CreateChatResponse{
		PortalKey:  portal.PortalKey,
		PortalInfo: chatInfo,
		Portal:     portal,
	}, nil
}

// allocateNextChatIndex increments and returns the next chat index for this login
func (oc *AIClient) allocateNextChatIndex(ctx context.Context) (int, error) {
	meta := loginMetadata(oc.UserLogin)
	oc.chatLock.Lock()
	defer oc.chatLock.Unlock()

	meta.NextChatIndex++
	if err := oc.UserLogin.Save(ctx); err != nil {
		meta.NextChatIndex-- // Rollback on error
		return 0, fmt.Errorf("failed to save login: %w", err)
	}

	return meta.NextChatIndex, nil
}

// PortalInitOpts contains options for initializing a chat portal
type PortalInitOpts struct {
	ModelID      string
	Title        string
	SystemPrompt string
	CopyFrom     *PortalMetadata // For forked chats - copies config from source
	PortalKey    *networkid.PortalKey
}

// initPortalForChat handles common portal initialization logic.
// Returns the configured portal, chat info, and any error.
func (oc *AIClient) initPortalForChat(ctx context.Context, opts PortalInitOpts) (*bridgev2.Portal, *bridgev2.ChatInfo, error) {
	chatIndex, err := oc.allocateNextChatIndex(ctx)
	if err != nil {
		return nil, nil, err
	}

	slug := formatChatSlug(chatIndex)
	modelID := opts.ModelID
	if modelID == "" {
		modelID = oc.effectiveModel(nil)
	}

	title := opts.Title
	if title == "" {
		modelName := modelContactName(modelID, oc.findModelInfo(modelID))
		title = fmt.Sprintf("AI Chat with %s", modelName)
	}

	portalKey := portalKeyForChat(oc.UserLogin.ID)
	if opts.PortalKey != nil {
		portalKey = *opts.PortalKey
	}
	portal, err := oc.UserLogin.Bridge.GetPortalByKey(ctx, portalKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get portal: %w", err)
	}

	// Initialize or copy metadata
	var pmeta *PortalMetadata
	loginMeta := loginMetadata(oc.UserLogin)
	if opts.CopyFrom != nil {
		pmeta = &PortalMetadata{
			Model:               opts.CopyFrom.Model,
			Slug:                slug,
			Title:               title,
			SystemPrompt:        opts.CopyFrom.SystemPrompt,
			Temperature:         opts.CopyFrom.Temperature,
			MaxContextMessages:  opts.CopyFrom.MaxContextMessages,
			MaxCompletionTokens: opts.CopyFrom.MaxCompletionTokens,
			ReasoningEffort:     opts.CopyFrom.ReasoningEffort,
			Capabilities:        opts.CopyFrom.Capabilities,
			ToolsConfig:         opts.CopyFrom.ToolsConfig,
			ConversationMode:    opts.CopyFrom.ConversationMode,
			DefaultAgentID:      opts.CopyFrom.DefaultAgentID,
			AgentPrompt:         opts.CopyFrom.AgentPrompt,
		}
		modelID = opts.CopyFrom.Model
	} else {
		pmeta = &PortalMetadata{
			Model:        modelID,
			Slug:         slug,
			Title:        title,
			SystemPrompt: opts.SystemPrompt,
			Capabilities: getModelCapabilities(modelID, oc.findModelInfo(modelID)),
			ToolsConfig:  getDefaultToolsConfig(loginMeta.Provider),
		}
	}
	portal.Metadata = pmeta

	portal.RoomType = database.RoomTypeDM
	portal.OtherUserID = modelUserID(modelID)
	portal.Name = title
	portal.NameSet = true
	// Note: portal.Topic is NOT set to SystemPrompt - they are separate concepts

	if err := portal.Save(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to save portal: %w", err)
	}

	chatInfo := oc.composeChatInfo(title, modelID)
	return portal, chatInfo, nil
}

// handleFork creates a new chat and copies messages from the current conversation
func (oc *AIClient) handleFork(
	ctx context.Context,
	_ *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	arg string,
) {
	runCtx := oc.backgroundContext(ctx)

	// 1. Retrieve all messages from current chat
	messages, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(runCtx, portal.PortalKey, 10000)
	if err != nil {
		oc.sendSystemNotice(runCtx, portal, "Failed to retrieve messages: "+err.Error())
		return
	}

	if len(messages) == 0 {
		oc.sendSystemNotice(runCtx, portal, "No messages to fork.")
		return
	}

	// 2. If event ID specified, filter messages up to that point
	var messagesToCopy []*database.Message
	if arg != "" {
		// Validate Matrix event ID format
		if !strings.HasPrefix(arg, "$") {
			oc.sendSystemNotice(runCtx, portal, "Invalid event ID. Must start with '$'.")
			return
		}

		// Messages are newest-first, reverse iterate to find target
		found := false
		for i := len(messages) - 1; i >= 0; i-- {
			msg := messages[i]
			messagesToCopy = append(messagesToCopy, msg)

			// Check MXID field (Matrix event ID)
			if msg.MXID != "" && string(msg.MXID) == arg {
				found = true
				break
			}
			// Check message ID format "mx:$eventid"
			if strings.HasSuffix(string(msg.ID), arg) {
				found = true
				break
			}
		}

		if !found {
			oc.sendSystemNotice(runCtx, portal, fmt.Sprintf("Could not find event: %s", arg))
			return
		}
	} else {
		// Copy all messages (reverse to get chronological order)
		for i := len(messages) - 1; i >= 0; i-- {
			messagesToCopy = append(messagesToCopy, messages[i])
		}
	}

	// 3. Create new chat with same configuration
	newPortal, chatInfo, err := oc.createForkedChat(runCtx, portal, meta)
	if err != nil {
		oc.sendSystemNotice(runCtx, portal, "Failed to create forked chat: "+err.Error())
		return
	}

	// 4. Create Matrix room
	if err := newPortal.CreateMatrixRoom(runCtx, oc.UserLogin, chatInfo); err != nil {
		oc.sendSystemNotice(runCtx, portal, "Failed to create room: "+err.Error())
		return
	}

	// 5. Copy messages to new chat
	copiedCount := oc.copyMessagesToChat(runCtx, newPortal, messagesToCopy)

	// 6. Send notice with link
	roomLink := fmt.Sprintf("https://matrix.to/#/%s", newPortal.MXID)
	oc.sendSystemNotice(runCtx, portal, fmt.Sprintf(
		"Forked %d messages to new chat.\nOpen: %s",
		copiedCount, roomLink,
	))
}

// handleNewChat creates a new empty chat with a specified model
func (oc *AIClient) handleNewChat(
	ctx context.Context,
	_ *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	arg string,
) {
	runCtx := oc.backgroundContext(ctx)

	// Determine model: use argument or current chat's model
	modelID := arg
	if modelID == "" {
		modelID = oc.effectiveModel(meta)
	}

	// Validate model
	valid, err := oc.validateModel(runCtx, modelID)
	if err != nil || !valid {
		oc.sendSystemNotice(runCtx, portal, fmt.Sprintf("Invalid model: %s", modelID))
		return
	}

	// Create new chat with default settings
	newPortal, chatInfo, err := oc.createNewChatWithModel(runCtx, modelID)
	if err != nil {
		oc.sendSystemNotice(runCtx, portal, "Failed to create chat: "+err.Error())
		return
	}

	// Create Matrix room
	if err := newPortal.CreateMatrixRoom(runCtx, oc.UserLogin, chatInfo); err != nil {
		oc.sendSystemNotice(runCtx, portal, "Failed to create room: "+err.Error())
		return
	}

	// Send welcome message (excluded from LLM history)
	oc.sendWelcomeMessage(runCtx, newPortal)

	// Send confirmation with link
	roomLink := fmt.Sprintf("https://matrix.to/#/%s", newPortal.MXID)
	oc.sendSystemNotice(runCtx, portal, fmt.Sprintf(
		"Created new %s chat.\nOpen: %s",
		modelContactName(modelID, oc.findModelInfo(modelID)), roomLink,
	))
}

// createForkedChat creates a new portal inheriting config from source
func (oc *AIClient) createForkedChat(
	ctx context.Context,
	sourcePortal *bridgev2.Portal,
	sourceMeta *PortalMetadata,
) (*bridgev2.Portal, *bridgev2.ChatInfo, error) {
	sourceTitle := sourceMeta.Title
	if sourceTitle == "" {
		sourceTitle = sourcePortal.Name
	}
	title := fmt.Sprintf("%s (Fork)", sourceTitle)

	return oc.initPortalForChat(ctx, PortalInitOpts{
		Title:    title,
		CopyFrom: sourceMeta,
	})
}

// copyMessagesToChat queues messages to be bridged to the new chat
// Returns the count of successfully queued messages
func (oc *AIClient) copyMessagesToChat(
	_ context.Context,
	destPortal *bridgev2.Portal,
	messages []*database.Message,
) int {
	copiedCount := 0
	skippedCount := 0

	for _, srcMsg := range messages {
		srcMeta := messageMeta(srcMsg)
		if srcMeta == nil || srcMeta.Body == "" {
			skippedCount++
			continue
		}

		// Determine sender
		var sender bridgev2.EventSender
		if srcMeta.Role == "user" {
			sender = bridgev2.EventSender{
				Sender:      humanUserID(oc.UserLogin.ID),
				SenderLogin: oc.UserLogin.ID,
				IsFromMe:    true,
			}
		} else {
			sender = bridgev2.EventSender{
				Sender:      srcMsg.SenderID,
				SenderLogin: oc.UserLogin.ID,
				IsFromMe:    false,
			}
		}

		// Create remote message for bridging
		remoteMsg := &OpenAIRemoteMessage{
			PortalKey: destPortal.PortalKey,
			ID:        networkid.MessageID(fmt.Sprintf("fork:%s", uuid.NewString())),
			Sender:    sender,
			Content:   srcMeta.Body,
			Timestamp: srcMsg.Timestamp,
			Metadata: &MessageMetadata{
				Role: srcMeta.Role,
				Body: srcMeta.Body,
			},
		}

		oc.UserLogin.QueueRemoteEvent(remoteMsg)
		copiedCount++
	}

	// Log if partial copy occurred (some messages were skipped)
	if skippedCount > 0 {
		oc.log.Warn().
			Int("copied", copiedCount).
			Int("skipped", skippedCount).
			Int("total", len(messages)).
			Msg("Partial fork - some messages were skipped due to missing metadata")
	}

	return copiedCount
}

// createNewChatWithModel creates a new chat portal with the specified model and default settings
func (oc *AIClient) createNewChatWithModel(ctx context.Context, modelID string) (*bridgev2.Portal, *bridgev2.ChatInfo, error) {
	return oc.initPortalForChat(ctx, PortalInitOpts{
		ModelID: modelID,
	})
}

// chatInfoFromPortal builds ChatInfo from an existing portal
func (oc *AIClient) chatInfoFromPortal(portal *bridgev2.Portal) *bridgev2.ChatInfo {
	meta := portalMeta(portal)
	modelID := oc.effectiveModel(meta)
	title := meta.Title
	if title == "" {
		if portal.Name != "" {
			title = portal.Name
		} else {
			title = modelContactName(modelID, oc.findModelInfo(modelID))
		}
	}
	return oc.composeChatInfo(title, modelID)
}

// composeChatInfo creates a ChatInfo struct for a chat
func (oc *AIClient) composeChatInfo(title, modelID string) *bridgev2.ChatInfo {
	if modelID == "" {
		modelID = oc.effectiveModel(nil)
	}
	modelInfo := oc.findModelInfo(modelID)
	modelName := modelContactName(modelID, modelInfo)
	if title == "" {
		title = modelName
	}
	members := bridgev2.ChatMemberMap{
		humanUserID(oc.UserLogin.ID): {
			EventSender: bridgev2.EventSender{
				IsFromMe:    true,
				SenderLogin: oc.UserLogin.ID,
			},
			Membership: event.MembershipJoin,
		},
		modelUserID(modelID): {
			EventSender: bridgev2.EventSender{
				Sender:      modelUserID(modelID),
				SenderLogin: oc.UserLogin.ID,
			},
			Membership: event.MembershipJoin,
			UserInfo: &bridgev2.UserInfo{
				Name:        ptr.Ptr(modelName),
				IsBot:       ptr.Ptr(false),
				Identifiers: modelContactIdentifiers(modelID, modelInfo),
			},
			// Set displayname directly in membership event content
			// This works because MemberEventContent.Displayname has omitempty
			MemberEventExtra: map[string]any{
				"displayname":         modelName,
				"com.beeper.ai.model": modelID,
			},
		},
	}
	return &bridgev2.ChatInfo{
		Name:  ptr.Ptr(title),
		Topic: nil, // Topic managed via Matrix events, not system prompt
		Type:  ptr.Ptr(database.RoomTypeDM),
		Members: &bridgev2.ChatMemberList{
			IsFull:      true,
			OtherUserID: modelUserID(modelID),
			MemberMap:   members,
			// Set power levels so only bridge bot can modify room_capabilities (100)
			// while any user can modify room_settings (0)
			PowerLevels: &bridgev2.PowerLevelOverrides{
				Events: map[event.Type]int{
					RoomCapabilitiesEventType: 100, // Only bridge bot
					RoomSettingsEventType:     0,   // Any user
				},
			},
		},
		// Broadcast initial room config after room creation so desktop clients
		// can read the model and other settings from room state
		ExtraUpdates: func(ctx context.Context, portal *bridgev2.Portal) bool {
			if err := oc.BroadcastRoomState(ctx, portal); err != nil {
				oc.log.Warn().Err(err).Msg("Failed to broadcast initial room state")
			}
			return false // no portal changes needed
		},
	}
}

// updatePortalConfig applies room settings to portal metadata
func (oc *AIClient) updatePortalConfig(ctx context.Context, portal *bridgev2.Portal, config *RoomSettingsEventContent) {
	meta := portalMeta(portal)
	loginMeta := loginMetadata(oc.UserLogin)

	// Track old model for membership change
	oldModel := meta.Model

	// Update only non-empty/non-zero values
	if config.Model != "" {
		meta.Model = config.Model
		// Update capabilities when model changes
		meta.Capabilities = getModelCapabilities(config.Model, oc.findModelInfo(config.Model))
	}
	if config.SystemPrompt != "" {
		meta.SystemPrompt = config.SystemPrompt
	}
	if config.Temperature != nil {
		meta.Temperature = *config.Temperature
	}
	if config.MaxContextMessages > 0 {
		meta.MaxContextMessages = config.MaxContextMessages
	}
	if config.MaxCompletionTokens > 0 {
		meta.MaxCompletionTokens = config.MaxCompletionTokens
	}
	if config.ReasoningEffort != "" {
		meta.ReasoningEffort = config.ReasoningEffort
	}
	if config.ConversationMode != "" {
		meta.ConversationMode = config.ConversationMode
	}
	if config.DefaultAgentID != "" {
		meta.DefaultAgentID = config.DefaultAgentID
	}

	// Handle tool toggle from client
	if config.ToolToggle != nil {
		oc.applyToolToggle(meta, *config.ToolToggle, loginMeta.Provider)
	}

	meta.LastRoomStateSync = time.Now().Unix()

	// Handle model switch - generate membership events if model changed
	if config.Model != "" && oldModel != "" && config.Model != oldModel {
		oc.handleModelSwitch(ctx, portal, oldModel, config.Model)
	}

	// Persist changes
	if err := portal.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to save portal after config update")
	}

	// Re-broadcast room state to confirm changes to all clients
	if err := oc.BroadcastRoomState(ctx, portal); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to re-broadcast room state after config update")
	}
}

// handleModelSwitch generates membership change events when switching models
// This creates leave/join events to show the model transition in the room timeline
// For agent rooms, it swaps the agent+model ghost (e.g., "Beeper AI (Claude)" -> "Beeper AI (GPT)")
func (oc *AIClient) handleModelSwitch(ctx context.Context, portal *bridgev2.Portal, oldModel, newModel string) {
	if oldModel == newModel || oldModel == "" || newModel == "" {
		return
	}

	meta := portalMeta(portal)
	agentID := meta.AgentID
	if agentID == "" {
		agentID = meta.DefaultAgentID
	}

	// Check if this is an agent room - use agent+model ghosts for swap
	if agentID != "" {
		oc.handleAgentModelSwitch(ctx, portal, agentID, oldModel, newModel)
		return
	}

	// For non-agent rooms, use model-only ghosts
	oc.log.Info().
		Str("old_model", oldModel).
		Str("new_model", newModel).
		Stringer("portal", portal.PortalKey).
		Msg("Handling model switch")

	oldInfo := oc.findModelInfo(oldModel)
	newInfo := oc.findModelInfo(newModel)
	oldModelName := modelContactName(oldModel, oldInfo)
	newModelName := modelContactName(newModel, newInfo)

	// Pre-update the new model ghost's profile before queueing the event
	// This ensures the ghost has a display name set in its Matrix profile
	newGhost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, modelUserID(newModel))
	if err != nil {
		oc.log.Warn().Err(err).Str("model", newModel).Msg("Failed to get ghost for model switch")
	} else {
		oc.ensureGhostDisplayNameWithGhost(ctx, newGhost, newModel, newInfo)
	}

	// Create member changes: old model leaves, new model joins
	// Use MemberEventExtra to set displayname directly in the membership event
	// This works because MemberEventContent.Displayname has omitempty, so our Raw value is preserved
	memberChanges := &bridgev2.ChatMemberList{
		MemberMap: bridgev2.ChatMemberMap{
			modelUserID(oldModel): {
				EventSender: bridgev2.EventSender{
					Sender:      modelUserID(oldModel),
					SenderLogin: oc.UserLogin.ID,
				},
				Membership:     event.MembershipLeave,
				PrevMembership: event.MembershipJoin,
			},
			modelUserID(newModel): {
				EventSender: bridgev2.EventSender{
					Sender:      modelUserID(newModel),
					SenderLogin: oc.UserLogin.ID,
				},
				Membership: event.MembershipJoin,
				UserInfo: &bridgev2.UserInfo{
					Name:        ptr.Ptr(newModelName),
					IsBot:       ptr.Ptr(false),
					Identifiers: modelContactIdentifiers(newModel, newInfo),
				},
				MemberEventExtra: map[string]any{
					"displayname":         newModelName,
					"com.beeper.ai.model": newModel,
				},
			},
		},
	}

	// Update portal's OtherUserID to new model
	portal.OtherUserID = modelUserID(newModel)

	// Queue the ChatInfoChange event
	evt := &simplevent.ChatInfoChange{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventChatInfoChange,
			PortalKey: portal.PortalKey,
			Timestamp: time.Now(),
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Str("action", "model_switch").
					Str("old_model", oldModel).
					Str("new_model", newModel)
			},
		},
		ChatInfoChange: &bridgev2.ChatInfoChange{
			MemberChanges: memberChanges,
		},
	}

	oc.UserLogin.QueueRemoteEvent(evt)

	// Send a notice about the model change from the bridge bot
	notice := fmt.Sprintf("Switched from %s to %s", oldModelName, newModelName)
	oc.sendSystemNotice(ctx, portal, notice)

	// Update bridge info and capabilities to resend room features state event with new capabilities
	// This ensures the client knows what features the new model supports (vision, audio, etc.)
	portal.UpdateBridgeInfo(ctx)
	portal.UpdateCapabilities(ctx, oc.UserLogin, true)

	// Ensure only 1 AI ghost in room
	if err := oc.ensureSingleAIGhost(ctx, portal); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to ensure single AI ghost after model switch")
	}
}

// handleAgentModelSwitch handles model switching for agent rooms.
// Swaps the agent+model ghost (e.g., "Beeper AI (Claude)" -> "Beeper AI (GPT)")
func (oc *AIClient) handleAgentModelSwitch(ctx context.Context, portal *bridgev2.Portal, agentID, oldModel, newModel string) {
	// Get the agent to determine display name
	store := NewAgentStoreAdapter(oc)
	agent, err := store.GetAgentByID(ctx, agentID)
	if err != nil || agent == nil {
		oc.log.Warn().Err(err).Str("agent", agentID).Msg("Agent not found for model switch")
		return
	}

	oc.log.Info().
		Str("agent", agentID).
		Str("old_model", oldModel).
		Str("new_model", newModel).
		Stringer("portal", portal.PortalKey).
		Msg("Handling agent model switch")

	oldGhostID := agentModelUserID(agentID, oldModel)
	newGhostID := agentModelUserID(agentID, newModel)

	oldDisplayName := oc.agentModelDisplayName(agent.Name, oldModel)
	newDisplayName := oc.agentModelDisplayName(agent.Name, newModel)

	// Create member changes: old agent+model leaves, new agent+model joins
	memberChanges := &bridgev2.ChatMemberList{
		MemberMap: bridgev2.ChatMemberMap{
			oldGhostID: {
				EventSender: bridgev2.EventSender{
					Sender:      oldGhostID,
					SenderLogin: oc.UserLogin.ID,
				},
				Membership:     event.MembershipLeave,
				PrevMembership: event.MembershipJoin,
			},
			newGhostID: {
				EventSender: bridgev2.EventSender{
					Sender:      newGhostID,
					SenderLogin: oc.UserLogin.ID,
				},
				Membership: event.MembershipJoin,
				UserInfo: &bridgev2.UserInfo{
					Name:  ptr.Ptr(newDisplayName),
					IsBot: ptr.Ptr(true),
				},
				MemberEventExtra: map[string]any{
					"displayname":         newDisplayName,
					"com.beeper.ai.model": newModel,
					"com.beeper.ai.agent": agentID,
				},
			},
		},
	}

	// Update portal's OtherUserID to new agent+model ghost
	portal.OtherUserID = newGhostID

	// Queue the ChatInfoChange event
	evt := &simplevent.ChatInfoChange{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventChatInfoChange,
			PortalKey: portal.PortalKey,
			Timestamp: time.Now(),
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Str("action", "agent_model_switch").
					Str("agent", agentID).
					Str("old_model", oldModel).
					Str("new_model", newModel)
			},
		},
		ChatInfoChange: &bridgev2.ChatInfoChange{
			MemberChanges: memberChanges,
		},
	}

	oc.UserLogin.QueueRemoteEvent(evt)

	// Send a notice about the model change
	notice := fmt.Sprintf("Switched from %s to %s", oldDisplayName, newDisplayName)
	oc.sendSystemNotice(ctx, portal, notice)

	// Update bridge info and capabilities
	portal.UpdateBridgeInfo(ctx)
	portal.UpdateCapabilities(ctx, oc.UserLogin, true)

	// Ensure only 1 AI ghost in room
	if err := oc.ensureSingleAIGhost(ctx, portal); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to ensure single AI ghost after agent model switch")
	}
}

// ensureSingleAIGhost ensures only 1 model/agent ghost is in the room at a time.
// Updates portal.OtherUserID if it doesn't match the expected ghost.
func (oc *AIClient) ensureSingleAIGhost(ctx context.Context, portal *bridgev2.Portal) error {
	meta := portalMeta(portal)

	// Determine which ghost SHOULD be in the room
	var expectedGhostID networkid.UserID
	agentID := meta.AgentID
	if agentID == "" {
		agentID = meta.DefaultAgentID
	}

	modelID := oc.effectiveModel(meta)
	if agentID != "" {
		expectedGhostID = agentModelUserID(agentID, modelID)
	} else {
		expectedGhostID = modelUserID(modelID)
	}

	// Update portal.OtherUserID if mismatched
	if portal.OtherUserID != expectedGhostID {
		oc.log.Debug().
			Str("old_ghost", string(portal.OtherUserID)).
			Str("new_ghost", string(expectedGhostID)).
			Stringer("portal", portal.PortalKey).
			Msg("Updating portal OtherUserID to match expected ghost")
		portal.OtherUserID = expectedGhostID
		return portal.Save(ctx)
	}
	return nil
}

// BroadcastRoomState sends current room capabilities and settings to Matrix room state
func (oc *AIClient) BroadcastRoomState(ctx context.Context, portal *bridgev2.Portal) error {
	if err := oc.broadcastCapabilities(ctx, portal); err != nil {
		return err
	}
	return oc.broadcastSettings(ctx, portal)
}

// buildEffectiveSettings builds the effective settings with source explanations
func (oc *AIClient) buildEffectiveSettings(meta *PortalMetadata) *EffectiveSettings {
	loginMeta := loginMetadata(oc.UserLogin)

	return &EffectiveSettings{
		Model:           oc.getModelWithSource(meta, loginMeta),
		SystemPrompt:    oc.getPromptWithSource(meta, loginMeta),
		Temperature:     oc.getTempWithSource(meta, loginMeta),
		ReasoningEffort: oc.getReasoningWithSource(meta, loginMeta),
	}
}

func (oc *AIClient) getModelWithSource(meta *PortalMetadata, loginMeta *UserLoginMetadata) SettingExplanation {
	if meta != nil && meta.Model != "" {
		return SettingExplanation{Value: meta.Model, Source: SourceRoomOverride}
	}
	if loginMeta.Defaults != nil && loginMeta.Defaults.Model != "" {
		return SettingExplanation{Value: loginMeta.Defaults.Model, Source: SourceUserDefault}
	}
	return SettingExplanation{Value: oc.defaultModelForProvider(), Source: SourceProviderConfig}
}

func (oc *AIClient) getPromptWithSource(meta *PortalMetadata, loginMeta *UserLoginMetadata) SettingExplanation {
	if meta != nil && meta.SystemPrompt != "" {
		return SettingExplanation{Value: meta.SystemPrompt, Source: SourceRoomOverride}
	}
	if loginMeta.Defaults != nil && loginMeta.Defaults.SystemPrompt != "" {
		return SettingExplanation{Value: loginMeta.Defaults.SystemPrompt, Source: SourceUserDefault}
	}
	if oc.connector.Config.DefaultSystemPrompt != "" {
		return SettingExplanation{Value: oc.connector.Config.DefaultSystemPrompt, Source: SourceProviderConfig}
	}
	return SettingExplanation{Value: "", Source: SourceGlobalDefault}
}

func (oc *AIClient) getTempWithSource(meta *PortalMetadata, loginMeta *UserLoginMetadata) SettingExplanation {
	if meta != nil && meta.Temperature > 0 {
		return SettingExplanation{Value: meta.Temperature, Source: SourceRoomOverride}
	}
	if loginMeta.Defaults != nil && loginMeta.Defaults.Temperature != nil {
		return SettingExplanation{Value: *loginMeta.Defaults.Temperature, Source: SourceUserDefault}
	}
	return SettingExplanation{Value: defaultTemperature, Source: SourceGlobalDefault}
}

func (oc *AIClient) getReasoningWithSource(meta *PortalMetadata, loginMeta *UserLoginMetadata) SettingExplanation {
	// Check model support first
	if meta != nil && !meta.Capabilities.SupportsReasoning {
		return SettingExplanation{Value: nil, Source: SourceModelLimit, Reason: "Model does not support reasoning"}
	}
	if meta != nil && meta.ReasoningEffort != "" {
		return SettingExplanation{Value: meta.ReasoningEffort, Source: SourceRoomOverride}
	}
	if loginMeta.Defaults != nil && loginMeta.Defaults.ReasoningEffort != "" {
		return SettingExplanation{Value: loginMeta.Defaults.ReasoningEffort, Source: SourceUserDefault}
	}
	if meta != nil && meta.Capabilities.SupportsReasoning {
		return SettingExplanation{Value: defaultReasoningEffort, Source: SourceGlobalDefault}
	}
	return SettingExplanation{Value: "", Source: SourceGlobalDefault}
}

// broadcastCapabilities sends bridge-controlled capabilities to Matrix room state
// This event is protected by power levels (100) so only the bridge bot can modify
func (oc *AIClient) broadcastCapabilities(ctx context.Context, portal *bridgev2.Portal) error {
	if portal.MXID == "" {
		return fmt.Errorf("portal has no Matrix room ID")
	}

	meta := portalMeta(portal)
	loginMeta := loginMetadata(oc.UserLogin)

	// Ensure tools config is initialized
	if ensureToolsConfig(meta, loginMeta.Provider) {
		if err := portal.Save(ctx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to save portal after tools initialization")
		}
	}

	// Build reasoning effort options if model supports reasoning
	var reasoningEfforts []ReasoningEffortOption
	if meta.Capabilities.SupportsReasoning {
		reasoningEfforts = []ReasoningEffortOption{
			{Value: "low", Label: "Low"},
			{Value: "medium", Label: "Medium"},
			{Value: "high", Label: "High"},
		}
	}

	content := &RoomCapabilitiesEventContent{
		Capabilities:           &meta.Capabilities,
		AvailableTools:         oc.buildAvailableTools(meta),
		ReasoningEffortOptions: reasoningEfforts,
		Provider:               loginMeta.Provider,
		EffectiveSettings:      oc.buildEffectiveSettings(meta),
	}

	bot := oc.UserLogin.Bridge.Bot
	_, err := bot.SendState(ctx, portal.MXID, RoomCapabilitiesEventType, "", &event.Content{
		Parsed: content,
	}, time.Time{})

	if err != nil {
		oc.log.Warn().Err(err).Msg("Failed to broadcast room capabilities")
		return err
	}

	oc.log.Debug().Str("model", meta.Model).Msg("Broadcasted room capabilities")
	return nil
}

// broadcastSettings sends user-editable settings to Matrix room state
// This event uses normal power levels (0) so users can modify
func (oc *AIClient) broadcastSettings(ctx context.Context, portal *bridgev2.Portal) error {
	if portal.MXID == "" {
		return fmt.Errorf("portal has no Matrix room ID")
	}

	meta := portalMeta(portal)

	content := &RoomSettingsEventContent{
		Model:               meta.Model,
		SystemPrompt:        meta.SystemPrompt,
		Temperature:         &meta.Temperature,
		MaxContextMessages:  meta.MaxContextMessages,
		MaxCompletionTokens: meta.MaxCompletionTokens,
		ReasoningEffort:     meta.ReasoningEffort,
		ConversationMode:    meta.ConversationMode,
		DefaultAgentID:      meta.DefaultAgentID,
		// Note: ToolToggle is only for setting changes, not broadcasts
	}

	bot := oc.UserLogin.Bridge.Bot
	_, err := bot.SendState(ctx, portal.MXID, RoomSettingsEventType, "", &event.Content{
		Parsed: content,
	}, time.Time{})

	if err != nil {
		oc.log.Warn().Err(err).Msg("Failed to broadcast room settings")
		return err
	}

	meta.LastRoomStateSync = time.Now().Unix()
	if err := portal.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to save portal after state broadcast")
	}

	oc.log.Debug().Str("model", meta.Model).Msg("Broadcasted room settings")
	return nil
}

// sendSystemNotice sends an informational notice to the room from the bridge bot
func (oc *AIClient) sendSystemNotice(ctx context.Context, portal *bridgev2.Portal, message string) {
	if portal == nil || portal.MXID == "" {
		return
	}
	bot := oc.UserLogin.Bridge.Bot
	if bot == nil {
		return
	}

	content := &event.MessageEventContent{
		MsgType: event.MsgNotice,
		Body:    message,
	}

	if _, err := bot.SendMessage(ctx, portal.MXID, event.EventMessage, &event.Content{
		Parsed: content,
	}, nil); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to send system notice")
	}
}

// Bootstrap and initialization functions

func (oc *AIClient) scheduleBootstrap() {
	backgroundCtx := oc.UserLogin.Bridge.BackgroundCtx
	oc.bootstrapOnce.Do(func() {
		go oc.bootstrap(backgroundCtx)
	})
}

func (oc *AIClient) bootstrap(ctx context.Context) {
	logCtx := oc.log.With().Str("component", "openai-chat-bootstrap").Logger().WithContext(ctx)
	oc.waitForLoginPersisted(logCtx)

	meta := loginMetadata(oc.UserLogin)

	// Check if bootstrap already completed successfully
	if meta.ChatsSynced {
		oc.log.Debug().Msg("Chats already synced, skipping bootstrap")
		// Still sync counter in case portals were created externally
		if err := oc.syncChatCounter(logCtx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to sync chat counter")
		}
		return
	}

	oc.log.Info().Msg("Starting bootstrap for new login")

	if err := oc.syncChatCounter(logCtx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to sync chat counter")
		return
	}

	// Create default chat room with Beeper AI agent
	if err := oc.ensureDefaultChat(logCtx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to ensure default chat")
		// Continue anyway - default chat is optional
	}

	// Mark bootstrap as complete only after successful completion
	meta.ChatsSynced = true
	if err := oc.UserLogin.Save(logCtx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to save ChatsSynced flag")
	} else {
		oc.log.Info().Msg("Bootstrap completed successfully, ChatsSynced flag set")
	}
}

func (oc *AIClient) waitForLoginPersisted(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(60 * time.Second)
	for {
		_, err := oc.UserLogin.Bridge.DB.UserLogin.GetByID(ctx, oc.UserLogin.ID)
		if err == nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			oc.log.Warn().Msg("Timed out waiting for login to persist, continuing anyway")
			return
		case <-ticker.C:
		}
	}
}

func (oc *AIClient) syncChatCounter(ctx context.Context) error {
	meta := loginMetadata(oc.UserLogin)
	portals, err := oc.listAllChatPortals(ctx)
	if err != nil {
		return err
	}
	maxIdx := meta.NextChatIndex
	for _, portal := range portals {
		pm := portalMeta(portal)
		if idx, ok := parseChatSlug(pm.Slug); ok && idx > maxIdx {
			maxIdx = idx
		}
	}
	if maxIdx > meta.NextChatIndex {
		meta.NextChatIndex = maxIdx
		return oc.UserLogin.Save(ctx)
	}
	return nil
}

func (oc *AIClient) ensureDefaultChat(ctx context.Context) error {
	oc.log.Debug().Msg("Ensuring default AI chat room exists")
	loginMeta := loginMetadata(oc.UserLogin)

	if loginMeta.DefaultChatPortalID != "" {
		portalKey := networkid.PortalKey{
			ID:       networkid.PortalID(loginMeta.DefaultChatPortalID),
			Receiver: oc.UserLogin.ID,
		}
		portal, err := oc.UserLogin.Bridge.GetPortalByKey(ctx, portalKey)
		if err != nil {
			oc.log.Warn().Err(err).Msg("Failed to load default chat portal by ID")
		} else if portal != nil {
			if portal.MXID != "" {
				oc.log.Debug().Stringer("portal", portal.PortalKey).Msg("Existing default chat already has MXID")
				return nil
			}
			info := oc.chatInfoFromPortal(portal)
			oc.log.Info().Stringer("portal", portal.PortalKey).Msg("Default chat missing MXID; creating Matrix room")
			err := portal.CreateMatrixRoom(ctx, oc.UserLogin, info)
			if err != nil {
				oc.log.Err(err).Msg("Failed to create Matrix room for default chat")
			}
			oc.sendWelcomeMessage(ctx, portal)
			return err
		}
	}

	portals, err := oc.listAllChatPortals(ctx)
	if err != nil {
		oc.log.Err(err).Msg("Failed to list chat portals")
		return err
	}

	var defaultPortal *bridgev2.Portal
	var minIdx int
	for _, portal := range portals {
		pm := portalMeta(portal)
		if idx, ok := parseChatSlug(pm.Slug); ok {
			if defaultPortal == nil || idx < minIdx {
				minIdx = idx
				defaultPortal = portal
			}
		} else if defaultPortal == nil {
			defaultPortal = portal
		}
	}

	if defaultPortal != nil {
		loginMeta.DefaultChatPortalID = string(defaultPortal.PortalKey.ID)
		if err := oc.UserLogin.Save(ctx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to persist default chat portal ID")
		}
		if defaultPortal.MXID != "" {
			oc.log.Debug().Stringer("portal", defaultPortal.PortalKey).Msg("Existing chat already has MXID")
			return nil
		}
		info := oc.chatInfoFromPortal(defaultPortal)
		oc.log.Info().Stringer("portal", defaultPortal.PortalKey).Msg("Existing portal missing MXID; creating Matrix room")
		err := defaultPortal.CreateMatrixRoom(ctx, oc.UserLogin, info)
		if err != nil {
			oc.log.Err(err).Msg("Failed to create Matrix room for existing portal")
		}
		oc.sendWelcomeMessage(ctx, defaultPortal)
		return err
	}

	// Create default chat with Beeper AI agent
	beeperAgent := agents.GetBeeperAI()
	if beeperAgent == nil {
		return fmt.Errorf("beeper AI agent not found")
	}

	// Determine model from agent config or use default
	modelID := beeperAgent.Model.Primary
	if modelID == "" {
		modelID = oc.effectiveModel(nil)
	}

	portal, chatInfo, err := oc.initPortalForChat(ctx, PortalInitOpts{
		ModelID:      modelID,
		Title:        "New AI Chat",
		SystemPrompt: beeperAgent.SystemPrompt,
	})
	if err != nil {
		oc.log.Err(err).Msg("Failed to create default portal")
		return err
	}

	// Set agent-specific metadata
	pm := portalMeta(portal)
	pm.AgentID = beeperAgent.ID
	pm.DefaultAgentID = beeperAgent.ID
	if beeperAgent.SystemPrompt != "" {
		pm.SystemPrompt = beeperAgent.SystemPrompt
	}

	// Update the OtherUserID to be the agent+model ghost
	// This allows different model variants to have different ghosts
	agentGhostID := agentModelUserID(beeperAgent.ID, modelID)
	portal.OtherUserID = agentGhostID

	if err := portal.Save(ctx); err != nil {
		oc.log.Err(err).Msg("Failed to save portal with agent config")
		return err
	}

	// Get display name for agent+model combination
	agentDisplayName := oc.agentModelDisplayName(beeperAgent.Name, modelID)

	// Update chat info members to use agent ghost
	members := chatInfo.Members
	if members == nil {
		members = &bridgev2.ChatMemberList{}
	}
	if members.MemberMap == nil {
		members.MemberMap = make(bridgev2.ChatMemberMap)
	}
	humanID := humanUserID(oc.UserLogin.ID)
	humanMember := members.MemberMap[humanID]
	humanMember.EventSender = bridgev2.EventSender{Sender: humanID}
	members.MemberMap[humanID] = humanMember
	agentMember := members.MemberMap[agentGhostID]
	agentMember.EventSender = bridgev2.EventSender{Sender: agentGhostID}
	agentMember.UserInfo = &bridgev2.UserInfo{
		Name:  ptr.Ptr(agentDisplayName),
		IsBot: ptr.Ptr(true),
	}
	members.MemberMap[agentGhostID] = agentMember
	chatInfo.Members = members

	loginMeta.DefaultChatPortalID = string(portal.PortalKey.ID)
	if err := oc.UserLogin.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to persist default chat portal ID")
	}
	err = portal.CreateMatrixRoom(ctx, oc.UserLogin, chatInfo)
	if err != nil {
		oc.log.Err(err).Msg("Failed to create Matrix room for default chat")
		return err
	}
	oc.sendWelcomeMessage(ctx, portal)
	oc.log.Info().Stringer("portal", portal.PortalKey).Msg("New AI Chat room created")
	return nil
}

func (oc *AIClient) listAllChatPortals(ctx context.Context) ([]*bridgev2.Portal, error) {
	// Query all portals and filter by receiver (our login ID)
	// This works because all our portals have Receiver set to our UserLogin.ID
	allDBPortals, err := oc.UserLogin.Bridge.DB.Portal.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	portals := make([]*bridgev2.Portal, 0)
	for _, dbPortal := range allDBPortals {
		// Filter to only portals owned by this user login
		if dbPortal.Receiver != oc.UserLogin.ID {
			continue
		}
		portal, err := oc.UserLogin.Bridge.GetPortalByKey(ctx, dbPortal.PortalKey)
		if err != nil {
			return nil, err
		}
		if portal != nil {
			portals = append(portals, portal)
		}
	}
	return portals, nil
}

// HandleMatrixMessageRemove handles message deletions from Matrix
// For AI bridge, we just delete from our database - there's no "remote" to sync to
func (oc *AIClient) HandleMatrixMessageRemove(ctx context.Context, msg *bridgev2.MatrixMessageRemove) error {
	oc.log.Debug().
		Stringer("event_id", msg.TargetMessage.MXID).
		Stringer("portal", msg.Portal.PortalKey).
		Msg("Handling message deletion")

	// Delete from our database - the Matrix side is already handled by the bridge framework
	if err := oc.UserLogin.Bridge.DB.Message.Delete(ctx, msg.TargetMessage.RowID); err != nil {
		oc.log.Warn().Err(err).Stringer("event_id", msg.TargetMessage.MXID).Msg("Failed to delete message from database")
		return err
	}

	return nil
}

// HandleMatrixDisappearingTimer handles disappearing message timer changes from Matrix
// For AI bridge, we just update the portal's disappear field - the bridge framework handles the actual deletion
func (oc *AIClient) HandleMatrixDisappearingTimer(ctx context.Context, msg *bridgev2.MatrixDisappearingTimer) (bool, error) {
	oc.log.Debug().
		Stringer("portal", msg.Portal.PortalKey).
		Str("type", string(msg.Content.Type)).
		Dur("timer", msg.Content.Timer.Duration).
		Msg("Handling disappearing timer change")

	// Convert event to database setting and update portal
	setting := database.DisappearingSettingFromEvent(msg.Content)
	changed := msg.Portal.UpdateDisappearingSetting(ctx, setting, bridgev2.UpdateDisappearingSettingOpts{
		Save: true,
	})

	return changed, nil
}
