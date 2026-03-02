package streamtransport

import (
	"strings"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"
)

// DebouncedEditContent is the rendered content for a debounced streaming edit.
type DebouncedEditContent struct {
	Body          string
	FormattedBody string
	Format        event.Format
}

// DebouncedEditParams holds the inputs needed by BuildDebouncedEditContent.
type DebouncedEditParams struct {
	PortalMXID     id.RoomID
	Force          bool
	SuppressSend   bool
	VisibleBody    string
	FallbackBody   string
	InitialEventID id.EventID
}

// BuildDebouncedEditContent validates inputs and renders the edit content.
// Returns nil if the edit should be skipped.
func BuildDebouncedEditContent(p DebouncedEditParams) *DebouncedEditContent {
	if p.PortalMXID == "" || p.InitialEventID == "" {
		return nil
	}
	if p.SuppressSend {
		return nil
	}
	body := strings.TrimSpace(p.VisibleBody)
	if body == "" {
		body = strings.TrimSpace(p.FallbackBody)
	}
	if body == "" {
		return nil
	}
	if !p.Force {
		return nil
	}
	rendered := format.RenderMarkdown(body, true, true)
	return &DebouncedEditContent{
		Body:          rendered.Body,
		FormattedBody: rendered.FormattedBody,
		Format:        rendered.Format,
	}
}
