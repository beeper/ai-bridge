package search

import (
	"context"
	"slices"
	"strings"

	basesearch "github.com/beeper/ai-bridge/pkg/matrixai/search"
)

type Request struct {
	Query      string
	Count      int
	Country    string
	SearchLang string
	UILang     string
	Freshness  string
}

type Result struct {
	ID          string
	Title       string
	URL         string
	Description string
	Published   string
	SiteName    string
	Author      string
	Image       string
	Favicon     string
}

type Response struct {
	Query      string
	Provider   string
	Count      int
	TookMs     int64
	Results    []Result
	Answer     string
	Summary    string
	Definition string
	Warning    string
	NoResults  bool
	Cached     bool
	Extras     map[string]any
}

type Config struct {
	Provider  string
	Fallbacks []string

	Exa        ExaConfig
	OpenRouter OpenRouterConfig
}

type ExaConfig struct {
	Enabled           *bool
	BaseURL           string
	APIKey            string
	Type              string
	Category          string
	NumResults        int
	IncludeText       bool
	TextMaxCharacters int
	Highlights        bool
}

type OpenRouterConfig struct {
	Enabled      *bool
	APIKey       string
	BaseURL      string
	Model        string
	TimeoutSecs  int
	CacheTtlSecs int
}

const (
	ProviderExa        = basesearch.ProviderExa
	ProviderOpenRouter = basesearch.ProviderOpenRouter

	DefaultSearchCount  = basesearch.DefaultSearchCount
	MaxSearchCount      = basesearch.MaxSearchCount
	DefaultTimeoutSecs  = basesearch.DefaultTimeoutSecs
	DefaultCacheTtlSecs = basesearch.DefaultCacheTtlSecs
)

var DefaultFallbackOrder = basesearch.DefaultFallbackOrder

func Search(ctx context.Context, req Request, cfg *Config) (*Response, error) {
	resp, err := basesearch.Search(ctx, toBaseRequest(req), toBaseConfig(cfg))
	if err != nil || resp == nil {
		return nil, err
	}
	converted := fromBaseResponse(*resp)
	return &converted, nil
}

func ApplyEnvDefaults(cfg *Config) *Config {
	return fromBaseConfig(basesearch.ApplyEnvDefaults(toBaseConfig(cfg)))
}

func (c *Config) WithDefaults() *Config {
	if c == nil {
		c = &Config{}
	}
	if strings.TrimSpace(c.Provider) == "" {
		if strings.TrimSpace(c.Exa.APIKey) != "" {
			c.Provider = ProviderExa
		} else {
			c.Provider = ProviderOpenRouter
		}
	}
	if len(c.Fallbacks) == 0 {
		c.Fallbacks = slices.Clone(DefaultFallbackOrder)
	}
	c.Exa = c.Exa.withDefaults()
	c.OpenRouter = c.OpenRouter.withDefaults()
	return c
}

func (c ExaConfig) withDefaults() ExaConfig {
	if c.BaseURL == "" {
		c.BaseURL = "https://api.exa.ai"
	}
	if c.Type == "" {
		c.Type = "auto"
	}
	if c.NumResults <= 0 {
		c.NumResults = DefaultSearchCount
	}
	if c.TextMaxCharacters <= 0 {
		c.TextMaxCharacters = 500
	}
	c.Highlights = true
	return c
}

func (c OpenRouterConfig) withDefaults() OpenRouterConfig {
	if c.BaseURL == "" {
		c.BaseURL = "https://openrouter.ai/api/v1"
	}
	if c.Model == "" {
		c.Model = "openai/gpt-5.2"
	}
	if c.TimeoutSecs <= 0 {
		c.TimeoutSecs = DefaultTimeoutSecs
	}
	if c.CacheTtlSecs <= 0 {
		c.CacheTtlSecs = DefaultCacheTtlSecs
	}
	return c
}

func toBaseRequest(req Request) basesearch.Request {
	return basesearch.Request{
		Query:      req.Query,
		Count:      req.Count,
		Country:    req.Country,
		SearchLang: req.SearchLang,
		UILang:     req.UILang,
		Freshness:  req.Freshness,
	}
}

func toBaseConfig(cfg *Config) *basesearch.Config {
	if cfg == nil {
		return nil
	}
	return &basesearch.Config{
		Provider:  cfg.Provider,
		Fallbacks: cfg.Fallbacks,
		Exa: basesearch.ExaConfig{
			Enabled:           cfg.Exa.Enabled,
			BaseURL:           cfg.Exa.BaseURL,
			APIKey:            cfg.Exa.APIKey,
			Type:              cfg.Exa.Type,
			Category:          cfg.Exa.Category,
			NumResults:        cfg.Exa.NumResults,
			IncludeText:       cfg.Exa.IncludeText,
			TextMaxCharacters: cfg.Exa.TextMaxCharacters,
			Highlights:        cfg.Exa.Highlights,
		},
		OpenRouter: basesearch.OpenRouterConfig{
			Enabled:      cfg.OpenRouter.Enabled,
			APIKey:       cfg.OpenRouter.APIKey,
			BaseURL:      cfg.OpenRouter.BaseURL,
			Model:        cfg.OpenRouter.Model,
			TimeoutSecs:  cfg.OpenRouter.TimeoutSecs,
			CacheTtlSecs: cfg.OpenRouter.CacheTtlSecs,
		},
	}
}

func fromBaseConfig(cfg *basesearch.Config) *Config {
	if cfg == nil {
		return nil
	}
	return &Config{
		Provider:  cfg.Provider,
		Fallbacks: cfg.Fallbacks,
		Exa: ExaConfig{
			Enabled:           cfg.Exa.Enabled,
			BaseURL:           cfg.Exa.BaseURL,
			APIKey:            cfg.Exa.APIKey,
			Type:              cfg.Exa.Type,
			Category:          cfg.Exa.Category,
			NumResults:        cfg.Exa.NumResults,
			IncludeText:       cfg.Exa.IncludeText,
			TextMaxCharacters: cfg.Exa.TextMaxCharacters,
			Highlights:        cfg.Exa.Highlights,
		},
		OpenRouter: OpenRouterConfig{
			Enabled:      cfg.OpenRouter.Enabled,
			APIKey:       cfg.OpenRouter.APIKey,
			BaseURL:      cfg.OpenRouter.BaseURL,
			Model:        cfg.OpenRouter.Model,
			TimeoutSecs:  cfg.OpenRouter.TimeoutSecs,
			CacheTtlSecs: cfg.OpenRouter.CacheTtlSecs,
		},
	}
}

func fromBaseResponse(resp basesearch.Response) Response {
	results := make([]Result, 0, len(resp.Results))
	for _, result := range resp.Results {
		results = append(results, Result{
			ID:          result.ID,
			Title:       result.Title,
			URL:         result.URL,
			Description: result.Description,
			Published:   result.Published,
			SiteName:    result.SiteName,
			Author:      result.Author,
			Image:       result.Image,
			Favicon:     result.Favicon,
		})
	}
	return Response{
		Query:      resp.Query,
		Provider:   resp.Provider,
		Count:      resp.Count,
		TookMs:     resp.TookMs,
		Results:    results,
		Answer:     resp.Answer,
		Summary:    resp.Summary,
		Definition: resp.Definition,
		Warning:    resp.Warning,
		NoResults:  resp.NoResults,
		Cached:     resp.Cached,
		Extras:     resp.Extras,
	}
}
