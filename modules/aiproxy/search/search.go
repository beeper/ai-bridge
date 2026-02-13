package search

import (
	"context"

	basesearch "github.com/beeper/ai-bridge/pkg/search"
)

type Request = basesearch.Request
type Result = basesearch.Result
type Response = basesearch.Response
type Config = basesearch.Config
type ExaConfig = basesearch.ExaConfig
type BraveConfig = basesearch.BraveConfig
type PerplexityConfig = basesearch.PerplexityConfig
type OpenRouterConfig = basesearch.OpenRouterConfig

const (
	ProviderExa        = basesearch.ProviderExa
	ProviderBrave      = basesearch.ProviderBrave
	ProviderPerplexity = basesearch.ProviderPerplexity
	ProviderOpenRouter = basesearch.ProviderOpenRouter

	DefaultSearchCount  = basesearch.DefaultSearchCount
	MaxSearchCount      = basesearch.MaxSearchCount
	DefaultTimeoutSecs  = basesearch.DefaultTimeoutSecs
	DefaultCacheTtlSecs = basesearch.DefaultCacheTtlSecs
)

var DefaultFallbackOrder = basesearch.DefaultFallbackOrder

func Search(ctx context.Context, req Request, cfg *Config) (*Response, error) {
	return basesearch.Search(ctx, req, cfg)
}

func ApplyEnvDefaults(cfg *Config) *Config {
	return basesearch.ApplyEnvDefaults(cfg)
}
