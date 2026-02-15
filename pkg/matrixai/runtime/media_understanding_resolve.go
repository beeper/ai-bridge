package runtime

import (
	"time"

	"github.com/beeper/ai-bridge/pkg/core/aimedia"
)

func resolveMediaTimeoutSeconds(value int, cfg *MediaUnderstandingConfig, fallback int) time.Duration {
	return aimedia.ResolveTimeout(value, toCoreMediaConfig(cfg), fallback)
}

func resolveMediaPrompt(capability MediaUnderstandingCapability, entryPrompt string, cfg *MediaUnderstandingConfig, maxChars int) string {
	return aimedia.ResolvePrompt(toCoreMediaCapability(capability), entryPrompt, toCoreMediaConfig(cfg), maxChars)
}

func resolveMediaMaxChars(capability MediaUnderstandingCapability, entry MediaUnderstandingModelConfig, cfg *MediaUnderstandingConfig) int {
	return aimedia.ResolveMaxChars(toCoreMediaCapability(capability), toCoreMediaModelConfig(entry), toCoreMediaConfig(cfg))
}

func resolveMediaMaxBytes(capability MediaUnderstandingCapability, entry MediaUnderstandingModelConfig, cfg *MediaUnderstandingConfig) int {
	return aimedia.ResolveMaxBytes(toCoreMediaCapability(capability), toCoreMediaModelConfig(entry), toCoreMediaConfig(cfg))
}

func resolveMediaLanguage(entry MediaUnderstandingModelConfig, cfg *MediaUnderstandingConfig) string {
	return aimedia.ResolveLanguage(toCoreMediaModelConfig(entry), toCoreMediaConfig(cfg))
}

func resolveMediaEntries(cfg *MediaToolsConfig, capCfg *MediaUnderstandingConfig, capability MediaUnderstandingCapability) []MediaUnderstandingModelConfig {
	coreEntries := aimedia.ResolveEntries(toCoreToolsConfig(cfg), toCoreMediaConfig(capCfg), toCoreMediaCapability(capability))
	out := make([]MediaUnderstandingModelConfig, 0, len(coreEntries))
	for _, entry := range coreEntries {
		out = append(out, fromCoreMediaModelConfig(entry))
	}
	return out
}

func capabilityInList(capability MediaUnderstandingCapability, values []string) bool {
	return aimedia.CapabilityInList(toCoreMediaCapability(capability), values)
}

func capabilityInCapabilities(capability MediaUnderstandingCapability, values []MediaUnderstandingCapability) bool {
	for _, value := range values {
		if value == capability {
			return true
		}
	}
	return false
}

func providerSupportsCapability(providerID string, capability MediaUnderstandingCapability) bool {
	return aimedia.ProviderSupportsCapability(providerID, toCoreMediaCapability(capability))
}
