package connector

import (
	"strings"

	"maunium.net/go/mautrix/id"
)

// appendMessageIDHint appends a message_id hint on a new line if one isn't already present.
func appendMessageIDHint(body string, mxid id.EventID) string {
	if mxid == "" || body == "" {
		return body
	}

	trimmed := strings.TrimRight(body, " \t\r\n")
	if trimmed == "" {
		return body
	}

	lastLine := trimmed
	if idx := strings.LastIndex(trimmed, "\n"); idx >= 0 {
		lastLine = trimmed[idx+1:]
	}
	line := strings.TrimSpace(lastLine)
	if strings.HasPrefix(strings.ToLower(line), "[message_id:") && strings.HasSuffix(line, "]") {
		return body
	}

	sep := "\n"
	if strings.HasSuffix(body, "\n") {
		sep = ""
	}
	return body + sep + "[message_id: " + string(mxid) + "]"
}
