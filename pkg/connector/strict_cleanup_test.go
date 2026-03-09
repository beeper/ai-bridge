package connector

import "testing"

func TestParseDesktopSessionKeyRejectsLegacyAliasPrefix(t *testing.T) {
	if _, _, ok := parseDesktopSessionKey("desktop:default:chat-1"); ok {
		t.Fatalf("expected legacy desktop: prefix to be rejected")
	}
	instance, chatID, ok := parseDesktopSessionKey("desktop-api:default:chat-1")
	if !ok || instance != "default" || chatID != "chat-1" {
		t.Fatalf("expected canonical desktop-api session key to parse, got ok=%v instance=%q chatID=%q", ok, instance, chatID)
	}
}

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
