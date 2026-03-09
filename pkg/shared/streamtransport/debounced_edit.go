package streamtransport

import (
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
)

// DebouncedEditContent is the rendered content for a debounced streaming edit.
type DebouncedEditContent struct {
	Body          string
	FormattedBody string
	Format        event.Format
}

// DebouncedEditParams holds the inputs needed by BuildDebouncedEditContent.
type DebouncedEditParams struct {
	PortalMXID   string
	Force        bool
	SuppressSend bool
	VisibleBody  string
	FallbackBody string
}

// BuildDebouncedEditContent validates inputs and renders the edit content.
// Returns nil if the edit should be skipped.
func BuildDebouncedEditContent(p DebouncedEditParams) *DebouncedEditContent {
	if strings.TrimSpace(p.PortalMXID) == "" {
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

// BuildConvertedEdit wraps rendered message content into a standard Matrix edit.
func BuildConvertedEdit(content *event.MessageEventContent, topLevelExtra map[string]any) *bridgev2.ConvertedEdit {
	if content == nil {
		return nil
	}
	if topLevelExtra == nil {
		topLevelExtra = map[string]any{}
	}
	if _, ok := topLevelExtra["body"]; !ok {
		topLevelExtra["body"] = content.Body
	}
	if content.Format != "" {
		if _, ok := topLevelExtra["format"]; !ok {
			topLevelExtra["format"] = content.Format
		}
		if _, ok := topLevelExtra["formatted_body"]; !ok {
			topLevelExtra["formatted_body"] = content.FormattedBody
		}
	}
	return &bridgev2.ConvertedEdit{
		ModifiedParts: []*bridgev2.ConvertedEditPart{{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType:       content.MsgType,
				Body:          content.Body,
				Format:        content.Format,
				FormattedBody: content.FormattedBody,
			},
			Extra:         map[string]any{"m.mentions": map[string]any{}},
			TopLevelExtra: topLevelExtra,
		}},
	}
}
