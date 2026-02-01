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

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/event"
)

// Tool name constants
const (
	ToolNameCalculator        = "calculator"
	ToolNameWebSearch         = "web_search"
	ToolNameOnline            = "online"
	ToolNameWebSearchProvider = "web_search_provider"
	ToolNameCodeInterpreter   = "code_interpreter"
)

// getDefaultToolsConfig returns the default tools configuration for a new room
// using MCP SDK types. OpenRouter/Beeper use :online plugin by default,
// other providers use builtin web search.
func getDefaultToolsConfig(provider string) ToolsConfig {
	isOpenRouter := provider == ProviderOpenRouter || provider == ProviderBeeper

	config := ToolsConfig{
		Tools: make(map[string]*ToolEntry),
	}

	// Register builtin tools
	registerTool(&config, mcp.Tool{
		Name:        ToolNameCalculator,
		Description: "Perform arithmetic calculations",
		Annotations: &mcp.ToolAnnotations{Title: "Calculator"},
	}, "builtin")

	registerTool(&config, mcp.Tool{
		Name:        ToolNameWebSearch,
		Description: "Search the web using DuckDuckGo",
		Annotations: &mcp.ToolAnnotations{Title: "Web Search"},
	}, "builtin")

	// OpenRouter :online plugin
	if isOpenRouter {
		registerTool(&config, mcp.Tool{
			Name:        ToolNameOnline,
			Description: "Real-time web search via OpenRouter",
			Annotations: &mcp.ToolAnnotations{Title: "Online Search"},
		}, "plugin")
		// Enable online by default, disable builtin web_search
		config.Tools[ToolNameOnline].Enabled = ptr.Ptr(true)
		config.Tools[ToolNameWebSearch].Enabled = ptr.Ptr(false)
	}

	// Provider tools (available on all providers, provider decides actual support)
	registerTool(&config, mcp.Tool{
		Name:        ToolNameWebSearchProvider,
		Description: "Native provider web search API",
		Annotations: &mcp.ToolAnnotations{Title: "Provider Web Search"},
	}, "provider")

	registerTool(&config, mcp.Tool{
		Name:        ToolNameCodeInterpreter,
		Description: "Execute Python code in sandbox",
		Annotations: &mcp.ToolAnnotations{Title: "Code Interpreter"},
	}, "provider")

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
	if len(meta.ToolsConfig.Tools) > 0 {
		return false
	}
	meta.ToolsConfig = getDefaultToolsConfig(provider)
	return true
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

	var tools []ToolInfo

	for name, entry := range meta.ToolsConfig.Tools {
		if entry == nil {
			continue
		}

		// Skip online plugin for non-OpenRouter providers
		if name == ToolNameOnline && !isOpenRouter {
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

		tools = append(tools, ToolInfo{
			Name:        name,
			DisplayName: displayName,
			Description: entry.Tool.Description,
			Type:        entry.Type,
			Enabled:     oc.isToolEnabled(meta, name),
			Available:   available,
		})
	}

	return tools
}

// isToolEnabled checks if a specific tool is enabled based on ToolsConfig
func (oc *AIClient) isToolEnabled(meta *PortalMetadata, toolName string) bool {
	// Look up in tools map
	if entry, ok := meta.ToolsConfig.Tools[toolName]; ok && entry != nil {
		if entry.Enabled != nil {
			return *entry.Enabled
		}
	}

	// Use defaults based on tool type and context
	return oc.getDefaultToolState(meta, toolName)
}

// getDefaultToolState returns the default enabled state for a tool
func (oc *AIClient) getDefaultToolState(meta *PortalMetadata, toolName string) bool {
	loginMeta := loginMetadata(oc.UserLogin)
	provider := loginMeta.Provider
	isOpenRouter := provider == ProviderOpenRouter || provider == ProviderBeeper

	switch toolName {
	case ToolNameCalculator:
		// Calculator enabled by default if model supports tools
		return meta.Capabilities.SupportsToolCalling
	case ToolNameWebSearch:
		// Web search disabled by default if online plugin is enabled
		if isOpenRouter {
			if entry, ok := meta.ToolsConfig.Tools[ToolNameOnline]; ok && entry != nil && entry.Enabled != nil && *entry.Enabled {
				return false
			}
		}
		return meta.Capabilities.SupportsToolCalling
	case ToolNameOnline:
		// Online plugin enabled by default for OpenRouter/Beeper
		return isOpenRouter
	case ToolNameWebSearchProvider, ToolNameCodeInterpreter:
		// Provider tools disabled by default
		return false
	default:
		// Unknown tools disabled by default
		return false
	}
}

// applyToolToggle applies a tool toggle from client
func (oc *AIClient) applyToolToggle(meta *PortalMetadata, toggle ToolToggle, provider string) {
	// Ensure tools config is initialized
	ensureToolsConfig(meta, provider)

	isOpenRouter := provider == ProviderOpenRouter || provider == ProviderBeeper

	// Normalize tool name aliases
	toolName := normalizeToolName(toggle.Name)

	// Check if tool exists
	entry, ok := meta.ToolsConfig.Tools[toolName]
	if !ok || entry == nil {
		oc.log.Warn().Str("tool", toggle.Name).Msg("Unknown tool in toggle request")
		return
	}

	// Validate online plugin for non-OpenRouter
	if toolName == ToolNameOnline && !isOpenRouter {
		oc.log.Warn().Str("tool", toolName).Msg("Online plugin only available with OpenRouter")
		return
	}

	// Apply toggle
	entry.Enabled = &toggle.Enabled

	// If enabling online, disable builtin web_search to avoid duplication
	if toolName == ToolNameOnline && toggle.Enabled {
		if wsEntry, ok := meta.ToolsConfig.Tools[ToolNameWebSearch]; ok && wsEntry != nil {
			wsEntry.Enabled = ptr.Ptr(false)
		}
	}

	oc.log.Info().Str("tool", toolName).Bool("enabled", toggle.Enabled).Msg("Applied tool toggle from client")
}

// normalizeToolName converts common aliases to canonical tool names
func normalizeToolName(name string) string {
	switch name {
	case "calc":
		return ToolNameCalculator
	case "websearch", "search":
		return ToolNameWebSearch
	case "websearchprovider", "provider_search":
		return ToolNameWebSearchProvider
	case "codeinterpreter", "interpreter":
		return ToolNameCodeInterpreter
	default:
		return name
	}
}

// SearchUsers searches available AI models by name/ID
func (oc *AIClient) SearchUsers(ctx context.Context, query string) ([]*bridgev2.ResolveIdentifierResponse, error) {
	oc.log.Debug().Str("query", query).Msg("User search requested")

	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, nil
	}

	// Fetch available models
	models, err := oc.listAvailableModels(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	// Filter models by query (match ID or display name)
	var results []*bridgev2.ResolveIdentifierResponse
	for i := range models {
		model := &models[i]
		displayName := FormatModelDisplay(model.ID)

		// Check if query matches model ID or display name (case-insensitive)
		if !strings.Contains(strings.ToLower(model.ID), query) &&
			!strings.Contains(strings.ToLower(displayName), query) &&
			!strings.Contains(strings.ToLower(model.Name), query) {
			continue
		}

		userID := modelUserID(model.ID)
		ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, userID)
		if err != nil {
			oc.log.Warn().Err(err).Str("model", model.ID).Msg("Failed to get ghost for search result")
			continue
		}

		results = append(results, &bridgev2.ResolveIdentifierResponse{
			UserID: userID,
			UserInfo: &bridgev2.UserInfo{
				Name:        ptr.Ptr(displayName),
				IsBot:       ptr.Ptr(false),
				Identifiers: []string{model.ID},
			},
			Ghost: ghost,
		})
	}

	oc.log.Info().Str("query", query).Int("results", len(results)).Msg("Search completed")
	return results, nil
}

// GetContactList returns a list of available AI models as contacts
func (oc *AIClient) GetContactList(ctx context.Context) ([]*bridgev2.ResolveIdentifierResponse, error) {
	oc.log.Debug().Msg("Contact list requested")

	// Fetch available models (use cache if available)
	models, err := oc.listAvailableModels(ctx, false)
	if err != nil {
		oc.log.Error().Err(err).Msg("Failed to list models, using fallback")
		// Return default model as fallback based on provider
		meta := loginMetadata(oc.UserLogin)
		fallbackID := DefaultModelForProvider(meta.Provider)
		models = []ModelInfo{{
			ID:       fallbackID,
			Name:     FormatModelDisplay(fallbackID),
			Provider: meta.Provider,
		}}
	}

	// Create a contact for each model
	contacts := make([]*bridgev2.ResolveIdentifierResponse, 0, len(models))

	for i := range models {
		model := &models[i]
		// Get or create ghost for this model
		userID := modelUserID(model.ID)
		ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, userID)
		if err != nil {
			oc.log.Warn().Err(err).Str("model", model.ID).Msg("Failed to get ghost for model")
			continue
		}

		contacts = append(contacts, &bridgev2.ResolveIdentifierResponse{
			UserID: userID,
			UserInfo: &bridgev2.UserInfo{
				Name:        ptr.Ptr(FormatModelDisplay(model.ID)),
				IsBot:       ptr.Ptr(false),
				Identifiers: []string{model.ID},
			},
			Ghost: ghost,
		})
	}

	oc.log.Info().Int("count", len(contacts)).Msg("Returning model contact list")
	return contacts, nil
}

// ResolveIdentifier resolves a model ID to a ghost and optionally creates a chat
func (oc *AIClient) ResolveIdentifier(ctx context.Context, identifier string, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	// Identifier is the model ID (e.g., "gpt-4o", "gpt-4-turbo")
	modelID := strings.TrimSpace(identifier)
	if modelID == "" {
		return nil, fmt.Errorf("model identifier is required")
	}

	// Validate model exists (check cache first)
	models, _ := oc.listAvailableModels(ctx, false)
	var modelInfo *ModelInfo
	for i := range models {
		if models[i].ID == modelID {
			modelInfo = &models[i]
			break
		}
	}

	if modelInfo == nil {
		// Model not in cache, assume it's valid (user might have access to beta models)
		oc.log.Warn().Str("model", modelID).Msg("Model not in cache, assuming valid")
	}

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

	return &bridgev2.ResolveIdentifierResponse{
		UserID: userID,
		UserInfo: &bridgev2.UserInfo{
			Name:        ptr.Ptr(FormatModelDisplay(modelID)),
			IsBot:       ptr.Ptr(false),
			Identifiers: []string{modelID},
		},
		Ghost: ghost,
		Chat:  chatResp,
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
		title = fmt.Sprintf("AI Chat with %s", FormatModelDisplay(modelID))
	}

	portalKey := portalKeyForChat(oc.UserLogin.ID)
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
			EmitThinking:        opts.CopyFrom.EmitThinking,
			EmitToolArgs:        opts.CopyFrom.EmitToolArgs,
			DefaultAgentID:      opts.CopyFrom.DefaultAgentID,
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
	portal.Topic = pmeta.SystemPrompt
	portal.TopicSet = pmeta.SystemPrompt != ""

	if err := portal.Save(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to save portal: %w", err)
	}

	chatInfo := oc.composeChatInfo(title, pmeta.SystemPrompt, modelID)
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

	// Send confirmation with link
	roomLink := fmt.Sprintf("https://matrix.to/#/%s", newPortal.MXID)
	oc.sendSystemNotice(runCtx, portal, fmt.Sprintf(
		"Created new %s chat.\nOpen: %s",
		FormatModelDisplay(modelID), roomLink,
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
			title = FormatModelDisplay(modelID)
		}
	}
	return oc.composeChatInfo(title, meta.SystemPrompt, modelID)
}

// composeChatInfo creates a ChatInfo struct for a chat
func (oc *AIClient) composeChatInfo(title, prompt, modelID string) *bridgev2.ChatInfo {
	if modelID == "" {
		modelID = oc.effectiveModel(nil)
	}
	modelName := FormatModelDisplay(modelID)
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
				Name:  ptr.Ptr(modelName),
				IsBot: ptr.Ptr(false),
			},
			// Set displayname directly in membership event content
			// This works because MemberEventContent.Displayname has omitempty
			MemberEventExtra: map[string]any{
				"displayname": modelName,
			},
		},
	}
	return &bridgev2.ChatInfo{
		Name:  ptr.Ptr(title),
		Topic: ptrIfNotEmpty(prompt),
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
	// Boolean fields - only apply when explicitly set (non-nil)
	if config.EmitThinking != nil {
		meta.EmitThinking = *config.EmitThinking
	}
	if config.EmitToolArgs != nil {
		meta.EmitToolArgs = *config.EmitToolArgs
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
func (oc *AIClient) handleModelSwitch(ctx context.Context, portal *bridgev2.Portal, oldModel, newModel string) {
	if oldModel == newModel || oldModel == "" || newModel == "" {
		return
	}

	oc.log.Info().
		Str("old_model", oldModel).
		Str("new_model", newModel).
		Stringer("portal", portal.PortalKey).
		Msg("Handling model switch")

	oldModelName := FormatModelDisplay(oldModel)
	newModelName := FormatModelDisplay(newModel)

	// Pre-update the new model ghost's profile before queueing the event
	// This ensures the ghost has a display name set in its Matrix profile
	newGhost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, modelUserID(newModel))
	if err != nil {
		oc.log.Warn().Err(err).Str("model", newModel).Msg("Failed to get ghost for model switch")
	} else {
		newGhost.UpdateInfo(ctx, &bridgev2.UserInfo{
			Name:  ptr.Ptr(newModelName),
			IsBot: ptr.Ptr(false),
		})
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
					Name:  ptr.Ptr(newModelName),
					IsBot: ptr.Ptr(false),
				},
				MemberEventExtra: map[string]any{
					"displayname": newModelName,
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

	// Update bridge info to resend room features state event with new capabilities
	// This ensures the client knows what features the new model supports (vision, audio, etc.)
	portal.UpdateBridgeInfo(ctx)
}

// BroadcastRoomState sends current room capabilities and settings to Matrix room state
func (oc *AIClient) BroadcastRoomState(ctx context.Context, portal *bridgev2.Portal) error {
	if err := oc.broadcastCapabilities(ctx, portal); err != nil {
		return err
	}
	return oc.broadcastSettings(ctx, portal)
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
		EmitThinking:        ptr.Ptr(meta.EmitThinking),
		EmitToolArgs:        ptr.Ptr(meta.EmitToolArgs),
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
	if err := oc.ensureDefaultChat(logCtx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to ensure default chat")
		return
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
	portals, err := oc.listAllChatPortals(ctx)
	if err != nil {
		oc.log.Err(err).Msg("Failed to list chat portals")
		return err
	}
	for _, portal := range portals {
		if portal.MXID != "" {
			oc.log.Debug().Stringer("portal", portal.PortalKey).Msg("Existing chat already has MXID")
			return nil
		}
	}
	if len(portals) > 0 {
		info := oc.chatInfoFromPortal(portals[0])
		oc.log.Info().Stringer("portal", portals[0].PortalKey).Msg("Existing portal missing MXID; creating Matrix room")
		err := portals[0].CreateMatrixRoom(ctx, oc.UserLogin, info)
		if err != nil {
			oc.log.Err(err).Msg("Failed to create Matrix room for existing portal")
		}
		oc.sendWelcomeMessage(ctx, portals[0])
		return err
	}
	resp, err := oc.createChat(ctx, "Welcome to AI Chats", "") // Default room title
	if err != nil {
		oc.log.Err(err).Msg("Failed to create default portal")
		return err
	}
	err = resp.Portal.CreateMatrixRoom(ctx, oc.UserLogin, resp.PortalInfo)
	if err != nil {
		oc.log.Err(err).Msg("Failed to create Matrix room for default chat")
		return err
	}
	oc.sendWelcomeMessage(ctx, resp.Portal)
	oc.log.Info().Stringer("portal", resp.PortalKey).Msg("Default AI chat room created")
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

func (oc *AIClient) createChat(ctx context.Context, title, systemPrompt string) (*bridgev2.CreateChatResponse, error) {
	portal, info, err := oc.spawnPortal(ctx, title, systemPrompt)
	if err != nil {
		return nil, err
	}
	return &bridgev2.CreateChatResponse{
		PortalKey:  portal.PortalKey,
		Portal:     portal,
		PortalInfo: info,
	}, nil
}

func (oc *AIClient) spawnPortal(ctx context.Context, title, systemPrompt string) (*bridgev2.Portal, *bridgev2.ChatInfo, error) {
	oc.log.Debug().Str("title", title).Msg("Allocating portal for new chat")
	return oc.initPortalForChat(ctx, PortalInitOpts{
		Title:        title,
		SystemPrompt: systemPrompt,
	})
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
