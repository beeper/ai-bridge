package connector

import (
	"github.com/beeper/ai-bridge/pkg/aierrors"
	"maunium.net/go/mautrix/event"
)

func messageStatusForError(err error) event.MessageStatus {
	switch {
	case aierrors.IsAuthError(err),
		aierrors.IsBillingError(err),
		aierrors.IsModelNotFound(err),
		aierrors.ParseContextLengthError(err) != nil,
		aierrors.IsImageError(err):
		return event.MessageStatusFail
	default:
		return event.MessageStatusRetriable
	}
}

func messageStatusReasonForError(err error) event.MessageStatusReason {
	switch {
	case aierrors.IsAuthError(err), aierrors.IsBillingError(err):
		return event.MessageStatusNoPermission
	case aierrors.IsModelNotFound(err):
		return event.MessageStatusUnsupported
	case aierrors.ParseContextLengthError(err) != nil, aierrors.IsImageError(err):
		return event.MessageStatusUnsupported
	case aierrors.IsRateLimitError(err), aierrors.IsOverloadedError(err), aierrors.IsTimeoutError(err), aierrors.IsServerError(err):
		return event.MessageStatusNetworkError
	default:
		return event.MessageStatusGenericError
	}
}
