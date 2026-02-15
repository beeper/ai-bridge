package connectorutil

import (
	"strings"
	"time"
	"unicode/utf8"

	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/event"
)

func PtrIfNotEmpty(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return ptr.Ptr(trimmed)
}

func MatrixEventTimestamp(evt *event.Event) time.Time {
	if evt == nil {
		return time.Now()
	}
	if evt.Timestamp > 0 {
		return time.UnixMilli(evt.Timestamp)
	}
	return time.Now()
}

func ReadStringArgAny(raw map[string]any, key string) string {
	v, ok := raw[key]
	if !ok || v == nil {
		return ""
	}
	switch value := v.(type) {
	case string:
		return value
	default:
		return ""
	}
}

func ReadStringArg(raw map[string]any, key string) (string, bool) {
	value := strings.TrimSpace(ReadStringArgAny(raw, key))
	return value, value != ""
}

func SplitAtMarkdownBoundary(text string, maxBytes int) (string, string) {
	if len(text) <= maxBytes || maxBytes <= 0 {
		return text, ""
	}
	cut := maxBytes
	for cut > 0 && !utf8.ValidString(text[:cut]) {
		cut--
	}
	if cut <= 0 {
		return "", text
	}
	split := strings.LastIndexAny(text[:cut], "\n\t ")
	if split <= 0 {
		split = cut
	}
	first := strings.TrimSpace(text[:split])
	rest := strings.TrimSpace(text[split:])
	return first, rest
}
