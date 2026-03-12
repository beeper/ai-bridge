package ai

import (
	"strings"

	"maunium.net/go/mautrix/bridgev2/database"
)

func ensureCanonicalUserMessage(msg *database.Message) {
	if msg == nil {
		return
	}
	meta, ok := msg.Metadata.(*MessageMetadata)
	if !ok || meta == nil || strings.TrimSpace(meta.Role) != "user" {
		return
	}
	if len(meta.CanonicalPromptMessages) > 0 && meta.CanonicalPromptSchema == canonicalPromptSchemaV1 {
		return
	}

	body := strings.TrimSpace(meta.Body)
	if body != "" {
		meta.CanonicalPromptSchema = canonicalPromptSchemaV1
		meta.CanonicalPromptMessages = encodePromptMessages(textPromptMessage(body))
	}
}
