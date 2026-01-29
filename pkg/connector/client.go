package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var (
	_ bridgev2.NetworkAPI                    = (*AIClient)(nil)
	_ bridgev2.IdentifierResolvingNetworkAPI = (*AIClient)(nil)
	_ bridgev2.ContactListingNetworkAPI      = (*AIClient)(nil)
	_ bridgev2.EditHandlingNetworkAPI        = (*AIClient)(nil)
)

var rejectAllMediaFileFeatures = &event.FileFeatures{
	MimeTypes: map[string]event.CapabilitySupportLevel{
		"*/*": event.CapLevelRejected,
	},
	Caption: event.CapLevelRejected,
}

func cloneRejectAllMediaFeatures() *event.FileFeatures {
	return rejectAllMediaFileFeatures.Clone()
}

type AIClient struct {
	UserLogin *bridgev2.UserLogin
	connector *OpenAIConnector
	api       openai.Client
	apiKey    string
	log       zerolog.Logger

	// Provider abstraction layer - all providers use OpenAI SDK
	provider AIProvider

	loggedIn atomic.Bool
	chatLock sync.Mutex

	// Turn-based message queuing: only one response per room at a time
	activeRooms   map[id.RoomID]bool
	activeRoomsMu sync.Mutex

	// Pending message queue per room (for turn-based behavior)
	pendingMessages   map[id.RoomID][]pendingMessage
	pendingMessagesMu sync.Mutex
}

// pendingMessageType indicates what kind of pending message this is
type pendingMessageType string

const (
	pendingTypeText           pendingMessageType = "text"
	pendingTypeImage          pendingMessageType = "image"
	pendingTypeRegenerate     pendingMessageType = "regenerate"
	pendingTypeEditRegenerate pendingMessageType = "edit_regenerate"
)

// pendingMessage represents a queued message waiting for AI processing
// Prompt is built fresh when processing starts to ensure up-to-date history
type pendingMessage struct {
	Event       *event.Event
	Portal      *bridgev2.Portal
	Meta        *PortalMetadata
	Type        pendingMessageType
	MessageBody string              // For text, regenerate, edit_regenerate
	ImageURL    string              // For image messages
	TargetMsgID networkid.MessageID // For edit_regenerate
}

func newAIClient(login *bridgev2.UserLogin, connector *OpenAIConnector, apiKey string) (*AIClient, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, fmt.Errorf("missing API key")
	}

	// Get per-user credentials from login metadata
	meta := login.Metadata.(*UserLoginMetadata)
	log := login.Log.With().Str("component", "ai-network").Str("provider", meta.Provider).Logger()

	// Create base client struct
	oc := &AIClient{
		UserLogin:       login,
		connector:       connector,
		apiKey:          key,
		log:             log,
		activeRooms:     make(map[id.RoomID]bool),
		pendingMessages: make(map[id.RoomID][]pendingMessage),
	}

	// Use per-user base_url if provided
	baseURL := strings.TrimSpace(meta.BaseURL)

	// Initialize provider based on login metadata
	// All providers use the OpenAI SDK with different base URLs
	switch meta.Provider {
	case ProviderBeeper:
		// Beeper mode: routes through Beeper's OpenRouter proxy
		beeperBaseURL := baseURL
		if beeperBaseURL == "" {
			return nil, fmt.Errorf("beeper base_url is required for Beeper provider")
		}

		// Get user ID for rate limiting
		userID := login.User.MXID.String()

		openrouterURL := beeperBaseURL + "/openrouter/v1"
		provider, err := NewOpenAIProviderWithUserID(key, openrouterURL, userID, log)
		if err != nil {
			return nil, fmt.Errorf("failed to create Beeper provider: %w", err)
		}
		oc.provider = provider
		oc.api = provider.Client()

	case ProviderOpenRouter:
		// OpenRouter direct access
		openrouterURL := baseURL
		if openrouterURL == "" {
			openrouterURL = "https://openrouter.ai/api/v1"
		}
		provider, err := NewOpenAIProviderWithBaseURL(key, openrouterURL, log)
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenRouter provider: %w", err)
		}
		oc.provider = provider
		oc.api = provider.Client()

	default:
		// OpenAI (default) or Custom OpenAI-compatible provider
		openaiURL := baseURL
		if openaiURL == "" {
			openaiURL = "https://api.openai.com/v1"
		}
		provider, err := NewOpenAIProviderWithBaseURL(key, openaiURL, log)
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenAI provider: %w", err)
		}
		oc.provider = provider
		oc.api = provider.Client()
	}

	return oc, nil
}

// acquireRoom tries to acquire a room for processing. Returns false if the room is already busy.
func (oc *AIClient) acquireRoom(roomID id.RoomID) bool {
	oc.activeRoomsMu.Lock()
	defer oc.activeRoomsMu.Unlock()
	if oc.activeRooms[roomID] {
		return false // already processing
	}
	oc.activeRooms[roomID] = true
	return true
}

// releaseRoom releases a room after processing is complete.
func (oc *AIClient) releaseRoom(roomID id.RoomID) {
	oc.activeRoomsMu.Lock()
	defer oc.activeRoomsMu.Unlock()
	delete(oc.activeRooms, roomID)
}

// queuePendingMessage adds a message to the pending queue for later processing
func (oc *AIClient) queuePendingMessage(roomID id.RoomID, msg pendingMessage) {
	oc.pendingMessagesMu.Lock()
	defer oc.pendingMessagesMu.Unlock()
	oc.pendingMessages[roomID] = append(oc.pendingMessages[roomID], msg)
	oc.log.Debug().
		Str("room_id", roomID.String()).
		Int("queue_length", len(oc.pendingMessages[roomID])).
		Msg("Message queued for later processing")
}

// popNextPending removes and returns the next pending message for a room, or nil if none
func (oc *AIClient) popNextPending(roomID id.RoomID) *pendingMessage {
	oc.pendingMessagesMu.Lock()
	defer oc.pendingMessagesMu.Unlock()
	queue := oc.pendingMessages[roomID]
	if len(queue) == 0 {
		return nil
	}
	msg := queue[0]
	oc.pendingMessages[roomID] = queue[1:]
	if len(oc.pendingMessages[roomID]) == 0 {
		delete(oc.pendingMessages, roomID)
	}
	return &msg
}

// processNextPending processes the next pending message for a room if one exists
func (oc *AIClient) processNextPending(ctx context.Context, roomID id.RoomID) {
	pending := oc.popNextPending(roomID)
	if pending == nil {
		return
	}

	oc.log.Debug().
		Str("room_id", roomID.String()).
		Str("type", string(pending.Type)).
		Msg("Processing next pending message")

	// Re-acquire the room lock and process
	if !oc.acquireRoom(roomID) {
		// Room somehow got busy again, re-queue the message
		oc.queuePendingMessage(roomID, *pending)
		return
	}

	// Build prompt NOW with fresh history (includes previous AI responses)
	var promptMessages []openai.ChatCompletionMessageParamUnion
	var err error

	switch pending.Type {
	case pendingTypeText:
		promptMessages, err = oc.buildPrompt(ctx, pending.Portal, pending.Meta, pending.MessageBody)
	case pendingTypeImage:
		promptMessages, err = oc.buildPromptWithImage(ctx, pending.Portal, pending.Meta, pending.MessageBody, pending.ImageURL)
	case pendingTypeRegenerate:
		promptMessages, err = oc.buildPromptForRegenerate(ctx, pending.Portal, pending.Meta, pending.MessageBody)
	case pendingTypeEditRegenerate:
		promptMessages, err = oc.buildPromptUpToMessage(ctx, pending.Portal, pending.Meta, pending.TargetMsgID, pending.MessageBody)
	default:
		err = fmt.Errorf("unknown pending message type: %s", pending.Type)
	}

	if err != nil {
		oc.log.Err(err).Str("type", string(pending.Type)).Msg("Failed to build prompt for pending message")
		oc.releaseRoom(roomID)
		oc.processNextPending(oc.backgroundContext(ctx), roomID)
		return
	}

	// Send SUCCESS status synchronously - message is now being processed
	oc.sendSuccessStatus(ctx, pending.Portal, pending.Event)

	// Process in background, will release room when done
	go func() {
		defer func() {
			oc.releaseRoom(roomID)
			// Check for more pending messages
			oc.processNextPending(oc.backgroundContext(ctx), roomID)
		}()
		oc.dispatchCompletionInternal(ctx, pending.Event, pending.Portal, pending.Meta, promptMessages)
	}()
}

func (oc *AIClient) Connect(ctx context.Context) {
	// Use a default model for validation (any model works to verify credentials)
	model := "gpt-4o-mini"
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := oc.api.Models.Get(timeoutCtx, model)
	if err != nil {
		oc.log.Warn().Err(err).Str("model", model).Msg("Failed to validate OpenAI credentials")
		oc.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateTransientDisconnect,
			Error:      "openai-auth-error",
			Message:    "Failed to validate OpenAI credentials",
			Info: map[string]any{
				"model": model,
				"error": err.Error(),
			},
		})
		return
	}
	oc.loggedIn.Store(true)
	oc.UserLogin.BridgeState.Send(status.BridgeState{
		StateEvent: status.StateConnected,
		Message:    "Connected to OpenAI",
	})
}

func (oc *AIClient) Disconnect() {
	oc.loggedIn.Store(false)
}

func (oc *AIClient) IsLoggedIn() bool {
	return oc.loggedIn.Load()
}

func (oc *AIClient) LogoutRemote(ctx context.Context) {
	oc.Disconnect()
	oc.UserLogin.BridgeState.Send(status.BridgeState{
		StateEvent: status.StateLoggedOut,
		Message:    "Disconnected by user",
	})
}

func (oc *AIClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	return userID == humanUserID(oc.UserLogin.ID)
}

func (oc *AIClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	meta := portalMeta(portal)
	title := meta.Title
	if title == "" {
		if portal.Name != "" {
			title = portal.Name
		} else {
			title = "ChatGPT"
		}
	}
	return &bridgev2.ChatInfo{
		Name:  ptr.Ptr(title),
		Topic: ptrIfNotEmpty(meta.SystemPrompt),
	}, nil
}

func (oc *AIClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	ghostID := string(ghost.ID)

	// Parse model from ghost ID (format: "model-{escaped-model-id}")
	if modelID := parseModelFromGhostID(ghostID); modelID != "" {
		caps := getModelCapabilities(modelID)
		displayName := FormatModelDisplay(modelID)
		if caps.SupportsVision {
			displayName += " (Vision)"
		}
		return &bridgev2.UserInfo{
			Name:  ptr.Ptr(displayName),
			IsBot: ptr.Ptr(false),
		}, nil
	}

	// Fallback for unknown ghost types
	return &bridgev2.UserInfo{
		Name:  ptr.Ptr("AI Assistant"),
		IsBot: ptr.Ptr(false),
	}, nil
}

func (oc *AIClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *event.RoomFeatures {
	meta := portalMeta(portal)

	// Base capabilities for all AI chats
	caps := &event.RoomFeatures{
		File: event.FileFeatureMap{
			event.MsgVideo:      cloneRejectAllMediaFeatures(),
			event.MsgAudio:      cloneRejectAllMediaFeatures(),
			event.MsgFile:       cloneRejectAllMediaFeatures(),
			event.CapMsgVoice:   cloneRejectAllMediaFeatures(),
			event.CapMsgGIF:     cloneRejectAllMediaFeatures(),
			event.CapMsgSticker: cloneRejectAllMediaFeatures(),
		},
		Reply:               event.CapLevelFullySupported,
		Thread:              event.CapLevelFullySupported,
		Edit:                event.CapLevelFullySupported,
		Delete:              event.CapLevelPartialSupport,
		TypingNotifications: true, // Always enabled
		ReadReceipts:        true,
	}

	// Add image support if model supports vision (from cached capabilities)
	if meta.Capabilities.SupportsVision {
		caps.File[event.MsgImage] = &event.FileFeatures{
			MimeTypes: map[string]event.CapabilitySupportLevel{
				"image/png":  event.CapLevelFullySupported,
				"image/jpeg": event.CapLevelFullySupported,
				"image/webp": event.CapLevelFullySupported,
				"image/gif":  event.CapLevelFullySupported,
			},
			Caption:          event.CapLevelFullySupported,
			MaxCaptionLength: 10000,
			MaxSize:          20 * 1024 * 1024, // 20MB
		}
	} else {
		caps.File[event.MsgImage] = cloneRejectAllMediaFeatures()
	}

	return caps
}

func (oc *AIClient) GetContactList(ctx context.Context) ([]*bridgev2.ResolveIdentifierResponse, error) {
	oc.log.Debug().Msg("Contact list requested")

	// Fetch available models (use cache if available)
	models, err := oc.listAvailableModels(ctx, false)
	if err != nil {
		oc.log.Error().Err(err).Msg("Failed to list models, using fallback")
		// Return default model as fallback based on provider
		meta := loginMetadata(oc.UserLogin)
		models = []ModelInfo{{
			ID:       DefaultModelForProvider(meta.Provider),
			Name:     "GPT 4o Mini",
			Provider: meta.Provider,
		}}
	}

	// Create a contact for each model
	contacts := make([]*bridgev2.ResolveIdentifierResponse, 0, len(models))

	for _, model := range models {
		// Get or create ghost for this model
		userID := modelUserID(model.ID)
		ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, userID)
		if err != nil {
			oc.log.Warn().Err(err).Str("model", model.ID).Msg("Failed to get ghost for model")
			continue
		}

		// Create user info
		caps := getModelCapabilities(model.ID)
		displayName := FormatModelDisplay(model.ID)
		if caps.SupportsVision {
			displayName += " (Vision)"
		}

		contacts = append(contacts, &bridgev2.ResolveIdentifierResponse{
			UserID: userID,
			UserInfo: &bridgev2.UserInfo{
				Name:        ptr.Ptr(displayName),
				IsBot:       ptr.Ptr(false),
				Identifiers: []string{model.ID},
			},
			Ghost: ghost,
			// Chat will be created on-demand via ResolveIdentifier
		})
	}

	oc.log.Info().Int("count", len(contacts)).Msg("Returning model contact list")
	return contacts, nil
}

func (oc *AIClient) ResolveIdentifier(ctx context.Context, identifier string, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	// Identifier is the model ID (e.g., "gpt-4o", "gpt-4-turbo")
	modelID := strings.TrimSpace(identifier)
	if modelID == "" {
		return nil, fmt.Errorf("model identifier is required")
	}

	// Validate model exists (check cache first)
	models, _ := oc.listAvailableModels(ctx, false)
	var found bool
	for i := range models {
		if models[i].ID == modelID {
			found = true
			break
		}
	}

	if !found {
		// Model not in cache, assume it's valid (user might have access to beta models)
		oc.log.Warn().Str("model", modelID).Msg("Model not in cache, assuming valid")
	}

	// Compute capabilities for this model
	caps := getModelCapabilities(modelID)

	// Get or create ghost
	userID := modelUserID(modelID)
	ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ghost: %w", err)
	}

	var chatResp *bridgev2.CreateChatResponse
	if createChat {
		oc.log.Info().Str("model", modelID).Msg("Creating new chat for model")
		chatResp, err = oc.createNewChat(ctx, modelID, caps)
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
func (oc *AIClient) createNewChat(ctx context.Context, modelID string, caps ModelCapabilities) (*bridgev2.CreateChatResponse, error) {
	chatIndex, err := oc.allocateNextChatIndex(ctx)
	if err != nil {
		return nil, err
	}

	// Create portal metadata with model-specific settings
	// Per-room settings (SystemPrompt, Temperature, etc.) start at zero values
	// and use hardcoded defaults until user overrides via /commands
	portalMeta := &PortalMetadata{
		Model:        modelID,
		Slug:         formatChatSlug(chatIndex),
		Title:        fmt.Sprintf("%s Chat %d", FormatModelDisplay(modelID), chatIndex),
		Capabilities: caps,
	}

	portalKey := networkid.PortalKey{
		ID:       networkid.PortalID(fmt.Sprintf("openai:model:%s:%s", modelUserID(modelID), formatChatSlug(chatIndex))),
		Receiver: oc.UserLogin.ID,
	}

	portal, err := oc.UserLogin.Bridge.GetPortalByKey(ctx, portalKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get portal: %w", err)
	}

	// Set portal metadata
	portal.Metadata = portalMeta
	portal.RoomType = database.RoomTypeDM
	portal.OtherUserID = modelUserID(modelID)
	portal.Name = portalMeta.Title
	portal.NameSet = true

	if err := portal.Save(ctx); err != nil {
		return nil, fmt.Errorf("failed to save portal: %w", err)
	}

	// Create chat info with proper member list
	chatInfo := &bridgev2.ChatInfo{
		Name:    ptr.Ptr(portalMeta.Title),
		Topic:   ptr.Ptr(fmt.Sprintf("Conversation with %s", modelID)),
		Type:    ptr.Ptr(database.RoomTypeDM),
		Members: oc.buildChatMembers(modelID),
	}

	return &bridgev2.CreateChatResponse{
		PortalKey:  portalKey,
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

// buildChatMembers creates the standard member list for a chat portal
func (oc *AIClient) buildChatMembers(modelID string) *bridgev2.ChatMemberList {
	return &bridgev2.ChatMemberList{
		MemberMap: map[networkid.UserID]bridgev2.ChatMember{
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
					Name:  ptr.Ptr(FormatModelDisplay(modelID)),
					IsBot: ptr.Ptr(false),
				},
			},
		},
		IsFull:           true,
		TotalMemberCount: 2,
	}
}

func (oc *AIClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	if msg.Content == nil {
		return nil, fmt.Errorf("missing message content")
	}

	portal := msg.Portal
	if portal == nil {
		return nil, fmt.Errorf("portal is nil")
	}
	meta := portalMeta(portal)

	// Handle image messages for vision-capable models
	if msg.Content.MsgType == event.MsgImage {
		return oc.handleImageMessage(ctx, msg, portal, meta)
	}

	switch msg.Content.MsgType {
	case event.MsgText, event.MsgNotice, event.MsgEmote:
	default:
		return nil, fmt.Errorf("%s messages are not supported", msg.Content.MsgType)
	}
	body := strings.TrimSpace(msg.Content.Body)
	if body == "" {
		return nil, fmt.Errorf("empty messages are not supported")
	}

	// Check for commands
	if handled := oc.handleCommand(ctx, msg.Event, portal, meta, body); handled {
		// Return empty response - framework will send SUCCESS immediately
		// No DB message needed since commands aren't chat messages
		return &bridgev2.MatrixMessageResponse{}, nil
	}

	promptMessages, err := oc.buildPrompt(ctx, portal, meta, body)
	if err != nil {
		return nil, err
	}
	userMessage := &database.Message{
		ID:       networkid.MessageID(fmt.Sprintf("mx:%s", string(msg.Event.ID))),
		Room:     portal.PortalKey,
		SenderID: humanUserID(oc.UserLogin.ID),
		Metadata: &MessageMetadata{
			Role: "user",
			Body: body,
		},
		Timestamp: time.Now(),
	}

	// Try to acquire room SYNCHRONOUSLY - no race condition
	if oc.acquireRoom(portal.MXID) {
		// Room acquired - framework will save message and send SUCCESS
		go func() {
			defer func() {
				oc.releaseRoom(portal.MXID)
				oc.processNextPending(oc.backgroundContext(ctx), portal.MXID)
			}()
			oc.dispatchCompletionInternal(ctx, msg.Event, portal, meta, promptMessages)
		}()

		return &bridgev2.MatrixMessageResponse{
			DB: userMessage,
		}, nil
	}

	// Room busy - handle message saving ourselves to control status
	userMessage.MXID = msg.Event.ID
	err = oc.UserLogin.Bridge.DB.Message.Insert(ctx, userMessage)
	if err != nil {
		oc.log.Err(err).Msg("Failed to save queued message to database")
		// Continue anyway - the message will still be processed
	}

	// Queue the message for later processing (prompt built fresh when processed)
	oc.queuePendingMessage(portal.MXID, pendingMessage{
		Event:       msg.Event,
		Portal:      portal,
		Meta:        meta,
		Type:        pendingTypeText,
		MessageBody: body,
	})

	// Send PENDING status - message shows "Sending..." in client
	oc.sendPendingStatus(ctx, portal, msg.Event, "Waiting for previous response")

	// Return Pending: true so framework doesn't override our PENDING status with SUCCESS
	return &bridgev2.MatrixMessageResponse{
		Pending: true,
	}, nil
}

// HandleMatrixEdit handles edits to previously sent messages
func (oc *AIClient) HandleMatrixEdit(ctx context.Context, edit *bridgev2.MatrixEdit) error {
	if edit.Content == nil || edit.EditTarget == nil {
		return fmt.Errorf("invalid edit: missing content or target")
	}

	portal := edit.Portal
	if portal == nil {
		return fmt.Errorf("portal is nil")
	}
	meta := portalMeta(portal)

	// Get the new message body
	newBody := strings.TrimSpace(edit.Content.Body)
	if newBody == "" {
		return fmt.Errorf("empty edit body")
	}

	// Update the message metadata with the new content
	msgMeta := messageMeta(edit.EditTarget)
	if msgMeta == nil {
		msgMeta = &MessageMetadata{}
		edit.EditTarget.Metadata = msgMeta
	}
	msgMeta.Body = newBody

	// Only regenerate if this was a user message
	if msgMeta.Role != "user" {
		// Just update the content, don't regenerate
		return nil
	}

	oc.log.Info().
		Str("message_id", string(edit.EditTarget.ID)).
		Str("new_body", newBody).
		Msg("User edited message, regenerating response")

	// Find the assistant response that came after this message
	// We'll delete it and regenerate
	err := oc.regenerateFromEdit(ctx, edit.Event, portal, meta, edit.EditTarget, newBody)
	if err != nil {
		oc.log.Err(err).Msg("Failed to regenerate response after edit")
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Failed to regenerate response: %v", err))
	}

	return nil
}

// regenerateFromEdit regenerates the AI response based on an edited user message
func (oc *AIClient) regenerateFromEdit(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	editedMessage *database.Message,
	newBody string,
) error {
	// Get messages in the portal to find the assistant response after the edited message
	messages, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, 50)
	if err != nil {
		return fmt.Errorf("failed to get messages: %w", err)
	}

	// Find the assistant response that came after the edited message
	var assistantResponse *database.Message
	foundEditedMessage := false
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.ID == editedMessage.ID {
			foundEditedMessage = true
			continue
		}
		if foundEditedMessage {
			msgMeta := messageMeta(msg)
			if msgMeta != nil && msgMeta.Role == "assistant" {
				assistantResponse = msg
				break
			}
		}
	}

	// Build the prompt with the edited message included
	// We need to rebuild from scratch up to the edited message
	promptMessages, err := oc.buildPromptUpToMessage(ctx, portal, meta, editedMessage.ID, newBody)
	if err != nil {
		return fmt.Errorf("failed to build prompt: %w", err)
	}

	// If we found an assistant response, we'll redact/edit it
	if assistantResponse != nil {
		// Try to redact the old response
		if assistantResponse.MXID != "" {
			intent, _ := portal.GetIntentFor(ctx, bridgev2.EventSender{IsFromMe: true}, oc.UserLogin, bridgev2.RemoteEventMessageRemove)
			if intent != nil {
				_, _ = intent.SendMessage(ctx, portal.MXID, event.EventRedaction, &event.Content{
					Parsed: &event.RedactionEventContent{
						Redacts: assistantResponse.MXID,
					},
				}, nil)
			}
		}
	}

	// Dispatch a new completion with synchronous room acquisition
	if oc.acquireRoom(portal.MXID) {
		oc.sendSuccessStatus(ctx, portal, evt)
		go func() {
			defer func() {
				oc.releaseRoom(portal.MXID)
				oc.processNextPending(oc.backgroundContext(ctx), portal.MXID)
			}()
			oc.dispatchCompletionInternal(ctx, evt, portal, meta, promptMessages)
		}()
	} else {
		oc.queuePendingMessage(portal.MXID, pendingMessage{
			Event:       evt,
			Portal:      portal,
			Meta:        meta,
			Type:        pendingTypeEditRegenerate,
			MessageBody: newBody,
			TargetMsgID: editedMessage.ID,
		})
		oc.sendPendingStatus(ctx, portal, evt, "Waiting for previous response")
	}

	return nil
}

// buildPromptUpToMessage builds a prompt including messages up to and including the specified message
func (oc *AIClient) buildPromptUpToMessage(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	targetMessageID networkid.MessageID,
	newBody string,
) ([]openai.ChatCompletionMessageParamUnion, error) {
	var prompt []openai.ChatCompletionMessageParamUnion

	// Add system prompt
	systemPrompt := oc.effectivePrompt(meta)
	if systemPrompt != "" {
		prompt = append(prompt, openai.SystemMessage(systemPrompt))
	}

	// Get history
	historyLimit := oc.historyLimit(meta)
	if historyLimit > 0 {
		history, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, historyLimit)
		if err != nil {
			return nil, fmt.Errorf("failed to load prompt history: %w", err)
		}

		// Add messages up to the target message, replacing the target with newBody
		for i := len(history) - 1; i >= 0; i-- {
			msg := history[i]
			msgMeta := messageMeta(msg)

			// Stop after adding the target message
			if msg.ID == targetMessageID {
				// Use the new body for the edited message
				prompt = append(prompt, openai.UserMessage(newBody))
				break
			}

			// Skip commands and non-conversation messages
			if !shouldIncludeInHistory(msgMeta) {
				continue
			}

			// Skip assistant messages that came after the target (we're going backwards)
			switch msgMeta.Role {
			case "assistant":
				prompt = append(prompt, openai.AssistantMessage(msgMeta.Body))
			default:
				prompt = append(prompt, openai.UserMessage(msgMeta.Body))
			}
		}
	} else {
		// No history, just add the new message
		prompt = append(prompt, openai.UserMessage(newBody))
	}

	return prompt, nil
}

// handleImageMessage processes an image message for vision-capable models
func (oc *AIClient) handleImageMessage(
	ctx context.Context,
	msg *bridgev2.MatrixMessage,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
) (*bridgev2.MatrixMessageResponse, error) {
	// Check if model supports vision
	if !meta.Capabilities.SupportsVision {
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf(
			"The current model (%s) does not support image analysis. "+
				"Please switch to a vision-capable model like gpt-4o or gpt-4-turbo using /model.",
			oc.effectiveModel(meta),
		))
		return nil, nil
	}

	// Get the image URL from the message
	imageURL := msg.Content.URL
	if imageURL == "" && msg.Content.File != nil {
		imageURL = msg.Content.File.URL
	}
	if imageURL == "" {
		return nil, fmt.Errorf("image message has no URL")
	}

	// Get caption (body is usually the filename or caption)
	caption := strings.TrimSpace(msg.Content.Body)
	if caption == "" || (msg.Content.Info != nil && caption == msg.Content.Info.MimeType) {
		caption = "What's in this image?"
	}

	// Build prompt with image
	promptMessages, err := oc.buildPromptWithImage(ctx, portal, meta, caption, string(imageURL))
	if err != nil {
		return nil, err
	}

	userMessage := &database.Message{
		ID:       networkid.MessageID(fmt.Sprintf("mx:%s", string(msg.Event.ID))),
		Room:     portal.PortalKey,
		SenderID: humanUserID(oc.UserLogin.ID),
		Metadata: &MessageMetadata{
			Role: "user",
			Body: caption + " [image]",
		},
		Timestamp: time.Now(),
	}

	// Try to acquire room SYNCHRONOUSLY - no race condition
	if oc.acquireRoom(portal.MXID) {
		// Room acquired - framework will save message and send SUCCESS
		go func() {
			defer func() {
				oc.releaseRoom(portal.MXID)
				oc.processNextPending(oc.backgroundContext(ctx), portal.MXID)
			}()
			oc.dispatchCompletionInternal(ctx, msg.Event, portal, meta, promptMessages)
		}()

		return &bridgev2.MatrixMessageResponse{
			DB: userMessage,
		}, nil
	}

	// Room busy - handle message saving ourselves to control status
	userMessage.MXID = msg.Event.ID
	err = oc.UserLogin.Bridge.DB.Message.Insert(ctx, userMessage)
	if err != nil {
		oc.log.Err(err).Msg("Failed to save queued image message to database")
	}

	// Queue the message for later processing (prompt built fresh when processed)
	oc.queuePendingMessage(portal.MXID, pendingMessage{
		Event:       msg.Event,
		Portal:      portal,
		Meta:        meta,
		Type:        pendingTypeImage,
		MessageBody: caption,
		ImageURL:    string(imageURL),
	})

	// Send PENDING status - message shows "Sending..." in client
	oc.sendPendingStatus(ctx, portal, msg.Event, "Waiting for previous response")

	// Return Pending: true so framework doesn't override our PENDING status with SUCCESS
	return &bridgev2.MatrixMessageResponse{
		Pending: true,
	}, nil
}

// buildBasePrompt builds the system prompt and history portion of a prompt.
// This is the common pattern used by buildPrompt and buildPromptWithImage.
func (oc *AIClient) buildBasePrompt(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
) ([]openai.ChatCompletionMessageParamUnion, error) {
	var prompt []openai.ChatCompletionMessageParamUnion

	// Add system prompt
	systemPrompt := oc.effectivePrompt(meta)
	if systemPrompt != "" {
		prompt = append(prompt, openai.SystemMessage(systemPrompt))
	}

	// Add history
	historyLimit := oc.historyLimit(meta)
	if historyLimit > 0 {
		history, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, historyLimit)
		if err != nil {
			return nil, fmt.Errorf("failed to load prompt history: %w", err)
		}
		for i := len(history) - 1; i >= 0; i-- {
			msgMeta := messageMeta(history[i])
			if !shouldIncludeInHistory(msgMeta) {
				continue
			}
			switch msgMeta.Role {
			case "assistant":
				prompt = append(prompt, openai.AssistantMessage(msgMeta.Body))
			default:
				prompt = append(prompt, openai.UserMessage(msgMeta.Body))
			}
		}
	}

	return prompt, nil
}

// buildPromptWithImage builds a prompt that includes an image URL
func (oc *AIClient) buildPromptWithImage(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	caption string,
	imageURL string,
) ([]openai.ChatCompletionMessageParamUnion, error) {
	prompt, err := oc.buildBasePrompt(ctx, portal, meta)
	if err != nil {
		return nil, err
	}

	// Build the user message with image
	// Convert Matrix mxc:// URL to HTTP URL for download
	httpURL := oc.convertMxcToHttp(imageURL)

	imageContent := openai.ChatCompletionContentPartUnionParam{
		OfImageURL: &openai.ChatCompletionContentPartImageParam{
			ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
				URL:    httpURL,
				Detail: "auto",
			},
		},
	}

	textContent := openai.ChatCompletionContentPartUnionParam{
		OfText: &openai.ChatCompletionContentPartTextParam{
			Text: caption,
		},
	}

	// Create user message with both image and text
	userMsg := openai.ChatCompletionMessageParamUnion{
		OfUser: &openai.ChatCompletionUserMessageParam{
			Content: openai.ChatCompletionUserMessageParamContentUnion{
				OfArrayOfContentParts: []openai.ChatCompletionContentPartUnionParam{
					textContent,
					imageContent,
				},
			},
		},
	}

	prompt = append(prompt, userMsg)
	return prompt, nil
}

// convertMxcToHttp converts an mxc:// URL to an HTTP URL via the homeserver
func (oc *AIClient) convertMxcToHttp(mxcURL string) string {
	// mxc://server/mediaID -> https://homeserver/_matrix/media/v3/download/server/mediaID
	if !strings.HasPrefix(mxcURL, "mxc://") {
		return mxcURL // Already HTTP
	}

	// Get homeserver URL from bridge config
	homeserver := oc.UserLogin.Bridge.Matrix.ServerName()

	// Parse mxc URL
	parts := strings.SplitN(strings.TrimPrefix(mxcURL, "mxc://"), "/", 2)
	if len(parts) != 2 {
		return mxcURL
	}

	server := parts[0]
	mediaID := parts[1]

	return fmt.Sprintf("https://%s/_matrix/media/v3/download/%s/%s", homeserver, server, mediaID)
}

// handleCommand checks if the message is a command and handles it
// Returns true if the message was a command and was handled
func (oc *AIClient) handleCommand(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	body string,
) bool {
	// Check for slash commands
	if strings.HasPrefix(body, "/") {
		return oc.handleSlashCommand(ctx, evt, portal, meta, body)
	}

	// Check for regenerate command
	prefix := oc.connector.Config.Bridge.CommandPrefix
	if strings.HasPrefix(body, prefix+" regenerate") || body == prefix+" regenerate" ||
		body == "!regenerate" || body == "/regenerate" {
		go oc.handleRegenerate(ctx, evt, portal, meta)
		return true
	}

	return false
}

// handleSlashCommand handles slash commands like /model, /temp, /prompt
func (oc *AIClient) handleSlashCommand(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	body string,
) bool {
	parts := strings.SplitN(body, " ", 2)
	cmd := strings.ToLower(parts[0])
	var arg string
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}

	switch cmd {
	case "/model":
		if arg == "" {
			oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Current model: %s", oc.effectiveModel(meta)))
			return true
		}
		// Validate model
		valid, err := oc.validateModel(ctx, arg)
		if err != nil || !valid {
			oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Invalid model: %s", arg))
			return true
		}
		// Update model
		meta.Model = arg
		meta.Capabilities = getModelCapabilities(arg)
		if err := portal.Save(ctx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to save portal after model change")
		}
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Model changed to: %s", arg))
		return true

	case "/temp", "/temperature":
		if arg == "" {
			oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Current temperature: %.2f", oc.effectiveTemperature(meta)))
			return true
		}
		var temp float64
		if _, err := fmt.Sscanf(arg, "%f", &temp); err != nil || temp < 0 || temp > 2 {
			oc.sendSystemNotice(ctx, portal, "Invalid temperature. Must be between 0 and 2.")
			return true
		}
		meta.Temperature = temp
		if err := portal.Save(ctx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to save portal after temperature change")
		}
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Temperature set to: %.2f", temp))
		return true

	case "/prompt", "/system":
		if arg == "" {
			current := oc.effectivePrompt(meta)
			if current == "" {
				current = "(none)"
			} else if len(current) > 100 {
				current = current[:100] + "..."
			}
			oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Current system prompt: %s", current))
			return true
		}
		meta.SystemPrompt = arg
		if err := portal.Save(ctx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to save portal after prompt change")
		}
		oc.sendSystemNotice(ctx, portal, "System prompt updated.")
		return true

	case "/context":
		if arg == "" {
			oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Current context limit: %d messages", oc.historyLimit(meta)))
			return true
		}
		var limit int
		if _, err := fmt.Sscanf(arg, "%d", &limit); err != nil || limit < 1 || limit > 100 {
			oc.sendSystemNotice(ctx, portal, "Invalid context limit. Must be between 1 and 100.")
			return true
		}
		meta.MaxContextMessages = limit
		if err := portal.Save(ctx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to save portal after context change")
		}
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Context limit set to: %d messages", limit))
		return true

	case "/tokens", "/maxtokens":
		if arg == "" {
			oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Current max tokens: %d", oc.effectiveMaxTokens(meta)))
			return true
		}
		var tokens int
		if _, err := fmt.Sscanf(arg, "%d", &tokens); err != nil || tokens < 1 || tokens > 16384 {
			oc.sendSystemNotice(ctx, portal, "Invalid max tokens. Must be between 1 and 16384.")
			return true
		}
		meta.MaxCompletionTokens = tokens
		if err := portal.Save(ctx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to save portal after tokens change")
		}
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Max tokens set to: %d", tokens))
		return true

	case "/config":
		mode := meta.ConversationMode
		if mode == "" {
			mode = "messages"
		}
		config := fmt.Sprintf(
			"Current configuration:\n"+
				"• Model: %s\n"+
				"• Temperature: %.2f\n"+
				"• Context: %d messages\n"+
				"• Max tokens: %d\n"+
				"• Vision: %v\n"+
				"• Mode: %s",
			oc.effectiveModel(meta),
			oc.effectiveTemperature(meta),
			oc.historyLimit(meta),
			oc.effectiveMaxTokens(meta),
			meta.Capabilities.SupportsVision,
			mode,
		)
		oc.sendSystemNotice(ctx, portal, config)
		return true

	case "/tools":
		if arg == "" {
			status := "disabled"
			if meta.ToolsEnabled {
				status = "enabled"
			}
			toolList := ""
			for _, tool := range BuiltinTools() {
				toolList += fmt.Sprintf("  - %s: %s\n", tool.Name, tool.Description)
			}
			oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Tools are currently %s.\n\nAvailable tools:\n%s\nUse /tools on or /tools off to toggle.", status, toolList))
			return true
		}
		switch strings.ToLower(arg) {
		case "on", "enable", "true", "1":
			meta.ToolsEnabled = true
			if err := portal.Save(ctx); err != nil {
				oc.log.Warn().Err(err).Msg("Failed to save portal after enabling tools")
			}
			if err := oc.BroadcastRoomState(ctx, portal); err != nil {
				oc.log.Warn().Err(err).Msg("Failed to broadcast room state after enabling tools")
			}
			oc.sendSystemNotice(ctx, portal, "Tools enabled. The AI can now use calculator and web search.")
		case "off", "disable", "false", "0":
			meta.ToolsEnabled = false
			if err := portal.Save(ctx); err != nil {
				oc.log.Warn().Err(err).Msg("Failed to save portal after disabling tools")
			}
			if err := oc.BroadcastRoomState(ctx, portal); err != nil {
				oc.log.Warn().Err(err).Msg("Failed to broadcast room state after disabling tools")
			}
			oc.sendSystemNotice(ctx, portal, "Tools disabled.")
		default:
			oc.sendSystemNotice(ctx, portal, "Invalid option. Use /tools on or /tools off.")
		}
		return true

	case "/mode":
		mode := meta.ConversationMode
		if mode == "" {
			mode = "messages"
		}
		if arg == "" {
			modeHelp := "Conversation modes:\n" +
				"• messages - Build full message history for each request (default)\n" +
				"• responses - Use OpenAI's previous_response_id for context chaining\n\n" +
				"Current mode: " + mode
			oc.sendSystemNotice(ctx, portal, modeHelp)
			return true
		}
		newMode := strings.ToLower(arg)
		if newMode != "messages" && newMode != "responses" {
			oc.sendSystemNotice(ctx, portal, "Invalid mode. Use 'messages' or 'responses'.")
			return true
		}
		meta.ConversationMode = newMode
		if newMode == "messages" {
			meta.LastResponseID = "" // Clear when switching to messages mode
		}
		if err := portal.Save(ctx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to save portal after mode change")
		}
		if err := oc.BroadcastRoomState(ctx, portal); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to broadcast room state after mode change")
		}
		oc.sendSystemNotice(ctx, portal, fmt.Sprintf("Conversation mode set to: %s", newMode))
		return true

	case "/cost":
		// Calculate cost from conversation history
		messages, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, 1000)
		if err != nil {
			oc.sendSystemNotice(ctx, portal, "Failed to retrieve message history for cost calculation.")
			return true
		}
		var totalPromptTokens, totalCompletionTokens int64
		var totalCost float64
		model := oc.effectiveModel(meta)
		for _, msg := range messages {
			msgMeta := messageMeta(msg)
			if msgMeta != nil && msgMeta.Role == "assistant" {
				totalPromptTokens += msgMeta.PromptTokens
				totalCompletionTokens += msgMeta.CompletionTokens
				totalCost += CalculateCost(model, msgMeta.PromptTokens, msgMeta.CompletionTokens)
			}
		}
		costMsg := fmt.Sprintf(
			"Conversation cost (%s):\n"+
				"• Input tokens: %d\n"+
				"• Output tokens: %d\n"+
				"• Estimated cost: %s",
			model,
			totalPromptTokens,
			totalCompletionTokens,
			FormatCost(totalCost),
		)
		oc.sendSystemNotice(ctx, portal, costMsg)
		return true

	case "/help":
		help := "Available commands:\n" +
			"• /model [name] - Get or set the AI model\n" +
			"• /temp [0-2] - Get or set temperature\n" +
			"• /prompt [text] - Get or set system prompt\n" +
			"• /context [1-100] - Get or set context message limit\n" +
			"• /tokens [1-16384] - Get or set max completion tokens\n" +
			"• /tools [on|off] - Enable/disable function calling tools\n" +
			"• /mode [messages|responses] - Set conversation context mode\n" +
			"• /new [model] - Create a new chat (uses current model if none specified)\n" +
			"• /fork [event_id] - Fork conversation to a new chat\n" +
			"• /config - Show current configuration\n" +
			"• /cost - Show conversation token usage and cost\n" +
			"• /regenerate - Regenerate the last response\n" +
			"• /help - Show this help message"
		oc.sendSystemNotice(ctx, portal, help)
		return true

	case "/fork":
		go oc.handleFork(ctx, evt, portal, meta, arg)
		return true

	case "/new":
		go oc.handleNewChat(ctx, evt, portal, meta, arg)
		return true

	case "/regenerate":
		go oc.handleRegenerate(ctx, evt, portal, meta)
		return true
	}

	return false
}

// handleFork creates a new chat and copies messages from the current conversation
func (oc *AIClient) handleFork(
	ctx context.Context,
	evt *event.Event,
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
	evt *event.Event,
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
	// Allocate new chat index
	chatIndex, err := oc.allocateNextChatIndex(ctx)
	if err != nil {
		return nil, nil, err
	}

	slug := formatChatSlug(chatIndex)

	// Generate title
	sourceTitle := sourceMeta.Title
	if sourceTitle == "" {
		sourceTitle = sourcePortal.Name
	}
	title := fmt.Sprintf("%s (Fork)", sourceTitle)

	// Create portal key
	portalKey := portalKeyForChat(oc.UserLogin.ID, slug)

	portal, err := oc.UserLogin.Bridge.GetPortalByKey(ctx, portalKey)
	if err != nil {
		return nil, nil, err
	}

	// Copy configuration from source
	portal.Metadata = &PortalMetadata{
		Model:               sourceMeta.Model,
		Slug:                slug,
		Title:               title,
		SystemPrompt:        sourceMeta.SystemPrompt,
		Temperature:         sourceMeta.Temperature,
		MaxContextMessages:  sourceMeta.MaxContextMessages,
		MaxCompletionTokens: sourceMeta.MaxCompletionTokens,
		ReasoningEffort:     sourceMeta.ReasoningEffort,
		Capabilities:        sourceMeta.Capabilities,
		ToolsEnabled:        sourceMeta.ToolsEnabled,
	}

	portal.RoomType = database.RoomTypeDM
	portal.OtherUserID = modelUserID(sourceMeta.Model)
	portal.Name = title
	portal.NameSet = true
	portal.Topic = sourceMeta.SystemPrompt
	portal.TopicSet = sourceMeta.SystemPrompt != ""

	if err := portal.Save(ctx); err != nil {
		return nil, nil, err
	}

	// Create chat info
	chatInfo := &bridgev2.ChatInfo{
		Name:    ptr.Ptr(title),
		Topic:   ptr.Ptr(sourceMeta.SystemPrompt),
		Type:    ptr.Ptr(database.RoomTypeDM),
		Members: oc.buildChatMembers(sourceMeta.Model),
	}

	return portal, chatInfo, nil
}

// copyMessagesToChat queues messages to be bridged to the new chat
func (oc *AIClient) copyMessagesToChat(
	ctx context.Context,
	destPortal *bridgev2.Portal,
	messages []*database.Message,
) int {
	copiedCount := 0

	for _, srcMsg := range messages {
		srcMeta := messageMeta(srcMsg)
		if srcMeta == nil || srcMeta.Body == "" {
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

	return copiedCount
}

// handleRegenerate regenerates the last AI response
func (oc *AIClient) handleRegenerate(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
) {
	runCtx := oc.backgroundContext(ctx)
	runCtx = oc.log.WithContext(runCtx)

	// Get message history
	history, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(runCtx, portal.PortalKey, 10)
	if err != nil || len(history) == 0 {
		oc.sendSystemNotice(runCtx, portal, "No messages to regenerate from.")
		return
	}

	// Find the last user message
	var lastUserMessage *database.Message
	for _, msg := range history {
		msgMeta := messageMeta(msg)
		if msgMeta != nil && msgMeta.Role == "user" {
			lastUserMessage = msg
			break
		}
	}

	if lastUserMessage == nil {
		oc.sendSystemNotice(runCtx, portal, "No user message found to regenerate from.")
		return
	}

	userMeta := messageMeta(lastUserMessage)
	if userMeta == nil || userMeta.Body == "" {
		oc.sendSystemNotice(runCtx, portal, "Cannot regenerate: message content not available.")
		return
	}

	oc.sendSystemNotice(runCtx, portal, "Regenerating response...")

	// Build prompt excluding the old assistant response
	prompt, err := oc.buildPromptForRegenerate(runCtx, portal, meta, userMeta.Body)
	if err != nil {
		oc.sendSystemNotice(runCtx, portal, "Failed to regenerate: "+err.Error())
		return
	}

	// Dispatch new completion with synchronous room acquisition
	if oc.acquireRoom(portal.MXID) {
		oc.sendSuccessStatus(runCtx, portal, evt)
		go func() {
			defer func() {
				oc.releaseRoom(portal.MXID)
				oc.processNextPending(oc.backgroundContext(runCtx), portal.MXID)
			}()
			oc.dispatchCompletionInternal(runCtx, evt, portal, meta, prompt)
		}()
	} else {
		oc.queuePendingMessage(portal.MXID, pendingMessage{
			Event:       evt,
			Portal:      portal,
			Meta:        meta,
			Type:        pendingTypeRegenerate,
			MessageBody: userMeta.Body,
		})
		oc.sendPendingStatus(runCtx, portal, evt, "Waiting for previous response")
	}
}

// buildPromptForRegenerate builds a prompt for regeneration, excluding the last assistant message
func (oc *AIClient) buildPromptForRegenerate(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	latestUserBody string,
) ([]openai.ChatCompletionMessageParamUnion, error) {
	var prompt []openai.ChatCompletionMessageParamUnion
	systemPrompt := oc.effectivePrompt(meta)
	if systemPrompt != "" {
		prompt = append(prompt, openai.SystemMessage(systemPrompt))
	}

	historyLimit := oc.historyLimit(meta)
	if historyLimit > 0 {
		history, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, historyLimit+2)
		if err != nil {
			return nil, fmt.Errorf("failed to load prompt history: %w", err)
		}

		// Skip the most recent messages (last user and assistant) and build from older history
		skippedUser := false
		skippedAssistant := false
		for _, msg := range history {
			msgMeta := messageMeta(msg)
			// Skip commands and non-conversation messages
			if !shouldIncludeInHistory(msgMeta) {
				continue
			}

			// Skip the last user message and last assistant message
			if !skippedUser && msgMeta.Role == "user" {
				skippedUser = true
				continue
			}
			if !skippedAssistant && msgMeta.Role == "assistant" {
				skippedAssistant = true
				continue
			}

			switch msgMeta.Role {
			case "assistant":
				prompt = append(prompt, openai.AssistantMessage(msgMeta.Body))
			default:
				prompt = append(prompt, openai.UserMessage(msgMeta.Body))
			}
		}

		// Reverse to get chronological order
		for i, j := len(prompt)-1, 0; i > j; i, j = i-1, j+1 {
			prompt[i], prompt[j] = prompt[j], prompt[i]
		}
	}

	prompt = append(prompt, openai.UserMessage(latestUserBody))
	return prompt, nil
}

func (oc *AIClient) buildPrompt(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, latest string) ([]openai.ChatCompletionMessageParamUnion, error) {
	prompt, err := oc.buildBasePrompt(ctx, portal, meta)
	if err != nil {
		return nil, err
	}
	prompt = append(prompt, openai.UserMessage(latest))
	return prompt, nil
}

// dispatchCompletionInternal contains the actual completion logic
func (oc *AIClient) dispatchCompletionInternal(
	ctx context.Context,
	sourceEvent *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
) {
	runCtx := oc.backgroundContext(ctx)
	runCtx = oc.log.WithContext(runCtx)

	// Always use streaming responses
	oc.streamingResponseWithRetry(runCtx, sourceEvent, portal, meta, prompt)
}

// streamingResponseWithRetry handles Responses API streaming with automatic retry on context overflow
// responseFunc is the signature for response handlers that can be retried on context length errors
type responseFunc func(ctx context.Context, evt *event.Event, portal *bridgev2.Portal, meta *PortalMetadata, prompt []openai.ChatCompletionMessageParamUnion) (bool, *ContextLengthError)

// responseWithRetry wraps a response function with context length retry logic
func (oc *AIClient) responseWithRetry(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
	responseFn responseFunc,
	logLabel string,
) {
	currentPrompt := prompt

	for attempt := range maxRetryAttempts {
		success, cle := responseFn(ctx, evt, portal, meta, currentPrompt)
		if success {
			return
		}

		// If we got a context length error, try to truncate and retry
		if cle != nil {
			truncated := oc.truncatePrompt(currentPrompt)
			if len(truncated) <= 2 {
				oc.notifyContextLengthExceeded(ctx, portal, cle, false)
				return
			}

			oc.notifyContextLengthExceeded(ctx, portal, cle, true)
			currentPrompt = truncated

			oc.log.Debug().
				Int("attempt", attempt+1).
				Int("new_prompt_len", len(currentPrompt)).
				Str("log_label", logLabel).
				Msg("Retrying Responses API with truncated context")
			continue
		}

		// Non-context error, already handled in responseFn
		return
	}

	oc.notifyMatrixSendFailure(ctx, portal, evt,
		fmt.Errorf("exceeded retry attempts for context length"))
}

func (oc *AIClient) streamingResponseWithRetry(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
) {
	oc.responseWithRetry(ctx, evt, portal, meta, prompt, oc.streamingResponse, "streaming")
}

func (oc *AIClient) notifyMatrixSendFailure(ctx context.Context, portal *bridgev2.Portal, evt *event.Event, err error) {
	if portal == nil || portal.Bridge == nil || evt == nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to send message via OpenAI")
		return
	}
	status := bridgev2.WrapErrorInStatus(err).
		WithStatus(event.MessageStatusRetriable).
		WithMessage("Failed to reach OpenAI").
		WithIsCertain(true).
		WithSendNotice(true)
	portal.Bridge.Matrix.SendMessageStatus(ctx, &status, bridgev2.StatusEventInfoFromEvent(evt))
}

// sendPendingStatus sends a PENDING status for a message that is queued
func (oc *AIClient) sendPendingStatus(ctx context.Context, portal *bridgev2.Portal, evt *event.Event, message string) {
	if portal == nil || portal.Bridge == nil || evt == nil {
		return
	}
	status := bridgev2.MessageStatus{
		Status:    event.MessageStatusPending,
		Message:   message,
		IsCertain: true,
	}
	portal.Bridge.Matrix.SendMessageStatus(ctx, &status, bridgev2.StatusEventInfoFromEvent(evt))
}

// sendSuccessStatus sends a SUCCESS status for a message that was previously pending
func (oc *AIClient) sendSuccessStatus(ctx context.Context, portal *bridgev2.Portal, evt *event.Event) {
	if portal == nil || portal.Bridge == nil || evt == nil {
		return
	}
	status := bridgev2.MessageStatus{
		Status:    event.MessageStatusSuccess,
		IsCertain: true,
	}
	portal.Bridge.Matrix.SendMessageStatus(ctx, &status, bridgev2.StatusEventInfoFromEvent(evt))
}

// setModelTyping sets the typing indicator for the current model's ghost user
func (oc *AIClient) setModelTyping(ctx context.Context, portal *bridgev2.Portal, typing bool) {
	if portal == nil || portal.MXID == "" {
		return
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return
	}
	var timeout time.Duration
	if typing {
		timeout = 30 * time.Second
	} else {
		timeout = 0 // Zero timeout stops typing
	}
	if err := intent.MarkTyping(ctx, portal.MXID, bridgev2.TypingTypeText, timeout); err != nil {
		oc.log.Warn().Err(err).Bool("typing", typing).Msg("Failed to set typing indicator")
	}
}

// emitStreamEvent sends a streaming delta event to the room
// Uses Matrix-spec compliant m.relates_to to correlate with the initial message
// contentType identifies what kind of content this is (text, reasoning, tool_call, tool_result)
// seq is a sequence number for ordering events
// metadata contains optional fields like tool_name, item_id, status
func (oc *AIClient) emitStreamEvent(ctx context.Context, portal *bridgev2.Portal, relatedEventID id.EventID, contentType StreamContentType, delta string, seq int, metadata map[string]any) {
	if portal == nil || portal.MXID == "" {
		return
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return
	}
	eventContent := &event.Content{
		Raw: map[string]any{
			"body":         delta,
			"content_type": string(contentType),
			"seq":          seq,
			"m.relates_to": map[string]any{
				"rel_type": "m.reference",
				"event_id": relatedEventID.String(),
			},
		},
	}
	// Merge optional metadata (tool_name, item_id, status, etc.)
	maps.Copy(eventContent.Raw, metadata)
	if _, err := intent.SendMessage(ctx, portal.MXID, StreamTokenEventType, eventContent, nil); err != nil {
		oc.log.Warn().Err(err).Stringer("related_event_id", relatedEventID).Str("content_type", string(contentType)).Int("seq", seq).Msg("Failed to emit stream event")
	}
}

// executeBuiltinTool finds and executes a builtin tool by name
func (oc *AIClient) executeBuiltinTool(ctx context.Context, toolName string, argsJSON string) (string, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid tool arguments: %w", err)
	}

	for _, tool := range BuiltinTools() {
		if tool.Name == toolName {
			return tool.Execute(ctx, args)
		}
	}
	return "", fmt.Errorf("unknown tool: %s", toolName)
}

// buildResponsesAPIParams creates common Responses API parameters for both streaming and non-streaming paths
func (oc *AIClient) buildResponsesAPIParams(ctx context.Context, meta *PortalMetadata, messages []openai.ChatCompletionMessageParamUnion) responses.ResponseNewParams {
	log := zerolog.Ctx(ctx)

	params := responses.ResponseNewParams{
		Model:           shared.ResponsesModel(oc.effectiveModelForAPI(meta)),
		MaxOutputTokens: openai.Int(int64(oc.effectiveMaxTokens(meta))),
	}

	// Use previous_response_id if in "responses" mode and ID exists
	if meta.ConversationMode == "responses" && meta.LastResponseID != "" {
		params.PreviousResponseID = openai.String(meta.LastResponseID)
		// Still need to pass the latest user message as input
		if len(messages) > 0 {
			latestMsg := messages[len(messages)-1]
			input := oc.convertToResponsesInput([]openai.ChatCompletionMessageParamUnion{latestMsg}, meta)
			params.Input = responses.ResponseNewParamsInputUnion{
				OfInputItemList: input,
			}
		}
		log.Debug().Str("previous_response_id", meta.LastResponseID).Msg("Using previous_response_id for context")
	} else {
		// Build full message history
		input := oc.convertToResponsesInput(messages, meta)
		params.Input = responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		}
	}

	// Add reasoning effort if configured
	if meta.ReasoningEffort != "" {
		params.Reasoning = shared.ReasoningParam{
			Effort: shared.ReasoningEffort(meta.ReasoningEffort),
		}
	}

	// Add built-in tools if enabled
	if meta.WebSearchEnabled {
		params.Tools = append(params.Tools, responses.ToolParamOfWebSearchPreview(responses.WebSearchPreviewToolTypeWebSearchPreview))
		log.Debug().Msg("Web search tool enabled")
	}
	if meta.CodeInterpreterEnabled {
		params.Tools = append(params.Tools, responses.ToolParamOfCodeInterpreter("auto"))
		log.Debug().Msg("Code interpreter tool enabled")
	}

	return params
}

// streamingResponse handles streaming using the Responses API
// This is the preferred streaming method as it supports reasoning tokens
// Returns (success, contextLengthError)
func (oc *AIClient) streamingResponse(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	messages []openai.ChatCompletionMessageParamUnion,
) (bool, *ContextLengthError) {
	log := zerolog.Ctx(ctx).With().
		Str("portal_id", string(portal.ID)).
		Logger()

	// Set typing indicator when streaming starts
	oc.setModelTyping(ctx, portal, true)
	defer oc.setModelTyping(ctx, portal, false)

	// Build Responses API params using shared helper
	params := oc.buildResponsesAPIParams(ctx, meta, messages)

	stream := oc.api.Responses.NewStreaming(ctx, params)
	if stream == nil {
		log.Error().Msg("Failed to create Responses API streaming request")
		oc.notifyMatrixSendFailure(ctx, portal, evt, fmt.Errorf("responses streaming not available"))
		return false, nil
	}

	// Track streaming state
	var (
		accumulated    strings.Builder
		reasoning      strings.Builder
		firstToken     = true
		initialEventID id.EventID
		finishReason   string
		sequenceNum    int // Global sequence number for ordering all stream events
	)

	// Process stream events - no debouncing, stream every delta immediately
	for stream.Next() {
		streamEvent := stream.Current()

		switch streamEvent.Type {
		case "response.output_text.delta":
			accumulated.WriteString(streamEvent.Delta)

			// First token - send initial message synchronously to capture event_id
			if firstToken && accumulated.Len() > 0 {
				firstToken = false
				initialEventID = oc.sendInitialStreamMessage(ctx, portal, accumulated.String())
				if initialEventID == "" {
					log.Error().Msg("Failed to send initial streaming message")
					return false, nil
				}
			}

			// Stream every text delta immediately (no debouncing for snappy UX)
			if !firstToken && initialEventID != "" {
				sequenceNum++
				oc.emitStreamEvent(ctx, portal, initialEventID, StreamContentText, streamEvent.Delta, sequenceNum, nil)
			}

		case "response.reasoning_text.delta":
			reasoning.WriteString(streamEvent.Delta)
			// Stream reasoning tokens in real-time too
			if !firstToken && initialEventID != "" {
				sequenceNum++
				oc.emitStreamEvent(ctx, portal, initialEventID, StreamContentReasoning, streamEvent.Delta, sequenceNum, nil)
			}

		case "response.function_call_arguments.delta":
			// Stream function call arguments as they arrive
			if initialEventID != "" {
				sequenceNum++
				oc.emitStreamEvent(ctx, portal, initialEventID, StreamContentToolCall, streamEvent.Delta, sequenceNum, map[string]any{
					"tool_name": streamEvent.Name,
					"item_id":   streamEvent.ItemID,
					"status":    "streaming",
				})
			}

		case "response.function_call_arguments.done":
			// Function call complete - execute the tool and stream result
			if initialEventID != "" && meta.ToolsEnabled {
				result, err := oc.executeBuiltinTool(ctx, streamEvent.Name, streamEvent.Arguments)
				if err != nil {
					log.Warn().Err(err).Str("tool", streamEvent.Name).Msg("Tool execution failed")
					result = fmt.Sprintf("Error: %s", err.Error())
				}
				sequenceNum++
				oc.emitStreamEvent(ctx, portal, initialEventID, StreamContentToolResult, result, sequenceNum, map[string]any{
					"tool_name": streamEvent.Name,
					"item_id":   streamEvent.ItemID,
					"status":    "completed",
				})
			}

		case "response.web_search_call.searching":
			// Web search starting
			if initialEventID != "" {
				sequenceNum++
				oc.emitStreamEvent(ctx, portal, initialEventID, StreamContentToolCall, "", sequenceNum, map[string]any{
					"tool_name": "web_search",
					"item_id":   streamEvent.ItemID,
					"status":    "searching",
				})
			}

		case "response.web_search_call.completed":
			// Web search completed
			if initialEventID != "" {
				sequenceNum++
				oc.emitStreamEvent(ctx, portal, initialEventID, StreamContentToolResult, "", sequenceNum, map[string]any{
					"tool_name": "web_search",
					"item_id":   streamEvent.ItemID,
					"status":    "completed",
				})
			}

		case "response.completed":
			if streamEvent.Response.Status == "completed" {
				finishReason = "stop"
			} else {
				finishReason = string(streamEvent.Response.Status)
			}
			// Store response ID for "responses" mode context chaining
			if streamEvent.Response.ID != "" && meta.ConversationMode == "responses" {
				meta.LastResponseID = streamEvent.Response.ID
				if err := portal.Save(ctx); err != nil {
					log.Warn().Err(err).Msg("Failed to save portal after storing response ID")
				}
			}
			log.Debug().Str("reason", finishReason).Str("response_id", streamEvent.Response.ID).Msg("Response stream completed")

		case "error":
			log.Error().Str("error", streamEvent.Message).Msg("Responses API stream error")
			// Check for context length error
			if strings.Contains(streamEvent.Message, "context_length") || strings.Contains(streamEvent.Message, "token") {
				return false, &ContextLengthError{
					OriginalError: fmt.Errorf("%s", streamEvent.Message),
				}
			}
			oc.notifyMatrixSendFailure(ctx, portal, evt, fmt.Errorf("API error: %s", streamEvent.Message))
			return false, nil
		}
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		log.Error().Err(err).Msg("Responses API streaming error")
		cle := ParseContextLengthError(err)
		if cle != nil {
			return false, cle
		}
		oc.notifyMatrixSendFailure(ctx, portal, evt, err)
		return false, nil
	}

	// Send final edit to persist complete content with metadata (including reasoning)
	if initialEventID != "" {
		oc.sendFinalEditWithReasoning(ctx, portal, initialEventID, accumulated.String(), reasoning.String(), meta, finishReason)
	}

	log.Info().
		Str("finish_reason", finishReason).
		Int("content_length", accumulated.Len()).
		Int("reasoning_length", reasoning.Len()).
		Msg("Responses API streaming finished")

	// Generate room title after first response
	oc.maybeGenerateTitle(ctx, portal, accumulated.String())

	return true, nil
}

// convertToResponsesInput converts Chat Completion messages to Responses API input items
func (oc *AIClient) convertToResponsesInput(messages []openai.ChatCompletionMessageParamUnion, _ *PortalMetadata) responses.ResponseInputParam {
	var input responses.ResponseInputParam

	for _, msg := range messages {
		// Use shared helper to extract content and role (avoids JSON roundtrip)
		content, role := extractMessageContent(msg)
		if role == "" || content == "" {
			continue
		}

		// Map Chat Completions role to Responses API role
		var responsesRole responses.EasyInputMessageRole
		switch role {
		case "system":
			responsesRole = responses.EasyInputMessageRoleSystem
		case "user":
			responsesRole = responses.EasyInputMessageRoleUser
		case "assistant":
			responsesRole = responses.EasyInputMessageRoleAssistant
		default:
			responsesRole = responses.EasyInputMessageRoleUser
		}

		input = append(input, responses.ResponseInputItemUnionParam{
			OfMessage: &responses.EasyInputMessageParam{
				Role: responsesRole,
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: openai.String(content),
				},
			},
		})
	}

	return input
}

// sendInitialStreamMessage sends the first message in a streaming session and returns its event ID
func (oc *AIClient) sendInitialStreamMessage(ctx context.Context, portal *bridgev2.Portal, content string) id.EventID {
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return ""
	}

	eventContent := &event.Content{
		Raw: map[string]any{
			"msgtype": "m.text",
			"body":    content,
		},
	}
	resp, err := intent.SendMessage(ctx, portal.MXID, event.EventMessage, eventContent, nil)
	if err != nil {
		oc.log.Error().Err(err).Msg("Failed to send initial streaming message")
		return ""
	}
	oc.log.Info().Stringer("event_id", resp.EventID).Msg("Initial streaming message sent")
	return resp.EventID
}

// sendFinalEditWithReasoning sends an edit event including reasoning/thinking content
func (oc *AIClient) sendFinalEditWithReasoning(ctx context.Context, portal *bridgev2.Portal, initialEventID id.EventID, content string, reasoning string, meta *PortalMetadata, finishReason string) {
	if portal == nil || portal.MXID == "" {
		return
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return
	}

	// Build AI metadata for rich UI rendering
	aiMetadata := map[string]any{
		"model":         oc.effectiveModel(meta),
		"finish_reason": finishReason,
	}

	// Include reasoning/thinking if present
	if reasoning != "" {
		aiMetadata["thinking"] = reasoning
	}

	// Send edit event with m.replace relation and m.new_content
	eventContent := &event.Content{
		Raw: map[string]any{
			"msgtype": "m.text",
			"body":    "* " + content, // Fallback with edit marker
			"m.new_content": map[string]any{
				"msgtype": "m.text",
				"body":    content,
			},
			"m.relates_to": map[string]any{
				"rel_type": "m.replace",
				"event_id": initialEventID.String(),
			},
			"com.beeper.ai":                 aiMetadata,
			"com.beeper.dont_render_edited": true, // Don't show "edited" indicator for streaming updates
		},
	}

	if _, err := intent.SendMessage(ctx, portal.MXID, event.EventMessage, eventContent, nil); err != nil {
		oc.log.Warn().Err(err).Stringer("initial_event_id", initialEventID).Msg("Failed to send final edit")
	} else {
		oc.log.Debug().
			Str("initial_event_id", initialEventID.String()).
			Bool("has_reasoning", reasoning != "").
			Msg("Sent final edit with metadata")
	}
}

func (oc *AIClient) backgroundContext(ctx context.Context) context.Context {
	if oc.UserLogin != nil && oc.UserLogin.Bridge != nil && oc.UserLogin.Bridge.BackgroundCtx != nil {
		return oc.UserLogin.Bridge.BackgroundCtx
	}
	if ctx == nil || ctx.Err() != nil {
		return context.Background()
	}
	return ctx
}

func (oc *AIClient) sendWelcomeMessage(ctx context.Context, portal *bridgev2.Portal) {
	meta := portalMeta(portal)
	if meta.WelcomeSent {
		return
	}
	modelID := oc.effectiveModel(meta)
	modelName := FormatModelDisplay(modelID)
	body := fmt.Sprintf("This chat was created automatically. Send a message to start talking to %s.", modelName)
	event := &OpenAIRemoteMessage{
		PortalKey: portal.PortalKey,
		ID:        networkid.MessageID(fmt.Sprintf("openai:welcome:%s", uuid.NewString())),
		Sender: bridgev2.EventSender{
			Sender:      modelUserID(modelID),
			ForceDMUser: true,
			SenderLogin: oc.UserLogin.ID,
			IsFromMe:    false,
		},
		Content:   body,
		Timestamp: time.Now(),
		Metadata: &MessageMetadata{
			Role: "assistant",
			Body: body,
		},
	}
	oc.UserLogin.QueueRemoteEvent(event)
	meta.WelcomeSent = true
	if err := portal.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to persist welcome message state")
	}
}

// effectiveModel returns the full prefixed model ID (e.g., "openai/gpt-5.2")
// Used for routing and display purposes
func (oc *AIClient) effectiveModel(meta *PortalMetadata) string {
	if meta != nil && meta.Model != "" {
		return meta.Model
	}
	return oc.defaultModelForProvider()
}

// effectiveModelForAPI returns the actual model name to send to the API
// For OpenRouter/Beeper, returns the full model ID (e.g., "openai/gpt-5.2")
// For direct providers, strips the prefix (e.g., "openai/gpt-5.2" → "gpt-5.2")
func (oc *AIClient) effectiveModelForAPI(meta *PortalMetadata) string {
	modelID := oc.effectiveModel(meta)

	// OpenRouter and Beeper route through a gateway that expects the full model ID
	loginMeta := loginMetadata(oc.UserLogin)
	if loginMeta.Provider == ProviderOpenRouter || loginMeta.Provider == ProviderBeeper {
		return modelID
	}

	// Direct OpenAI provider needs the prefix stripped
	_, actualModel := ParseModelPrefix(modelID)
	return actualModel
}

// defaultModelForProvider returns the configured default model for this login's provider
func (oc *AIClient) defaultModelForProvider() string {
	loginMeta := loginMetadata(oc.UserLogin)
	providers := oc.connector.Config.Providers

	switch loginMeta.Provider {
	case ProviderOpenAI:
		if providers.OpenAI.DefaultModel != "" {
			return providers.OpenAI.DefaultModel
		}
		return DefaultModelOpenAI
	case ProviderOpenRouter:
		if providers.OpenRouter.DefaultModel != "" {
			return providers.OpenRouter.DefaultModel
		}
		return DefaultModelOpenRouter
	case ProviderBeeper:
		if providers.Beeper.DefaultModel != "" {
			return providers.Beeper.DefaultModel
		}
		return DefaultModelBeeper
	default:
		return DefaultModelOpenAI
	}
}

func (oc *AIClient) effectivePrompt(meta *PortalMetadata) string {
	if meta != nil && meta.SystemPrompt != "" {
		return meta.SystemPrompt
	}
	return oc.connector.Config.DefaultSystemPrompt
}

func (oc *AIClient) effectiveTemperature(meta *PortalMetadata) float64 {
	if meta != nil && meta.Temperature > 0 {
		return meta.Temperature
	}
	return defaultTemperature
}

func (oc *AIClient) historyLimit(meta *PortalMetadata) int {
	if meta != nil && meta.MaxContextMessages > 0 {
		return meta.MaxContextMessages
	}
	return defaultMaxContextMessages
}

func (oc *AIClient) effectiveMaxTokens(meta *PortalMetadata) int {
	if meta != nil && meta.MaxCompletionTokens > 0 {
		return meta.MaxCompletionTokens
	}
	return defaultMaxTokens
}

// validateModel checks if a model is available for this user
func (oc *AIClient) validateModel(ctx context.Context, modelID string) (bool, error) {
	if modelID == "" {
		return true, nil
	}

	// First check local model cache
	models, err := oc.listAvailableModels(ctx, false)
	if err == nil {
		for _, model := range models {
			if model.ID == modelID {
				return true, nil
			}
		}
	}

	// Check against OpenRouter's model list (cached)
	_, actualModel := ParseModelPrefix(modelID)
	if IsValidOpenRouterModel(actualModel) {
		return true, nil
	}

	// Try to validate by making a minimal API call as last resort
	timeoutCtx, cancel := context.WithTimeout(ctx, modelValidationTimeout)
	defer cancel()

	_, err = oc.api.Models.Get(timeoutCtx, actualModel)
	return err == nil, nil
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
		meta.Capabilities = getModelCapabilities(config.Model)
	}
	if config.SystemPrompt != "" {
		meta.SystemPrompt = config.SystemPrompt
	}
	if config.Temperature > 0 {
		meta.Temperature = config.Temperature
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
	// Boolean fields - always apply
	meta.ToolsEnabled = config.ToolsEnabled
	meta.WebSearchEnabled = config.WebSearchEnabled
	meta.FileSearchEnabled = config.FileSearchEnabled
	meta.CodeInterpreterEnabled = config.CodeInterpreterEnabled

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

// getModelIntent returns the Matrix intent for the current model's ghost in the portal, or nil if unavailable.
// Uses the portal's model configuration to determine which model ghost to use.
func (oc *AIClient) getModelIntent(ctx context.Context, portal *bridgev2.Portal) bridgev2.MatrixAPI {
	meta := portalMeta(portal)
	modelID := oc.effectiveModel(meta)
	ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, modelUserID(modelID))
	if err != nil {
		oc.log.Warn().Err(err).Str("model", modelID).Msg("Failed to get model ghost")
		return nil
	}
	return ghost.Intent
}

const (
	maxRetryAttempts        = 3                 // Maximum retry attempts for context length errors
	defaultHistoryLookup    = 50                // Default number of messages to include in history
	forkMessageLimit        = 10000             // Maximum messages to consider when forking
	modelValidationTimeout  = 5 * time.Second   // Timeout for model validation API calls
	maxImageSize            = 20 * 1024 * 1024  // Maximum image size (20MB)
)

// notifyContextLengthExceeded sends a user-friendly notice about context overflow
func (oc *AIClient) notifyContextLengthExceeded(
	ctx context.Context,
	portal *bridgev2.Portal,
	cle *ContextLengthError,
	willRetry bool,
) {
	var message string
	if willRetry {
		message = fmt.Sprintf(
			"Your conversation exceeded the model's context limit (%d tokens requested, %d max). "+
				"Automatically trimming older messages and retrying...",
			cle.RequestedTokens, cle.ModelMaxTokens,
		)
	} else {
		message = fmt.Sprintf(
			"Your message is too long for this model's context window (%d tokens max). "+
				"Please try a shorter message or start a new conversation.",
			cle.ModelMaxTokens,
		)
	}

	oc.sendSystemNotice(ctx, portal, message)
}

// truncatePrompt removes older messages from the prompt while preserving
// the system message (if any) and the latest user message
func (oc *AIClient) truncatePrompt(
	prompt []openai.ChatCompletionMessageParamUnion,
) []openai.ChatCompletionMessageParamUnion {
	if len(prompt) <= 2 {
		return nil // Can't truncate further
	}

	// Determine if first message is system prompt
	hasSystem := prompt[0].OfSystem != nil

	// Calculate how many history messages to keep
	historyCount := len(prompt)
	startIdx := 0
	if hasSystem {
		historyCount-- // Don't count system
		startIdx = 1
	}
	historyCount-- // Don't count latest user message

	// Remove approximately half of history
	keepCount := max(historyCount/2, 1)

	// Build new prompt: [system] + [last N history] + [latest user]
	var result []openai.ChatCompletionMessageParamUnion

	if hasSystem {
		result = append(result, prompt[0])
	}

	// Keep only the most recent history messages
	historyStart := max(len(prompt)-1-keepCount, startIdx)

	result = append(result, prompt[historyStart:]...)
	return result
}

// listAvailableModels fetches models from OpenAI API and caches them
// Returns ModelInfo list from the provider
func (oc *AIClient) listAvailableModels(ctx context.Context, forceRefresh bool) ([]ModelInfo, error) {
	meta := loginMetadata(oc.UserLogin)

	// Check cache (refresh every 6 hours unless forced)
	if !forceRefresh && meta.ModelCache != nil {
		age := time.Now().Unix() - meta.ModelCache.LastRefresh
		if age < meta.ModelCache.CacheDuration {
			return meta.ModelCache.Models, nil
		}
	}

	// Fetch models from provider
	oc.log.Debug().Msg("Fetching available models from provider")

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var allModels []ModelInfo

	// List models from the provider
	if oc.provider != nil {
		models, err := oc.provider.ListModels(timeoutCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to list models: %w", err)
		}
		allModels = models
	}

	// Update cache
	if meta.ModelCache == nil {
		meta.ModelCache = &ModelCache{
			CacheDuration: int64(oc.connector.Config.ModelCacheDuration.Seconds()),
		}
	}
	meta.ModelCache.Models = allModels
	meta.ModelCache.LastRefresh = time.Now().Unix()

	// Save metadata
	if err := oc.UserLogin.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to save model cache")
	}

	oc.log.Info().Int("count", len(allModels)).Msg("Cached available models")
	return allModels, nil
}

// supportsReasoning checks if a model supports reasoning/thinking capabilities
// These models can use reasoning_effort parameter and stream reasoning tokens via Responses API
func supportsReasoning(modelID string) bool {
	// O-series reasoning models
	if strings.HasPrefix(modelID, "o1") || strings.HasPrefix(modelID, "o3") || strings.HasPrefix(modelID, "o4") {
		return true
	}
	// GPT-5.x models with reasoning support
	if strings.HasPrefix(modelID, "gpt-5") {
		return true
	}
	return false
}

// getModelCapabilities computes capabilities for a model
func getModelCapabilities(modelID string) ModelCapabilities {
	return ModelCapabilities{
		SupportsVision:    detectVisionSupport(modelID),
		SupportsReasoning: supportsReasoning(modelID),
	}
}

// detectVisionSupport checks if a model supports vision/images
func detectVisionSupport(modelID string) bool {
	// Known vision-capable models
	visionModels := map[string]bool{
		"gpt-4o":                    true,
		"gpt-4o-mini":               true,
		"gpt-4-turbo":               true,
		"gpt-4-turbo-2024-04-09":    true,
		"gpt-4-vision-preview":      true,
		"gpt-4-1106-vision-preview": true,
	}

	if visionModels[modelID] {
		return true
	}

	// Check by prefix/contains
	return strings.HasPrefix(modelID, "gpt-4o") ||
		strings.HasPrefix(modelID, "gpt-4-turbo") ||
		strings.Contains(modelID, "vision")
}

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
	for {
		_, err := oc.UserLogin.Bridge.DB.UserLogin.GetByID(ctx, oc.UserLogin.ID)
		if err == nil {
			return
		}
		select {
		case <-ctx.Done():
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
		if idx := parseChatSlug(pm.Slug); idx > maxIdx {
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
	if strings.TrimSpace(title) == "" {
		meta := loginMetadata(oc.UserLogin)
		next := meta.NextChatIndex + 1
		title = fmt.Sprintf("ChatGPT %d", next)
	}
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
	oc.chatLock.Lock()
	defer oc.chatLock.Unlock()
	oc.log.Debug().Str("title", title).Msg("Allocating portal for new chat")

	// Get default model for new chats
	defaultModelID := oc.effectiveModel(nil)

	meta := loginMetadata(oc.UserLogin)
	meta.NextChatIndex++
	index := meta.NextChatIndex
	slug := formatChatSlug(index)
	if title == "" {
		modelName := FormatModelDisplay(defaultModelID)
		title = fmt.Sprintf("%s %d", modelName, index)
	}
	key := portalKeyForChat(oc.UserLogin.ID, slug)
	portal, err := oc.UserLogin.Bridge.GetPortalByKey(ctx, key)
	if err != nil {
		meta.NextChatIndex--
		return nil, nil, err
	}
	pmeta := portalMeta(portal)
	pmeta.Slug = slug
	pmeta.Title = title
	pmeta.Model = defaultModelID
	pmeta.Capabilities = getModelCapabilities(defaultModelID)
	if systemPrompt != "" {
		pmeta.SystemPrompt = systemPrompt
	}
	portal.RoomType = database.RoomTypeDM
	portal.OtherUserID = modelUserID(defaultModelID)
	portal.Name = title
	portal.NameSet = true
	portal.Topic = systemPrompt
	portal.TopicSet = systemPrompt != ""
	if err := portal.Save(ctx); err != nil {
		meta.NextChatIndex--
		return nil, nil, err
	}
	if err := oc.UserLogin.Save(ctx); err != nil {
		return nil, nil, err
	}
	info := oc.composeChatInfo(title, systemPrompt, defaultModelID)
	return portal, info, nil
}

// createNewChatWithModel creates a new chat portal with the specified model and default settings
func (oc *AIClient) createNewChatWithModel(ctx context.Context, modelID string) (*bridgev2.Portal, *bridgev2.ChatInfo, error) {
	chatIndex, err := oc.allocateNextChatIndex(ctx)
	if err != nil {
		return nil, nil, err
	}

	slug := formatChatSlug(chatIndex)
	modelName := FormatModelDisplay(modelID)
	title := fmt.Sprintf("%s %d", modelName, chatIndex)

	key := portalKeyForChat(oc.UserLogin.ID, slug)
	portal, err := oc.UserLogin.Bridge.GetPortalByKey(ctx, key)
	if err != nil {
		return nil, nil, err
	}

	pmeta := portalMeta(portal)
	pmeta.Slug = slug
	pmeta.Title = title
	pmeta.Model = modelID
	pmeta.Capabilities = getModelCapabilities(modelID)

	portal.RoomType = database.RoomTypeDM
	portal.OtherUserID = modelUserID(modelID)
	portal.Name = title
	portal.NameSet = true

	if err := portal.Save(ctx); err != nil {
		return nil, nil, err
	}

	info := oc.composeChatInfo(title, "", modelID) // No default system prompt
	return portal, info, nil
}

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

// BroadcastRoomState sends current room config to Matrix room state
func (oc *AIClient) BroadcastRoomState(ctx context.Context, portal *bridgev2.Portal) error {
	if portal.MXID == "" {
		return fmt.Errorf("portal has no Matrix room ID")
	}

	meta := portalMeta(portal)

	stateContent := &RoomConfigEventContent{
		Model:               meta.Model,
		SystemPrompt:        meta.SystemPrompt,
		Temperature:         meta.Temperature,
		MaxContextMessages:  meta.MaxContextMessages,
		MaxCompletionTokens: meta.MaxCompletionTokens,
		ReasoningEffort:     meta.ReasoningEffort,
		ToolsEnabled:        meta.ToolsEnabled,
		ConversationMode:    meta.ConversationMode,
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

func ptrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return ptr.Ptr(value)
}

// generateRoomTitle asks the model to generate a short descriptive title for the conversation
func (oc *AIClient) generateRoomTitle(ctx context.Context, userMessage, assistantResponse string) (string, error) {
	model := oc.effectiveModelForAPI(nil)
	// Use a faster/cheaper model for title generation if available
	if strings.HasPrefix(model, "gpt-4") {
		model = "gpt-4o-mini"
	}

	// Build Responses API input
	input := responses.ResponseInputParam{
		{
			OfMessage: &responses.EasyInputMessageParam{
				Role: responses.EasyInputMessageRoleUser,
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: openai.String(fmt.Sprintf("User: %s\n\nAssistant: %s", userMessage, assistantResponse)),
				},
			},
		},
	}

	resp, err := oc.api.Responses.New(ctx, responses.ResponseNewParams{
		Model:           model,
		Input:           responses.ResponseNewParamsInputUnion{OfInputItemList: input},
		Instructions:    openai.String("Generate a very short title (3-5 words max) that summarizes this conversation. Reply with ONLY the title, no quotes, no punctuation at the end."),
		MaxOutputTokens: openai.Int(20),
	})
	if err != nil {
		return "", err
	}

	// Extract text from response
	var title string
	for _, item := range resp.Output {
		if msg, ok := item.AsAny().(responses.ResponseOutputMessage); ok {
			for _, contentPart := range msg.Content {
				if text, ok := contentPart.AsAny().(responses.ResponseOutputText); ok {
					title = text.Text
					break
				}
			}
		}
	}

	if title == "" {
		return "", fmt.Errorf("no response from model")
	}

	title = strings.TrimSpace(title)
	// Remove quotes if the model added them
	title = strings.Trim(title, "\"'")
	// Limit length
	if len(title) > 50 {
		title = title[:50]
	}
	return title, nil
}

// setRoomName sets the Matrix room name via m.room.name state event
func (oc *AIClient) setRoomName(ctx context.Context, portal *bridgev2.Portal, name string) error {
	if portal.MXID == "" {
		return fmt.Errorf("portal has no Matrix room ID")
	}

	bot := oc.UserLogin.Bridge.Bot
	_, err := bot.SendState(ctx, portal.MXID, event.StateRoomName, "", &event.Content{
		Parsed: &event.RoomNameEventContent{Name: name},
	}, time.Time{})

	if err != nil {
		return fmt.Errorf("failed to set room name: %w", err)
	}

	// Update portal metadata
	meta := portalMeta(portal)
	meta.Title = name
	meta.TitleGenerated = true
	if err := portal.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to save portal after setting room name")
	}

	oc.log.Debug().Str("name", name).Msg("Set Matrix room name")
	return nil
}

// maybeGenerateTitle generates a title for the room after the first exchange
func (oc *AIClient) maybeGenerateTitle(ctx context.Context, portal *bridgev2.Portal, assistantResponse string) {
	meta := portalMeta(portal)

	// Skip if title was already generated
	if meta.TitleGenerated {
		return
	}

	// Generate title in background to not block the message flow
	go func() {
		bgCtx := oc.backgroundContext(ctx)

		// Fetch the last user message from database
		messages, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(bgCtx, portal.PortalKey, 10)
		if err != nil {
			oc.log.Warn().Err(err).Msg("Failed to get messages for title generation")
			return
		}

		var userMessage string
		for _, msg := range messages {
			msgMeta, ok := msg.Metadata.(*MessageMetadata)
			if ok && msgMeta != nil && msgMeta.Role == "user" && msgMeta.Body != "" {
				userMessage = msgMeta.Body
				break
			}
		}

		if userMessage == "" {
			oc.log.Debug().Msg("No user message found for title generation")
			return
		}

		title, err := oc.generateRoomTitle(bgCtx, userMessage, assistantResponse)
		if err != nil {
			oc.log.Warn().Err(err).Msg("Failed to generate room title")
			return
		}

		if title == "" {
			return
		}

		if err := oc.setRoomName(bgCtx, portal, title); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to set room name")
		}
	}()
}
