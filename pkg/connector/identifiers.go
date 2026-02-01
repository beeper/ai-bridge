package connector

import (
	"crypto/sha256"
	"encoding/hex"
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

// makeUserLoginID creates a unique login ID for each account.
// The ID includes the provider and a hash of the API key to ensure
// multiple accounts of the same provider have distinct login IDs.
func makeUserLoginID(mxid id.UserID, provider, apiKey string) networkid.UserLoginID {
	escaped := url.PathEscape(string(mxid))
	// Hash the API key to create unique but stable identifier per account
	keyHash := sha256.Sum256([]byte(apiKey))
	keyHashShort := hex.EncodeToString(keyHash[:8]) // First 8 bytes = 16 hex chars
	return networkid.UserLoginID(fmt.Sprintf("openai:%s:%s:%s", escaped, provider, keyHashShort))
}

func portalKeyForChat(loginID networkid.UserLoginID) networkid.PortalKey {
	return networkid.PortalKey{
		ID:       networkid.PortalID(fmt.Sprintf("openai:%s:%s", loginID, xid.New().String())),
		Receiver: loginID,
	}
}

func portalKeyForDefaultChat(loginID networkid.UserLoginID) networkid.PortalKey {
	return networkid.PortalKey{
		ID:       networkid.PortalID(fmt.Sprintf("openai:%s:default", loginID)),
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

// agentUserID creates a ghost user ID for an agent.
// Format: "agent-{agent-id}"
func agentUserID(agentID string) networkid.UserID {
	return networkid.UserID(fmt.Sprintf("agent-%s", url.PathEscape(agentID)))
}

// parseAgentFromGhostID extracts the agent ID from a ghost ID (format: "agent-{escaped-agent-id}")
// Returns the agent ID and true if successful, empty string and false otherwise.
func parseAgentFromGhostID(ghostID string) (string, bool) {
	if suffix, ok := strings.CutPrefix(ghostID, "agent-"); ok {
		agentID, err := url.PathUnescape(suffix)
		if err == nil {
			return agentID, true
		}
	}
	return "", false
}

func humanUserID(loginID networkid.UserLoginID) networkid.UserID {
	return networkid.UserID(fmt.Sprintf("openai-user:%s", loginID))
}

func portalMeta(portal *bridgev2.Portal) *PortalMetadata {
	return portal.Metadata.(*PortalMetadata)
}

func messageMeta(msg *database.Message) *MessageMetadata {
	if msg == nil || msg.Metadata == nil {
		return nil
	}
	return msg.Metadata.(*MessageMetadata)
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
	return login.Metadata.(*UserLoginMetadata)
}

func formatChatSlug(index int) string {
	return fmt.Sprintf("chat-%d", index)
}

func parseChatSlug(slug string) (int, bool) {
	if suffix, ok := strings.CutPrefix(slug, "chat-"); ok {
		if idx, err := strconv.Atoi(suffix); err == nil {
			return idx, true
		}
	}
	return 0, false
}

// MakeMessageID creates a message ID from a Matrix event ID
func MakeMessageID(eventID id.EventID) networkid.MessageID {
	return networkid.MessageID(fmt.Sprintf("mx:%s", string(eventID)))
}
