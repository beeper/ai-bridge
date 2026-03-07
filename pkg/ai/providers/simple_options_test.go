package providers

import (
	"testing"

	"github.com/beeper/ai-bridge/pkg/ai"
)

func TestClampReasoning(t *testing.T) {
	if got := ClampReasoning(ai.ThinkingXHigh); got != ai.ThinkingHigh {
		t.Fatalf("expected xhigh to clamp to high, got %s", got)
	}
	if got := ClampReasoning(ai.ThinkingLow); got != ai.ThinkingLow {
		t.Fatalf("expected low to stay low, got %s", got)
	}
}

func TestAdjustMaxTokensForThinking(t *testing.T) {
	maxTokens, budget := AdjustMaxTokensForThinking(4000, 10000, ai.ThinkingMedium, ai.ThinkingBudgets{})
	if maxTokens != 10000 {
		t.Fatalf("expected maxTokens capped to model max 10000, got %d", maxTokens)
	}
	if budget != 8192 {
		t.Fatalf("expected medium budget 8192, got %d", budget)
	}

	maxTokens, budget = AdjustMaxTokensForThinking(1200, 1500, ai.ThinkingHigh, ai.ThinkingBudgets{})
	if budget < 0 || maxTokens <= 0 {
		t.Fatalf("expected non-negative budget and positive max tokens, got max=%d budget=%d", maxTokens, budget)
	}

	maxTokens, budget = AdjustMaxTokensForThinking(1000, 9000, ai.ThinkingLow, ai.ThinkingBudgets{Low: 3333})
	if budget != 3333 {
		t.Fatalf("expected custom low budget 3333, got %d", budget)
	}
	if maxTokens != 4333 {
		t.Fatalf("expected max tokens 4333, got %d", maxTokens)
	}
}

func TestBuildBaseOptions(t *testing.T) {
	model := ai.Model{MaxTokens: 8000}
	temp := 0.2
	opts := &ai.SimpleStreamOptions{
		StreamOptions: ai.StreamOptions{
			Temperature: &temp,
			MaxTokens:   0,
			APIKey:      "from-options",
		},
	}
	base := BuildBaseOptions(model, opts, "from-param")
	if base.APIKey != "from-param" {
		t.Fatalf("expected explicit apiKey to win, got %s", base.APIKey)
	}
	if base.MaxTokens != 8000 {
		t.Fatalf("expected default maxTokens=min(model,32000)=8000, got %d", base.MaxTokens)
	}
	if base.Temperature == nil || *base.Temperature != 0.2 {
		t.Fatalf("unexpected temperature in base options")
	}
}
