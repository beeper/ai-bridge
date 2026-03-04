package utils

import (
	"testing"

	"github.com/beeper/ai-bridge/pkg/ai"
)

func TestNormalizeUsageTotalTokens(t *testing.T) {
	usage := NormalizeUsageTotalTokens(ai.Usage{
		Input:      100,
		Output:     50,
		CacheRead:  10,
		CacheWrite: 5,
	})
	if usage.TotalTokens != 165 {
		t.Fatalf("expected computed totalTokens=165, got %d", usage.TotalTokens)
	}

	usage = NormalizeUsageTotalTokens(ai.Usage{
		Input:       100,
		Output:      50,
		CacheRead:   10,
		CacheWrite:  5,
		TotalTokens: 120,
	})
	if usage.TotalTokens != 165 {
		t.Fatalf("expected totalTokens uplifted to components sum=165, got %d", usage.TotalTokens)
	}

	usage = NormalizeUsageTotalTokens(ai.Usage{
		Input:       100,
		Output:      50,
		CacheRead:   10,
		CacheWrite:  5,
		TotalTokens: 200,
	})
	if usage.TotalTokens != 200 {
		t.Fatalf("expected larger totalTokens to be preserved, got %d", usage.TotalTokens)
	}
}
