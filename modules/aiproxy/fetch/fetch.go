package fetch

import (
	"context"

	basefetch "github.com/beeper/ai-bridge/pkg/matrixai/fetch"
)

type Request = basefetch.Request
type Response = basefetch.Response
type Config = basefetch.Config
type ExaConfig = basefetch.ExaConfig
type DirectConfig = basefetch.DirectConfig

const (
	ProviderExa        = basefetch.ProviderExa
	ProviderDirect     = basefetch.ProviderDirect
	DefaultTimeoutSecs = basefetch.DefaultTimeoutSecs
	DefaultMaxChars    = basefetch.DefaultMaxChars
)

var DefaultFallbackOrder = basefetch.DefaultFallbackOrder

func Fetch(ctx context.Context, req Request, cfg *Config) (*Response, error) {
	return basefetch.Fetch(ctx, req, cfg)
}

func ApplyEnvDefaults(cfg *Config) *Config {
	return basefetch.ApplyEnvDefaults(cfg)
}
