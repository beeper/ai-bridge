package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	aipkg "github.com/beeper/ai-bridge/pkg/ai"
)

func aiUsageToConnectorUsage(usage aipkg.Usage) *UsageInfo {
	total := usage.TotalTokens
	computed := usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
	if total <= 0 || total < computed {
		total = computed
	}
	if usage.Input == 0 && usage.Output == 0 && usage.CacheRead == 0 && usage.CacheWrite == 0 && total == 0 {
		return nil
	}
	return &UsageInfo{
		PromptTokens:     usage.Input + usage.CacheRead + usage.CacheWrite,
		CompletionTokens: usage.Output,
		TotalTokens:      total,
	}
}

func aiEventToStreamEvent(event aipkg.AssistantMessageEvent) (StreamEvent, bool) {
	switch event.Type {
	case aipkg.EventTextDelta:
		return StreamEvent{
			Type:  StreamEventDelta,
			Delta: event.Delta,
		}, true
	case aipkg.EventThinkingDelta:
		return StreamEvent{
			Type:           StreamEventReasoning,
			ReasoningDelta: event.Delta,
		}, true
	case aipkg.EventToolCallEnd:
		toolCall := event.ToolCall
		if toolCall == nil && event.ContentIndex >= 0 && event.ContentIndex < len(event.Partial.Content) {
			toolCall = &event.Partial.Content[event.ContentIndex]
		}
		if toolCall == nil {
			return StreamEvent{}, false
		}
		args := "{}"
		if toolCall.Arguments != nil {
			if raw, err := json.Marshal(toolCall.Arguments); err == nil {
				args = string(raw)
			}
		}
		return StreamEvent{
			Type: StreamEventToolCall,
			ToolCall: &ToolCallResult{
				ID:        strings.TrimSpace(toolCall.ID),
				Name:      strings.TrimSpace(toolCall.Name),
				Arguments: args,
			},
		}, true
	case aipkg.EventDone:
		reason := strings.TrimSpace(string(event.Reason))
		if reason == "" {
			reason = strings.TrimSpace(string(event.Message.StopReason))
		}
		return StreamEvent{
			Type:         StreamEventComplete,
			FinishReason: reason,
			Usage:        aiUsageToConnectorUsage(event.Message.Usage),
		}, true
	case aipkg.EventError:
		errText := strings.TrimSpace(event.Error.ErrorMessage)
		if errText == "" {
			errText = "pkg/ai stream error"
		}
		return StreamEvent{
			Type:  StreamEventError,
			Error: fmt.Errorf("%s", errText),
		}, true
	default:
		return StreamEvent{}, false
	}
}

func streamEventsFromAIStream(ctx context.Context, stream *aipkg.AssistantMessageEventStream) <-chan StreamEvent {
	events := make(chan StreamEvent, 64)
	go func() {
		defer close(events)
		for {
			event, err := stream.Next(ctx)
			if err != nil {
				if err != io.EOF && err != context.Canceled {
					events <- StreamEvent{
						Type:  StreamEventError,
						Error: err,
					}
				}
				return
			}
			if mapped, ok := aiEventToStreamEvent(event); ok {
				events <- mapped
			}
			if event.Type == aipkg.EventDone || event.Type == aipkg.EventError {
				return
			}
		}
	}()
	return events
}
