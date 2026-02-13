package connector

import "context"

// File-backed identity sync is not part of the simple bridge tool surface.
func maybeRefreshAgentIdentity(context.Context, string) {}
