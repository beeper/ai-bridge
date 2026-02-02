package connector

import (
	"testing"

	"github.com/beeper/ai-bridge/pkg/shared/toolspec"
)

func TestDefaultToolsConfigIncludesCoreTools(t *testing.T) {
	cfg := getDefaultToolsConfig("")

	want := []string{
		toolspec.CalculatorName,
		toolspec.WebSearchName,
		toolspec.WebFetchName,
		toolspec.SetChatInfoName,
		toolspec.SessionStatusName,
		toolspec.MemorySearchName,
		toolspec.MemoryGetName,
		toolspec.MemoryStoreName,
		toolspec.MemoryForgetName,
	}

	for _, name := range want {
		if _, ok := cfg.Tools[name]; !ok {
			t.Fatalf("expected tool %q in default config", name)
		}
	}
}
