package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var (
	_ bridgev2.NetworkAPI                    = (*OpenAIClient)(nil)
	_ bridgev2.IdentifierResolvingNetworkAPI = (*OpenAIClient)(nil)
	_ bridgev2.ContactListingNetworkAPI      = (*OpenAIClient)(nil)
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

type OpenAIClient struct {
	UserLogin *bridgev2.UserLogin
	connector *OpenAIConnector
	api       openai.Client
	apiKey    string
	log       zerolog.Logger

	loggedIn atomic.Bool
	chatLock sync.Mutex
}

func newOpenAIClient(login *bridgev2.UserLogin, connector *OpenAIConnector, apiKey string) (*OpenAIClient, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, fmt.Errorf("missing OpenAI API key")
	}

	// Get per-user credentials from login metadata
	meta := login.Metadata.(*UserLoginMetadata)

	opts := []option.RequestOption{
		option.WithAPIKey(key),
	}

	// Use per-user base_url if provided, otherwise default to OpenAI's endpoint
	baseURL := strings.TrimSpace(meta.BaseURL)
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	opts = append(opts, option.WithBaseURL(baseURL))

	client := openai.NewClient(opts...)
	return &OpenAIClient{
		UserLogin: login,
		connector: connector,
		api:       client,
		apiKey:    key,
		log:       login.Log.With().Str("component", "openai-network").Logger(),
	}, nil
}

func (oc *OpenAIClient) Connect(ctx context.Context) {
	// Use a default model for validation (any model works to verify credentials)
	model := "gpt-4o-mini"
	timeoutCtx, cancel := context.WithTimeout(ctx, oc.connector.Config.OpenAI.RequestTimeout)
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

func (oc *OpenAIClient) Disconnect() {
	oc.loggedIn.Store(false)
}

func (oc *OpenAIClient) IsLoggedIn() bool {
	return oc.loggedIn.Load()
}

func (oc *OpenAIClient) LogoutRemote(ctx context.Context) {
	oc.Disconnect()
	oc.UserLogin.BridgeState.Send(status.BridgeState{
		StateEvent: status.StateLoggedOut,
		Message:    "Disconnected by user",
	})
}

func (oc *OpenAIClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	return userID == humanUserID(oc.UserLogin.ID)
}

func (oc *OpenAIClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	meta := portalMeta(portal)
	title := meta.Title
	if title == "" {
		if portal.Name != "" {
			title = portal.Name
		} else {
			title = "ChatGPT"
		}
	}
	prompt := meta.SystemPrompt
	if prompt == "" {
		prompt = oc.connector.Config.OpenAI.SystemPrompt
	}
	return &bridgev2.ChatInfo{
		Name:  ptr.Ptr(title),
		Topic: ptrIfNotEmpty(prompt),
	}, nil
}

func (oc *OpenAIClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	name := ptr.Ptr("ChatGPT")
	isBot := ptr.Ptr(true)
	return &bridgev2.UserInfo{
		Name:  name,
		IsBot: isBot,
	}, nil
}

func (oc *OpenAIClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *event.RoomFeatures {
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
		TypingNotifications: oc.connector.Config.Bridge.TypingNotifications,
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

func (oc *OpenAIClient) GetContactList(ctx context.Context) ([]*bridgev2.ResolveIdentifierResponse, error) {
	oc.log.Debug().Msg("Contact list requested")

	// Fetch available models (use cache if available)
	models, err := oc.listAvailableModels(ctx, false)
	if err != nil {
		oc.log.Error().Err(err).Msg("Failed to list models, using fallback")
		// Return default model as fallback (create minimal Model struct)
		models = []openai.Model{{
			ID:      "gpt-4o-mini",
			Created: time.Now().Unix(),
			OwnedBy: "openai",
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
		displayName := formatModelName(model.ID)
		if caps.SupportsVision {
			displayName += " (Vision)"
		}

		contacts = append(contacts, &bridgev2.ResolveIdentifierResponse{
			UserID: userID,
			UserInfo: &bridgev2.UserInfo{
				Name:        ptr.Ptr(displayName),
				IsBot:       ptr.Ptr(true),
				Identifiers: []string{model.ID},
			},
			Ghost: ghost,
			// Chat will be created on-demand via ResolveIdentifier
		})
	}

	oc.log.Info().Int("count", len(contacts)).Msg("Returning model contact list")
	return contacts, nil
}

func (oc *OpenAIClient) ResolveIdentifier(ctx context.Context, identifier string, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
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
			Name:        ptr.Ptr(formatModelName(modelID)),
			IsBot:       ptr.Ptr(true),
			Identifiers: []string{modelID},
		},
		Ghost: ghost,
		Chat:  chatResp,
	}, nil
}

// createNewChat creates a new portal for a specific model
func (oc *OpenAIClient) createNewChat(ctx context.Context, modelID string, caps ModelCapabilities) (*bridgev2.CreateChatResponse, error) {
	chatIndex, err := oc.allocateNextChatIndex(ctx)
	if err != nil {
		return nil, err
	}

	// Create portal metadata with model-specific settings and bridge-wide defaults
	config := oc.connector.Config.OpenAI
	portalMeta := &PortalMetadata{
		Model:               modelID,
		Slug:                formatChatSlug(chatIndex),
		Title:               fmt.Sprintf("%s Chat %d", formatModelName(modelID), chatIndex),
		Capabilities:        caps,
		SystemPrompt:        config.SystemPrompt,        // Bridge default
		Temperature:         config.DefaultTemperature,  // Bridge default
		MaxContextMessages:  config.MaxContextMessages,  // Bridge default
		MaxCompletionTokens: config.MaxCompletionTokens, // Bridge default
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
		Name: ptr.Ptr(portalMeta.Title),
		Topic: ptr.Ptr(fmt.Sprintf("Conversation with %s", modelID)),
		Type: ptr.Ptr(database.RoomTypeDM),
		Members: &bridgev2.ChatMemberList{
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
						Name:  ptr.Ptr(formatModelName(modelID)),
						IsBot: ptr.Ptr(true),
					},
				},
			},
			IsFull:            true,
			TotalMemberCount:  2,
		},
	}

	return &bridgev2.CreateChatResponse{
		PortalKey:  portalKey,
		PortalInfo: chatInfo,
		Portal:     portal,
	}, nil
}

// allocateNextChatIndex increments and returns the next chat index for this login
func (oc *OpenAIClient) allocateNextChatIndex(ctx context.Context) (int, error) {
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

func (oc *OpenAIClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	if msg.Content == nil {
		return nil, fmt.Errorf("missing message content")
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
	portal := msg.Portal
	meta := portalMeta(portal)
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
	// Dispatch completion handling in the background so the Matrix send pipeline can ack immediately.
	go oc.dispatchCompletion(ctx, msg.Event, portal, meta, promptMessages)
	return &bridgev2.MatrixMessageResponse{
		DB: userMessage,
	}, nil
}

func (oc *OpenAIClient) requestCompletion(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, meta *PortalMetadata) (*openai.ChatCompletion, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("prompt had no messages")
	}
	params := openai.ChatCompletionNewParams{
		Model:               shared.ChatModel(oc.effectiveModel(meta)),
		Messages:            messages,
		Temperature:         openai.Float(oc.effectiveTemperature(meta)),
		MaxCompletionTokens: openai.Int(int64(oc.effectiveMaxTokens(meta))),
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, oc.connector.Config.OpenAI.RequestTimeout)
	defer cancel()
	resp, err := oc.api.Chat.Completions.New(timeoutCtx, params)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (oc *OpenAIClient) buildPrompt(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, latest string) ([]openai.ChatCompletionMessageParamUnion, error) {
	var prompt []openai.ChatCompletionMessageParamUnion
	systemPrompt := oc.effectivePrompt(meta)
	if systemPrompt != "" {
		prompt = append(prompt, openai.SystemMessage(systemPrompt))
	}
	historyLimit := oc.historyLimit(meta)
	if historyLimit > 0 {
		history, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, historyLimit)
		if err != nil {
			return nil, fmt.Errorf("failed to load prompt history: %w", err)
		}
		for i := len(history) - 1; i >= 0; i-- {
			meta := messageMeta(history[i])
			if meta == nil || meta.Body == "" {
				continue
			}
			switch meta.Role {
			case "assistant":
				prompt = append(prompt, openai.AssistantMessage(meta.Body))
			default:
				prompt = append(prompt, openai.UserMessage(meta.Body))
			}
		}
	}
	prompt = append(prompt, openai.UserMessage(latest))
	return prompt, nil
}

func (oc *OpenAIClient) queueAssistantMessage(portal *bridgev2.Portal, completion *openai.ChatCompletion) {
	if completion == nil || len(completion.Choices) == 0 {
		return
	}
	choice := completion.Choices[0]
	body := strings.TrimSpace(choice.Message.Content)
	if body == "" {
		return
	}
	meta := &MessageMetadata{
		Role:             "assistant",
		Body:             body,
		CompletionID:     completion.ID,
		FinishReason:     choice.FinishReason,
		PromptTokens:     completion.Usage.PromptTokens,
		CompletionTokens: completion.Usage.CompletionTokens,
	}
	event := &OpenAIRemoteMessage{
		PortalKey: portal.PortalKey,
		ID:        networkid.MessageID(fmt.Sprintf("openai:%s", uuid.NewString())),
		Sender: bridgev2.EventSender{
			Sender:      assistantUserID(oc.UserLogin.ID),
			ForceDMUser: true,
			SenderLogin: oc.UserLogin.ID,
			IsFromMe:    false,
		},
		Content:   body,
		Timestamp: time.Unix(completion.Created, 0),
		Metadata:  meta,
	}
	oc.UserLogin.QueueRemoteEvent(event)
}

func (oc *OpenAIClient) dispatchCompletion(
	ctx context.Context,
	sourceEvent *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
) {
	runCtx := oc.backgroundContext(ctx)
	runCtx = oc.log.WithContext(runCtx)

	// Check if streaming is enabled
	if oc.connector.Config.OpenAI.EnableStreaming {
		oc.streamingCompletion(runCtx, sourceEvent, portal, meta, prompt)
		return
	}

	// Non-streaming path (original)
	resp, err := oc.requestCompletion(runCtx, prompt, meta)
	if err != nil {
		oc.notifyMatrixSendFailure(runCtx, portal, sourceEvent, err)
		return
	}
	if resp != nil {
		oc.queueAssistantMessage(portal, resp)
	}
}

// streamingCompletion handles streaming chat completions with transient token updates
func (oc *OpenAIClient) streamingCompletion(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	messages []openai.ChatCompletionMessageParamUnion,
) {
	log := zerolog.Ctx(ctx).With().
		Str("portal_id", string(portal.ID)).
		Logger()

	// Set typing indicator when streaming starts
	oc.setAssistantTyping(ctx, portal, true)
	defer oc.setAssistantTyping(ctx, portal, false)

	// Start streaming request
	params := openai.ChatCompletionNewParams{
		Model:               shared.ChatModel(oc.effectiveModel(meta)),
		Messages:            messages,
		Temperature:         openai.Float(oc.effectiveTemperature(meta)),
		MaxCompletionTokens: openai.Int(int64(oc.effectiveMaxTokens(meta))),
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, oc.connector.Config.OpenAI.RequestTimeout)
	defer cancel()

	stream := oc.api.Chat.Completions.NewStreaming(timeoutCtx, params)
	if stream == nil {
		log.Error().Msg("Failed to create streaming request")
		oc.notifyMatrixSendFailure(ctx, portal, evt, fmt.Errorf("streaming not available"))
		return
	}

	// Track streaming state
	var (
		accumulated     strings.Builder
		firstToken      bool = true
		initialEventID  id.EventID
		finishReason    string

		// Transient token debouncing
		transientDebounce = time.Duration(oc.connector.Config.OpenAI.TransientDebounceMs) * time.Millisecond
		lastTransientTime = time.Now()
	)

	// Process stream chunks
	for stream.Next() {
		chunk := stream.Current()

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta

		// Accumulate content
		if delta.Content != "" {
			accumulated.WriteString(delta.Content)
		}

		// First token - send initial message synchronously to capture event_id
		if firstToken && accumulated.Len() > 0 {
			firstToken = false

			log.Info().Msg("Sending initial streaming message")

			// Get ghost intent for sending
			ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, assistantUserID(oc.UserLogin.ID))
			if err != nil {
				log.Error().Err(err).Msg("Failed to get ghost for initial message")
				continue
			}
			intent := ghost.Intent
			if intent == nil {
				log.Error().Msg("Ghost intent is nil")
				continue
			}

			// Send initial message synchronously to get event_id
			eventContent := &event.Content{
				Raw: map[string]interface{}{
					"msgtype": "m.text",
					"body":    accumulated.String(),
				},
			}
			resp, err := intent.SendMessage(ctx, portal.MXID, event.EventMessage, eventContent, nil)
			if err != nil {
				log.Error().Err(err).Msg("Failed to send initial streaming message")
				oc.notifyMatrixSendFailure(ctx, portal, evt, err)
				return
			}
			initialEventID = resp.EventID
			log.Info().Str("event_id", initialEventID.String()).Msg("Initial streaming message sent")
			lastTransientTime = time.Now()
		}

		// Subsequent tokens - emit transient updates with debouncing
		if !firstToken && initialEventID != "" {
			now := time.Now()
			if now.Sub(lastTransientTime) >= transientDebounce {
				oc.emitTransientToken(ctx, portal, initialEventID, accumulated.String())
				lastTransientTime = now
			}
		}

		// Handle finish
		if choice.FinishReason != "" {
			finishReason = choice.FinishReason
			log.Debug().Str("reason", finishReason).Msg("Stream finished")
		}
	}

	// Check for errors
	if err := stream.Err(); err != nil {
		log.Error().Err(err).Msg("Streaming error")
		oc.notifyMatrixSendFailure(ctx, portal, evt, err)
		return
	}

	// Emit final transient update with complete content
	if initialEventID != "" {
		oc.emitTransientToken(ctx, portal, initialEventID, accumulated.String())
	}

	log.Info().
		Str("finish_reason", finishReason).
		Int("length", accumulated.Len()).
		Msg("Streaming completion finished")
}

func (oc *OpenAIClient) notifyMatrixSendFailure(ctx context.Context, portal *bridgev2.Portal, evt *event.Event, err error) {
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

// setAssistantTyping sets the typing indicator for the assistant ghost user
func (oc *OpenAIClient) setAssistantTyping(ctx context.Context, portal *bridgev2.Portal, typing bool) {
	if portal == nil || portal.MXID == "" {
		return
	}
	ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, assistantUserID(oc.UserLogin.ID))
	if err != nil {
		oc.log.Warn().Err(err).Msg("Failed to get ghost for typing indicator")
		return
	}
	intent := ghost.Intent
	if intent == nil {
		oc.log.Warn().Msg("Ghost intent is nil, cannot set typing")
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

// emitTransientToken sends a transient streaming token update to the room
// Uses Matrix-spec compliant m.relates_to to correlate with the initial message
func (oc *OpenAIClient) emitTransientToken(ctx context.Context, portal *bridgev2.Portal, relatedEventID id.EventID, content string) {
	if portal == nil || portal.MXID == "" {
		return
	}
	ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, assistantUserID(oc.UserLogin.ID))
	if err != nil {
		oc.log.Warn().Err(err).Msg("Failed to get ghost for transient token")
		return
	}
	intent := ghost.Intent
	if intent == nil {
		oc.log.Warn().Msg("Ghost intent is nil, cannot emit transient token")
		return
	}
	eventContent := &event.Content{
		Raw: map[string]interface{}{
			"body": content,
			"m.relates_to": map[string]interface{}{
				"rel_type": "m.reference",
				"event_id": relatedEventID.String(),
			},
		},
	}
	_, err = intent.SendMessage(ctx, portal.MXID, event.Type{
		Type:  "com.beeper.ai.stream_token",
		Class: event.MessageEventType,
	}, eventContent, nil)
	if err != nil {
		oc.log.Warn().Err(err).Str("related_event_id", relatedEventID.String()).Msg("Failed to emit transient token")
	}
}

func (oc *OpenAIClient) backgroundContext(ctx context.Context) context.Context {
	if oc.UserLogin != nil && oc.UserLogin.Bridge != nil && oc.UserLogin.Bridge.BackgroundCtx != nil {
		return oc.UserLogin.Bridge.BackgroundCtx
	}
	if ctx == nil || ctx.Err() != nil {
		return context.Background()
	}
	return ctx
}

func (oc *OpenAIClient) sendWelcomeMessage(ctx context.Context, portal *bridgev2.Portal) {
	meta := portalMeta(portal)
	if meta.WelcomeSent {
		return
	}
	body := "This chat was created automatically. Send a message to start talking to ChatGPT."
	event := &OpenAIRemoteMessage{
		PortalKey: portal.PortalKey,
		ID:        networkid.MessageID(fmt.Sprintf("openai:welcome:%s", uuid.NewString())),
		Sender: bridgev2.EventSender{
			Sender:      assistantUserID(oc.UserLogin.ID),
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

func (oc *OpenAIClient) effectiveModel(meta *PortalMetadata) string {
	if meta != nil && meta.Model != "" {
		return meta.Model
	}
	// Fallback to default model if not set in portal metadata
	return "gpt-4o-mini"
}

func (oc *OpenAIClient) effectivePrompt(meta *PortalMetadata) string {
	if meta != nil && meta.SystemPrompt != "" {
		return meta.SystemPrompt
	}
	return oc.connector.Config.OpenAI.SystemPrompt
}

func (oc *OpenAIClient) effectiveTemperature(meta *PortalMetadata) float64 {
	if meta != nil && meta.Temperature > 0 {
		return meta.Temperature
	}
	if oc.connector.Config.OpenAI.DefaultTemperature > 0 {
		return oc.connector.Config.OpenAI.DefaultTemperature
	}
	return defaultTemperature
}

func (oc *OpenAIClient) historyLimit(meta *PortalMetadata) int {
	if meta != nil && meta.MaxContextMessages > 0 {
		return meta.MaxContextMessages
	}
	if oc.connector.Config.OpenAI.MaxContextMessages > 0 {
		return oc.connector.Config.OpenAI.MaxContextMessages
	}
	return defaultMaxContextMessages
}

func (oc *OpenAIClient) effectiveMaxTokens(meta *PortalMetadata) int {
	if meta != nil && meta.MaxCompletionTokens > 0 {
		return meta.MaxCompletionTokens
	}
	if oc.connector.Config.OpenAI.MaxCompletionTokens > 0 {
		return oc.connector.Config.OpenAI.MaxCompletionTokens
	}
	return defaultMaxTokens
}

// listAvailableModels fetches models from OpenAI API and caches them
// Returns openai.Model directly from the SDK
func (oc *OpenAIClient) listAvailableModels(ctx context.Context, forceRefresh bool) ([]openai.Model, error) {
	meta := loginMetadata(oc.UserLogin)

	// Check cache (refresh every 6 hours unless forced)
	if !forceRefresh && meta.ModelCache != nil {
		age := time.Now().Unix() - meta.ModelCache.LastRefresh
		if age < meta.ModelCache.CacheDuration {
			return meta.ModelCache.Models, nil
		}
	}

	// Fetch models from API
	oc.log.Debug().Msg("Fetching available models from OpenAI API")

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	page, err := oc.api.Models.List(timeoutCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	var models []openai.Model
	for page != nil {
		for _, model := range page.Data {
			// Only include chat models (filter out embeddings, audio, etc.)
			if !isChatModel(model.ID) {
				continue
			}

			models = append(models, model)
		}

		// Get next page if available
		page, err = page.GetNextPage()
		if err != nil {
			return nil, fmt.Errorf("error iterating models: %w", err)
		}
	}

	// Update cache
	if meta.ModelCache == nil {
		meta.ModelCache = &ModelCache{
			CacheDuration: 6 * 60 * 60, // 6 hours
		}
	}
	meta.ModelCache.Models = models
	meta.ModelCache.LastRefresh = time.Now().Unix()

	// Save metadata
	if err := oc.UserLogin.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to save model cache")
	}

	oc.log.Info().Int("count", len(models)).Msg("Cached available models")
	return models, nil
}

// isChatModel determines if a model ID is a chat completion model
func isChatModel(modelID string) bool {
	// Include gpt-* models, exclude embeddings, whisper, tts, dall-e, etc.
	if strings.HasPrefix(modelID, "gpt-") {
		return true
	}
	// Future: add o1, o3, or other chat model families as they're released
	return false
}

// getModelCapabilities computes capabilities for a model
func getModelCapabilities(modelID string) ModelCapabilities {
	return ModelCapabilities{
		SupportsVision: detectVisionSupport(modelID),
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

// formatModelName converts model ID to human-readable name
func formatModelName(modelID string) string {
	// Convert "gpt-4o-mini" → "GPT-4o Mini"
	parts := strings.Split(modelID, "-")
	formatted := make([]string, len(parts))
	for i, part := range parts {
		if i == 0 {
			formatted[i] = strings.ToUpper(part)
		} else {
			formatted[i] = strings.Title(part)
		}
	}
	return strings.Join(formatted, "-")
}

func (oc *OpenAIClient) scheduleBootstrap() {
	backgroundCtx := oc.UserLogin.Bridge.BackgroundCtx
	go oc.bootstrap(backgroundCtx)
}

func (oc *OpenAIClient) bootstrap(ctx context.Context) {
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

func (oc *OpenAIClient) waitForLoginPersisted(ctx context.Context) {
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

func (oc *OpenAIClient) syncChatCounter(ctx context.Context) error {
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

func (oc *OpenAIClient) ensureDefaultChat(ctx context.Context) error {
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
		return err
	}
	resp, err := oc.createChat(ctx, "", oc.connector.Config.OpenAI.SystemPrompt)
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
	oc.log.Info().Stringer("portal", resp.PortalKey).Msg("Default ChatGPT room created")
	return nil
}

func (oc *OpenAIClient) listAllChatPortals(ctx context.Context) ([]*bridgev2.Portal, error) {
	dbPortals, err := oc.UserLogin.Bridge.DB.Portal.GetAllDMsWith(ctx, assistantUserID(oc.UserLogin.ID))
	if err != nil {
		return nil, err
	}
	portals := make([]*bridgev2.Portal, 0, len(dbPortals))
	for _, dbPortal := range dbPortals {
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

func (oc *OpenAIClient) createChat(ctx context.Context, title, systemPrompt string) (*bridgev2.CreateChatResponse, error) {
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

func (oc *OpenAIClient) spawnPortal(ctx context.Context, title, systemPrompt string) (*bridgev2.Portal, *bridgev2.ChatInfo, error) {
	oc.chatLock.Lock()
	defer oc.chatLock.Unlock()
	oc.log.Debug().Str("title", title).Msg("Allocating portal for new chat")

	meta := loginMetadata(oc.UserLogin)
	meta.NextChatIndex++
	index := meta.NextChatIndex
	slug := formatChatSlug(index)
	if title == "" {
		title = fmt.Sprintf("ChatGPT %d", index)
	}
	if systemPrompt == "" {
		systemPrompt = oc.connector.Config.OpenAI.SystemPrompt
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
	if systemPrompt != "" {
		pmeta.SystemPrompt = systemPrompt
	}
	portal.RoomType = database.RoomTypeDM
	portal.OtherUserID = assistantUserID(oc.UserLogin.ID)
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
	info := oc.composeChatInfo(title, systemPrompt)
	return portal, info, nil
}

func (oc *OpenAIClient) chatInfoFromPortal(portal *bridgev2.Portal) *bridgev2.ChatInfo {
	meta := portalMeta(portal)
	title := meta.Title
	if title == "" {
		if portal.Name != "" {
			title = portal.Name
		} else {
			title = "ChatGPT"
		}
	}
	prompt := meta.SystemPrompt
	if prompt == "" {
		prompt = oc.connector.Config.OpenAI.SystemPrompt
	}
	return oc.composeChatInfo(title, prompt)
}

func (oc *OpenAIClient) composeChatInfo(title, prompt string) *bridgev2.ChatInfo {
	if title == "" {
		title = "ChatGPT"
	}
	if prompt == "" {
		prompt = oc.connector.Config.OpenAI.SystemPrompt
	}
	members := bridgev2.ChatMemberMap{
		humanUserID(oc.UserLogin.ID): {
			EventSender: bridgev2.EventSender{
				IsFromMe:    true,
				SenderLogin: oc.UserLogin.ID,
			},
			Membership: event.MembershipJoin,
		},
		assistantUserID(oc.UserLogin.ID): {
			EventSender: bridgev2.EventSender{
				Sender:      assistantUserID(oc.UserLogin.ID),
				SenderLogin: oc.UserLogin.ID,
			},
			Membership: event.MembershipJoin,
			UserInfo: &bridgev2.UserInfo{
				Name:  ptr.Ptr("ChatGPT"),
				IsBot: ptr.Ptr(true),
			},
		},
	}
	return &bridgev2.ChatInfo{
		Name:  ptr.Ptr(title),
		Topic: ptrIfNotEmpty(prompt),
		Type:  ptr.Ptr(database.RoomTypeDM),
		Members: &bridgev2.ChatMemberList{
			IsFull:      true,
			OtherUserID: assistantUserID(oc.UserLogin.ID),
			MemberMap:   members,
		},
	}
}

// RoomStateEvent represents ai-bridge configuration in room state
type RoomStateEvent struct {
	Model               string  `json:"model,omitempty"`
	SystemPrompt        string  `json:"system_prompt,omitempty"`
	Temperature         float64 `json:"temperature,omitempty"`
	MaxContextMessages  int     `json:"max_context_messages,omitempty"`
	MaxCompletionTokens int     `json:"max_completion_tokens,omitempty"`
}

// HandleRoomStateEvent processes room state changes for ai-bridge config
// NOTE: This is a stub implementation. To fully implement, we need:
// 1. A way to look up the portal by Matrix room ID (not available in current bridgev2)
// 2. Or, integrate with bridgev2's room state handler pattern
// For now, room configuration can be edited by looking up the portal via other means
func (oc *OpenAIClient) HandleRoomStateEvent(ctx context.Context, evt *event.Event) error {
	log := zerolog.Ctx(ctx)

	// Parse room state
	var stateContent RoomStateEvent
	if err := json.Unmarshal(evt.Content.VeryRaw, &stateContent); err != nil {
		log.Debug().Err(err).Msg("Failed to parse room state, ignoring")
		return nil // Non-fatal, might not be our room state
	}

	// TODO: Implement portal lookup by Matrix room ID
	// This requires bridgev2 to expose a method like GetPortalByMatrixID
	// or to integrate with the room state handler pattern in bridgev2
	log.Debug().
		Str("room_id", string(evt.RoomID)).
		Str("model", stateContent.Model).
		Msg("Room state event received (stub - portal lookup not yet implemented)")

	return nil
}

// BroadcastRoomState sends current room config to Matrix room state
func (oc *OpenAIClient) BroadcastRoomState(ctx context.Context, portal *bridgev2.Portal) error {
	meta := portal.Metadata.(*PortalMetadata)

	stateContent := RoomStateEvent{
		Model:               meta.Model,
		SystemPrompt:        meta.SystemPrompt,
		Temperature:         meta.Temperature,
		MaxContextMessages:  meta.MaxContextMessages,
		MaxCompletionTokens: meta.MaxCompletionTokens,
	}

	// Marshal to JSON
	contentBytes, err := json.Marshal(stateContent)
	if err != nil {
		return fmt.Errorf("failed to marshal room state: %w", err)
	}

	// Send state event through bridgev2 intent
	// Note: This requires bridgev2 room state API support
	// For now, this is a stub that can be called but won't fully work without bridgev2 updates
	log := zerolog.Ctx(ctx)
	log.Debug().Str("model", meta.Model).Msg("Broadcasting room state (stub - bridgev2 API support needed)")

	_ = contentBytes // Suppress unused variable warning
	return nil
}

func ptrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return ptr.Ptr(value)
}
