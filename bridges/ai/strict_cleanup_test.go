package ai

import "testing"

func TestToolApprovalsAskFallbackAlwaysDenies(t *testing.T) {
	oc := &AIClient{}
	if got := oc.toolApprovalsAskFallback(); got != "deny" {
		t.Fatalf("expected deny fallback, got %q", got)
	}
}

func TestNormalizeModelAPIAcceptsOnlyCanonicalNames(t *testing.T) {
	if got := normalizeModelAPI("responses"); got != ModelAPIResponses {
		t.Fatalf("expected canonical responses API name, got %q", got)
	}
	if got := normalizeModelAPI("openai-responses"); got != "" {
		t.Fatalf("expected legacy alias to be rejected, got %q", got)
	}
}
