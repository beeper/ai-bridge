package providers

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/beeper/ai-bridge/pkg/ai"
)

func TestStreamAzureOpenAIResponses_MissingBaseURLEmitsError(t *testing.T) {
	t.Setenv("AZURE_OPENAI_BASE_URL", "")
	t.Setenv("AZURE_OPENAI_RESOURCE_NAME", "")
	t.Setenv("AZURE_OPENAI_API_KEY", "test-key")
	stream := streamAzureOpenAIResponses(ai.Model{
		ID:       "gpt-4.1-mini",
		Provider: "azure-openai-responses",
		API:      ai.APIAzureOpenAIResponse,
	}, ai.Context{
		Messages: []ai.Message{{Role: ai.RoleUser, Text: "hello"}},
	}, &ai.StreamOptions{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	evt, err := stream.Next(ctx)
	if err != nil {
		t.Fatalf("expected terminal error event, got %v", err)
	}
	if evt.Type != ai.EventError {
		t.Fatalf("expected error event, got %s", evt.Type)
	}
	if !strings.Contains(strings.ToLower(evt.Error.ErrorMessage), "base url") {
		t.Fatalf("expected base url error message, got %q", evt.Error.ErrorMessage)
	}
	if _, err = stream.Next(ctx); err != io.EOF {
		t.Fatalf("expected EOF after terminal event, got %v", err)
	}
}

func TestStreamAzureOpenAIResponses_MissingAPIKeyEmitsError(t *testing.T) {
	t.Setenv("AZURE_OPENAI_BASE_URL", "https://my-resource.openai.azure.com/openai/v1")
	t.Setenv("AZURE_OPENAI_API_KEY", "")
	stream := streamAzureOpenAIResponses(ai.Model{
		ID:       "gpt-4.1-mini",
		Provider: "azure-openai-responses",
		API:      ai.APIAzureOpenAIResponse,
	}, ai.Context{
		Messages: []ai.Message{{Role: ai.RoleUser, Text: "hello"}},
	}, &ai.StreamOptions{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	evt, err := stream.Next(ctx)
	if err != nil {
		t.Fatalf("expected terminal error event, got %v", err)
	}
	if evt.Type != ai.EventError {
		t.Fatalf("expected error event, got %s", evt.Type)
	}
	if !strings.Contains(strings.ToLower(evt.Error.ErrorMessage), "api key") {
		t.Fatalf("expected missing api key message, got %q", evt.Error.ErrorMessage)
	}
	if _, err = stream.Next(ctx); err != io.EOF {
		t.Fatalf("expected EOF after terminal event, got %v", err)
	}
}
