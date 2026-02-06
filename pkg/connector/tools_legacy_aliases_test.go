package connector

import (
	"context"
	"strings"
	"testing"
)

func TestExecuteMessageRejectsLegacyActionAliases(t *testing.T) {
	ctx := WithBridgeToolContext(context.Background(), &BridgeToolContext{})
	legacyActions := []string{"unsend", "open", "select", "broadcast", "sendWithEffect"}

	for _, action := range legacyActions {
		_, err := executeMessage(ctx, map[string]any{"action": action})
		if err == nil {
			t.Fatalf("expected action %q to be rejected", action)
		}
		if !strings.Contains(err.Error(), "unknown action") {
			t.Fatalf("expected unknown action error for %q, got %v", action, err)
		}
	}
}

func TestExecuteMessageRejectsLegacyMessageIDAliases(t *testing.T) {
	ctx := WithBridgeToolContext(context.Background(), &BridgeToolContext{})
	_, err := executeMessage(ctx, map[string]any{
		"action":    "reply",
		"message":   "hello",
		"messageId": "$legacy:example.com",
	})
	if err == nil {
		t.Fatalf("expected legacy messageId alias to be rejected")
	}
	if !strings.Contains(err.Error(), "requires 'message_id'") {
		t.Fatalf("expected canonical message_id validation error, got %v", err)
	}
}
