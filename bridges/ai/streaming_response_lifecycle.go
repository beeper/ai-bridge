package ai

import (
	"context"
	"strings"

	"github.com/openai/openai-go/v3/responses"
	"maunium.net/go/mautrix/bridgev2"
)

func (oc *AIClient) handleResponseLifecycleEvent(
	ctx context.Context,
	portal *bridgev2.Portal,
	state *streamingState,
	meta *PortalMetadata,
	eventType string,
	response responses.Response,
) {
	if strings.TrimSpace(response.ID) != "" {
		state.responseID = response.ID
	}

	switch eventType {
	case "response.created", "response.queued", "response.in_progress":
		// No additional state changes needed.
	case "response.failed":
		state.finishReason = "error"
	case "response.incomplete":
		state.finishReason = strings.TrimSpace(string(response.IncompleteDetails.Reason))
		if state.finishReason == "" {
			state.finishReason = "other"
		}
	default:
		return
	}

	oc.emitUIRuntimeMetadata(ctx, portal, state, meta, responseMetadataDeltaFromResponse(response))

	if eventType == "response.failed" {
		if msg := strings.TrimSpace(response.Error.Message); msg != "" {
			oc.uiEmitter(state).EmitUIError(ctx, portal, msg)
		}
	}
}
