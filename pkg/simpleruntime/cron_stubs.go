package connector

import (
	"fmt"

	"github.com/beeper/ai-bridge/pkg/simpleruntime/cron"
)

func formatCronStatusText(enabled bool, storePath string, jobCount int, nextWake int64) string {
	_ = nextWake
	if !enabled {
		return "Cron is disabled."
	}
	return fmt.Sprintf("Cron enabled. Jobs: %d\nStore: %s", jobCount, storePath)
}

func formatCronJobListText(jobs []cron.CronJob) string {
	if len(jobs) == 0 {
		return "No cron jobs."
	}
	return fmt.Sprintf("Cron jobs: %d", len(jobs))
}

func formatCronRunsText(jobID string, entries []cron.CronRunLogEntry) string {
	return fmt.Sprintf("Runs for %s: %d", jobID, len(entries))
}

func (oc *AIClient) readCronRuns(_ string, _ int) ([]cron.CronRunLogEntry, error) {
	return nil, nil
}
