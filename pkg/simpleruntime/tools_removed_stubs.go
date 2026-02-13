package connector

import (
	"strings"
	"time"

	cron "github.com/beeper/ai-bridge/pkg/simpleruntime/simplecron"
)

const defaultNexusTimeoutSeconds = 30

func isNexusToolName(string) bool {
	return false
}

func isNexusCompactToolName(string) bool {
	return false
}

func nexusConfigured(cfg *NexusToolsConfig) bool {
	if cfg == nil {
		return false
	}
	return strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Token) != ""
}

func cronRunLogEntryFromEvent(evt cron.CronEvent) cron.CronRunLogEntry {
	return cron.CronRunLogEntry{
		TS:          time.Now().UnixMilli(),
		JobID:       evt.JobID,
		Action:      evt.Action,
		Status:      evt.Status,
		Error:       evt.Error,
		Summary:     evt.Summary,
		RunAtMs:     evt.RunAtMs,
		DurationMs:  evt.DurationMs,
		NextRunAtMs: evt.NextRunAtMs,
	}
}
