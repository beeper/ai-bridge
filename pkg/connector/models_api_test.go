package connector

import "testing"

func TestResolveAlias_StrictTrimOnly(t *testing.T) {
	in := " anthropic/claude-opus-4-20250514 "
	got := ResolveAlias(in)
	if got != "anthropic/claude-opus-4-20250514" {
		t.Fatalf("unexpected alias resolution: got %q", got)
	}
}
