package codex

import "testing"

func TestIsCodexIdentifier(t *testing.T) {
	valid := []string{"codex", "@codex", "codex:default", "codex:codex", "codex"}
	for _, tc := range valid {
		if !isCodexIdentifier(tc) {
			t.Fatalf("expected %q to resolve as Codex identifier", tc)
		}
	}

	invalid := []string{"", "gpt-5", "opencode", "codex:workspace"}
	for _, tc := range invalid {
		if isCodexIdentifier(tc) {
			t.Fatalf("expected %q to be rejected", tc)
		}
	}
}
