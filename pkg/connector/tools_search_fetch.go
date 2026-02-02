package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/beeper/ai-bridge/pkg/fetch"
	"github.com/beeper/ai-bridge/pkg/search"
	"github.com/beeper/ai-bridge/pkg/shared/websearch"
)

func executeWebSearchWithProviders(ctx context.Context, args map[string]any) (string, error) {
	query, ok := args["query"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid 'query' argument")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("missing or invalid 'query' argument")
	}

	count, _ := websearch.ParseCountAndIgnoredOptions(args)
	country, _ := args["country"].(string)
	searchLang, _ := args["search_lang"].(string)
	uiLang, _ := args["ui_lang"].(string)
	freshness, _ := args["freshness"].(string)

	req := search.Request{
		Query:      query,
		Count:      count,
		Country:    strings.TrimSpace(country),
		SearchLang: strings.TrimSpace(searchLang),
		UILang:     strings.TrimSpace(uiLang),
		Freshness:  strings.TrimSpace(freshness),
	}

	cfg := resolveSearchConfig(ctx)
	resp, err := search.Search(ctx, req, cfg)
	if err != nil {
		return "", err
	}

	payload := map[string]any{
		"query":      resp.Query,
		"provider":   resp.Provider,
		"count":      resp.Count,
		"tookMs":     resp.TookMs,
		"answer":     resp.Answer,
		"summary":    resp.Summary,
		"definition": resp.Definition,
		"warning":    resp.Warning,
		"noResults":  resp.NoResults,
		"cached":     resp.Cached,
	}

	if len(resp.Results) > 0 {
		results := make([]map[string]any, 0, len(resp.Results))
		for _, r := range resp.Results {
			results = append(results, map[string]any{
				"title":       r.Title,
				"url":         r.URL,
				"description": r.Description,
				"published":   r.Published,
				"siteName":    r.SiteName,
			})
		}
		payload["results"] = results
	}

	if resp.Extras != nil {
		payload["extras"] = resp.Extras
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to encode web_search response: %w", err)
	}
	return string(raw), nil
}

func executeWebFetchWithProviders(ctx context.Context, args map[string]any) (string, error) {
	urlStr, ok := args["url"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid 'url' argument")
	}
	urlStr = strings.TrimSpace(urlStr)
	if urlStr == "" {
		return "", fmt.Errorf("missing or invalid 'url' argument")
	}

	extractMode := "markdown"
	if mode, ok := args["extractMode"].(string); ok && strings.EqualFold(strings.TrimSpace(mode), "text") {
		extractMode = "text"
	}

	maxChars := 0
	if mc, ok := args["max_chars"].(float64); ok && mc > 0 {
		maxChars = int(mc)
	} else if mc, ok := args["maxChars"].(float64); ok && mc > 0 {
		maxChars = int(mc)
	}

	req := fetch.Request{
		URL:         urlStr,
		ExtractMode: extractMode,
		MaxChars:    maxChars,
	}

	cfg := resolveFetchConfig(ctx)
	resp, err := fetch.Fetch(ctx, req, cfg)
	if err != nil {
		return "", err
	}

	payload := map[string]any{
		"url":           resp.URL,
		"finalUrl":      resp.FinalURL,
		"status":        resp.Status,
		"contentType":   resp.ContentType,
		"extractMode":   resp.ExtractMode,
		"extractor":     resp.Extractor,
		"truncated":     resp.Truncated,
		"length":        resp.Length,
		"rawLength":     resp.RawLength,
		"wrappedLength": resp.WrappedLength,
		"fetchedAt":     resp.FetchedAt,
		"tookMs":        resp.TookMs,
		"text":          resp.Text,
		"content":       resp.Text,
		"provider":      resp.Provider,
		"warning":       resp.Warning,
		"cached":        resp.Cached,
	}
	if resp.Extras != nil {
		payload["extras"] = resp.Extras
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to encode web_fetch response: %w", err)
	}
	return string(raw), nil
}

func resolveSearchConfig(ctx context.Context) *search.Config {
	var cfg *search.Config
	if btc := GetBridgeToolContext(ctx); btc != nil && btc.Client != nil {
		src := btc.Client.connector.Config.Tools.Search
		cfg = mapSearchConfig(src)
	}
	return search.ApplyEnvDefaults(cfg)
}

func resolveFetchConfig(ctx context.Context) *fetch.Config {
	var cfg *fetch.Config
	if btc := GetBridgeToolContext(ctx); btc != nil && btc.Client != nil {
		src := btc.Client.connector.Config.Tools.Fetch
		cfg = mapFetchConfig(src)
	}
	return fetch.ApplyEnvDefaults(cfg)
}

func mapSearchConfig(src *SearchConfig) *search.Config {
	if src == nil {
		return nil
	}
	return &search.Config{
		Provider:  src.Provider,
		Fallbacks: src.Fallbacks,
		Proxy: search.ProxyConfig{
			Enabled:       src.Proxy.Enabled,
			BaseURL:       src.Proxy.BaseURL,
			APIKey:        src.Proxy.APIKey,
			SearchPath:    src.Proxy.SearchPath,
			TimeoutSecs:   src.Proxy.TimeoutSecs,
			CacheTtlSecs:  src.Proxy.CacheTtlSecs,
			ForwardHeader: src.Proxy.ForwardHeader,
		},
		Exa: search.ExaConfig{
			Enabled:           src.Exa.Enabled,
			BaseURL:           src.Exa.BaseURL,
			APIKey:            src.Exa.APIKey,
			Type:              src.Exa.Type,
			Category:          src.Exa.Category,
			NumResults:        src.Exa.NumResults,
			IncludeText:       src.Exa.IncludeText,
			TextMaxCharacters: src.Exa.TextMaxCharacters,
			Highlights:        src.Exa.Highlights,
		},
		Brave: search.BraveConfig{
			Enabled:          src.Brave.Enabled,
			BaseURL:          src.Brave.BaseURL,
			APIKey:           src.Brave.APIKey,
			TimeoutSecs:      src.Brave.TimeoutSecs,
			CacheTtlSecs:     src.Brave.CacheTtlSecs,
			SearchLang:       src.Brave.SearchLang,
			UILang:           src.Brave.UILang,
			DefaultCountry:   src.Brave.DefaultCountry,
			DefaultFreshness: src.Brave.DefaultFreshness,
		},
		Perplexity: search.PerplexityConfig{
			Enabled:      src.Perplexity.Enabled,
			APIKey:       src.Perplexity.APIKey,
			BaseURL:      src.Perplexity.BaseURL,
			Model:        src.Perplexity.Model,
			TimeoutSecs:  src.Perplexity.TimeoutSecs,
			CacheTtlSecs: src.Perplexity.CacheTtlSecs,
		},
		OpenRouter: search.OpenRouterConfig{
			Enabled:      src.OpenRouter.Enabled,
			APIKey:       src.OpenRouter.APIKey,
			BaseURL:      src.OpenRouter.BaseURL,
			Model:        src.OpenRouter.Model,
			TimeoutSecs:  src.OpenRouter.TimeoutSecs,
			CacheTtlSecs: src.OpenRouter.CacheTtlSecs,
		},
		DDG: search.DDGConfig{
			Enabled:     src.DDG.Enabled,
			TimeoutSecs: src.DDG.TimeoutSecs,
		},
	}
}

func mapFetchConfig(src *FetchConfig) *fetch.Config {
	if src == nil {
		return nil
	}
	return &fetch.Config{
		Provider:  src.Provider,
		Fallbacks: src.Fallbacks,
		Proxy: fetch.ProxyConfig{
			Enabled:      src.Proxy.Enabled,
			BaseURL:      src.Proxy.BaseURL,
			APIKey:       src.Proxy.APIKey,
			ContentsPath: src.Proxy.ContentsPath,
			TimeoutSecs:  src.Proxy.TimeoutSecs,
		},
		Exa: fetch.ExaConfig{
			Enabled:           src.Exa.Enabled,
			BaseURL:           src.Exa.BaseURL,
			APIKey:            src.Exa.APIKey,
			IncludeText:       src.Exa.IncludeText,
			TextMaxCharacters: src.Exa.TextMaxCharacters,
		},
		Direct: fetch.DirectConfig{
			Enabled:      src.Direct.Enabled,
			TimeoutSecs:  src.Direct.TimeoutSecs,
			UserAgent:    src.Direct.UserAgent,
			Readability:  src.Direct.Readability,
			MaxChars:     src.Direct.MaxChars,
			MaxRedirects: src.Direct.MaxRedirects,
			CacheTtlSecs: src.Direct.CacheTtlSecs,
		},
	}
}
