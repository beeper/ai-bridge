package codex

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/xid"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

func makeCodexUserLoginID(mxid id.UserID, instanceID string) networkid.UserLoginID {
	escaped := url.PathEscape(string(mxid))
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		instanceID = xid.New().String()
	}
	return networkid.UserLoginID(fmt.Sprintf("codex:%s:%s", escaped, instanceID))
}

func codexChatPortalKey(loginID networkid.UserLoginID, slug string) networkid.PortalKey {
	return networkid.PortalKey{
		ID:       networkid.PortalID(fmt.Sprintf("codex:%s:%s", loginID, slug)),
		Receiver: loginID,
	}
}

// MakeMessageID creates a message ID from a Matrix event ID.
func MakeMessageID(eventID id.EventID) networkid.MessageID {
	return networkid.MessageID(fmt.Sprintf("mx:%s", string(eventID)))
}

func generateShortID() string {
	return xid.New().String()
}

func expandUserPath(value string) string {
	if strings.HasPrefix(value, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return value
		}
		trimmed := strings.TrimPrefix(value, "~")
		if trimmed == "" {
			return home
		}
		if strings.HasPrefix(trimmed, string(filepath.Separator)) {
			return filepath.Join(home, trimmed[1:])
		}
		return filepath.Join(home, trimmed)
	}
	return value
}

