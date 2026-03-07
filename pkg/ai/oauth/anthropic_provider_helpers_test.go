package oauth

import (
	"net/url"
	"testing"
	"time"
)

func TestAnthropicOAuthHelperFunctions(t *testing.T) {
	authURL := BuildAnthropicAuthorizeURL("challenge", "state-verifier")
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("expected valid auth URL, got error: %v", err)
	}
	query := parsed.Query()
	if query.Get("client_id") != AnthropicClientID() {
		t.Fatalf("expected anthropic client id in query")
	}
	if query.Get("code_challenge") != "challenge" {
		t.Fatalf("expected code_challenge in query")
	}
	if query.Get("state") != "state-verifier" {
		t.Fatalf("expected state in query")
	}

	code, state := ParseAnthropicAuthorizationCode("abc123#xyz")
	if code != "abc123" || state != "xyz" {
		t.Fatalf("unexpected parsed code/state: code=%q state=%q", code, state)
	}
	code, state = ParseAnthropicAuthorizationCode("abc123")
	if code != "abc123" || state != "" {
		t.Fatalf("unexpected parsed code/state for no-state input: code=%q state=%q", code, state)
	}

	now := time.UnixMilli(1_700_000_000_000)
	expires := OAuthExpiryWithBuffer(now, 3600)
	expected := now.Add(55 * time.Minute).UnixMilli()
	if expires != expected {
		t.Fatalf("expected expiry with 5m buffer %d, got %d", expected, expires)
	}
}
