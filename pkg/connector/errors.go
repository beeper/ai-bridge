package connector

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/openai/openai-go/v3"
	"maunium.net/go/mautrix/bridgev2/status"
)

// Bridge state error codes for AI-specific errors
const (
	AIRateLimited    status.BridgeStateErrorCode = "ai-rate-limited"
	AIAuthFailed     status.BridgeStateErrorCode = "ai-auth-failed"
	AIContextTooLong status.BridgeStateErrorCode = "ai-context-too-long"
	AIModelNotFound  status.BridgeStateErrorCode = "ai-model-not-found"
	AIProviderError  status.BridgeStateErrorCode = "ai-provider-error"
)

// BridgeStateHumanErrors provides human-readable messages for AI bridge error codes
var BridgeStateHumanErrors = map[status.BridgeStateErrorCode]string{
	AIRateLimited:    "Rate limited by AI provider. Please wait before retrying.",
	AIAuthFailed:     "API key is invalid or has expired.",
	AIContextTooLong: "Conversation is too long for this model's context window.",
	AIModelNotFound:  "The requested model is not available.",
	AIProviderError:  "The AI provider returned an error.",
}

// MapErrorToStateCode maps an API error to a bridge state error code.
// Returns empty string if the error doesn't map to a known state code.
func MapErrorToStateCode(err error) status.BridgeStateErrorCode {
	if err == nil {
		return ""
	}
	if IsRateLimitError(err) {
		return AIRateLimited
	}
	if IsAuthError(err) {
		return AIAuthFailed
	}
	if ParseContextLengthError(err) != nil {
		return AIContextTooLong
	}
	if IsModelNotFound(err) {
		return AIModelNotFound
	}
	if IsServerError(err) {
		return AIProviderError
	}
	return ""
}

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

// IsModelNotFound checks if the error is a model not found (404) error
func IsModelNotFound(err error) bool {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 404
	}
	return false
}
