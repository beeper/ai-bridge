package agents

import (
	"sync"
	"time"
)

// ContextGuard tracks token usage and message counts per session to prevent
// runaway conversations and context window overflow.
// Inspired by OpenClaw's context window guardrails.
type ContextGuard struct {
	mu sync.RWMutex

	// Limits
	maxMessages       int
	maxTokensEstimate int
	maxTurnsPerMinute int

	// Current state
	messageCount   int
	tokenEstimate  int
	turnTimestamps []time.Time

	// Session info
	sessionID string
	startedAt time.Time

	// Callbacks
	onWarning func(warning ContextWarning)
}

// ContextWarning represents a warning about context usage.
type ContextWarning struct {
	Type      ContextWarningType
	Message   string
	Current   int
	Limit     int
	SessionID string
}

// ContextWarningType categorizes context warnings.
type ContextWarningType string

const (
	WarningHighMessageCount ContextWarningType = "high_message_count"
	WarningHighTokenUsage   ContextWarningType = "high_token_usage"
	WarningHighTurnRate     ContextWarningType = "high_turn_rate"
	WarningContextOverflow  ContextWarningType = "context_overflow"
)

// ContextGuardConfig holds configuration for the context guard.
type ContextGuardConfig struct {
	// MaxMessages is the maximum number of messages before warning (default: 100)
	MaxMessages int

	// MaxTokensEstimate is the estimated max tokens before warning (default: 100000)
	MaxTokensEstimate int

	// MaxTurnsPerMinute limits rapid-fire interactions (default: 30)
	MaxTurnsPerMinute int

	// OnWarning is called when a limit is approached
	OnWarning func(ContextWarning)
}

// DefaultContextGuardConfig returns sensible defaults.
func DefaultContextGuardConfig() ContextGuardConfig {
	return ContextGuardConfig{
		MaxMessages:       100,
		MaxTokensEstimate: 100000,
		MaxTurnsPerMinute: 30,
	}
}

// NewContextGuard creates a new context guard with the given configuration.
func NewContextGuard(sessionID string, config ContextGuardConfig) *ContextGuard {
	if config.MaxMessages == 0 {
		config.MaxMessages = 100
	}
	if config.MaxTokensEstimate == 0 {
		config.MaxTokensEstimate = 100000
	}
	if config.MaxTurnsPerMinute == 0 {
		config.MaxTurnsPerMinute = 30
	}

	return &ContextGuard{
		sessionID:         sessionID,
		maxMessages:       config.MaxMessages,
		maxTokensEstimate: config.MaxTokensEstimate,
		maxTurnsPerMinute: config.MaxTurnsPerMinute,
		startedAt:         time.Now(),
		turnTimestamps:    make([]time.Time, 0, config.MaxTurnsPerMinute),
		onWarning:         config.OnWarning,
	}
}

// RecordMessage records a new message and its estimated token count.
// Returns any warnings triggered by this message.
func (g *ContextGuard) RecordMessage(tokenEstimate int) []ContextWarning {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.messageCount++
	g.tokenEstimate += tokenEstimate

	now := time.Now()
	g.turnTimestamps = append(g.turnTimestamps, now)

	// Clean old timestamps (older than 1 minute)
	cutoff := now.Add(-time.Minute)
	newTimestamps := make([]time.Time, 0, len(g.turnTimestamps))
	for _, ts := range g.turnTimestamps {
		if ts.After(cutoff) {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	g.turnTimestamps = newTimestamps

	return g.checkLimitsLocked()
}

// checkLimitsLocked checks all limits and returns any warnings.
// Must be called with lock held.
func (g *ContextGuard) checkLimitsLocked() []ContextWarning {
	var warnings []ContextWarning

	// Check message count (warn at 80%)
	messageThreshold := int(float64(g.maxMessages) * 0.8)
	if g.messageCount >= g.maxMessages {
		w := ContextWarning{
			Type:      WarningContextOverflow,
			Message:   "Maximum message count reached",
			Current:   g.messageCount,
			Limit:     g.maxMessages,
			SessionID: g.sessionID,
		}
		warnings = append(warnings, w)
	} else if g.messageCount >= messageThreshold {
		w := ContextWarning{
			Type:      WarningHighMessageCount,
			Message:   "Approaching maximum message count",
			Current:   g.messageCount,
			Limit:     g.maxMessages,
			SessionID: g.sessionID,
		}
		warnings = append(warnings, w)
	}

	// Check token estimate (warn at 80%)
	tokenThreshold := int(float64(g.maxTokensEstimate) * 0.8)
	if g.tokenEstimate >= g.maxTokensEstimate {
		w := ContextWarning{
			Type:      WarningContextOverflow,
			Message:   "Estimated token limit reached",
			Current:   g.tokenEstimate,
			Limit:     g.maxTokensEstimate,
			SessionID: g.sessionID,
		}
		warnings = append(warnings, w)
	} else if g.tokenEstimate >= tokenThreshold {
		w := ContextWarning{
			Type:      WarningHighTokenUsage,
			Message:   "Approaching estimated token limit",
			Current:   g.tokenEstimate,
			Limit:     g.maxTokensEstimate,
			SessionID: g.sessionID,
		}
		warnings = append(warnings, w)
	}

	// Check turn rate
	turnsLastMinute := len(g.turnTimestamps)
	if turnsLastMinute >= g.maxTurnsPerMinute {
		w := ContextWarning{
			Type:      WarningHighTurnRate,
			Message:   "High turn rate detected",
			Current:   turnsLastMinute,
			Limit:     g.maxTurnsPerMinute,
			SessionID: g.sessionID,
		}
		warnings = append(warnings, w)
	}

	// Call warning callback if set
	if g.onWarning != nil {
		for _, w := range warnings {
			g.onWarning(w)
		}
	}

	return warnings
}

// Stats returns current usage statistics.
func (g *ContextGuard) Stats() ContextStats {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return ContextStats{
		SessionID:       g.sessionID,
		MessageCount:    g.messageCount,
		TokenEstimate:   g.tokenEstimate,
		TurnsLastMinute: len(g.turnTimestamps),
		Duration:        time.Since(g.startedAt),
		MaxMessages:     g.maxMessages,
		MaxTokens:       g.maxTokensEstimate,
		MaxTurnRate:     g.maxTurnsPerMinute,
	}
}

// ContextStats holds current context usage statistics.
type ContextStats struct {
	SessionID       string
	MessageCount    int
	TokenEstimate   int
	TurnsLastMinute int
	Duration        time.Duration
	MaxMessages     int
	MaxTokens       int
	MaxTurnRate     int
}

// UsagePercent returns the percentage of context used (by messages or tokens, whichever is higher).
func (s ContextStats) UsagePercent() float64 {
	msgPercent := float64(s.MessageCount) / float64(s.MaxMessages) * 100
	tokPercent := float64(s.TokenEstimate) / float64(s.MaxTokens) * 100
	return max(msgPercent, tokPercent)
}

// IsHealthy returns true if usage is below 80% on all metrics.
func (s ContextStats) IsHealthy() bool {
	return s.UsagePercent() < 80 && s.TurnsLastMinute < s.MaxTurnRate
}

// ShouldWarn returns true if any limit is approaching.
func (g *ContextGuard) ShouldWarn() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	messageThreshold := int(float64(g.maxMessages) * 0.8)
	tokenThreshold := int(float64(g.maxTokensEstimate) * 0.8)

	return g.messageCount >= messageThreshold ||
		g.tokenEstimate >= tokenThreshold ||
		len(g.turnTimestamps) >= g.maxTurnsPerMinute
}

// ShouldBlock returns true if any hard limit is exceeded.
func (g *ContextGuard) ShouldBlock() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.messageCount >= g.maxMessages || g.tokenEstimate >= g.maxTokensEstimate
}

// Reset clears all tracked state (for new sessions or manual reset).
func (g *ContextGuard) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.messageCount = 0
	g.tokenEstimate = 0
	g.turnTimestamps = make([]time.Time, 0, g.maxTurnsPerMinute)
	g.startedAt = time.Now()
}

// EstimateTokens provides a simple token estimation for text.
// Uses the approximation of ~4 characters per token for English text.
func EstimateTokens(text string) int {
	// Rough estimate: ~4 chars per token for English
	// This is intentionally conservative
	chars := len(text)
	tokens := (chars + 3) / 4 // Round up
	return tokens
}
