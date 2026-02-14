package connector

import (
	"context"

	"github.com/openai/openai-go/v3/responses"
	"maunium.net/go/mautrix/bridgev2"
)

// StreamingHooks defines extension points for the streaming engine.
// Beep overrides these to inject MCP tools, tool approval gating,
// and memory flush behavior. The simple bridge uses NoopStreamingHooks.
type StreamingHooks interface {
	// AdditionalTools returns extra tool parameters to include in API requests.
	// Called during buildResponsesAPIParams after builtin tools are added.
	// Beep uses this to inject MCP tool definitions.
	AdditionalTools(ctx context.Context, meta *PortalMetadata) []responses.ToolUnionParam

	// OnContinuationPreSend is called before each continuation round in the
	// streaming loop. It may modify the pending function outputs (e.g. to
	// inject MCP approval items) and return additional input items to prepend
	// to the continuation request.
	OnContinuationPreSend(ctx context.Context, state *streamingState, outputs []functionCallOutput) (extraInput responses.ResponseInputParam, modifiedOutputs []functionCallOutput)

	// ShouldContinue is called at the top of each continuation iteration.
	// Return false to break out of the tool-call loop early.
	ShouldContinue(state *streamingState) bool

	// OnToolCallComplete is called after a builtin tool finishes execution,
	// before the result is recorded for continuation. Beep uses this for
	// tool-approval gating.
	OnToolCallComplete(ctx context.Context, toolCallID, toolName string, state *streamingState)

	// OnStreamFinished is called after the streaming response is fully
	// complete (messages sent, state saved). Beep uses this for memory flush.
	OnStreamFinished(ctx context.Context, portal *bridgev2.Portal, state *streamingState, meta *PortalMetadata)
}

// NoopStreamingHooks provides default no-op implementations of StreamingHooks.
// Used by the simple bridge where no extension behavior is needed.
type NoopStreamingHooks struct{}

var _ StreamingHooks = NoopStreamingHooks{}

func (NoopStreamingHooks) AdditionalTools(context.Context, *PortalMetadata) []responses.ToolUnionParam {
	return nil
}

func (NoopStreamingHooks) OnContinuationPreSend(_ context.Context, _ *streamingState, outputs []functionCallOutput) (responses.ResponseInputParam, []functionCallOutput) {
	return nil, outputs
}

func (NoopStreamingHooks) ShouldContinue(*streamingState) bool {
	return true
}

func (NoopStreamingHooks) OnToolCallComplete(context.Context, string, string, *streamingState) {}

func (NoopStreamingHooks) OnStreamFinished(context.Context, *bridgev2.Portal, *streamingState, *PortalMetadata) {
}
