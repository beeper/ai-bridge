package connector

import (
	"context"
	"strings"

	"github.com/openai/openai-go/v3/responses"
	"maunium.net/go/mautrix/bridgev2"
)

// captureResponseID stores the response ID from the API response if present.
func captureResponseID(state *streamingState, resp responses.Response) {
	if id := strings.TrimSpace(resp.ID); id != "" {
		state.responseID = id
	}
}

func (oc *AIClient) handleResponseLifecycleEvent(
	ctx context.Context,
	portal *bridgev2.Portal,
	state *streamingState,
	meta *PortalMetadata,
	eventType string,
	response responses.Response,
) {
	captureResponseID(state, response)
	oc.emitUIRuntimeMetadata(ctx, portal, state, meta, responseMetadataDeltaFromResponse(response))

	switch eventType {
	case "response.failed":
		state.finishReason = "error"
		if msg := strings.TrimSpace(response.Error.Message); msg != "" {
			oc.uiEmitter(state).EmitUIError(ctx, portal, msg)
		}
	case "response.incomplete":
		state.finishReason = strings.TrimSpace(string(response.IncompleteDetails.Reason))
		if state.finishReason == "" {
			state.finishReason = "other"
		}
	}
}
