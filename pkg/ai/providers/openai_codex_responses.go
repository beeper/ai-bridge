package providers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/beeper/ai-bridge/pkg/ai"
)

const defaultCodexBaseURL = "https://chatgpt.com/backend-api"

var codexToolCallProviders = map[string]struct{}{
	"openai":       {},
	"openai-codex": {},
	"opencode":     {},
}

type OpenAICodexResponsesOptions struct {
	StreamOptions    ai.StreamOptions
	ReasoningEffort  string
	ReasoningSummary string
	TextVerbosity    string
}

func BuildOpenAICodexResponsesParams(model ai.Model, context ai.Context, options OpenAICodexResponsesOptions) map[string]any {
	messages := ConvertResponsesMessages(model, context, codexToolCallProviders, &ConvertResponsesMessagesOptions{
		IncludeSystemPrompt: false,
	})
	params := map[string]any{
		"model":               model.ID,
		"store":               false,
		"stream":              true,
		"instructions":        context.SystemPrompt,
		"input":               messages,
		"include":             []string{"reasoning.encrypted_content"},
		"tool_choice":         "auto",
		"parallel_tool_calls": true,
		"text": map[string]any{
			"verbosity": coalesceCodexTextVerbosity(options.TextVerbosity),
		},
	}
	if strings.TrimSpace(options.StreamOptions.SessionID) != "" {
		params["prompt_cache_key"] = options.StreamOptions.SessionID
	}
	if options.StreamOptions.Temperature != nil {
		params["temperature"] = *options.StreamOptions.Temperature
	}
	if len(context.Tools) > 0 {
		params["tools"] = ConvertResponsesTools(context.Tools, false)
	}
	if strings.TrimSpace(options.ReasoningEffort) != "" {
		effort := ClampCodexReasoningEffort(model.ID, options.ReasoningEffort)
		summary := strings.TrimSpace(options.ReasoningSummary)
		if summary == "" {
			summary = "auto"
		}
		params["reasoning"] = map[string]any{
			"effort":  effort,
			"summary": summary,
		}
	}
	return params
}

func ClampCodexReasoningEffort(modelID string, effort string) string {
	effort = strings.ToLower(strings.TrimSpace(effort))
	id := modelID
	if strings.Contains(id, "/") {
		parts := strings.Split(id, "/")
		id = parts[len(parts)-1]
	}
	if (strings.HasPrefix(id, "gpt-5.2") || strings.HasPrefix(id, "gpt-5.3")) && effort == "minimal" {
		return "low"
	}
	if id == "gpt-5.1" && effort == "xhigh" {
		return "high"
	}
	if id == "gpt-5.1-codex-mini" {
		if effort == "high" || effort == "xhigh" {
			return "high"
		}
		return "medium"
	}
	return effort
}

func ResolveCodexURL(baseURL string) string {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalized == "" {
		normalized = defaultCodexBaseURL
	}
	if strings.HasSuffix(normalized, "/codex/responses") {
		return normalized
	}
	if strings.HasSuffix(normalized, "/codex") {
		return normalized + "/responses"
	}
	return normalized + "/codex/responses"
}

func ResolveCodexWebSocketURL(baseURL string) string {
	resolved := ResolveCodexURL(baseURL)
	parsed, err := url.Parse(resolved)
	if err != nil {
		return resolved
	}
	if parsed.Scheme == "https" {
		parsed.Scheme = "wss"
	} else if parsed.Scheme == "http" {
		parsed.Scheme = "ws"
	}
	return parsed.String()
}

func coalesceCodexTextVerbosity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "high":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "medium"
	}
}

// ProcessCodexSSEPayload maps Codex SSE payload chunks into unified stream events.
// This is a deterministic helper used by tests while the full transport integration
// is being ported.
func ProcessCodexSSEPayload(payload string, output *ai.Message) ([]ai.AssistantMessageEvent, error) {
	if output == nil {
		return nil, fmt.Errorf("output message is required")
	}
	lines := strings.Split(payload, "\n")
	events := make([]ai.AssistantMessageEvent, 0)
	currentTextIndex := -1

	emit := func(evt ai.AssistantMessageEvent) {
		evt.Partial = *output
		events = append(events, evt)
	}

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, err
		}
		eventType, _ := event["type"].(string)
		switch eventType {
		case "response.output_item.added":
			item, _ := event["item"].(map[string]any)
			itemType, _ := item["type"].(string)
			if itemType != "message" {
				continue
			}
			output.Content = append(output.Content, ai.ContentBlock{
				Type: ai.ContentTypeText,
			})
			currentTextIndex = len(output.Content) - 1
			emit(ai.AssistantMessageEvent{
				Type:         ai.EventTextStart,
				ContentIndex: currentTextIndex,
			})
		case "response.output_text.delta":
			delta, _ := event["delta"].(string)
			if currentTextIndex >= 0 && currentTextIndex < len(output.Content) {
				output.Content[currentTextIndex].Text += delta
				emit(ai.AssistantMessageEvent{
					Type:         ai.EventTextDelta,
					ContentIndex: currentTextIndex,
					Delta:        delta,
				})
			}
		case "response.output_item.done":
			item, _ := event["item"].(map[string]any)
			itemType, _ := item["type"].(string)
			if itemType != "message" {
				continue
			}
			content, _ := item["content"].([]any)
			finalText := ""
			for _, raw := range content {
				part, _ := raw.(map[string]any)
				if part["type"] == "output_text" {
					if text, ok := part["text"].(string); ok {
						finalText += text
					}
				}
			}
			if currentTextIndex >= 0 && currentTextIndex < len(output.Content) {
				output.Content[currentTextIndex].Text = finalText
				emit(ai.AssistantMessageEvent{
					Type:         ai.EventTextEnd,
					ContentIndex: currentTextIndex,
					Content:      finalText,
				})
			}
		case "response.completed":
			response, _ := event["response"].(map[string]any)
			status, _ := response["status"].(string)
			usage, _ := response["usage"].(map[string]any)
			inputTokens := int(numberValue(usage["input_tokens"]))
			outputTokens := int(numberValue(usage["output_tokens"]))
			totalTokens := int(numberValue(usage["total_tokens"]))
			cacheRead := 0
			if details, ok := usage["input_tokens_details"].(map[string]any); ok {
				cacheRead = int(numberValue(details["cached_tokens"]))
			}
			output.Usage = ai.Usage{
				Input:       inputTokens - cacheRead,
				Output:      outputTokens,
				CacheRead:   cacheRead,
				CacheWrite:  0,
				TotalTokens: totalTokens,
			}
			if status == "completed" {
				output.StopReason = ai.StopReasonStop
			} else if status == "incomplete" {
				output.StopReason = ai.StopReasonLength
			} else {
				output.StopReason = ai.StopReasonError
			}
			emit(ai.AssistantMessageEvent{
				Type:    ai.EventDone,
				Reason:  output.StopReason,
				Message: *output,
			})
		case "error":
			msg, _ := event["message"].(string)
			output.StopReason = ai.StopReasonError
			output.ErrorMessage = msg
			emit(ai.AssistantMessageEvent{
				Type:   ai.EventError,
				Reason: ai.StopReasonError,
				Error:  *output,
			})
		}
	}

	return events, nil
}

func numberValue(v any) float64 {
	switch value := v.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return 0
	}
}
