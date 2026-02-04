package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/beeper/ai-bridge/pkg/textfs"
)

const heartbeatStoreAgentID = "__heartbeat__"
const heartbeatSessionStorePath = "heartbeat/sessions.json"

type heartbeatSessionEntry struct {
	UpdatedAt           int64  `json:"updatedAt,omitempty"`
	LastHeartbeatText   string `json:"lastHeartbeatText,omitempty"`
	LastHeartbeatSentAt int64  `json:"lastHeartbeatSentAt,omitempty"`
	LastChannel         string `json:"lastChannel,omitempty"`
	LastTo              string `json:"lastTo,omitempty"`
	LastAccountID       string `json:"lastAccountId,omitempty"`
}

type heartbeatSessionStore struct {
	Sessions map[string]heartbeatSessionEntry `json:"sessions"`
}

func heartbeatStoreKey(agentID string) string {
	normalized := normalizeAgentID(agentID)
	if normalized == "" {
		normalized = "main"
	}
	return "agent:" + normalized
}

func (oc *AIClient) heartbeatTextFSStore() (*textfs.Store, error) {
	if oc == nil || oc.UserLogin == nil || oc.UserLogin.Bridge == nil || oc.UserLogin.Bridge.DB == nil {
		return nil, fmt.Errorf("heartbeat store not available")
	}
	bridgeID := string(oc.UserLogin.Bridge.DB.BridgeID)
	loginID := string(oc.UserLogin.ID)
	agentID := strings.TrimSpace(heartbeatStoreAgentID)
	if agentID == "" {
		agentID = "heartbeat"
	}
	return textfs.NewStore(oc.UserLogin.Bridge.DB.Database, bridgeID, loginID, agentID), nil
}

func (oc *AIClient) loadHeartbeatSessionStore(ctx context.Context) (heartbeatSessionStore, error) {
	store, err := oc.heartbeatTextFSStore()
	if err != nil {
		return heartbeatSessionStore{Sessions: map[string]heartbeatSessionEntry{}}, err
	}
	entry, found, err := store.Read(ctx, heartbeatSessionStorePath)
	if err != nil || !found {
		return heartbeatSessionStore{Sessions: map[string]heartbeatSessionEntry{}}, nil
	}
	var parsed heartbeatSessionStore
	if err := json.Unmarshal([]byte(entry.Content), &parsed); err != nil {
		return heartbeatSessionStore{Sessions: map[string]heartbeatSessionEntry{}}, nil
	}
	if parsed.Sessions == nil {
		parsed.Sessions = map[string]heartbeatSessionEntry{}
	}
	return parsed, nil
}

func (oc *AIClient) saveHeartbeatSessionStore(ctx context.Context, store heartbeatSessionStore) error {
	if store.Sessions == nil {
		store.Sessions = map[string]heartbeatSessionEntry{}
	}
	blob, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	textStore, err := oc.heartbeatTextFSStore()
	if err != nil {
		return err
	}
	_, err = textStore.Write(ctx, heartbeatSessionStorePath, string(blob))
	return err
}

func (oc *AIClient) getHeartbeatSessionEntry(ctx context.Context, sessionKey string) (heartbeatSessionEntry, bool) {
	store, err := oc.loadHeartbeatSessionStore(ctx)
	if err != nil {
		return heartbeatSessionEntry{}, false
	}
	entry, ok := store.Sessions[sessionKey]
	return entry, ok
}

func (oc *AIClient) updateHeartbeatSessionEntry(ctx context.Context, sessionKey string, updater func(entry heartbeatSessionEntry) heartbeatSessionEntry) {
	if oc == nil || strings.TrimSpace(sessionKey) == "" {
		return
	}
	store, err := oc.loadHeartbeatSessionStore(ctx)
	if err != nil {
		return
	}
	entry := store.Sessions[sessionKey]
	entry = updater(entry)
	store.Sessions[sessionKey] = entry
	_ = oc.saveHeartbeatSessionStore(ctx, store)
}

func heartbeatUpdatedAtNow() int64 {
	return time.Now().UnixMilli()
}
