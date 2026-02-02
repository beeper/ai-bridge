package connector

import "maunium.net/go/mautrix/event"

func messageStatusForError(err error) event.MessageStatus {
	if !IsPreDeltaError(err) {
		return event.MessageStatusRetriable
	}
	if IsRateLimitError(err) || IsOverloadedError(err) || IsTimeoutError(err) || IsServerError(err) {
		return event.MessageStatusRetriable
	}
	return event.MessageStatusFail
}

func messageStatusReasonForError(err error) event.MessageStatusReason {
	switch {
	case IsAuthError(err), IsBillingError(err):
		return event.MessageStatusNoPermission
	case IsModelNotFound(err):
		return event.MessageStatusUnsupported
	case ParseContextLengthError(err) != nil, IsImageError(err):
		return event.MessageStatusUnsupported
	case IsRateLimitError(err), IsOverloadedError(err), IsTimeoutError(err), IsServerError(err):
		return event.MessageStatusNetworkError
	default:
		return event.MessageStatusGenericError
	}
}
