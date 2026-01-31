package connector

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/rs/zerolog"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/ai-bridge/pkg/aiid"
)

const (
	maxRetryAttempts = 3 // Maximum retry attempts for context length errors
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

	// generatedImage tracks a pending image from image generation
	type generatedImage struct {
		itemID   string
		imageB64 string
	}

	// Track streaming state
	var (
		accumulated    strings.Builder
		reasoning      strings.Builder
		firstToken     = true
		initialEventID id.EventID
		finishReason   string
		sequenceNum    int              // Global sequence number for ordering all stream events
		responseID     string           // Capture response ID for persistence and context chaining
		pendingImages  []generatedImage // Images generated during the response
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
				// Ensure ghost display name is set before sending the first message
				oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
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

		case "response.image_generation_call.in_progress", "response.image_generation_call.generating":
			// Image generation in progress
			if initialEventID != "" {
				sequenceNum++
				oc.emitStreamEvent(ctx, portal, initialEventID, StreamContentToolCall, "", sequenceNum, map[string]any{
					"tool_name": "image_generation",
					"item_id":   streamEvent.ItemID,
					"status":    "generating",
				})
			}
			log.Debug().Str("item_id", streamEvent.ItemID).Msg("Image generation in progress")

		case "response.image_generation_call.completed":
			// Image generation completed - the actual image data will be in response.completed
			if initialEventID != "" {
				sequenceNum++
				oc.emitStreamEvent(ctx, portal, initialEventID, StreamContentToolResult, "", sequenceNum, map[string]any{
					"tool_name": "image_generation",
					"item_id":   streamEvent.ItemID,
					"status":    "completed",
				})
			}
			log.Info().Str("item_id", streamEvent.ItemID).Msg("Image generation completed")

		case "response.completed":
			if streamEvent.Response.Status == "completed" {
				finishReason = "stop"
			} else {
				finishReason = string(streamEvent.Response.Status)
			}
			// Capture response ID for persistence (will save to DB and portal after streaming completes)
			if streamEvent.Response.ID != "" {
				responseID = streamEvent.Response.ID
			}

			// Extract any generated images from response output
			for _, output := range streamEvent.Response.Output {
				if output.Type == "image_generation_call" {
					imgOutput := output.AsImageGenerationCall()
					if imgOutput.Status == "completed" && imgOutput.Result != "" {
						pendingImages = append(pendingImages, generatedImage{
							itemID:   imgOutput.ID,
							imageB64: imgOutput.Result,
						})
						log.Debug().Str("item_id", imgOutput.ID).Msg("Captured generated image from response")
					}
				}
			}

			log.Debug().Str("reason", finishReason).Str("response_id", responseID).Int("images", len(pendingImages)).Msg("Response stream completed")

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

		// Save assistant message to database for history reconstruction
		// This is CRITICAL - without this, the AI only sees user messages and tries to answer all of them
		modelID := oc.effectiveModel(meta)
		assistantMsg := &database.Message{
			ID:        aiid.MakeMessageID(initialEventID),
			Room:      portal.PortalKey,
			SenderID:  modelUserID(modelID),
			MXID:      initialEventID,
			Timestamp: time.Now(),
			Metadata: &MessageMetadata{
				Role:         "assistant",
				Body:         accumulated.String(),
				CompletionID: responseID,
				FinishReason: finishReason,
				Model:        modelID,
			},
		}
		if err := oc.UserLogin.Bridge.DB.Message.Insert(ctx, assistantMsg); err != nil {
			log.Warn().Err(err).Msg("Failed to save assistant message to database")
		} else {
			log.Debug().Str("msg_id", string(assistantMsg.ID)).Msg("Saved assistant message to database")
		}

		// Save LastResponseID for "responses" mode context chaining AFTER message persistence
		// This ensures we don't save an ID for a message that failed to persist
		if meta.ConversationMode == "responses" && responseID != "" {
			meta.LastResponseID = responseID
			if err := portal.Save(ctx); err != nil {
				log.Warn().Err(err).Msg("Failed to save portal after storing response ID")
			}
		}
	}

	// Send any generated images as separate messages
	for _, img := range pendingImages {
		imageData, mimeType, err := decodeBase64Image(img.imageB64)
		if err != nil {
			log.Warn().Err(err).Str("item_id", img.itemID).Msg("Failed to decode generated image")
			continue
		}
		eventID, err := oc.sendGeneratedImage(ctx, portal, imageData, mimeType)
		if err != nil {
			log.Warn().Err(err).Str("item_id", img.itemID).Msg("Failed to send generated image to Matrix")
			continue
		}
		log.Info().Stringer("event_id", eventID).Str("item_id", img.itemID).Msg("Sent generated image to Matrix")
	}

	log.Info().
		Str("finish_reason", finishReason).
		Int("content_length", accumulated.Len()).
		Int("reasoning_length", reasoning.Len()).
		Str("response_id", responseID).
		Int("images_sent", len(pendingImages)).
		Msg("Responses API streaming finished")

	// Generate room title after first response
	oc.maybeGenerateTitle(ctx, portal, accumulated.String())

	return true, nil
}

// convertToResponsesInput converts Chat Completion messages to Responses API input items
// NOTE: This currently extracts text content only. For multimodal messages (images, PDFs, audio, video),
// the media URL/description is appended to the text content. Full multimodal support for Responses API
// would require using the native array content format which may not be fully supported.
func (oc *AIClient) convertToResponsesInput(messages []openai.ChatCompletionMessageParamUnion, _ *PortalMetadata) responses.ResponseInputParam {
	var input responses.ResponseInputParam

	for _, msg := range messages {
		// Use shared helper to extract content and role (avoids JSON roundtrip)
		content, role := extractMessageContent(msg)

		// Also extract any multimodal content descriptions for user messages
		if msg.OfUser != nil && msg.OfUser.Content.OfArrayOfContentParts != nil {
			var extraContent []string
			for _, part := range msg.OfUser.Content.OfArrayOfContentParts {
				if part.OfImageURL != nil && part.OfImageURL.ImageURL.URL != "" {
					extraContent = append(extraContent, "[Image: "+part.OfImageURL.ImageURL.URL+"]")
				}
				if part.OfInputAudio != nil {
					extraContent = append(extraContent, "[Audio content attached]")
				}
				if part.OfFile != nil {
					extraContent = append(extraContent, "[File content attached]")
				}
			}
			if len(extraContent) > 0 {
				content = content + "\n" + strings.Join(extraContent, "\n")
			}
		}

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
	// Skip reserved keys that metadata cannot overwrite
	reservedKeys := map[string]bool{
		"body": true, "content_type": true, "seq": true, "m.relates_to": true,
	}
	for k, v := range metadata {
		if !reservedKeys[k] {
			eventContent.Raw[k] = v
		}
	}
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

	// Check for rate limit errors - send transient disconnect
	if IsRateLimitError(err) {
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

	// Map error to state code for user feedback
	errorCode := MapErrorToStateCode(err)
	errorMessage := "Failed to reach OpenAI"
	if humanErr, ok := BridgeStateHumanErrors[errorCode]; ok {
		errorMessage = humanErr
	}

	msgStatus := bridgev2.WrapErrorInStatus(err).
		WithStatus(event.MessageStatusRetriable).
		WithMessage(errorMessage).
		WithIsCertain(true).
		WithSendNotice(true)
	portal.Bridge.Matrix.SendMessageStatus(ctx, &msgStatus, bridgev2.StatusEventInfoFromEvent(evt))
}

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

// decodeBase64Image decodes a base64-encoded image and detects its MIME type
func decodeBase64Image(b64Data string) ([]byte, string, error) {
	data, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return nil, "", fmt.Errorf("base64 decode failed: %w", err)
	}
	mimeType := http.DetectContentType(data)
	// Fallback to PNG if detection fails (common for AI-generated images)
	if mimeType == "application/octet-stream" {
		mimeType = "image/png"
	}
	return data, mimeType, nil
}

// sendGeneratedImage uploads an AI-generated image to Matrix and sends it as a message
func (oc *AIClient) sendGeneratedImage(
	ctx context.Context,
	portal *bridgev2.Portal,
	imageData []byte,
	mimeType string,
) (id.EventID, error) {
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return "", fmt.Errorf("failed to get model intent")
	}

	// Generate filename based on timestamp and mime type
	ext := "png"
	switch mimeType {
	case "image/jpeg":
		ext = "jpg"
	case "image/webp":
		ext = "webp"
	case "image/gif":
		ext = "gif"
	}
	fileName := fmt.Sprintf("generated-%d.%s", time.Now().UnixMilli(), ext)

	// Upload to Matrix
	uri, file, err := intent.UploadMedia(ctx, portal.MXID, imageData, fileName, mimeType)
	if err != nil {
		return "", fmt.Errorf("upload failed: %w", err)
	}

	// Build image message content
	content := &event.MessageEventContent{
		MsgType: event.MsgImage,
		Body:    fileName,
		Info: &event.FileInfo{
			MimeType: mimeType,
			Size:     len(imageData),
		},
	}
	if file != nil {
		content.File = file
	} else {
		content.URL = uri
	}

	// Send message
	resp, err := intent.SendMessage(ctx, portal.MXID, event.EventMessage, &event.Content{Parsed: content}, nil)
	if err != nil {
		return "", fmt.Errorf("send failed: %w", err)
	}
	return resp.EventID, nil
}

// sendWelcomeMessage sends a welcome message when a new chat is created
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

	modelID := oc.effectiveModel(meta)
	modelName := FormatModelDisplay(modelID)
	body := fmt.Sprintf("This chat was created automatically. Send a message to start talking to %s.", modelName)

	// Ensure ghost display name is set before sending welcome message
	oc.ensureGhostDisplayName(ctx, modelID)

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
