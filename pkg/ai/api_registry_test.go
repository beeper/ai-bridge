package ai

import "testing"

func TestAPIRegistryLifecycle(t *testing.T) {
	ClearAPIProviders()
	t.Cleanup(ClearAPIProviders)

	RegisterAPIProvider(APIProvider{
		API: APIOpenAIResponses,
		StreamSimple: func(model Model, context Context, options *SimpleStreamOptions) *AssistantMessageEventStream {
			return NewAssistantMessageEventStream(1)
		},
	}, "source-a")

	RegisterAPIProvider(APIProvider{
		API: APIAnthropicMessages,
		Stream: func(model Model, context Context, options *StreamOptions) *AssistantMessageEventStream {
			return NewAssistantMessageEventStream(1)
		},
	}, "source-b")

	providers := GetAPIProviders()
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}
	if providers[0].API != APIAnthropicMessages || providers[1].API != APIOpenAIResponses {
		t.Fatalf("expected providers sorted by api, got %#v", providers)
	}

	if _, ok := GetAPIProvider(APIOpenAIResponses); !ok {
		t.Fatalf("expected openai responses provider in registry")
	}

	UnregisterAPIProviders("source-a")
	if _, ok := GetAPIProvider(APIOpenAIResponses); ok {
		t.Fatalf("expected source-a providers to be removed")
	}
	if _, ok := GetAPIProvider(APIAnthropicMessages); !ok {
		t.Fatalf("expected source-b provider to remain")
	}
}
