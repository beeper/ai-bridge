package providers

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/beeper/ai-bridge/pkg/ai"
)

func TestStreamBedrockConverse_MissingCredentialsEmitsError(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "")
	t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "")
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")

	stream := streamBedrockConverse(ai.Model{
		ID:       "us.anthropic.claude-3-5-sonnet-20241022-v2:0",
		Provider: "amazon-bedrock",
		API:      ai.APIBedrockConverse,
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
	if !strings.Contains(strings.ToLower(evt.Error.ErrorMessage), "credentials") {
		t.Fatalf("expected missing credentials message, got %q", evt.Error.ErrorMessage)
	}
	if _, err := stream.Next(ctx); err != io.EOF {
		t.Fatalf("expected EOF after terminal event, got %v", err)
	}
}

func TestMapBedrockStopReason(t *testing.T) {
	cases := map[bedrocktypes.StopReason]ai.StopReason{
		bedrocktypes.StopReasonEndTurn:      ai.StopReasonStop,
		bedrocktypes.StopReasonStopSequence: ai.StopReasonStop,
		bedrocktypes.StopReasonMaxTokens:    ai.StopReasonLength,
		bedrocktypes.StopReasonToolUse:      ai.StopReasonToolUse,
	}
	for in, want := range cases {
		if got := mapBedrockStopReason(in); got != want {
			t.Fatalf("mapBedrockStopReason(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMapBedrockToolChoice(t *testing.T) {
	if mapBedrockToolChoice("none") != nil {
		t.Fatalf("expected none tool choice to map to nil")
	}
	if got := mapBedrockToolChoice("any"); got == nil {
		t.Fatalf("expected any tool choice")
	}
	if got := mapBedrockToolChoice("auto"); got == nil {
		t.Fatalf("expected auto tool choice")
	}
}
