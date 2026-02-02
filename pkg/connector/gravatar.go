package connector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	gravatarAPIBaseURL    = "https://api.gravatar.com/v3"
	gravatarAvatarBaseURL = "https://0.gravatar.com/avatar"
)

func normalizeGravatarEmail(email string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(email))
	if normalized == "" {
		return "", fmt.Errorf("email is required")
	}
	if !strings.Contains(normalized, "@") {
		return "", fmt.Errorf("invalid email address")
	}
	return normalized, nil
}

func gravatarHash(email string) string {
	hash := sha256.Sum256([]byte(email))
	return hex.EncodeToString(hash[:])
}

func ensureGravatarState(meta *UserLoginMetadata) *GravatarState {
	if meta.Gravatar == nil {
		meta.Gravatar = &GravatarState{}
	}
	return meta.Gravatar
}

func fetchGravatarProfile(ctx context.Context, email string) (*GravatarProfile, error) {
	normalized, err := normalizeGravatarEmail(email)
	if err != nil {
		return nil, err
	}
	hash := gravatarHash(normalized)

	reqURL := fmt.Sprintf("%s/profiles/%s", gravatarAPIBaseURL, hash)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Gravatar profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("gravatar profile not found for %s", normalized)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gravatar profile request failed: status %d", resp.StatusCode)
	}

	var profile map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("failed to decode Gravatar profile: %w", err)
	}

	if _, ok := profile["hash"]; !ok {
		profile["hash"] = hash
	}
	if _, ok := profile["avatar_url"]; !ok {
		profile["avatar_url"] = fmt.Sprintf("%s/%s", gravatarAvatarBaseURL, hash)
	}

	return &GravatarProfile{
		Email:     normalized,
		Hash:      hash,
		Profile:   profile,
		FetchedAt: time.Now().Unix(),
	}, nil
}

func formatGravatarProfileJSON(profile map[string]any) string {
	if profile == nil {
		return "{}"
	}
	payload, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func formatGravatarContext(profile *GravatarProfile) string {
	if profile == nil {
		return ""
	}
	lines := []string{
		"## User Profile (Gravatar)",
		fmt.Sprintf("Email: %s", profile.Email),
		fmt.Sprintf("Hash: %s", profile.Hash),
	}
	if profile.FetchedAt > 0 {
		lines = append(lines, fmt.Sprintf("FetchedAt: %s", time.Unix(profile.FetchedAt, 0).UTC().Format(time.RFC3339)))
	}
	lines = append(lines,
		"Profile JSON:",
		formatGravatarProfileJSON(profile.Profile),
	)
	return strings.Join(lines, "\n")
}

func (oc *AIClient) gravatarContext() string {
	loginMeta := loginMetadata(oc.UserLogin)
	if loginMeta == nil || loginMeta.Gravatar == nil || loginMeta.Gravatar.Primary == nil {
		return ""
	}
	return formatGravatarContext(loginMeta.Gravatar.Primary)
}
