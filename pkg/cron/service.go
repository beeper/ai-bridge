package cron

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.mau.fi/util/ptr"
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
	NowMs               func() int64
	Log                 Logger
	StorePath           string
	Store               StoreBackend
	MaxConcurrentRuns   int
	CronEnabled         bool
	EnqueueSystemEvent  func(text string, agentID string) error
	RequestHeartbeatNow func(reason string)
	RunHeartbeatOnce    func(reason string) HeartbeatRunResult
	RunIsolatedAgentJob func(job CronJob, message string) (status string, summary string, outputText string, err error)
	OnEvent             func(evt CronEvent)
}

// CronService schedules and runs jobs.
type CronService struct {
	deps           CronServiceDeps
	store          *CronStoreFile
	timer          *time.Timer
	running        bool
	warnedDisabled bool
	mu             sync.Mutex
}

func (c *CronService) withStoreLock(fn func() error) error {
	lock := storeLockForPath(c.deps.StorePath)
	lock.Lock()
	defer lock.Unlock()
	return fn()
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
	return c.withStoreLock(func() error {
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
	})
}

// Stop stops the scheduler.
func (c *CronService) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopTimerLocked()
}

// Status returns scheduler status.
func (c *CronService) Status() (bool, string, int, *int64, error) {
	var enabled bool
	var storePath string
	var jobs int
	var next *int64
	err := c.withStoreLock(func() error {
		c.mu.Lock()
		defer c.mu.Unlock()
		if err := c.ensureLoaded(); err != nil {
			return err
		}
		enabled = c.deps.CronEnabled
		storePath = c.deps.StorePath
		jobs = len(c.store.Jobs)
		if c.deps.CronEnabled {
			next = nextWakeAtMs(c.store)
		}
		return nil
	})
	if err != nil {
		return false, c.deps.StorePath, 0, nil, err
	}
	return enabled, storePath, jobs, next, nil
}

// List returns jobs.
func (c *CronService) List(includeDisabled bool) ([]CronJob, error) {
	var jobs []CronJob
	err := c.withStoreLock(func() error {
		c.mu.Lock()
		defer c.mu.Unlock()
		if err := c.ensureLoaded(); err != nil {
			return err
		}
		list := make([]CronJob, 0)
		for _, job := range c.store.Jobs {
			if includeDisabled || job.Enabled {
				list = append(list, job)
			}
		}
		sortJobs(list)
		jobs = list
		return nil
	})
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

// Add creates a job.
func (c *CronService) Add(input CronJobCreate) (CronJob, error) {
	var job CronJob
	err := c.withStoreLock(func() error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.warnIfDisabled("add")
		if err := c.ensureLoaded(); err != nil {
			return err
		}
		created, err := createJob(c.deps.NowMs(), input)
		if err != nil {
			return err
		}
		c.store.Jobs = append(c.store.Jobs, created)
		if err := c.persist(); err != nil {
			return err
		}
		c.armTimerLocked()
		c.emit(CronEvent{JobID: created.ID, Action: "added", NextRunAtMs: ptr.Val(created.State.NextRunAtMs)})
		job = created
		return nil
	})
	if err != nil {
		return CronJob{}, err
	}
	return job, nil
}

// Update modifies a job.
func (c *CronService) Update(id string, patch CronJobPatch) (CronJob, error) {
	var job CronJob
	err := c.withStoreLock(func() error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.warnIfDisabled("update")
		if err := c.ensureLoaded(); err != nil {
			return err
		}
		idx := findJobIndex(c.store.Jobs, id)
		if idx == -1 {
			return fmt.Errorf("unknown cron job id: %s", id)
		}
		current := c.store.Jobs[idx]
		if err := applyJobPatch(&current, patch); err != nil {
			return err
		}
		current.UpdatedAtMs = c.deps.NowMs()
		if current.Enabled {
			current.State.NextRunAtMs = computeJobNextRunAtMs(current, c.deps.NowMs())
		} else {
			current.State.NextRunAtMs = nil
			current.State.RunningAtMs = nil
		}
		c.store.Jobs[idx] = current
		if err := c.persist(); err != nil {
			return err
		}
		c.armTimerLocked()
		c.emit(CronEvent{JobID: current.ID, Action: "updated", NextRunAtMs: ptr.Val(current.State.NextRunAtMs)})
		job = current
		return nil
	})
	if err != nil {
		return CronJob{}, err
	}
	return job, nil
}

// Remove deletes a job.
func (c *CronService) Remove(id string) (bool, error) {
	var removed bool
	err := c.withStoreLock(func() error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.warnIfDisabled("remove")
		if err := c.ensureLoaded(); err != nil {
			return err
		}
		before := len(c.store.Jobs)
		filtered := make([]CronJob, 0, len(c.store.Jobs))
		for _, job := range c.store.Jobs {
			if job.ID != id {
				filtered = append(filtered, job)
			}
		}
		removed = len(filtered) != before
		c.store.Jobs = filtered
		if err := c.persist(); err != nil {
			return err
		}
		c.armTimerLocked()
		if removed {
			c.emit(CronEvent{JobID: id, Action: "removed"})
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return removed, nil
}

// Run executes a job if due (or forced).
func (c *CronService) Run(id string, mode string) (bool, string, error) {
	c.warnIfDisabled("run")
	forced := mode == "force"
	var ran bool
	var reason string
	err := c.withStoreLock(func() error {
		c.mu.Lock()
		if err := c.ensureLoaded(); err != nil {
			c.mu.Unlock()
			return err
		}
		idx := findJobIndex(c.store.Jobs, id)
		if idx == -1 {
			c.mu.Unlock()
			return fmt.Errorf("unknown cron job id: %s", id)
		}
		job := c.store.Jobs[idx]
		if !isJobDue(job, c.deps.NowMs(), forced) {
			reason = "not-due"
			c.mu.Unlock()
			return nil
		}
		c.mu.Unlock()
		_, err := c.executeJobLocked(id, forced)
		if err == nil {
			ran = true
		}
		return err
	})
	if err != nil {
		return false, "", err
	}
	if !ran {
		return false, reason, nil
	}
	return true, "", nil
}

// Wake enqueues a system event.
func (c *CronService) Wake(mode string, text string) (bool, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false, nil
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
	_ = c.withStoreLock(func() error {
		c.mu.Lock()
		if c.running {
			c.mu.Unlock()
			return nil
		}
		c.running = true
		c.mu.Unlock()

		defer func() {
			c.mu.Lock()
			c.running = false
			c.mu.Unlock()
		}()

		c.mu.Lock()
		if err := c.ensureLoaded(); err != nil {
			c.mu.Unlock()
			return nil
		}
		due := c.dueJobIDsLocked()
		c.mu.Unlock()
		for _, jobID := range due {
			_, _ = c.executeJobLocked(jobID, false)
		}
		c.mu.Lock()
		c.armTimerLocked()
		c.mu.Unlock()
		return nil
	})
}

func (c *CronService) dueJobIDsLocked() []string {
	if c.store == nil {
		return nil
	}
	now := c.deps.NowMs()
	due := make([]string, 0)
	for _, job := range c.store.Jobs {
		if !job.Enabled || job.State.RunningAtMs != nil || job.State.NextRunAtMs == nil || now < *job.State.NextRunAtMs {
			continue
		}
		due = append(due, job.ID)
	}
	return due
}

func (c *CronService) executeJobLocked(jobID string, forced bool) (bool, error) {
	var deleted bool
	c.mu.Lock()
	if err := c.ensureLoaded(); err != nil {
		c.mu.Unlock()
		return false, err
	}
	idx := findJobIndex(c.store.Jobs, jobID)
	if idx == -1 {
		c.mu.Unlock()
		return false, fmt.Errorf("unknown cron job id: %s", jobID)
	}
	job := c.store.Jobs[idx]
	startedAt := c.deps.NowMs()
	if job.State.RunningAtMs != nil {
		startedAt = *job.State.RunningAtMs
	} else {
		job.State.RunningAtMs = &startedAt
		job.State.LastError = ""
		c.store.Jobs[idx] = job
		c.emit(CronEvent{JobID: job.ID, Action: "started", RunAtMs: startedAt})
		_ = c.persist()
	}
	c.mu.Unlock()

	finish := func(statusVal, errVal, summaryVal, outputVal string) {
		endedAt := c.deps.NowMs()
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.store == nil {
			return
		}
		idx := findJobIndex(c.store.Jobs, jobID)
		if idx == -1 {
			return
		}
		job := c.store.Jobs[idx]
		job.State.RunningAtMs = nil
		job.State.LastRunAtMs = &startedAt
		job.State.LastStatus = statusVal
		job.State.LastDurationMs = ptr.Ptr(max(0, endedAt-startedAt))
		job.State.LastError = errVal
		job.UpdatedAtMs = endedAt

		shouldDelete := job.Schedule.Kind == "at" && statusVal == "ok" && job.DeleteAfterRun
		if !shouldDelete {
			if job.Schedule.Kind == "at" && statusVal == "ok" {
				job.Enabled = false
				job.State.NextRunAtMs = nil
			} else if job.Enabled {
				job.State.NextRunAtMs = computeJobNextRunAtMs(job, endedAt)
			} else {
				job.State.NextRunAtMs = nil
			}
		}
		if !forced && job.Enabled && !shouldDelete {
			job.State.NextRunAtMs = computeJobNextRunAtMs(job, c.deps.NowMs())
		}

		c.emit(CronEvent{
			JobID:       job.ID,
			Action:      "finished",
			RunAtMs:     startedAt,
			DurationMs:  max(0, endedAt-startedAt),
			Status:      statusVal,
			Error:       errVal,
			Summary:     summaryVal,
			NextRunAtMs: ptr.Val(job.State.NextRunAtMs),
		})

		if shouldDelete {
			filtered := make([]CronJob, 0, len(c.store.Jobs))
			for _, existing := range c.store.Jobs {
				if existing.ID != job.ID {
					filtered = append(filtered, existing)
				}
			}
			c.store.Jobs = filtered
			c.emit(CronEvent{JobID: job.ID, Action: "removed"})
			deleted = true
		} else {
			c.store.Jobs[idx] = job
		}
		_ = c.persist()
		c.armTimerLocked()

		if !shouldDelete && job.SessionTarget == CronSessionIsolated {
			summaryText := strings.TrimSpace(summaryVal)
			deliveryMode := CronDeliveryAnnounce
			if job.Delivery != nil && strings.TrimSpace(string(job.Delivery.Mode)) != "" {
				deliveryMode = job.Delivery.Mode
			}
			if summaryText != "" && deliveryMode != CronDeliveryNone && c.deps.EnqueueSystemEvent != nil {
				prefix := "Cron"
				label := fmt.Sprintf("%s: %s", prefix, summaryText)
				if statusVal != "ok" {
					label = fmt.Sprintf("%s (%s): %s", prefix, statusVal, summaryText)
				}
				_ = c.deps.EnqueueSystemEvent(strings.TrimSpace(label), job.AgentID)
				if job.WakeMode == CronWakeNow && c.deps.RequestHeartbeatNow != nil {
					c.deps.RequestHeartbeatNow("cron:" + job.ID + ":post")
				}
			}
		}
	}

	if job.SessionTarget == CronSessionMain {
		text, reason := resolveJobPayloadTextForMain(job)
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
	status, summary, output, runErr := c.deps.RunIsolatedAgentJob(job, job.Payload.Message)
	if runErr != nil {
		finish("error", runErr.Error(), summary, output)
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
	if cached := getCachedStore(c.deps.StorePath); cached != nil {
		c.store = cached
		return nil
	}
	if c.deps.Store == nil {
		return fmt.Errorf("cron store backend not configured")
	}
	store, err := LoadCronStore(context.Background(), c.deps.Store, c.deps.StorePath)
	if err != nil {
		return err
	}
	c.store = &store
	// fix names/description
	mutated := false
	for i := range c.store.Jobs {
		job := c.store.Jobs[i]
		name := strings.TrimSpace(job.Name)
		if name == "" {
			name = inferLegacyName(&CronJobCreate{Payload: job.Payload, Schedule: job.Schedule})
			mutated = true
		} else if name != job.Name {
			mutated = true
		}
		job.Name = name
		if strings.TrimSpace(job.Description) != job.Description {
			job.Description = strings.TrimSpace(job.Description)
			mutated = true
		}
		c.store.Jobs[i] = job
	}
	if mutated {
		setCachedStore(c.deps.StorePath, c.store)
		return c.persist()
	}
	setCachedStore(c.deps.StorePath, c.store)
	return nil
}

func (c *CronService) persist() error {
	if c.store == nil {
		return nil
	}
	if c.deps.Store == nil {
		return fmt.Errorf("cron store backend not configured")
	}
	return SaveCronStore(context.Background(), c.deps.Store, c.deps.StorePath, *c.store)
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
	delayMs := max(0, *next-c.deps.NowMs())
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
