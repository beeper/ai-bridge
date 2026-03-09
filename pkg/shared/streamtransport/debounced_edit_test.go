package streamtransport

import (
	"testing"

	"maunium.net/go/mautrix/event"
)

func TestBuildDebouncedEditContent_WithoutEventID(t *testing.T) {
	content := BuildDebouncedEditContent(DebouncedEditParams{
		PortalMXID:  "test-room",
		Force:       true,
		VisibleBody: "hello",
	})
	if content == nil {
		t.Fatal("expected debounced edit content without event ID")
	}
	if content.Body == "" {
		t.Fatal("expected non-empty body")
	}
}

func TestBuildConvertedEdit_PopulatesTopLevelRenderedFallback(t *testing.T) {
	edit := BuildConvertedEdit(&event.MessageEventContent{
		MsgType:       event.MsgText,
		Body:          "Hello",
		Format:        event.FormatHTML,
		FormattedBody: "<p>Hello</p>",
	}, map[string]any{
		"com.beeper.dont_render_edited": true,
	})
	if edit == nil || len(edit.ModifiedParts) != 1 {
		t.Fatal("expected single modified part")
	}
	extra := edit.ModifiedParts[0].TopLevelExtra
	if extra["body"] != "Hello" {
		t.Fatalf("expected top-level body fallback, got %#v", extra["body"])
	}
	if extra["format"] != event.FormatHTML {
		t.Fatalf("expected top-level format fallback, got %#v", extra["format"])
	}
	if extra["formatted_body"] != "<p>Hello</p>" {
		t.Fatalf("expected top-level formatted_body fallback, got %#v", extra["formatted_body"])
	}
}
