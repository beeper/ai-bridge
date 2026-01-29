package connector

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/openai/openai-go/v3"
)

// ContextLengthError contains parsed details from context_length_exceeded errors
type ContextLengthError struct {
	ModelMaxTokens  int
	RequestedTokens int
	OriginalError   error
}

func (e *ContextLengthError) Error() string {
	return e.OriginalError.Error()
}

// ParseContextLengthError checks if err is a context length exceeded error
// and extracts the token counts from the error message
func ParseContextLengthError(err error) *ContextLengthError {
	if err == nil {
		return nil
	}

	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return nil
	}

	// Check for context_length_exceeded error
	// OpenAI returns: "This model's maximum context length is X tokens.
	// However, your messages resulted in Y tokens."
	if apiErr.StatusCode != 400 {
		return nil
	}

	msg := apiErr.Message
	if !strings.Contains(msg, "context length") && !strings.Contains(msg, "maximum") && !strings.Contains(msg, "tokens") {
		return nil
	}

	cle := &ContextLengthError{OriginalError: err}

	// Parse token counts from message
	// Pattern: "maximum context length is X tokens"
	maxPattern := regexp.MustCompile(`maximum context length is (\d+) tokens`)
	if matches := maxPattern.FindStringSubmatch(msg); len(matches) > 1 {
		cle.ModelMaxTokens, _ = strconv.Atoi(matches[1])
	}

	// Pattern: "resulted in Y tokens" or "your messages resulted in Y tokens"
	reqPattern := regexp.MustCompile(`resulted in (\d+) tokens`)
	if matches := reqPattern.FindStringSubmatch(msg); len(matches) > 1 {
		cle.RequestedTokens, _ = strconv.Atoi(matches[1])
	}

	return cle
}

// IsRateLimitError checks if the error is a rate limit (429) error
func IsRateLimitError(err error) bool {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 429
	}
	return false
}

// IsServerError checks if the error is a server-side (5xx) error
func IsServerError(err error) bool {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode >= 500
	}
	return false
}

// IsAuthError checks if the error is an authentication error
func IsAuthError(err error) bool {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 401 || apiErr.StatusCode == 403
	}
	return false
}

// fallbackContextWindows stores fallback context window sizes for known models
// Used when runtime cache is unavailable
var fallbackContextWindows = map[string]int{
	"gpt-4o":        128000,
	"gpt-4o-mini":   128000,
	"gpt-4-turbo":   128000,
	"gpt-4":         8192,
	"gpt-4-32k":     32768,
	"gpt-3.5-turbo": 16385,
	"o1":            128000,
	"o1-mini":       128000,
	"o3-mini":       200000,
}

const defaultContextWindow = 8192

// GetModelContextWindow returns the context window size for a model
func GetModelContextWindow(modelID string) int {
	// First, try runtime cache from OpenRouter
	if size := GetOpenRouterContextWindow(modelID); size > 0 {
		return size
	}

	// Fall back to hardcoded values
	// Check exact match first
	if size, ok := fallbackContextWindows[modelID]; ok {
		return size
	}

	// Check prefix matches (for versioned models like gpt-4o-2024-05-13)
	for prefix, size := range fallbackContextWindows {
		if strings.HasPrefix(modelID, prefix) {
			return size
		}
	}

	// Default fallback for unknown models
	return defaultContextWindow
}
