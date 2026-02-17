package opencode

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

func makeOpenCodeUserLoginID(mxid id.UserID, instanceID string) networkid.UserLoginID {
	escaped := url.PathEscape(string(mxid))
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		h := sha256.Sum256([]byte("default"))
		instanceID = hex.EncodeToString(h[:8])
	}
	return networkid.UserLoginID(fmt.Sprintf("opencode:%s:%s", escaped, instanceID))
}
