package oauth

import (
	"encoding/json"
	"net/url"
	"strings"
)

const (
	geminiCliClientID     = "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com"
	geminiCliClientSecret = "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"
	geminiCliRedirectURI  = "http://localhost:8085/oauth2callback"
	geminiCliAuthURL      = "https://accounts.google.com/o/oauth2/v2/auth"
	geminiCliTokenURL     = "https://oauth2.googleapis.com/token"
)

var geminiCliScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
}

func GeminiCliClientID() string {
	return geminiCliClientID
}

func GeminiCliClientSecret() string {
	return geminiCliClientSecret
}

func GeminiCliRedirectURI() string {
	return geminiCliRedirectURI
}

func GeminiCliTokenURL() string {
	return geminiCliTokenURL
}

func BuildGeminiCliAuthorizeURL(codeChallenge string, state string) string {
	params := url.Values{}
	params.Set("client_id", geminiCliClientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", geminiCliRedirectURI)
	params.Set("scope", strings.Join(geminiCliScopes, " "))
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("state", state)
	params.Set("access_type", "offline")
	params.Set("prompt", "consent")
	return geminiCliAuthURL + "?" + params.Encode()
}

func ParseOAuthRedirectURL(input string) (code string, state string) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", ""
	}
	return strings.TrimSpace(parsed.Query().Get("code")), strings.TrimSpace(parsed.Query().Get("state"))
}

func BuildGoogleOAuthAPIKey(accessToken string, projectID string) (string, error) {
	payload := map[string]string{
		"token":     strings.TrimSpace(accessToken),
		"projectId": strings.TrimSpace(projectID),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func ParseGoogleOAuthAPIKey(apiKey string) (token string, projectID string, ok bool) {
	var payload struct {
		Token     string `json:"token"`
		ProjectID string `json:"projectId"`
	}
	if err := json.Unmarshal([]byte(apiKey), &payload); err != nil {
		return "", "", false
	}
	token = strings.TrimSpace(payload.Token)
	projectID = strings.TrimSpace(payload.ProjectID)
	return token, projectID, token != "" && projectID != ""
}
