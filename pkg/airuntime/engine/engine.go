package engine

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/beeper/ai-bridge/pkg/airuntime/stream"
	"github.com/beeper/ai-bridge/pkg/airuntime/tools"
	"github.com/beeper/ai-bridge/pkg/matrixtransport"
)

// Engine is the future shared runtime entry point.
//
// Initial extraction is intentionally incremental: existing bridge logic remains
// in pkg/connector until this engine takes over provider/tool loops.
type Engine struct {
	Log      zerolog.Logger
	T        matrixtransport.Transport
	Stream   *stream.Emitter
	ToolExec *tools.Executor
}

func (e *Engine) HandleEvent(ctx context.Context) error {
	_ = ctx
	// TODO: implement. This placeholder is here to stabilize package layout while
	// we refactor connector logic into the engine.
	return nil
}
