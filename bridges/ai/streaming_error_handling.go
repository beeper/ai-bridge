package ai

import (
	"context"
	"errors"
	"time"

	"maunium.net/go/mautrix/bridgev2"
)

// NonFallbackError marks an error as ineligible for fallback retries once output has been sent.
type NonFallbackError struct {
	Err error
}

func (e *NonFallbackError) Error() string {
	return e.Err.Error()
}

func (e *NonFallbackError) Unwrap() error {
	return e.Err
}

func streamFailureError(state *streamingState, err error) error {
	if state != nil && state.hasEditTarget() {
		return &NonFallbackError{Err: err}
	}
	return &PreDeltaError{Err: err}
}

func (oc *AIClient) handleResponsesStreamErr(
	ctx context.Context,
	portal *bridgev2.Portal,
	state *streamingState,
	meta *PortalMetadata,
	err error,
	includeContextLength bool,
) (*ContextLengthError, error) {
	if errors.Is(err, context.Canceled) {
		state.finishReason = "cancelled"
		state.completedAtMs = time.Now().UnixMilli()
		oc.uiEmitter(state).EmitUIAbort(context.Background(), portal, "cancelled")
		oc.emitUIFinish(context.Background(), portal, state, meta)
		oc.persistTerminalAssistantTurn(context.Background(), *oc.loggerForContext(ctx), portal, state, meta)
		return nil, streamFailureError(state, err)
	}

	if includeContextLength {
		cle := ParseContextLengthError(err)
		if cle != nil {
			return cle, nil
		}
	}

	state.finishReason = "error"
	state.completedAtMs = time.Now().UnixMilli()
	oc.uiEmitter(state).EmitUIError(ctx, portal, err.Error())
	oc.emitUIFinish(ctx, portal, state, meta)
	oc.persistTerminalAssistantTurn(ctx, *oc.loggerForContext(ctx), portal, state, meta)
	return nil, streamFailureError(state, err)
}
