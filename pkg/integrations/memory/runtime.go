package memory

import (
	"context"

	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/bridgev2/networkid"

	memorycore "github.com/beeper/ai-bridge/pkg/memory"
)

// SessionPortal identifies a chat session portal that can be indexed into memory.
type SessionPortal struct {
	Key       string
	PortalKey networkid.PortalKey
}

// Runtime adapts connector-specific context for memory manager logic.
type Runtime interface {
	ResolveConfig(agentID string) (*memorycore.ResolvedConfig, error)

	ResolveOpenAIEmbeddingConfig(cfg *memorycore.ResolvedConfig) (string, string, map[string]string)
	ResolveDirectOpenAIEmbeddingConfig(cfg *memorycore.ResolvedConfig) (string, string, map[string]string)
	ResolveGeminiEmbeddingConfig(cfg *memorycore.ResolvedConfig) (string, string, map[string]string)

	ResolvePromptWorkspaceDir() string
	ListSessionPortals(ctx context.Context, loginID, agentID string) ([]SessionPortal, error)

	BridgeDB() *dbutil.Database
	BridgeID() string
	LoginID() string
	Logger() zerolog.Logger
}
