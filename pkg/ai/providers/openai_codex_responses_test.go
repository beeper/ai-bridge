package providers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/beeper/ai-bridge/pkg/ai"
)

func TestProcessCodexSSEPayload_MapsToAssistantEvents(t *testing.T) {
	payload := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`,
		``,
		`data: {"type":"response.content_part.added","part":{"type":"output_text","text":""}}`,
		``,
		`data: {"type":"response.output_text.delta","delta":"Hello"}`,
		``,
		`data: {"type":"response.output_item.done","item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Hello"}]}}`,
		``,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":5,"output_tokens":3,"total_tokens":8,"input_tokens_details":{"cached_tokens":0}}}}`,
		``,
	}, "\n")

	output := ai.Message{
		Role:     ai.RoleAssistant,
		API:      ai.APIOpenAICodexResponse,
		Provider: "openai-codex",
		Model:    "gpt-5.1-codex",
	}
	events, err := ProcessCodexSSEPayload(payload, &output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected events from payload")
	}

	var sawTextDelta, sawDone bool
	for _, evt := range events {
		if evt.Type == ai.EventTextDelta {
			sawTextDelta = true
			if evt.Delta != "Hello" {
				t.Fatalf("expected text delta Hello, got %q", evt.Delta)
			}
		}
		if evt.Type == ai.EventDone {
			sawDone = true
			if evt.Message.StopReason != ai.StopReasonStop {
				t.Fatalf("expected done stop reason stop, got %s", evt.Message.StopReason)
			}
		}
	}
	if !sawTextDelta {
		t.Fatalf("expected text delta event")
	}
	if !sawDone {
		t.Fatalf("expected done event")
	}
	if output.Usage.TotalTokens != 8 || output.Usage.Input != 5 || output.Usage.Output != 3 {
		t.Fatalf("unexpected usage: %+v", output.Usage)
	}
	if len(output.Content) != 1 || output.Content[0].Text != "Hello" {
		t.Fatalf("unexpected output content: %+v", output.Content)
	}
}

func TestClampCodexReasoningEffort(t *testing.T) {
	cases := []struct {
		modelID string
		effort  string
		want    string
	}{
		{modelID: "gpt-5.2", effort: "minimal", want: "low"},
		{modelID: "openai/gpt-5.3-pro", effort: "minimal", want: "low"},
		{modelID: "gpt-5.1", effort: "xhigh", want: "high"},
		{modelID: "gpt-5.1-codex-mini", effort: "low", want: "medium"},
		{modelID: "gpt-5.1-codex-mini", effort: "xhigh", want: "high"},
		{modelID: "gpt-4.1-mini", effort: "high", want: "high"},
	}
	for _, tc := range cases {
		got := ClampCodexReasoningEffort(tc.modelID, tc.effort)
		if got != tc.want {
			t.Fatalf("ClampCodexReasoningEffort(%q,%q)=%q want %q", tc.modelID, tc.effort, got, tc.want)
		}
	}
}

func TestResolveCodexURLAndWebSocketURL(t *testing.T) {
	if got := ResolveCodexURL(""); got != "https://chatgpt.com/backend-api/codex/responses" {
		t.Fatalf("unexpected default codex URL: %q", got)
	}
	if got := ResolveCodexURL("https://chatgpt.com/backend-api/codex"); got != "https://chatgpt.com/backend-api/codex/responses" {
		t.Fatalf("unexpected codex URL for /codex base: %q", got)
	}
	if got := ResolveCodexURL("https://chatgpt.com/backend-api/codex/responses"); got != "https://chatgpt.com/backend-api/codex/responses" {
		t.Fatalf("unexpected codex URL when already resolved: %q", got)
	}
	if got := ResolveCodexWebSocketURL("https://chatgpt.com/backend-api"); !strings.HasPrefix(got, "wss://") {
		t.Fatalf("expected websocket URL to use wss scheme, got %q", got)
	}
}

func TestBuildOpenAICodexResponsesParams(t *testing.T) {
	temp := 0.2
	params := BuildOpenAICodexResponsesParams(ai.Model{
		ID:       "gpt-5.1-codex-mini",
		Provider: "openai-codex",
		API:      ai.APIOpenAICodexResponse,
	}, ai.Context{
		SystemPrompt: "you are helpful",
		Messages: []ai.Message{
			{Role: ai.RoleUser, Text: "say hi"},
		},
		Tools: []ai.Tool{
			{Name: "lookup", Description: "Lookup docs", Parameters: map[string]any{"type": "object"}},
		},
	}, OpenAICodexResponsesOptions{
		StreamOptions: ai.StreamOptions{
			SessionID:   "session-1",
			Temperature: &temp,
		},
		ReasoningEffort:  "xhigh",
		ReasoningSummary: "detailed",
	})

	if params["model"] != "gpt-5.1-codex-mini" {
		t.Fatalf("unexpected model in params: %#v", params["model"])
	}
	if params["prompt_cache_key"] != "session-1" {
		t.Fatalf("expected prompt cache key, got %#v", params["prompt_cache_key"])
	}
	reasoning, ok := params["reasoning"].(map[string]any)
	if !ok {
		t.Fatalf("expected reasoning object in params")
	}
	if reasoning["effort"] != "high" {
		t.Fatalf("expected xhigh clamp to high for codex-mini, got %#v", reasoning["effort"])
	}
	if reasoning["summary"] != "detailed" {
		t.Fatalf("expected reasoning summary detailed, got %#v", reasoning["summary"])
	}
	tools, ok := params["tools"].([]map[string]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected one tool payload, got %#v", params["tools"])
	}
	body, _ := json.Marshal(params)
	if !strings.Contains(string(body), "reasoning.encrypted_content") {
		t.Fatalf("expected include reasoning encrypted content in payload")
	}
}
