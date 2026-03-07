package memory

import (
	"context"

	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

// SessionPortal identifies a chat session portal that can be indexed into memory.
type SessionPortal struct {
	Key       string
	PortalKey networkid.PortalKey
}

// Runtime adapts connector-specific context for memory manager logic.
type Runtime interface {
	ResolveConfig(agentID string) (*ResolvedConfig, error)

	ResolvePromptWorkspaceDir() string
	ListSessionPortals(ctx context.Context, loginID, agentID string) ([]SessionPortal, error)

	BridgeDB() *dbutil.Database
	BridgeID() string
	LoginID() string
	Logger() zerolog.Logger
}
