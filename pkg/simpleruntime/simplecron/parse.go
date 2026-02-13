package cron

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// parseAbsoluteTimeMs parses an absolute time string into unix ms.
// Accepts RFC3339 and millisecond unix timestamps.
var (
	isoTZRe       = regexp.MustCompile(`(?i)(Z|[+-]\d{2}:?\d{2})$`)
	isoDateRe     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	isoDateTimeRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T`)
)

func normalizeUtcIso(raw string) string {
	if isoTZRe.MatchString(raw) {
		return raw
	}
	if isoDateRe.MatchString(raw) {
		return raw + "T00:00:00Z"
	}
	if isoDateTimeRe.MatchString(raw) {
		return raw + "Z"
	}
	return raw
}

func parseAbsoluteTimeMs(raw string) (int64, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
	}
	if ts, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		if ts > 0 {
			return ts, true
		}
		return 0, false
	}
	normalized := normalizeUtcIso(trimmed)
	if t, err := time.Parse(time.RFC3339, normalized); err == nil {
		return t.UTC().UnixMilli(), true
	}
	return 0, false
}
