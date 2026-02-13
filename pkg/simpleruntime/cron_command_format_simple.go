package connector

import (
	"fmt"
	"strings"

	"github.com/beeper/ai-bridge/pkg/simpleruntime/simpledeps/cron"
)

func formatCronStatusText(enabled bool, storePath string, jobCount int, _ *int64) string {
	return fmt.Sprintf("Cron enabled: %t\nStore: %s\nJobs: %d", enabled, storePath, jobCount)
}

func formatCronJobListText(jobs []cron.CronJob) string {
	if len(jobs) == 0 {
		return "No cron jobs."
	}
	lines := make([]string, 0, len(jobs)+1)
	lines = append(lines, "Cron jobs:")
	for _, job := range jobs {
		lines = append(lines, fmt.Sprintf("- %s", strings.TrimSpace(job.ID)))
	}
	return strings.Join(lines, "\n")
}

func formatCronRunsText(jobID string, _ []cron.CronRunLogEntry) string {
	return fmt.Sprintf("No run entries for %s", strings.TrimSpace(jobID))
}

func formatCronSchedule(_ cron.CronSchedule) string { return "" }
func formatDurationMs(ms int64) string              { return fmt.Sprintf("%dms", ms) }
func formatUnixMs(ms int64) string                  { return fmt.Sprintf("%d", ms) }
func cronShortID(id string) string                  { return id }
