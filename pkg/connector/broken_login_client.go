package connector

import (
	"maunium.net/go/mautrix/bridgev2"

	"github.com/beeper/ai-bridge/pkg/bridgeadapter"
)

// newBrokenLoginClient creates a BrokenLoginClient that also wires up
// best-effort login data purge on logout.
func newBrokenLoginClient(login *bridgev2.UserLogin, reason string) *bridgeadapter.BrokenLoginClient {
	c := bridgeadapter.NewBrokenLoginClient(login, reason)
	c.OnLogout = purgeLoginDataBestEffort
	return c
}
