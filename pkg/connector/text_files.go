package connector

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"maunium.net/go/mautrix/event"
)

const maxTextFileBytes = 5 * 1024 * 1024

func normalizeMimeType(mimeType string) string {
	lower := strings.ToLower(strings.TrimSpace(mimeType))
	if lower == "" {
		return lower
	}
	if semi := strings.IndexByte(lower, ';'); semi >= 0 {
		return strings.TrimSpace(lower[:semi])
	}
	return lower
}

func textFileMimeTypes() map[string]event.CapabilitySupportLevel {
	return map[string]event.CapabilitySupportLevel{
		"text/plain":                event.CapLevelFullySupported,
		"text/markdown":             event.CapLevelFullySupported,
		"text/csv":                  event.CapLevelFullySupported,
		"text/tab-separated-values": event.CapLevelFullySupported,
		"text/html":                 event.CapLevelFullySupported,
		"application/json":          event.CapLevelFullySupported,
		"application/xml":           event.CapLevelFullySupported,
		"application/xhtml+xml":     event.CapLevelFullySupported,
		"application/x-yaml":        event.CapLevelFullySupported,
		"application/yaml":          event.CapLevelFullySupported,
		"application/toml":          event.CapLevelFullySupported,
		"application/x-toml":        event.CapLevelFullySupported,
		"application/csv":           event.CapLevelFullySupported,
		"application/x-csv":         event.CapLevelFullySupported,
	}
}

func isTextFileMime(mimeType string) bool {
	mimeType = normalizeMimeType(mimeType)
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}
	if strings.HasSuffix(mimeType, "+json") || strings.HasSuffix(mimeType, "+xml") || strings.HasSuffix(mimeType, "+yaml") {
		return true
	}
	_, ok := textFileMimeTypes()[mimeType]
	return ok
}

func decodeBase64String(b64Data string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(b64Data)
	if err == nil {
		return data, nil
	}
	data, err = base64.URLEncoding.DecodeString(b64Data)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}
	return data, nil
}

func trimTextForModel(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if len(text) <= AIMaxTextLength {
		return text, false
	}
	return text[:AIMaxTextLength] + "...", true
}

func (oc *AIClient) downloadTextFile(ctx context.Context, mediaURL string, encryptedFile *event.EncryptedFileInfo, mimeType string) (string, bool, error) {
	b64Data, _, err := oc.downloadMediaBase64(ctx, mediaURL, encryptedFile, maxTextFileBytes/(1024*1024), mimeType)
	if err != nil {
		return "", false, err
	}
	data, err := decodeBase64String(b64Data)
	if err != nil {
		return "", false, err
	}
	if len(data) > maxTextFileBytes {
		return "", false, fmt.Errorf("file too large (%d bytes)", len(data))
	}
	if !utf8.Valid(data) {
		return "", false, fmt.Errorf("file is not valid UTF-8 text")
	}
	trimmed, truncated := trimTextForModel(string(data))
	return trimmed, truncated, nil
}

func buildTextFileMessage(caption string, hasUserCaption bool, filename string, mimeType string, content string, truncated bool) string {
	var b strings.Builder
	if hasUserCaption {
		caption = strings.TrimSpace(caption)
		if caption != "" {
			b.WriteString(caption)
			b.WriteString("\n\n")
		}
	}
	if filename != "" {
		b.WriteString("File: ")
		b.WriteString(filename)
		b.WriteString("\n")
	}
	if mimeType != "" {
		b.WriteString("MIME: ")
		b.WriteString(mimeType)
		b.WriteString("\n")
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	if truncated {
		b.WriteString("Content (truncated):\n")
	} else {
		b.WriteString("Content:\n")
	}
	b.WriteString(content)
	return strings.TrimSpace(b.String())
}
