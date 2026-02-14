package toolspec

import "testing"

func TestMessageSchemaRemovesLegacyAliasProperties(t *testing.T) {
	schema := MessageSchema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("message schema properties missing")
	}

	legacyKeys := []string{
		"effectId",
		"messageId",
		"replyTo",
		"threadId",
		"filePath",
		"contentType",
		"chatID",
		"title",
		"description",
	}
	for _, key := range legacyKeys {
		if _, exists := props[key]; exists {
			t.Fatalf("expected legacy property %q to be removed", key)
		}
	}
}

func TestMessageSchemaRemovesLegacyAliasActions(t *testing.T) {
	schema := MessageSchema()
	props := schema["properties"].(map[string]any)
	actionDef := props["action"].(map[string]any)
	rawEnum := actionDef["enum"].([]string)

	actions := make(map[string]struct{}, len(rawEnum))
	for _, action := range rawEnum {
		actions[action] = struct{}{}
	}

	legacyActions := []string{"unsend", "open", "select", "broadcast", "sendWithEffect"}
	for _, action := range legacyActions {
		if _, exists := actions[action]; exists {
			t.Fatalf("expected legacy action %q to be removed", action)
		}
	}
}
