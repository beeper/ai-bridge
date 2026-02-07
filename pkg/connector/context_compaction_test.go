package connector

import (
	"context"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/rs/zerolog"
)

func TestCompactorCreation(t *testing.T) {
	log := zerolog.Nop()
	config := DefaultCompactionConfig()

	compactor := NewCompactor(nil, log, config)
	if compactor == nil {
		t.Fatal("NewCompactor returned nil")
	}

	if compactor.config == nil {
		t.Error("Compactor config is nil")
	}

	if compactor.config.MaxHistoryShare != 0.5 {
		t.Errorf("Expected MaxHistoryShare 0.5, got %f", compactor.config.MaxHistoryShare)
	}
}

func TestCompactorSplitMessages(t *testing.T) {
	log := zerolog.Nop()
	config := DefaultCompactionConfig()
	config.MaxHistoryShare = 0.3 // Force compaction by using low threshold

	compactor := NewCompactor(nil, log, config)

	// Create test messages
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("You are a helpful assistant."),
		openai.UserMessage("Hello, how are you?"),
		openai.AssistantMessage("I'm doing well, thank you!"),
		openai.UserMessage("What is 2+2?"),
		openai.AssistantMessage("2+2 equals 4."),
		openai.UserMessage("Thanks!"),
		openai.AssistantMessage("You're welcome!"),
	}

	// With a small context window, should split messages
	toSummarize, toKeep := compactor.splitMessagesForCompaction(messages, 1000)

	// System prompt should always be kept
	hasSystem := false
	for _, msg := range toKeep {
		if msg.OfSystem != nil {
			hasSystem = true
			break
		}
	}
	if !hasSystem {
		t.Error("System message should be kept, not summarized")
	}

	// Recent assistant messages should be protected
	if len(toKeep) < 3 {
		t.Errorf("Expected at least 3 messages kept (including recent assistants), got %d", len(toKeep))
	}

	t.Logf("Split result: %d to summarize, %d to keep", len(toSummarize), len(toKeep))
}

func TestCompactorFallbackSummary(t *testing.T) {
	log := zerolog.Nop()
	config := DefaultCompactionConfig()
	compactor := NewCompactor(nil, log, config)

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("Hello"),
		openai.AssistantMessage("Hi there!"),
		openai.UserMessage("How are you?"),
		openai.AssistantMessage("I'm good!"),
	}

	summary := compactor.generateFallbackSummary(messages)

	if summary == "" {
		t.Error("Fallback summary should not be empty")
	}

	if len(summary) < 20 {
		t.Errorf("Fallback summary too short: %s", summary)
	}

	t.Logf("Fallback summary: %s", summary)
}

func TestCompactionHooks(t *testing.T) {
	// Test hook registration
	hookCalled := false

	RegisterBeforeCompactionHook(func(ctx context.Context, hookCtx *CompactionHookContext) (*CompactionHookResult, error) {
		hookCalled = true
		return &CompactionHookResult{Skip: true}, nil
	})

	// Run hooks
	result, err := globalCompactionHooks.runBeforeHooks(context.Background(), &CompactionHookContext{
		SessionID:    "test",
		MessageCount: 10,
		TokenCount:   1000,
	})

	if err != nil {
		t.Errorf("Hook returned error: %v", err)
	}

	if !hookCalled {
		t.Error("Before hook was not called")
	}

	if result == nil || !result.Skip {
		t.Error("Hook result should indicate skip")
	}
}

func TestCompactionConfig(t *testing.T) {
	config := DefaultCompactionConfig()

	if config.PruningConfig == nil {
		t.Error("PruningConfig should not be nil in default config")
	}

	if config.SummarizationEnabled == nil || !*config.SummarizationEnabled {
		t.Error("SummarizationEnabled should default to true")
	}

	if config.MaxSummaryTokens != 500 {
		t.Errorf("Expected MaxSummaryTokens 500, got %d", config.MaxSummaryTokens)
	}

	if config.MaxHistoryShare != 0.5 {
		t.Errorf("Expected MaxHistoryShare 0.5, got %f", config.MaxHistoryShare)
	}

	if config.ReserveTokens != 20000 {
		t.Errorf("Expected ReserveTokens 20000, got %d", config.ReserveTokens)
	}
}

func TestCompactionEventTypes(t *testing.T) {
	if CompactionEventStart != "compaction_start" {
		t.Errorf("Expected CompactionEventStart to be 'compaction_start', got %s", CompactionEventStart)
	}

	if CompactionEventEnd != "compaction_end" {
		t.Errorf("Expected CompactionEventEnd to be 'compaction_end', got %s", CompactionEventEnd)
	}

}

func TestEstimateTotalTokens(t *testing.T) {
	log := zerolog.Nop()
	config := DefaultCompactionConfig()
	compactor := NewCompactor(nil, log, config)

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage("You are a helpful assistant."),
		openai.UserMessage("Hello, how are you?"),
		openai.AssistantMessage("I'm doing well, thank you for asking!"),
	}

	tokens := compactor.estimateTotalTokens(messages)

	if tokens <= 0 {
		t.Error("Token estimate should be positive")
	}

	// Rough estimate: ~20 words = ~25-30 tokens
	if tokens < 10 || tokens > 100 {
		t.Errorf("Token estimate seems off: %d", tokens)
	}

	t.Logf("Estimated tokens: %d", tokens)
}
