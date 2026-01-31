package aiid

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/rs/xid"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

// MakeUserLoginID creates a login ID from a Matrix user ID
func MakeUserLoginID(mxid id.UserID) networkid.UserLoginID {
	escaped := url.PathEscape(string(mxid))
	return networkid.UserLoginID(fmt.Sprintf("openai:%s", escaped))
}

// MakePortalKey creates a new portal key for a chat
func MakePortalKey(loginID networkid.UserLoginID) networkid.PortalKey {
	return networkid.PortalKey{
		ID:       networkid.PortalID(fmt.Sprintf("openai:%s:%s", loginID, xid.New().String())),
		Receiver: loginID,
	}
}

// MakeMessageID creates a message ID from a Matrix event ID
func MakeMessageID(eventID id.EventID) networkid.MessageID {
	return networkid.MessageID(fmt.Sprintf("mx:%s", string(eventID)))
}

// MakeCompletionID creates a message ID from an OpenAI completion ID
func MakeCompletionID(completionID string) networkid.MessageID {
	return networkid.MessageID(fmt.Sprintf("openai:%s", completionID))
}

// MakeModelUserID creates a user ID for an AI model ghost
// Format: "model-{escaped-model-id}"
func MakeModelUserID(modelID string) networkid.UserID {
	return networkid.UserID(fmt.Sprintf("model-%s", url.PathEscape(modelID)))
}

// MakeHumanUserID creates a user ID for the human user
func MakeHumanUserID(loginID networkid.UserLoginID) networkid.UserID {
	return networkid.UserID(fmt.Sprintf("openai-user:%s", loginID))
}

// ParseModelFromGhostID extracts the model ID from a ghost ID (format: "model-{escaped-model-id}")
// Returns empty string if the ghost ID doesn't match the expected format.
func ParseModelFromGhostID(ghostID string) string {
	if suffix, ok := strings.CutPrefix(ghostID, "model-"); ok {
		modelID, err := url.PathUnescape(suffix)
		if err == nil {
			return modelID
		}
	}
	return ""
}

// ParsePortalID extracts the login ID and chat ID from a portal ID
// Format: "openai:{loginID}:{chatID}"
func ParsePortalID(portalID networkid.PortalID) (loginID, chatID string) {
	parts := strings.SplitN(string(portalID), ":", 3)
	if len(parts) == 3 && parts[0] == "openai" {
		return parts[1], parts[2]
	}
	return "", ""
}

// FormatChatSlug creates a chat slug from an index
func FormatChatSlug(index int) string {
	return fmt.Sprintf("chat-%d", index)
}

// ParseChatSlug extracts the index from a chat slug
func ParseChatSlug(slug string) int {
	if suffix, ok := strings.CutPrefix(slug, "chat-"); ok {
		if idx, err := strconv.Atoi(suffix); err == nil {
			return idx
		}
	}
	return 0
}
