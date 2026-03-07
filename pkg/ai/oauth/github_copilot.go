package oauth

import (
	"net/url"
	"regexp"
	"strings"
)

const defaultGitHubDomain = "github.com"

var proxyEndpointPattern = regexp.MustCompile(`proxy-ep=([^;]+)`)

func NormalizeDomain(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	rawURL := trimmed
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || strings.TrimSpace(parsed.Hostname()) == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}

func getBaseURLFromCopilotToken(token string) string {
	matches := proxyEndpointPattern.FindStringSubmatch(token)
	if len(matches) != 2 {
		return ""
	}
	proxyHost := strings.TrimSpace(matches[1])
	if proxyHost == "" {
		return ""
	}
	apiHost := strings.TrimPrefix(proxyHost, "proxy.")
	return "https://api." + apiHost
}

func GetGitHubCopilotBaseURL(token string, enterpriseDomain string) string {
	if fromToken := getBaseURLFromCopilotToken(token); fromToken != "" {
		return fromToken
	}
	if normalizedEnterprise := NormalizeDomain(enterpriseDomain); normalizedEnterprise != "" {
		return "https://copilot-api." + normalizedEnterprise
	}
	return "https://api.individual.githubcopilot.com"
}

func ResolveGitHubDomain(enterpriseDomain string) string {
	if normalized := NormalizeDomain(enterpriseDomain); normalized != "" {
		return normalized
	}
	return defaultGitHubDomain
}
