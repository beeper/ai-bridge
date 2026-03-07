package msgconv

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestAppendUIMessageArtifacts_PreservesProgrammaticParts(t *testing.T) {
	uiMessage := BuildUIMessage(UIMessageParams{
		TurnID: "turn-1",
		Role:   "assistant",
		Parts: []map[string]any{
			{"type": "text", "text": "hello"},
			{"type": "source-url", "url": "https://example.com/existing"},
		},
	})

	updated := AppendUIMessageArtifacts(
		uiMessage,
		[]map[string]any{
			{"type": "source-url", "url": "https://example.com/existing"},
			{"type": "source-url", "url": "https://example.com/new"},
		},
		[]map[string]any{
			{"type": "file", "url": "mxc://example.org/file"},
		},
	)

	parts := normalizeUIParts(updated["parts"])
	if len(parts) != 4 {
		t.Fatalf("expected original parts plus two unique artifacts, got %#v", parts)
	}
	if parts[0]["type"] != "text" || parts[1]["url"] != "https://example.com/existing" {
		t.Fatalf("expected original programmatic parts to be preserved, got %#v", parts)
	}
	if parts[2]["url"] != "https://example.com/new" {
		t.Fatalf("expected new source artifact to be appended, got %#v", parts[2])
	}
	if parts[3]["url"] != "mxc://example.org/file" {
		t.Fatalf("expected file artifact to be appended, got %#v", parts[3])
	}
}

func TestArtifactPartKey_UnknownTypeIncludesPayload(t *testing.T) {
	keyA := artifactPartKey(map[string]any{"type": "custom", "text": "first"})
	keyB := artifactPartKey(map[string]any{"type": "custom", "text": "second"})
	if keyA == keyB {
		t.Fatalf("expected distinct keys for distinct unknown parts, got %q", keyA)
	}
}

func TestRelatesToReplaceRequiresInitialEventID(t *testing.T) {
	rel := RelatesToReplace("", id.EventID("$reply"))
	if rel != nil {
		t.Fatalf("expected nil relates_to when initial event id is missing, got %#v", rel)
	}
}
