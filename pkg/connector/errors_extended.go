package connector

import (
	"encoding/json"
	"strings"
)

// ProxyError represents a structured error from the hungryserv proxy
type ProxyError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   string `json:"details"`
	Provider  string `json:"provider"`
	Retryable bool   `json:"retryable"`
	Type      string `json:"type"`
	Status    int    `json:"status"`
}

// ProxyErrorResponse is the wrapper for proxy errors
type ProxyErrorResponse struct {
	Error ProxyError `json:"error"`
}

// ParseProxyError attempts to parse a structured proxy error from an error message
func ParseProxyError(err error) *ProxyError {
	if err == nil {
		return nil
	}
	msg := err.Error()

	// Try to find JSON in the error message
	startIdx := strings.Index(msg, "{")
	if startIdx == -1 {
		return nil
	}

	var resp ProxyErrorResponse
	if jsonErr := json.Unmarshal([]byte(msg[startIdx:]), &resp); jsonErr == nil {
		if resp.Error.Type == "proxy_error" {
			return &resp.Error
		}
	}

	// Try parsing directly as ProxyError
	var proxyErr ProxyError
	if jsonErr := json.Unmarshal([]byte(msg[startIdx:]), &proxyErr); jsonErr == nil {
		if proxyErr.Type == "proxy_error" {
			return &proxyErr
		}
	}

	return nil
}

// IsProxyError checks if the error is a structured proxy error
func IsProxyError(err error) bool {
	return ParseProxyError(err) != nil
}

// FormatProxyError formats a proxy error for user display
func FormatProxyError(proxyErr *ProxyError) string {
	if proxyErr == nil {
		return ""
	}

	switch proxyErr.Code {
	case "timeout", "connection_timeout":
		return "Request timed out waiting for AI provider. Please try again."
	case "connection_refused":
		return "Could not connect to AI provider. The service may be down."
	case "connection_reset", "connection_closed":
		return "Connection to AI provider was lost. Please try again."
	case "dns_error":
		return "Could not reach AI provider. Please check your connection."
	case "request_cancelled":
		return "Request was cancelled."
	default:
		if proxyErr.Message != "" {
			return proxyErr.Message
		}
		return "Failed to reach AI provider. Please try again."
	}
}

// FallbackReasoningLevel returns a lower reasoning level to try when the current one fails.
// Returns empty string if there's no fallback available (already at "none" or unknown level).
func FallbackReasoningLevel(current string) string {
	// Reasoning level hierarchy: xhigh -> high -> medium -> low -> none
	switch current {
	case "xhigh":
		return "high"
	case "high":
		return "medium"
	case "medium":
		return "low"
	case "low":
		return "none"
	case "none", "":
		return "" // No fallback available
	default:
		return "medium" // Unknown level, try medium
	}
}

// containsAnyPattern checks if the lowercased error message contains any of the given patterns.
func containsAnyPattern(err error, patterns []string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, pattern := range patterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

// IsBillingError checks if the error is a billing/payment error (402)
func IsBillingError(err error) bool {
	return containsAnyPattern(err, []string{
		"402",
		"payment required",
		"insufficient credits",
		"credit balance",
		"exceeded your current quota",
		"quota exceeded",
		"billing",
		"plans & billing",
	})
}

// IsOverloadedError checks if the error indicates the service is overloaded
func IsOverloadedError(err error) bool {
	return containsAnyPattern(err, []string{
		"overloaded_error",
		"overloaded",
		"resource_exhausted",
		"service unavailable",
		"503",
	})
}

// IsTimeoutError checks if the error is a timeout error
func IsTimeoutError(err error) bool {
	return containsAnyPattern(err, []string{
		"timeout",
		"timed out",
		"deadline exceeded",
		"context deadline exceeded",
		"etimedout",
		"esockettimedout",
		"econnreset",
		"econnaborted",
		"408",
		"504",
	})
}

// IsImageError checks if the error is related to image size or dimensions
func IsImageError(err error) bool {
	return containsAnyPattern(err, []string{
		"image exceeds",
		"image dimensions exceed",
		"image too large",
		"image size",
		"max allowed size",
	})
}

// IsReasoningError checks if the error is related to unsupported reasoning/thinking levels
func IsReasoningError(err error) bool {
	return containsAnyPattern(err, []string{
		"reasoning",
		"thinking",
		"extended thinking",
		"reasoning_effort",
	})
}

// IsRoleOrderingError checks if the error is related to message role ordering conflicts
func IsRoleOrderingError(err error) bool {
	return containsAnyPattern(err, []string{
		"incorrect role information",
		"roles must alternate",
		"consecutive user",
		"consecutive assistant",
	})
}

// FormatUserFacingError transforms an API error into a user-friendly message.
// Returns a sanitized message suitable for display to end users.
func FormatUserFacingError(err error) string {
	if err == nil {
		return "An unknown error occurred."
	}

	// Check specific error types and return user-friendly messages
	if IsBillingError(err) {
		return "Billing issue with AI provider. Please check your account credits or upgrade your plan."
	}

	if IsOverloadedError(err) {
		return "The AI service is temporarily overloaded. Please try again in a moment."
	}

	if IsRateLimitError(err) {
		return "Rate limited by AI provider. Please wait a moment before retrying."
	}

	if IsTimeoutError(err) {
		return "Request timed out. The server took too long to respond. Please try again."
	}

	if IsAuthError(err) {
		return "Authentication failed. Please check your API key or re-login."
	}

	if cle := ParseContextLengthError(err); cle != nil {
		if cle.ModelMaxTokens > 0 {
			return "Context overflow: prompt too large for the model. Try again with less input or a larger-context model."
		}
		return "Your message is too long for this model's context window. Please try a shorter message."
	}

	if IsImageError(err) {
		return "Image is too large or has invalid dimensions. Please resize the image and try again."
	}

	if IsRoleOrderingError(err) {
		return "Message ordering conflict - please try again. If this persists, start a new conversation."
	}

	if IsReasoningError(err) {
		return "This model doesn't support the requested reasoning level. Try reducing reasoning effort in settings."
	}

	if IsModelNotFound(err) {
		return "The requested model is not available. Please select a different model."
	}

	// Check for structured proxy errors (from hungryserv)
	if proxyErr := ParseProxyError(err); proxyErr != nil {
		return FormatProxyError(proxyErr)
	}

	if IsServerError(err) {
		return "The AI provider encountered an error. Please try again later."
	}

	// For unknown errors, try to extract a sensible message
	msg := err.Error()

	// Strip common error prefixes
	prefixes := []string{
		"error:",
		"api error:",
		"openai error:",
		"anthropic error:",
	}
	lower := strings.ToLower(msg)
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			msg = strings.TrimSpace(msg[len(prefix):])
			break
		}
	}

	// Truncate very long error messages
	if len(msg) > 200 {
		msg = msg[:200] + "..."
	}

	// If the message looks like raw JSON, return a generic message
	if strings.HasPrefix(strings.TrimSpace(msg), "{") {
		return "The AI provider returned an error. Please try again."
	}

	return msg
}
