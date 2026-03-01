package runtime

import "strings"

func ClassifyFallbackError(err error) FailureClass {
	if err == nil {
		return FailureClassUnknown
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(text, "api key"), strings.Contains(text, "invalid_api_key"),
		strings.Contains(text, "authentication"), strings.Contains(text, "unauthorized"),
		strings.Contains(text, "forbidden"), strings.Contains(text, "permission"):
		return FailureClassAuth
	case strings.Contains(text, "context") && strings.Contains(text, "length"):
		return FailureClassContextOverflow
	case strings.Contains(text, "rate") && strings.Contains(text, "limit"),
		strings.Contains(text, "429"), strings.Contains(text, "resource_exhausted"),
		strings.Contains(text, "quota"), strings.Contains(text, "overloaded"),
		strings.Contains(text, "too many requests"):
		return FailureClassRateLimit
	case strings.Contains(text, "timeout"), strings.Contains(text, "deadline exceeded"),
		strings.Contains(text, "timed out"), strings.Contains(text, "request_timeout"):
		return FailureClassTimeout
	case strings.Contains(text, "connection"), strings.Contains(text, "network"),
		strings.Contains(text, "connection reset"), strings.Contains(text, "econnreset"):
		return FailureClassNetwork
	case strings.Contains(text, "provider"), strings.Contains(text, "model"),
		strings.Contains(text, "not found"), strings.Contains(text, "404"),
		strings.Contains(text, "payment"), strings.Contains(text, "billing"),
		strings.Contains(text, "402"), strings.Contains(text, "500"),
		strings.Contains(text, "502"), strings.Contains(text, "503"), strings.Contains(text, "504"):
		return FailureClassProviderHard
	default:
		return FailureClassUnknown
	}
}

func ShouldTriggerFallback(class FailureClass) bool {
	switch class {
	case FailureClassAuth, FailureClassRateLimit, FailureClassTimeout, FailureClassNetwork, FailureClassContextOverflow, FailureClassProviderHard:
		return true
	default:
		return false
	}
}
