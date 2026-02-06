package connector

import (
	"errors"
	"testing"

	"github.com/openai/openai-go/v3"
)

func TestParseContextLengthError_OpenAIStyle(t *testing.T) {
	err := &openai.Error{
		StatusCode: 400,
		Message:    "This model's maximum context length is 128000 tokens. However, your messages resulted in 130532 tokens.",
	}

	cle := ParseContextLengthError(err)
	if cle == nil {
		t.Fatal("expected context length error")
	}
	if cle.ModelMaxTokens != 128000 {
		t.Fatalf("expected max tokens 128000, got %d", cle.ModelMaxTokens)
	}
	if cle.RequestedTokens != 130532 {
		t.Fatalf("expected requested tokens 130532, got %d", cle.RequestedTokens)
	}
}

func TestParseContextLengthError_OpenRouterAnthropicStyle(t *testing.T) {
	err := errors.New(`POST "https://ai.bt.hn/openrouter/v1/chat/completions": 400 Bad Request {"message":"Provider returned error","code":400,"metadata":{"raw":"{\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"message\":\"prompt is too long: 217779 tokens > 200000 maximum\"}}"}}`)

	cle := ParseContextLengthError(err)
	if cle == nil {
		t.Fatal("expected context length error")
	}
	if cle.ModelMaxTokens != 200000 {
		t.Fatalf("expected max tokens 200000, got %d", cle.ModelMaxTokens)
	}
	if cle.RequestedTokens != 217779 {
		t.Fatalf("expected requested tokens 217779, got %d", cle.RequestedTokens)
	}
}

func TestParseContextLengthError_NonContextError(t *testing.T) {
	err := errors.New("POST https://example.test/chat/completions: 400 Bad Request invalid schema")
	if cle := ParseContextLengthError(err); cle != nil {
		t.Fatal("expected nil for non-context error")
	}
}
