package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/beeper/ai-bridge/pkg/cron"
	"github.com/beeper/ai-bridge/pkg/textfs"
)

const cronStoreAgentID = "__cron__"

type cronTextFSBackend struct {
	store *textfs.Store
}

func (b *cronTextFSBackend) Read(ctx context.Context, path string) ([]byte, bool, error) {
	if b == nil || b.store == nil {
		return nil, false, fmt.Errorf("cron store not available")
	}
	entry, found, err := b.store.Read(ctx, path)
	if err != nil || !found {
		return nil, found, err
	}
	return []byte(entry.Content), true, nil
}

func (b *cronTextFSBackend) Write(ctx context.Context, path string, data []byte) error {
	if b == nil || b.store == nil {
		return fmt.Errorf("cron store not available")
	}
	_, err := b.store.Write(ctx, path, string(data))
	return err
}

func (oc *AIClient) cronTextFSStore() (*textfs.Store, error) {
	if oc == nil || oc.UserLogin == nil || oc.UserLogin.Bridge == nil || oc.UserLogin.Bridge.DB == nil {
		return nil, fmt.Errorf("cron store not available")
	}
	bridgeID := string(oc.UserLogin.Bridge.DB.BridgeID)
	loginID := string(oc.UserLogin.ID)
	agentID := strings.TrimSpace(cronStoreAgentID)
	if agentID == "" {
		agentID = "cron"
	}
	return textfs.NewStore(oc.UserLogin.Bridge.DB.Database, bridgeID, loginID, agentID), nil
}

func (oc *AIClient) cronStoreBackend() cron.StoreBackend {
	store, err := oc.cronTextFSStore()
	if err != nil {
		return nil
	}
	return &cronTextFSBackend{store: store}
}
