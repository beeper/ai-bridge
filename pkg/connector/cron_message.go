package connector

import (
	"fmt"
	"strings"
	"time"
)

func buildCronMessage(jobID, jobName, message, timezone string) string {
	base := strings.TrimSpace(message)
	name := strings.TrimSpace(jobName)
	if name == "" {
		name = "cron"
	}
	if base == "" {
		base = name
	}
	header := fmt.Sprintf("[cron:%s %s] %s", strings.TrimSpace(jobID), name, base)
	timeLine := fmt.Sprintf("Current time: %s", formatCronTime(timezone))
	return strings.TrimSpace(header + "\n" + timeLine)
}

func formatCronTime(timezone string) string {
	loc := time.UTC
	if tz := strings.TrimSpace(timezone); tz != "" {
		if loaded, err := time.LoadLocation(tz); err == nil {
			loc = loaded
		}
	}
	now := time.Now().In(loc)
	weekday := now.Format("Monday")
	month := now.Format("January")
	day := now.Day()
	ordinal := dayOrdinal(day)
	year := now.Year()
	hour := now.Hour()
	minute := now.Minute()
	suffix := "AM"
	if hour >= 12 {
		suffix = "PM"
	}
	hour12 := hour % 12
	if hour12 == 0 {
		hour12 = 12
	}
	return fmt.Sprintf("%s, %s %d%s, %d — %d:%02d %s (%s)", weekday, month, day, ordinal, year, hour12, minute, suffix, loc.String())
}

func dayOrdinal(day int) string {
	if day%100 >= 11 && day%100 <= 13 {
		return "th"
	}
	switch day % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	default:
		return "th"
	}
}
