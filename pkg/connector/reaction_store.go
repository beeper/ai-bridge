package connector

import (
	"sync"
	"time"

	"maunium.net/go/mautrix/id"
)

const (
	sentReactionTTL             = 24 * time.Hour
	sentReactionCleanupInterval = 30 * time.Minute
)

type sentReactionEntry struct {
	reactionEventID id.EventID
	storedAt        time.Time
}

var (
	sentReactionStore   = make(map[id.RoomID]map[id.EventID]map[string][]sentReactionEntry)
	sentReactionStoreMu sync.Mutex
)

func init() {
	go cleanupSentReactionStore()
}

func storeSentReaction(roomID id.RoomID, targetEventID id.EventID, emoji string, reactionEventID id.EventID) {
	if roomID == "" || targetEventID == "" || emoji == "" || reactionEventID == "" {
		return
	}

	sentReactionStoreMu.Lock()
	defer sentReactionStoreMu.Unlock()

	roomReactions := sentReactionStore[roomID]
	if roomReactions == nil {
		roomReactions = make(map[id.EventID]map[string][]sentReactionEntry)
		sentReactionStore[roomID] = roomReactions
	}

	messageReactions := roomReactions[targetEventID]
	if messageReactions == nil {
		messageReactions = make(map[string][]sentReactionEntry)
		roomReactions[targetEventID] = messageReactions
	}

	messageReactions[emoji] = append(messageReactions[emoji], sentReactionEntry{
		reactionEventID: reactionEventID,
		storedAt:        time.Now(),
	})
}

func takeSentReactions(roomID id.RoomID, targetEventID id.EventID, emoji string) []id.EventID {
	if roomID == "" || targetEventID == "" {
		return nil
	}

	sentReactionStoreMu.Lock()
	defer sentReactionStoreMu.Unlock()

	roomReactions := sentReactionStore[roomID]
	if roomReactions == nil {
		return nil
	}
	messageReactions := roomReactions[targetEventID]
	if messageReactions == nil {
		return nil
	}

	var removed []id.EventID
	if emoji == "" {
		for _, entries := range messageReactions {
			for _, entry := range entries {
				removed = append(removed, entry.reactionEventID)
			}
		}
		delete(roomReactions, targetEventID)
		if len(roomReactions) == 0 {
			delete(sentReactionStore, roomID)
		}
		return removed
	}

	entries := messageReactions[emoji]
	if len(entries) == 0 {
		return nil
	}
	removed = make([]id.EventID, 0, len(entries))
	for _, entry := range entries {
		removed = append(removed, entry.reactionEventID)
	}

	delete(messageReactions, emoji)
	if len(messageReactions) == 0 {
		delete(roomReactions, targetEventID)
		if len(roomReactions) == 0 {
			delete(sentReactionStore, roomID)
		}
	}

	return removed
}

func cleanupSentReactionStore() {
	ticker := time.NewTicker(sentReactionCleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-sentReactionTTL)
		sentReactionStoreMu.Lock()
		for roomID, roomReactions := range sentReactionStore {
			for eventID, messageReactions := range roomReactions {
				for emoji, entries := range messageReactions {
					kept := entries[:0]
					for _, entry := range entries {
						if entry.storedAt.After(cutoff) {
							kept = append(kept, entry)
						}
					}
					if len(kept) == 0 {
						delete(messageReactions, emoji)
					} else {
						messageReactions[emoji] = kept
					}
				}
				if len(messageReactions) == 0 {
					delete(roomReactions, eventID)
				}
			}
			if len(roomReactions) == 0 {
				delete(sentReactionStore, roomID)
			}
		}
		sentReactionStoreMu.Unlock()
	}
}
