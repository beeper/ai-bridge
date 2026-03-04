package providers

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/beeper/ai-bridge/pkg/ai"
)

func TestRegisterBuiltInAPIProviders(t *testing.T) {
	ai.ClearAPIProviders()
	t.Cleanup(ai.ClearAPIProviders)

	RegisterBuiltInAPIProviders()
	providers := ai.GetAPIProviders()
	if len(providers) < 9 {
		t.Fatalf("expected builtin providers to be registered, got %d", len(providers))
	}

	stream, err := ai.Stream(ai.Model{
		ID:       "gpt-5",
		Provider: "openai",
		API:      ai.APIOpenAIResponses,
	}, ai.Context{}, nil)
	if err != nil {
		t.Fatalf("unexpected stream resolve error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	evt, err := stream.Next(ctx)
	if err != nil {
		t.Fatalf("expected terminal error event, got %v", err)
	}
	if evt.Type != ai.EventError {
		t.Fatalf("expected error event, got %s", evt.Type)
	}
	if evt.Error.StopReason != ai.StopReasonError {
		t.Fatalf("expected stopReason=error, got %s", evt.Error.StopReason)
	}
	if _, err := stream.Next(ctx); err != io.EOF {
		t.Fatalf("expected EOF after terminal event, got %v", err)
	}
}
