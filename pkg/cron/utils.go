package cron

import "strings"

//lint:ignore U1000 reserved for upcoming cron scheduling helpers
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
