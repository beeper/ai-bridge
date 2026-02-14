//lint:file-ignore U1000 Hard-cut compatibility: pending full dead-code deletion.
package connector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"

	"github.com/beeper/ai-bridge/pkg/aimodels"
	"github.com/beeper/ai-bridge/pkg/shared/toolspec"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Tool name constants
const (
	ToolNameCalculator = toolspec.CalculatorName
	ToolNameWebSearch  = toolspec.WebSearchName
)

// defaultRawModeSystemPrompt is the default system prompt for model-only (raw) rooms.
const defaultRawModeSystemPrompt = "You are a helpful assistant."

func hasAssignedAgent(meta *PortalMetadata) bool {
	if meta == nil {
		return false
	}
	return meta.AgentID != ""
}

func (oc *AIClient) isSimpleProfile() bool {
	if oc == nil || oc.connector == nil {
		return false
	}
	return strings.TrimSpace(oc.connector.bridgePolicy().NetworkID) == "ai-simple"
}

func modelMatchesQuery(model *ModelInfo, query string) bool {
	if model == nil || query == "" {
		return false
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return false
	}
	if strings.Contains(strings.ToLower(model.ID), q) ||
		strings.Contains(strings.ToLower(model.Name), q) ||
		strings.Contains(strings.ToLower(model.Description), q) ||
		strings.Contains(strings.ToLower(model.Provider), q) {
		return true
	}
	for _, ident := range aimodels.ModelContactIdentifiers(model.ID, model) {
		if strings.Contains(strings.ToLower(ident), q) {
			return true
		}
	}
	return false
}

func (oc *AIClient) modelContacts(ctx context.Context, query string) ([]*bridgev2.ResolveIdentifierResponse, error) {
	models, err := oc.listAvailableModels(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("failed to load models: %w", err)
	}

	query = strings.ToLower(strings.TrimSpace(query))
	contacts := make([]*bridgev2.ResolveIdentifierResponse, 0, len(models))
	for i := range models {
		model := &models[i]
		if model.ID == "" {
			continue
		}
		if query != "" && !modelMatchesQuery(model, query) {
			continue
		}

		userID := modelUserID(model.ID)
		ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, userID)
		if err != nil {
			oc.loggerForContext(ctx).Warn().Err(err).Str("model", model.ID).Msg("Failed to get ghost for model")
			continue
		}
		oc.ensureGhostDisplayNameWithGhost(ctx, ghost, model.ID, model)

		contacts = append(contacts, &bridgev2.ResolveIdentifierResponse{
			UserID: userID,
			UserInfo: &bridgev2.UserInfo{
				Name:        ptr.Ptr(aimodels.ModelContactName(model.ID, model)),
				IsBot:       ptr.Ptr(false),
				Identifiers: aimodels.ModelContactIdentifiers(model.ID, model),
			},
			Ghost: ghost,
		})
	}
	return contacts, nil
}

// buildAvailableTools returns a list of ToolInfo for all tools based on tool policy.
func (oc *AIClient) buildAvailableTools(meta *PortalMetadata) []ToolInfo {
	names := oc.agentResolver.ToolNamesForPortal(meta)
	var toolsList []ToolInfo
	builtinByName := map[string]ToolDefinition{}
	for _, tool := range BuiltinTools() {
		builtinByName[tool.Name] = tool
	}

	for _, name := range names {
		displayName := name
		description := ""
		toolType := "builtin"
		if tool, ok := builtinByName[name]; ok {
			description = tool.Description
		}
		description = oc.toolDescriptionForPortal(meta, name, description)

		available, source, reason := oc.agentResolver.IsToolAvailable(meta, name)
		allowed := oc.agentResolver.IsToolAllowedByPolicy(meta, name)
		enabled := available && allowed

		if !allowed {
			source = SourceAgentPolicy
			if reason == "" {
				reason = "Disabled by tool policy"
			}
		}

		toolsList = append(toolsList, ToolInfo{
			Name:        name,
			DisplayName: displayName,
			Description: description,
			Type:        toolType,
			Enabled:     enabled,
			Available:   available,
			Source:      source,
			Reason:      reason,
		})
	}

	return toolsList
}

func (oc *AIClient) canUseImageGeneration() bool {
	if oc == nil || oc.UserLogin == nil || oc.UserLogin.Metadata == nil {
		return false
	}
	loginMeta := loginMetadata(oc.UserLogin)
	if loginMeta == nil || loginMeta.APIKey == "" {
		return false
	}
	switch loginMeta.Provider {
	case ProviderOpenAI, ProviderOpenRouter, ProviderBeeper, ProviderMagicProxy:
		return true
	default:
		return false
	}
}

// SearchUsers returns model contacts that match the query.
func (oc *AIClient) SearchUsers(ctx context.Context, query string) ([]*bridgev2.ResolveIdentifierResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	return oc.modelContacts(ctx, query)
}

// GetContactList returns model contacts only.
func (oc *AIClient) GetContactList(ctx context.Context) ([]*bridgev2.ResolveIdentifierResponse, error) {
	return oc.modelContacts(ctx, "")
}

// ResolveIdentifier resolves a model identifier to a ghost and optionally creates a chat.
func (oc *AIClient) ResolveIdentifier(ctx context.Context, identifier string, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	id := strings.TrimSpace(identifier)
	if id == "" {
		return nil, errors.New("identifier is required")
	}

	if modelID := parseModelFromGhostID(id); modelID != "" {
		return oc.resolveModelIdentifier(ctx, modelID, createChat)
	}
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
func (oc *AIClient) resolveAgentIdentifier(ctx context.Context, agent *AgentDefinition, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	return oc.resolveAgentIdentifierWithModel(ctx, agent, "", createChat)
}

func (oc *AIClient) resolveAgentIdentifierWithModel(ctx context.Context, agent *AgentDefinition, modelID string, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	explicitModel := modelID != ""
	if modelID == "" {
		modelID = oc.agentResolver.AgentDefaultModel(agent)
	}
	userID := agentUserID(agent.ID)
	ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ghost: %w", err)
	}

	agentName := oc.agentResolver.ResolveAgentDisplayName(ctx, agent)
	displayName := agentName
	oc.ensureAgentGhostDisplayName(ctx, agent.ID, modelID, agentName)

	var chatResp *bridgev2.CreateChatResponse
	if createChat {
		oc.loggerForContext(ctx).Info().Str("agent", agent.ID).Msg("Creating new chat for agent")
		chatResp, err = oc.createAgentChatWithModel(ctx, agent, modelID, explicitModel)
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
		oc.loggerForContext(ctx).Info().Str("model", modelID).Msg("Creating new chat for model")
		chatResp, err = oc.createNewChat(ctx, modelID)
		if err != nil {
			return nil, fmt.Errorf("failed to create chat: %w", err)
		}
	}

	info := oc.findModelInfo(modelID)
	return &bridgev2.ResolveIdentifierResponse{
		UserID: userID,
		UserInfo: &bridgev2.UserInfo{
			Name:        ptr.Ptr(aimodels.ModelContactName(modelID, info)),
			IsBot:       ptr.Ptr(false),
			Identifiers: aimodels.ModelContactIdentifiers(modelID, info),
		},
		Ghost: ghost,
		Chat:  chatResp,
	}, nil
}

// createAgentChat creates a new chat room for an agent
func (oc *AIClient) createAgentChat(ctx context.Context, agent *AgentDefinition) (*bridgev2.CreateChatResponse, error) {
	return oc.createAgentChatWithModel(ctx, agent, "", false)
}

func (oc *AIClient) createAgentChatWithModel(ctx context.Context, agent *AgentDefinition, modelID string, applyModelOverride bool) (*bridgev2.CreateChatResponse, error) {
	if modelID == "" {
		modelID = oc.agentResolver.AgentDefaultModel(agent)
	}

	agentName := oc.agentResolver.ResolveAgentDisplayName(ctx, agent)
	portal, chatInfo, err := oc.initPortalForChat(ctx, PortalInitOpts{
		ModelID:      modelID,
		Title:        fmt.Sprintf("Chat with %s", agentName),
		SystemPrompt: agent.SystemPrompt,
	})
	if err != nil {
		return nil, err
	}

	// Set agent-specific metadata
	pm := portalMeta(portal)
	pm.AgentID = agent.ID
	if agent.SystemPrompt != "" {
		pm.SystemPrompt = agent.SystemPrompt
	}
	if agent.ReasoningEffort != "" {
		pm.ReasoningEffort = agent.ReasoningEffort
	}
	if !applyModelOverride {
		pm.Model = ""
	}

	agentGhostID := agentUserID(agent.ID)

	// Update the OtherUserID to be the agent ghost
	portal.OtherUserID = agentGhostID
	agentAvatar := strings.TrimSpace(agent.AvatarURL)
	if agentAvatar == "" {
		agentAvatar = ""
	}
	if agentAvatar != "" {
		portal.AvatarID = networkid.AvatarID(agentAvatar)
		portal.AvatarMXC = id.ContentURIString(agentAvatar)
	}

	if err := portal.Save(ctx); err != nil {
		return nil, fmt.Errorf("failed to save portal with agent config: %w", err)
	}

	// Update chat info members to use agent ghost only
	oc.applyAgentChatInfo(chatInfo, agent.ID, agentName, modelID)

	// Rooms created via provisioning (ResolveIdentifier/CreateDM) won't go through our explicit
	// post-CreateMatrixRoom call sites. Schedule the welcome notice + auto-greeting for when the
	// Matrix room ID becomes available.
	oc.scheduleWelcomeMessage(ctx, portal.PortalKey)

	return &bridgev2.CreateChatResponse{
		PortalKey: portal.PortalKey,
		// Return the full ChatInfo so bridgev2 can apply ExtraUpdates (initial room state,
		// welcome notice, etc.) when creating the Matrix room via provisioning (CreateDM).
		PortalInfo: chatInfo,
	}, nil
}

// createNewChat creates a new portal for a specific model
func (oc *AIClient) createNewChat(ctx context.Context, modelID string) (*bridgev2.CreateChatResponse, error) {
	portal, chatInfo, err := oc.initPortalForChat(ctx, PortalInitOpts{
		ModelID:      modelID,
		SystemPrompt: defaultRawModeSystemPrompt,
	})
	if err != nil {
		return nil, err
	}

	// Keep model-only chats consistent with "!ai new <model>": raw/non-agentic.
	meta := portalMeta(portal)
	if meta != nil && !meta.IsRawMode {
		meta.IsRawMode = true
		if err := portal.Save(ctx); err != nil {
			return nil, fmt.Errorf("failed to save portal raw mode: %w", err)
		}
	}

	// Rooms created via provisioning (ResolveIdentifier/CreateDM) won't go through our explicit
	// post-CreateMatrixRoom call sites. Schedule the welcome notice for when the Matrix room exists.
	oc.scheduleWelcomeMessage(ctx, portal.PortalKey)

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
		modelName := aimodels.ModelContactName(modelID, oc.findModelInfo(modelID))
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
			ConversationMode:    opts.CopyFrom.ConversationMode,
			AgentID:             opts.CopyFrom.AgentID,
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
		}
	}
	portal.Metadata = pmeta

	portal.RoomType = database.RoomTypeDM
	portal.OtherUserID = modelUserID(modelID)
	portal.Name = title
	portal.NameSet = true
	defaultAvatar := ""
	if defaultAvatar != "" {
		portal.AvatarID = networkid.AvatarID(defaultAvatar)
		portal.AvatarMXC = id.ContentURIString(defaultAvatar)
	}
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
		oc.sendSystemNotice(runCtx, portal, "Couldn't load messages: "+err.Error())
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
			oc.sendSystemNotice(runCtx, portal, fmt.Sprintf("Couldn't find event: %s", arg))
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
		oc.sendSystemNotice(runCtx, portal, "Couldn't create the forked chat: "+err.Error())
		return
	}

	// 4. Create Matrix room
	if err := newPortal.CreateMatrixRoom(runCtx, oc.UserLogin, chatInfo); err != nil {
		oc.sendSystemNotice(runCtx, portal, "Couldn't create the room: "+err.Error())
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

// handleNewChat creates a new chat using the current room's agent/model,
// or an explicitly provided agent/model.
func (oc *AIClient) handleNewChat(
	ctx context.Context,
	_ *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	args []string,
) {
	runCtx := oc.backgroundContext(ctx)

	usage := "Usage: !ai new [model]"

	if len(args) > 0 {
		cmd := strings.ToLower(args[0])
		if cmd == "agent" {
			oc.sendSystemNotice(runCtx, portal, "Agent mode is not available in the simple bridge. Use `!ai new [model]`.")
			return
		}

		if len(args) != 1 {
			oc.sendSystemNotice(runCtx, portal, usage)
			return
		}
		modelID := strings.TrimSpace(args[0])
		resolvedModelID, valid, err := oc.resolveModelID(runCtx, modelID)
		if err != nil {
			oc.sendSystemNotice(runCtx, portal, err.Error())
			return
		}
		if valid && resolvedModelID != "" {
			modelID = resolvedModelID
		}
		if ok, _ := oc.validateModel(runCtx, modelID); !ok {
			oc.sendSystemNotice(runCtx, portal, fmt.Sprintf("That model isn't available: %s", modelID))
			return
		}
		oc.createAndOpenModelChat(runCtx, portal, modelID)
		return
	}

	// No args: create new room of same type
	if meta == nil {
		oc.sendSystemNotice(runCtx, portal, "Couldn't read current room settings.")
		return
	}
	modelID := oc.effectiveModel(meta)
	if modelID == "" {
		oc.sendSystemNotice(runCtx, portal, "No model configured for this room.")
		return
	}
	if ok, _ := oc.validateModel(runCtx, modelID); !ok {
		oc.sendSystemNotice(runCtx, portal, fmt.Sprintf("That model isn't available: %s", modelID))
		return
	}
	oc.createAndOpenModelChat(runCtx, portal, modelID)
}

func (oc *AIClient) resolveAgentModelForNewChat(ctx context.Context, agent *AgentDefinition, preferredModel string) (string, error) {
	if preferredModel != "" {
		if ok, _ := oc.validateModel(ctx, preferredModel); ok {
			return preferredModel, nil
		}
	}

	if agent != nil {
		defaultModel := oc.agentResolver.AgentDefaultModel(agent)
		if ok, _ := oc.validateModel(ctx, defaultModel); ok {
			return defaultModel, nil
		}
	}

	fallback := oc.effectiveModel(nil)
	if fallback != "" {
		if ok, _ := oc.validateModel(ctx, fallback); ok {
			return fallback, nil
		}
	}

	if preferredModel != "" {
		return "", fmt.Errorf("that model isn't available: %s", preferredModel)
	}
	return "", errors.New("no available model")
}

func (oc *AIClient) createAndOpenAgentChat(ctx context.Context, portal *bridgev2.Portal, agent *AgentDefinition, modelID string, modelOverride bool) {
	agentName := oc.agentResolver.ResolveAgentDisplayName(ctx, agent)
	chatResp, err := oc.createAgentChatWithModel(ctx, agent, modelID, modelOverride)
	if err != nil {
		oc.sendSystemNotice(ctx, portal, "Couldn't create the chat: "+err.Error())
		return
	}

	newPortal, err := oc.UserLogin.Bridge.GetPortalByKey(ctx, chatResp.PortalKey)
	if err != nil || newPortal == nil {
		msg := "Couldn't open the new chat."
		if err != nil {
			msg = "Couldn't open the new chat: " + err.Error()
		}
		oc.sendSystemNotice(ctx, portal, msg)
		return
	}

	chatInfo := chatResp.PortalInfo
	if err := newPortal.CreateMatrixRoom(ctx, oc.UserLogin, chatInfo); err != nil {
		oc.sendSystemNotice(ctx, portal, "Couldn't create the room: "+err.Error())
		return
	}

	oc.sendWelcomeMessage(ctx, newPortal)

	roomLink := fmt.Sprintf("https://matrix.to/#/%s", newPortal.MXID)
	oc.sendSystemNotice(ctx, portal, fmt.Sprintf(
		"New %s chat created.\nOpen: %s",
		agentName, roomLink,
	))
}

func (oc *AIClient) createAndOpenModelChat(ctx context.Context, portal *bridgev2.Portal, modelID string) {
	newPortal, chatInfo, err := oc.createNewChatWithModel(ctx, modelID)
	if err != nil {
		oc.sendSystemNotice(ctx, portal, "Couldn't create the chat: "+err.Error())
		return
	}

	if err := newPortal.CreateMatrixRoom(ctx, oc.UserLogin, chatInfo); err != nil {
		oc.sendSystemNotice(ctx, portal, "Couldn't create the room: "+err.Error())
		return
	}

	oc.sendWelcomeMessage(ctx, newPortal)

	roomLink := fmt.Sprintf("https://matrix.to/#/%s", newPortal.MXID)
	oc.sendSystemNotice(ctx, portal, fmt.Sprintf(
		"New %s chat created.\nOpen: %s",
		aimodels.ModelContactName(modelID, oc.findModelInfo(modelID)), roomLink,
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

	portal, chatInfo, err := oc.initPortalForChat(ctx, PortalInitOpts{
		Title:    title,
		CopyFrom: sourceMeta,
	})
	if err != nil {
		return nil, nil, err
	}

	agentID := sourceMeta.AgentID
	if agentID != "" {
		pm := portalMeta(portal)
		pm.AgentID = agentID

		modelID := oc.effectiveModel(pm)
		portal.OtherUserID = agentUserID(agentID)

		agentName := agentID
		agentAvatar := ""
		if agent, err := oc.agentResolver.GetAgent(ctx, agentID); err == nil && agent != nil {
			agentName = oc.agentResolver.ResolveAgentDisplayName(ctx, agent)
			agentAvatar = agent.AvatarURL
		}
		if agentAvatar != "" {
			portal.AvatarID = networkid.AvatarID(agentAvatar)
			portal.AvatarMXC = id.ContentURIString(agentAvatar)
		}
		oc.applyAgentChatInfo(chatInfo, agentID, agentName, modelID)

		if err := portal.Save(ctx); err != nil {
			return nil, nil, err
		}
	}

	return portal, chatInfo, nil
}

// copyMessagesToChat queues messages to be bridged to the new chat
// Returns the count of successfully queued messages
func (oc *AIClient) copyMessagesToChat(
	ctx context.Context,
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
		oc.loggerForContext(ctx).Warn().
			Int("copied", copiedCount).
			Int("skipped", skippedCount).
			Int("total", len(messages)).
			Msg("Partial fork - some messages were skipped due to missing metadata")
	}

	return copiedCount
}

// createNewChatWithModel creates a new chat portal with the specified model and default settings
func (oc *AIClient) createNewChatWithModel(ctx context.Context, modelID string) (*bridgev2.Portal, *bridgev2.ChatInfo, error) {
	portal, chatInfo, err := oc.initPortalForChat(ctx, PortalInitOpts{
		ModelID:      modelID,
		SystemPrompt: defaultRawModeSystemPrompt,
	})
	if err != nil {
		return nil, nil, err
	}

	// Model-only rooms are "raw"/non-agentic rooms. This disables directive processing
	// and prevents media-understanding unions from making the room appear more capable
	// than the base model.
	meta := portalMeta(portal)
	if meta != nil && !meta.IsRawMode {
		meta.IsRawMode = true
		if err := portal.Save(ctx); err != nil {
			return nil, nil, err
		}
	}

	return portal, chatInfo, nil
}

// chatInfoFromPortal builds ChatInfo from an existing portal
func (oc *AIClient) chatInfoFromPortal(ctx context.Context, portal *bridgev2.Portal) *bridgev2.ChatInfo {
	meta := portalMeta(portal)
	modelID := oc.effectiveModel(meta)
	title := meta.Title
	if title == "" {
		if portal.Name != "" {
			title = portal.Name
		} else {
			title = aimodels.ModelContactName(modelID, oc.findModelInfo(modelID))
		}
	}
	chatInfo := oc.composeChatInfo(title, modelID)

	agentID := resolveAgentID(meta)
	if agentID == "" {
		return chatInfo
	}

	agentName := agentID
	if ctx != nil {
		if agent, err := oc.agentResolver.GetAgent(ctx, agentID); err == nil && agent != nil {
			agentName = oc.agentResolver.ResolveAgentDisplayName(ctx, agent)
		}
	}

	oc.applyAgentChatInfo(chatInfo, agentID, agentName, modelID)
	return chatInfo
}

// composeChatInfo creates a ChatInfo struct for a chat
func (oc *AIClient) composeChatInfo(title, modelID string) *bridgev2.ChatInfo {
	if modelID == "" {
		modelID = oc.effectiveModel(nil)
	}
	modelInfo := oc.findModelInfo(modelID)
	modelName := aimodels.ModelContactName(modelID, modelInfo)
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
				Identifiers: aimodels.ModelContactIdentifiers(modelID, modelInfo),
			},
			// Set displayname directly in membership event content
			// This works because MemberEventContent.Displayname has omitempty
			MemberEventExtra: map[string]any{
				"displayname":            modelName,
				"com.beeper.ai.model_id": modelID,
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
	}
}

func (oc *AIClient) applyAgentChatInfo(chatInfo *bridgev2.ChatInfo, agentID, agentName, modelID string) {
	if chatInfo == nil || agentID == "" {
		return
	}
	if modelID == "" {
		modelID = oc.effectiveModel(nil)
	}

	agentGhostID := agentUserID(agentID)
	agentDisplayName := agentName

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
	modelInfo := oc.findModelInfo(modelID)
	agentMember.UserInfo = &bridgev2.UserInfo{
		Name:        ptr.Ptr(agentDisplayName),
		IsBot:       ptr.Ptr(true),
		Identifiers: aimodels.ModelContactIdentifiers(modelID, modelInfo),
	}
	agentMember.MemberEventExtra = map[string]any{
		"displayname":            agentDisplayName,
		"com.beeper.ai.model_id": modelID,
		"com.beeper.ai.agent":    agentID,
	}

	members.MemberMap = bridgev2.ChatMemberMap{
		humanID:      humanMember,
		agentGhostID: agentMember,
	}
	chatInfo.Members = members
}

// updatePortalConfig applies room settings to portal metadata with optimistic updates.
// If persistence fails, metadata is rolled back to the previous values.
func (oc *AIClient) updatePortalConfig(ctx context.Context, portal *bridgev2.Portal, config *RoomSettingsEventContent) error {
	meta := portalMeta(portal)
	before := clonePortalMetadata(meta)

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
	if config.AgentID != "" {
		meta.AgentID = config.AgentID
	}

	meta.LastRoomStateSync = time.Now().Unix()

	// Persist changes
	if err := portal.Save(ctx); err != nil {
		if before != nil {
			*meta = *before
		}
		oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to save portal after config update")
		return err
	}

	// Re-broadcast room state to confirm changes to all clients
	if err := oc.BroadcastRoomState(ctx, portal); err != nil {
		if before != nil {
			*meta = *before
			if saveErr := portal.Save(ctx); saveErr != nil {
				oc.loggerForContext(ctx).Warn().Err(saveErr).Msg("Failed to save rollback portal metadata after state broadcast failure")
			}
		}
		oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to re-broadcast room state after config update")
		return err
	}

	// Handle model switch - generate membership events if model changed.
	// This is done after persistence succeeds so optimistic updates can roll back safely.
	if config.Model != "" && oldModel != "" && config.Model != oldModel {
		oc.handleModelSwitch(ctx, portal, oldModel, config.Model)
	}

	return nil
}

// handleModelSwitch generates membership change events when switching models
// This creates leave/join events to show the model transition in the room timeline
// For agent rooms, it updates the agent ghost metadata.
func (oc *AIClient) handleModelSwitch(ctx context.Context, portal *bridgev2.Portal, oldModel, newModel string) {
	if oldModel == newModel || oldModel == "" || newModel == "" {
		return
	}

	meta := portalMeta(portal)
	agentID := resolveAgentID(meta)

	// Check if this is an agent room - update agent ghost metadata
	if agentID != "" {
		oc.handleAgentModelSwitch(ctx, portal, agentID, oldModel, newModel)
		return
	}

	// For non-agent rooms, use model-only ghosts
	oc.loggerForContext(ctx).Info().
		Str("old_model", oldModel).
		Str("new_model", newModel).
		Stringer("portal", portal.PortalKey).
		Msg("Handling model switch")

	oldInfo := oc.findModelInfo(oldModel)
	newInfo := oc.findModelInfo(newModel)
	oldModelName := aimodels.ModelContactName(oldModel, oldInfo)
	newModelName := aimodels.ModelContactName(newModel, newInfo)

	// Pre-update the new model ghost's profile before queueing the event
	// This ensures the ghost has a display name set in its Matrix profile
	newGhost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, modelUserID(newModel))
	if err != nil {
		oc.loggerForContext(ctx).Warn().Err(err).Str("model", newModel).Msg("Failed to get ghost for model switch")
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
					Identifiers: aimodels.ModelContactIdentifiers(newModel, newInfo),
				},
				MemberEventExtra: map[string]any{
					"displayname":            newModelName,
					"com.beeper.ai.model_id": newModel,
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
		oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to ensure single AI ghost after model switch")
	}
}

// handleAgentModelSwitch handles model switching for agent rooms.
// Keeps a single agent ghost and updates member metadata.
func (oc *AIClient) handleAgentModelSwitch(ctx context.Context, portal *bridgev2.Portal, agentID, oldModel, newModel string) {
	// Get the agent to determine display name
	agent, err := oc.agentResolver.GetAgent(ctx, agentID)
	if err != nil || agent == nil {
		oc.loggerForContext(ctx).Warn().Err(err).Str("agent", agentID).Msg("Agent not found for model switch")
		return
	}

	oc.loggerForContext(ctx).Info().
		Str("agent", agentID).
		Str("old_model", oldModel).
		Str("new_model", newModel).
		Stringer("portal", portal.PortalKey).
		Msg("Handling agent model switch")

	ghostID := agentUserID(agentID)
	agentName := oc.agentResolver.ResolveAgentDisplayName(ctx, agent)
	displayName := agentName
	oldModelName := aimodels.ModelContactName(oldModel, oc.findModelInfo(oldModel))
	newModelName := aimodels.ModelContactName(newModel, oc.findModelInfo(newModel))
	oldGhostID := portal.OtherUserID

	// Update member metadata for the agent ghost
	memberMap := bridgev2.ChatMemberMap{
		ghostID: {
			EventSender: bridgev2.EventSender{
				Sender:      ghostID,
				SenderLogin: oc.UserLogin.ID,
			},
			Membership: event.MembershipJoin,
			UserInfo: &bridgev2.UserInfo{
				Name:        ptr.Ptr(displayName),
				IsBot:       ptr.Ptr(true),
				Identifiers: aimodels.ModelContactIdentifiers(newModel, oc.findModelInfo(newModel)),
			},
			MemberEventExtra: map[string]any{
				"displayname":            displayName,
				"com.beeper.ai.model_id": newModel,
				"com.beeper.ai.agent":    agentID,
			},
		},
	}
	if oldGhostID != "" && oldGhostID != ghostID {
		memberMap[oldGhostID] = bridgev2.ChatMember{
			EventSender: bridgev2.EventSender{
				Sender:      oldGhostID,
				SenderLogin: oc.UserLogin.ID,
			},
			Membership:     event.MembershipLeave,
			PrevMembership: event.MembershipJoin,
		}
	}
	memberChanges := &bridgev2.ChatMemberList{MemberMap: memberMap}

	// Update portal's OtherUserID to agent ghost
	portal.OtherUserID = ghostID
	oc.ensureAgentGhostDisplayName(ctx, agentID, newModel, agentName)

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
	notice := fmt.Sprintf("Switched model from %s to %s", oldModelName, newModelName)
	oc.sendSystemNotice(ctx, portal, notice)

	// Update bridge info and capabilities
	portal.UpdateBridgeInfo(ctx)
	portal.UpdateCapabilities(ctx, oc.UserLogin, true)

	// Ensure only 1 AI ghost in room
	if err := oc.ensureSingleAIGhost(ctx, portal); err != nil {
		oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to ensure single AI ghost after agent model switch")
	}
}

// ensureSingleAIGhost ensures only 1 model/agent ghost is in the room at a time.
// Updates portal.OtherUserID if it doesn't match the expected ghost.
func (oc *AIClient) ensureSingleAIGhost(ctx context.Context, portal *bridgev2.Portal) error {
	meta := portalMeta(portal)

	// Determine which ghost SHOULD be in the room
	var expectedGhostID networkid.UserID
	agentID := resolveAgentID(meta)

	modelID := oc.effectiveModel(meta)
	if agentID != "" {
		expectedGhostID = agentUserID(agentID)
	} else {
		expectedGhostID = modelUserID(modelID)
	}

	// Update portal.OtherUserID if mismatched
	if portal.OtherUserID != expectedGhostID {
		oc.loggerForContext(ctx).Debug().
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
	return SettingExplanation{Value: nil, Source: SourceGlobalDefault, Reason: "provider/model default (unset)"}
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
		return errors.New("portal has no Matrix room ID")
	}

	meta := portalMeta(portal)
	loginMeta := loginMetadata(oc.UserLogin)

	// Refresh stored model capabilities (room capabilities may add image-understanding union separately)
	modelCaps := oc.getModelCapabilitiesForMeta(meta)
	if meta.Capabilities != modelCaps {
		meta.Capabilities = modelCaps
		if err := portal.Save(ctx); err != nil {
			oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to save portal after capability refresh")
		}
	}

	roomCaps := oc.getRoomCapabilities(ctx, meta)

	// Build reasoning effort options if model supports reasoning
	var reasoningEfforts []ReasoningEffortOption
	if roomCaps.SupportsReasoning {
		reasoningEfforts = []ReasoningEffortOption{
			{Value: "low", Label: "Low"},
			{Value: "medium", Label: "Medium"},
			{Value: "high", Label: "High"},
		}
	}

	content := &RoomCapabilitiesEventContent{
		Capabilities:           &roomCaps,
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
		oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to broadcast room capabilities")
		return err
	}

	// Also update standard room features for clients
	portal.UpdateCapabilities(ctx, oc.UserLogin, true)

	oc.loggerForContext(ctx).Debug().Str("model", meta.Model).Msg("Broadcasted room capabilities")
	return nil
}

// broadcastSettings sends user-editable settings to Matrix room state
// This event uses normal power levels (0) so users can modify
func (oc *AIClient) broadcastSettings(ctx context.Context, portal *bridgev2.Portal) error {
	if portal.MXID == "" {
		return errors.New("portal has no Matrix room ID")
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
		AgentID:             meta.AgentID,
	}

	bot := oc.UserLogin.Bridge.Bot
	_, err := bot.SendState(ctx, portal.MXID, RoomSettingsEventType, "", &event.Content{
		Parsed: content,
	}, time.Time{})

	if err != nil {
		oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to broadcast room settings")
		return err
	}

	meta.LastRoomStateSync = time.Now().Unix()
	if err := portal.Save(ctx); err != nil {
		oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to save portal after state broadcast")
	}

	oc.loggerForContext(ctx).Debug().Str("model", meta.Model).Msg("Broadcasted room settings")
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
		MsgType:  event.MsgNotice,
		Body:     message,
		Mentions: &event.Mentions{},
	}

	if _, err := bot.SendMessage(ctx, portal.MXID, event.EventMessage, &event.Content{
		Parsed: content,
	}, nil); err != nil {
		oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to send system notice")
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
	logCtx := oc.loggerForContext(ctx).With().Str("component", "openai-chat-bootstrap").Logger().WithContext(ctx)
	oc.waitForLoginPersisted(logCtx)

	meta := loginMetadata(oc.UserLogin)

	// Check if bootstrap already completed successfully
	if meta.ChatsSynced {
		oc.loggerForContext(ctx).Debug().Msg("Chats already synced, skipping bootstrap")
		// Still sync counter in case portals were created externally
		if err := oc.syncChatCounter(logCtx); err != nil {
			oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to sync chat counter")
		}
		return
	}

	oc.loggerForContext(ctx).Info().Msg("Starting bootstrap for new login")

	if err := oc.syncChatCounter(logCtx); err != nil {
		oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to sync chat counter, continuing with default chat creation")
		// Don't return - still create the default chat (matches other bridge patterns)
	}

	// Create default chat room for the active bridge profile.
	if err := oc.ensureDefaultChat(logCtx); err != nil {
		oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to ensure default chat")
		// Continue anyway - default chat is optional
	}

	// Mark bootstrap as complete only after successful completion
	meta.ChatsSynced = true
	if err := oc.UserLogin.Save(logCtx); err != nil {
		oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to save ChatsSynced flag")
	} else {
		oc.loggerForContext(ctx).Info().Msg("Bootstrap completed successfully, ChatsSynced flag set")
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
			oc.loggerForContext(ctx).Warn().Msg("Timed out waiting for login to persist, continuing anyway")
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
	oc.loggerForContext(ctx).Debug().Msg("Ensuring default AI chat room exists")
	loginMeta := loginMetadata(oc.UserLogin)
	defaultPortalKey := defaultChatPortalKey(oc.UserLogin.ID)

	if loginMeta.DefaultChatPortalID != "" {
		portalKey := networkid.PortalKey{
			ID:       networkid.PortalID(loginMeta.DefaultChatPortalID),
			Receiver: oc.UserLogin.ID,
		}
		portal, err := oc.UserLogin.Bridge.GetPortalByKey(ctx, portalKey)
		if err != nil {
			oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to load default chat portal by ID")
		} else if portal != nil {
			if portal.MXID != "" {
				oc.loggerForContext(ctx).Debug().Stringer("portal", portal.PortalKey).Msg("Existing default chat already has MXID")
				return nil
			}
			info := oc.chatInfoFromPortal(ctx, portal)
			oc.loggerForContext(ctx).Info().Stringer("portal", portal.PortalKey).Msg("Default chat missing MXID; creating Matrix room")
			err := portal.CreateMatrixRoom(ctx, oc.UserLogin, info)
			if err != nil {
				oc.loggerForContext(ctx).Err(err).Msg("Failed to create Matrix room for default chat")
			}
			oc.sendWelcomeMessage(ctx, portal)
			return err
		}
	}

	if loginMeta.DefaultChatPortalID == "" {
		portal, err := oc.UserLogin.Bridge.GetExistingPortalByKey(ctx, defaultPortalKey)
		if err != nil {
			oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to load default chat portal by deterministic key")
		} else if portal != nil {
			loginMeta.DefaultChatPortalID = string(portal.PortalKey.ID)
			if err := oc.UserLogin.Save(ctx); err != nil {
				oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to persist default chat portal ID")
			}
			if portal.MXID != "" {
				oc.loggerForContext(ctx).Debug().Stringer("portal", portal.PortalKey).Msg("Existing default chat already has MXID")
				return nil
			}
			info := oc.chatInfoFromPortal(ctx, portal)
			oc.loggerForContext(ctx).Info().Stringer("portal", portal.PortalKey).Msg("Default chat missing MXID; creating Matrix room")
			err := portal.CreateMatrixRoom(ctx, oc.UserLogin, info)
			if err != nil {
				oc.loggerForContext(ctx).Err(err).Msg("Failed to create Matrix room for default chat")
			}
			oc.sendWelcomeMessage(ctx, portal)
			return err
		}
	}

	portals, err := oc.listAllChatPortals(ctx)
	if err != nil {
		oc.loggerForContext(ctx).Err(err).Msg("Failed to list chat portals")
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
			oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to persist default chat portal ID")
		}
		if defaultPortal.MXID != "" {
			oc.loggerForContext(ctx).Debug().Stringer("portal", defaultPortal.PortalKey).Msg("Existing chat already has MXID")
			return nil
		}
		info := oc.chatInfoFromPortal(ctx, defaultPortal)
		oc.loggerForContext(ctx).Info().Stringer("portal", defaultPortal.PortalKey).Msg("Existing portal missing MXID; creating Matrix room")
		err := defaultPortal.CreateMatrixRoom(ctx, oc.UserLogin, info)
		if err != nil {
			oc.loggerForContext(ctx).Err(err).Msg("Failed to create Matrix room for existing portal")
		}
		oc.sendWelcomeMessage(ctx, defaultPortal)
		return err
	}

	modelID := oc.effectiveModel(nil)
	if modelID == "" {
		return errors.New("no default model configured")
	}

	portal, chatInfo, err := oc.initPortalForChat(ctx, PortalInitOpts{
		ModelID:      modelID,
		Title:        "New AI Chat",
		SystemPrompt: defaultRawModeSystemPrompt,
		PortalKey:    &defaultPortalKey,
	})
	if err != nil {
		existingPortal, existingErr := oc.UserLogin.Bridge.GetExistingPortalByKey(ctx, defaultPortalKey)
		if existingErr == nil && existingPortal != nil {
			loginMeta.DefaultChatPortalID = string(existingPortal.PortalKey.ID)
			if err := oc.UserLogin.Save(ctx); err != nil {
				oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to persist default chat portal ID")
			}
			if existingPortal.MXID != "" {
				oc.loggerForContext(ctx).Debug().Stringer("portal", existingPortal.PortalKey).Msg("Existing default chat already has MXID")
				return nil
			}
			info := oc.chatInfoFromPortal(ctx, existingPortal)
			oc.loggerForContext(ctx).Info().Stringer("portal", existingPortal.PortalKey).Msg("Default chat missing MXID; creating Matrix room")
			createErr := existingPortal.CreateMatrixRoom(ctx, oc.UserLogin, info)
			if createErr != nil {
				oc.loggerForContext(ctx).Err(createErr).Msg("Failed to create Matrix room for default chat")
				return createErr
			}
			oc.sendWelcomeMessage(ctx, existingPortal)
			oc.loggerForContext(ctx).Info().Stringer("portal", existingPortal.PortalKey).Msg("New AI Chat room created")
			return nil
		}
		oc.loggerForContext(ctx).Err(err).Msg("Failed to create default portal")
		return err
	}

	// Set simple room metadata.
	pm := portalMeta(portal)
	pm.AgentID = ""
	pm.SystemPrompt = defaultRawModeSystemPrompt
	pm.IsRawMode = true
	portal.OtherUserID = modelUserID(modelID)

	if err := portal.Save(ctx); err != nil {
		oc.loggerForContext(ctx).Err(err).Msg("Failed to save portal with agent config")
		return err
	}

	loginMeta.DefaultChatPortalID = string(portal.PortalKey.ID)
	if err := oc.UserLogin.Save(ctx); err != nil {
		oc.loggerForContext(ctx).Warn().Err(err).Msg("Failed to persist default chat portal ID")
	}
	err = portal.CreateMatrixRoom(ctx, oc.UserLogin, chatInfo)
	if err != nil {
		oc.loggerForContext(ctx).Err(err).Msg("Failed to create Matrix room for default chat")
		return err
	}
	oc.sendWelcomeMessage(ctx, portal)
	oc.loggerForContext(ctx).Info().Stringer("portal", portal.PortalKey).Msg("New AI Chat room created")
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
	oc.loggerForContext(ctx).Debug().
		Stringer("event_id", msg.TargetMessage.MXID).
		Stringer("portal", msg.Portal.PortalKey).
		Msg("Handling message deletion")

	// Delete from our database - the Matrix side is already handled by the bridge framework
	if err := oc.UserLogin.Bridge.DB.Message.Delete(ctx, msg.TargetMessage.RowID); err != nil {
		oc.loggerForContext(ctx).Warn().Err(err).Stringer("event_id", msg.TargetMessage.MXID).Msg("Failed to delete message from database")
		return err
	}

	return nil
}

// HandleMatrixDisappearingTimer handles disappearing message timer changes from Matrix
// For AI bridge, we just update the portal's disappear field - the bridge framework handles the actual deletion
func (oc *AIClient) HandleMatrixDisappearingTimer(ctx context.Context, msg *bridgev2.MatrixDisappearingTimer) (bool, error) {
	oc.loggerForContext(ctx).Debug().
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
