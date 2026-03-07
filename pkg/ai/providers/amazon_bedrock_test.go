package providers

import (
	"testing"

	"github.com/beeper/ai-bridge/pkg/ai"
)

func TestBedrockHelperFunctions(t *testing.T) {
	if !SupportsAdaptiveThinking("global.anthropic.claude-opus-4-6-v1") {
		t.Fatalf("expected adaptive thinking support for opus 4.6")
	}
	if SupportsAdaptiveThinking("global.anthropic.claude-sonnet-4-5-v1") {
		t.Fatalf("did not expect adaptive thinking support for sonnet 4.5")
	}

	t.Setenv("PI_CACHE_RETENTION", "long")
	if got := ResolveBedrockCacheRetention(""); got != ai.CacheRetentionLong {
		t.Fatalf("expected env long cache retention, got %s", got)
	}
	if got := ResolveBedrockCacheRetention(ai.CacheRetentionNone); got != ai.CacheRetentionNone {
		t.Fatalf("expected explicit cache retention none to win, got %s", got)
	}

	system := BuildBedrockSystemPrompt(
		"You are helpful",
		ai.Model{ID: "global.anthropic.claude-sonnet-4-5-v1:0"},
		ai.CacheRetentionLong,
	)
	if len(system) != 2 {
		t.Fatalf("expected system + cache point for cacheable model, got %#v", system)
	}
	cachePoint := system[1]["cachePoint"].(map[string]any)
	if cachePoint["ttl"] != "1h" {
		t.Fatalf("expected long cache ttl=1h, got %#v", cachePoint)
	}

	if got := MapBedrockStopReason("TOOL_USE"); got != ai.StopReasonToolUse {
		t.Fatalf("expected TOOL_USE->toolUse, got %s", got)
	}
	if got := MapBedrockStopReason("MAX_TOKENS"); got != ai.StopReasonLength {
		t.Fatalf("expected MAX_TOKENS->length, got %s", got)
	}
	if got := MapBedrockStopReason("OTHER_REASON"); got != ai.StopReasonError {
		t.Fatalf("expected unknown->error, got %s", got)
	}
}

func TestBuildBedrockAdditionalModelRequestFields(t *testing.T) {
	interleaved := true
	fields := BuildBedrockAdditionalModelRequestFields(
		ai.Model{
			ID:        "global.anthropic.claude-sonnet-4-5-v1:0",
			Provider:  "amazon-bedrock",
			API:       ai.APIBedrockConverse,
			Reasoning: true,
		},
		BedrockOptions{
			Reasoning:           ai.ThinkingMedium,
			ThinkingBudgets:     ai.ThinkingBudgets{Medium: 6000},
			InterleavedThinking: &interleaved,
		},
	)
	if fields == nil {
		t.Fatalf("expected additional fields for Claude model reasoning")
	}
	thinking := fields["thinking"].(map[string]any)
	if thinking["type"] != "enabled" || thinking["budget_tokens"] != 6000 {
		t.Fatalf("unexpected non-adaptive thinking payload: %#v", thinking)
	}
	beta := fields["anthropic_beta"].([]string)
	if len(beta) != 1 || beta[0] != "interleaved-thinking-2025-05-14" {
		t.Fatalf("expected interleaved thinking beta flag, got %#v", beta)
	}

	adaptive := BuildBedrockAdditionalModelRequestFields(
		ai.Model{
			ID:        "global.anthropic.claude-opus-4-6-v1",
			Provider:  "amazon-bedrock",
			API:       ai.APIBedrockConverse,
			Reasoning: true,
		},
		BedrockOptions{
			Reasoning: ai.ThinkingXHigh,
		},
	)
	if adaptive["thinking"].(map[string]any)["type"] != "adaptive" {
		t.Fatalf("expected adaptive thinking payload for opus-4-6")
	}
	outputConfig := adaptive["output_config"].(map[string]any)
	if outputConfig["effort"] != "max" {
		t.Fatalf("expected xhigh on opus-4-6 to map to max effort, got %#v", outputConfig["effort"])
	}
}

func TestConvertBedrockMessages_GroupsToolResultsAndAddsCachePoint(t *testing.T) {
	now := int64(1)
	model := ai.Model{
		ID:       "global.anthropic.claude-sonnet-4-5-v1:0",
		Provider: "amazon-bedrock",
		API:      ai.APIBedrockConverse,
	}
	messages := ConvertBedrockMessages(ai.Context{
		Messages: []ai.Message{
			{Role: ai.RoleUser, Text: "run tools", Timestamp: now},
			{
				Role: ai.RoleAssistant,
				Content: []ai.ContentBlock{
					{Type: ai.ContentTypeToolCall, ID: "call:1", Name: "echo", Arguments: map[string]any{"x": 1}},
				},
				StopReason: ai.StopReasonToolUse,
				Timestamp:  now + 1,
			},
			{
				Role:       ai.RoleToolResult,
				ToolCallID: "call:1",
				ToolName:   "echo",
				Content: []ai.ContentBlock{
					{Type: ai.ContentTypeText, Text: "first"},
				},
				Timestamp: now + 2,
			},
			{
				Role:       ai.RoleToolResult,
				ToolCallID: "call:2",
				ToolName:   "echo",
				Content: []ai.ContentBlock{
					{Type: ai.ContentTypeText, Text: "second"},
				},
				Timestamp: now + 3,
			},
		},
	}, model, ai.CacheRetentionLong)

	if len(messages) != 3 {
		t.Fatalf("expected 3 bedrock messages, got %d", len(messages))
	}
	last := messages[2]
	if last["role"] != "user" {
		t.Fatalf("expected grouped tool results as user message, got %#v", last["role"])
	}
	content := last["content"].([]map[string]any)
	if len(content) < 3 {
		t.Fatalf("expected grouped tool results plus cache point, got %#v", content)
	}
	tr0 := content[0]["toolResult"].(map[string]any)
	if tr0["toolUseId"] != "call_1" {
		t.Fatalf("expected normalized toolUseId, got %#v", tr0["toolUseId"])
	}
	cachePoint := content[len(content)-1]["cachePoint"].(map[string]any)
	if cachePoint["ttl"] != "1h" {
		t.Fatalf("expected long cache ttl on last user message, got %#v", cachePoint)
	}
}

func TestConvertBedrockMessages_ThinkingSignatureSupport(t *testing.T) {
	anthropicModel := ai.Model{
		ID:       "global.anthropic.claude-sonnet-4-5-v1:0",
		Provider: "amazon-bedrock",
		API:      ai.APIBedrockConverse,
	}
	anthropicContext := ai.Context{
		Messages: []ai.Message{
			{
				Role:     ai.RoleAssistant,
				Provider: anthropicModel.Provider,
				API:      anthropicModel.API,
				Model:    anthropicModel.ID,
				Content: []ai.ContentBlock{
					{
						Type:              ai.ContentTypeThinking,
						Thinking:          "reasoning",
						ThinkingSignature: "sig123",
					},
				},
			},
		},
	}

	anthropicMsgs := ConvertBedrockMessages(anthropicContext, anthropicModel, ai.CacheRetentionShort)
	anthropicReasoning := anthropicMsgs[0]["content"].([]map[string]any)[0]["reasoningContent"].(map[string]any)
	anthropicText := anthropicReasoning["reasoningText"].(map[string]any)
	if anthropicText["signature"] != "sig123" {
		t.Fatalf("expected anthropic model to include thinking signature, got %#v", anthropicText)
	}

	otherModel := ai.Model{
		ID:       "meta.llama-4-maverick",
		Provider: "amazon-bedrock",
		API:      ai.APIBedrockConverse,
	}
	otherContext := ai.Context{
		Messages: []ai.Message{
			{
				Role:     ai.RoleAssistant,
				Provider: otherModel.Provider,
				API:      otherModel.API,
				Model:    otherModel.ID,
				Content: []ai.ContentBlock{
					{
						Type:              ai.ContentTypeThinking,
						Thinking:          "reasoning",
						ThinkingSignature: "sig123",
					},
				},
			},
		},
	}
	otherMsgs := ConvertBedrockMessages(otherContext, otherModel, ai.CacheRetentionShort)
	otherReasoning := otherMsgs[0]["content"].([]map[string]any)[0]["reasoningContent"].(map[string]any)
	otherText := otherReasoning["reasoningText"].(map[string]any)
	if _, ok := otherText["signature"]; ok {
		t.Fatalf("expected non-anthropic model to omit signature, got %#v", otherText)
	}
}

func TestConvertBedrockToolConfig(t *testing.T) {
	tools := []ai.Tool{{Name: "echo", Description: "Echo", Parameters: map[string]any{"type": "object"}}}
	if cfg := ConvertBedrockToolConfig(tools, "none"); cfg != nil {
		t.Fatalf("expected nil tool config for choice none")
	}

	autoCfg := ConvertBedrockToolConfig(tools, "auto")
	choice := autoCfg["toolChoice"].(map[string]any)
	if _, ok := choice["auto"]; !ok {
		t.Fatalf("expected auto tool choice in config, got %#v", choice)
	}

	toolCfg := ConvertBedrockToolConfig(tools, BedrockToolChoice{Type: "tool", Name: "echo"})
	chosen := toolCfg["toolChoice"].(map[string]any)["tool"].(map[string]any)
	if chosen["name"] != "echo" {
		t.Fatalf("expected explicit tool choice name echo, got %#v", chosen["name"])
	}
}
