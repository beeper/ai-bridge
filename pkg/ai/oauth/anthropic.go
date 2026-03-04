package oauth

import (
	"net/url"
	"strings"
	"time"
)

const (
	anthropicAuthorizeURL = "https://claude.ai/oauth/authorize"
	anthropicTokenURL     = "https://console.anthropic.com/v1/oauth/token"
	anthropicRedirectURI  = "https://console.anthropic.com/oauth/code/callback"
	anthropicScopes       = "org:create_api_key user:profile user:inference"
	anthropicClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
)

func AnthropicClientID() string {
	return anthropicClientID
}

func AnthropicTokenURL() string {
	return anthropicTokenURL
}

func BuildAnthropicAuthorizeURL(codeChallenge string, state string) string {
	params := url.Values{}
	params.Set("code", "true")
	params.Set("client_id", anthropicClientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", anthropicRedirectURI)
	params.Set("scope", anthropicScopes)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("state", state)
	return anthropicAuthorizeURL + "?" + params.Encode()
}

func ParseAnthropicAuthorizationCode(input string) (code string, state string) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", ""
	}
	parts := strings.SplitN(raw, "#", 2)
	code = strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		state = strings.TrimSpace(parts[1])
	}
	return code, state
}

func OAuthExpiryWithBuffer(now time.Time, expiresInSeconds int64) int64 {
	return now.Add(time.Duration(expiresInSeconds)*time.Second - 5*time.Minute).UnixMilli()
}
