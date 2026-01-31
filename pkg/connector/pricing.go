package connector

import (
	"fmt"
	"strings"
)

// ModelPricing stores the cost per million tokens for input and output.
type ModelPricing struct {
	InputPerMillion  float64 // Cost per 1M input tokens
	OutputPerMillion float64 // Cost per 1M output tokens
}

// modelPricing maps model ID prefixes to their pricing.
// Uses prefix matching to handle versioned models.
var modelPricing = map[string]ModelPricing{
	// GPT-4o family
	"gpt-4o-mini": {InputPerMillion: 0.15, OutputPerMillion: 0.60},
	"gpt-4o":      {InputPerMillion: 2.50, OutputPerMillion: 10.00},

	// GPT-4 Turbo
	"gpt-4-turbo": {InputPerMillion: 10.00, OutputPerMillion: 30.00},

	// GPT-4 (original)
	"gpt-4-32k": {InputPerMillion: 60.00, OutputPerMillion: 120.00},
	"gpt-4":     {InputPerMillion: 30.00, OutputPerMillion: 60.00},

	// GPT-3.5 Turbo
	"gpt-3.5-turbo": {InputPerMillion: 0.50, OutputPerMillion: 1.50},

	// O1 reasoning models
	"o1-preview": {InputPerMillion: 15.00, OutputPerMillion: 60.00},
	"o1-mini":    {InputPerMillion: 3.00, OutputPerMillion: 12.00},
	"o1":         {InputPerMillion: 15.00, OutputPerMillion: 60.00},

	// O3 reasoning models (estimated)
	"o3-mini": {InputPerMillion: 1.10, OutputPerMillion: 4.40},
	"o3":      {InputPerMillion: 10.00, OutputPerMillion: 40.00},
}

// GetModelPricing returns the pricing for a model, matching by prefix.
// Returns nil if no pricing is found.
func GetModelPricing(modelID string) *ModelPricing {
	modelID = strings.ToLower(modelID)

	// Try exact match first
	if pricing, ok := modelPricing[modelID]; ok {
		return &pricing
	}

	// Try prefix matching (longer prefixes first for specificity)
	prefixes := []string{
		"gpt-4o-mini",
		"gpt-4o",
		"gpt-4-turbo",
		"gpt-4-32k",
		"gpt-4",
		"gpt-3.5-turbo",
		"o1-preview",
		"o1-mini",
		"o3-mini",
		"o1",
		"o3",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(modelID, prefix) {
			pricing := modelPricing[prefix]
			return &pricing
		}
	}

	return nil
}

// CalculateCost calculates the cost in USD for given token counts.
func CalculateCost(modelID string, promptTokens, completionTokens int64) float64 {
	pricing := GetModelPricing(modelID)
	if pricing == nil {
		return 0
	}

	inputCost := float64(promptTokens) / 1_000_000 * pricing.InputPerMillion
	outputCost := float64(completionTokens) / 1_000_000 * pricing.OutputPerMillion

	return inputCost + outputCost
}

// FormatCost formats a cost in USD for display.
func FormatCost(cost float64) string {
	if cost < 0.0001 {
		return "$0.00"
	}
	if cost < 0.01 {
		return "<$0.01"
	}
	return fmt.Sprintf("$%.2f", cost)
}
