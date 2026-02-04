package cron

import (
	"strings"
)

type normalizeOptions struct {
	applyDefaults bool
}

func normalizeCronJobInput(raw CronJobCreate, opts normalizeOptions) CronJobCreate {
	next := raw

	if opts.applyDefaults {
		if next.Enabled == nil {
			enabled := true
			next.Enabled = &enabled
		}
		if next.WakeMode == "" {
			next.WakeMode = CronWakeNextHeartbeat
		}
		if next.SessionTarget == "" {
			kind := strings.ToLower(strings.TrimSpace(next.Payload.Kind))
			if kind == "systemevent" {
				next.SessionTarget = CronSessionMain
			} else if kind == "agentturn" {
				next.SessionTarget = CronSessionIsolated
			}
		}
	}

	return next
}

// NormalizeCronJobCreate applies OpenClaw-like defaults.
func NormalizeCronJobCreate(raw CronJobCreate) CronJobCreate {
	return normalizeCronJobInput(raw, normalizeOptions{applyDefaults: true})
}

// NormalizeCronJobPatch currently no-op; provided for parity.
func NormalizeCronJobPatch(raw CronJobPatch) CronJobPatch {
	return raw
}

// CoerceSchedule fills kind/atMs based on fields.
func CoerceSchedule(schedule CronSchedule) CronSchedule {
	next := schedule
	kind := strings.TrimSpace(schedule.Kind)
	if kind == "" {
		switch {
		case schedule.AtMs > 0:
			next.Kind = "at"
		case schedule.EveryMs > 0:
			next.Kind = "every"
		case strings.TrimSpace(schedule.Expr) != "":
			next.Kind = "cron"
		}
	}
	return next
}

// CoerceScheduleFromInput supports at/atMs string parsing.
func CoerceScheduleFromInput(schedule CronSchedule, atRaw string) CronSchedule {
	next := schedule
	if next.AtMs == 0 && strings.TrimSpace(atRaw) != "" {
		if parsed, ok := parseAbsoluteTimeMs(atRaw); ok {
			next.AtMs = parsed
		}
	}
	return CoerceSchedule(next)
}
