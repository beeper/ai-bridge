package cron

import (
	"strings"
	"time"

	cronlib "github.com/robfig/cron/v3"
)

// ComputeNextRunAtMs returns the next run time in unix ms.
func ComputeNextRunAtMs(schedule CronSchedule, nowMs int64) *int64 {
	kind := strings.TrimSpace(schedule.Kind)
	switch kind {
	case "at":
		if schedule.AtMs > nowMs {
			return &schedule.AtMs
		}
		return nil
	case "every":
		everyMs := schedule.EveryMs
		if everyMs < 1 {
			everyMs = 1
		}
		anchor := schedule.AnchorMs
		if anchor <= 0 {
			anchor = nowMs
		}
		if nowMs < anchor {
			return &anchor
		}
		elapsed := nowMs - anchor
		steps := (elapsed + everyMs - 1) / everyMs
		if steps < 1 {
			steps = 1
		}
		next := anchor + steps*everyMs
		return &next
	case "cron":
		expr := strings.TrimSpace(schedule.Expr)
		if expr == "" {
			return nil
		}
		location := time.UTC
		if tz := strings.TrimSpace(schedule.TZ); tz != "" {
			if loc, err := time.LoadLocation(tz); err == nil {
				location = loc
			}
		}
		parser := cronlib.NewParser(cronlib.Minute | cronlib.Hour | cronlib.Dom | cronlib.Month | cronlib.Dow | cronlib.Descriptor)
		sched, err := parser.Parse(expr)
		if err != nil {
			return nil
		}
		next := sched.Next(time.UnixMilli(nowMs).In(location))
		if next.IsZero() {
			return nil
		}
		nextMs := next.UTC().UnixMilli()
		return &nextMs
	default:
		return nil
	}
}
