package providers

import "github.com/beeper/ai-bridge/pkg/ai"

func BuildBaseOptions(model ai.Model, options *ai.SimpleStreamOptions, apiKey string) ai.StreamOptions {
	if options == nil {
		return ai.StreamOptions{
			MaxTokens: minInt(model.MaxTokens, 32000),
			APIKey:    apiKey,
		}
	}
	maxTokens := options.MaxTokens
	if maxTokens <= 0 {
		maxTokens = minInt(model.MaxTokens, 32000)
	}
	return ai.StreamOptions{
		Temperature:     options.Temperature,
		MaxTokens:       maxTokens,
		Ctx:             options.Ctx,
		APIKey:          coalesce(apiKey, options.APIKey),
		Transport:       options.Transport,
		CacheRetention:  options.CacheRetention,
		SessionID:       options.SessionID,
		OnPayload:       options.OnPayload,
		Headers:         options.Headers,
		MaxRetryDelayMs: options.MaxRetryDelayMs,
		Metadata:        options.Metadata,
	}
}

func ClampReasoning(effort ai.ThinkingLevel) ai.ThinkingLevel {
	if effort == ai.ThinkingXHigh {
		return ai.ThinkingHigh
	}
	return effort
}

func AdjustMaxTokensForThinking(
	baseMaxTokens int,
	modelMaxTokens int,
	reasoningLevel ai.ThinkingLevel,
	customBudgets ai.ThinkingBudgets,
) (maxTokens int, thinkingBudget int) {
	defaultBudgets := ai.ThinkingBudgets{
		Minimal: 1024,
		Low:     2048,
		Medium:  8192,
		High:    16384,
	}
	budgets := mergeThinkingBudgets(defaultBudgets, customBudgets)
	level := ClampReasoning(reasoningLevel)
	switch level {
	case ai.ThinkingMinimal:
		thinkingBudget = budgets.Minimal
	case ai.ThinkingLow:
		thinkingBudget = budgets.Low
	case ai.ThinkingMedium:
		thinkingBudget = budgets.Medium
	default:
		thinkingBudget = budgets.High
	}

	maxTokens = minInt(baseMaxTokens+thinkingBudget, modelMaxTokens)
	minOutput := 1024
	if maxTokens <= thinkingBudget {
		thinkingBudget = maxInt(0, maxTokens-minOutput)
	}
	return maxTokens, thinkingBudget
}

func mergeThinkingBudgets(base, custom ai.ThinkingBudgets) ai.ThinkingBudgets {
	out := base
	if custom.Minimal > 0 {
		out.Minimal = custom.Minimal
	}
	if custom.Low > 0 {
		out.Low = custom.Low
	}
	if custom.Medium > 0 {
		out.Medium = custom.Medium
	}
	if custom.High > 0 {
		out.High = custom.High
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func coalesce(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}
