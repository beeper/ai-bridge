package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type openRouterProvider struct {
	cfg OpenRouterConfig
}

func newOpenRouterProvider(cfg *Config) Provider {
	if cfg == nil {
		return nil
	}
	enabled := isEnabled(cfg.OpenRouter.Enabled, true)
	if !enabled {
		return nil
	}
	apiKey := strings.TrimSpace(cfg.OpenRouter.APIKey)
	if apiKey == "" {
		return nil
	}
	return &openRouterProvider{cfg: cfg.OpenRouter}
}

func (p *openRouterProvider) Name() string {
	return ProviderOpenRouter
}

func (p *openRouterProvider) Search(ctx context.Context, req Request) (*Response, error) {
	endpoint := strings.TrimRight(p.cfg.BaseURL, "/") + "/chat/completions"
	payload := map[string]any{
		"model": p.cfg.Model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": req.Query,
			},
		},
		"plugins": []map[string]any{
			{
				"id":          "web",
				"max_results": clampCount(req.Count),
			},
		},
	}
	start := time.Now()
	data, _, err := postJSON(ctx, endpoint, map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", p.cfg.APIKey),
	}, payload, p.cfg.TimeoutSecs)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	answer := ""
	if len(resp.Choices) > 0 {
		answer = strings.TrimSpace(resp.Choices[0].Message.Content)
	}

	return &Response{
		Query:     req.Query,
		Provider:  ProviderOpenRouter,
		TookMs:    time.Since(start).Milliseconds(),
		Answer:    answer,
		NoResults: answer == "",
	}, nil
}

func clampCount(value int) int {
	if value <= 0 {
		return DefaultSearchCount
	}
	if value > MaxSearchCount {
		return MaxSearchCount
	}
	return value
}
