package codex

import (
	"testing"
	"time"

	"maunium.net/go/mautrix/bridgev2"
)

func TestCodexTurnTextPair(t *testing.T) {
	turn := codexTurn{
		ID: "turn_1",
		Items: []codexTurnItem{
			{
				Type: "userMessage",
				Content: []codexUserInput{
					{Type: "text", Text: "first line"},
					{Type: "mention", Text: "ignored"},
					{Type: "text", Text: "second line"},
				},
			},
			{Type: "agentMessage", ID: "a1", Text: "draft"},
			{Type: "agentMessage", ID: "a1", Text: "final"},
			{Type: "agentMessage", ID: "a2", Text: "follow-up"},
		},
	}

	userText, assistantText := codexTurnTextPair(turn)
	if userText != "first line\n\nsecond line" {
		t.Fatalf("unexpected user text: %q", userText)
	}
	if assistantText != "final\n\nfollow-up" {
		t.Fatalf("unexpected assistant text: %q", assistantText)
	}
}

func TestCodexThreadBackfillEntries(t *testing.T) {
	thread := codexThread{
		ID:        "thr_123",
		CreatedAt: 1_700_000_000,
		Turns: []codexTurn{
			{
				ID: "turn_1",
				Items: []codexTurnItem{
					{Type: "userMessage", Content: []codexUserInput{{Type: "text", Text: "hello"}}},
					{Type: "agentMessage", ID: "a1", Text: "hi"},
				},
			},
			{
				ID: "turn_2",
				Items: []codexTurnItem{
					{Type: "userMessage", Content: []codexUserInput{{Type: "text", Text: "how are you?"}}},
					{Type: "agentMessage", ID: "a2", Text: "doing well"},
				},
			},
		},
	}
	entries := codexThreadBackfillEntries(thread, bridgev2.EventSender{IsFromMe: true}, bridgev2.EventSender{})
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}
	for i := 1; i < len(entries); i++ {
		if entries[i].Timestamp.Before(entries[i-1].Timestamp) {
			t.Fatalf("entries out of order at index %d", i)
		}
		if entries[i].StreamOrder <= entries[i-1].StreamOrder {
			t.Fatalf("stream order is not strictly increasing at index %d", i)
		}
	}
	seenIDs := make(map[string]struct{})
	for _, entry := range entries {
		if entry.MessageID == "" {
			t.Fatalf("entry has empty message id: %+v", entry)
		}
		if _, exists := seenIDs[string(entry.MessageID)]; exists {
			t.Fatalf("duplicate message id: %q", entry.MessageID)
		}
		seenIDs[string(entry.MessageID)] = struct{}{}
	}
}

func TestCodexPaginateBackfillBackward(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	entries := []codexBackfillEntry{
		{MessageID: "m1", Timestamp: now, StreamOrder: 1},
		{MessageID: "m2", Timestamp: now.Add(time.Second), StreamOrder: 2},
		{MessageID: "m3", Timestamp: now.Add(2 * time.Second), StreamOrder: 3},
	}

	firstBatch, cursor, hasMore := codexPaginateBackfill(entries, bridgev2.FetchMessagesParams{
		Forward: false,
		Count:   2,
	})
	if len(firstBatch) != 2 || string(firstBatch[0].MessageID) != "m2" || string(firstBatch[1].MessageID) != "m3" {
		t.Fatalf("unexpected first backward batch: %+v", firstBatch)
	}
	if !hasMore || cursor == "" {
		t.Fatalf("expected hasMore=true and non-empty cursor, got hasMore=%v cursor=%q", hasMore, cursor)
	}

	secondBatch, _, hasMore := codexPaginateBackfill(entries, bridgev2.FetchMessagesParams{
		Forward: false,
		Cursor:  cursor,
		Count:   2,
	})
	if len(secondBatch) != 1 || string(secondBatch[0].MessageID) != "m1" {
		t.Fatalf("unexpected second backward batch: %+v", secondBatch)
	}
	if hasMore {
		t.Fatalf("expected hasMore=false on final batch")
	}
}
