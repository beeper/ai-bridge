package fetch

import (
	"os"
	"strings"
)

// ConfigFromEnv builds a fetch config using environment variables.
func ConfigFromEnv() *Config {
	cfg := (&Config{}).WithDefaults()

	if provider := strings.TrimSpace(os.Getenv("FETCH_PROVIDER")); provider != "" {
		cfg.Provider = provider
	}
	if fallbacks := strings.TrimSpace(os.Getenv("FETCH_FALLBACKS")); fallbacks != "" {
		cfg.Fallbacks = splitCSV(fallbacks)
	}

	cfg.Proxy.BaseURL = envOr(cfg.Proxy.BaseURL, os.Getenv("HUNGRYSERV_PROXY_BASE_URL"))
	cfg.Proxy.APIKey = envOr(cfg.Proxy.APIKey, os.Getenv("HUNGRYSERV_PROXY_API_KEY"))

	cfg.Exa.APIKey = envOr(cfg.Exa.APIKey, os.Getenv("EXA_API_KEY"))
	cfg.Exa.BaseURL = envOr(cfg.Exa.BaseURL, os.Getenv("EXA_BASE_URL"))

	return cfg
}

// ApplyEnvDefaults fills empty config fields from environment variables.
func ApplyEnvDefaults(cfg *Config) *Config {
	if cfg == nil {
		return ConfigFromEnv()
	}
	current := cfg.WithDefaults()
	envCfg := ConfigFromEnv()

	if strings.TrimSpace(current.Provider) == "" {
		current.Provider = envCfg.Provider
	}
	if len(current.Fallbacks) == 0 {
		current.Fallbacks = envCfg.Fallbacks
	}

	if current.Proxy.BaseURL == "" {
		current.Proxy.BaseURL = envCfg.Proxy.BaseURL
	}
	if current.Proxy.APIKey == "" {
		current.Proxy.APIKey = envCfg.Proxy.APIKey
	}

	if current.Exa.APIKey == "" {
		current.Exa.APIKey = envCfg.Exa.APIKey
	}
	if current.Exa.BaseURL == "" {
		current.Exa.BaseURL = envCfg.Exa.BaseURL
	}

	return current
}

func envOr(existing, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return existing
	}
	return value
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	var out []string
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
