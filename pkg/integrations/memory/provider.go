package memory

import (
	"errors"
	"fmt"
	"strings"

	memorycore "github.com/beeper/ai-bridge/pkg/memory"
	"github.com/beeper/ai-bridge/pkg/memory/embedding"
)

type memoryProviderResult struct {
	Provider    memorycore.EmbeddingProvider
	Status      memorycore.ProviderStatus
	ProviderKey string
	BaseURL     string
	Headers     map[string]string
}

type providerConfig struct {
	provider memorycore.EmbeddingProvider
	status   memorycore.ProviderStatus
	baseURL  string
	headers  map[string]string
}

func buildMemoryProvider(runtime Runtime, cfg *memorycore.ResolvedConfig) (*memoryProviderResult, error) {
	if runtime == nil || cfg == nil {
		return nil, errors.New("memory provider requires runtime and config")
	}
	requested := strings.TrimSpace(cfg.Provider)
	if requested == "" {
		requested = "auto"
	}
	fallback := strings.TrimSpace(cfg.Fallback)
	if fallback == "" {
		fallback = "none"
	}

	createProvider := func(kind string) (*providerConfig, error) {
		switch kind {
		case "gemini":
			apiKey, baseURL, headers := runtime.ResolveGeminiEmbeddingConfig(cfg)
			provider, err := embedding.NewGeminiProvider(apiKey, baseURL, cfg.Model, headers)
			if err != nil {
				return nil, err
			}
			return &providerConfig{
				provider: provider,
				status: memorycore.ProviderStatus{
					Provider: provider.ID(),
					Model:    provider.Model(),
				},
				baseURL: baseURL,
				headers: headers,
			}, nil
		case "openai":
			apiKey, baseURL, headers := runtime.ResolveOpenAIEmbeddingConfig(cfg)
			provider, err := embedding.NewOpenAIProvider(apiKey, baseURL, cfg.Model, headers)
			if err != nil {
				return nil, err
			}
			return &providerConfig{
				provider: provider,
				status: memorycore.ProviderStatus{
					Provider: provider.ID(),
					Model:    provider.Model(),
				},
				baseURL: baseURL,
				headers: headers,
			}, nil
		default:
			return nil, fmt.Errorf("unsupported embeddings provider: %s", kind)
		}
	}

	if requested == "auto" {
		if hasOpenAIEmbeddingConfig(runtime, cfg) {
			if provider, err := createProvider("openai"); err == nil {
				return finalizeProvider(provider), nil
			}
		}
		if hasGeminiEmbeddingConfig(runtime, cfg) {
			if provider, err := createProvider("gemini"); err == nil {
				return finalizeProvider(provider), nil
			}
		}
		return nil, errors.New("no embeddings provider available")
	}

	primary, err := createProvider(requested)
	if err == nil {
		return finalizeProvider(primary), nil
	}

	if fallback != "" && fallback != "none" && fallback != requested {
		fallbackProvider, fallbackErr := createProvider(fallback)
		if fallbackErr == nil {
			fallbackProvider.status.Fallback = &memorycore.FallbackStatus{
				From:   requested,
				Reason: err.Error(),
			}
			return finalizeProvider(fallbackProvider), nil
		}
		return nil, fmt.Errorf("%v; fallback to %s failed: %v", err, fallback, fallbackErr)
	}
	return nil, err
}

func finalizeProvider(provider *providerConfig) *memoryProviderResult {
	providerKey := memorycore.ComputeProviderKey(
		provider.provider.ID(),
		provider.provider.Model(),
		provider.baseURL,
		provider.headers,
	)
	return &memoryProviderResult{
		Provider:    provider.provider,
		Status:      provider.status,
		ProviderKey: providerKey,
		BaseURL:     provider.baseURL,
		Headers:     provider.headers,
	}
}

func hasOpenAIEmbeddingConfig(runtime Runtime, cfg *memorycore.ResolvedConfig) bool {
	apiKey, _, _ := runtime.ResolveOpenAIEmbeddingConfig(cfg)
	return strings.TrimSpace(apiKey) != ""
}

func hasGeminiEmbeddingConfig(runtime Runtime, cfg *memorycore.ResolvedConfig) bool {
	apiKey, _, _ := runtime.ResolveGeminiEmbeddingConfig(cfg)
	return strings.TrimSpace(apiKey) != ""
}
