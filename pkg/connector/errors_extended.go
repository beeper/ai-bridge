package connector

import (
	"encoding/json"
	"strings"

	"maunium.net/go/mautrix/bridgev2/status"
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

// Extended bridge state error codes for AI-specific errors
const (
	AIBillingError status.BridgeStateErrorCode = "ai-billing-error"
	AIOverloaded   status.BridgeStateErrorCode = "ai-overloaded"
	AITimeout      status.BridgeStateErrorCode = "ai-timeout"
	AIImageError   status.BridgeStateErrorCode = "ai-image-error"
)

func init() {
	// Add extended error messages to the main map
	BridgeStateHumanErrors[AIBillingError] = "Billing issue with AI provider. Please check your account credits."
	BridgeStateHumanErrors[AIOverloaded] = "The AI service is temporarily overloaded. Please try again in a moment."
	BridgeStateHumanErrors[AITimeout] = "Request timed out. Please try again."
	BridgeStateHumanErrors[AIImageError] = "Image is too large or has invalid dimensions for this model."
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

// IsBillingError checks if the error is a billing/payment error (402)
func IsBillingError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	// Check for 402 status code pattern
	if strings.Contains(msg, "402") {
		return true
	}
	// Check error message for billing-related keywords
	billingPatterns := []string{
		"payment required",
		"insufficient credits",
		"credit balance",
		"exceeded your current quota",
		"quota exceeded",
		"billing",
		"plans & billing",
	}
	for _, pattern := range billingPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

// IsOverloadedError checks if the error indicates the service is overloaded
func IsOverloadedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	overloadedPatterns := []string{
		"overloaded_error",
		"overloaded",
		"resource_exhausted",
		"service unavailable",
		"503",
	}
	for _, pattern := range overloadedPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

// IsTimeoutError checks if the error is a timeout error
func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	timeoutPatterns := []string{
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
	}
	for _, pattern := range timeoutPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

// IsImageError checks if the error is related to image size or dimensions
func IsImageError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	imagePatterns := []string{
		"image exceeds",
		"image dimensions exceed",
		"image too large",
		"image size",
		"max allowed size",
	}
	for _, pattern := range imagePatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

// IsReasoningError checks if the error is related to unsupported reasoning/thinking levels
func IsReasoningError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	reasoningPatterns := []string{
		"reasoning",
		"thinking",
		"extended thinking",
		"reasoning_effort",
	}
	for _, pattern := range reasoningPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

// IsRoleOrderingError checks if the error is related to message role ordering conflicts
func IsRoleOrderingError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	rolePatterns := []string{
		"incorrect role information",
		"roles must alternate",
		"consecutive user",
		"consecutive assistant",
	}
	for _, pattern := range rolePatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
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

// MapErrorToStateCodeExtended extends MapErrorToStateCode with new error types.
// Call this instead of MapErrorToStateCode for full error classification.
func MapErrorToStateCodeExtended(err error) status.BridgeStateErrorCode {
	if err == nil {
		return ""
	}
	// Check extended error types first
	if IsBillingError(err) {
		return AIBillingError
	}
	if IsOverloadedError(err) {
		return AIOverloaded
	}
	if IsTimeoutError(err) {
		return AITimeout
	}
	if IsImageError(err) {
		return AIImageError
	}
	// Fall back to original classification
	return MapErrorToStateCode(err)
}
