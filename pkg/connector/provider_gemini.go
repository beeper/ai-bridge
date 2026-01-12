package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rs/zerolog"
	"google.golang.org/genai"
)

// GeminiProvider implements AIProvider for Google's Gemini API
type GeminiProvider struct {
	client  *genai.Client
	log     zerolog.Logger
	baseURL string // For Beeper proxy routing
}

// NewGeminiProvider creates a new Gemini provider
func NewGeminiProvider(ctx context.Context, apiKey string, log zerolog.Logger) (*GeminiProvider, error) {
	return NewGeminiProviderWithBaseURL(ctx, apiKey, "", log)
}

// NewGeminiProviderWithBaseURL creates a Gemini provider with custom base URL (for Beeper proxy)
func NewGeminiProviderWithBaseURL(ctx context.Context, apiKey, baseURL string, log zerolog.Logger) (*GeminiProvider, error) {
	config := &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	}

	// Use custom base URL if provided (for Beeper proxy)
	if baseURL != "" {
		config.HTTPOptions = genai.HTTPOptions{
			BaseURL: baseURL,
		}
	}

	client, err := genai.NewClient(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	return &GeminiProvider{
		client:  client,
		log:     log.With().Str("provider", "gemini").Logger(),
		baseURL: baseURL,
	}, nil
}

func (g *GeminiProvider) Name() string {
	return "gemini"
}

func (g *GeminiProvider) SupportsTools() bool {
	return true
}

func (g *GeminiProvider) SupportsVision() bool {
	return true
}

func (g *GeminiProvider) SupportsStreaming() bool {
	return true
}

// GenerateStream generates a streaming response from Gemini
func (g *GeminiProvider) GenerateStream(ctx context.Context, params GenerateParams) (<-chan StreamEvent, error) {
	events := make(chan StreamEvent, 100)

	go func() {
		defer close(events)

		// Convert messages to Gemini format
		contents := ToGeminiContents(params.Messages)

		// Build config
		config := &genai.GenerateContentConfig{}

		// Set system instruction
		if params.SystemPrompt != "" {
			config.SystemInstruction = &genai.Content{
				Parts: []*genai.Part{{Text: params.SystemPrompt}},
			}
		}

		// Set temperature
		if params.Temperature > 0 {
			temp := float32(params.Temperature)
			config.Temperature = &temp
		}

		// Set max tokens
		if params.MaxCompletionTokens > 0 {
			config.MaxOutputTokens = int32(params.MaxCompletionTokens)
		}

		// Set tools
		if len(params.Tools) > 0 {
			config.Tools = ToGeminiTools(params.Tools)
		}

		// Make streaming request using iter.Seq2 pattern
		var totalText strings.Builder
		var finishReason string

		for resp, err := range g.client.Models.GenerateContentStream(ctx, params.Model, contents, config) {
			if err != nil {
				events <- StreamEvent{
					Type:  StreamEventError,
					Error: err,
				}
				return
			}

			if resp == nil {
				continue
			}

			// Process candidates
			for _, candidate := range resp.Candidates {
				if candidate.Content != nil {
					for _, part := range candidate.Content.Parts {
						if part.Text != "" {
							totalText.WriteString(part.Text)
							events <- StreamEvent{
								Type:  StreamEventDelta,
								Delta: part.Text,
							}
						}

						// Handle function calls
						if part.FunctionCall != nil {
							argsJSON := "{}"
							if part.FunctionCall.Args != nil {
								if argsBytes, err := json.Marshal(part.FunctionCall.Args); err == nil {
									argsJSON = string(argsBytes)
								}
							}
							events <- StreamEvent{
								Type: StreamEventToolCall,
								ToolCall: &ToolCallResult{
									Name:      part.FunctionCall.Name,
									Arguments: argsJSON,
								},
							}
						}
					}
				}

				// Extract finish reason
				if candidate.FinishReason != "" {
					finishReason = string(candidate.FinishReason)
				}
			}

			// Extract usage info if available
			if resp.UsageMetadata != nil {
				events <- StreamEvent{
					Type: StreamEventComplete,
					Usage: &UsageInfo{
						PromptTokens:     int(resp.UsageMetadata.PromptTokenCount),
						CompletionTokens: int(resp.UsageMetadata.CandidatesTokenCount),
						TotalTokens:      int(resp.UsageMetadata.TotalTokenCount),
					},
					FinishReason: finishReason,
				}
			}
		}

		// Send completion event if not already sent
		if finishReason == "" {
			events <- StreamEvent{
				Type:         StreamEventComplete,
				FinishReason: "stop",
			}
		}
	}()

	return events, nil
}

// Generate performs a non-streaming generation
func (g *GeminiProvider) Generate(ctx context.Context, params GenerateParams) (*GenerateResponse, error) {
	// Convert messages to Gemini format
	contents := ToGeminiContents(params.Messages)

	// Build config
	config := &genai.GenerateContentConfig{}

	// Set system instruction
	if params.SystemPrompt != "" {
		config.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: params.SystemPrompt}},
		}
	}

	// Set temperature
	if params.Temperature > 0 {
		temp := float32(params.Temperature)
		config.Temperature = &temp
	}

	// Set max tokens
	if params.MaxCompletionTokens > 0 {
		config.MaxOutputTokens = int32(params.MaxCompletionTokens)
	}

	// Set tools
	if len(params.Tools) > 0 {
		config.Tools = ToGeminiTools(params.Tools)
	}

	// Make request
	resp, err := g.client.Models.GenerateContent(ctx, params.Model, contents, config)
	if err != nil {
		return nil, fmt.Errorf("Gemini generation failed: %w", err)
	}

	// Extract response content
	var content strings.Builder
	var toolCalls []ToolCallResult
	var finishReason string

	for _, candidate := range resp.Candidates {
		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					content.WriteString(part.Text)
				}
				if part.FunctionCall != nil {
					argsJSON := "{}"
					if part.FunctionCall.Args != nil {
						if argsBytes, err := json.Marshal(part.FunctionCall.Args); err == nil {
							argsJSON = string(argsBytes)
						}
					}
					toolCalls = append(toolCalls, ToolCallResult{
						Name:      part.FunctionCall.Name,
						Arguments: argsJSON,
					})
				}
			}
		}
		if candidate.FinishReason != "" {
			finishReason = string(candidate.FinishReason)
		}
	}

	result := &GenerateResponse{
		Content:      content.String(),
		FinishReason: finishReason,
		ToolCalls:    toolCalls,
	}

	// Add usage info
	if resp.UsageMetadata != nil {
		result.Usage = UsageInfo{
			PromptTokens:     int(resp.UsageMetadata.PromptTokenCount),
			CompletionTokens: int(resp.UsageMetadata.CandidatesTokenCount),
			TotalTokens:      int(resp.UsageMetadata.TotalTokenCount),
		}
	}

	return result, nil
}

// ListModels returns available Gemini models
func (g *GeminiProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	// Gemini API supports listing models
	page, err := g.client.Models.List(ctx, nil)
	if err != nil {
		// If API listing fails, use known models
		return defaultGeminiModels(), nil
	}

	var models []ModelInfo
	for {
		for _, model := range page.Items {
			if model == nil {
				continue
			}

			// Extract model name (remove "models/" prefix)
			modelID := model.Name
			if strings.HasPrefix(modelID, "models/") {
				modelID = strings.TrimPrefix(modelID, "models/")
			}

			// Skip non-generative models
			if !strings.Contains(modelID, "gemini") {
				continue
			}

			models = append(models, ModelInfo{
				ID:                  AddModelPrefix(BackendGemini, modelID),
				Name:                model.DisplayName,
				Provider:            "gemini",
				Description:         model.Description,
				SupportsVision:      containsAny(model.SupportedActions, "generateContent"),
				SupportsToolCalling: true, // Most Gemini models support tools
				IsReasoningModel:    strings.Contains(modelID, "thinking"),
				ContextWindow:       int(model.InputTokenLimit),
				MaxOutputTokens:     int(model.OutputTokenLimit),
			})
		}

		// Check for next page
		if page.NextPageToken == "" {
			break
		}
		page, err = page.Next(ctx)
		if err != nil {
			break
		}
	}

	// If API listing fails or returns empty, use known models
	if len(models) == 0 {
		models = defaultGeminiModels()
	}

	return models, nil
}

// ValidateModel checks if a model is valid for Gemini
func (g *GeminiProvider) ValidateModel(ctx context.Context, modelID string) (bool, error) {
	// Parse prefix
	backend, actualModel := ParseModelPrefix(modelID)
	if backend != BackendGemini && backend != "" {
		return false, nil
	}

	// Try to get model info
	_, err := g.client.Models.Get(ctx, actualModel, nil)
	if err != nil {
		// Check if it's a known model
		for _, m := range defaultGeminiModels() {
			if strings.Contains(m.ID, actualModel) {
				return true, nil
			}
		}
		return false, nil
	}

	return true, nil
}

// CountTokens estimates token count for messages
func (g *GeminiProvider) CountTokens(ctx context.Context, messages []UnifiedMessage, model string) (int, error) {
	contents := ToGeminiContents(messages)

	// Use Gemini's token counting API
	resp, err := g.client.Models.CountTokens(ctx, model, contents, nil)
	if err != nil {
		// Fallback to estimation (~4 chars per token)
		total := 0
		for _, msg := range messages {
			total += len(msg.Text()) / 4
		}
		return total, nil
	}

	return int(resp.TotalTokens), nil
}

// defaultGeminiModels returns known Gemini models
func defaultGeminiModels() []ModelInfo {
	return []ModelInfo{
		{
			ID:                  "gemini/gemini-2.5-flash",
			Name:                "Gemini 2.5 Flash",
			Provider:            "gemini",
			Description:         "Fast and efficient model for most tasks",
			SupportsVision:      true,
			SupportsToolCalling: true,
			IsReasoningModel:    false,
			ContextWindow:       1000000,
			MaxOutputTokens:     8192,
		},
		{
			ID:                  "gemini/gemini-2.5-pro",
			Name:                "Gemini 2.5 Pro",
			Provider:            "gemini",
			Description:         "Advanced model for complex tasks",
			SupportsVision:      true,
			SupportsToolCalling: true,
			IsReasoningModel:    false,
			ContextWindow:       2000000,
			MaxOutputTokens:     8192,
		},
		{
			ID:                  "gemini/gemini-2.0-flash",
			Name:                "Gemini 2.0 Flash",
			Provider:            "gemini",
			Description:         "Previous generation fast model",
			SupportsVision:      true,
			SupportsToolCalling: true,
			IsReasoningModel:    false,
			ContextWindow:       1000000,
			MaxOutputTokens:     8192,
		},
		{
			ID:                  "gemini/gemini-2.0-flash-thinking",
			Name:                "Gemini 2.0 Flash Thinking",
			Provider:            "gemini",
			Description:         "Reasoning model with chain-of-thought",
			SupportsVision:      true,
			SupportsToolCalling: true,
			IsReasoningModel:    true,
			ContextWindow:       1000000,
			MaxOutputTokens:     8192,
		},
	}
}

// containsAny checks if slice contains any of the given values
func containsAny(slice []string, values ...string) bool {
	for _, s := range slice {
		for _, v := range values {
			if s == v {
				return true
			}
		}
	}
	return false
}

// ToGeminiTools converts tool definitions to Gemini format
func ToGeminiTools(tools []ToolDefinition) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}

	var declarations []*genai.FunctionDeclaration
	for _, tool := range tools {
		decl := &genai.FunctionDeclaration{
			Name:        tool.Name,
			Description: tool.Description,
		}

		// Convert parameters to Gemini Schema
		if tool.Parameters != nil {
			decl.Parameters = convertToGeminiSchema(tool.Parameters)
		}

		declarations = append(declarations, decl)
	}

	return []*genai.Tool{
		{FunctionDeclarations: declarations},
	}
}

// convertToGeminiSchema converts JSON Schema to Gemini Schema
func convertToGeminiSchema(params map[string]any) *genai.Schema {
	schema := &genai.Schema{}

	if typeStr, ok := params["type"].(string); ok {
		switch typeStr {
		case "object":
			schema.Type = genai.TypeObject
		case "array":
			schema.Type = genai.TypeArray
		case "string":
			schema.Type = genai.TypeString
		case "number":
			schema.Type = genai.TypeNumber
		case "integer":
			schema.Type = genai.TypeInteger
		case "boolean":
			schema.Type = genai.TypeBoolean
		}
	}

	if props, ok := params["properties"].(map[string]any); ok {
		schema.Properties = make(map[string]*genai.Schema)
		for name, prop := range props {
			if propMap, ok := prop.(map[string]any); ok {
				schema.Properties[name] = convertToGeminiSchema(propMap)
			}
		}
	}

	if required, ok := params["required"].([]string); ok {
		schema.Required = required
	} else if required, ok := params["required"].([]any); ok {
		for _, r := range required {
			if rs, ok := r.(string); ok {
				schema.Required = append(schema.Required, rs)
			}
		}
	}

	if desc, ok := params["description"].(string); ok {
		schema.Description = desc
	}

	return schema
}
