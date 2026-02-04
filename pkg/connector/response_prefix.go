package connector

import "strings"

func resolveResponsePrefix(cfg *Config, agentID string) string {
	if cfg == nil || cfg.Messages == nil {
		return ""
	}
	raw := strings.TrimSpace(cfg.Messages.ResponsePrefix)
	if strings.EqualFold(raw, "auto") {
		return ""
	}
	return raw
}
