package stringutil

import "strings"

// SplitCSV splits a comma-separated string into trimmed, non-empty parts.
func SplitCSV(value string) []string {
	parts := strings.Split(value, ",")
	var out []string
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
