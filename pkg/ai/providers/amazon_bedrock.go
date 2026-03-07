package providers

import (
	"os"
	"strings"

	"github.com/beeper/ai-bridge/pkg/ai"
	"github.com/beeper/ai-bridge/pkg/ai/utils"
)

type BedrockToolChoice struct {
	Type string
	Name string
}

type BedrockOptions struct {
	StreamOptions       ai.StreamOptions
	Region              string
	Profile             string
	ToolChoice          any
	Reasoning           ai.ThinkingLevel
	ThinkingBudgets     ai.ThinkingBudgets
	InterleavedThinking *bool
}

func BuildBedrockConverseInput(model ai.Model, context ai.Context, options BedrockOptions) map[string]any {
	cacheRetention := ResolveBedrockCacheRetention(options.StreamOptions.CacheRetention)
	input := map[string]any{
		"modelId":         model.ID,
		"messages":        ConvertBedrockMessages(context, model, cacheRetention),
		"system":          BuildBedrockSystemPrompt(context.SystemPrompt, model, cacheRetention),
		"inferenceConfig": map[string]any{"maxTokens": options.StreamOptions.MaxTokens, "temperature": options.StreamOptions.Temperature},
	}
	if tc := ConvertBedrockToolConfig(context.Tools, options.ToolChoice); tc != nil {
		input["toolConfig"] = tc
	}
	if extra := BuildBedrockAdditionalModelRequestFields(model, options); extra != nil {
		input["additionalModelRequestFields"] = extra
	}
	return input
}

func SupportsAdaptiveThinking(modelID string) bool {
	id := strings.ToLower(modelID)
	return strings.Contains(id, "opus-4-6") ||
		strings.Contains(id, "opus-4.6") ||
		strings.Contains(id, "sonnet-4-6") ||
		strings.Contains(id, "sonnet-4.6")
}

func ResolveBedrockCacheRetention(cacheRetention ai.CacheRetention) ai.CacheRetention {
	if cacheRetention != "" {
		return cacheRetention
	}
	if strings.EqualFold(os.Getenv("PI_CACHE_RETENTION"), "long") {
		return ai.CacheRetentionLong
	}
	return ai.CacheRetentionShort
}

func BuildBedrockSystemPrompt(systemPrompt string, model ai.Model, cacheRetention ai.CacheRetention) []map[string]any {
	if strings.TrimSpace(systemPrompt) == "" {
		return nil
	}
	blocks := []map[string]any{
		{"text": utils.SanitizeSurrogates(systemPrompt)},
	}
	if cacheRetention != ai.CacheRetentionNone && supportsBedrockPromptCaching(model) {
		cachePoint := map[string]any{"type": "default"}
		if cacheRetention == ai.CacheRetentionLong {
			cachePoint["ttl"] = "1h"
		}
		blocks = append(blocks, map[string]any{
			"cachePoint": cachePoint,
		})
	}
	return blocks
}

func supportsBedrockPromptCaching(model ai.Model) bool {
	if model.Cost.CacheRead > 0 || model.Cost.CacheWrite > 0 {
		return true
	}
	id := strings.ToLower(model.ID)
	if strings.Contains(id, "claude") && (strings.Contains(id, "-4-") || strings.Contains(id, "-4.")) {
		return true
	}
	if strings.Contains(id, "claude-3-7-sonnet") {
		return true
	}
	if strings.Contains(id, "claude-3-5-haiku") {
		return true
	}
	return false
}

func NormalizeBedrockToolCallID(id string) string {
	sanitized := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, id)
	if len(sanitized) > 64 {
		return sanitized[:64]
	}
	return sanitized
}

func ConvertBedrockMessages(context ai.Context, model ai.Model, cacheRetention ai.CacheRetention) []map[string]any {
	transformed := TransformMessages(context.Messages, model, func(id string, _ ai.Model, _ ai.Message) string {
		return NormalizeBedrockToolCallID(id)
	})
	result := make([]map[string]any, 0, len(transformed))
	supportsThinkingSignature := supportsBedrockThinkingSignature(model)

	for i := 0; i < len(transformed); i++ {
		msg := transformed[i]
		switch msg.Role {
		case ai.RoleUser:
			content := make([]map[string]any, 0, max(1, len(msg.Content)))
			if strings.TrimSpace(msg.Text) != "" {
				content = append(content, map[string]any{"text": utils.SanitizeSurrogates(msg.Text)})
			}
			for _, c := range msg.Content {
				switch c.Type {
				case ai.ContentTypeText:
					if strings.TrimSpace(c.Text) == "" {
						continue
					}
					content = append(content, map[string]any{"text": utils.SanitizeSurrogates(c.Text)})
				case ai.ContentTypeImage:
					content = append(content, map[string]any{
						"image": map[string]any{
							"format": imageFormatFromMIME(c.MimeType),
							"source": map[string]any{"bytes": c.Data},
						},
					})
				}
			}
			if len(content) == 0 {
				continue
			}
			result = append(result, map[string]any{
				"role":    "user",
				"content": content,
			})
		case ai.RoleAssistant:
			content := make([]map[string]any, 0, len(msg.Content))
			for _, c := range msg.Content {
				switch c.Type {
				case ai.ContentTypeText:
					if strings.TrimSpace(c.Text) == "" {
						continue
					}
					content = append(content, map[string]any{"text": utils.SanitizeSurrogates(c.Text)})
				case ai.ContentTypeToolCall:
					content = append(content, map[string]any{
						"toolUse": map[string]any{
							"toolUseId": c.ID,
							"name":      c.Name,
							"input":     c.Arguments,
						},
					})
				case ai.ContentTypeThinking:
					if strings.TrimSpace(c.Thinking) == "" {
						continue
					}
					reasoningText := map[string]any{"text": utils.SanitizeSurrogates(c.Thinking)}
					if supportsThinkingSignature && strings.TrimSpace(c.ThinkingSignature) != "" {
						reasoningText["signature"] = c.ThinkingSignature
					}
					content = append(content, map[string]any{
						"reasoningContent": map[string]any{
							"reasoningText": reasoningText,
						},
					})
				}
			}
			if len(content) == 0 {
				continue
			}
			result = append(result, map[string]any{
				"role":    "assistant",
				"content": content,
			})
		case ai.RoleToolResult:
			toolResults := make([]map[string]any, 0, 2)
			toolResults = append(toolResults, map[string]any{
				"toolResult": map[string]any{
					"toolUseId": msg.ToolCallID,
					"content":   bedrockToolResultContent(msg.Content),
					"status":    bedrockToolResultStatus(msg.IsError),
				},
			})

			j := i + 1
			for ; j < len(transformed) && transformed[j].Role == ai.RoleToolResult; j++ {
				next := transformed[j]
				toolResults = append(toolResults, map[string]any{
					"toolResult": map[string]any{
						"toolUseId": next.ToolCallID,
						"content":   bedrockToolResultContent(next.Content),
						"status":    bedrockToolResultStatus(next.IsError),
					},
				})
			}
			i = j - 1
			result = append(result, map[string]any{
				"role":    "user",
				"content": toolResults,
			})
		}
	}

	if cacheRetention != ai.CacheRetentionNone && supportsBedrockPromptCaching(model) && len(result) > 0 {
		last := result[len(result)-1]
		if lastRole, _ := last["role"].(string); lastRole == "user" {
			content, _ := last["content"].([]map[string]any)
			cachePoint := map[string]any{
				"cachePoint": map[string]any{"type": "default"},
			}
			if cacheRetention == ai.CacheRetentionLong {
				cachePoint["cachePoint"].(map[string]any)["ttl"] = "1h"
			}
			last["content"] = append(content, cachePoint)
			result[len(result)-1] = last
		}
	}
	return result
}

func ConvertBedrockToolConfig(tools []ai.Tool, toolChoice any) map[string]any {
	if len(tools) == 0 {
		return nil
	}
	if choice, ok := toolChoice.(string); ok && strings.EqualFold(choice, "none") {
		return nil
	}
	bedrockTools := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		bedrockTools = append(bedrockTools, map[string]any{
			"toolSpec": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"inputSchema": map[string]any{"json": tool.Parameters},
			},
		})
	}

	config := map[string]any{
		"tools": bedrockTools,
	}
	switch choice := toolChoice.(type) {
	case string:
		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "auto":
			config["toolChoice"] = map[string]any{"auto": map[string]any{}}
		case "any":
			config["toolChoice"] = map[string]any{"any": map[string]any{}}
		}
	case BedrockToolChoice:
		if strings.EqualFold(choice.Type, "tool") && strings.TrimSpace(choice.Name) != "" {
			config["toolChoice"] = map[string]any{"tool": map[string]any{"name": choice.Name}}
		}
	case *BedrockToolChoice:
		if choice != nil && strings.EqualFold(choice.Type, "tool") && strings.TrimSpace(choice.Name) != "" {
			config["toolChoice"] = map[string]any{"tool": map[string]any{"name": choice.Name}}
		}
	}
	return config
}

func MapBedrockStopReason(reason string) ai.StopReason {
	switch strings.ToUpper(strings.TrimSpace(reason)) {
	case "END_TURN", "STOP_SEQUENCE":
		return ai.StopReasonStop
	case "MAX_TOKENS", "MODEL_CONTEXT_WINDOW_EXCEEDED":
		return ai.StopReasonLength
	case "TOOL_USE":
		return ai.StopReasonToolUse
	default:
		return ai.StopReasonError
	}
}

func BuildBedrockAdditionalModelRequestFields(model ai.Model, options BedrockOptions) map[string]any {
	if options.Reasoning == "" || !model.Reasoning {
		return nil
	}
	id := strings.ToLower(model.ID)
	if !strings.Contains(id, "anthropic.claude") && !strings.Contains(id, "anthropic/claude") {
		return nil
	}

	if SupportsAdaptiveThinking(model.ID) {
		effort := "high"
		if options.Reasoning == ai.ThinkingXHigh && (strings.Contains(id, "opus-4-6") || strings.Contains(id, "opus-4.6")) {
			effort = "max"
		} else {
			effort = mapBedrockThinkingEffort(options.Reasoning)
		}
		return map[string]any{
			"thinking":      map[string]any{"type": "adaptive"},
			"output_config": map[string]any{"effort": effort},
		}
	}

	level := ClampReasoning(options.Reasoning)
	budgets := mergeThinkingBudgets(ai.ThinkingBudgets{
		Minimal: 1024,
		Low:     2048,
		Medium:  8192,
		High:    16384,
	}, options.ThinkingBudgets)
	budget := budgets.High
	switch level {
	case ai.ThinkingMinimal:
		budget = budgets.Minimal
	case ai.ThinkingLow:
		budget = budgets.Low
	case ai.ThinkingMedium:
		budget = budgets.Medium
	}
	result := map[string]any{
		"thinking": map[string]any{
			"type":          "enabled",
			"budget_tokens": budget,
		},
	}
	interleaved := true
	if options.InterleavedThinking != nil {
		interleaved = *options.InterleavedThinking
	}
	if interleaved {
		result["anthropic_beta"] = []string{"interleaved-thinking-2025-05-14"}
	}
	return result
}

func mapBedrockThinkingEffort(level ai.ThinkingLevel) string {
	switch level {
	case ai.ThinkingMinimal, ai.ThinkingLow:
		return "low"
	case ai.ThinkingMedium:
		return "medium"
	case ai.ThinkingHigh, ai.ThinkingXHigh:
		return "high"
	default:
		return "high"
	}
}

func supportsBedrockThinkingSignature(model ai.Model) bool {
	id := strings.ToLower(model.ID)
	return strings.Contains(id, "anthropic.claude") || strings.Contains(id, "anthropic/claude")
}

func imageFormatFromMIME(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return "jpeg"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	default:
		return "png"
	}
}

func bedrockToolResultStatus(isError bool) string {
	if isError {
		return "error"
	}
	return "success"
}

func bedrockToolResultContent(content []ai.ContentBlock) []map[string]any {
	out := make([]map[string]any, 0, len(content))
	for _, c := range content {
		if c.Type == ai.ContentTypeImage {
			out = append(out, map[string]any{
				"image": map[string]any{
					"format": imageFormatFromMIME(c.MimeType),
					"source": map[string]any{"bytes": c.Data},
				},
			})
			continue
		}
		if c.Type == ai.ContentTypeText {
			out = append(out, map[string]any{"text": utils.SanitizeSurrogates(c.Text)})
		}
	}
	if len(out) == 0 {
		return []map[string]any{{"text": ""}}
	}
	return out
}
