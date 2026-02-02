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

// agentModelUserID creates a ghost user ID for an agent+model combination.
// Format: "agent-{agent-id}:model-{model-id}"
// This allows different model variants of the same agent to have different ghosts.
func agentModelUserID(agentID, modelID string) networkid.UserID {
	return networkid.UserID(fmt.Sprintf("agent-%s:model-%s", url.PathEscape(agentID), url.PathEscape(modelID)))
}

// parseAgentModelFromGhostID extracts agent and model IDs from a composite ghost ID.
// Returns agentID, modelID, and true if successful.
func parseAgentModelFromGhostID(ghostID string) (agentID, modelID string, ok bool) {
	parts := strings.SplitN(ghostID, ":model-", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	agentPart := parts[0]
	modelPart := parts[1]

	if suffix, hasPrefix := strings.CutPrefix(agentPart, "agent-"); hasPrefix {
		agentID, err1 := url.PathUnescape(suffix)
		modelID, err2 := url.PathUnescape(modelPart)
		if err1 == nil && err2 == nil {
			return agentID, modelID, true
		}
	}
	return "", "", false
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
// Filters out commands (messages starting with /), non-conversation messages,
// and messages explicitly excluded (e.g., welcome messages).
func shouldIncludeInHistory(meta *MessageMetadata) bool {
	if meta == nil || meta.Body == "" {
		return false
	}
	// Skip messages explicitly excluded (welcome messages, etc.)
	if meta.ExcludeFromHistory {
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

// agentDataPortalKey creates a deterministic portal key for an agent's hidden data room.
// Format: "openai:{loginID}:agent-data:{agentID}"
func agentDataPortalKey(loginID networkid.UserLoginID, agentID string) networkid.PortalKey {
	return networkid.PortalKey{
		ID:       networkid.PortalID(fmt.Sprintf("openai:%s:agent-data:%s", loginID, url.PathEscape(agentID))),
		Receiver: loginID,
	}
}

// parseAgentIDFromDataRoom extracts the agent ID from an agent data room portal ID.
// Returns the agent ID and true if successful, empty string and false otherwise.
func parseAgentIDFromDataRoom(portalID networkid.PortalID) (string, bool) {
	parts := strings.Split(string(portalID), ":agent-data:")
	if len(parts) != 2 {
		return "", false
	}
	agentID, err := url.PathUnescape(parts[1])
	if err != nil {
		return "", false
	}
	return agentID, true
}

// globalMemoryPortalKey creates a deterministic portal key for the global memory room.
// Format: "openai:{loginID}:global-memory"
func globalMemoryPortalKey(loginID networkid.UserLoginID) networkid.PortalKey {
	return networkid.PortalKey{
		ID:       networkid.PortalID(fmt.Sprintf("openai:%s:global-memory", loginID)),
		Receiver: loginID,
	}
}

// isGlobalMemoryRoom checks if a portal ID represents a global memory room.
func isGlobalMemoryRoom(portalID networkid.PortalID) bool {
	return strings.HasSuffix(string(portalID), ":global-memory")
}

// MemoryScope represents where a memory is stored
type MemoryScope string

const (
	MemoryScopeAgent  MemoryScope = "agent"
	MemoryScopeGlobal MemoryScope = "global"
)

// parseMemoryPath parses a memory path into scope and fact ID.
// Supported formats:
//   - "agent:{agentID}/fact:{factID}" → (MemoryScopeAgent, factID, agentID)
//   - "global/fact:{factID}" → (MemoryScopeGlobal, factID, "")
//   - "{factID}" (legacy) → (MemoryScopeAgent, factID, "")
//
// Returns scope, factID, agentID (for agent scope), and ok.
func parseMemoryPath(path string) (scope MemoryScope, factID string, agentID string, ok bool) {
	if path == "" {
		return "", "", "", false
	}

	// Format: "global/fact:{factID}"
	if factID, ok := strings.CutPrefix(path, "global/fact:"); ok {
		if factID == "" {
			return "", "", "", false
		}
		return MemoryScopeGlobal, factID, "", true
	}

	// Format: "agent:{agentID}/fact:{factID}"
	if remainder, ok := strings.CutPrefix(path, "agent:"); ok {
		parts := strings.SplitN(remainder, "/fact:", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", "", false
		}
		agentID, err := url.PathUnescape(parts[0])
		if err != nil {
			return "", "", "", false
		}
		return MemoryScopeAgent, parts[1], agentID, true
	}

	// Legacy format: just a fact ID (assume agent scope)
	return MemoryScopeAgent, path, "", true
}

// formatMemoryPath creates a memory path from scope, factID, and optional agentID.
func formatMemoryPath(scope MemoryScope, factID string, agentID string) string {
	switch scope {
	case MemoryScopeGlobal:
		return "global/fact:" + factID
	case MemoryScopeAgent:
		if agentID != "" {
			return "agent:" + url.PathEscape(agentID) + "/fact:" + factID
		}
		// Default agent scope without explicit agent ID
		return "agent/fact:" + factID
	default:
		return factID
	}
}
