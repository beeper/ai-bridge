package sdk

import (
	"maunium.net/go/mautrix/bridgev2"
)

// LoginMeta extracts or initializes typed metadata from a UserLogin.
func LoginMeta[T any](login *bridgev2.UserLogin) *T {
	if login == nil {
		return new(T)
	}
	if meta, ok := login.Metadata.(*T); ok && meta != nil {
		return meta
	}
	meta := new(T)
	login.Metadata = meta
	return meta
}

// PortalMeta extracts or initializes typed metadata from a Portal.
func PortalMeta[T any](portal *bridgev2.Portal) *T {
	if portal == nil {
		return new(T)
	}
	if meta, ok := portal.Metadata.(*T); ok && meta != nil {
		return meta
	}
	meta := new(T)
	portal.Metadata = meta
	return meta
}

// GhostMeta extracts or initializes typed metadata from a Ghost.
func GhostMeta[T any](ghost *bridgev2.Ghost) *T {
	if ghost == nil {
		return new(T)
	}
	if meta, ok := ghost.Metadata.(*T); ok && meta != nil {
		return meta
	}
	meta := new(T)
	ghost.Metadata = meta
	return meta
}
