package connector

import "sync"

type HeartbeatIndicatorType string

const (
	HeartbeatIndicatorOK    HeartbeatIndicatorType = "ok"
	HeartbeatIndicatorAlert HeartbeatIndicatorType = "alert"
	HeartbeatIndicatorError HeartbeatIndicatorType = "error"
)

type HeartbeatEventPayload struct {
	TS            int64                   `json:"ts"`
	Status        string                  `json:"status"`
	To            string                  `json:"to,omitempty"`
	Preview       string                  `json:"preview,omitempty"`
	DurationMs    int64                   `json:"durationMs,omitempty"`
	HasMedia      bool                    `json:"hasMedia,omitempty"`
	Reason        string                  `json:"reason,omitempty"`
	Channel       string                  `json:"channel,omitempty"`
	Silent        bool                    `json:"silent,omitempty"`
	IndicatorType *HeartbeatIndicatorType `json:"indicatorType,omitempty"`
}

func resolveIndicatorType(status string) *HeartbeatIndicatorType {
	switch status {
	case "ok-empty", "ok-token":
		v := HeartbeatIndicatorOK
		return &v
	case "sent":
		v := HeartbeatIndicatorAlert
		return &v
	case "failed":
		v := HeartbeatIndicatorError
		return &v
	default:
		return nil
	}
}

var heartbeatEvents struct {
	mu        sync.Mutex
	last      *HeartbeatEventPayload
	listeners map[int]func(*HeartbeatEventPayload)
	nextID    int
}

func emitHeartbeatEvent(evt *HeartbeatEventPayload) {
	if evt == nil {
		return
	}
	heartbeatEvents.mu.Lock()
	heartbeatEvents.last = evt
	listeners := make([]func(*HeartbeatEventPayload), 0, len(heartbeatEvents.listeners))
	for _, fn := range heartbeatEvents.listeners {
		listeners = append(listeners, fn)
	}
	heartbeatEvents.mu.Unlock()
	for _, fn := range listeners {
		func(handler func(*HeartbeatEventPayload)) {
			defer func() { _ = recover() }()
			handler(evt)
		}(fn)
	}
}

//lint:ignore U1000 OpenClaw parity: expose heartbeat event subscription for UI integrations.
func onHeartbeatEvent(listener func(*HeartbeatEventPayload)) func() {
	if listener == nil {
		return func() {}
	}
	heartbeatEvents.mu.Lock()
	if heartbeatEvents.listeners == nil {
		heartbeatEvents.listeners = make(map[int]func(*HeartbeatEventPayload))
	}
	heartbeatEvents.nextID++
	id := heartbeatEvents.nextID
	heartbeatEvents.listeners[id] = listener
	heartbeatEvents.mu.Unlock()
	return func() {
		heartbeatEvents.mu.Lock()
		delete(heartbeatEvents.listeners, id)
		heartbeatEvents.mu.Unlock()
	}
}

//lint:ignore U1000 OpenClaw parity: expose last heartbeat snapshot for status panels.
func getLastHeartbeatEvent() *HeartbeatEventPayload {
	heartbeatEvents.mu.Lock()
	defer heartbeatEvents.mu.Unlock()
	if heartbeatEvents.last == nil {
		return nil
	}
	eventsCopy := *heartbeatEvents.last
	return &eventsCopy
}
