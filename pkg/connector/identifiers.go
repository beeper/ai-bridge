package connector

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

func makeUserLoginID(mxid id.UserID) networkid.UserLoginID {
	escaped := url.PathEscape(string(mxid))
	return networkid.UserLoginID(fmt.Sprintf("openai:%s", escaped))
}

func portalKeyForChat(loginID networkid.UserLoginID, slug string) networkid.PortalKey {
	return networkid.PortalKey{
		ID:       networkid.PortalID(fmt.Sprintf("openai:%s:%s", loginID, slug)),
		Receiver: loginID,
	}
}

func assistantUserID(loginID networkid.UserLoginID) networkid.UserID {
	return networkid.UserID(fmt.Sprintf("openai-assistant:%s", loginID))
}

func modelUserID(modelID string) networkid.UserID {
	// Convert "gpt-4o" to "model-gpt-4o"
	return networkid.UserID(fmt.Sprintf("model-%s", url.PathEscape(modelID)))
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
	if strings.HasPrefix(slug, "chat-") {
		if idx, err := strconv.Atoi(strings.TrimPrefix(slug, "chat-")); err == nil {
			return idx
		}
	}
	return 0
}
