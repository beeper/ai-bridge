package connector

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCodex_Dispatch_RoutesByThreadTurn(t *testing.T) {
	cc := &CodexClient{
		notifCh:       make(chan codexNotif, 16),
		turnSubs:      make(map[string]chan codexNotif),
		activeTurns:   make(map[string]*codexActiveTurn),
		loadedThreads: make(map[string]bool),
	}
	go cc.dispatchNotifications()

	ch1 := cc.subscribeTurn("thr1", "turn1")
	ch2 := cc.subscribeTurn("thr2", "turn2")
	defer cc.unsubscribeTurn("thr1", "turn1")
	defer cc.unsubscribeTurn("thr2", "turn2")

	p1, _ := json.Marshal(map[string]any{"threadId": "thr1", "turnId": "turn1", "delta": "a"})
	p2, _ := json.Marshal(map[string]any{"threadId": "thr2", "turnId": "turn2", "delta": "b"})

	cc.notifCh <- codexNotif{Method: "item/agentMessage/delta", Params: p1}
	cc.notifCh <- codexNotif{Method: "item/agentMessage/delta", Params: p2}

	// Each channel should receive only its own event.
	select {
	case evt := <-ch1:
		if evt.Method != "item/agentMessage/delta" {
			t.Fatalf("unexpected evt on ch1: %+v", evt)
		}
		var p map[string]any
		_ = json.Unmarshal(evt.Params, &p)
		if p["threadId"] != "thr1" || p["turnId"] != "turn1" {
			t.Fatalf("misrouted to ch1: %v", p)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for ch1")
	}

	select {
	case evt := <-ch2:
		if evt.Method != "item/agentMessage/delta" {
			t.Fatalf("unexpected evt on ch2: %+v", evt)
		}
		var p map[string]any
		_ = json.Unmarshal(evt.Params, &p)
		if p["threadId"] != "thr2" || p["turnId"] != "turn2" {
			t.Fatalf("misrouted to ch2: %v", p)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for ch2")
	}
}
