package cron

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const stuckRunMs int64 = 2 * 60 * 60 * 1000

func createJob(nowMs int64, input CronJobCreate) (CronJob, error) {
	name := input.Name
	if strings.TrimSpace(name) == "" {
		name = inferLegacyName(&input)
	}
	normalizedName, err := normalizeRequiredName(name)
	if err != nil {
		return CronJob{}, err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	deleteAfter := false
	if input.DeleteAfterRun != nil {
		deleteAfter = *input.DeleteAfterRun
	}
	wakeMode := input.WakeMode
	if wakeMode == "" {
		wakeMode = CronWakeNextHeartbeat
	}

	job := CronJob{
		ID:             randomID(),
		AgentID:        normalizeOptionalAgentID(input.AgentID),
		Name:           normalizedName,
		Description:    normalizeOptionalText(input.Description),
		Enabled:        enabled,
		DeleteAfterRun: deleteAfter,
		CreatedAtMs:    nowMs,
		UpdatedAtMs:    nowMs,
		Schedule:       input.Schedule,
		SessionTarget:  input.SessionTarget,
		WakeMode:       wakeMode,
		Payload:        input.Payload,
		Isolation:      input.Isolation,
		State:          CronJobState{},
	}
	if input.State != nil {
		job.State = *input.State
	}
	if err := assertSupportedJobSpec(job.SessionTarget, job.Payload); err != nil {
		return CronJob{}, err
	}
	job.State.NextRunAtMs = computeJobNextRunAtMs(job, nowMs)
	return job, nil
}

func applyJobPatch(job *CronJob, patch CronJobPatch) error {
	if job == nil {
		return fmt.Errorf("job required")
	}
	if patch.Name != nil {
		name, err := normalizeRequiredName(*patch.Name)
		if err != nil {
			return err
		}
		job.Name = name
	}
	if patch.Description != nil {
		job.Description = normalizeOptionalText(patch.Description)
	}
	if patch.Enabled != nil {
		job.Enabled = *patch.Enabled
	}
	if patch.DeleteAfterRun != nil {
		job.DeleteAfterRun = *patch.DeleteAfterRun
	}
	if patch.Schedule != nil {
		job.Schedule = *patch.Schedule
	}
	if patch.SessionTarget != nil {
		job.SessionTarget = *patch.SessionTarget
	}
	if patch.WakeMode != nil {
		job.WakeMode = *patch.WakeMode
	}
	if patch.Payload != nil {
		job.Payload = mergeCronPayload(job.Payload, *patch.Payload)
	}
	if patch.Isolation != nil {
		job.Isolation = patch.Isolation
	}
	if patch.State != nil {
		job.State = mergeCronState(job.State, *patch.State)
	}
	if patch.AgentID != nil {
		job.AgentID = normalizeOptionalAgentID(patch.AgentID)
	}
	return assertSupportedJobSpec(job.SessionTarget, job.Payload)
}

func mergeCronState(existing CronJobState, patch CronJobState) CronJobState {
	next := existing
	if patch.NextRunAtMs != nil {
		next.NextRunAtMs = patch.NextRunAtMs
	}
	if patch.RunningAtMs != nil {
		next.RunningAtMs = patch.RunningAtMs
	}
	if patch.LastRunAtMs != nil {
		next.LastRunAtMs = patch.LastRunAtMs
	}
	if patch.LastStatus != "" {
		next.LastStatus = patch.LastStatus
	}
	if patch.LastError != "" {
		next.LastError = patch.LastError
	}
	if patch.LastDurationMs != nil {
		next.LastDurationMs = patch.LastDurationMs
	}
	return next
}

func mergeCronPayload(existing CronPayload, patch CronPayloadPatch) CronPayload {
	if patch.Kind != "" && !strings.EqualFold(patch.Kind, existing.Kind) {
		return buildPayloadFromPatch(patch)
	}
	kind := strings.ToLower(existing.Kind)
	if kind == "systemevent" {
		text := existing.Text
		if patch.Text != nil {
			text = *patch.Text
		}
		return CronPayload{Kind: "systemEvent", Text: text}
	}
	next := existing
	if patch.Message != nil {
		next.Message = *patch.Message
	}
	if patch.Model != nil {
		next.Model = *patch.Model
	}
	if patch.Thinking != nil {
		next.Thinking = *patch.Thinking
	}
	if patch.TimeoutSeconds != nil {
		next.TimeoutSeconds = patch.TimeoutSeconds
	}
	if patch.AllowUnsafeExternal != nil {
		next.AllowUnsafeExternal = patch.AllowUnsafeExternal
	}
	if patch.Deliver != nil {
		next.Deliver = patch.Deliver
	}
	if patch.Channel != nil {
		next.Channel = *patch.Channel
	}
	if patch.To != nil {
		next.To = *patch.To
	}
	if patch.BestEffortDeliver != nil {
		next.BestEffortDeliver = patch.BestEffortDeliver
	}
	return next
}

func buildPayloadFromPatch(patch CronPayloadPatch) CronPayload {
	kind := strings.ToLower(patch.Kind)
	if kind == "systemevent" {
		text := ""
		if patch.Text != nil {
			text = *patch.Text
		}
		if strings.TrimSpace(text) == "" {
			panic("cron.update payload.kind=systemEvent requires text")
		}
		return CronPayload{Kind: "systemEvent", Text: text}
	}
	msg := ""
	if patch.Message != nil {
		msg = *patch.Message
	}
	if strings.TrimSpace(msg) == "" {
		panic("cron.update payload.kind=agentTurn requires message")
	}
	return CronPayload{
		Kind:              "agentTurn",
		Message:           msg,
		Model:             derefString(patch.Model),
		Thinking:          derefString(patch.Thinking),
		TimeoutSeconds:    patch.TimeoutSeconds,
		AllowUnsafeExternal: patch.AllowUnsafeExternal,
		Deliver:           patch.Deliver,
		Channel:           derefString(patch.Channel),
		To:                derefString(patch.To),
		BestEffortDeliver: patch.BestEffortDeliver,
	}
}

func derefString(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func assertSupportedJobSpec(target CronSessionTarget, payload CronPayload) error {
	if target == CronSessionMain && !strings.EqualFold(payload.Kind, "systemEvent") {
		return fmt.Errorf("main cron jobs require payload.kind=systemEvent")
	}
	if target == CronSessionIsolated && !strings.EqualFold(payload.Kind, "agentTurn") {
		return fmt.Errorf("isolated cron jobs require payload.kind=agentTurn")
	}
	return nil
}

func computeJobNextRunAtMs(job CronJob, nowMs int64) *int64 {
	if !job.Enabled {
		return nil
	}
	if strings.EqualFold(job.Schedule.Kind, "at") {
		if job.State.LastStatus == "ok" && job.State.LastRunAtMs != nil {
			return nil
		}
		if job.Schedule.AtMs <= 0 {
			return nil
		}
		return &job.Schedule.AtMs
	}
	return ComputeNextRunAtMs(job.Schedule, nowMs)
}

func recomputeNextRuns(store *CronStoreFile, nowMs int64, log Logger) {
	if store == nil {
		return
	}
	for idx := range store.Jobs {
		job := store.Jobs[idx]
		if !job.Enabled {
			job.State.NextRunAtMs = nil
			job.State.RunningAtMs = nil
			store.Jobs[idx] = job
			continue
		}
		if job.State.RunningAtMs != nil && nowMs-*job.State.RunningAtMs > stuckRunMs {
			if log != nil {
				log.Warn("cron: clearing stuck running marker", map[string]any{"jobId": job.ID, "runningAtMs": *job.State.RunningAtMs})
			}
			job.State.RunningAtMs = nil
		}
		job.State.NextRunAtMs = computeJobNextRunAtMs(job, nowMs)
		store.Jobs[idx] = job
	}
}

func nextWakeAtMs(store *CronStoreFile) *int64 {
	if store == nil {
		return nil
	}
	var next *int64
	for _, job := range store.Jobs {
		if !job.Enabled || job.State.NextRunAtMs == nil {
			continue
		}
		if next == nil || *job.State.NextRunAtMs < *next {
			val := *job.State.NextRunAtMs
			next = &val
		}
	}
	return next
}

func isJobDue(job CronJob, nowMs int64, forced bool) bool {
	if forced {
		return true
	}
	return job.Enabled && job.State.NextRunAtMs != nil && nowMs >= *job.State.NextRunAtMs
}

func findJobIndex(jobs []CronJob, id string) int {
	for i, job := range jobs {
		if job.ID == id {
			return i
		}
	}
	return -1
}

func sortJobs(jobs []CronJob) {
	sort.Slice(jobs, func(i, j int) bool {
		var a, b int64
		if jobs[i].State.NextRunAtMs != nil {
			a = *jobs[i].State.NextRunAtMs
		}
		if jobs[j].State.NextRunAtMs != nil {
			b = *jobs[j].State.NextRunAtMs
		}
		return a < b
	})
}

func randomID() string {
	return uuid.NewString()
}

// helpers defined in normalize.go and utils.go
