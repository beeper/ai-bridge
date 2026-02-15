package aimedia

import (
	"strconv"
	"strings"
	"time"
)

// ResolveTimeout picks the most specific timeout setting.
func ResolveTimeout(value int, cfg *CapabilityConfig, fallback int) time.Duration {
	seconds := value
	if seconds <= 0 && cfg != nil && cfg.TimeoutSeconds > 0 {
		seconds = cfg.TimeoutSeconds
	}
	if seconds <= 0 {
		seconds = fallback
	}
	if seconds <= 0 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

// ResolvePrompt picks the most specific prompt, appending a char limit hint
// for non-audio capabilities.
func ResolvePrompt(
	capability MediaUnderstandingCapability,
	entryPrompt string,
	cfg *CapabilityConfig,
	maxChars int,
) string {
	base := strings.TrimSpace(entryPrompt)
	if base == "" && cfg != nil {
		base = strings.TrimSpace(cfg.Prompt)
	}
	if base == "" {
		base = DefaultPromptByCapability[capability]
	}
	if maxChars <= 0 || capability == MediaCapabilityAudio {
		return base
	}
	return base + " Respond in at most " + strconv.Itoa(maxChars) + " characters."
}

// ResolveMaxChars picks the most specific maxChars setting.
func ResolveMaxChars(capability MediaUnderstandingCapability, entry ModelConfig, cfg *CapabilityConfig) int {
	if entry.MaxChars > 0 {
		return entry.MaxChars
	}
	if cfg != nil && cfg.MaxChars > 0 {
		return cfg.MaxChars
	}
	return DefaultMaxCharsByCapability[capability]
}

// ResolveMaxBytes picks the most specific maxBytes setting.
func ResolveMaxBytes(capability MediaUnderstandingCapability, entry ModelConfig, cfg *CapabilityConfig) int {
	if entry.MaxBytes > 0 {
		return entry.MaxBytes
	}
	if cfg != nil && cfg.MaxBytes > 0 {
		return cfg.MaxBytes
	}
	return DefaultMaxBytesByCapability[capability]
}

// ResolveLanguage picks the most specific language setting.
func ResolveLanguage(entry ModelConfig, cfg *CapabilityConfig) string {
	if strings.TrimSpace(entry.Language) != "" {
		return strings.TrimSpace(entry.Language)
	}
	if cfg != nil && strings.TrimSpace(cfg.Language) != "" {
		return strings.TrimSpace(cfg.Language)
	}
	return ""
}

// ResolveEntries filters model config entries for a given capability.
func ResolveEntries(cfg *ToolsConfig, capCfg *CapabilityConfig, capability MediaUnderstandingCapability) []ModelConfig {
	type entryWithSource struct {
		entry  ModelConfig
		source string
	}
	var entries []entryWithSource
	if capCfg != nil {
		for _, entry := range capCfg.Models {
			entries = append(entries, entryWithSource{entry: entry, source: "capability"})
		}
	}
	if cfg != nil {
		for _, entry := range cfg.Models {
			entries = append(entries, entryWithSource{entry: entry, source: "shared"})
		}
	}
	if len(entries) == 0 {
		return nil
	}

	filtered := make([]ModelConfig, 0, len(entries))
	for _, item := range entries {
		entry := item.entry
		if len(entry.Capabilities) > 0 {
			if !CapabilityInList(capability, entry.Capabilities) {
				continue
			}
			filtered = append(filtered, entry)
			continue
		}
		if item.source == "shared" {
			provider := NormalizeProviderID(entry.Provider)
			if provider == "" {
				continue
			}
			if caps, ok := ProviderCapabilities[provider]; ok && CapabilityInCapabilities(capability, caps) {
				filtered = append(filtered, entry)
			}
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

// CapabilityInList checks if a capability string appears in a list of strings.
func CapabilityInList(capability MediaUnderstandingCapability, list []string) bool {
	for _, entry := range list {
		if strings.TrimSpace(strings.ToLower(entry)) == string(capability) {
			return true
		}
	}
	return false
}

// CapabilityInCapabilities checks if a capability is in a typed slice.
func CapabilityInCapabilities(capability MediaUnderstandingCapability, list []MediaUnderstandingCapability) bool {
	for _, entry := range list {
		if entry == capability {
			return true
		}
	}
	return false
}

// ResolveBaseURL picks the most specific base URL.
func ResolveBaseURL(cfg *CapabilityConfig, entry ModelConfig) string {
	if strings.TrimSpace(entry.BaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(entry.BaseURL), "/")
	}
	if cfg != nil && strings.TrimSpace(cfg.BaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	}
	return ""
}

// MergeHeaders merges capability-level and entry-level headers.
func MergeHeaders(cfg *CapabilityConfig, entry ModelConfig) map[string]string {
	merged := map[string]string{}
	if cfg != nil {
		for k, v := range cfg.Headers {
			merged[k] = v
		}
	}
	for k, v := range entry.Headers {
		merged[k] = v
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

// ProviderSupportsCapability checks if a provider supports a given capability.
func ProviderSupportsCapability(providerID string, capability MediaUnderstandingCapability) bool {
	caps, ok := ProviderCapabilities[NormalizeProviderID(providerID)]
	if !ok {
		return false
	}
	return CapabilityInCapabilities(capability, caps)
}
