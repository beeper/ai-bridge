package connector

import (
	"fmt"
	"strconv"
	"strings"
)

// ToolStrictMode controls whether OpenAI strict mode is used for tool schemas.
type ToolStrictMode string

const (
	ToolStrictModeOff ToolStrictMode = "off"
	ToolStrictModeOn  ToolStrictMode = "on"
)

func resolveToolStrictMode(openRouter bool) ToolStrictMode {
	if openRouter {
		return ToolStrictModeOff
	}
	return ToolStrictModeOn
}

func shouldUseStrictMode(mode ToolStrictMode, _ map[string]any) bool {
	return mode == ToolStrictModeOn
}

// appendMessageIDHint is a no-op in the simple bridge.
// Agentic bridges override to embed message ID hints in prompts.
func appendMessageIDHint(text string, _ any) string { return text }

// stripMessageIDHintLines is a no-op in the simple bridge.
func stripMessageIDHintLines(text string) string { return text }

func parsePositiveInt(value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, fmt.Errorf("value must be positive")
	}
	return n, nil
}

func isAllowedValue(value string, allowed map[string]bool) bool {
	_, ok := allowed[strings.TrimSpace(value)]
	return ok
}

func sanitizeToolSchemaWithReport(schema map[string]any) (map[string]any, []string) {
	return schema, nil
}

func logSchemaSanitization(_ any, _ string, _ []string) {}
