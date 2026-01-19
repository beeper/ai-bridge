package connector

import (
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// ModelBackend identifies which backend to use for a model
// All backends use the OpenAI SDK with different base URLs
type ModelBackend string

const (
	BackendOpenAI     ModelBackend = "openai"
	BackendOpenRouter ModelBackend = "openrouter"
)

// Default models for each provider (with prefixes)
const (
	DefaultModelOpenAI     = "openai/gpt-4o-mini"
	DefaultModelOpenRouter = "openrouter/openai/gpt-4o-mini"
	DefaultModelBeeper     = "openai/gpt-5.2"
)

// ParseModelPrefix extracts the backend and actual model ID from a prefixed model
// Examples:
//   - "openai/gpt-5.2" → (BackendOpenAI, "gpt-5.2")
//   - "openrouter/openai/gpt-5" → (BackendOpenRouter, "openai/gpt-5")
//   - "gpt-4o" (no prefix) → ("", "gpt-4o")
func ParseModelPrefix(modelID string) (backend ModelBackend, actualModel string) {
	parts := strings.SplitN(modelID, "/", 2)
	if len(parts) != 2 {
		return "", modelID // No prefix, return as-is
	}

	switch parts[0] {
	case "openai":
		return BackendOpenAI, parts[1]
	case "openrouter":
		return BackendOpenRouter, parts[1] // parts[1] = "openai/gpt-5" (nested)
	default:
		return "", modelID // Unknown prefix, return as-is
	}
}

// HasValidPrefix checks if a model ID has a valid backend prefix
func HasValidPrefix(modelID string) bool {
	backend, _ := ParseModelPrefix(modelID)
	return backend != ""
}

// GetModelPrefix returns just the prefix portion of a model ID
func GetModelPrefix(modelID string) string {
	parts := strings.SplitN(modelID, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}

// AddModelPrefix adds a prefix to a model ID if it doesn't have one
func AddModelPrefix(backend ModelBackend, modelID string) string {
	if HasValidPrefix(modelID) {
		return modelID
	}
	return string(backend) + "/" + modelID
}

// DefaultModelForProvider returns the default model for a given provider
func DefaultModelForProvider(provider string) string {
	switch provider {
	case ProviderOpenAI:
		return DefaultModelOpenAI
	case ProviderOpenRouter:
		return DefaultModelOpenRouter
	case ProviderBeeper:
		return DefaultModelBeeper
	default:
		return DefaultModelOpenAI
	}
}

// ValidateModelForProvider checks if a model can be used with a given provider
// Returns an error if the model's backend doesn't match the provider
func ValidateModelForProvider(modelID, provider string) error {
	backend, _ := ParseModelPrefix(modelID)

	// No prefix - legacy model, needs to be updated
	if backend == "" {
		return fmt.Errorf("model %q is missing backend prefix (use %s/%s)", modelID, inferBackendFromModel(modelID), modelID)
	}

	switch provider {
	case ProviderBeeper:
		// Beeper supports all backends via OpenRouter
		return nil
	case ProviderOpenAI:
		if backend != BackendOpenAI {
			return fmt.Errorf("OpenAI provider only supports openai/* models, got %q", modelID)
		}
	case ProviderOpenRouter:
		if backend != BackendOpenRouter {
			return fmt.Errorf("OpenRouter provider only supports openrouter/* models, got %q", modelID)
		}
	case ProviderCustom:
		// Custom provider accepts any model (relies on user's endpoint)
		return nil
	}

	return nil
}

// inferBackendFromModel tries to guess the backend from a legacy (unprefixed) model name
func inferBackendFromModel(_ string) string {
	// Default to openai - all providers use OpenAI SDK
	return "openai"
}

// BackendForProvider returns the expected backend for a provider
func BackendForProvider(provider string) ModelBackend {
	switch provider {
	case ProviderOpenAI:
		return BackendOpenAI
	case ProviderOpenRouter:
		return BackendOpenRouter
	default:
		return ""
	}
}

// FormatModelDisplay formats a prefixed model ID for display
// "openai/gpt-4o" → "GPT 4o"
// "gemini/gemini-2.5-flash" → "Gemini 2.5 Flash"
// "anthropic/claude-sonnet-4-5" → "Claude Sonnet 4.5"
func FormatModelDisplay(modelID string) string {
	_, actualModel := ParseModelPrefix(modelID)
	return formatModelDisplayName(actualModel)
}

// formatModelDisplayName formats a model name for display
func formatModelDisplayName(model string) string {
	// Handle common model naming patterns
	model = strings.ReplaceAll(model, "-", " ")
	model = strings.ReplaceAll(model, "_", " ")

	caser := cases.Title(language.English)

	// Capitalize words
	words := strings.Fields(model)
	for i, word := range words {
		// Special cases for model names
		switch strings.ToLower(word) {
		case "gpt":
			words[i] = "GPT"
		case "o1", "o3":
			words[i] = strings.ToUpper(word)
		case "claude":
			words[i] = "Claude"
		case "gemini":
			words[i] = "Gemini"
		case "pro", "mini", "flash", "opus", "sonnet", "haiku":
			words[i] = caser.String(word)
		default:
			// Keep version numbers and other short words as-is
			if len(word) <= 3 {
				words[i] = word
			} else {
				words[i] = caser.String(word)
			}
		}
	}

	return strings.Join(words, " ")
}
