package msgconv

import (
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func TestRelatesToReplaceRequiresInitialEventID(t *testing.T) {
	rel := RelatesToReplace("", id.EventID("$reply"))
	if rel != nil {
		t.Fatalf("expected nil relates_to when initial event id is missing, got %#v", rel)
	}
}

func TestBuildFinalEditContentOmitsRelatesToWhenNil(t *testing.T) {
	content := BuildFinalEditContent(FinalEditContentParams{
		Rendered: event.MessageEventContent{
			Body: "hello",
		},
		RelatesTo: nil,
		UIMessage: map[string]any{"id": "turn-1", "role": "assistant"},
	})
	if content == nil || content.Raw == nil {
		t.Fatalf("expected content with raw payload")
	}
	if _, ok := content.Raw["m.relates_to"]; ok {
		t.Fatalf("expected m.relates_to to be omitted when not provided")
	}
}
