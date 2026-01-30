package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// OpenRouterCache provides runtime model validation by fetching and caching
// model lists from OpenRouter's API.
type OpenRouterCache struct {
	models    map[string]OpenRouterModelInfo
	mu        sync.RWMutex
	lastFetch time.Time
	ttl       time.Duration
	apiKey    string
	baseURL   string
}

// OpenRouterModelInfo stores cached model information from OpenRouter
type OpenRouterModelInfo struct {
	ID            string
	Name          string
	ContextWindow int
	MaxOutput     int
	Pricing       *OpenRouterPricing
}

// OpenRouterPricing contains cost information for a model
type OpenRouterPricing struct {
	PromptCostPer1M     float64
	CompletionCostPer1M float64
}

// openRouterModelsResponse represents the response from /api/v1/models
type openRouterModelsResponse struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	ContextLength int                    `json:"context_length"`
	TopProvider   *openRouterTopProvider `json:"top_provider,omitempty"`
	Pricing       *openRouterPricing     `json:"pricing,omitempty"`
}

type openRouterTopProvider struct {
	MaxCompletionTokens int `json:"max_completion_tokens,omitempty"`
}

type openRouterPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

const (
	defaultCacheTTL     = 1 * time.Hour
	modelFetchTimeout   = 30 * time.Second
	openRouterModelsURL = "https://openrouter.ai/api/v1/models"
)

// globalOpenRouterCache is the package-level model cache instance
var globalOpenRouterCache = &OpenRouterCache{
	models:  make(map[string]OpenRouterModelInfo),
	ttl:     defaultCacheTTL,
	baseURL: openRouterModelsURL,
}

// InitOpenRouterCache initializes the global model cache with an API key
func InitOpenRouterCache(apiKey string) {
	globalOpenRouterCache.mu.Lock()
	defer globalOpenRouterCache.mu.Unlock()
	globalOpenRouterCache.apiKey = apiKey
}

// IsValidOpenRouterModel checks if a model ID is valid according to the cache
// Returns true if the model exists or if the cache is unavailable (fail open)
func IsValidOpenRouterModel(modelID string) bool {
	return globalOpenRouterCache.IsValidModel(modelID)
}

// GetOpenRouterModelInfo returns cached model info for a model ID
func GetOpenRouterModelInfo(modelID string) (OpenRouterModelInfo, bool) {
	return globalOpenRouterCache.GetModelInfo(modelID)
}

// GetOpenRouterContextWindow returns the context window for a model from cache
// Returns 0 if model not found in cache
func GetOpenRouterContextWindow(modelID string) int {
	info, ok := globalOpenRouterCache.GetModelInfo(modelID)
	if !ok {
		return 0
	}
	return info.ContextWindow
}

// RefreshOpenRouterCache forces a refresh of the model cache
func RefreshOpenRouterCache(ctx context.Context) error {
	return globalOpenRouterCache.Refresh(ctx)
}

// IsValidModel checks if a model ID exists in the cache
func (mc *OpenRouterCache) IsValidModel(modelID string) bool {
	mc.refreshIfNeeded()
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// If cache is empty (fetch failed), allow all models (fail open)
	if len(mc.models) == 0 {
		return true
	}

	_, ok := mc.models[modelID]
	return ok
}

// GetModelInfo returns cached model information
func (mc *OpenRouterCache) GetModelInfo(modelID string) (OpenRouterModelInfo, bool) {
	mc.refreshIfNeeded()
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	info, ok := mc.models[modelID]
	return info, ok
}

// Refresh fetches fresh model data from OpenRouter
func (mc *OpenRouterCache) Refresh(ctx context.Context) error {
	models, err := mc.fetchModels(ctx)
	if err != nil {
		return err
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.models = models
	mc.lastFetch = time.Now()
	return nil
}

// refreshIfNeeded checks if cache needs refresh and triggers it in background
func (mc *OpenRouterCache) refreshIfNeeded() {
	mc.mu.RLock()
	needsRefresh := time.Since(mc.lastFetch) > mc.ttl
	mc.mu.RUnlock()

	if needsRefresh {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), modelFetchTimeout)
			defer cancel()
			_ = mc.Refresh(ctx) // Ignore errors, keep using stale cache
		}()
	}
}

// fetchModels retrieves model list from OpenRouter API
func (mc *OpenRouterCache) fetchModels(ctx context.Context) (map[string]OpenRouterModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mc.baseURL, nil)
	if err != nil {
		return nil, err
	}

	if mc.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+mc.apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenRouter API returned status %d: %s", resp.StatusCode, string(body))
	}

	var response openRouterModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	models := make(map[string]OpenRouterModelInfo, len(response.Data))
	for _, m := range response.Data {
		info := OpenRouterModelInfo{
			ID:            m.ID,
			Name:          m.Name,
			ContextWindow: m.ContextLength,
		}

		if m.TopProvider != nil {
			info.MaxOutput = m.TopProvider.MaxCompletionTokens
		}

		if m.Pricing != nil {
			info.Pricing = parseOpenRouterPricing(m.Pricing)
		}

		models[m.ID] = info
	}

	return models, nil
}

// parseOpenRouterPricing converts string pricing to float64
func parseOpenRouterPricing(p *openRouterPricing) *OpenRouterPricing {
	if p == nil {
		return nil
	}

	var pricing OpenRouterPricing
	if p.Prompt != "" {
		// Parse as float, pricing is per token in strings
		var promptCost float64
		if err := json.Unmarshal([]byte(p.Prompt), &promptCost); err == nil {
			// OpenRouter gives cost per token, convert to per 1M
			pricing.PromptCostPer1M = promptCost * 1_000_000
		}
	}
	if p.Completion != "" {
		var completionCost float64
		if err := json.Unmarshal([]byte(p.Completion), &completionCost); err == nil {
			pricing.CompletionCostPer1M = completionCost * 1_000_000
		}
	}

	return &pricing
}
