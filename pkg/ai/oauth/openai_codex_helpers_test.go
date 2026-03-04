package oauth

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"testing"
)

func TestOpenAICodexOAuthHelpers(t *testing.T) {
	authURL := BuildOpenAICodexAuthorizeURL("challenge", "state", "pi")
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("expected valid codex auth URL, got err=%v", err)
	}
	query := parsed.Query()
	if query.Get("client_id") != OpenAICodexClientID() {
		t.Fatalf("expected codex client id in query")
	}
	if query.Get("code_challenge") != "challenge" || query.Get("state") != "state" {
		t.Fatalf("expected challenge/state in codex auth query")
	}
	if query.Get("originator") != "pi" {
		t.Fatalf("expected originator=pi in query")
	}

	code, state := ParseOpenAICodexAuthorizationInput("http://localhost:1455/auth/callback?code=abc&state=xyz")
	if code != "abc" || state != "xyz" {
		t.Fatalf("expected URL parser code/state, got code=%q state=%q", code, state)
	}
	code, state = ParseOpenAICodexAuthorizationInput("abc#xyz")
	if code != "abc" || state != "xyz" {
		t.Fatalf("expected hash parser code/state, got code=%q state=%q", code, state)
	}
	code, state = ParseOpenAICodexAuthorizationInput("code=abc&state=xyz")
	if code != "abc" || state != "xyz" {
		t.Fatalf("expected query-string parser code/state, got code=%q state=%q", code, state)
	}

	payload := map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct_123",
		},
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal jwt payload: %v", err)
	}
	jwt := "header." + base64.RawURLEncoding.EncodeToString(rawPayload) + ".sig"
	if got := ExtractOpenAICodexAccountID(jwt); got != "acct_123" {
		t.Fatalf("expected extracted account id acct_123, got %q", got)
	}
	if got := ExtractOpenAICodexAccountID("invalid-token"); got != "" {
		t.Fatalf("expected invalid jwt extraction to return empty string, got %q", got)
	}
}
