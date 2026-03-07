package oauth

import (
	"net/url"
	"strings"
)

const (
	antigravityClientID       = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	antigravityClientSecret   = "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"
	antigravityRedirectURI    = "http://localhost:51121/oauth-callback"
	antigravityDefaultProject = "rising-fact-p41fc"
)

var antigravityScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/cclog",
	"https://www.googleapis.com/auth/experimentsandconfigs",
}

func AntigravityClientID() string {
	return antigravityClientID
}

func AntigravityClientSecret() string {
	return antigravityClientSecret
}

func AntigravityRedirectURI() string {
	return antigravityRedirectURI
}

func AntigravityDefaultProjectID() string {
	return antigravityDefaultProject
}

func BuildAntigravityAuthorizeURL(codeChallenge string, state string) string {
	params := url.Values{}
	params.Set("client_id", antigravityClientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", antigravityRedirectURI)
	params.Set("scope", strings.Join(antigravityScopes, " "))
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("state", state)
	params.Set("access_type", "offline")
	params.Set("prompt", "consent")
	return "https://accounts.google.com/o/oauth2/v2/auth?" + params.Encode()
}

func ResolveAntigravityProjectID(loadCodeAssistPayload map[string]any) string {
	if loadCodeAssistPayload == nil {
		return antigravityDefaultProject
	}
	if raw, ok := loadCodeAssistPayload["cloudaicompanionProject"]; ok {
		switch value := raw.(type) {
		case string:
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		case map[string]any:
			if idRaw, ok := value["id"].(string); ok {
				if trimmed := strings.TrimSpace(idRaw); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return antigravityDefaultProject
}
