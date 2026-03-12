package msgconv

import (
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/agentremote"
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

func TestToolCallPartMarksProviderExecutedAndSuccess(t *testing.T) {
	part := ToolCallPart(agentremote.ToolCallMetadata{
		CallID:       "call-1",
		ToolName:     "search",
		ToolType:     "provider",
		Input:        map[string]any{"q": "golang"},
		Output:       map[string]any{"result": "ok"},
		ResultStatus: "success",
	}, "provider", "success", "denied")

	if got := part["state"]; got != "output-available" {
		t.Fatalf("expected success state, got %#v", got)
	}
	if got := part["providerExecuted"]; got != true {
		t.Fatalf("expected providerExecuted flag, got %#v", got)
	}
}

func TestContentPartsIncludesReasoningAndText(t *testing.T) {
	parts := ContentParts("answer", "thinking")
	if len(parts) != 2 {
		t.Fatalf("expected reasoning and text parts, got %#v", parts)
	}
	if parts[0]["type"] != "reasoning" || parts[1]["type"] != "text" {
		t.Fatalf("expected reasoning followed by text, got %#v", parts)
	}
}

func TestRelatesToThreadFallsBackToReply(t *testing.T) {
	rel := RelatesToThread("", id.EventID("$reply"))
	inReplyTo, ok := rel["m.in_reply_to"].(map[string]any)
	if !ok || inReplyTo["event_id"] != "$reply" {
		t.Fatalf("expected reply fallback, got %#v", rel)
	}
}

func TestConvertAIResponseBuildsConvertedMessage(t *testing.T) {
	converted, err := ConvertAIResponse(AIResponseParams{
		Content:          "hello",
		FormattedContent: "<b>hello</b>",
		ReplyToEventID:   id.EventID("$reply"),
		Metadata: UIMessageMetadataParams{
			TurnID:       "turn-1",
			AgentID:      "agent-1",
			Model:        "gpt-test",
			FinishReason: "stop",
		},
		ThinkingContent: "reasoning",
		ToolCalls: []agentremote.ToolCallMetadata{{
			CallID:       "call-1",
			ToolName:     "search",
			ResultStatus: "success",
			Output:       map[string]any{"result": "ok"},
		}},
		SuccessStatus: "success",
		DBMetadata:    map[string]any{"kind": "assistant"},
	})
	if err != nil {
		t.Fatalf("expected conversion to succeed, got %v", err)
	}
	if converted == nil {
		t.Fatal("expected converted message")
	}
	if converted.ReplyTo != nil {
		t.Fatalf("expected reply relation to live in part extra, got %#v", converted.ReplyTo)
	}
	if len(converted.Parts) == 0 {
		t.Fatalf("expected at least one converted part, got %#v", converted)
	}
	if converted.Parts[0].Content.MsgType != event.MsgText {
		t.Fatalf("expected text message part, got %#v", converted.Parts[0].Content.MsgType)
	}
	if converted.Parts[0].Type != event.EventMessage {
		t.Fatalf("expected message event type, got %#v", converted.Parts[0].Type)
	}
	if _, ok := converted.Parts[0].Extra["m.relates_to"].(map[string]any); !ok {
		t.Fatalf("expected threaded relation in extra, got %#v", converted.Parts[0].Extra)
	}
}
