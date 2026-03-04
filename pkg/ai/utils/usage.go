package utils

import "github.com/beeper/ai-bridge/pkg/ai"

// NormalizeUsageTotalTokens keeps usage.totalTokens coherent with component counters.
// Some providers omit total tokens in partial/aborted responses; this computes a safe fallback.
func NormalizeUsageTotalTokens(usage ai.Usage) ai.Usage {
	computed := usage.Input + usage.Output + usage.CacheRead + usage.CacheWrite
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = computed
		return usage
	}
	if usage.TotalTokens < computed {
		usage.TotalTokens = computed
	}
	return usage
}
