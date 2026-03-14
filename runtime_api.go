package agentremote

import (
	"maunium.net/go/mautrix/bridgev2"

	"github.com/beeper/agentremote/store"
)

// RuntimeConfig describes the bridge-scoped inputs required to construct the
// public agentremote runtime facade.
type RuntimeConfig struct {
	Bridge  *bridgev2.Bridge
	Login   *bridgev2.UserLogin
	AgentID string
}

// Runtime is the top-level bridge builder entrypoint. It groups the managed
// turn, approval, and store services for a specific login scope.
type Runtime struct {
	Bridge    *bridgev2.Bridge
	Login     *bridgev2.UserLogin
	AgentID   string
	Turns     *TurnManager
	Approvals *ApprovalFlow[map[string]any]
	Stores    *store.Scope
}
