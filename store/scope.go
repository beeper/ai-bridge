package store

import "go.mau.fi/util/dbutil"

// Scope is a typed handle over the shared child DB for one bridge/login/agent
// tuple. Individual stores derive their queries from this scope.
type Scope struct {
	DB       *dbutil.Database
	BridgeID string
	LoginID  string
	AgentID  string
}
