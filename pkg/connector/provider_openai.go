package connector

import (
	"context"
	"fmt"
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
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}

	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
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

func (o *OpenAIProvider) SupportsTools() bool {
	return true
}

func (o *OpenAIProvider) SupportsVision() bool {
	return true
}

func (o *OpenAIProvider) SupportsStreaming() bool {
	return true
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

		// Set temperature
		if params.Temperature > 0 {
			responsesParams.Temperature = openai.Float(params.Temperature)
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

	// Set temperature
	if params.Temperature > 0 {
		responsesParams.Temperature = openai.Float(params.Temperature)
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
			IsReasoningModel:    strings.HasPrefix(model.ID, "o1") || strings.HasPrefix(model.ID, "o3"),
		})
	}

	if len(models) == 0 {
		return defaultOpenAIModels(), nil
	}

	return models, nil
}

// ValidateModel checks if a model is valid for OpenAI
func (o *OpenAIProvider) ValidateModel(ctx context.Context, modelID string) (bool, error) {
	// Parse prefix
	backend, actualModel := ParseModelPrefix(modelID)
	if backend != BackendOpenAI && backend != "" {
		return false, nil
	}

	// Try to get model info
	_, err := o.client.Models.Get(ctx, actualModel)
	if err != nil {
		// Check if it's a known model
		for _, m := range defaultOpenAIModels() {
			_, knownModel := ParseModelPrefix(m.ID)
			if actualModel == knownModel {
				return true, nil
			}
		}
		return false, nil
	}

	return true, nil
}

// CountTokens estimates token count for messages
func (o *OpenAIProvider) CountTokens(ctx context.Context, messages []UnifiedMessage, model string) (int, error) {
	// Use tiktoken for accurate counting when available
	// Fallback to estimation (~4 chars per token)
	total := 0
	for _, msg := range messages {
		total += len(msg.Text()) / 4
	}
	return total, nil
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

// ChatCompletionsGenerate performs generation using Chat Completions API (fallback)
func (o *OpenAIProvider) ChatCompletionsGenerate(ctx context.Context, params GenerateParams) (*GenerateResponse, error) {
	chatParams := openai.ChatCompletionNewParams{
		Model:    params.Model,
		Messages: ToOpenAIChatMessages(params.Messages),
	}

	if params.MaxCompletionTokens > 0 {
		chatParams.MaxCompletionTokens = openai.Int(int64(params.MaxCompletionTokens))
	}

	if params.Temperature > 0 {
		chatParams.Temperature = openai.Float(params.Temperature)
	}

	if len(params.Tools) > 0 {
		chatParams.Tools = ToOpenAIChatTools(params.Tools)
	}

	resp, err := o.client.Chat.Completions.New(ctx, chatParams)
	if err != nil {
		return nil, fmt.Errorf("OpenAI Chat Completions failed: %w", err)
	}

	var content strings.Builder
	var toolCalls []ToolCallResult

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		content.WriteString(choice.Message.Content)

		for _, tc := range choice.Message.ToolCalls {
			toolCalls = append(toolCalls, ToolCallResult{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
	}

	finishReason := "stop"
	if len(resp.Choices) > 0 {
		finishReason = string(resp.Choices[0].FinishReason)
	}

	return &GenerateResponse{
		Content:      content.String(),
		FinishReason: finishReason,
		ToolCalls:    toolCalls,
		Usage: UsageInfo{
			PromptTokens:     int(resp.Usage.PromptTokens),
			CompletionTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:      int(resp.Usage.TotalTokens),
		},
	}, nil
}

// ToOpenAIChatTools converts tool definitions to Chat Completions format
func ToOpenAIChatTools(tools []ToolDefinition) []openai.ChatCompletionToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	var result []openai.ChatCompletionToolUnionParam
	for _, tool := range tools {
		funcDef := openai.FunctionDefinitionParam{
			Name:        tool.Name,
			Description: openai.String(tool.Description),
		}

		if tool.Parameters != nil {
			funcDef.Parameters = openai.FunctionParameters(tool.Parameters)
		}

		result = append(result, openai.ChatCompletionFunctionTool(funcDef))
	}

	return result
}

