package cron

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Logger matches OpenClaw logger shape.
type Logger interface {
	Debug(msg string, fields ...any)
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
}

// HeartbeatRunResult mirrors OpenClaw heartbeat results.
type HeartbeatRunResult struct {
	Status string
	Reason string
}

// CronEvent is emitted on job changes.
type CronEvent struct {
	JobID       string
	Action      string
	RunAtMs     int64
	DurationMs  int64
	Status      string
	Error       string
	Summary     string
	NextRunAtMs int64
}

// CronServiceDeps provides integration hooks.
type CronServiceDeps struct {
	NowMs              func() int64
	Log                Logger
	StorePath          string
	CronEnabled        bool
	EnqueueSystemEvent func(text string, agentID string) error
	RequestHeartbeatNow func(reason string)
	RunHeartbeatOnce   func(reason string) HeartbeatRunResult
	RunIsolatedAgentJob func(job CronJob, message string) (status string, summary string, outputText string, err error)
	OnEvent            func(evt CronEvent)
}

// CronService schedules and runs jobs.
type CronService struct {
	deps          CronServiceDeps
	store         *CronStoreFile
	timer         *time.Timer
	running       bool
	warnedDisabled bool
	mu            sync.Mutex
}

// NewCronService creates a new cron service.
func NewCronService(deps CronServiceDeps) *CronService {
	if deps.NowMs == nil {
		deps.NowMs = func() int64 { return time.Now().UnixMilli() }
	}
	return &CronService{deps: deps}
}

// Start initializes the scheduler.
func (c *CronService) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.deps.CronEnabled {
		c.logInfo("cron: disabled", map[string]any{"enabled": false})
		return nil
	}
	if err := c.ensureLoaded(); err != nil {
		return err
	}
	recomputeNextRuns(c.store, c.deps.NowMs(), c.deps.Log)
	if err := c.persist(); err != nil {
		return err
	}
	c.armTimerLocked()
	c.logInfo("cron: started", map[string]any{
		"enabled":      true,
		"jobs":         len(c.store.Jobs),
		"nextWakeAtMs": nextWakeAtMs(c.store),
	})
	return nil
}

// Stop stops the scheduler.
func (c *CronService) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopTimerLocked()
}

// Status returns scheduler status.
func (c *CronService) Status() (bool, string, int, *int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureLoaded(); err != nil {
		return false, c.deps.StorePath, 0, nil, err
	}
	var next *int64
	if c.deps.CronEnabled {
		next = nextWakeAtMs(c.store)
	}
	return c.deps.CronEnabled, c.deps.StorePath, len(c.store.Jobs), next, nil
}

// List returns jobs.
func (c *CronService) List(includeDisabled bool) ([]CronJob, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureLoaded(); err != nil {
		return nil, err
	}
	jobs := make([]CronJob, 0)
	for _, job := range c.store.Jobs {
		if includeDisabled || job.Enabled {
			jobs = append(jobs, job)
		}
	}
	sortJobs(jobs)
	return jobs, nil
}

// Add creates a job.
func (c *CronService) Add(input CronJobCreate) (CronJob, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.warnIfDisabled("add")
	if err := c.ensureLoaded(); err != nil {
		return CronJob{}, err
	}
	job, err := createJob(c.deps.NowMs(), input)
	if err != nil {
		return CronJob{}, err
	}
	c.store.Jobs = append(c.store.Jobs, job)
	if err := c.persist(); err != nil {
		return CronJob{}, err
	}
	c.armTimerLocked()
	c.emit(CronEvent{JobID: job.ID, Action: "added", NextRunAtMs: derefInt64(job.State.NextRunAtMs)})
	return job, nil
}

// Update modifies a job.
func (c *CronService) Update(id string, patch CronJobPatch) (CronJob, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.warnIfDisabled("update")
	if err := c.ensureLoaded(); err != nil {
		return CronJob{}, err
	}
	idx := findJobIndex(c.store.Jobs, id)
	if idx == -1 {
		return CronJob{}, fmt.Errorf("unknown cron job id: %s", id)
	}
	job := c.store.Jobs[idx]
	if err := applyJobPatch(&job, patch); err != nil {
		return CronJob{}, err
	}
	job.UpdatedAtMs = c.deps.NowMs()
	if job.Enabled {
		job.State.NextRunAtMs = computeJobNextRunAtMs(job, c.deps.NowMs())
	} else {
		job.State.NextRunAtMs = nil
		job.State.RunningAtMs = nil
	}
	c.store.Jobs[idx] = job
	if err := c.persist(); err != nil {
		return CronJob{}, err
	}
	c.armTimerLocked()
	c.emit(CronEvent{JobID: job.ID, Action: "updated", NextRunAtMs: derefInt64(job.State.NextRunAtMs)})
	return job, nil
}

// Remove deletes a job.
func (c *CronService) Remove(id string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.warnIfDisabled("remove")
	if err := c.ensureLoaded(); err != nil {
		return false, err
	}
	before := len(c.store.Jobs)
	filtered := make([]CronJob, 0, len(c.store.Jobs))
	for _, job := range c.store.Jobs {
		if job.ID != id {
			filtered = append(filtered, job)
		}
	}
	removed := len(filtered) != before
	c.store.Jobs = filtered
	if err := c.persist(); err != nil {
		return false, err
	}
	c.armTimerLocked()
	if removed {
		c.emit(CronEvent{JobID: id, Action: "removed"})
	}
	return removed, nil
}

// Run executes a job if due (or forced).
func (c *CronService) Run(id string, mode string) (bool, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.warnIfDisabled("run")
	if err := c.ensureLoaded(); err != nil {
		return false, "", err
	}
	idx := findJobIndex(c.store.Jobs, id)
	if idx == -1 {
		return false, "", fmt.Errorf("unknown cron job id: %s", id)
	}
	job := c.store.Jobs[idx]
	forced := mode == "force"
	if !isJobDue(job, c.deps.NowMs(), forced) {
		return false, "not-due", nil
	}
	deleted, err := c.executeJob(&job, forced)
	if err != nil {
		return false, "", err
	}
	if deleted {
		filtered := make([]CronJob, 0, len(c.store.Jobs))
		for _, existing := range c.store.Jobs {
			if existing.ID != job.ID {
				filtered = append(filtered, existing)
			}
		}
		c.store.Jobs = filtered
		c.emit(CronEvent{JobID: job.ID, Action: "removed"})
	} else {
		c.store.Jobs[idx] = job
	}
	if err := c.persist(); err != nil {
		return false, "", err
	}
	c.armTimerLocked()
	return true, "", nil
}

// Wake enqueues a system event.
func (c *CronService) Wake(mode string, text string) (bool, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false, errors.New("text required")
	}
	if c.deps.EnqueueSystemEvent == nil {
		return false, errors.New("enqueueSystemEvent not configured")
	}
	if err := c.deps.EnqueueSystemEvent(trimmed, ""); err != nil {
		return false, err
	}
	if mode == "now" && c.deps.RequestHeartbeatNow != nil {
		c.deps.RequestHeartbeatNow("wake")
	}
	return true, nil
}

func (c *CronService) onTimer() {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureLoaded(); err != nil {
		return
	}
	c.runDueJobsLocked()
	_ = c.persist()
	c.armTimerLocked()
}

func (c *CronService) runDueJobsLocked() {
	now := c.deps.NowMs()
	filtered := make([]CronJob, 0, len(c.store.Jobs))
	for _, job := range c.store.Jobs {
		if !job.Enabled || job.State.RunningAtMs != nil || job.State.NextRunAtMs == nil || now < *job.State.NextRunAtMs {
			filtered = append(filtered, job)
			continue
		}
		deleted, _ := c.executeJob(&job, false)
		if deleted {
			c.emit(CronEvent{JobID: job.ID, Action: "removed"})
			continue
		}
		filtered = append(filtered, job)
	}
	c.store.Jobs = filtered
}

func (c *CronService) executeJob(job *CronJob, forced bool) (bool, error) {
	startedAt := c.deps.NowMs()
	job.State.RunningAtMs = &startedAt
	job.State.LastError = ""
	c.emit(CronEvent{JobID: job.ID, Action: "started", RunAtMs: startedAt})

	deleted := false

	finish := func(statusVal, errVal, summaryVal, outputVal string) {
		endedAt := c.deps.NowMs()
		job.State.RunningAtMs = nil
		job.State.LastRunAtMs = &startedAt
		job.State.LastStatus = statusVal
		job.State.LastDurationMs = ptrInt64(maxInt64(0, endedAt-startedAt))
		job.State.LastError = errVal

		shouldDelete := job.Schedule.Kind == "at" && statusVal == "ok" && job.DeleteAfterRun
		if !shouldDelete {
			if job.Schedule.Kind == "at" && statusVal == "ok" {
				job.Enabled = false
				job.State.NextRunAtMs = nil
			} else if job.Enabled {
				job.State.NextRunAtMs = computeJobNextRunAtMs(*job, endedAt)
			} else {
				job.State.NextRunAtMs = nil
			}
		}

		nextRun := int64(0)
		if job.State.NextRunAtMs != nil {
			nextRun = *job.State.NextRunAtMs
		}
		c.emit(CronEvent{
			JobID:       job.ID,
			Action:      "finished",
			Status:      statusVal,
			Error:       errVal,
			Summary:     summaryVal,
			RunAtMs:     startedAt,
			DurationMs:  derefInt64(job.State.LastDurationMs),
			NextRunAtMs: nextRun,
		})

		if shouldDelete {
			deleted = true
			c.emit(CronEvent{JobID: job.ID, Action: "removed"})
		}

		if job.SessionTarget == CronSessionIsolated {
			prefix := ""
			if job.Isolation != nil {
				prefix = strings.TrimSpace(job.Isolation.PostToMainPrefix)
			}
			if prefix == "" {
				prefix = "Cron"
			}
			mode := ""
			if job.Isolation != nil {
				mode = strings.TrimSpace(job.Isolation.PostToMainMode)
			}
			if mode == "" {
				mode = "summary"
			}
			body := strings.TrimSpace(summaryVal)
			if body == "" {
				body = strings.TrimSpace(errVal)
			}
			if body == "" {
				body = statusVal
			}
			if mode == "full" {
				maxChars := 8000
				if job.Isolation != nil && job.Isolation.PostToMainMaxChars != nil && *job.Isolation.PostToMainMaxChars > 0 {
					maxChars = *job.Isolation.PostToMainMaxChars
				}
				full := strings.TrimSpace(outputVal)
				if full != "" {
					if len(full) > maxChars {
						full = full[:maxChars] + "…"
					}
					body = full
				}
			}
			statusPrefix := prefix
			if statusVal != "ok" {
				statusPrefix = fmt.Sprintf("%s (%s)", prefix, statusVal)
			}
			_ = c.deps.EnqueueSystemEvent(fmt.Sprintf("%s: %s", statusPrefix, body), job.AgentID)
			if job.WakeMode == CronWakeNow && c.deps.RequestHeartbeatNow != nil {
				c.deps.RequestHeartbeatNow("cron:" + job.ID + ":post")
			}
		}
	}

	defer func() {
		job.UpdatedAtMs = c.deps.NowMs()
		if !forced && job.Enabled && !deleted {
			job.State.NextRunAtMs = computeJobNextRunAtMs(*job, c.deps.NowMs())
		}
		if deleted {
			// deletion handled by caller
		}
	}()

	if job.SessionTarget == CronSessionMain {
		text, reason := resolveJobPayloadTextForMain(*job)
		if strings.TrimSpace(text) == "" {
			finish("skipped", reason, "", "")
			return deleted, nil
		}
		if c.deps.EnqueueSystemEvent == nil {
			finish("error", "enqueueSystemEvent not configured", "", "")
			return deleted, nil
		}
		_ = c.deps.EnqueueSystemEvent(text, job.AgentID)
		if job.WakeMode == CronWakeNow && c.deps.RunHeartbeatOnce != nil {
			reason := "cron:" + job.ID
			maxWaitMs := int64(2 * 60_000)
			startWait := c.deps.NowMs()
			for {
				res := c.deps.RunHeartbeatOnce(reason)
				if res.Status != "skipped" || res.Reason != "requests-in-flight" {
					switch res.Status {
					case "ran":
						finish("ok", "", text, "")
					case "skipped":
						finish("skipped", res.Reason, text, "")
					default:
						finish("error", res.Reason, text, "")
					}
					return deleted, nil
				}
				if c.deps.NowMs()-startWait > maxWaitMs {
					finish("skipped", "timeout waiting for main lane to become idle", text, "")
					return deleted, nil
				}
				time.Sleep(250 * time.Millisecond)
			}
		}
		if c.deps.RequestHeartbeatNow != nil {
			c.deps.RequestHeartbeatNow("cron:" + job.ID)
		}
		finish("ok", "", text, "")
		return deleted, nil
	}

	if strings.ToLower(job.Payload.Kind) != "agentturn" {
		finish("skipped", "isolated job requires payload.kind=agentTurn", "", "")
		return deleted, nil
	}

	if c.deps.RunIsolatedAgentJob == nil {
		finish("error", "isolated cron jobs not supported", "", "")
		return deleted, nil
	}
	status, summary, output, err := c.deps.RunIsolatedAgentJob(*job, job.Payload.Message)
	if err != nil {
		finish("error", err.Error(), summary, output)
		return deleted, nil
	}
	if status == "ok" {
		finish("ok", "", summary, output)
		return deleted, nil
	}
	if status == "skipped" {
		finish("skipped", "", summary, output)
		return deleted, nil
	}
	finish("error", "cron job failed", summary, output)
	return deleted, nil
}

func (c *CronService) ensureLoaded() error {
	if c.store != nil {
		return nil
	}
	store, err := LoadCronStore(c.deps.StorePath)
	if err != nil {
		return err
	}
	c.store = &store
	// fix names/description
	mutated := false
	for i := range c.store.Jobs {
		job := c.store.Jobs[i]
		if strings.TrimSpace(job.Name) == "" {
			name := inferLegacyName(&CronJobCreate{Payload: job.Payload, Schedule: job.Schedule})
			job.Name = name
			mutated = true
		}
		if strings.TrimSpace(job.Description) != job.Description {
			job.Description = strings.TrimSpace(job.Description)
			mutated = true
		}
		if migrated := migrateLegacyPayload(&job.Payload); migrated {
			mutated = true
		}
		c.store.Jobs[i] = job
	}
	if mutated {
		return c.persist()
	}
	return nil
}

func (c *CronService) persist() error {
	if c.store == nil {
		return nil
	}
	return SaveCronStore(c.deps.StorePath, *c.store)
}

func (c *CronService) warnIfDisabled(action string) {
	if c.deps.CronEnabled {
		return
	}
	if c.warnedDisabled {
		return
	}
	c.warnedDisabled = true
	c.logWarn("cron: scheduler disabled; jobs will not run automatically", map[string]any{
		"enabled":   false,
		"action":    action,
		"storePath": c.deps.StorePath,
	})
}

func (c *CronService) armTimerLocked() {
	c.stopTimerLocked()
	if !c.deps.CronEnabled || c.store == nil {
		return
	}
	next := nextWakeAtMs(c.store)
	if next == nil {
		return
	}
	delayMs := maxInt64(0, *next-c.deps.NowMs())
	const maxTimeoutMs int64 = (1 << 31) - 1
	if delayMs > maxTimeoutMs {
		delayMs = maxTimeoutMs
	}
	delay := time.Duration(delayMs) * time.Millisecond
	c.timer = time.AfterFunc(delay, func() { c.onTimer() })
}

func (c *CronService) stopTimerLocked() {
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}

func (c *CronService) emit(evt CronEvent) {
	if c.deps.OnEvent == nil {
		return
	}
	c.deps.OnEvent(evt)
}

func (c *CronService) logInfo(msg string, fields map[string]any) {
	if c.deps.Log != nil {
		c.deps.Log.Info(msg, fields)
	}
}

func (c *CronService) logWarn(msg string, fields map[string]any) {
	if c.deps.Log != nil {
		c.deps.Log.Warn(msg, fields)
	}
}

// utils in utils.go
