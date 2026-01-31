package connector

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/rs/xid"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

func makeUserLoginID(mxid id.UserID) networkid.UserLoginID {
	escaped := url.PathEscape(string(mxid))
	return networkid.UserLoginID(fmt.Sprintf("openai:%s", escaped))
}

func portalKeyForChat(loginID networkid.UserLoginID) networkid.PortalKey {
	return networkid.PortalKey{
		ID:       networkid.PortalID(fmt.Sprintf("openai:%s:%s", loginID, xid.New().String())),
		Receiver: loginID,
	}
}

func modelUserID(modelID string) networkid.UserID {
	// Convert "gpt-4o" to "model-gpt-4o"
	return networkid.UserID(fmt.Sprintf("model-%s", url.PathEscape(modelID)))
}

// parseModelFromGhostID extracts the model ID from a ghost ID (format: "model-{escaped-model-id}")
// Returns empty string if the ghost ID doesn't match the expected format.
func parseModelFromGhostID(ghostID string) string {
	if suffix, ok := strings.CutPrefix(ghostID, "model-"); ok {
		modelID, err := url.PathUnescape(suffix)
		if err == nil {
			return modelID
		}
	}
	return ""
}

func humanUserID(loginID networkid.UserLoginID) networkid.UserID {
	return networkid.UserID(fmt.Sprintf("openai-user:%s", loginID))
}

func portalMeta(portal *bridgev2.Portal) *PortalMetadata {
	if portal.Metadata == nil {
		meta := &PortalMetadata{}
		portal.Metadata = meta
		return meta
	}
	if typed, ok := portal.Metadata.(*PortalMetadata); ok {
		return typed
	}
	meta := &PortalMetadata{}
	portal.Metadata = meta
	return meta
}

func messageMeta(msg *database.Message) *MessageMetadata {
	if msg == nil {
		return nil
	}
	if meta, ok := msg.Metadata.(*MessageMetadata); ok {
		return meta
	}
	return nil
}

// shouldIncludeInHistory checks if a message should be included in LLM history.
// Filters out commands (messages starting with /) and non-conversation messages.
func shouldIncludeInHistory(meta *MessageMetadata) bool {
	if meta == nil || meta.Body == "" {
		return false
	}
	// Skip command messages
	if strings.HasPrefix(meta.Body, "/") {
		return false
	}
	// Only include user and assistant messages
	if meta.Role != "user" && meta.Role != "assistant" {
		return false
	}
	return true
}

func loginMetadata(login *bridgev2.UserLogin) *UserLoginMetadata {
	meta, ok := login.Metadata.(*UserLoginMetadata)
	if !ok || meta == nil {
		meta = &UserLoginMetadata{}
		login.Metadata = meta
	}
	return meta
}

func formatChatSlug(index int) string {
	return fmt.Sprintf("chat-%d", index)
}

func parseChatSlug(slug string) int {
	if suffix, ok := strings.CutPrefix(slug, "chat-"); ok {
		if idx, err := strconv.Atoi(suffix); err == nil {
			return idx
		}
	}
	return 0
}
