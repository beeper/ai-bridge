package sdk

// SessionAs extracts a typed session from a Conversation. Returns a zero-value
// pointer if the session is nil or not of the expected type.
func SessionAs[T any](conv *Conversation) *T {
	if conv == nil {
		return new(T)
	}
	raw := conv.Session()
	if raw == nil {
		return new(T)
	}
	if typed, ok := raw.(*T); ok && typed != nil {
		return typed
	}
	return new(T)
}
