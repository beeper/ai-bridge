package connector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/rs/zerolog"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"
)

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

// notifyMatrixSendFailure sends an error status back to Matrix
func (oc *AIClient) notifyMatrixSendFailure(ctx context.Context, portal *bridgev2.Portal, evt *event.Event, err error) {
	// Check for auth errors (401/403) - trigger reauth with StateBadCredentials
	if IsAuthError(err) {
		oc.loggedIn.Store(false)
		oc.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateBadCredentials,
			Error:      AIAuthFailed,
			Message:    "Authentication failed - please re-login",
			Info: map[string]any{
				"error": err.Error(),
			},
		})
	}

	// Check for billing errors - send transient disconnect with billing message
	if IsBillingError(err) {
		oc.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateTransientDisconnect,
			Error:      AIBillingError,
			Message:    "Billing issue with AI provider. Please check your account credits.",
		})
	}

	// Check for rate limit or overloaded errors - send transient disconnect
	if IsRateLimitError(err) || IsOverloadedError(err) {
		oc.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateTransientDisconnect,
			Error:      AIRateLimited,
			Message:    "Rate limited by AI provider. Please wait before retrying.",
		})
	}

	if portal == nil || portal.Bridge == nil || evt == nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to send message via OpenAI")
		return
	}

	// Use FormatUserFacingError for consistent, user-friendly error messages
	errorMessage := FormatUserFacingError(err)

	status := messageStatusForError(err)
	reason := messageStatusReasonForError(err)

	msgStatus := bridgev2.WrapErrorInStatus(err).
		WithStatus(status).
		WithErrorReason(reason).
		WithMessage(errorMessage).
		WithIsCertain(true).
		WithSendNotice(true)
	portal.Bridge.Matrix.SendMessageStatus(ctx, &msgStatus, bridgev2.StatusEventInfoFromEvent(evt))
	for _, extra := range statusEventsFromContext(ctx) {
		if extra != nil {
			portal.Bridge.Matrix.SendMessageStatus(ctx, &msgStatus, bridgev2.StatusEventInfoFromEvent(extra))
		}
	}
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

// sendWelcomeMessage sends a welcome message when a new chat is created.
// The message is excluded from LLM history so it doesn't affect conversation context.
func (oc *AIClient) sendWelcomeMessage(ctx context.Context, portal *bridgev2.Portal) {
	meta := portalMeta(portal)
	if meta.WelcomeSent {
		return
	}

	// Mark as sent BEFORE queuing to prevent duplicate welcome messages on race
	meta.WelcomeSent = true
	if err := portal.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to persist welcome message state")
		return // Don't send if we can't persist state
	}

	// Use portal.OtherUserID as authoritative source for sender
	// This ensures welcome message sender always matches the room's configured ghost
	senderID := portal.OtherUserID
	var displayName string

	agentID := resolveAgentID(meta)

	// Fallback: compute sender if portal.OtherUserID is not set
	if senderID == "" {
		modelID := oc.effectiveModel(meta)
		if agentID != "" {
			senderID = agentUserID(agentID)
		} else {
			senderID = modelUserID(modelID)
		}
	}

	// Determine display name based on whether this is an agent room
	if agentID != "" {
		// Agent room - get agent display name
		modelID := oc.effectiveModel(meta)
		store := NewAgentStoreAdapter(oc)
		if agent, err := store.GetAgentByID(ctx, agentID); err == nil && agent != nil {
			agentName := oc.resolveAgentDisplayName(ctx, agent)
			displayName = oc.agentModelDisplayName(agentName, modelID)
			oc.ensureAgentGhostDisplayName(ctx, agentID, modelID, agentName)
		} else {
			displayName = agentID // Fallback to agent ID
		}
	} else {
		// Model room - get model display name
		modelID := oc.effectiveModel(meta)
		displayName = modelContactName(modelID, oc.findModelInfo(modelID))
		oc.ensureGhostDisplayName(ctx, modelID)
	}

	body := fmt.Sprintf("Hello! I'm %s. Send a message to start our conversation.", displayName)

	event := &OpenAIRemoteMessage{
		PortalKey: portal.PortalKey,
		ID:        networkid.MessageID(fmt.Sprintf("openai:welcome:%s", uuid.NewString())),
		Sender: bridgev2.EventSender{
			Sender:      senderID,
			ForceDMUser: true,
			SenderLogin: oc.UserLogin.ID,
			IsFromMe:    false,
		},
		Content:   body,
		Timestamp: time.Now(),
		Metadata: &MessageMetadata{
			Role:               "assistant",
			Body:               body,
			ExcludeFromHistory: true, // Don't include in LLM context
		},
	}
	oc.UserLogin.QueueRemoteEvent(event)
}

// maybeGenerateTitle generates a title for the room after the first exchange
func (oc *AIClient) maybeGenerateTitle(ctx context.Context, portal *bridgev2.Portal, assistantResponse string) {
	meta := portalMeta(portal)

	if !oc.isOpenRouterProvider() {
		return
	}

	// Skip if title was already generated
	if meta.TitleGenerated {
		return
	}

	// Generate title in background to not block the message flow
	go func() {
		// Use a bounded timeout to prevent goroutine leaks if the API blocks
		bgCtx, cancel := context.WithTimeout(oc.backgroundContext(ctx), 15*time.Second)
		defer cancel()

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

// getTitleGenerationModel returns the model to use for generating chat titles.
// Priority: UserLoginMetadata.TitleGenerationModel > provider-specific default > current model
func (oc *AIClient) getTitleGenerationModel() string {
	meta := loginMetadata(oc.UserLogin)

	if meta.Provider != ProviderOpenRouter && meta.Provider != ProviderBeeper && meta.Provider != ProviderMagicProxy {
		return ""
	}

	// Use configured title generation model if set
	if meta.TitleGenerationModel != "" {
		return meta.TitleGenerationModel
	}

	// Provider-specific defaults for title generation
	switch meta.Provider {
	case ProviderOpenRouter, ProviderBeeper, ProviderMagicProxy:
		return "google/gemini-2.5-flash"
	default:
		// For non-OpenRouter providers, title generation is disabled.
		return ""
	}
}

// generateRoomTitle asks the model to generate a short descriptive title for the conversation
// Uses Responses API for OpenRouter compatibility (the PDF plugins middleware adds a 'plugins'
// field that is only valid for Responses API, not Chat Completions API)
func (oc *AIClient) generateRoomTitle(ctx context.Context, userMessage, assistantResponse string) (string, error) {
	model := oc.getTitleGenerationModel()
	if model == "" {
		return "", fmt.Errorf("title generation disabled for this provider")
	}

	oc.log.Debug().Str("model", model).Msg("Generating room title")

	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(model),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(fmt.Sprintf(
				"Generate a very short title (3-5 words max) that summarizes this conversation. Reply with ONLY the title, no quotes, no punctuation at the end.\n\nUser: %s\n\nAssistant: %s",
				userMessage, assistantResponse,
			)),
		},
		MaxOutputTokens: openai.Int(20),
	}

	// Disable reasoning for title generation to keep it fast and cheap.
	if oc.isOpenRouterProvider() {
		params.Reasoning = shared.ReasoningParam{
			Effort: shared.ReasoningEffortNone,
		}
	}

	// Use Responses API for OpenRouter compatibility (plugins field is only valid here)
	resp, err := oc.api.Responses.New(ctx, params)
	if err != nil && params.Reasoning.Effort != "" {
		oc.log.Warn().Err(err).Str("model", model).Msg("Title generation failed with reasoning disabled; retrying without reasoning param")
		params.Reasoning = shared.ReasoningParam{}
		resp, err = oc.api.Responses.New(ctx, params)
	}
	if err != nil {
		oc.log.Warn().Err(err).Str("model", model).Msg("Title generation API call failed")
		return "", err
	}

	title := extractTitleFromResponse(resp)

	if title == "" {
		oc.log.Warn().
			Str("model", model).
			Int("output_items", len(resp.Output)).
			Str("status", string(resp.Status)).
			Msg("Title generation returned no content")
		return "", fmt.Errorf("no response from model")
	}

	title = strings.TrimSpace(title)
	title = strings.Trim(title, "\"'")
	if len(title) > 50 {
		title = title[:50]
	}
	return title, nil
}

func extractTitleFromResponse(resp *responses.Response) string {
	var content strings.Builder
	var reasoning strings.Builder

	for _, item := range resp.Output {
		switch item := item.AsAny().(type) {
		case responses.ResponseOutputMessage:
			for _, part := range item.Content {
				// OpenRouter sometimes returns "text" instead of "output_text".
				if part.Type == "output_text" || part.Type == "text" {
					if part.Text != "" {
						content.WriteString(part.Text)
					}
					continue
				}
				if part.Text != "" {
					content.WriteString(part.Text)
				}
				if part.Type == "refusal" && part.Refusal != "" {
					content.WriteString(part.Refusal)
				}
			}
		case responses.ResponseReasoningItem:
			for _, summary := range item.Summary {
				if summary.Text != "" {
					reasoning.WriteString(summary.Text)
				}
			}
		}
	}

	if content.Len() > 0 {
		return content.String()
	}
	if reasoning.Len() > 0 {
		return reasoning.String()
	}
	return ""
}

// setRoomName sets the Matrix room name via m.room.name state event
func (oc *AIClient) setRoomName(ctx context.Context, portal *bridgev2.Portal, name string) error {
	return oc.setRoomNameInternal(ctx, portal, name, true)
}

func (oc *AIClient) setRoomNameNoSave(ctx context.Context, portal *bridgev2.Portal, name string) error {
	return oc.setRoomNameInternal(ctx, portal, name, false)
}

func (oc *AIClient) setRoomNameInternal(ctx context.Context, portal *bridgev2.Portal, name string, save bool) error {
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
	if save {
		if err := portal.Save(ctx); err != nil {
			oc.log.Warn().Err(err).Msg("Failed to save portal after setting room name")
		}
	}

	oc.log.Debug().Str("name", name).Msg("Set Matrix room name")
	return nil
}

// setRoomTopic sets the Matrix room topic via m.room.topic state event
func (oc *AIClient) setRoomTopic(ctx context.Context, portal *bridgev2.Portal, topic string) error {
	if portal.MXID == "" {
		return fmt.Errorf("portal has no Matrix room ID")
	}

	bot := oc.UserLogin.Bridge.Bot
	_, err := bot.SendState(ctx, portal.MXID, event.StateTopic, "", &event.Content{
		Parsed: &event.TopicEventContent{Topic: topic},
	}, time.Time{})

	if err != nil {
		return fmt.Errorf("failed to set room topic: %w", err)
	}

	portal.Topic = topic
	if err := portal.Save(ctx); err != nil {
		oc.log.Warn().Err(err).Msg("Failed to save portal after setting room topic")
	}

	oc.log.Debug().Str("topic", topic).Msg("Set Matrix room topic")
	return nil
}

// getModelContextWindow returns the context window size for the current model
func (oc *AIClient) getModelContextWindow(meta *PortalMetadata) int {
	modelID := oc.effectiveModel(meta)

	// Check cached model info first
	loginMeta := loginMetadata(oc.UserLogin)
	if loginMeta.ModelCache != nil {
		for _, m := range loginMeta.ModelCache.Models {
			if m.ID == modelID {
				return m.ContextWindow
			}
		}
	}

	// Fallback: check model catalog
	if info := oc.findModelInfoInCatalog(modelID); info != nil {
		return info.ContextWindow
	}

	// Default for unknown models
	return 128000
}

// setRoomSystemPrompt updates the room's system prompt in metadata.
// This is separate from room topic (which is display-only).
func (oc *AIClient) setRoomSystemPrompt(ctx context.Context, portal *bridgev2.Portal, prompt string) error {
	return oc.setRoomSystemPromptInternal(ctx, portal, prompt, true)
}

func (oc *AIClient) setRoomSystemPromptNoSave(ctx context.Context, portal *bridgev2.Portal, prompt string) error {
	return oc.setRoomSystemPromptInternal(ctx, portal, prompt, false)
}

func (oc *AIClient) setRoomSystemPromptInternal(ctx context.Context, portal *bridgev2.Portal, prompt string, save bool) error {
	if portal.MXID == "" {
		return fmt.Errorf("portal has no Matrix room ID")
	}

	meta := portalMeta(portal)
	meta.SystemPrompt = prompt

	if save {
		if err := portal.Save(ctx); err != nil {
			return fmt.Errorf("failed to save portal: %w", err)
		}
		oc.log.Debug().Str("prompt_len", fmt.Sprintf("%d", len(prompt))).Msg("Set room system prompt")
	}
	return nil
}
