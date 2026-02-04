package cron

import "strings"

const (
	defaultCronPostPrefix   = "Cron"
	defaultCronPostMaxChars = 8000
)

type CronIsolationMode string

const (
	CronIsolationSummary CronIsolationMode = "summary"
	CronIsolationFull    CronIsolationMode = "full"
)

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func ptrInt64(v int64) *int64 {
	return &v
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func migrateLegacyPayload(payload *CronPayload) bool {
	if payload == nil {
		return false
	}
	mutated := false
	channel := strings.TrimSpace(payload.Channel)
	provider := strings.TrimSpace(payload.LegacyProviderDeprecated)
	if channel == "" && provider != "" {
		channel = provider
		payload.Channel = strings.ToLower(channel)
		mutated = true
	}
	if channel != "" {
		lowered := strings.ToLower(channel)
		if payload.Channel != lowered {
			payload.Channel = lowered
			mutated = true
		}
	}
	if payload.LegacyProviderDeprecated != "" {
		payload.LegacyProviderDeprecated = ""
		mutated = true
	}
	return mutated
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
