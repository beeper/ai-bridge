package connector

import (
	"strings"
	"time"

	"github.com/beeper/ai-bridge/pkg/aityping"
)

type TypingMode = aityping.TypingMode

const (
	TypingModeNever    = aityping.TypingModeNever
	TypingModeInstant  = aityping.TypingModeInstant
	TypingModeThinking = aityping.TypingModeThinking
	TypingModeMessage  = aityping.TypingModeMessage
)

const defaultTypingInterval = 6 * time.Second

var normalizeTypingMode = aityping.NormalizeTypingMode

func (oc *AIClient) resolveTypingMode(meta *PortalMetadata, ctx *TypingContext, isHeartbeat bool) TypingMode {
	if isHeartbeat {
		return TypingModeNever
	}
	if meta != nil {
		if mode, ok := normalizeTypingMode(meta.TypingMode); ok {
			return mode
		}
	}
	isGroup := false
	wasMentioned := false
	if ctx != nil {
		isGroup = ctx.IsGroup
		wasMentioned = ctx.WasMentioned
	}
	if !isGroup || wasMentioned {
		return TypingModeInstant
	}
	return TypingModeMessage
}

func (oc *AIClient) resolveTypingInterval(meta *PortalMetadata) time.Duration {
	interval := defaultTypingInterval
	if meta != nil && meta.TypingIntervalSeconds != nil {
		interval = time.Duration(*meta.TypingIntervalSeconds) * time.Second
		if interval <= 0 {
			return 0
		}
		return interval
	}
	if interval <= 0 {
		return 0
	}
	return interval
}

type TypingSignaler struct {
	mode                 TypingMode
	typing               *TypingController
	disabled             bool
	shouldStartImmediate bool
	shouldStartOnMessage bool
	shouldStartOnText    bool
	shouldStartOnReason  bool
	hasRenderableText    bool
}

func NewTypingSignaler(typing *TypingController, mode TypingMode, isHeartbeat bool) *TypingSignaler {
	disabled := isHeartbeat || mode == TypingModeNever || typing == nil
	return &TypingSignaler{
		mode:                 mode,
		typing:               typing,
		disabled:             disabled,
		shouldStartImmediate: mode == TypingModeInstant,
		shouldStartOnMessage: mode == TypingModeMessage,
		shouldStartOnText:    mode == TypingModeMessage || mode == TypingModeInstant,
		shouldStartOnReason:  mode == TypingModeThinking,
	}
}

func (ts *TypingSignaler) SignalRunStart() {
	if ts == nil || ts.disabled || !ts.shouldStartImmediate {
		return
	}
	ts.typing.Start()
}

func (ts *TypingSignaler) SignalMessageStart() {
	if ts == nil || ts.disabled || !ts.shouldStartOnMessage {
		return
	}
	if !ts.hasRenderableText {
		return
	}
	ts.typing.Start()
}

func (ts *TypingSignaler) SignalTextDelta(text string) {
	if ts == nil || ts.disabled {
		return
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}
	renderable := !isSilentReplyText(trimmed)
	if renderable {
		ts.hasRenderableText = true
	} else {
		return
	}
	if ts.shouldStartOnText {
		ts.typing.Start()
		ts.typing.RefreshTTL()
		return
	}
	if ts.shouldStartOnReason {
		if !ts.typing.IsActive() {
			ts.typing.Start()
		}
		ts.typing.RefreshTTL()
	}
}

func (ts *TypingSignaler) SignalReasoningDelta() {
	if ts == nil || ts.disabled || !ts.shouldStartOnReason {
		return
	}
	if !ts.hasRenderableText {
		return
	}
	ts.typing.Start()
	ts.typing.RefreshTTL()
}

func (ts *TypingSignaler) SignalToolStart() {
	if ts == nil || ts.disabled {
		return
	}
	if !ts.typing.IsActive() {
		ts.typing.Start()
		ts.typing.RefreshTTL()
		return
	}
	ts.typing.RefreshTTL()
}
