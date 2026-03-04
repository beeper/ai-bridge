package oauth

import (
	"net/url"
	"testing"
)

func TestGoogleGeminiCliOAuthHelpers(t *testing.T) {
	authURL := BuildGeminiCliAuthorizeURL("challenge", "state")
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("expected valid gemini-cli auth URL, got err=%v", err)
	}
	query := parsed.Query()
	if query.Get("client_id") != GeminiCliClientID() {
		t.Fatalf("expected gemini client id in query")
	}
	if query.Get("redirect_uri") != GeminiCliRedirectURI() {
		t.Fatalf("expected redirect uri in query")
	}
	if query.Get("code_challenge") != "challenge" || query.Get("state") != "state" {
		t.Fatalf("expected challenge/state in query")
	}

	code, state := ParseOAuthRedirectURL("http://localhost:8085/oauth2callback?code=abc&state=xyz")
	if code != "abc" || state != "xyz" {
		t.Fatalf("expected parsed redirect code/state, got code=%q state=%q", code, state)
	}

	apiKey, err := BuildGoogleOAuthAPIKey("token-1", "project-1")
	if err != nil {
		t.Fatalf("unexpected error building google oauth api key: %v", err)
	}
	token, projectID, ok := ParseGoogleOAuthAPIKey(apiKey)
	if !ok || token != "token-1" || projectID != "project-1" {
		t.Fatalf("unexpected parsed google oauth api key: token=%q project=%q ok=%v", token, projectID, ok)
	}
	if _, _, ok := ParseGoogleOAuthAPIKey("{invalid-json"); ok {
		t.Fatalf("expected invalid json api key parse to fail")
	}
}
