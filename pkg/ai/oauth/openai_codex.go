package oauth

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
)

const (
	openAICodexClientID    = "app_EMoamEEZ73f0CkXaXp7hrann"
	openAICodexAuthorize   = "https://auth.openai.com/oauth/authorize"
	openAICodexTokenURL    = "https://auth.openai.com/oauth/token"
	openAICodexRedirectURI = "http://localhost:1455/auth/callback"
	openAICodexScope       = "openid profile email offline_access"
	openAICodexJWTClaim    = "https://api.openai.com/auth"
)

func OpenAICodexClientID() string {
	return openAICodexClientID
}

func OpenAICodexTokenURL() string {
	return openAICodexTokenURL
}

func BuildOpenAICodexAuthorizeURL(codeChallenge string, state string, originator string) string {
	if strings.TrimSpace(originator) == "" {
		originator = "pi"
	}
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", openAICodexClientID)
	params.Set("redirect_uri", openAICodexRedirectURI)
	params.Set("scope", openAICodexScope)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")
	params.Set("state", state)
	params.Set("id_token_add_organizations", "true")
	params.Set("codex_cli_simplified_flow", "true")
	params.Set("originator", originator)
	return openAICodexAuthorize + "?" + params.Encode()
}

func ParseOpenAICodexAuthorizationInput(input string) (code string, state string) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", ""
	}
	if parsed, err := url.Parse(value); err == nil {
		query := parsed.Query()
		if query.Get("code") != "" || query.Get("state") != "" {
			return strings.TrimSpace(query.Get("code")), strings.TrimSpace(query.Get("state"))
		}
	}
	if strings.Contains(value, "#") {
		parts := strings.SplitN(value, "#", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	if strings.Contains(value, "code=") {
		query, err := url.ParseQuery(value)
		if err == nil {
			return strings.TrimSpace(query.Get("code")), strings.TrimSpace(query.Get("state"))
		}
	}
	return value, ""
}

func ExtractOpenAICodexAccountID(accessToken string) string {
	parts := strings.Split(accessToken, ".")
	if len(parts) != 3 {
		return ""
	}
	payloadSegment := parts[1]
	decoded, err := base64.RawURLEncoding.DecodeString(payloadSegment)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(payloadSegment)
		if err != nil {
			return ""
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return ""
	}
	authRaw, ok := payload[openAICodexJWTClaim]
	if !ok {
		return ""
	}
	authClaims, ok := authRaw.(map[string]any)
	if !ok {
		return ""
	}
	accountID, _ := authClaims["chatgpt_account_id"].(string)
	return strings.TrimSpace(accountID)
}
