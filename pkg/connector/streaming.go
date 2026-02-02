package connector

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/beeper/ai-bridge/pkg/agents"
	"github.com/beeper/ai-bridge/pkg/agents/tools"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"
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

	// Directive processing
	sourceEventID id.EventID // The triggering user message event ID (for [[reply_to_current]])
}

// newStreamingState creates a new streaming state with initialized fields
func newStreamingState(meta *PortalMetadata, sourceEventID id.EventID) *streamingState {
	return &streamingState{
		turnID:        NewTurnID(),
		agentID:       meta.DefaultAgentID,
		startedAtMs:   time.Now().UnixMilli(),
		firstToken:    true,
		sourceEventID: sourceEventID,
	}
}

// generatedImage tracks a pending image from image generation
type generatedImage struct {
	itemID   string
	imageB64 string
	turnID   string
}

// functionCallOutput tracks a completed function call output for API continuation
type functionCallOutput struct {
	callID    string // The ItemID from the stream event (used as call_id in continuation)
	name      string // Tool name (for stateless continuations)
	arguments string // Raw arguments JSON (for stateless continuations)
	output    string // The result from executing the tool
}

func buildFunctionCallOutputItem(callID, output string, includeID bool) responses.ResponseInputItemUnionParam {
	item := responses.ResponseInputItemFunctionCallOutputParam{
		CallID: callID,
		Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
			OfString: param.NewOpt(output),
		},
	}
	if includeID {
		item.ID = param.NewOpt("fc_output_" + callID)
	}
	return responses.ResponseInputItemUnionParam{OfFunctionCallOutput: &item}
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

// buildResponsesAPIParams creates common Responses API parameters for both streaming and non-streaming paths
func (oc *AIClient) buildResponsesAPIParams(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, messages []openai.ChatCompletionMessageParamUnion) responses.ResponseNewParams {
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

	// OpenRouter's Responses API only supports function-type tools.
	isOpenRouter := oc.isOpenRouterProvider()
	log.Debug().
		Bool("is_openrouter", isOpenRouter).
		Str("detected_provider", loginMetadata(oc.UserLogin).Provider).
		Msg("Provider detection for tool filtering")

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

	// Add boss tools if this is the Builder room
	if oc.isBuilderRoom(portal) {
		bossTools := tools.BossTools()
		params.Tools = append(params.Tools, bossToolsToOpenAI(bossTools)...)
		log.Debug().Int("count", len(bossTools)).Msg("Added boss agent tools")
	}

	return params
}

// bossToolsToOpenAI converts boss tools to OpenAI Responses API format.
func bossToolsToOpenAI(bossTools []*tools.Tool) []responses.ToolUnionParam {
	var result []responses.ToolUnionParam
	for _, t := range bossTools {
		var schema map[string]any
		switch v := t.InputSchema.(type) {
		case nil:
			schema = nil
		case map[string]any:
			schema = v
		default:
			encoded, err := json.Marshal(v)
			if err == nil {
				if err := json.Unmarshal(encoded, &schema); err != nil {
					schema = nil
				}
			}
		}
		// Use SDK helper which properly sets Type field (required by OpenRouter)
		toolParam := responses.ToolParamOfFunction(t.Name, schema, true)
		if t.Description != "" && toolParam.OfFunction != nil {
			toolParam.OfFunction.Description = openai.String(t.Description)
		}
		result = append(result, toolParam)
	}
	return result
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
) (bool, *ContextLengthError, error) {
	log := zerolog.Ctx(ctx).With().
		Str("portal_id", string(portal.ID)).
		Logger()

	// Ensure model ghost is in the room before any operations
	if err := oc.ensureModelInRoom(ctx, portal); err != nil {
		log.Warn().Err(err).Msg("Failed to ensure model is in room")
		// Continue anyway - typing will fail gracefully
	}

	// Create typing controller with TTL and automatic refresh
	typingCtrl := NewTypingController(oc, ctx, portal)
	typingCtrl.Start()
	defer typingCtrl.Stop()

	// Apply proactive context pruning if enabled
	messages = oc.applyProactivePruning(ctx, messages, meta)

	// Build Responses API params using shared helper
	params := oc.buildResponsesAPIParams(ctx, portal, meta, messages)

	// Inject per-room PDF engine into context for OpenRouter/Beeper providers
	if oc.isOpenRouterProvider() {
		ctx = WithPDFEngine(ctx, oc.effectivePDFEngine(meta))
	}

	stream := oc.api.Responses.NewStreaming(ctx, params)
	if stream == nil {
		log.Error().Msg("Failed to create Responses API streaming request")
		return false, nil, fmt.Errorf("responses streaming not available")
	}

	// Initialize streaming state with turn tracking
	// Pass source event ID for [[reply_to_current]] directive support
	var sourceEventID id.EventID
	if evt != nil {
		sourceEventID = evt.ID
	}
	state := newStreamingState(meta, sourceEventID)

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
		state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, "...", state.turnID, state.sourceEventID)
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
				state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, state.accumulated.String(), state.turnID, state.sourceEventID)
				if state.initialEventID == "" {
					log.Error().Msg("Failed to send initial streaming message")
					return false, nil, fmt.Errorf("failed to send initial streaming message")
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
				state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, "...", state.turnID, state.sourceEventID)
				if state.initialEventID == "" {
					log.Error().Msg("Failed to send initial streaming message")
					return false, nil, fmt.Errorf("failed to send initial streaming message")
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
					return false, nil, fmt.Errorf("failed to send initial streaming message for tool call")
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
					return false, nil, fmt.Errorf("failed to send initial streaming message for tool call")
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

			// Update tool name from done event (delta events don't have the name)
			if tool.toolName == "" && streamEvent.Name != "" {
				tool.toolName = streamEvent.Name
			}

			// Store the item ID for continuation (this is the call_id for the Responses API)
			tool.itemID = streamEvent.ItemID

			if !ensureInitialEvent() {
				return false, nil, fmt.Errorf("failed to send initial streaming message for tool call")
			}

			resultStatus := ResultStatusSuccess
			var result string
			if !oc.isToolEnabled(meta, streamEvent.Name) {
				resultStatus = ResultStatusError
				result = fmt.Sprintf("Error: tool %s is disabled", streamEvent.Name)
			} else {
				// Wrap context with bridge info for tools that need it (e.g., set_chat_info, react)
				toolCtx := WithBridgeToolContext(ctx, &BridgeToolContext{
					Client:        oc,
					Portal:        portal,
					Meta:          meta,
					SourceEventID: state.sourceEventID,
				})
				var err error
				result, err = oc.executeBuiltinTool(toolCtx, portal, streamEvent.Name, streamEvent.Arguments)
				if err != nil {
					log.Warn().Err(err).Str("tool", streamEvent.Name).Msg("Tool execution failed")
					result = fmt.Sprintf("Error: %s", err.Error())
					resultStatus = ResultStatusError
				}
			}

			// Check for TTS audio result (AUDIO: prefix)
			displayResult := result
			if strings.HasPrefix(result, TTSResultPrefix) {
				audioB64 := strings.TrimPrefix(result, TTSResultPrefix)
				audioData, err := base64.StdEncoding.DecodeString(audioB64)
				if err != nil {
					log.Warn().Err(err).Msg("Failed to decode TTS audio")
					displayResult = "Error: failed to decode TTS audio"
					resultStatus = ResultStatusError
				} else {
					// Send audio message
					if _, err := oc.sendGeneratedAudio(ctx, portal, audioData, "audio/mpeg", state.turnID); err != nil {
						log.Warn().Err(err).Msg("Failed to send TTS audio")
						displayResult = "Error: failed to send TTS audio"
						resultStatus = ResultStatusError
					} else {
						displayResult = "Audio message sent successfully"
					}
				}
				result = displayResult
			}

			// Check for image generation result (IMAGE: prefix)
			if strings.HasPrefix(result, ImageResultPrefix) {
				imageB64 := strings.TrimPrefix(result, ImageResultPrefix)
				imageData, err := base64.StdEncoding.DecodeString(imageB64)
				if err != nil {
					log.Warn().Err(err).Msg("Failed to decode generated image")
					displayResult = "Error: failed to decode generated image"
					resultStatus = ResultStatusError
				} else {
					// Send image message
					if _, err := oc.sendGeneratedImage(ctx, portal, imageData, "image/png", state.turnID); err != nil {
						log.Warn().Err(err).Msg("Failed to send generated image")
						displayResult = "Error: failed to send generated image"
						resultStatus = ResultStatusError
					} else {
						displayResult = "Image generated and sent successfully"
					}
				}
				result = displayResult
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
				}, nil
			}
			apiErr := fmt.Errorf("API error: %s", streamEvent.Message)
			if state.initialEventID != "" {
				return false, nil, &NonFallbackError{Err: apiErr}
			}
			return false, nil, apiErr
		}
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		log.Error().Err(err).Msg("Responses API streaming error")
		cle := ParseContextLengthError(err)
		if cle != nil {
			return false, cle, nil
		}
		if state.initialEventID != "" {
			return false, nil, &NonFallbackError{Err: err}
		}
		return false, nil, err
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
				state.baseInput = append(state.baseInput, buildFunctionCallOutputItem(output.callID, output.output, true))
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
						return false, nil, fmt.Errorf("failed to send initial streaming message for tool call")
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
						return false, nil, fmt.Errorf("failed to send initial streaming message for tool call")
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

				// Update tool name from done event (delta events don't have the name)
				if tool.toolName == "" && streamEvent.Name != "" {
					tool.toolName = streamEvent.Name
				}

				tool.itemID = streamEvent.ItemID

				if !ensureInitialEvent() {
					return false, nil, fmt.Errorf("failed to send initial streaming message for tool call")
				}

				resultStatus := ResultStatusSuccess
				var result string
				if !oc.isToolEnabled(meta, streamEvent.Name) {
					resultStatus = ResultStatusError
					result = fmt.Sprintf("Error: tool %s is disabled", streamEvent.Name)
				} else {
					toolCtx := WithBridgeToolContext(ctx, &BridgeToolContext{
						Client:        oc,
						Portal:        portal,
						Meta:          meta,
						SourceEventID: state.sourceEventID,
					})
					var err error
					result, err = oc.executeBuiltinTool(toolCtx, portal, streamEvent.Name, streamEvent.Arguments)
					if err != nil {
						log.Warn().Err(err).Str("tool", streamEvent.Name).Msg("Tool execution failed (continuation)")
						result = fmt.Sprintf("Error: %s", err.Error())
						resultStatus = ResultStatusError
					}
				}

				// Check for TTS audio result (AUDIO: prefix)
				displayResult := result
				if strings.HasPrefix(result, TTSResultPrefix) {
					audioB64 := strings.TrimPrefix(result, TTSResultPrefix)
					audioData, err := base64.StdEncoding.DecodeString(audioB64)
					if err != nil {
						log.Warn().Err(err).Msg("Failed to decode TTS audio (continuation)")
						displayResult = "Error: failed to decode TTS audio"
						resultStatus = ResultStatusError
					} else {
						if _, err := oc.sendGeneratedAudio(ctx, portal, audioData, "audio/mpeg", state.turnID); err != nil {
							log.Warn().Err(err).Msg("Failed to send TTS audio (continuation)")
							displayResult = "Error: failed to send TTS audio"
							resultStatus = ResultStatusError
						} else {
							displayResult = "Audio message sent successfully"
						}
					}
					result = displayResult
				}

				// Check for image generation result (IMAGE: prefix)
				if strings.HasPrefix(result, ImageResultPrefix) {
					imageB64 := strings.TrimPrefix(result, ImageResultPrefix)
					imageData, err := base64.StdEncoding.DecodeString(imageB64)
					if err != nil {
						log.Warn().Err(err).Msg("Failed to decode generated image (continuation)")
						displayResult = "Error: failed to decode generated image"
						resultStatus = ResultStatusError
					} else {
						if _, err := oc.sendGeneratedImage(ctx, portal, imageData, "image/png", state.turnID); err != nil {
							log.Warn().Err(err).Msg("Failed to send generated image (continuation)")
							displayResult = "Error: failed to send generated image"
							resultStatus = ResultStatusError
						} else {
							displayResult = "Image generated and sent successfully"
						}
					}
					result = displayResult
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

	return true, nil, nil
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
		input = append(input, buildFunctionCallOutputItem(output.callID, output.output, isOpenRouter))
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

	// OpenRouter's Responses API only supports function-type tools.
	if meta.Capabilities.SupportsToolCalling {
		enabledTools := GetEnabledBuiltinTools(func(name string) bool {
			return oc.isToolEnabled(meta, name)
		})
		if len(enabledTools) > 0 {
			params.Tools = append(params.Tools, ToOpenAITools(enabledTools)...)
		}
	}

	// Add boss tools for Boss agent rooms (needed for multi-turn tool use)
	agentID := meta.AgentID
	if agentID == "" {
		agentID = meta.DefaultAgentID
	}
	if agents.IsBossAgent(agentID) {
		bossTools := tools.BossTools()
		params.Tools = append(params.Tools, bossToolsToOpenAI(bossTools)...)
	}

	// Prevent duplicate tool names (Anthropic rejects duplicates)
	params.Tools = dedupeToolParams(params.Tools)

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
) (bool, *ContextLengthError, error) {
	log := zerolog.Ctx(ctx).With().
		Str("action", "stream_chat_completions").
		Str("portal", string(portal.ID)).
		Logger()

	// Ensure model ghost is in the room before any operations
	if err := oc.ensureModelInRoom(ctx, portal); err != nil {
		log.Warn().Err(err).Msg("Failed to ensure model is in room")
		// Continue anyway - typing will fail gracefully
	}

	// Create typing controller with TTL and automatic refresh
	typingCtrl := NewTypingController(oc, ctx, portal)
	typingCtrl.Start()
	defer typingCtrl.Stop()

	// Apply proactive context pruning if enabled
	messages = oc.applyProactivePruning(ctx, messages, meta)

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
		return false, nil, fmt.Errorf("chat completions streaming not available")
	}

	// Initialize streaming state with source event ID for [[reply_to_current]] support
	var sourceEventID id.EventID
	if evt != nil {
		sourceEventID = evt.ID
	}
	state := newStreamingState(meta, sourceEventID)

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
					state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, state.accumulated.String(), state.turnID, state.sourceEventID)
					if state.initialEventID == "" {
						log.Error().Msg("Failed to send initial streaming message")
						return false, nil, fmt.Errorf("failed to send initial streaming message")
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
						state.initialEventID = oc.sendInitialStreamMessage(ctx, portal, "...", state.turnID, state.sourceEventID)
						if state.initialEventID == "" {
							log.Error().Msg("Failed to send initial streaming message for tool call")
							return false, nil, fmt.Errorf("failed to send initial streaming message for tool call")
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
			return false, cle, nil
		}
		log.Error().Err(err).Msg("Chat Completions stream error")
		if state.initialEventID != "" {
			return false, nil, &NonFallbackError{Err: err}
		}
		return false, nil, err
	}

	// Execute any accumulated tool calls
	for _, tool := range activeTools {
		if tool.input.Len() > 0 && tool.toolName != "" && oc.isToolEnabled(meta, tool.toolName) {
			// Wrap context with bridge info for tools that need it (e.g., set_chat_info, react)
			toolCtx := WithBridgeToolContext(ctx, &BridgeToolContext{
				Client:        oc,
				Portal:        portal,
				Meta:          meta,
				SourceEventID: state.sourceEventID,
			})
			result, err := oc.executeBuiltinTool(toolCtx, portal, tool.toolName, tool.input.String())
			resultStatus := ResultStatusSuccess
			if err != nil {
				log.Warn().Err(err).Str("tool", tool.toolName).Msg("Tool execution failed (Chat Completions)")
				result = fmt.Sprintf("Error: %s", err.Error())
				resultStatus = ResultStatusError
			}

			// Check for TTS audio result (AUDIO: prefix)
			if strings.HasPrefix(result, TTSResultPrefix) {
				audioB64 := strings.TrimPrefix(result, TTSResultPrefix)
				audioData, decodeErr := base64.StdEncoding.DecodeString(audioB64)
				if decodeErr != nil {
					log.Warn().Err(decodeErr).Msg("Failed to decode TTS audio (Chat Completions)")
					result = "Error: failed to decode TTS audio"
					resultStatus = ResultStatusError
				} else {
					if _, sendErr := oc.sendGeneratedAudio(ctx, portal, audioData, "audio/mpeg", state.turnID); sendErr != nil {
						log.Warn().Err(sendErr).Msg("Failed to send TTS audio (Chat Completions)")
						result = "Error: failed to send TTS audio"
						resultStatus = ResultStatusError
					} else {
						result = "Audio message sent successfully"
					}
				}
			}

			// Check for image generation result (IMAGE: prefix)
			if strings.HasPrefix(result, ImageResultPrefix) {
				imageB64 := strings.TrimPrefix(result, ImageResultPrefix)
				imageData, decodeErr := base64.StdEncoding.DecodeString(imageB64)
				if decodeErr != nil {
					log.Warn().Err(decodeErr).Msg("Failed to decode generated image (Chat Completions)")
					result = "Error: failed to decode generated image"
					resultStatus = ResultStatusError
				} else {
					if _, sendErr := oc.sendGeneratedImage(ctx, portal, imageData, "image/png", state.turnID); sendErr != nil {
						log.Warn().Err(sendErr).Msg("Failed to send generated image (Chat Completions)")
						result = "Error: failed to send generated image"
						resultStatus = ResultStatusError
					} else {
						result = "Image generated and sent successfully"
					}
				}
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
	return true, nil, nil
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
func (oc *AIClient) sendInitialStreamMessage(ctx context.Context, portal *bridgev2.Portal, content string, turnID string, replyTo id.EventID) id.EventID {
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return ""
	}

	var relatesTo map[string]any
	if replyTo != "" {
		relatesTo = map[string]any{
			"m.in_reply_to": map[string]any{
				"event_id": replyTo.String(),
			},
		}
	}

	eventContent := &event.Content{
		Raw: map[string]any{
			"msgtype":      event.MsgText,
			"body":         content,
			"m.relates_to": relatesTo,
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

// sendFinalAssistantTurn sends an edit event with the complete assistant turn data.
// It processes response directives (reply tags, silent replies) before sending when in natural mode.
// Matches OpenClaw's directive processing behavior.
func (oc *AIClient) sendFinalAssistantTurn(ctx context.Context, portal *bridgev2.Portal, state *streamingState, meta *PortalMetadata) {
	if portal == nil || portal.MXID == "" {
		return
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return
	}

	rawContent := state.accumulated.String()

	// Check response mode - raw mode skips directive processing
	responseMode := oc.getAgentResponseMode(meta)
	if responseMode == agents.ResponseModeRaw {
		// Raw mode: send content directly without directive processing
		rendered := format.RenderMarkdown(rawContent, true, true)
		oc.sendFinalAssistantTurnContent(ctx, portal, state, meta, intent, rendered, nil)
		return
	}

	// Natural mode: process directives (OpenClaw-style)
	directives := ParseResponseDirectives(rawContent, state.sourceEventID)

	// Handle silent replies - redact the streaming message
	if directives.IsSilent {
		oc.log.Debug().
			Str("turn_id", state.turnID).
			Str("initial_event_id", state.initialEventID.String()).
			Msg("Silent reply detected, redacting streaming message")

		// Redact the initial streaming message
		if state.initialEventID != "" {
			_, err := intent.SendMessage(ctx, portal.MXID, event.EventRedaction, &event.Content{
				Parsed: &event.RedactionEventContent{
					Redacts: state.initialEventID,
				},
			}, nil)
			if err != nil {
				oc.log.Warn().Err(err).Stringer("event_id", state.initialEventID).Msg("Failed to redact silent reply message")
			}
		}
		return
	}

	// Use cleaned content (directives stripped)
	cleanedContent := directives.Text
	rendered := format.RenderMarkdown(cleanedContent, true, true)

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

	// Build m.relates_to with replace relation
	relatesTo := map[string]any{
		"rel_type": RelReplace,
		"event_id": state.initialEventID.String(),
	}

	// Add reply relation if directive specifies one
	if directives.ReplyToEventID != "" {
		relatesTo["m.in_reply_to"] = map[string]any{
			"event_id": directives.ReplyToEventID.String(),
		}
	}

	// Generate link previews for URLs in the response
	linkPreviews := oc.generateOutboundLinkPreviews(ctx, cleanedContent, intent, portal)

	// Send edit event with m.replace relation and m.new_content
	eventRawContent := map[string]any{
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
		"m.relates_to":                  relatesTo,
		BeeperAIKey:                     aiMetadata,
		"com.beeper.dont_render_edited": true, // Don't show "edited" indicator for streaming updates
	}

	// Attach link previews if any were generated
	if len(linkPreviews) > 0 {
		eventRawContent["com.beeper.linkpreviews"] = PreviewsToMapSlice(linkPreviews)
	}

	eventContent := &event.Content{Raw: eventRawContent}

	if _, err := intent.SendMessage(ctx, portal.MXID, event.EventMessage, eventContent, nil); err != nil {
		oc.log.Warn().Err(err).Stringer("initial_event_id", state.initialEventID).Msg("Failed to send final assistant turn")
	} else {
		oc.log.Debug().
			Str("initial_event_id", state.initialEventID.String()).
			Str("turn_id", state.turnID).
			Bool("has_thinking", state.reasoning.Len() > 0).
			Int("tool_calls", len(state.toolCalls)).
			Bool("has_reply", directives.ReplyToEventID != "").
			Int("link_previews", len(linkPreviews)).
			Msg("Sent final assistant turn with metadata")
	}
}

// sendFinalAssistantTurnContent is a helper for raw mode that sends content without directive processing.
func (oc *AIClient) sendFinalAssistantTurnContent(ctx context.Context, portal *bridgev2.Portal, state *streamingState, meta *PortalMetadata, intent bridgev2.MatrixAPI, rendered event.MessageEventContent, replyToEventID *id.EventID) {
	// Build AI metadata
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

	if state.agentID != "" {
		aiMetadata["agent_id"] = state.agentID
	}

	if state.reasoning.Len() > 0 {
		aiMetadata["thinking"] = map[string]any{
			"content":     state.reasoning.String(),
			"token_count": len(strings.Fields(state.reasoning.String())),
		}
	}

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

	relatesTo := map[string]any{
		"rel_type": RelReplace,
		"event_id": state.initialEventID.String(),
	}

	if replyToEventID != nil && *replyToEventID != "" {
		relatesTo["m.in_reply_to"] = map[string]any{
			"event_id": replyToEventID.String(),
		}
	}

	// Generate link previews for URLs in the response
	linkPreviews := oc.generateOutboundLinkPreviews(ctx, rendered.Body, intent, portal)

	rawContent2 := map[string]any{
		"msgtype":                       event.MsgText,
		"body":                          "* " + rendered.Body,
		"format":                        rendered.Format,
		"formatted_body":                "* " + rendered.FormattedBody,
		"m.new_content":                 map[string]any{"msgtype": event.MsgText, "body": rendered.Body, "format": rendered.Format, "formatted_body": rendered.FormattedBody},
		"m.relates_to":                  relatesTo,
		BeeperAIKey:                     aiMetadata,
		"com.beeper.dont_render_edited": true,
	}

	// Attach link previews if any were generated
	if len(linkPreviews) > 0 {
		rawContent2["com.beeper.linkpreviews"] = PreviewsToMapSlice(linkPreviews)
	}

	eventContent := &event.Content{Raw: rawContent2}

	if _, err := intent.SendMessage(ctx, portal.MXID, event.EventMessage, eventContent, nil); err != nil {
		oc.log.Warn().Err(err).Stringer("initial_event_id", state.initialEventID).Msg("Failed to send final assistant turn (raw mode)")
	} else {
		oc.log.Debug().
			Str("initial_event_id", state.initialEventID.String()).
			Str("turn_id", state.turnID).
			Str("mode", "raw").
			Int("link_previews", len(linkPreviews)).
			Msg("Sent final assistant turn (raw mode)")
	}
}

// generateOutboundLinkPreviews extracts URLs from AI response text, generates link previews, and uploads images to Matrix.
func (oc *AIClient) generateOutboundLinkPreviews(ctx context.Context, text string, intent bridgev2.MatrixAPI, portal *bridgev2.Portal) []*event.BeeperLinkPreview {
	config := oc.getLinkPreviewConfig()
	if !config.Enabled {
		return nil
	}

	urls := ExtractURLs(text, config.MaxURLsOutbound)
	if len(urls) == 0 {
		return nil
	}

	previewer := NewLinkPreviewer(config)
	fetchCtx, cancel := context.WithTimeout(ctx, config.FetchTimeout*time.Duration(len(urls)))
	defer cancel()

	previewsWithImages := previewer.FetchPreviews(fetchCtx, urls)

	// Upload images to Matrix and get final previews
	return UploadPreviewImages(ctx, previewsWithImages, intent, portal.MXID)
}

// getAgentResponseMode returns the response mode for the current agent.
// Defaults to ResponseModeNatural if not set.
// IsRawMode on the portal overrides all other settings (for playground rooms).
func (oc *AIClient) getAgentResponseMode(meta *PortalMetadata) agents.ResponseMode {
	// IsRawMode flag takes priority (set by playground command)
	if meta.IsRawMode {
		return agents.ResponseModeRaw
	}

	agentID := meta.AgentID
	if agentID == "" {
		agentID = meta.DefaultAgentID
	}

	if agentID != "" {
		store := NewAgentStoreAdapter(oc)
		if agent, err := store.GetAgentByID(context.Background(), agentID); err == nil && agent != nil {
			if agent.ResponseMode != "" {
				return agent.ResponseMode
			}
		}
	}

	// Default to natural mode (OpenClaw-style)
	return agents.ResponseModeNatural
}

// sendReaction sends a reaction emoji to a Matrix event.
func (oc *AIClient) sendReaction(ctx context.Context, portal *bridgev2.Portal, targetEventID id.EventID, emoji string) {
	if portal == nil || portal.MXID == "" || targetEventID == "" || emoji == "" {
		return
	}
	intent := oc.getModelIntent(ctx, portal)
	if intent == nil {
		return
	}

	eventContent := &event.Content{
		Raw: map[string]any{
			"m.relates_to": map[string]any{
				"rel_type": "m.annotation",
				"event_id": targetEventID.String(),
				"key":      emoji,
			},
		},
	}

	if _, err := intent.SendMessage(ctx, portal.MXID, event.EventReaction, eventContent, nil); err != nil {
		oc.log.Warn().Err(err).
			Stringer("target_event", targetEventID).
			Str("emoji", emoji).
			Msg("Failed to send reaction")
	} else {
		oc.log.Debug().
			Stringer("target_event", targetEventID).
			Str("emoji", emoji).
			Msg("Sent reaction")
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

// applyProactivePruning applies context pruning before sending to the API
func (oc *AIClient) applyProactivePruning(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, meta *PortalMetadata) []openai.ChatCompletionMessageParamUnion {
	config := oc.connector.Config.Pruning
	if config == nil || !config.Enabled {
		return messages
	}

	// Get model context window (default to 128k if unknown)
	contextWindow := oc.getModelContextWindow(meta)
	if contextWindow <= 0 {
		contextWindow = 128000
	}

	log := zerolog.Ctx(ctx)
	beforeCount := len(messages)

	pruned := PruneContext(messages, config, contextWindow)

	if len(pruned) != beforeCount {
		log.Debug().
			Int("before", beforeCount).
			Int("after", len(pruned)).
			Int("context_window", contextWindow).
			Msg("Applied proactive context pruning")
	}

	return pruned
}
