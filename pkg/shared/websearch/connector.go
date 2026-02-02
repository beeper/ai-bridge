package websearch

import (
	"fmt"
	"strings"
)

// ConnectorError maps web search errors to the legacy connector error messages.
func ConnectorError(err error) error {
	if err == nil {
		return nil
	}

	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "failed to create request:"):
		return err
	case strings.HasPrefix(msg, "failed to parse results:"):
		trimmed := strings.TrimPrefix(msg, "failed to parse results: ")
		return fmt.Errorf("failed to parse search results: %s", trimmed)
	case strings.HasPrefix(msg, "status "):
		return fmt.Errorf("web search failed: %s", msg)
	case strings.HasPrefix(msg, "request failed:"):
		trimmed := strings.TrimPrefix(msg, "request failed: ")
		return fmt.Errorf("web search failed: %s", trimmed)
	default:
		return fmt.Errorf("web search failed: %s", msg)
	}
}
