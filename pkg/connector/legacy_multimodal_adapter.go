package connector

func legacyUnifiedMessagesNeedChatAdapter(messages []UnifiedMessage) bool {
	for _, msg := range messages {
		for _, part := range msg.Content {
			switch part.Type {
			case ContentTypeAudio, ContentTypeVideo:
				return true
			}
		}
	}
	return false
}
