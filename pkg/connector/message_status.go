package connector

import (
	"maunium.net/go/mautrix/event"

	airuntime "github.com/beeper/ai-bridge/pkg/runtime"
)

func messageStatusForError(err error) event.MessageStatus {
	switch {
	case IsAuthError(err),
		IsBillingError(err),
		IsModelNotFound(err),
		ParseContextLengthError(err) != nil,
		IsImageError(err):
		return event.MessageStatusFail
	default:
		return event.MessageStatusRetriable
	}
}

func messageStatusReasonForError(err error) event.MessageStatusReason {
	// Use the runtime classifier first; fall back to per-error checks
	// for errors that DecideFallback does not yet classify.
	switch airuntime.DecideFallback(err).Class {
	case airuntime.FailureClassAuth:
		return event.MessageStatusNoPermission
	case airuntime.FailureClassRateLimit, airuntime.FailureClassTimeout, airuntime.FailureClassNetwork:
		return event.MessageStatusNetworkError
	case airuntime.FailureClassContextOverflow:
		return event.MessageStatusUnsupported
	default:
		return messageStatusReasonFallback(err)
	}
}

// messageStatusReasonFallback handles errors that the runtime classifier does not cover.
func messageStatusReasonFallback(err error) event.MessageStatusReason {
	switch {
	case IsAuthError(err), IsBillingError(err):
		return event.MessageStatusNoPermission
	case IsModelNotFound(err), ParseContextLengthError(err) != nil, IsImageError(err):
		return event.MessageStatusUnsupported
	case IsRateLimitError(err), IsOverloadedError(err), IsTimeoutError(err), IsServerError(err):
		return event.MessageStatusNetworkError
	default:
		return event.MessageStatusGenericError
	}
}
