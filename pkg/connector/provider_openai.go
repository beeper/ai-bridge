package connector

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/rs/zerolog"
)

// OpenAIProvider implements AIProvider for OpenAI's API
type OpenAIProvider struct {
	client  openai.Client
	log     zerolog.Logger
	baseURL string
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(apiKey string, log zerolog.Logger) (*OpenAIProvider, error) {
	return NewOpenAIProviderWithBaseURL(apiKey, "", log)
}

// NewOpenAIProviderWithBaseURL creates an OpenAI provider with custom base URL
// Used for OpenRouter, Beeper proxy, or custom endpoints
func NewOpenAIProviderWithBaseURL(apiKey, baseURL string, log zerolog.Logger) (*OpenAIProvider, error) {
	return NewOpenAIProviderWithUserID(apiKey, baseURL, "", log)
}

// NewOpenAIProviderWithUserID creates an OpenAI provider that passes user_id with each request.
// Used for Beeper proxy to ensure correct rate limiting and feature flags per user.
func NewOpenAIProviderWithUserID(apiKey, baseURL, userID string, log zerolog.Logger) (*OpenAIProvider, error) {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}

	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	if userID != "" {
		opts = append(opts, option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			q := req.URL.Query()
			q.Set("user_id", userID)
			req.URL.RawQuery = q.Encode()
			return next(req)
		}))
	}

	client := openai.NewClient(opts...)

	return &OpenAIProvider{
		client:  client,
		log:     log.With().Str("provider", "openai").Logger(),
		baseURL: baseURL,
	}, nil
}

func (o *OpenAIProvider) Name() string {
	return "openai"
}

// Client returns the underlying OpenAI client for direct access
// Used by the bridge for advanced features like Responses API
func (o *OpenAIProvider) Client() openai.Client {
	return o.client
}

// GenerateStream generates a streaming response from OpenAI using Responses API
func (o *OpenAIProvider) GenerateStream(ctx context.Context, params GenerateParams) (<-chan StreamEvent, error) {
	events := make(chan StreamEvent, 100)

	go func() {
		defer close(events)

		// Build Responses API params
		responsesParams := responses.ResponseNewParams{
			Model: params.Model,
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: ToOpenAIResponsesInput(params.Messages),
			},
		}

		// Set max tokens
		if params.MaxCompletionTokens > 0 {
			responsesParams.MaxOutputTokens = openai.Int(int64(params.MaxCompletionTokens))
		}

		// Set system prompt via instructions
		if params.SystemPrompt != "" {
			responsesParams.Instructions = openai.String(params.SystemPrompt)
		}

		// Set tools
		if len(params.Tools) > 0 {
			responsesParams.Tools = ToOpenAITools(params.Tools)
		}

		// Handle reasoning effort for o1/o3 models
		if params.ReasoningEffort != "" && params.ReasoningEffort != "none" {
			switch params.ReasoningEffort {
			case "low":
				responsesParams.Reasoning = responses.ReasoningParam{
					Effort: responses.ReasoningEffortLow,
				}
			case "medium":
				responsesParams.Reasoning = responses.ReasoningParam{
					Effort: responses.ReasoningEffortMedium,
				}
			case "high":
				responsesParams.Reasoning = responses.ReasoningParam{
					Effort: responses.ReasoningEffortHigh,
				}
			}
		}

		// Previous response for conversation continuation
		if params.PreviousResponseID != "" {
			responsesParams.PreviousResponseID = openai.String(params.PreviousResponseID)
		}

		// Web search
		if params.WebSearchEnabled {
			responsesParams.Tools = append(responsesParams.Tools, responses.ToolUnionParam{
				OfWebSearch: &responses.WebSearchToolParam{},
			})
		}

		// Create streaming request
		stream := o.client.Responses.NewStreaming(ctx, responsesParams)
		if stream == nil {
			events <- StreamEvent{
				Type:  StreamEventError,
				Error: fmt.Errorf("failed to create streaming request"),
			}
			return
		}

		var responseID string

		// Process stream events
		for stream.Next() {
			streamEvent := stream.Current()

			switch streamEvent.Type {
			case "response.output_text.delta":
				events <- StreamEvent{
					Type:  StreamEventDelta,
					Delta: streamEvent.Delta,
				}

			case "response.reasoning_text.delta":
				events <- StreamEvent{
					Type:           StreamEventReasoning,
					ReasoningDelta: streamEvent.Delta,
				}

			case "response.function_call_arguments.done":
				events <- StreamEvent{
					Type: StreamEventToolCall,
					ToolCall: &ToolCallResult{
						ID:        streamEvent.ItemID,
						Name:      streamEvent.Name,
						Arguments: streamEvent.Arguments,
					},
				}

			case "response.completed":
				responseID = streamEvent.Response.ID
				finishReason := "stop"
				if streamEvent.Response.Status != "completed" {
					finishReason = string(streamEvent.Response.Status)
				}

				// Extract usage
				var usage *UsageInfo
				if streamEvent.Response.Usage.InputTokens > 0 || streamEvent.Response.Usage.OutputTokens > 0 {
					usage = &UsageInfo{
						PromptTokens:     int(streamEvent.Response.Usage.InputTokens),
						CompletionTokens: int(streamEvent.Response.Usage.OutputTokens),
						TotalTokens:      int(streamEvent.Response.Usage.TotalTokens),
					}
					if streamEvent.Response.Usage.OutputTokensDetails.ReasoningTokens > 0 {
						usage.ReasoningTokens = int(streamEvent.Response.Usage.OutputTokensDetails.ReasoningTokens)
					}
				}

				events <- StreamEvent{
					Type:         StreamEventComplete,
					FinishReason: finishReason,
					ResponseID:   responseID,
					Usage:        usage,
				}

			case "error":
				events <- StreamEvent{
					Type:  StreamEventError,
					Error: fmt.Errorf("API error: %s", streamEvent.Message),
				}
				return
			}
		}

		if err := stream.Err(); err != nil {
			events <- StreamEvent{
				Type:  StreamEventError,
				Error: err,
			}
		}
	}()

	return events, nil
}

// Generate performs a non-streaming generation using Responses API
func (o *OpenAIProvider) Generate(ctx context.Context, params GenerateParams) (*GenerateResponse, error) {
	// Build Responses API params
	responsesParams := responses.ResponseNewParams{
		Model: params.Model,
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: ToOpenAIResponsesInput(params.Messages),
		},
	}

	// Set max tokens
	if params.MaxCompletionTokens > 0 {
		responsesParams.MaxOutputTokens = openai.Int(int64(params.MaxCompletionTokens))
	}

	// Set system prompt via instructions
	if params.SystemPrompt != "" {
		responsesParams.Instructions = openai.String(params.SystemPrompt)
	}

	// Set tools
	if len(params.Tools) > 0 {
		responsesParams.Tools = ToOpenAITools(params.Tools)
	}

	// Handle reasoning effort
	if params.ReasoningEffort != "" && params.ReasoningEffort != "none" {
		switch params.ReasoningEffort {
		case "low":
			responsesParams.Reasoning = responses.ReasoningParam{
				Effort: responses.ReasoningEffortLow,
			}
		case "medium":
			responsesParams.Reasoning = responses.ReasoningParam{
				Effort: responses.ReasoningEffortMedium,
			}
		case "high":
			responsesParams.Reasoning = responses.ReasoningParam{
				Effort: responses.ReasoningEffortHigh,
			}
		}
	}

	// Previous response for conversation continuation
	if params.PreviousResponseID != "" {
		responsesParams.PreviousResponseID = openai.String(params.PreviousResponseID)
	}

	// Web search
	if params.WebSearchEnabled {
		responsesParams.Tools = append(responsesParams.Tools, responses.ToolUnionParam{
			OfWebSearch: &responses.WebSearchToolParam{},
		})
	}

	// Make request
	resp, err := o.client.Responses.New(ctx, responsesParams)
	if err != nil {
		return nil, fmt.Errorf("OpenAI generation failed: %w", err)
	}

	// Extract response content
	var content strings.Builder
	var toolCalls []ToolCallResult

	for _, item := range resp.Output {
		switch item := item.AsAny().(type) {
		case responses.ResponseOutputMessage:
			for _, contentPart := range item.Content {
				switch part := contentPart.AsAny().(type) {
				case responses.ResponseOutputText:
					content.WriteString(part.Text)
				}
			}
		case responses.ResponseFunctionToolCall:
			toolCalls = append(toolCalls, ToolCallResult{
				ID:        item.ID,
				Name:      item.Name,
				Arguments: item.Arguments,
			})
		}
	}

	finishReason := "stop"
	if resp.Status != "completed" {
		finishReason = string(resp.Status)
	}

	return &GenerateResponse{
		Content:      content.String(),
		FinishReason: finishReason,
		ResponseID:   resp.ID,
		ToolCalls:    toolCalls,
		Usage: UsageInfo{
			PromptTokens:     int(resp.Usage.InputTokens),
			CompletionTokens: int(resp.Usage.OutputTokens),
			TotalTokens:      int(resp.Usage.TotalTokens),
			ReasoningTokens:  int(resp.Usage.OutputTokensDetails.ReasoningTokens),
		},
	}, nil
}

// ListModels returns available OpenAI models
func (o *OpenAIProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	// Try to list models from API
	page, err := o.client.Models.List(ctx)
	if err != nil {
		// Fallback to known models
		return defaultOpenAIModels(), nil
	}

	var models []ModelInfo
	for page != nil {
		for _, model := range page.Data {
			// Filter to only relevant models
			if !strings.HasPrefix(model.ID, "gpt-") &&
				!strings.HasPrefix(model.ID, "o1") &&
				!strings.HasPrefix(model.ID, "o3") &&
				!strings.HasPrefix(model.ID, "chatgpt") {
				continue
			}

			models = append(models, ModelInfo{
				ID:                  AddModelPrefix(BackendOpenAI, model.ID),
				Name:                formatModelDisplayName(model.ID),
				Provider:            "openai",
				SupportsVision:      strings.Contains(model.ID, "vision") || strings.Contains(model.ID, "4o") || strings.Contains(model.ID, "4-turbo"),
				SupportsToolCalling: true,
				SupportsReasoning:   strings.HasPrefix(model.ID, "o1") || strings.HasPrefix(model.ID, "o3"),
			})
		}

		// Get next page
		page, err = page.GetNextPage()
		if err != nil {
			break
		}
	}

	if len(models) == 0 {
		return defaultOpenAIModels(), nil
	}

	return models, nil
}

// defaultOpenAIModels returns known OpenAI models
func defaultOpenAIModels() []ModelInfo {
	return GetDefaultModels("openai")
}

// ToOpenAITools converts tool definitions to OpenAI Responses API format
func ToOpenAITools(tools []ToolDefinition) []responses.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	var result []responses.ToolUnionParam
	for _, tool := range tools {
		funcParam := responses.FunctionToolParam{
			Name:        tool.Name,
			Description: openai.String(tool.Description),
		}

		// Parameters is map[string]any in the SDK
		if tool.Parameters != nil {
			funcParam.Parameters = tool.Parameters
		}

		result = append(result, responses.ToolUnionParam{
			OfFunction: &funcParam,
		})
	}

	return result
}
