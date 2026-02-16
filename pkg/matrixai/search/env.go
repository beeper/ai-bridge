package search

import (
	"os"
	"strings"

	"github.com/beeper/ai-bridge/pkg/core/shared/stringutil"
)

// ConfigFromEnv builds a search config using environment variables.
func ConfigFromEnv() *Config {
	cfg := &Config{}

	if provider := strings.TrimSpace(os.Getenv("SEARCH_PROVIDER")); provider != "" {
		cfg.Provider = provider
	}
	if fallbacks := strings.TrimSpace(os.Getenv("SEARCH_FALLBACKS")); fallbacks != "" {
		cfg.Fallbacks = stringutil.SplitCSV(fallbacks)
	}
	cfg.Exa.APIKey = stringutil.EnvOr(cfg.Exa.APIKey, os.Getenv("EXA_API_KEY"))
	cfg.Exa.BaseURL = stringutil.EnvOr(cfg.Exa.BaseURL, os.Getenv("EXA_BASE_URL"))

	cfg.OpenRouter.APIKey = stringutil.EnvOr(cfg.OpenRouter.APIKey, os.Getenv("OPENROUTER_API_KEY"))
	cfg.OpenRouter.BaseURL = stringutil.EnvOr(cfg.OpenRouter.BaseURL, os.Getenv("OPENROUTER_BASE_URL"))
	cfg.OpenRouter.Model = stringutil.EnvOr(cfg.OpenRouter.Model, os.Getenv("OPENROUTER_MODEL"))

	return cfg.WithDefaults()
}

// ApplyEnvDefaults fills empty config fields from environment variables.
func ApplyEnvDefaults(cfg *Config) *Config {
	if cfg == nil {
		return ConfigFromEnv()
	}
	providerSet := strings.TrimSpace(cfg.Provider) != ""
	current := cfg.WithDefaults()
	envCfg := ConfigFromEnv()

	if strings.TrimSpace(current.Provider) == "" {
		current.Provider = envCfg.Provider
	}
	if len(current.Fallbacks) == 0 {
		current.Fallbacks = envCfg.Fallbacks
	}

	if current.Exa.APIKey == "" {
		current.Exa.APIKey = envCfg.Exa.APIKey
	}
	if current.Exa.BaseURL == "" {
		current.Exa.BaseURL = envCfg.Exa.BaseURL
	}

	if current.OpenRouter.APIKey == "" {
		current.OpenRouter.APIKey = envCfg.OpenRouter.APIKey
	}
	if current.OpenRouter.BaseURL == "" {
		current.OpenRouter.BaseURL = envCfg.OpenRouter.BaseURL
	}
	if current.OpenRouter.Model == "" {
		current.OpenRouter.Model = envCfg.OpenRouter.Model
	}

	if !providerSet && strings.TrimSpace(current.Exa.APIKey) != "" {
		current.Provider = ProviderExa
	}

	return current
}
