package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

func ComputeProviderKey(providerID, model, baseURL string, headers map[string]string) string {
	headerNames := make([]string, 0, len(headers))
	for key := range headers {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "" {
			continue
		}
		headerNames = append(headerNames, normalized)
	}
	sort.Strings(headerNames)
	payload := map[string]any{
		"provider": providerID,
		"model":    model,
		"baseUrl":  strings.TrimSpace(baseURL),
		"headers":  headerNames,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
