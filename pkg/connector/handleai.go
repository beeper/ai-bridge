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
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"
)

const (
	maxRetryAttempts = 3 // Maximum retry attempts for context length errors
)

// streamingState tracks the state of a streaming response
type streamingState struct {
	turnID         string
	agentID        string
	startedAtMs    int64
	firstTokenAtMs int64
	completedAtMs  int64

	baseInput              responses.ResponseInputParam
	accumulated            strings.Builder
	reasoning              strings.Builder
	toolCalls              []ToolCallMetadata
	pendingImages          []generatedImage
	pendingFunctionOutputs []functionCallOutput // Function outputs to send back to API for continuation
	initialEventID         id.EventID
	finishReason           string
	responseID             string
	sequenceNum            int
	firstToken             bool
	skipNextTextDelta      bool
}

// newStreamingState creates a new streaming state with initialized fields
func newStreamingState(meta *PortalMetadata) *streamingState {
	return &streamingState{
		turnID:      NewTurnID(),
		agentID:     meta.DefaultAgentID,
		startedAtMs: time.Now().UnixMilli(),
		firstToken:  true,
	}
}

// generatedImage tracks a pending image from image generation
type generatedImage struct {
	itemID   string
	imageB64 string
	turnID   string
}

// activeToolCall tracks a tool call that's in progress
type activeToolCall struct {
	callID      string
	toolName    string
	toolType    ToolType
	input       strings.Builder
	startedAtMs int64
	eventID     id.EventID // Event ID of the tool call timeline event
	result      string     // Result from tool execution (for continuation)
	itemID      string     // Item ID from the stream event (used as call_id for continuation)
}

// functionCallOutput tracks a completed function call output for API continuation
type functionCallOutput struct {
	callID    string // The ItemID from the stream event (used as call_id in continuation)
	name      string // Tool name (for stateless continuations)
	arguments string // Raw arguments JSON (for stateless continuations)
	output    string // The result from executing the tool
}

// saveAssistantMessage saves the completed assistant message to the database
func (oc *AIClient) saveAssistantMessage(
	ctx context.Context,
	log zerolog.Logger,
	portal *bridgev2.Portal,
	state *streamingState,
	meta *PortalMetadata,
) {
	modelID := oc.effectiveModel(meta)
	assistantMsg := &database.Message{
		ID:        MakeMessageID(state.initialEventID),
		Room:      portal.PortalKey,
		SenderID:  modelUserID(modelID),
		MXID:      state.initialEventID,
		Timestamp: time.Now(),
		Metadata: &MessageMetadata{
			Role:           "assistant",
			Body:           state.accumulated.String(),
			CompletionID:   state.responseID,
			FinishReason:   state.finishReason,
			Model:          modelID,
			TurnID:         state.turnID,
			AgentID:        state.agentID,
			ToolCalls:      state.toolCalls,
			StartedAtMs:    state.startedAtMs,
			FirstTokenAtMs: state.firstTokenAtMs,
			CompletedAtMs:  state.completedAtMs,
			HasToolCalls:   len(state.toolCalls) > 0,
			// Reasoning fields (only populated by Responses API)
			ThinkingContent:    state.reasoning.String(),
			ThinkingTokenCount: len(strings.Fields(state.reasoning.String())),
		},
	}
	if err := oc.UserLogin.Bridge.DB.Message.Insert(ctx, assistantMsg); err != nil {
		log.Warn().Err(err).Msg("Failed to save assistant message to database")
	} else {
		log.Debug().Str("msg_id", string(assistantMsg.ID)).Msg("Saved assistant message to database")
	}

	// Save LastResponseID for "responses" mode context chaining (OpenAI-only)
	if meta.ConversationMode == "responses" && state.responseID != "" && !oc.isOpenRouterProvider() {
		meta.LastResponseID = state.responseID
		if err := portal.Save(ctx); err != nil {
			log.Warn().Err(err).Msg("Failed to save portal after storing response ID")
		}
	}
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
	// OpenRouter multimodal inputs are handled via Chat Completions.
	if oc.isOpenRouterProvider() && hasMultimodalContent(prompt) {
		oc.responseWithRetry(ctx, evt, portal, meta, prompt, oc.streamChatCompletions, "chat_completions")
		return
	}
	// Use Chat Completions API for audio (native support)
	// SDK v3.16.0 has ResponseInputAudioParam but it's not wired into the union
	if hasAudioContent(prompt) {
		oc.responseWithRetry(ctx, evt, portal, meta, prompt, oc.streamChatCompletions, "chat_completions")
		return
	}
	// Use Responses API for other content (images, files, text)
	oc.responseWithRetry(ctx, evt, portal, meta, prompt, oc.streamingResponse, "responses")
}

// buildResponsesAPIParams creates common Responses API parameters for both streaming and non-streaming paths
func (oc *AIClient) buildResponsesAPIParams(ctx context.Context, meta *PortalMetadata, messages []openai.ChatCompletionMessageParamUnion) responses.ResponseNewParams {
	log := zerolog.Ctx(ctx)

	params := responses.ResponseNewParams{
		Model:           shared.ResponsesModel(oc.effectiveModelForAPI(meta)),
		MaxOutputTokens: openai.Int(int64(oc.effectiveMaxTokens(meta))),
	}

	systemPrompt := oc.effectivePrompt(meta)

	// Use previous_response_id if in "responses" mode and ID exists.
	// OpenRouter's Responses API is stateless, so always send full history there.
	usePreviousResponse := meta.ConversationMode == "responses" && meta.LastResponseID != "" && !oc.isOpenRouterProvider()
	if usePreviousResponse {
		params.PreviousResponseID = openai.String(meta.LastResponseID)
		if systemPrompt != "" {
			params.Instructions = openai.String(systemPrompt)
		}
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

	// Add reasoning effort if configured (uses inheritance: room → user → default)
	if reasoningEffort := oc.effectiveReasoningEffort(meta); reasoningEffort != "" {
		params.Reasoning = shared.ReasoningParam{
			Effort: shared.ReasoningEffort(reasoningEffort),
		}
	}

	// Add built-in provider tools if enabled (only for native OpenAI, not OpenRouter)
	// OpenRouter's Responses API only supports function-type tools
	isOpenRouter := oc.isOpenRouterProvider()
	log.Debug().
		Bool("is_openrouter", isOpenRouter).
		Str("detected_provider", loginMetadata(oc.UserLogin).Provider).
		Msg("Provider detection for tool filtering")

	if !isOpenRouter {
		if oc.isToolEnabled(meta, ToolNameWebSearchProvider) {
			params.Tools = append(params.Tools, responses.ToolParamOfWebSearchPreview(responses.WebSearchPreviewToolTypeWebSearchPreview))
			log.Debug().Msg("Web search tool enabled")
		}
		if oc.isToolEnabled(meta, ToolNameCodeInterpreter) {
			params.Tools = append(params.Tools, responses.ToolParamOfCodeInterpreter("auto"))
			log.Debug().Msg("Code interpreter tool enabled")
		}
	}

	// Add builtin function tools if model supports tool calling
	if meta.Capabilities.SupportsToolCalling {
		enabledTools := GetEnabledBuiltinTools(func(name string) bool {
			return oc.isToolEnabled(meta, name)
		})
		if len(enabledTools) > 0 {
			params.Tools = append(params.Tools, ToOpenAITools(enabledTools)...)
			log.Debug().Int("count", len(enabledTools)).Msg("Added builtin function tools")
		}
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

	// Ensure model ghost is in the room before any operations
	if err := oc.ensureModelInRoom(ctx, portal); err != nil {
		log.Warn().Err(err).Msg("Failed to ensure model is in room")
		// Continue anyway - typing will fail gracefully
	}

	// Set typing indicator when streaming starts
	oc.setModelTyping(ctx, portal, true)
	defer oc.setModelTyping(ctx, portal, false)

	// Build Responses API params using shared helper
	params := oc.buildResponsesAPIParams(ctx, meta, messages)

	// Inject per-room PDF engine into context for OpenRouter/Beeper providers
	if oc.isOpenRouterProvider() {
		ctx = WithPDFEngine(ctx, oc.effectivePDFEngine(meta))
	}

	stream := oc.api.Responses.NewStreaming(ctx, params)
	if stream == nil {
		log.Error().Msg("Failed to create Responses API streaming request")
		oc.notifyMatrixSendFailure(ctx, portal, evt, fmt.Errorf("responses streaming not available"))
		return false, nil
	}

	// Initialize streaming state with turn tracking
	state := newStreamingState(meta)

	// Store base input for OpenRouter stateless continuations
	if params.Input.OfInputItemList != nil {
		state.baseInput = params.Input.OfInputItemList
	}

	ensureInitialEvent := func() bool {
		if state.initialEventID != "" {
			return true
		}
		state.firstToken = false
		state.firstTokenAtMs = time.Now().UnixMilli()
		oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
		state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, "...", state.turnID)
		if state.initialEventID == "" {
			log.Error().Msg("Failed to send initial streaming message for tool call")
			return false
		}
		return true
	}

	// Track active tool calls
	activeTools := make(map[string]*activeToolCall)

	// Emit generation status: starting
	oc.emitGenerationStatus(ctx, portal, state, "starting", "Starting generation...", nil)

	// Process stream events - no debouncing, stream every delta immediately
	for stream.Next() {
		streamEvent := stream.Current()

		switch streamEvent.Type {
		case "response.output_text.delta":
			state.accumulated.WriteString(streamEvent.Delta)

			// First token - send initial message synchronously to capture event_id
			if state.firstToken && state.accumulated.Len() > 0 {
				state.firstToken = false
				state.firstTokenAtMs = time.Now().UnixMilli()
				// Ensure ghost display name is set before sending the first message
				oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
				state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, state.accumulated.String(), state.turnID)
				if state.initialEventID == "" {
					log.Error().Msg("Failed to send initial streaming message")
					return false, nil
				}
				// Initial message already includes this delta
				state.skipNextTextDelta = true
				// Update status to generating
				oc.emitGenerationStatus(ctx, portal, state, "generating", "Generating response...", nil)
			}

			// Stream every text delta immediately (no debouncing for snappy UX)
			if !state.firstToken && state.initialEventID != "" {
				if state.skipNextTextDelta {
					state.skipNextTextDelta = false
					break
				}
				state.sequenceNum++
				oc.emitStreamDelta(ctx, portal, state, StreamContentText, streamEvent.Delta, nil)
			}

		case "response.reasoning_text.delta":
			state.reasoning.WriteString(streamEvent.Delta)

			// Check if this is first content (reasoning before text)
			if state.firstToken && state.reasoning.Len() > 0 {
				state.firstToken = false
				state.firstTokenAtMs = time.Now().UnixMilli()
				oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
				// Send empty initial message - will be replaced with content later
				state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, "...", state.turnID)
				if state.initialEventID == "" {
					log.Error().Msg("Failed to send initial streaming message")
					return false, nil
				}
				// Update status to thinking
				oc.emitGenerationStatus(ctx, portal, state, "thinking", "Thinking...", nil)
			}

			// Stream reasoning tokens in real-time
			if !state.firstToken && state.initialEventID != "" {
				state.sequenceNum++
				oc.emitStreamDelta(ctx, portal, state, StreamContentReasoning, streamEvent.Delta, nil)
			}

		case "response.function_call_arguments.delta":
			// Get or create active tool call
			tool, exists := activeTools[streamEvent.ItemID]
			if !exists {
				if !ensureInitialEvent() {
					return false, nil
				}
				tool = &activeToolCall{
					callID:      NewCallID(),
					toolName:    streamEvent.Name,
					toolType:    ToolTypeFunction,
					startedAtMs: time.Now().UnixMilli(),
				}
				activeTools[streamEvent.ItemID] = tool

				// Send tool call timeline event
				tool.eventID = oc.sendToolCallEvent(ctx, portal, state, tool)

				// Update status to tool_use
				oc.emitGenerationStatus(ctx, portal, state, "tool_use", fmt.Sprintf("Calling %s...", streamEvent.Name), &GenerationDetails{
					CurrentTool: streamEvent.Name,
					CallID:      tool.callID,
				})
			}

			// Accumulate arguments
			tool.input.WriteString(streamEvent.Delta)

			// Stream function call arguments as they arrive
			if state.initialEventID != "" {
				state.sequenceNum++
				oc.emitStreamDelta(ctx, portal, state, StreamContentToolInput, streamEvent.Delta, map[string]any{
					"call_id":   tool.callID,
					"tool_name": streamEvent.Name,
				})
			}

		case "response.function_call_arguments.done":
			// Function call complete - execute the tool and send result
			tool, exists := activeTools[streamEvent.ItemID]
			if !exists {
				// Create tool if we missed the delta events
				if !ensureInitialEvent() {
					return false, nil
				}
				tool = &activeToolCall{
					callID:      NewCallID(),
					toolName:    streamEvent.Name,
					toolType:    ToolTypeFunction,
					startedAtMs: time.Now().UnixMilli(),
				}
				tool.input.WriteString(streamEvent.Arguments)
				activeTools[streamEvent.ItemID] = tool
			}

			// Store the item ID for continuation (this is the call_id for the Responses API)
			tool.itemID = streamEvent.ItemID

			if !ensureInitialEvent() {
				return false, nil
			}

			resultStatus := ResultStatusSuccess
			var result string
			if !oc.isToolEnabled(meta, streamEvent.Name) {
				resultStatus = ResultStatusError
				result = fmt.Sprintf("Error: tool %s is disabled", streamEvent.Name)
			} else {
				// Wrap context with bridge info for tools that need it (e.g., set_room_title)
				toolCtx := WithBridgeToolContext(ctx, &BridgeToolContext{
					Client: oc,
					Portal: portal,
					Meta:   meta,
				})
				var err error
				result, err = oc.executeBuiltinTool(toolCtx, streamEvent.Name, streamEvent.Arguments)
				if err != nil {
					log.Warn().Err(err).Str("tool", streamEvent.Name).Msg("Tool execution failed")
					result = fmt.Sprintf("Error: %s", err.Error())
					resultStatus = ResultStatusError
				}
			}

			// Store result for API continuation
			tool.result = result
			args := strings.TrimSpace(tool.input.String())
			if args == "" {
				args = strings.TrimSpace(streamEvent.Arguments)
			}
			if args == "" {
				args = "{}"
			}
			name := strings.TrimSpace(tool.toolName)
			if name == "" {
				name = strings.TrimSpace(streamEvent.Name)
			}
			state.pendingFunctionOutputs = append(state.pendingFunctionOutputs, functionCallOutput{
				callID:    streamEvent.ItemID,
				name:      name,
				arguments: args,
				output:    result,
			})

			// Parse input for storage
			var inputMap map[string]any
			_ = json.Unmarshal([]byte(streamEvent.Arguments), &inputMap)

			// Send tool result timeline event
			resultEventID := oc.sendToolResultEvent(ctx, portal, state, tool, result, resultStatus)

			// Track tool call in metadata
			state.toolCalls = append(state.toolCalls, ToolCallMetadata{
				CallID:        tool.callID,
				ToolName:      streamEvent.Name,
				ToolType:      string(tool.toolType),
				Input:         inputMap,
				Output:        map[string]any{"result": result},
				Status:        string(ToolStatusCompleted),
				ResultStatus:  string(resultStatus),
				StartedAtMs:   tool.startedAtMs,
				CompletedAtMs: time.Now().UnixMilli(),
				CallEventID:   tool.eventID.String(),
				ResultEventID: resultEventID.String(),
			})

			// Stream result
			state.sequenceNum++
			oc.emitStreamDelta(ctx, portal, state, StreamContentToolResult, result, map[string]any{
				"call_id":   tool.callID,
				"tool_name": streamEvent.Name,
				"status":    "completed",
			})

			// Update status back to generating
			oc.emitGenerationStatus(ctx, portal, state, "generating", "Continuing generation...", nil)

		case "response.web_search_call.searching":
			// Web search starting
			tool := &activeToolCall{
				callID:      NewCallID(),
				toolName:    "web_search",
				toolType:    ToolTypeProvider,
				startedAtMs: time.Now().UnixMilli(),
			}
			activeTools[streamEvent.ItemID] = tool

			// Send tool call timeline event
			tool.eventID = oc.sendToolCallEvent(ctx, portal, state, tool)

			// Update status
			oc.emitGenerationStatus(ctx, portal, state, "tool_use", "Searching the web...", &GenerationDetails{
				CurrentTool: "web_search",
				CallID:      tool.callID,
			})

			// Emit tool progress
			oc.emitToolProgress(ctx, portal, state, tool, ToolStatusRunning, "Searching...", 0)

		case "response.web_search_call.completed":
			// Web search completed
			tool, exists := activeTools[streamEvent.ItemID]
			if exists {
				// Send tool result timeline event
				resultEventID := oc.sendToolResultEvent(ctx, portal, state, tool, "", ResultStatusSuccess)

				// Track tool call
				state.toolCalls = append(state.toolCalls, ToolCallMetadata{
					CallID:        tool.callID,
					ToolName:      "web_search",
					ToolType:      string(tool.toolType),
					Status:        string(ToolStatusCompleted),
					ResultStatus:  string(ResultStatusSuccess),
					StartedAtMs:   tool.startedAtMs,
					CompletedAtMs: time.Now().UnixMilli(),
					CallEventID:   tool.eventID.String(),
					ResultEventID: resultEventID.String(),
				})

				// Emit completion
				state.sequenceNum++
				oc.emitStreamDelta(ctx, portal, state, StreamContentToolResult, "", map[string]any{
					"call_id":   tool.callID,
					"tool_name": "web_search",
					"status":    "completed",
				})
			}

			// Update status back to generating
			oc.emitGenerationStatus(ctx, portal, state, "generating", "Continuing generation...", nil)

		case "response.image_generation_call.in_progress", "response.image_generation_call.generating":
			// Image generation in progress
			tool, exists := activeTools[streamEvent.ItemID]
			if !exists {
				tool = &activeToolCall{
					callID:      NewCallID(),
					toolName:    "image_generation",
					toolType:    ToolTypeProvider,
					startedAtMs: time.Now().UnixMilli(),
				}
				activeTools[streamEvent.ItemID] = tool

				// Send tool call timeline event
				tool.eventID = oc.sendToolCallEvent(ctx, portal, state, tool)
			}

			// Update status
			oc.emitGenerationStatus(ctx, portal, state, "image_generating", "Generating image...", &GenerationDetails{
				CurrentTool: "image_generation",
				CallID:      tool.callID,
			})

			// Emit tool progress
			oc.emitToolProgress(ctx, portal, state, tool, ToolStatusRunning, "Generating image...", 50)

			log.Debug().Str("item_id", streamEvent.ItemID).Msg("Image generation in progress")

		case "response.image_generation_call.completed":
			// Image generation completed - the actual image data will be in response.completed
			tool, exists := activeTools[streamEvent.ItemID]
			if exists {
				// Track tool call
				state.toolCalls = append(state.toolCalls, ToolCallMetadata{
					CallID:        tool.callID,
					ToolName:      "image_generation",
					ToolType:      string(tool.toolType),
					Status:        string(ToolStatusCompleted),
					ResultStatus:  string(ResultStatusSuccess),
					StartedAtMs:   tool.startedAtMs,
					CompletedAtMs: time.Now().UnixMilli(),
					CallEventID:   tool.eventID.String(),
				})

				state.sequenceNum++
				oc.emitStreamDelta(ctx, portal, state, StreamContentToolResult, "", map[string]any{
					"call_id":   tool.callID,
					"tool_name": "image_generation",
					"status":    "completed",
				})
			}
			log.Info().Str("item_id", streamEvent.ItemID).Msg("Image generation completed")

		case "response.completed":
			state.completedAtMs = time.Now().UnixMilli()

			if streamEvent.Response.Status == "completed" {
				state.finishReason = "stop"
			} else {
				state.finishReason = string(streamEvent.Response.Status)
			}
			// Capture response ID for persistence (will save to DB and portal after streaming completes)
			if streamEvent.Response.ID != "" {
				state.responseID = streamEvent.Response.ID
			}

			// Extract any generated images from response output
			for _, output := range streamEvent.Response.Output {
				if output.Type == "image_generation_call" {
					imgOutput := output.AsImageGenerationCall()
					if imgOutput.Status == "completed" && imgOutput.Result != "" {
						state.pendingImages = append(state.pendingImages, generatedImage{
							itemID:   imgOutput.ID,
							imageB64: imgOutput.Result,
							turnID:   state.turnID,
						})
						log.Debug().Str("item_id", imgOutput.ID).Msg("Captured generated image from response")
					}
				}
			}

			// Emit final status
			oc.emitGenerationStatus(ctx, portal, state, "finalizing", "Completing response...", nil)

			log.Debug().Str("reason", state.finishReason).Str("response_id", state.responseID).Int("images", len(state.pendingImages)).Msg("Response stream completed")

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

	// If there are pending function outputs, send them back to the API for continuation
	// This loop continues until the model generates a response without tool calls
	for len(state.pendingFunctionOutputs) > 0 && state.responseID != "" {
		log.Debug().
			Int("pending_outputs", len(state.pendingFunctionOutputs)).
			Str("previous_response_id", state.responseID).
			Msg("Continuing response with function call outputs")

		// Build continuation request with function call outputs
		continuationParams := oc.buildContinuationParams(state, meta)

		// OpenRouter Responses API is stateless; persist tool calls in base input.
		if oc.isOpenRouterProvider() && len(state.baseInput) > 0 {
			for _, output := range state.pendingFunctionOutputs {
				if output.name != "" {
					args := output.arguments
					if strings.TrimSpace(args) == "" {
						args = "{}"
					}
					state.baseInput = append(state.baseInput, responses.ResponseInputItemParamOfFunctionCall(args, output.callID, output.name))
				}
				// Use SDK helper to ensure Type field is properly set
				state.baseInput = append(state.baseInput, responses.ResponseInputItemParamOfFunctionCallOutput(output.callID, output.output))
			}
		}

		// Clear pending outputs (they're being sent)
		state.pendingFunctionOutputs = nil

		// Reset active tools for new iteration
		activeTools = make(map[string]*activeToolCall)

		// Start continuation stream
		stream = oc.api.Responses.NewStreaming(ctx, continuationParams)
		if stream == nil {
			log.Error().Msg("Failed to create continuation streaming request")
			break
		}

		// Process continuation stream events
		for stream.Next() {
			streamEvent := stream.Current()

			switch streamEvent.Type {
			case "response.output_text.delta":
				state.accumulated.WriteString(streamEvent.Delta)
				if !state.firstToken && state.initialEventID != "" {
					state.sequenceNum++
					oc.emitStreamDelta(ctx, portal, state, StreamContentText, streamEvent.Delta, nil)
				}

			case "response.reasoning_text.delta":
				state.reasoning.WriteString(streamEvent.Delta)
				if !state.firstToken && state.initialEventID != "" {
					state.sequenceNum++
					oc.emitStreamDelta(ctx, portal, state, StreamContentReasoning, streamEvent.Delta, nil)
				}

			case "response.function_call_arguments.delta":
				tool, exists := activeTools[streamEvent.ItemID]
				if !exists {
					if !ensureInitialEvent() {
						return false, nil
					}
					tool = &activeToolCall{
						callID:      NewCallID(),
						toolName:    streamEvent.Name,
						toolType:    ToolTypeFunction,
						startedAtMs: time.Now().UnixMilli(),
					}
					activeTools[streamEvent.ItemID] = tool
					tool.eventID = oc.sendToolCallEvent(ctx, portal, state, tool)
					oc.emitGenerationStatus(ctx, portal, state, "tool_use", fmt.Sprintf("Calling %s...", streamEvent.Name), &GenerationDetails{
						CurrentTool: streamEvent.Name,
						CallID:      tool.callID,
					})
				}
				tool.input.WriteString(streamEvent.Delta)

			case "response.function_call_arguments.done":
				tool, exists := activeTools[streamEvent.ItemID]
				if !exists {
					if !ensureInitialEvent() {
						return false, nil
					}
					tool = &activeToolCall{
						callID:      NewCallID(),
						toolName:    streamEvent.Name,
						toolType:    ToolTypeFunction,
						startedAtMs: time.Now().UnixMilli(),
					}
					tool.input.WriteString(streamEvent.Arguments)
					activeTools[streamEvent.ItemID] = tool
				}
				tool.itemID = streamEvent.ItemID

				if !ensureInitialEvent() {
					return false, nil
				}

				resultStatus := ResultStatusSuccess
				var result string
				if !oc.isToolEnabled(meta, streamEvent.Name) {
					resultStatus = ResultStatusError
					result = fmt.Sprintf("Error: tool %s is disabled", streamEvent.Name)
				} else {
					toolCtx := WithBridgeToolContext(ctx, &BridgeToolContext{
						Client: oc,
						Portal: portal,
						Meta:   meta,
					})
					var err error
					result, err = oc.executeBuiltinTool(toolCtx, streamEvent.Name, streamEvent.Arguments)
					if err != nil {
						log.Warn().Err(err).Str("tool", streamEvent.Name).Msg("Tool execution failed (continuation)")
						result = fmt.Sprintf("Error: %s", err.Error())
						resultStatus = ResultStatusError
					}
				}

				tool.result = result
				args := strings.TrimSpace(tool.input.String())
				if args == "" {
					args = strings.TrimSpace(streamEvent.Arguments)
				}
				if args == "" {
					args = "{}"
				}
				name := strings.TrimSpace(tool.toolName)
				if name == "" {
					name = strings.TrimSpace(streamEvent.Name)
				}
				state.pendingFunctionOutputs = append(state.pendingFunctionOutputs, functionCallOutput{
					callID:    streamEvent.ItemID,
					name:      name,
					arguments: args,
					output:    result,
				})

				var inputMap map[string]any
				_ = json.Unmarshal([]byte(streamEvent.Arguments), &inputMap)

				resultEventID := oc.sendToolResultEvent(ctx, portal, state, tool, result, resultStatus)

				state.toolCalls = append(state.toolCalls, ToolCallMetadata{
					CallID:        tool.callID,
					ToolName:      streamEvent.Name,
					ToolType:      string(tool.toolType),
					Input:         inputMap,
					Output:        map[string]any{"result": result},
					Status:        string(ToolStatusCompleted),
					ResultStatus:  string(resultStatus),
					StartedAtMs:   tool.startedAtMs,
					CompletedAtMs: time.Now().UnixMilli(),
					CallEventID:   tool.eventID.String(),
					ResultEventID: resultEventID.String(),
				})

				state.sequenceNum++
				oc.emitStreamDelta(ctx, portal, state, StreamContentToolResult, result, map[string]any{
					"call_id":   tool.callID,
					"tool_name": streamEvent.Name,
					"status":    "completed",
				})
				oc.emitGenerationStatus(ctx, portal, state, "generating", "Continuing generation...", nil)

			case "response.completed":
				state.completedAtMs = time.Now().UnixMilli()
				if streamEvent.Response.Status == "completed" {
					state.finishReason = "stop"
				} else {
					state.finishReason = string(streamEvent.Response.Status)
				}
				if streamEvent.Response.ID != "" {
					state.responseID = streamEvent.Response.ID
				}
				log.Debug().Str("reason", state.finishReason).Str("response_id", state.responseID).Msg("Continuation stream completed")

			case "error":
				log.Error().Str("error", streamEvent.Message).Msg("Continuation stream error")
			}
		}

		if err := stream.Err(); err != nil {
			log.Error().Err(err).Msg("Continuation streaming error")
			break
		}
	}

	// Send final edit to persist complete content with metadata (including reasoning)
	if state.initialEventID != "" {
		oc.sendFinalAssistantTurn(ctx, portal, state, meta)
		oc.saveAssistantMessage(ctx, log, portal, state, meta)
	}

	// Send any generated images as separate messages
	for _, img := range state.pendingImages {
		imageData, mimeType, err := decodeBase64Image(img.imageB64)
		if err != nil {
			log.Warn().Err(err).Str("item_id", img.itemID).Msg("Failed to decode generated image")
			continue
		}
		eventID, err := oc.sendGeneratedImage(ctx, portal, imageData, mimeType, img.turnID)
		if err != nil {
			log.Warn().Err(err).Str("item_id", img.itemID).Msg("Failed to send generated image to Matrix")
			continue
		}
		log.Info().Stringer("event_id", eventID).Str("item_id", img.itemID).Msg("Sent generated image to Matrix")
	}

	log.Info().
		Str("turn_id", state.turnID).
		Str("finish_reason", state.finishReason).
		Int("content_length", state.accumulated.Len()).
		Int("reasoning_length", state.reasoning.Len()).
		Int("tool_calls", len(state.toolCalls)).
		Str("response_id", state.responseID).
		Int("images_sent", len(state.pendingImages)).
		Msg("Responses API streaming finished")

	// Generate room title after first response
	oc.maybeGenerateTitle(ctx, portal, state.accumulated.String())

	return true, nil
}

// buildContinuationParams builds params for continuing a response after tool execution
func (oc *AIClient) buildContinuationParams(state *streamingState, meta *PortalMetadata) responses.ResponseNewParams {
	params := responses.ResponseNewParams{
		Model:           shared.ResponsesModel(oc.effectiveModelForAPI(meta)),
		MaxOutputTokens: openai.Int(int64(oc.effectiveMaxTokens(meta))),
	}

	if systemPrompt := oc.effectivePrompt(meta); systemPrompt != "" {
		params.Instructions = openai.String(systemPrompt)
	}

	isOpenRouter := oc.isOpenRouterProvider()
	if !isOpenRouter {
		params.PreviousResponseID = openai.String(state.responseID)
	}

	// Build function call outputs as input
	var input responses.ResponseInputParam
	if isOpenRouter && len(state.baseInput) > 0 {
		// OpenRouter Responses API is stateless: include full history plus tool calls.
		input = append(input, state.baseInput...)
	}
	for _, output := range state.pendingFunctionOutputs {
		if isOpenRouter && output.name != "" {
			args := output.arguments
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			input = append(input, responses.ResponseInputItemParamOfFunctionCall(args, output.callID, output.name))
		}
		// Use SDK helper to ensure Type field is properly set
		input = append(input, responses.ResponseInputItemParamOfFunctionCallOutput(output.callID, output.output))
	}
	params.Input = responses.ResponseNewParamsInputUnion{
		OfInputItemList: input,
	}

	// Add reasoning effort if configured
	if reasoningEffort := oc.effectiveReasoningEffort(meta); reasoningEffort != "" {
		params.Reasoning = shared.ReasoningParam{
			Effort: shared.ReasoningEffort(reasoningEffort),
		}
	}

	// Add tools (same as initial request)
	if !oc.isOpenRouterProvider() {
		if oc.isToolEnabled(meta, ToolNameWebSearchProvider) {
			params.Tools = append(params.Tools, responses.ToolParamOfWebSearchPreview(responses.WebSearchPreviewToolTypeWebSearchPreview))
		}
		if oc.isToolEnabled(meta, ToolNameCodeInterpreter) {
			params.Tools = append(params.Tools, responses.ToolParamOfCodeInterpreter("auto"))
		}
	}
	if meta.Capabilities.SupportsToolCalling {
		enabledTools := GetEnabledBuiltinTools(func(name string) bool {
			return oc.isToolEnabled(meta, name)
		})
		if len(enabledTools) > 0 {
			params.Tools = append(params.Tools, ToOpenAITools(enabledTools)...)
		}
	}

	return params
}

// streamChatCompletions handles streaming using Chat Completions API (for audio support)
// This is used as a fallback when the prompt contains audio content, since
// SDK v3.16.0 has ResponseInputAudioParam defined but NOT wired into ResponseInputContentUnionParam.
func (oc *AIClient) streamChatCompletions(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	messages []openai.ChatCompletionMessageParamUnion,
) (bool, *ContextLengthError) {
	log := zerolog.Ctx(ctx).With().
		Str("action", "stream_chat_completions").
		Str("portal", string(portal.ID)).
		Logger()

	// Ensure model ghost is in the room before any operations
	if err := oc.ensureModelInRoom(ctx, portal); err != nil {
		log.Warn().Err(err).Msg("Failed to ensure model is in room")
		// Continue anyway - typing will fail gracefully
	}

	oc.setModelTyping(ctx, portal, true)
	defer oc.setModelTyping(ctx, portal, false)

	params := openai.ChatCompletionNewParams{
		Model:    oc.effectiveModelForAPI(meta),
		Messages: messages,
	}
	if maxTokens := oc.effectiveMaxTokens(meta); maxTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(maxTokens))
	}
	if temp := oc.effectiveTemperature(meta); temp > 0 {
		params.Temperature = openai.Float(temp)
	}

	stream := oc.api.Chat.Completions.NewStreaming(ctx, params)
	if stream == nil {
		log.Error().Msg("Failed to create Chat Completions streaming request")
		oc.notifyMatrixSendFailure(ctx, portal, evt, fmt.Errorf("chat completions streaming not available"))
		return false, nil
	}

	state := newStreamingState(meta)

	// Track active tool calls by index
	activeTools := make(map[int]*activeToolCall)

	oc.emitGenerationStatus(ctx, portal, state, "starting", "Starting generation...", nil)

	for stream.Next() {
		chunk := stream.Current()

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				state.accumulated.WriteString(choice.Delta.Content)

				if state.firstToken && state.accumulated.Len() > 0 {
					state.firstToken = false
					state.firstTokenAtMs = time.Now().UnixMilli()
					oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
					state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, state.accumulated.String(), state.turnID)
					if state.initialEventID == "" {
						log.Error().Msg("Failed to send initial streaming message")
						return false, nil
					}
					// Initial message already includes this delta
					state.skipNextTextDelta = true
					oc.emitGenerationStatus(ctx, portal, state, "generating", "Generating response...", nil)
				}

				if !state.firstToken && state.initialEventID != "" {
					if state.skipNextTextDelta {
						state.skipNextTextDelta = false
						continue
					}
					state.sequenceNum++
					oc.emitStreamDelta(ctx, portal, state, StreamContentText, choice.Delta.Content, nil)
				}
			}

			// Handle tool calls from Chat Completions API
			for _, toolDelta := range choice.Delta.ToolCalls {
				toolIdx := int(toolDelta.Index)
				tool, exists := activeTools[toolIdx]
				if !exists {
					tool = &activeToolCall{
						callID:      NewCallID(),
						toolType:    ToolTypeFunction,
						startedAtMs: time.Now().UnixMilli(),
					}
					activeTools[toolIdx] = tool
				}

				// Capture tool ID if provided (used by OpenAI for tracking)
				if toolDelta.ID != "" && tool.callID == "" {
					tool.callID = toolDelta.ID
				}

				// Update tool name if provided in this delta
				if toolDelta.Function.Name != "" {
					tool.toolName = toolDelta.Function.Name

					// Ensure we have an initial message for tool call events
					if state.firstToken {
						state.firstToken = false
						state.firstTokenAtMs = time.Now().UnixMilli()
						oc.ensureGhostDisplayName(ctx, oc.effectiveModel(meta))
						state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, "...", state.turnID)
						if state.initialEventID == "" {
							log.Error().Msg("Failed to send initial streaming message for tool call")
							return false, nil
						}
					}

					// Send tool call timeline event on first delta with name
					if tool.eventID == "" {
						tool.eventID = oc.sendToolCallEvent(ctx, portal, state, tool)
						oc.emitGenerationStatus(ctx, portal, state, "tool_use",
							fmt.Sprintf("Calling %s...", tool.toolName), &GenerationDetails{
								CurrentTool: tool.toolName,
								CallID:      tool.callID,
							})
					}
				}

				// Accumulate arguments
				if toolDelta.Function.Arguments != "" {
					tool.input.WriteString(toolDelta.Function.Arguments)

					// Stream tool input delta
					if state.initialEventID != "" {
						state.sequenceNum++
						oc.emitStreamDelta(ctx, portal, state, StreamContentToolInput,
							toolDelta.Function.Arguments, map[string]any{
								"call_id":   tool.callID,
								"tool_name": tool.toolName,
							})
					}
				}
			}

			if choice.FinishReason != "" {
				state.finishReason = string(choice.FinishReason)
			}
		}
	}

	if err := stream.Err(); err != nil {
		if cle := ParseContextLengthError(err); cle != nil {
			return false, cle
		}
		log.Error().Err(err).Msg("Chat Completions stream error")
		oc.notifyMatrixSendFailure(ctx, portal, evt, err)
		return false, nil
	}

	// Execute any accumulated tool calls
	for _, tool := range activeTools {
		if tool.input.Len() > 0 && tool.toolName != "" && oc.isToolEnabled(meta, tool.toolName) {
			// Wrap context with bridge info for tools that need it (e.g., set_room_title)
			toolCtx := WithBridgeToolContext(ctx, &BridgeToolContext{
				Client: oc,
				Portal: portal,
				Meta:   meta,
			})
			result, err := oc.executeBuiltinTool(toolCtx, tool.toolName, tool.input.String())
			resultStatus := ResultStatusSuccess
			if err != nil {
				log.Warn().Err(err).Str("tool", tool.toolName).Msg("Tool execution failed (Chat Completions)")
				result = fmt.Sprintf("Error: %s", err.Error())
				resultStatus = ResultStatusError
			}

			// Parse input for storage
			var inputMap map[string]any
			_ = json.Unmarshal([]byte(tool.input.String()), &inputMap)

			// Send tool result timeline event
			resultEventID := oc.sendToolResultEvent(ctx, portal, state, tool, result, resultStatus)

			// Track tool call in metadata
			state.toolCalls = append(state.toolCalls, ToolCallMetadata{
				CallID:        tool.callID,
				ToolName:      tool.toolName,
				ToolType:      string(tool.toolType),
				Input:         inputMap,
				Output:        map[string]any{"result": result},
				Status:        string(ToolStatusCompleted),
				ResultStatus:  string(resultStatus),
				StartedAtMs:   tool.startedAtMs,
				CompletedAtMs: time.Now().UnixMilli(),
				CallEventID:   tool.eventID.String(),
				ResultEventID: resultEventID.String(),
			})

			// Stream result
			if state.initialEventID != "" {
				state.sequenceNum++
				oc.emitStreamDelta(ctx, portal, state, StreamContentToolResult, result, map[string]any{
					"call_id":   tool.callID,
					"tool_name": tool.toolName,
					"status":    "completed",
				})
			}

			// Update status back to generating if there might be more content
			oc.emitGenerationStatus(ctx, portal, state, "generating", "Continuing generation...", nil)
		}
	}

	state.completedAtMs = time.Now().UnixMilli()
	oc.emitGenerationStatus(ctx, portal, state, "completed", "Generation complete", nil)

	// Send final edit and save to database
	if state.initialEventID != "" {
		oc.sendFinalAssistantTurn(ctx, portal, state, meta)
		oc.saveAssistantMessage(ctx, log, portal, state, meta)
	}

	log.Info().
		Str("turn_id", state.turnID).
		Str("finish_reason", state.finishReason).
		Int("content_length", state.accumulated.Len()).
		Int("tool_calls", len(state.toolCalls)).
		Msg("Chat Completions streaming finished")

	oc.maybeGenerateTitle(ctx, portal, state.accumulated.String())
	return true, nil
}

// convertToResponsesInput converts Chat Completion messages to Responses API input items
// Supports native multimodal content: images (ResponseInputImageParam), files/PDFs (ResponseInputFileParam)
// Note: Audio is handled via Chat Completions API fallback (SDK v3.16.0 lacks Responses API audio union support)
func (oc *AIClient) convertToResponsesInput(messages []openai.ChatCompletionMessageParamUnion, _ *PortalMetadata) responses.ResponseInputParam {
	var input responses.ResponseInputParam

	for _, msg := range messages {
		if msg.OfUser != nil {
			var contentParts responses.ResponseInputMessageContentListParam
			hasMultimodal := false
			textContent := ""

			if msg.OfUser.Content.OfString.Value != "" {
				textContent = msg.OfUser.Content.OfString.Value
			}

			if len(msg.OfUser.Content.OfArrayOfContentParts) > 0 {
				for _, part := range msg.OfUser.Content.OfArrayOfContentParts {
					if part.OfText != nil && part.OfText.Text != "" {
						if textContent != "" {
							textContent += "\n"
						}
						textContent += part.OfText.Text
					}
					if part.OfImageURL != nil && part.OfImageURL.ImageURL.URL != "" {
						hasMultimodal = true
						detail := responses.ResponseInputImageDetailAuto
						switch part.OfImageURL.ImageURL.Detail {
						case "low":
							detail = responses.ResponseInputImageDetailLow
						case "high":
							detail = responses.ResponseInputImageDetailHigh
						}
						contentParts = append(contentParts, responses.ResponseInputContentUnionParam{
							OfInputImage: &responses.ResponseInputImageParam{
								ImageURL: openai.String(part.OfImageURL.ImageURL.URL),
								Detail:   detail,
							},
						})
					}
					if part.OfFile != nil && part.OfFile.File.FileData.Value != "" {
						hasMultimodal = true
						contentParts = append(contentParts, responses.ResponseInputContentUnionParam{
							OfInputFile: &responses.ResponseInputFileParam{
								FileData: openai.String(part.OfFile.File.FileData.Value),
							},
						})
					}
					// Note: Audio handled by Chat Completions fallback, skip here
				}
			}

			if textContent != "" {
				textPart := responses.ResponseInputContentUnionParam{
					OfInputText: &responses.ResponseInputTextParam{
						Text: textContent,
					},
				}
				contentParts = append([]responses.ResponseInputContentUnionParam{textPart}, contentParts...)
			}

			if hasMultimodal && len(contentParts) > 0 {
				input = append(input, responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRoleUser,
						Content: responses.EasyInputMessageContentUnionParam{
							OfInputItemContentList: contentParts,
						},
					},
				})
			} else if textContent != "" {
				input = append(input, responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Role: responses.EasyInputMessageRoleUser,
						Content: responses.EasyInputMessageContentUnionParam{
							OfString: openai.String(textContent),
						},
					},
				})
			}
			continue
		}

		content, role := extractMessageContent(msg)
		if role == "" || content == "" {
			continue
		}

		var responsesRole responses.EasyInputMessageRole
		switch role {
		case "system":
			responsesRole = responses.EasyInputMessageRoleSystem
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

// hasAudioContent checks if the prompt contains audio content
func hasAudioContent(messages []openai.ChatCompletionMessageParamUnion) bool {
	for _, msg := range messages {
		if msg.OfUser != nil && len(msg.OfUser.Content.OfArrayOfContentParts) > 0 {
			for _, part := range msg.OfUser.Content.OfArrayOfContentParts {
				if part.OfInputAudio != nil {
					return true
				}
			}
		}
	}
	return false
}

// hasMultimodalContent checks if the prompt contains non-text content (image, file, audio).
func hasMultimodalContent(messages []openai.ChatCompletionMessageParamUnion) bool {
	for _, msg := range messages {
		if msg.OfUser != nil && len(msg.OfUser.Content.OfArrayOfContentParts) > 0 {
			for _, part := range msg.OfUser.Content.OfArrayOfContentParts {
				if part.OfImageURL != nil || part.OfFile != nil || part.OfInputAudio != nil {
					return true
				}
			}
		}
	}
	return false
}

// sendInitialStreamMessage sends the first message in a streaming session and returns its event ID
func (oc *AIClient) sendInitialStreamMessage(ctx context.Context, portal *bridgev2.Portal, content string, turnID string) id.EventID {
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return ""
	}

	eventContent := &event.Content{
		Raw: map[string]any{
			"msgtype": event.MsgText,
			"body":    content,
			BeeperAIKey: map[string]any{
				"turn_id": turnID,
				"status":  string(TurnStatusGenerating),
			},
		},
	}
	resp, err := intent.SendMessage(ctx, portal.MXID, event.EventMessage, eventContent, nil)
	if err != nil {
		oc.log.Error().Err(err).Msg("Failed to send initial streaming message")
		return ""
	}
	oc.log.Info().Stringer("event_id", resp.EventID).Str("turn_id", turnID).Msg("Initial streaming message sent")
	return resp.EventID
}

// sendFinalAssistantTurn sends an edit event with the complete assistant turn data
func (oc *AIClient) sendFinalAssistantTurn(ctx context.Context, portal *bridgev2.Portal, state *streamingState, meta *PortalMetadata) {
	if portal == nil || portal.MXID == "" {
		return
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return
	}

	// Render markdown to HTML for rich display
	rawContent := state.accumulated.String()
	rendered := format.RenderMarkdown(rawContent, true, true)

	// Build AI metadata following the new schema
	aiMetadata := map[string]any{
		"turn_id":       state.turnID,
		"model":         oc.effectiveModel(meta),
		"status":        string(TurnStatusCompleted),
		"finish_reason": state.finishReason,
		"timing": map[string]any{
			"started_at":     state.startedAtMs,
			"first_token_at": state.firstTokenAtMs,
			"completed_at":   state.completedAtMs,
		},
	}

	// Add agent_id if set
	if state.agentID != "" {
		aiMetadata["agent_id"] = state.agentID
	}

	// Include embedded thinking if present
	if state.reasoning.Len() > 0 {
		aiMetadata["thinking"] = map[string]any{
			"content":     state.reasoning.String(),
			"token_count": len(strings.Fields(state.reasoning.String())), // Approximate
		}
	}

	// Include tool call event IDs
	if len(state.toolCalls) > 0 {
		toolCallIDs := make([]string, 0, len(state.toolCalls))
		for _, tc := range state.toolCalls {
			if tc.CallEventID != "" {
				toolCallIDs = append(toolCallIDs, tc.CallEventID)
			}
		}
		if len(toolCallIDs) > 0 {
			aiMetadata["tool_calls"] = toolCallIDs
		}
	}

	// Send edit event with m.replace relation and m.new_content
	eventContent := &event.Content{
		Raw: map[string]any{
			"msgtype":        event.MsgText,
			"body":           "* " + rendered.Body, // Fallback with edit marker
			"format":         rendered.Format,
			"formatted_body": "* " + rendered.FormattedBody,
			"m.new_content": map[string]any{
				"msgtype":        event.MsgText,
				"body":           rendered.Body,
				"format":         rendered.Format,
				"formatted_body": rendered.FormattedBody,
			},
			"m.relates_to": map[string]any{
				"rel_type": RelReplace,
				"event_id": state.initialEventID.String(),
			},
			BeeperAIKey:                     aiMetadata,
			"com.beeper.dont_render_edited": true, // Don't show "edited" indicator for streaming updates
		},
	}

	if _, err := intent.SendMessage(ctx, portal.MXID, event.EventMessage, eventContent, nil); err != nil {
		oc.log.Warn().Err(err).Stringer("initial_event_id", state.initialEventID).Msg("Failed to send final assistant turn")
	} else {
		oc.log.Debug().
			Str("initial_event_id", state.initialEventID.String()).
			Str("turn_id", state.turnID).
			Bool("has_thinking", state.reasoning.Len() > 0).
			Int("tool_calls", len(state.toolCalls)).
			Msg("Sent final assistant turn with metadata")
	}
}

// emitStreamDelta sends a streaming delta event to the room
func (oc *AIClient) emitStreamDelta(ctx context.Context, portal *bridgev2.Portal, state *streamingState, contentType StreamContentType, delta string, extra map[string]any) {
	if portal == nil || portal.MXID == "" {
		return
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return
	}

	eventContent := &event.Content{
		Raw: map[string]any{
			"turn_id":      state.turnID,
			"target_event": state.initialEventID.String(),
			"content_type": string(contentType),
			"delta":        delta,
			"seq":          state.sequenceNum,
			"m.relates_to": map[string]any{
				"rel_type": RelReference,
				"event_id": state.initialEventID.String(),
			},
		},
	}

	// Add agent_id if set
	if state.agentID != "" {
		eventContent.Raw["agent_id"] = state.agentID
	}

	// Merge extra fields
	for k, v := range extra {
		if _, exists := eventContent.Raw[k]; !exists {
			eventContent.Raw[k] = v
		}
	}

	if _, err := intent.SendMessage(ctx, portal.MXID, StreamDeltaEventType, eventContent, nil); err != nil {
		oc.log.Warn().Err(err).
			Stringer("target_event", state.initialEventID).
			Str("content_type", string(contentType)).
			Int("seq", state.sequenceNum).
			Msg("Failed to emit stream delta")
	}
}

// emitGenerationStatus sends a generation status update event
func (oc *AIClient) emitGenerationStatus(ctx context.Context, portal *bridgev2.Portal, state *streamingState, statusType string, message string, details *GenerationDetails) {
	if portal == nil || portal.MXID == "" {
		return
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return
	}

	content := map[string]any{
		"turn_id":        state.turnID,
		"status":         statusType,
		"status_message": message,
	}

	if state.initialEventID != "" {
		content["target_event"] = state.initialEventID.String()
	}

	if state.agentID != "" {
		content["agent_id"] = state.agentID
	}

	if details != nil {
		detailsMap := map[string]any{}
		if details.CurrentTool != "" {
			detailsMap["current_tool"] = details.CurrentTool
		}
		if details.CallID != "" {
			detailsMap["call_id"] = details.CallID
		}
		if details.ToolsCompleted > 0 || details.ToolsTotal > 0 {
			detailsMap["tools_completed"] = details.ToolsCompleted
			detailsMap["tools_total"] = details.ToolsTotal
		}
		if len(detailsMap) > 0 {
			content["details"] = detailsMap
		}
	}

	eventContent := &event.Content{Raw: content}

	if _, err := intent.SendMessage(ctx, portal.MXID, GenerationStatusEventType, eventContent, nil); err != nil {
		oc.log.Debug().Err(err).Str("status", statusType).Msg("Failed to emit generation status")
	}
}

// emitToolProgress sends a tool progress update event
func (oc *AIClient) emitToolProgress(ctx context.Context, portal *bridgev2.Portal, state *streamingState, tool *activeToolCall, status ToolStatus, message string, percent int) {
	if portal == nil || portal.MXID == "" {
		return
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return
	}

	content := map[string]any{
		"call_id":   tool.callID,
		"turn_id":   state.turnID,
		"tool_name": tool.toolName,
		"status":    string(status),
		"progress": map[string]any{
			"message": message,
			"percent": percent,
		},
	}

	if state.agentID != "" {
		content["agent_id"] = state.agentID
	}

	eventContent := &event.Content{Raw: content}

	if _, err := intent.SendMessage(ctx, portal.MXID, ToolProgressEventType, eventContent, nil); err != nil {
		oc.log.Debug().Err(err).Str("tool", tool.toolName).Msg("Failed to emit tool progress")
	}
}

// sendToolCallEvent sends a tool call as a timeline event
func (oc *AIClient) sendToolCallEvent(ctx context.Context, portal *bridgev2.Portal, state *streamingState, tool *activeToolCall) id.EventID {
	if portal == nil || portal.MXID == "" {
		return ""
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return ""
	}

	// Build display info
	displayTitle := tool.toolName
	switch tool.toolName {
	case "web_search":
		displayTitle = "Web Search"
	case "code_interpreter":
		displayTitle = "Code Interpreter"
	case "image_generation":
		displayTitle = "Image Generation"
	}

	toolCallData := map[string]any{
		"call_id":   tool.callID,
		"turn_id":   state.turnID,
		"tool_name": tool.toolName,
		"tool_type": string(tool.toolType),
		"status":    string(ToolStatusRunning),
		"display": map[string]any{
			"title":     displayTitle,
			"collapsed": false,
		},
		"timing": map[string]any{
			"started_at": tool.startedAtMs,
		},
	}

	if state.agentID != "" {
		toolCallData["agent_id"] = state.agentID
	}

	eventContent := &event.Content{
		Raw: map[string]any{
			"body":              fmt.Sprintf("Calling %s...", displayTitle),
			"msgtype":           event.MsgNotice,
			BeeperAIToolCallKey: toolCallData,
			"m.relates_to": map[string]any{
				"rel_type": RelReference,
				"event_id": state.initialEventID.String(),
			},
		},
	}

	resp, err := intent.SendMessage(ctx, portal.MXID, ToolCallEventType, eventContent, nil)
	if err != nil {
		oc.log.Warn().Err(err).Str("tool", tool.toolName).Msg("Failed to send tool call event")
		return ""
	}

	oc.log.Debug().
		Stringer("event_id", resp.EventID).
		Str("call_id", tool.callID).
		Str("tool", tool.toolName).
		Msg("Sent tool call timeline event")

	return resp.EventID
}

// sendToolResultEvent sends a tool result as a timeline event
func (oc *AIClient) sendToolResultEvent(ctx context.Context, portal *bridgev2.Portal, state *streamingState, tool *activeToolCall, result string, resultStatus ResultStatus) id.EventID {
	if portal == nil || portal.MXID == "" {
		return ""
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return ""
	}

	// Truncate result for body if too long
	bodyText := result
	if len(bodyText) > 200 {
		bodyText = bodyText[:200] + "..."
	}
	if bodyText == "" {
		bodyText = fmt.Sprintf("%s completed", tool.toolName)
	}

	toolResultData := map[string]any{
		"call_id":   tool.callID,
		"turn_id":   state.turnID,
		"tool_name": tool.toolName,
		"status":    string(resultStatus),
		"display": map[string]any{
			"expandable":       len(result) > 200,
			"default_expanded": len(result) <= 500,
		},
	}

	if state.agentID != "" {
		toolResultData["agent_id"] = state.agentID
	}

	if result != "" {
		toolResultData["output"] = map[string]any{
			"result": result,
		}
	}

	eventContent := &event.Content{
		Raw: map[string]any{
			"body":                bodyText,
			"msgtype":             event.MsgNotice,
			BeeperAIToolResultKey: toolResultData,
			"m.relates_to": map[string]any{
				"rel_type": RelReference,
				"event_id": tool.eventID.String(),
			},
		},
	}

	resp, err := intent.SendMessage(ctx, portal.MXID, ToolResultEventType, eventContent, nil)
	if err != nil {
		oc.log.Warn().Err(err).Str("tool", tool.toolName).Msg("Failed to send tool result event")
		return ""
	}

	oc.log.Debug().
		Stringer("event_id", resp.EventID).
		Str("call_id", tool.callID).
		Str("tool", tool.toolName).
		Str("status", string(resultStatus)).
		Msg("Sent tool result timeline event")

	return resp.EventID
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

// decodeBase64Image decodes a base64-encoded image and detects its MIME type.
// Handles both raw base64 and data URL format (data:image/png;base64,...).
func decodeBase64Image(b64Data string) ([]byte, string, error) {
	var mimeType string

	// Handle data URL format: data:{mimeType};base64,{data}
	if after, found := strings.CutPrefix(b64Data, "data:"); found {
		prefix, data, hasComma := strings.Cut(after, ",")
		if !hasComma {
			return nil, "", fmt.Errorf("invalid data URL: no comma found")
		}
		// Extract MIME type from "{mimeType};base64" prefix
		if mime, _, hasBase64 := strings.Cut(prefix, ";base64"); hasBase64 {
			mimeType = mime
		}
		b64Data = data
	}

	data, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		// Try URL-safe base64 as fallback
		data, err = base64.URLEncoding.DecodeString(b64Data)
		if err != nil {
			return nil, "", fmt.Errorf("base64 decode failed: %w", err)
		}
	}

	// If MIME type wasn't extracted from data URL, detect from bytes
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
		// Fallback to PNG if detection fails (common for AI-generated images)
		if mimeType == "application/octet-stream" {
			mimeType = "image/png"
		}
	}

	return data, mimeType, nil
}

// sendGeneratedImage uploads an AI-generated image to Matrix and sends it as a message
func (oc *AIClient) sendGeneratedImage(
	ctx context.Context,
	portal *bridgev2.Portal,
	imageData []byte,
	mimeType string,
	turnID string,
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

	// Build image message content with AI metadata
	rawContent := map[string]any{
		"msgtype": event.MsgImage,
		"body":    fileName,
		"info": map[string]any{
			"mimetype": mimeType,
			"size":     len(imageData),
		},
	}

	if file != nil {
		rawContent["file"] = file
	} else {
		rawContent["url"] = string(uri)
	}

	// Add image generation metadata
	if turnID != "" {
		rawContent["com.beeper.ai.image_generation"] = map[string]any{
			"turn_id": turnID,
		}
	}

	// Send message
	eventContent := &event.Content{Raw: rawContent}
	resp, err := intent.SendMessage(ctx, portal.MXID, event.EventMessage, eventContent, nil)
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

	if meta.Provider != ProviderOpenRouter && meta.Provider != ProviderBeeper {
		return ""
	}

	// Use configured title generation model if set
	if meta.TitleGenerationModel != "" {
		return meta.TitleGenerationModel
	}

	// Provider-specific defaults for title generation
	switch meta.Provider {
	case ProviderOpenRouter, ProviderBeeper:
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
