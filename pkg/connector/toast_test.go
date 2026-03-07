package connector

import (
	"reflect"
	"testing"
	"time"

	"maunium.net/go/mautrix/id"
)

func TestBuildApprovalSnapshotUIMessage_OutputDeniedUsesOriginalToolCallID(t *testing.T) {
	uiMessage := buildApprovalSnapshotUIMessage("approval-1", "call-1", "message", "turn-1", "output-denied", "Denied")

	metadata, ok := uiMessage["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata map, got %#v", uiMessage["metadata"])
	}
	if metadata["approvalId"] != "approval-1" {
		t.Fatalf("expected approvalId metadata, got %#v", metadata["approvalId"])
	}
	if metadata["turn_id"] != "turn-1" {
		t.Fatalf("expected turn_id metadata, got %#v", metadata["turn_id"])
	}

	parts, ok := uiMessage["parts"].([]map[string]any)
	if !ok || len(parts) != 1 {
		t.Fatalf("expected single typed part, got %#v", uiMessage["parts"])
	}
	part := parts[0]
	if part["toolCallId"] != "call-1" {
		t.Fatalf("expected rejection snapshot to keep original toolCallId, got %#v", part["toolCallId"])
	}
	if part["toolName"] != "message" {
		t.Fatalf("expected toolName to be preserved, got %#v", part["toolName"])
	}
	if part["state"] != "output-denied" {
		t.Fatalf("expected output-denied state, got %#v", part["state"])
	}
	if part["errorText"] != "Denied" {
		t.Fatalf("expected denial error text, got %#v", part["errorText"])
	}

	approval, ok := part["approval"].(map[string]any)
	if !ok {
		t.Fatalf("expected approval payload, got %#v", part["approval"])
	}
	if approval["approved"] != false {
		t.Fatalf("expected approved=false, got %#v", approval["approved"])
	}
	if approval["reason"] != "Denied" {
		t.Fatalf("expected denial reason, got %#v", approval["reason"])
	}
}

func TestBuildApprovalSnapshotPart_PreservesCanonicalEnvelope(t *testing.T) {
	uiMessage := buildApprovalSnapshotUIMessage("approval-1", "call-1", "message", "turn-1", "output-denied", "Denied")
	part := buildApprovalSnapshotPart("Approval denied", uiMessage, "Approval denied", id.EventID("$reply"))

	if part.Content == nil || part.Content.Body != "Approval denied" {
		t.Fatalf("expected notice content body, got %#v", part.Content)
	}
	if part.DBMetadata == nil {
		t.Fatal("expected DB metadata")
	}

	meta, ok := part.DBMetadata.(*MessageMetadata)
	if !ok {
		t.Fatalf("expected MessageMetadata, got %T", part.DBMetadata)
	}
	if !meta.ExcludeFromHistory {
		t.Fatal("expected approval snapshot to be excluded from history")
	}
	if meta.CanonicalSchema != "ai-sdk-ui-message-v1" {
		t.Fatalf("expected canonical schema, got %q", meta.CanonicalSchema)
	}
	if !reflect.DeepEqual(meta.CanonicalUIMessage, uiMessage) {
		t.Fatalf("expected canonical UI message to match snapshot")
	}

	raw := part.Extra
	if raw["body"] != "Approval denied" {
		t.Fatalf("expected body in raw content, got %#v", raw["body"])
	}
	if _, ok := raw["com.beeper.ai.toast"].(map[string]any); !ok {
		t.Fatalf("expected toast metadata, got %#v", raw["com.beeper.ai.toast"])
	}
	if !reflect.DeepEqual(raw[BeeperAIKey], uiMessage) {
		t.Fatalf("expected raw UI message to match snapshot")
	}
}

func TestLookupApprovalSnapshotInfo_UsesPendingApprovalData(t *testing.T) {
	oc := newTestAIClient(id.UserID("@owner:example.com"))
	oc.registerToolApproval(ToolApprovalParams{
		ApprovalID:   "approval-1",
		RoomID:       id.RoomID("!room:example.com"),
		TurnID:       "turn-1",
		ToolCallID:   "call-1",
		ToolName:     "message",
		ToolKind:     ToolApprovalKindBuiltin,
		RuleToolName: "message",
		Action:       "send",
		TTL:          time.Second,
	})

	toolCallID, toolName, turnID := oc.lookupApprovalSnapshotInfo("approval-1")
	if toolCallID != "call-1" || toolName != "message" || turnID != "turn-1" {
		t.Fatalf("unexpected approval snapshot info: toolCallID=%q toolName=%q turnID=%q", toolCallID, toolName, turnID)
	}
}
