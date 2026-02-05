package connector

import "strings"

const systemMark = "⚙️"

func formatSystemAck(text string) string {
	if text == "" {
		return text
	}
	if strings.HasPrefix(text, systemMark) {
		return text
	}
	return systemMark + " " + text
}
