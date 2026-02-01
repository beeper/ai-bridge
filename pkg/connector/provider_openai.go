package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// pdfEngineContextKey is the context key for per-request PDF engine override
type pdfEngineContextKey struct{}

// GetPDFEngineFromContext retrieves the PDF engine override from context
func GetPDFEngineFromContext(ctx context.Context) string {
	if engine, ok := ctx.Value(pdfEngineContextKey{}).(string); ok {
		return engine
	}
	return ""
}

// WithPDFEngine adds a PDF engine override to the context
func WithPDFEngine(ctx context.Context, engine string) context.Context {
	return context.WithValue(ctx, pdfEngineContextKey{}, engine)
}

// reasoningEffortMap maps string effort levels to SDK constants
var reasoningEffortMap = map[string]responses.ReasoningEffort{
	"low":    responses.ReasoningEffortLow,
	"medium": responses.ReasoningEffortMedium,
	"high":   responses.ReasoningEffortHigh,
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

func appendHeaderOptions(opts []option.RequestOption, headers map[string]string) []option.RequestOption {
	for key, value := range headers {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		opts = append(opts, option.WithHeader(key, trimmed))
	}
	return opts
}

// NewOpenAIProviderWithPDFPlugin creates an OpenAI provider with PDF plugin middleware.
// Used for OpenRouter/Beeper to enable universal PDF support via file-parser plugin.
func NewOpenAIProviderWithPDFPlugin(apiKey, baseURL, userID, pdfEngine string, headers map[string]string, log zerolog.Logger) (*OpenAIProvider, error) {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}

	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	// Add user_id query parameter if provided
	if userID != "" {
		opts = append(opts, option.WithMiddleware(func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			q := req.URL.Query()
			q.Set("user_id", userID)
			req.URL.RawQuery = q.Encode()
			return next(req)
		}))
	}

	opts = appendHeaderOptions(opts, headers)

	// Add PDF plugin middleware
	opts = append(opts, option.WithMiddleware(MakePDFPluginMiddleware(pdfEngine)))

	client := openai.NewClient(opts...)

	return &OpenAIProvider{
		client:  client,
		log:     log.With().Str("provider", "openai").Str("pdf_engine", pdfEngine).Logger(),
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

// buildResponsesParams constructs Responses API parameters from GenerateParams
func (o *OpenAIProvider) buildResponsesParams(params GenerateParams) responses.ResponseNewParams {
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
	if effort, ok := reasoningEffortMap[params.ReasoningEffort]; ok {
		responsesParams.Reasoning = responses.ReasoningParam{
			Effort: effort,
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

	// Ensure tool names are unique – Anthropic rejects duplicates
	responsesParams.Tools = dedupeToolParams(responsesParams.Tools)

	return responsesParams
}

// GenerateStream generates a streaming response from OpenAI using Responses API
func (o *OpenAIProvider) GenerateStream(ctx context.Context, params GenerateParams) (<-chan StreamEvent, error) {
	events := make(chan StreamEvent, 100)

	go func() {
		defer close(events)

		responsesParams := o.buildResponsesParams(params)

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
	responsesParams := o.buildResponsesParams(params)

	// Make request
	resp, err := o.client.Responses.New(ctx, responsesParams)
	if err != nil {
		return nil, fmt.Errorf("OpenAI generation failed: %w", err)
	}

	// Extract response content
	var content strings.Builder
	var toolCalls []ToolCallResult

	var reasoning strings.Builder
	for _, item := range resp.Output {
		switch item := item.AsAny().(type) {
		case responses.ResponseOutputMessage:
			for _, contentPart := range item.Content {
				switch part := contentPart.AsAny().(type) {
				case responses.ResponseOutputText:
					content.WriteString(part.Text)
				}
			}
		case responses.ResponseReasoningItem:
			// Handle reasoning model output - extract from summary
			for _, summary := range item.Summary {
				if summary.Text != "" {
					reasoning.WriteString(summary.Text)
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

	// If no regular content but we have reasoning, use that as content
	if content.Len() == 0 && reasoning.Len() > 0 {
		content = reasoning
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

			fullModelID := AddModelPrefix(BackendOpenAI, model.ID)
			models = append(models, ModelInfo{
				ID:                  fullModelID,
				Name:                GetModelDisplayName(fullModelID),
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

// defaultOpenAIModels returns known OpenAI models from the manifest
func defaultOpenAIModels() []ModelInfo {
	return GetOpenAIModels()
}

// PDFPluginConfig holds configuration for the PDF file-parser plugin
type PDFPluginConfig struct {
	ID     string          `json:"id"`
	Config json.RawMessage `json:"config,omitempty"`
}

// MakePDFPluginMiddleware creates middleware that injects the file-parser plugin for PDFs.
// The defaultEngine parameter is used as a fallback when no per-request engine is set in context.
// To set a per-request engine, use WithPDFEngine() to add it to the request context.
func MakePDFPluginMiddleware(defaultEngine string) option.Middleware {
	// Validate default engine, default to mistral-ocr
	switch defaultEngine {
	case "pdf-text", "mistral-ocr", "native":
		// valid
	default:
		defaultEngine = "mistral-ocr"
	}

	return func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		// Only modify POST requests with JSON body (API calls)
		if req.Method != http.MethodPost || req.Body == nil {
			return next(req)
		}
		// Only apply PDF plugin to Responses or Chat Completions requests.
		isResponses := strings.Contains(req.URL.Path, "/responses")
		isChatCompletions := strings.Contains(req.URL.Path, "/chat/completions")
		if !isResponses && !isChatCompletions {
			return next(req)
		}

		// Check context for per-request engine override
		engine := GetPDFEngineFromContext(req.Context())
		if engine == "" {
			engine = defaultEngine
		}
		// Validate per-request engine
		switch engine {
		case "pdf-text", "mistral-ocr", "native":
			// valid
		default:
			engine = defaultEngine
		}

		contentType := req.Header.Get("Content-Type")
		if !strings.Contains(contentType, "application/json") {
			return next(req)
		}

		// Read the existing body
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return next(req)
		}
		req.Body.Close()

		// Parse as JSON
		var body map[string]any
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			// Not valid JSON, pass through
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			return next(req)
		}

		hasPDF := func() bool {
			hasPDFFile := func(fileData any) bool {
				data, ok := fileData.(string)
				return ok && strings.Contains(data, "application/pdf")
			}
			hasPDFInParts := func(parts []any) bool {
				for _, part := range parts {
					partMap, ok := part.(map[string]any)
					if !ok {
						continue
					}
					partType, _ := partMap["type"].(string)
					switch partType {
					case "file":
						if fileObj, ok := partMap["file"].(map[string]any); ok {
							if hasPDFFile(fileObj["file_data"]) {
								return true
							}
						}
					case "input_file":
						if fileObj, ok := partMap["input_file"].(map[string]any); ok {
							if hasPDFFile(fileObj["file_data"]) {
								return true
							}
						}
					}
				}
				return false
			}
			// Chat Completions: messages[].content[]
			if messages, ok := body["messages"].([]any); ok {
				for _, msg := range messages {
					msgMap, ok := msg.(map[string]any)
					if !ok {
						continue
					}
					content, ok := msgMap["content"].([]any)
					if ok && hasPDFInParts(content) {
						return true
					}
				}
			}
			// Responses: input[] with type=message content[]
			if inputItems, ok := body["input"].([]any); ok {
				for _, item := range inputItems {
					itemMap, ok := item.(map[string]any)
					if !ok {
						continue
					}
					itemType, _ := itemMap["type"].(string)
					if itemType != "message" {
						continue
					}
					content, ok := itemMap["content"].([]any)
					if ok && hasPDFInParts(content) {
						return true
					}
				}
			}
			return false
		}()

		if !hasPDF {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			return next(req)
		}

		// Add plugins array with file-parser plugin
		plugins := []map[string]any{
			{
				"id": "file-parser",
				"pdf": map[string]any{
					"engine": engine,
				},
			},
		}

		// Merge with existing plugins if any
		if existingPlugins, ok := body["plugins"].([]any); ok {
			for _, p := range existingPlugins {
				if pMap, ok := p.(map[string]any); ok {
					plugins = append(plugins, pMap)
				}
			}
		}
		body["plugins"] = plugins

		// Re-encode
		newBody, err := json.Marshal(body)
		if err != nil {
			// Encoding failed, use original
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			return next(req)
		}

		req.Body = io.NopCloser(bytes.NewReader(newBody))
		req.ContentLength = int64(len(newBody))
		req.Header.Set("Content-Length", fmt.Sprintf("%d", len(newBody)))

		return next(req)
	}
}

// ToOpenAITools converts tool definitions to OpenAI Responses API format
func ToOpenAITools(tools []ToolDefinition) []responses.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	var result []responses.ToolUnionParam
	for _, tool := range tools {
		// Use SDK helper which properly sets all required fields including Type
		toolParam := responses.ToolParamOfFunction(tool.Name, tool.Parameters, true)

		// Add description if available (SDK helper doesn't support this directly)
		if tool.Description != "" && toolParam.OfFunction != nil {
			toolParam.OfFunction.Description = openai.String(tool.Description)
		}

		result = append(result, toolParam)
	}

	return result
}

// dedupeToolParams removes tools with duplicate identifiers to satisfy providers
// like Anthropic that reject duplicated tool names.
func dedupeToolParams(tools []responses.ToolUnionParam) []responses.ToolUnionParam {
	seen := make(map[string]struct{}, len(tools))
	var result []responses.ToolUnionParam
	for _, t := range tools {
		key := ""
		switch {
		case t.OfFunction != nil:
			key = "function:" + t.OfFunction.Name
		case t.OfWebSearch != nil:
			key = "web_search"
		default:
			key = fmt.Sprintf("%v", t) // fallback, should rarely hit
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, t)
	}
	return result
}
