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
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

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
	_ bridgev2.EditHandlingNetworkAPI        = (*OpenAIClient)(nil)
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

	portal := msg.Portal
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
		return nil, nil // Command was handled, no message to save
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
	// Dispatch completion handling in the background so the Matrix send pipeline can ack immediately.
	go oc.dispatchCompletion(ctx, msg.Event, portal, meta, promptMessages)
	return &bridgev2.MatrixMessageResponse{
		DB: userMessage,
	}, nil
}

// HandleMatrixEdit handles edits to previously sent messages
func (oc *OpenAIClient) HandleMatrixEdit(ctx context.Context, edit *bridgev2.MatrixEdit) error {
	if edit.Content == nil || edit.EditTarget == nil {
		return fmt.Errorf("invalid edit: missing content or target")
	}

	portal := edit.Portal
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
func (oc *OpenAIClient) regenerateFromEdit(
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

	// Dispatch a new completion
	go oc.dispatchCompletion(ctx, evt, portal, meta, promptMessages)

	return nil
}

// buildPromptUpToMessage builds a prompt including messages up to and including the specified message
func (oc *OpenAIClient) buildPromptUpToMessage(
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
			if msgMeta == nil || msgMeta.Body == "" {
				continue
			}

			// Stop after adding the target message
			if msg.ID == targetMessageID {
				// Use the new body for the edited message
				prompt = append(prompt, openai.UserMessage(newBody))
				break
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
func (oc *OpenAIClient) handleImageMessage(
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

	// Dispatch completion handling in the background
	go oc.dispatchCompletion(ctx, msg.Event, portal, meta, promptMessages)
	return &bridgev2.MatrixMessageResponse{
		DB: userMessage,
	}, nil
}

// buildPromptWithImage builds a prompt that includes an image URL
func (oc *OpenAIClient) buildPromptWithImage(
	ctx context.Context,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	caption string,
	imageURL string,
) ([]openai.ChatCompletionMessageParamUnion, error) {
	var prompt []openai.ChatCompletionMessageParamUnion

	// Add system prompt
	systemPrompt := oc.effectivePrompt(meta)
	if systemPrompt != "" {
		prompt = append(prompt, openai.SystemMessage(systemPrompt))
	}

	// Add history (text-only for now)
	historyLimit := oc.historyLimit(meta)
	if historyLimit > 0 {
		history, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, historyLimit)
		if err != nil {
			return nil, fmt.Errorf("failed to load prompt history: %w", err)
		}
		for i := len(history) - 1; i >= 0; i-- {
			msgMeta := messageMeta(history[i])
			if msgMeta == nil || msgMeta.Body == "" {
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
func (oc *OpenAIClient) convertMxcToHttp(mxcURL string) string {
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
func (oc *OpenAIClient) handleCommand(
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
func (oc *OpenAIClient) handleSlashCommand(
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
		config := fmt.Sprintf(
			"Current configuration:\n"+
				"• Model: %s\n"+
				"• Temperature: %.2f\n"+
				"• Context: %d messages\n"+
				"• Max tokens: %d\n"+
				"• Vision: %v",
			oc.effectiveModel(meta),
			oc.effectiveTemperature(meta),
			oc.historyLimit(meta),
			oc.effectiveMaxTokens(meta),
			meta.Capabilities.SupportsVision,
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
			"• /config - Show current configuration\n" +
			"• /cost - Show conversation token usage and cost\n" +
			"• /regenerate - Regenerate the last response\n" +
			"• /help - Show this help message"
		oc.sendSystemNotice(ctx, portal, help)
		return true

	case "/regenerate":
		go oc.handleRegenerate(ctx, evt, portal, meta)
		return true
	}

	return false
}

// handleRegenerate regenerates the last AI response
func (oc *OpenAIClient) handleRegenerate(
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
	for i := 0; i < len(history); i++ {
		msgMeta := messageMeta(history[i])
		if msgMeta != nil && msgMeta.Role == "user" {
			lastUserMessage = history[i]
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

	// Dispatch new completion
	oc.dispatchCompletion(runCtx, evt, portal, meta, prompt)
}

// buildPromptForRegenerate builds a prompt for regeneration, excluding the last assistant message
func (oc *OpenAIClient) buildPromptForRegenerate(
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
		for i := 0; i < len(history); i++ {
			msgMeta := messageMeta(history[i])
			if msgMeta == nil || msgMeta.Body == "" {
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

func (oc *OpenAIClient) requestCompletion(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, meta *PortalMetadata) (*openai.ChatCompletion, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("prompt had no messages")
	}
	model := oc.effectiveModel(meta)
	params := openai.ChatCompletionNewParams{
		Model:               shared.ChatModel(model),
		Messages:            messages,
		Temperature:         openai.Float(oc.effectiveTemperature(meta)),
		MaxCompletionTokens: openai.Int(int64(oc.effectiveMaxTokens(meta))),
	}

	// Add tools if enabled
	if meta.ToolsEnabled && !isReasoningModel(model) {
		params.Tools = ToOpenAITools(BuiltinTools())
	}

	// Use extended timeout for reasoning models (O1/O3)
	timeout := oc.connector.Config.OpenAI.RequestTimeout
	if isReasoningModel(model) {
		timeout = reasoningModelRequestTimeout
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := oc.api.Chat.Completions.New(timeoutCtx, params)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// requestCompletionWithToolLoop handles tool calls in a loop until the model gives a final response
// It returns the final response and a list of tool call messages that were sent
func (oc *OpenAIClient) requestCompletionWithToolLoop(
	ctx context.Context,
	messages []openai.ChatCompletionMessageParamUnion,
	meta *PortalMetadata,
	onToolCall func(toolName, args, result string),
) (*openai.ChatCompletion, error) {
	currentMessages := messages
	maxToolIterations := 10 // Prevent infinite loops

	for i := 0; i < maxToolIterations; i++ {
		resp, err := oc.requestCompletion(ctx, currentMessages, meta)
		if err != nil {
			return nil, err
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("no choices in response")
		}

		choice := resp.Choices[0]

		// Check if there are tool calls
		if len(choice.Message.ToolCalls) == 0 {
			// No tool calls, this is the final response
			return resp, nil
		}

		// Execute tool calls
		// First, add the assistant's message with tool_calls to the conversation
		// Convert tool calls to params
		toolCallParams := make([]openai.ChatCompletionMessageToolCallParam, len(choice.Message.ToolCalls))
		for j, tc := range choice.Message.ToolCalls {
			toolCallParams[j] = tc.ToParam()
		}
		currentMessages = append(currentMessages, openai.ChatCompletionMessageParamUnion{
			OfAssistant: &openai.ChatCompletionAssistantMessageParam{
				ToolCalls: toolCallParams,
			},
		})

		// Execute each tool call and add results
		for _, toolCall := range choice.Message.ToolCalls {
			toolName := toolCall.Function.Name
			toolArgs := toolCall.Function.Arguments

			// Parse arguments
			var args map[string]any
			if err := json.Unmarshal([]byte(toolArgs), &args); err != nil {
				args = map[string]any{"raw": toolArgs}
			}

			// Execute tool
			tool := GetToolByName(toolName)
			var result string
			if tool != nil {
				toolResult, err := tool.Execute(ctx, args)
				if err != nil {
					result = fmt.Sprintf("Error: %v", err)
				} else {
					result = toolResult
				}
			} else {
				result = fmt.Sprintf("Error: Unknown tool '%s'", toolName)
			}

			// Notify callback if provided
			if onToolCall != nil {
				onToolCall(toolName, toolArgs, result)
			}

			// Add tool result to messages
			currentMessages = append(currentMessages, openai.ToolMessage(result, toolCall.ID))
		}
	}

	return nil, fmt.Errorf("exceeded maximum tool iterations (%d)", maxToolIterations)
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

	// Check if streaming is enabled and model supports it
	// O1/O3 reasoning models don't support streaming
	model := oc.effectiveModel(meta)
	if oc.connector.Config.OpenAI.EnableStreaming && !isReasoningModel(model) {
		oc.streamingCompletionWithRetry(runCtx, sourceEvent, portal, meta, prompt)
		return
	}

	// Non-streaming path with retry (also used for reasoning models)
	oc.nonStreamingCompletionWithRetry(runCtx, sourceEvent, portal, meta, prompt)
}

// nonStreamingCompletionWithRetry handles non-streaming completions with automatic retry on context overflow
func (oc *OpenAIClient) nonStreamingCompletionWithRetry(
	ctx context.Context,
	sourceEvent *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
) {
	currentPrompt := prompt

	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		var resp *openai.ChatCompletion
		var err error

		// Use tool loop if tools are enabled
		if meta.ToolsEnabled {
			resp, err = oc.requestCompletionWithToolLoop(ctx, currentPrompt, meta, func(toolName, args, result string) {
				// Send a notice for each tool call
				notice := fmt.Sprintf("Using %s...\n%s", toolName, result)
				if len(notice) > 500 {
					notice = notice[:497] + "..."
				}
				oc.sendSystemNotice(ctx, portal, notice)
			})
		} else {
			resp, err = oc.requestCompletion(ctx, currentPrompt, meta)
		}

		if err == nil {
			if resp != nil {
				oc.queueAssistantMessage(portal, resp)
				// Generate room title after first response
				if len(resp.Choices) > 0 {
					oc.maybeGenerateTitle(ctx, portal, resp.Choices[0].Message.Content)
				}
			}
			return
		}

		// Check for context length error
		cle := ParseContextLengthError(err)
		if cle == nil {
			// Not a context error, report and exit
			oc.notifyMatrixSendFailure(ctx, portal, sourceEvent, err)
			return
		}

		// Can we truncate further?
		truncated := oc.truncatePrompt(currentPrompt)
		if truncated == nil || len(truncated) <= 2 {
			// Can't truncate more (only system + latest user message left)
			oc.notifyContextLengthExceeded(ctx, portal, cle, false)
			return
		}

		// Notify user and retry
		oc.notifyContextLengthExceeded(ctx, portal, cle, true)
		currentPrompt = truncated

		oc.log.Debug().
			Int("attempt", attempt+1).
			Int("new_prompt_len", len(currentPrompt)).
			Msg("Retrying with truncated context")
	}

	// Exhausted retries
	oc.notifyMatrixSendFailure(ctx, portal, sourceEvent,
		fmt.Errorf("exceeded retry attempts for context length"))
}

// streamingCompletionWithRetry handles streaming completions with automatic retry on context overflow
func (oc *OpenAIClient) streamingCompletionWithRetry(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
) {
	currentPrompt := prompt

	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		success, cle := oc.streamingCompletion(ctx, evt, portal, meta, currentPrompt)
		if success {
			return
		}

		// If we got a context length error, try to truncate and retry
		if cle != nil {
			truncated := oc.truncatePrompt(currentPrompt)
			if truncated == nil || len(truncated) <= 2 {
				// Can't truncate more
				oc.notifyContextLengthExceeded(ctx, portal, cle, false)
				return
			}

			// Notify user and retry
			oc.notifyContextLengthExceeded(ctx, portal, cle, true)
			currentPrompt = truncated

			oc.log.Debug().
				Int("attempt", attempt+1).
				Int("new_prompt_len", len(currentPrompt)).
				Msg("Retrying streaming with truncated context")
			continue
		}

		// Non-context error, already handled in streamingCompletion
		return
	}
}

// streamingCompletion handles streaming chat completions with transient token updates
// Returns (success, contextLengthError) - contextLengthError is non-nil only when the error is a context length issue
func (oc *OpenAIClient) streamingCompletion(
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
		return false, nil
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
				cle := ParseContextLengthError(err)
				if cle != nil {
					return false, cle
				}
				oc.notifyMatrixSendFailure(ctx, portal, evt, err)
				return false, nil
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
		cle := ParseContextLengthError(err)
		if cle != nil {
			return false, cle
		}
		oc.notifyMatrixSendFailure(ctx, portal, evt, err)
		return false, nil
	}

	// Emit final transient update with complete content
	if initialEventID != "" {
		oc.emitTransientToken(ctx, portal, initialEventID, accumulated.String())
	}

	log.Info().
		Str("finish_reason", finishReason).
		Int("length", accumulated.Len()).
		Msg("Streaming completion finished")

	// Generate room title after first response
	oc.maybeGenerateTitle(ctx, portal, accumulated.String())

	return true, nil
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
		Raw: map[string]any{
			"body": content,
			"m.relates_to": map[string]any{
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

// knownModelPrefixes is a list of known valid model ID prefixes
var knownModelPrefixes = []string{
	"gpt-4", "gpt-3.5", "o1", "o3", "chatgpt",
}

// validateModel checks if a model is available for this user
func (oc *OpenAIClient) validateModel(ctx context.Context, modelID string) (bool, error) {
	if modelID == "" {
		return true, nil
	}

	// First check cache
	models, err := oc.listAvailableModels(ctx, false)
	if err == nil {
		for _, model := range models {
			if model.ID == modelID {
				return true, nil
			}
		}
	}

	// Check against known patterns
	for _, prefix := range knownModelPrefixes {
		if strings.HasPrefix(modelID, prefix) {
			return true, nil
		}
	}

	// Try to validate by making a minimal API call
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err = oc.api.Models.Get(timeoutCtx, modelID)
	return err == nil, nil
}

// updatePortalConfig applies room config to portal metadata
func (oc *OpenAIClient) updatePortalConfig(ctx context.Context, portal *bridgev2.Portal, config *RoomConfigEventContent) {
	meta := portalMeta(portal)

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
	// ToolsEnabled is a boolean - always apply it
	meta.ToolsEnabled = config.ToolsEnabled

	meta.LastRoomStateSync = time.Now().Unix()

	// Persist changes
	if err := portal.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to save portal after config update")
	}
}

// sendConfigNotice sends a notice to the room about configuration changes
func (oc *OpenAIClient) sendConfigNotice(ctx context.Context, portal *bridgev2.Portal, message string) {
	oc.sendSystemNotice(ctx, portal, message)
}

// sendSystemNotice sends an informational notice to the room as the assistant
func (oc *OpenAIClient) sendSystemNotice(ctx context.Context, portal *bridgev2.Portal, message string) {
	if portal == nil || portal.MXID == "" {
		return
	}

	ghost, err := oc.UserLogin.Bridge.GetGhostByID(ctx, assistantUserID(oc.UserLogin.ID))
	if err != nil {
		oc.log.Warn().Err(err).Msg("Failed to get ghost for system notice")
		return
	}

	intent := ghost.Intent
	if intent == nil {
		return
	}

	content := &event.MessageEventContent{
		MsgType: event.MsgNotice,
		Body:    message,
	}

	_, err = intent.SendMessage(ctx, portal.MXID, event.EventMessage, &event.Content{
		Parsed: content,
	}, nil)
	if err != nil {
		oc.log.Warn().Err(err).Msg("Failed to send system notice")
	}
}

const (
	maxRetryAttempts = 3 // Maximum retry attempts for context length errors
)

// notifyContextLengthExceeded sends a user-friendly notice about context overflow
func (oc *OpenAIClient) notifyContextLengthExceeded(
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
func (oc *OpenAIClient) truncatePrompt(
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
	// O1/O3 reasoning models
	if strings.HasPrefix(modelID, "o1") || strings.HasPrefix(modelID, "o3") {
		return true
	}
	return false
}

// isReasoningModel checks if the model is an O1/O3 reasoning model
// These models don't support streaming and have longer response times
func isReasoningModel(modelID string) bool {
	return strings.HasPrefix(modelID, "o1") || strings.HasPrefix(modelID, "o3")
}

// getModelCapabilities computes capabilities for a model
func getModelCapabilities(modelID string) ModelCapabilities {
	return ModelCapabilities{
		SupportsVision:   detectVisionSupport(modelID),
		IsReasoningModel: isReasoningModel(modelID),
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
	titleCaser := cases.Title(language.English)
	for i, part := range parts {
		if i == 0 {
			formatted[i] = strings.ToUpper(part)
		} else {
			formatted[i] = titleCaser.String(part)
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
		// Broadcast initial room state
		go oc.BroadcastRoomState(ctx, portals[0])
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
	// Broadcast initial room state
	go oc.BroadcastRoomState(ctx, resp.Portal)
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


// BroadcastRoomState sends current room config to Matrix room state
func (oc *OpenAIClient) BroadcastRoomState(ctx context.Context, portal *bridgev2.Portal) error {
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
		ToolsEnabled:        meta.ToolsEnabled,
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
func (oc *OpenAIClient) generateRoomTitle(ctx context.Context, userMessage, assistantResponse string) (string, error) {
	prompt := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("Generate a very short title (3-5 words max) that summarizes this conversation. Reply with ONLY the title, no quotes, no punctuation at the end."),
		openai.UserMessage(fmt.Sprintf("User: %s\n\nAssistant: %s", userMessage, assistantResponse)),
	}

	model := oc.effectiveModel(nil)
	// Use a faster/cheaper model for title generation if available
	if strings.HasPrefix(model, "gpt-4") {
		model = "gpt-4o-mini"
	}

	resp, err := oc.api.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:     model,
		Messages:  prompt,
		MaxTokens: openai.Int(20),
	})
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from model")
	}

	title := strings.TrimSpace(resp.Choices[0].Message.Content)
	// Remove quotes if the model added them
	title = strings.Trim(title, "\"'")
	// Limit length
	if len(title) > 50 {
		title = title[:50]
	}
	return title, nil
}

// setRoomName sets the Matrix room name via m.room.name state event
func (oc *OpenAIClient) setRoomName(ctx context.Context, portal *bridgev2.Portal, name string) error {
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
func (oc *OpenAIClient) maybeGenerateTitle(ctx context.Context, portal *bridgev2.Portal, assistantResponse string) {
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
