package connector

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/beeper/ai-bridge/pkg/search"
)

// executeWebSearchOpenRouter performs web search via OpenRouter's web plugin only.
func executeWebSearchOpenRouter(ctx context.Context, args map[string]any) (string, error) {
	req, err := searchRequestFromArgs(args)
	if err != nil {
		return "", err
	}

	cfg := resolveOpenRouterSearchConfig(ctx)
	resp, err := search.Search(ctx, req, cfg)
	if err != nil {
		return "", err
	}

	payload := buildSearchPayload(resp)
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to encode web_search_openrouter response: %w", err)
	}
	return string(raw), nil
}

func resolveOpenRouterSearchConfig(ctx context.Context) *search.Config {
	cfg := resolveSearchConfig(ctx)
	if cfg == nil {
		cfg = &search.Config{}
	}
	cfg.Provider = search.ProviderOpenRouter
	cfg.Fallbacks = []string{search.ProviderOpenRouter}
	return cfg
}
