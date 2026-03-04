package connector

import (
	"context"
	"testing"

	"github.com/openai/openai-go/v3"
)

func TestPkgAIRuntimeEnabledFromEnv(t *testing.T) {
	t.Setenv("PI_USE_PKG_AI_RUNTIME", "")
	if pkgAIRuntimeEnabled() {
		t.Fatalf("expected runtime flag disabled by default")
	}

	t.Setenv("PI_USE_PKG_AI_RUNTIME", "1")
	if !pkgAIRuntimeEnabled() {
		t.Fatalf("expected runtime flag enabled for value 1")
	}

	t.Setenv("PI_USE_PKG_AI_RUNTIME", "true")
	if !pkgAIRuntimeEnabled() {
		t.Fatalf("expected runtime flag enabled for value true")
	}

	t.Setenv("PI_USE_PKG_AI_RUNTIME", "off")
	if pkgAIRuntimeEnabled() {
		t.Fatalf("expected runtime flag disabled for value off")
	}

	t.Setenv("PI_USE_PKG_AI_RUNTIME_DRY_RUN", "")
	if pkgAIRuntimeDryRunEnabled() {
		t.Fatalf("expected dry-run flag disabled by default")
	}
	t.Setenv("PI_USE_PKG_AI_RUNTIME_DRY_RUN", "yes")
	if !pkgAIRuntimeDryRunEnabled() {
		t.Fatalf("expected dry-run flag enabled for value yes")
	}
	t.Setenv("PI_USE_PKG_AI_RUNTIME_DRY_RUN", "0")
	if pkgAIRuntimeDryRunEnabled() {
		t.Fatalf("expected dry-run flag disabled for value 0")
	}
}

func TestChooseStreamingRuntimePath(t *testing.T) {
	if got := chooseStreamingRuntimePath(true, ModelAPIResponses, true); got != streamingRuntimeChatCompletions {
		t.Fatalf("expected audio to force chat completions, got %s", got)
	}
	if got := chooseStreamingRuntimePath(false, ModelAPIResponses, true); got != streamingRuntimePkgAI {
		t.Fatalf("expected pkg_ai path when preferred and no audio, got %s", got)
	}
	if got := chooseStreamingRuntimePath(false, ModelAPIChatCompletions, false); got != streamingRuntimeChatCompletions {
		t.Fatalf("expected chat model api path, got %s", got)
	}
	if got := chooseStreamingRuntimePath(false, ModelAPIResponses, false); got != streamingRuntimeResponses {
		t.Fatalf("expected responses path fallback, got %s", got)
	}
}

func TestChatPromptToUnifiedMessages_ConvertsRolesAndImages(t *testing.T) {
	prompt := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("system guidance"),
		{
			OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfArrayOfContentParts: []openai.ChatCompletionContentPartUnionParam{
						{
							OfText: &openai.ChatCompletionContentPartTextParam{
								Text: "look at image",
							},
						},
						{
							OfImageURL: &openai.ChatCompletionContentPartImageParam{
								ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
									URL: "https://example.com/image.png",
								},
							},
						},
					},
				},
			},
		},
		openai.AssistantMessage("ack"),
		openai.ToolMessage("tool output", "call_1"),
	}

	unified := chatPromptToUnifiedMessages(prompt)
	if len(unified) != 3 {
		t.Fatalf("expected three non-system unified messages, got %d", len(unified))
	}
	if unified[0].Role != RoleUser {
		t.Fatalf("expected first role user, got %s", unified[0].Role)
	}
	if len(unified[0].Content) < 2 || unified[0].Content[1].Type != ContentTypeImage {
		t.Fatalf("expected user message to include image content part, got %#v", unified[0].Content)
	}
	if unified[1].Role != RoleAssistant || unified[1].Text() != "ack" {
		t.Fatalf("expected assistant text mapping, got %#v", unified[1])
	}
	if unified[2].Role != RoleTool || unified[2].ToolCallID != "call_1" {
		t.Fatalf("expected tool mapping with tool_call_id, got %#v", unified[2])
	}
}

func TestBuildPkgAIContext_UsesSystemPromptAndMappedMessages(t *testing.T) {
	prompt := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("inline system"),
		openai.UserMessage("hello"),
		openai.AssistantMessage("hi"),
	}
	ctx := buildPkgAIContext("effective system prompt", prompt)
	if ctx.SystemPrompt != "effective system prompt" {
		t.Fatalf("expected explicit effective system prompt in ai context, got %q", ctx.SystemPrompt)
	}
	if len(ctx.Messages) != 2 {
		t.Fatalf("expected 2 mapped messages (system stripped), got %d", len(ctx.Messages))
	}
	if ctx.Messages[0].Role != "user" || ctx.Messages[1].Role != "assistant" {
		t.Fatalf("unexpected mapped roles: %#v", ctx.Messages)
	}
}

func TestPromptContainsToolCalls(t *testing.T) {
	if promptContainsToolCalls([]openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("hello"),
	}) {
		t.Fatalf("did not expect tool call detection for plain user prompt")
	}
	if !promptContainsToolCalls([]openai.ChatCompletionMessageParamUnion{
		{
			OfAssistant: &openai.ChatCompletionAssistantMessageParam{
				ToolCalls: []openai.ChatCompletionMessageToolCallUnionParam{
					{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: "call_1",
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      "search",
								Arguments: "{}",
							},
						},
					},
				},
			},
		},
	}) {
		t.Fatalf("expected assistant tool calls to be detected")
	}
	if !promptContainsToolCalls([]openai.ChatCompletionMessageParamUnion{
		openai.ToolMessage("tool result", "call_1"),
	}) {
		t.Fatalf("expected tool role messages to be detected")
	}
}

func TestShouldUsePkgAIBridgeStreaming(t *testing.T) {
	client := &AIClient{}
	if !client.shouldUsePkgAIBridgeStreaming(context.Background(), &PortalMetadata{}, []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("hello"),
	}) {
		t.Fatalf("expected bridge streaming to be enabled for non-tool prompt")
	}
	if !client.shouldUsePkgAIBridgeStreaming(context.Background(), &PortalMetadata{
		Capabilities: ModelCapabilities{SupportsToolCalling: true},
	}, []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("hello"),
	}) {
		t.Fatalf("expected bridge streaming enabled when tool calling has no active tools")
	}
	if client.shouldUsePkgAIBridgeStreaming(context.Background(), &PortalMetadata{
		Capabilities: ModelCapabilities{SupportsToolCalling: true},
		AgentID:      "agent-1",
	}, []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("hello"),
	}) {
		t.Fatalf("expected bridge streaming disabled when agent tool mode is active")
	}
}

func TestBuildPkgAIBridgeGenerateParams(t *testing.T) {
	client := &AIClient{}
	meta := &PortalMetadata{
		Model:               "claude-sonnet-4-5",
		SystemPrompt:        "You are helpful",
		Temperature:         0.2,
		MaxCompletionTokens: 2048,
		ReasoningEffort:     "medium",
		Capabilities: ModelCapabilities{
			SupportsReasoning: true,
		},
	}
	params := client.buildPkgAIBridgeGenerateParams(meta, []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("ignored"),
		openai.UserMessage("hello"),
		openai.AssistantMessage("hi"),
	})
	if params.Model != "claude-sonnet-4-5" {
		t.Fatalf("unexpected model mapping: %q", params.Model)
	}
	if params.SystemPrompt != "You are helpful" {
		t.Fatalf("unexpected system prompt mapping: %q", params.SystemPrompt)
	}
	if params.Temperature != 0.2 {
		t.Fatalf("unexpected temperature mapping: %f", params.Temperature)
	}
	if params.MaxCompletionTokens != 2048 {
		t.Fatalf("unexpected max token mapping: %d", params.MaxCompletionTokens)
	}
	if params.ReasoningEffort != "medium" {
		t.Fatalf("unexpected reasoning mapping: %q", params.ReasoningEffort)
	}
	if len(params.Messages) != 2 {
		t.Fatalf("expected mapped user+assistant messages, got %d", len(params.Messages))
	}
}

func TestPkgAIProviderBridgeCredentials(t *testing.T) {
	client := &AIClient{
		provider: &OpenAIProvider{
			baseURL: "https://api.anthropic.com",
			apiKey:  "secret",
		},
	}
	baseURL, apiKey, ok := client.pkgAIProviderBridgeCredentials()
	if !ok {
		t.Fatalf("expected credential extraction for OpenAIProvider")
	}
	if baseURL != "https://api.anthropic.com" || apiKey != "secret" {
		t.Fatalf("unexpected credential extraction: %q %q", baseURL, apiKey)
	}
}
