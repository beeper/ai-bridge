package oauth

import (
	"net/url"
	"testing"
)

func TestGoogleAntigravityOAuthHelpers(t *testing.T) {
	authURL := BuildAntigravityAuthorizeURL("challenge", "state")
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("expected valid antigravity auth URL, got err=%v", err)
	}
	query := parsed.Query()
	if query.Get("client_id") != AntigravityClientID() {
		t.Fatalf("expected antigravity client id in query")
	}
	if query.Get("redirect_uri") != AntigravityRedirectURI() {
		t.Fatalf("expected antigravity redirect uri in query")
	}
	if query.Get("code_challenge") != "challenge" || query.Get("state") != "state" {
		t.Fatalf("expected challenge/state in antigravity auth query")
	}

	if got := ResolveAntigravityProjectID(nil); got != AntigravityDefaultProjectID() {
		t.Fatalf("expected default project for nil payload, got %q", got)
	}
	if got := ResolveAntigravityProjectID(map[string]any{"cloudaicompanionProject": "explicit-project"}); got != "explicit-project" {
		t.Fatalf("expected explicit string project id, got %q", got)
	}
	if got := ResolveAntigravityProjectID(map[string]any{"cloudaicompanionProject": map[string]any{"id": "nested-project"}}); got != "nested-project" {
		t.Fatalf("expected nested project id, got %q", got)
	}
	if got := ResolveAntigravityProjectID(map[string]any{"cloudaicompanionProject": map[string]any{"id": ""}}); got != AntigravityDefaultProjectID() {
		t.Fatalf("expected default project when nested id empty, got %q", got)
	}
}
