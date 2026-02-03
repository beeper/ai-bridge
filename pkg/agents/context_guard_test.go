package agents

import (
	"sync"
	"testing"
	"time"
)

func TestContextGuardWarningCallbackNoDeadlock(t *testing.T) {
	done := make(chan struct{})
	var once sync.Once
	var guard *ContextGuard

	cfg := ContextGuardConfig{
		MaxMessages:       1,
		MaxTokensEstimate: 1,
		MaxTurnsPerMinute: 1,
		OnWarning: func(_ ContextWarning) {
			_ = guard.Stats()
			once.Do(func() {
				close(done)
			})
		},
	}

	guard = NewContextGuard("session", cfg)
	guard.RecordMessage(1)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for warning callback")
	}
}
