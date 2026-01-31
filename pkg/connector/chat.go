package connector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/event"
)

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
	if opts.CopyFrom != nil {
		pmeta = &PortalMetadata{
			Model:                  opts.CopyFrom.Model,
			Slug:                   slug,
			Title:                  title,
			SystemPrompt:           opts.CopyFrom.SystemPrompt,
			Temperature:            opts.CopyFrom.Temperature,
			MaxContextMessages:     opts.CopyFrom.MaxContextMessages,
			MaxCompletionTokens:    opts.CopyFrom.MaxCompletionTokens,
			ReasoningEffort:        opts.CopyFrom.ReasoningEffort,
			Capabilities:           opts.CopyFrom.Capabilities,
			ToolsEnabled:           opts.CopyFrom.ToolsEnabled,
			ConversationMode:       opts.CopyFrom.ConversationMode,
			WebSearchEnabled:       opts.CopyFrom.WebSearchEnabled,
			FileSearchEnabled:      opts.CopyFrom.FileSearchEnabled,
			CodeInterpreterEnabled: opts.CopyFrom.CodeInterpreterEnabled,
			EmitThinking:           opts.CopyFrom.EmitThinking,
			EmitToolArgs:           opts.CopyFrom.EmitToolArgs,
			DefaultAgentID:         opts.CopyFrom.DefaultAgentID,
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

	// 6. Broadcast room state
	go oc.BroadcastRoomState(runCtx, newPortal)

	// 7. Send notice with link
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

	// Broadcast room state
	go oc.BroadcastRoomState(runCtx, newPortal)

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
		},
	}
}

// updatePortalConfig applies room config to portal metadata
func (oc *AIClient) updatePortalConfig(ctx context.Context, portal *bridgev2.Portal, config *RoomConfigEventContent) {
	meta := portalMeta(portal)

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
	if config.ToolsEnabled != nil {
		meta.ToolsEnabled = *config.ToolsEnabled
	}
	if config.WebSearchEnabled != nil {
		meta.WebSearchEnabled = *config.WebSearchEnabled
	}
	if config.FileSearchEnabled != nil {
		meta.FileSearchEnabled = *config.FileSearchEnabled
	}
	if config.CodeInterpreterEnabled != nil {
		meta.CodeInterpreterEnabled = *config.CodeInterpreterEnabled
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
}

// BroadcastRoomState sends current room config to Matrix room state
func (oc *AIClient) BroadcastRoomState(ctx context.Context, portal *bridgev2.Portal) error {
	if portal.MXID == "" {
		return fmt.Errorf("portal has no Matrix room ID")
	}

	meta := portalMeta(portal)

	stateContent := &RoomConfigEventContent{
		Model:                  meta.Model,
		SystemPrompt:           meta.SystemPrompt,
		Temperature:            &meta.Temperature,
		MaxContextMessages:     meta.MaxContextMessages,
		MaxCompletionTokens:    meta.MaxCompletionTokens,
		ReasoningEffort:        meta.ReasoningEffort,
		ToolsEnabled:           ptr.Ptr(meta.ToolsEnabled),
		ConversationMode:       meta.ConversationMode,
		WebSearchEnabled:       ptr.Ptr(meta.WebSearchEnabled),
		FileSearchEnabled:      ptr.Ptr(meta.FileSearchEnabled),
		CodeInterpreterEnabled: ptr.Ptr(meta.CodeInterpreterEnabled),
	}

	// Use bot intent to send state event
	bot := oc.UserLogin.Bridge.Bot
	_, err := bot.SendState(ctx, portal.MXID, RoomConfigEventType, "", &event.Content{
		Parsed: stateContent,
	}, time.Time{})

	if err != nil {
		oc.log.Warn().Err(err).Msg("Failed to broadcast room state")
		return err
	}

	meta.LastRoomStateSync = time.Now().Unix()
	if err := portal.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to save portal after state broadcast")
	}

	oc.log.Debug().Str("model", meta.Model).Msg("Broadcasted room state")
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
	go oc.bootstrap(backgroundCtx)
}

func (oc *AIClient) bootstrap(ctx context.Context) {
	logCtx := oc.log.With().Str("component", "openai-chat-bootstrap").Logger().WithContext(ctx)
	oc.waitForLoginPersisted(logCtx)
	if err := oc.syncChatCounter(logCtx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to sync chat counter")
		return
	}
	if err := oc.ensureDefaultChat(logCtx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to ensure default chat")
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
	oc.log.Debug().Msg("Ensuring default ChatGPT room exists")
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
		// Broadcast initial room state
		go oc.BroadcastRoomState(ctx, portals[0])
		return err
	}
	resp, err := oc.createChat(ctx, "", "") // No default system prompt
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
	// Broadcast initial room state
	go oc.BroadcastRoomState(ctx, resp.Portal)
	oc.log.Info().Stringer("portal", resp.PortalKey).Msg("Default ChatGPT room created")
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
