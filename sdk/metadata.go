package sdk

import (
	"maunium.net/go/mautrix/bridgev2"

	"github.com/beeper/agentremote"
)

// LoginMeta extracts or initializes typed metadata from a UserLogin.
func LoginMeta[T any](login *bridgev2.UserLogin) *T {
	return agentremote.EnsureLoginMetadata[T](login)
}

// PortalMeta extracts or initializes typed metadata from a Portal.
func PortalMeta[T any](portal *bridgev2.Portal) *T {
	return agentremote.EnsurePortalMetadata[T](portal)
}

// GhostMeta extracts or initializes typed metadata from a Ghost.
func GhostMeta[T any](ghost *bridgev2.Ghost) *T {
	return agentremote.EnsureGhostMetadata[T](ghost)
}

// SessionAs extracts a typed session from a Conversation. Returns a zero-value
// pointer if the session is nil or not of the expected type.
func SessionAs[T any](conv *Conversation) *T {
	if conv == nil {
		return new(T)
	}
	raw := conv.Session()
	if raw == nil {
		return new(T)
	}
	if typed, ok := raw.(*T); ok && typed != nil {
		return typed
	}
	return new(T)
}
