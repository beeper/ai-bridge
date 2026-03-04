package providers

import (
	"strings"
	"testing"

	"github.com/beeper/ai-bridge/pkg/ai"
)

func TestNormalizeResponsesToolCallID(t *testing.T) {
	got := NormalizeResponsesToolCallID("call_abc|item+/==")
	if !strings.Contains(got, "|") {
		t.Fatalf("expected normalized id to keep pipe separator, got %q", got)
	}
	parts := strings.SplitN(got, "|", 2)
	if len(parts) != 2 {
		t.Fatalf("expected two parts in normalized id, got %q", got)
	}
	if strings.ContainsAny(parts[0], "+/=") {
		t.Fatalf("expected call id sanitized, got %q", parts[0])
	}
	if !strings.HasPrefix(parts[1], "fc") {
		t.Fatalf("expected item id to start with fc prefix, got %q", parts[1])
	}
}

func TestConvertResponsesMessages_NormalizesAllowedProviderToolIDs(t *testing.T) {
	model := ai.Model{
		ID:       "gpt-5",
		Provider: "openai",
		API:      ai.APIOpenAIResponses,
	}
	context := ai.Context{
		SystemPrompt: "system prompt",
		Messages: []ai.Message{
			{Role: ai.RoleUser, Text: "hi"},
			{
				Role: ai.RoleAssistant,
				Content: []ai.ContentBlock{
					{
						Type:      ai.ContentTypeToolCall,
						ID:        "call_abc|item+/==",
						Name:      "echo",
						Arguments: map[string]any{"message": "hello"},
					},
				},
				Provider:   "github-copilot",
				API:        ai.APIOpenAIResponses,
				Model:      "gpt-5.2-codex",
				StopReason: ai.StopReasonToolUse,
			},
			{
				Role:       ai.RoleToolResult,
				ToolCallID: "call_abc|item+/==",
				ToolName:   "echo",
				Content: []ai.ContentBlock{
					{Type: ai.ContentTypeText, Text: "hello"},
				},
			},
		},
	}

	output := ConvertResponsesMessages(model, context, openAIToolCallProviders, nil)
	if len(output) < 4 {
		t.Fatalf("expected converted response input items, got %d", len(output))
	}
	functionCall := output[2]
	callID, _ := functionCall["call_id"].(string)
	itemID, _ := functionCall["id"].(string)
	if callID == "call_abc" && strings.Contains(itemID, "+") {
		t.Fatalf("expected normalized function call ids, got call=%q item=%q", callID, itemID)
	}
	if !strings.HasPrefix(itemID, "fc") {
		t.Fatalf("expected function call item id to start with fc, got %q", itemID)
	}

	functionOutput := output[3]
	if functionOutput["call_id"] != callID {
		t.Fatalf("expected function_call_output call_id to match normalized call_id, got output=%q call=%q", functionOutput["call_id"], callID)
	}
}

func TestConvertResponsesMessages_CanOmitSystemPrompt(t *testing.T) {
	output := ConvertResponsesMessages(
		ai.Model{Provider: "openai", API: ai.APIOpenAIResponses},
		ai.Context{
			SystemPrompt: "system prompt",
			Messages: []ai.Message{
				{Role: ai.RoleUser, Text: "hello"},
			},
		},
		openAIToolCallProviders,
		&ConvertResponsesMessagesOptions{IncludeSystemPrompt: false},
	)
	if len(output) == 0 {
		t.Fatalf("expected user message output")
	}
	first := output[0]
	if role, _ := first["role"].(string); role == "system" || role == "developer" {
		t.Fatalf("expected no system/developer prompt in output when omitted, got %#v", first)
	}
}

func TestConvertResponsesMessages_OmitsFunctionCallItemIDForDifferentModel(t *testing.T) {
	model := ai.Model{
		ID:       "gpt-5.2-codex",
		Provider: "openai",
		API:      ai.APIOpenAIResponses,
	}
	context := ai.Context{
		Messages: []ai.Message{
			{Role: ai.RoleUser, Text: "use tool"},
			{
				Role: ai.RoleAssistant,
				Content: []ai.ContentBlock{
					{
						Type:      ai.ContentTypeToolCall,
						ID:        "call_123|fc_456",
						Name:      "double_number",
						Arguments: map[string]any{"value": 21},
					},
				},
				Provider:   "openai",
				API:        ai.APIOpenAIResponses,
				Model:      "gpt-5-mini",
				StopReason: ai.StopReasonToolUse,
			},
		},
	}

	output := ConvertResponsesMessages(model, context, openAIToolCallProviders, nil)
	if len(output) < 2 {
		t.Fatalf("expected function call message in output, got %d entries", len(output))
	}
	functionCall := output[len(output)-1]
	if functionCall["type"] != "function_call" {
		t.Fatalf("expected function_call entry, got %#v", functionCall)
	}
	if _, hasID := functionCall["id"]; hasID {
		t.Fatalf("expected function_call id to be omitted for different-model handoff, got %#v", functionCall["id"])
	}
	if callID, _ := functionCall["call_id"].(string); callID != "call_123" {
		t.Fatalf("expected call_id preserved, got %q", callID)
	}
}

func TestConvertResponsesMessages_DropsAbortedReasoningOnlyAssistant(t *testing.T) {
	model := ai.Model{
		ID:       "gpt-5-mini",
		Provider: "openai",
		API:      ai.APIOpenAIResponses,
	}
	context := ai.Context{
		Messages: []ai.Message{
			{Role: ai.RoleUser, Text: "use tool"},
			{
				Role: ai.RoleAssistant,
				Content: []ai.ContentBlock{
					{
						Type:              ai.ContentTypeThinking,
						Thinking:          "",
						ThinkingSignature: `{"type":"reasoning","id":"rs_123","summary":[]}`,
					},
				},
				Provider:   "openai",
				API:        ai.APIOpenAIResponses,
				Model:      "gpt-5-mini",
				StopReason: ai.StopReasonAborted,
			},
			{Role: ai.RoleUser, Text: "say hi"},
		},
	}

	output := ConvertResponsesMessages(model, context, openAIToolCallProviders, nil)
	for _, item := range output {
		itemType, _ := item["type"].(string)
		if itemType == "reasoning" {
			t.Fatalf("expected aborted reasoning history to be omitted, got %#v", item)
		}
	}
}
