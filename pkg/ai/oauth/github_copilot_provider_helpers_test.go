package oauth

import "testing"

func TestGitHubCopilotOAuthHelperFunctions(t *testing.T) {
	if got := NormalizeDomain(" https://Company.GHE.com/path "); got != "company.ghe.com" {
		t.Fatalf("expected normalized enterprise domain, got %q", got)
	}
	if got := NormalizeDomain("github.com"); got != "github.com" {
		t.Fatalf("expected plain hostname normalization, got %q", got)
	}
	if got := NormalizeDomain("://bad-url"); got != "" {
		t.Fatalf("expected invalid URL to normalize to empty string, got %q", got)
	}

	token := "tid=abc;exp=123;proxy-ep=proxy.individual.githubcopilot.com;foo=bar"
	if got := GetGitHubCopilotBaseURL(token, ""); got != "https://api.individual.githubcopilot.com" {
		t.Fatalf("expected base URL from token proxy endpoint, got %q", got)
	}
	if got := GetGitHubCopilotBaseURL("", "ghe.example.com"); got != "https://copilot-api.ghe.example.com" {
		t.Fatalf("expected enterprise fallback base URL, got %q", got)
	}
	if got := GetGitHubCopilotBaseURL("", ""); got != "https://api.individual.githubcopilot.com" {
		t.Fatalf("expected default individual base URL, got %q", got)
	}

	if got := ResolveGitHubDomain("enterprise.github.local"); got != "enterprise.github.local" {
		t.Fatalf("expected enterprise domain resolution, got %q", got)
	}
	if got := ResolveGitHubDomain(""); got != "github.com" {
		t.Fatalf("expected default github.com domain, got %q", got)
	}
}
