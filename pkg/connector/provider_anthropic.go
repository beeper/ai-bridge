package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/rs/zerolog"
)

// AnthropicProvider implements AIProvider for Anthropic's Claude API
type AnthropicProvider struct {
	client  anthropic.Client
	log     zerolog.Logger
	baseURL string // For Beeper proxy routing
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(apiKey string, log zerolog.Logger) (*AnthropicProvider, error) {
	return NewAnthropicProviderWithBaseURL(apiKey, "", log)
}

// NewAnthropicProviderWithBaseURL creates an Anthropic provider with custom base URL (for Beeper proxy)
func NewAnthropicProviderWithBaseURL(apiKey, baseURL string, log zerolog.Logger) (*AnthropicProvider, error) {
	return NewAnthropicProviderWithUserID(apiKey, baseURL, "", log)
}

// NewAnthropicProviderWithUserID creates an Anthropic provider that passes user_id with each request.
// Used for Beeper proxy to ensure correct rate limiting and feature flags per user.
func NewAnthropicProviderWithUserID(apiKey, baseURL, userID string, log zerolog.Logger) (*AnthropicProvider, error) {
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

	client := anthropic.NewClient(opts...)

	return &AnthropicProvider{
		client:  client,
		log:     log.With().Str("provider", "anthropic").Logger(),
		baseURL: baseURL,
	}, nil
}

func (a *AnthropicProvider) Name() string {
	return "anthropic"
}

func (a *AnthropicProvider) SupportsTools() bool {
	return true
}

func (a *AnthropicProvider) SupportsVision() bool {
	return true
}

func (a *AnthropicProvider) SupportsStreaming() bool {
	return true
}

// GenerateStream generates a streaming response from Anthropic
func (a *AnthropicProvider) GenerateStream(ctx context.Context, params GenerateParams) (<-chan StreamEvent, error) {
	events := make(chan StreamEvent, 100)

	go func() {
		defer close(events)

		// Build message params
		messageParams := anthropic.MessageNewParams{
			Model:     anthropic.Model(params.Model),
			Messages:  ToAnthropicMessages(params.Messages),
			MaxTokens: int64(params.MaxCompletionTokens),
		}

		// Set system prompt
		if params.SystemPrompt != "" {
			messageParams.System = []anthropic.TextBlockParam{
				{Text: params.SystemPrompt},
			}
		}

		// Set temperature
		if params.Temperature > 0 {
			messageParams.Temperature = anthropic.Float(params.Temperature)
		}

		// Set tools
		if len(params.Tools) > 0 {
			messageParams.Tools = ToAnthropicTools(params.Tools)
		}

		// Create streaming request
		stream := a.client.Messages.NewStreaming(ctx, messageParams)

		var currentToolCall *ToolCallResult
		var currentToolInput strings.Builder

		for stream.Next() {
			event := stream.Current()

			switch evt := event.AsAny().(type) {
			case anthropic.ContentBlockDeltaEvent:
				switch delta := evt.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					events <- StreamEvent{
						Type:  StreamEventDelta,
						Delta: delta.Text,
					}
				case anthropic.InputJSONDelta:
					// Accumulate tool input JSON
					currentToolInput.WriteString(delta.PartialJSON)
				}

			case anthropic.ContentBlockStartEvent:
				switch block := evt.ContentBlock.AsAny().(type) {
				case anthropic.ToolUseBlock:
					currentToolCall = &ToolCallResult{
						ID:   block.ID,
						Name: block.Name,
					}
					currentToolInput.Reset()
				}

			case anthropic.ContentBlockStopEvent:
				// Send completed tool call
				if currentToolCall != nil {
					currentToolCall.Arguments = currentToolInput.String()
					if currentToolCall.Arguments == "" {
						currentToolCall.Arguments = "{}"
					}
					events <- StreamEvent{
						Type:     StreamEventToolCall,
						ToolCall: currentToolCall,
					}
					currentToolCall = nil
				}

			case anthropic.MessageDeltaEvent:
				// Extract finish reason and usage
				var usage *UsageInfo
				if evt.Usage.OutputTokens > 0 {
					usage = &UsageInfo{
						CompletionTokens: int(evt.Usage.OutputTokens),
					}
				}
				events <- StreamEvent{
					Type:         StreamEventComplete,
					FinishReason: string(evt.Delta.StopReason),
					Usage:        usage,
				}

			case anthropic.MessageStartEvent:
				// Initial message with input tokens
				if evt.Message.Usage.InputTokens > 0 {
					// Store for later when we have output tokens
				}
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

// Generate performs a non-streaming generation
func (a *AnthropicProvider) Generate(ctx context.Context, params GenerateParams) (*GenerateResponse, error) {
	// Build message params
	messageParams := anthropic.MessageNewParams{
		Model:     anthropic.Model(params.Model),
		Messages:  ToAnthropicMessages(params.Messages),
		MaxTokens: int64(params.MaxCompletionTokens),
	}

	// Set system prompt
	if params.SystemPrompt != "" {
		messageParams.System = []anthropic.TextBlockParam{
			{Text: params.SystemPrompt},
		}
	}

	// Set temperature
	if params.Temperature > 0 {
		messageParams.Temperature = anthropic.Float(params.Temperature)
	}

	// Set tools
	if len(params.Tools) > 0 {
		messageParams.Tools = ToAnthropicTools(params.Tools)
	}

	// Make request
	resp, err := a.client.Messages.New(ctx, messageParams)
	if err != nil {
		return nil, fmt.Errorf("anthropic generation failed: %w", err)
	}

	// Extract response content
	var content strings.Builder
	var toolCalls []ToolCallResult

	for _, block := range resp.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			content.WriteString(b.Text)
		case anthropic.ToolUseBlock:
			argsJSON := "{}"
			if b.Input != nil {
				if argsBytes, err := json.Marshal(b.Input); err == nil {
					argsJSON = string(argsBytes)
				}
			}
			toolCalls = append(toolCalls, ToolCallResult{
				ID:        b.ID,
				Name:      b.Name,
				Arguments: argsJSON,
			})
		}
	}

	return &GenerateResponse{
		Content:      content.String(),
		FinishReason: string(resp.StopReason),
		ToolCalls:    toolCalls,
		Usage: UsageInfo{
			PromptTokens:     int(resp.Usage.InputTokens),
			CompletionTokens: int(resp.Usage.OutputTokens),
			TotalTokens:      int(resp.Usage.InputTokens + resp.Usage.OutputTokens),
		},
	}, nil
}

// ListModels returns available Anthropic models
func (a *AnthropicProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	// Anthropic doesn't have a models listing API, return known models
	return defaultAnthropicModels(), nil
}

// ValidateModel checks if a model is valid for Anthropic
func (a *AnthropicProvider) ValidateModel(ctx context.Context, modelID string) (bool, error) {
	// Parse prefix
	backend, actualModel := ParseModelPrefix(modelID)
	if backend != BackendAnthropic && backend != "" {
		return false, nil
	}

	// Check against known models
	for _, m := range defaultAnthropicModels() {
		_, knownModel := ParseModelPrefix(m.ID)
		if actualModel == knownModel || strings.HasPrefix(actualModel, knownModel) {
			return true, nil
		}
	}

	// Accept any claude model as potentially valid
	if strings.HasPrefix(actualModel, "claude") {
		return true, nil
	}

	return false, nil
}

// CountTokens estimates token count for messages
func (a *AnthropicProvider) CountTokens(ctx context.Context, messages []UnifiedMessage, model string) (int, error) {
	// Anthropic has a token counting API, but for simplicity use estimation
	// ~4 chars per token for Claude models
	total := 0
	for _, msg := range messages {
		total += len(msg.Text()) / 4
	}
	return total, nil
}

// defaultAnthropicModels returns known Anthropic/Claude models
func defaultAnthropicModels() []ModelInfo {
	return GetDefaultModels("anthropic")
}

// ToAnthropicTools converts tool definitions to Anthropic format
func ToAnthropicTools(tools []ToolDefinition) []anthropic.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	var result []anthropic.ToolUnionParam
	for _, tool := range tools {
		toolParam := anthropic.ToolParam{
			Name:        tool.Name,
			Description: anthropic.String(tool.Description),
		}

		// Convert parameters to Anthropic input schema
		if tool.Parameters != nil {
			toolParam.InputSchema = convertToAnthropicSchema(tool.Parameters)
		} else {
			// Provide default empty object schema (Type defaults to "object")
			toolParam.InputSchema = anthropic.ToolInputSchemaParam{}
		}

		result = append(result, anthropic.ToolUnionParam{
			OfTool: &toolParam,
		})
	}

	return result
}

// convertToAnthropicSchema converts JSON Schema to Anthropic input schema
func convertToAnthropicSchema(params map[string]any) anthropic.ToolInputSchemaParam {
	schema := anthropic.ToolInputSchemaParam{
		// Type defaults to "object" automatically
	}

	if props, ok := params["properties"].(map[string]any); ok {
		schema.Properties = props
	}

	if required, ok := params["required"].([]string); ok {
		schema.Required = required
	} else if required, ok := params["required"].([]any); ok {
		// Convert []any to []string
		var reqStr []string
		for _, r := range required {
			if rs, ok := r.(string); ok {
				reqStr = append(reqStr, rs)
			}
		}
		schema.Required = reqStr
	}

	return schema
}
