package tools

import (
	"sync"
	"time"
)

// Guard tracks pending tool calls to prevent duplicate execution
// and ensure tool results are properly matched to requests.
// Based on clawdbot's session-tool-result-guard pattern.
type Guard struct {
	mu       sync.Mutex
	pending  map[string]*PendingCall // callID -> pending call info
	timeout  time.Duration
	stopChan chan struct{} // signals cleanup goroutine to stop
	stopped  bool          // true if Stop() has been called
}

// PendingCall tracks a tool call waiting for result.
type PendingCall struct {
	CallID    string
	ToolName  string
	Input     map[string]any
	StartedAt time.Time
	Callback  func(*Result) // Optional callback when result arrives
}

// NewGuard creates a new guard with the specified timeout.
// The guard starts a background goroutine that periodically cleans up stale entries.
// Call Stop() when the guard is no longer needed to stop the cleanup goroutine.
func NewGuard(timeout time.Duration) *Guard {
	g := &Guard{
		pending:  make(map[string]*PendingCall),
		timeout:  timeout,
		stopChan: make(chan struct{}),
	}
	go g.cleanupLoop()
	return g
}

// DefaultGuard creates a guard with a 5-minute timeout.
func DefaultGuard() *Guard {
	return NewGuard(5 * time.Minute)
}

// cleanupLoop runs in the background and periodically removes stale pending calls.
func (g *Guard) cleanupLoop() {
	// Cleanup every minute or at timeout/2, whichever is smaller
	interval := g.timeout / 2
	if interval > time.Minute {
		interval = time.Minute
	}
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			g.CleanupStale()
		case <-g.stopChan:
			return
		}
	}
}

// Stop stops the background cleanup goroutine.
// After calling Stop, the guard should not be used.
func (g *Guard) Stop() {
	g.mu.Lock()
	if !g.stopped {
		g.stopped = true
		close(g.stopChan)
	}
	g.mu.Unlock()
}

// Register marks a tool call as pending.
// Returns false if the call ID is already registered (duplicate).
func (g *Guard) Register(callID, toolName string, input map[string]any) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.pending[callID]; exists {
		return false // Duplicate call
	}

	g.pending[callID] = &PendingCall{
		CallID:    callID,
		ToolName:  toolName,
		Input:     input,
		StartedAt: time.Now(),
	}
	return true
}

// RegisterWithCallback registers a pending call with a completion callback.
func (g *Guard) RegisterWithCallback(callID, toolName string, input map[string]any, cb func(*Result)) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.pending[callID]; exists {
		return false // Duplicate call
	}

	g.pending[callID] = &PendingCall{
		CallID:    callID,
		ToolName:  toolName,
		Input:     input,
		StartedAt: time.Now(),
		Callback:  cb,
	}
	return true
}

// Complete marks a tool call as done and returns the pending call info.
// Returns nil if the call was not registered or already completed.
func (g *Guard) Complete(callID string) *PendingCall {
	g.mu.Lock()
	defer g.mu.Unlock()

	call := g.pending[callID]
	delete(g.pending, callID)
	return call
}

// CompleteWithResult marks a call as done and invokes its callback if present.
func (g *Guard) CompleteWithResult(callID string, result *Result) *PendingCall {
	call := g.Complete(callID)
	if call != nil && call.Callback != nil {
		call.Callback(result)
	}
	return call
}

// IsPending checks if a call is currently pending.
func (g *Guard) IsPending(callID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, exists := g.pending[callID]
	return exists
}

// Get retrieves a pending call without completing it.
func (g *Guard) Get(callID string) *PendingCall {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.pending[callID]
}

// Pending returns all pending calls.
func (g *Guard) Pending() []*PendingCall {
	g.mu.Lock()
	defer g.mu.Unlock()

	calls := make([]*PendingCall, 0, len(g.pending))
	for _, call := range g.pending {
		calls = append(calls, call)
	}
	return calls
}

// PendingCount returns the number of pending calls.
func (g *Guard) PendingCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.pending)
}

// CleanupStale removes calls that have exceeded the timeout.
// Returns the removed calls for handling (e.g., sending timeout errors).
func (g *Guard) CleanupStale() []*PendingCall {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	var stale []*PendingCall

	for callID, call := range g.pending {
		if now.Sub(call.StartedAt) > g.timeout {
			stale = append(stale, call)
			delete(g.pending, callID)
		}
	}

	return stale
}

// Clear removes all pending calls.
func (g *Guard) Clear() []*PendingCall {
	g.mu.Lock()
	defer g.mu.Unlock()

	calls := make([]*PendingCall, 0, len(g.pending))
	for _, call := range g.pending {
		calls = append(calls, call)
	}
	g.pending = make(map[string]*PendingCall)
	return calls
}

// Duration returns how long a call has been pending.
func (p *PendingCall) Duration() time.Duration {
	return time.Since(p.StartedAt)
}

// IsExpired checks if the call has exceeded the given timeout.
func (p *PendingCall) IsExpired(timeout time.Duration) bool {
	return p.Duration() > timeout
}
