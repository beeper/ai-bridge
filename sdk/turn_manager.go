package sdk

import (
	"sync"
)

// TurnConfig configures per-room turn serialization.
type TurnConfig struct {
	OneAtATime bool
	DebounceMs int
	QueueSize  int
}

// TurnManager serializes turns per room.
type TurnManager struct {
	cfg   *TurnConfig
	mu    sync.Mutex
	rooms map[string]*roomTurnState
}

type roomTurnState struct {
	active bool
}

// NewTurnManager creates a new TurnManager with the given configuration.
func NewTurnManager(cfg *TurnConfig) *TurnManager {
	if cfg == nil {
		cfg = &TurnConfig{}
	}
	return &TurnManager{
		cfg:   cfg,
		rooms: make(map[string]*roomTurnState),
	}
}

// Acquire marks a room as having an active turn. If OneAtATime is enabled,
// it blocks conceptually (currently just marks active).
func (tm *TurnManager) Acquire(roomID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	state, ok := tm.rooms[roomID]
	if !ok {
		state = &roomTurnState{}
		tm.rooms[roomID] = state
	}
	state.active = true
}

// Release marks the room's turn as complete.
func (tm *TurnManager) Release(roomID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if state, ok := tm.rooms[roomID]; ok {
		state.active = false
	}
}

// IsActive returns whether a turn is currently active in the given room.
func (tm *TurnManager) IsActive(roomID string) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if state, ok := tm.rooms[roomID]; ok {
		return state.active
	}
	return false
}
