package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/appservice"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/matrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	integrationcron "github.com/beeper/ai-bridge/pkg/integrations/cron"
)

const (
	schedulePlannerHorizon        = 6*24*time.Hour + 23*time.Hour
	scheduleImmediateDelay        = 2 * time.Second
	scheduleHeartbeatCoalesce     = 60 * time.Second
	defaultCronTimeoutSeconds     = 600
	defaultScheduleEventSource    = "schedule"
	scheduleTickKindCronRun       = "cron-run"
	scheduleTickKindCronPlan      = "cron-plan"
	scheduleTickKindHeartbeatRun  = "heartbeat-run"
	scheduleTickKindHeartbeatPlan = "heartbeat-plan"
)

type schedulerRuntime struct {
	client *AIClient
	mu     sync.Mutex
}

type scheduledCronStore struct {
	Jobs []scheduledCronJob `json:"jobs"`
}

type scheduledCronJob struct {
	Job               integrationcron.Job `json:"job"`
	RoomID            string              `json:"roomId,omitempty"`
	Revision          int                 `json:"revision,omitempty"`
	PendingDelayID    string              `json:"pendingDelayId,omitempty"`
	PendingDelayKind  string              `json:"pendingDelayKind,omitempty"`
	PendingRunKey     string              `json:"pendingRunKey,omitempty"`
	LastOutputPreview string              `json:"lastOutputPreview,omitempty"`
	ProcessedRunKeys  []string            `json:"processedRunKeys,omitempty"`
}

type managedHeartbeatStore struct {
	Agents []managedHeartbeatState `json:"agents"`
}

type managedHeartbeatState struct {
	AgentID          string                      `json:"agentId"`
	Enabled          bool                        `json:"enabled"`
	IntervalMs       int64                       `json:"intervalMs"`
	ActiveHours      *HeartbeatActiveHoursConfig `json:"activeHours,omitempty"`
	RoomID           string                      `json:"roomId,omitempty"`
	Revision         int                         `json:"revision,omitempty"`
	NextRunAtMs      int64                       `json:"nextRunAtMs,omitempty"`
	PendingDelayID   string                      `json:"pendingDelayId,omitempty"`
	PendingDelayKind string                      `json:"pendingDelayKind,omitempty"`
	PendingRunKey    string                      `json:"pendingRunKey,omitempty"`
	LastRunAtMs      int64                       `json:"lastRunAtMs,omitempty"`
	LastResult       string                      `json:"lastResult,omitempty"`
	LastError        string                      `json:"lastError,omitempty"`
	ProcessedRunKeys []string                    `json:"processedRunKeys,omitempty"`
}

func newSchedulerRuntime(client *AIClient) *schedulerRuntime {
	return &schedulerRuntime{client: client}
}

func (s *schedulerRuntime) Start(ctx context.Context) {
	if s == nil || s.client == nil {
		return
	}
	if err := s.reconcile(ctx); err != nil {
		s.client.log.Warn().Err(err).Msg("Failed to reconcile scheduler state")
	}
}

func (s *schedulerRuntime) CronStatus(ctx context.Context) (bool, string, int, *int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.loadCronStoreLocked(ctx)
	if err != nil {
		return false, "sqlite:ai_cron_jobs", 0, nil, err
	}
	var next *int64
	for i := range store.Jobs {
		job := &store.Jobs[i]
		if !job.Job.Enabled || job.Job.State.NextRunAtMs == nil || *job.Job.State.NextRunAtMs <= 0 {
			continue
		}
		if next == nil || *job.Job.State.NextRunAtMs < *next {
			val := *job.Job.State.NextRunAtMs
			next = &val
		}
	}
	return true, "sqlite:ai_cron_jobs", len(store.Jobs), next, nil
}

func (s *schedulerRuntime) CronList(ctx context.Context, includeDisabled bool) ([]integrationcron.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.loadCronStoreLocked(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]integrationcron.Job, 0, len(store.Jobs))
	for _, job := range store.Jobs {
		if !includeDisabled && !job.Job.Enabled {
			continue
		}
		out = append(out, job.Job)
	}
	slices.SortFunc(out, func(a, b integrationcron.Job) int {
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

func (s *schedulerRuntime) CronAdd(ctx context.Context, jobInput integrationcron.JobCreate) (integrationcron.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	nowMs := time.Now().UnixMilli()
	jobInput = integrationcron.NormalizeJobCreate(jobInput)
	if err := normalizeCronCreateForScheduler(&jobInput); err != nil {
		return integrationcron.Job{}, err
	}
	if result := integrationcron.ValidateSchedule(jobInput.Schedule); !result.Ok {
		return integrationcron.Job{}, errors.New(result.Message)
	}
	if result := integrationcron.ValidateScheduleTimestamp(jobInput.Schedule, nowMs); !result.Ok {
		return integrationcron.Job{}, errors.New(result.Message)
	}

	store, err := s.loadCronStoreLocked(ctx)
	if err != nil {
		return integrationcron.Job{}, err
	}

	job := integrationcron.Job{
		ID:             uuid.NewString(),
		AgentID:        normalizedCronAgentID(jobInput.AgentID),
		Name:           resolveCronJobName(jobInput),
		Description:    optionalCronDescription(jobInput.Description),
		Enabled:        cronCreateEnabled(jobInput.Enabled),
		DeleteAfterRun: cronDeleteAfterRun(jobInput),
		CreatedAtMs:    nowMs,
		UpdatedAtMs:    nowMs,
		Schedule:       jobInput.Schedule,
		Payload:        jobInput.Payload,
		Delivery:       normalizeCronDelivery(jobInput.Delivery),
	}
	record := scheduledCronJob{Job: job, Revision: 1}
	if err := s.ensureCronRoomLocked(ctx, &record); err != nil {
		return integrationcron.Job{}, err
	}
	s.scheduleCronRecordLocked(ctx, &record, nowMs, false)

	store.Jobs = append(store.Jobs, record)
	if err := s.saveCronStoreLocked(ctx, store); err != nil {
		return integrationcron.Job{}, err
	}
	return record.Job, nil
}

func (s *schedulerRuntime) CronUpdate(ctx context.Context, jobID string, patch integrationcron.JobPatch) (integrationcron.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.loadCronStoreLocked(ctx)
	if err != nil {
		return integrationcron.Job{}, err
	}
	idx := findScheduledCronJob(store.Jobs, jobID)
	if idx < 0 {
		return integrationcron.Job{}, fmt.Errorf("cron job not found: %s", strings.TrimSpace(jobID))
	}
	record := store.Jobs[idx]
	if err := normalizeCronPatchForScheduler(&patch); err != nil {
		return integrationcron.Job{}, err
	}
	updated, err := applyScheduledCronPatch(record, patch, time.Now().UnixMilli())
	if err != nil {
		return integrationcron.Job{}, err
	}
	if err := s.cancelPendingDelayLocked(ctx, record.PendingDelayID); err != nil {
		s.client.log.Warn().Err(err).Str("job_id", record.Job.ID).Msg("Failed to cancel pending cron delay during update")
	}
	record = updated
	if err := s.ensureCronRoomLocked(ctx, &record); err != nil {
		return integrationcron.Job{}, err
	}
	s.scheduleCronRecordLocked(ctx, &record, time.Now().UnixMilli(), false)
	store.Jobs[idx] = record
	if err := s.saveCronStoreLocked(ctx, store); err != nil {
		return integrationcron.Job{}, err
	}
	return record.Job, nil
}

func (s *schedulerRuntime) CronRemove(ctx context.Context, jobID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.loadCronStoreLocked(ctx)
	if err != nil {
		return false, err
	}
	idx := findScheduledCronJob(store.Jobs, jobID)
	if idx < 0 {
		return false, nil
	}
	record := store.Jobs[idx]
	if err := s.cancelPendingDelayLocked(ctx, record.PendingDelayID); err != nil {
		s.client.log.Warn().Err(err).Str("job_id", record.Job.ID).Msg("Failed to cancel pending cron delay during remove")
	}
	store.Jobs = append(store.Jobs[:idx], store.Jobs[idx+1:]...)
	if err := s.saveCronStoreLocked(ctx, store); err != nil {
		return false, err
	}
	return true, nil
}

func (s *schedulerRuntime) CronRun(ctx context.Context, jobID string) (bool, string, error) {
	s.mu.Lock()
	store, err := s.loadCronStoreLocked(ctx)
	if err != nil {
		s.mu.Unlock()
		return false, "", err
	}
	idx := findScheduledCronJob(store.Jobs, jobID)
	if idx < 0 {
		s.mu.Unlock()
		return false, "not-found", nil
	}
	record := store.Jobs[idx]
	if !record.Job.Enabled {
		s.mu.Unlock()
		return false, "disabled", nil
	}
	s.mu.Unlock()

	tick := ScheduleTickContent{
		Kind:           scheduleTickKindCronRun,
		EntityID:       record.Job.ID,
		Revision:       record.Revision,
		ScheduledForMs: time.Now().UnixMilli(),
		RunKey:         buildTickRunKey(record.Revision, "manual", time.Now().UnixMilli()),
		Reason:         "manual",
	}
	if err := s.handleCronRun(context.Background(), tick, true); err != nil {
		return false, "", err
	}
	return true, "", nil
}

func (s *schedulerRuntime) RunHeartbeatSweep(ctx context.Context, reason string) (string, string) {
	if s == nil || s.client == nil {
		return "skipped", "disabled"
	}
	agents := resolveHeartbeatAgents(&s.client.connector.Config)
	if len(agents) == 0 {
		return "skipped", "disabled"
	}
	ran := false
	for _, agent := range agents {
		res := s.client.runHeartbeatOnce(agent.agentID, agent.heartbeat, reason)
		if res.Status == "skipped" && res.Reason == "requests-in-flight" {
			return res.Status, res.Reason
		}
		if res.Status == "ran" {
			ran = true
		}
	}
	if ran {
		return "ran", ""
	}
	return "skipped", "disabled"
}

func (s *schedulerRuntime) RequestHeartbeatNow(ctx context.Context, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.loadHeartbeatStoreLocked(ctx)
	if err != nil {
		s.client.log.Warn().Err(err).Msg("Failed to load managed heartbeat store")
		return
	}
	nowMs := time.Now().UnixMilli()
	changed := false
	for _, agent := range resolveHeartbeatAgents(&s.client.connector.Config) {
		state := upsertManagedHeartbeat(&store, agent.agentID, agent.heartbeat)
		if state == nil || !state.Enabled {
			continue
		}
		if state.NextRunAtMs > 0 && state.NextRunAtMs-nowMs <= int64(scheduleHeartbeatCoalesce/time.Millisecond) {
			continue
		}
		if err := s.ensureHeartbeatRoomLocked(ctx, state); err != nil {
			s.client.log.Warn().Err(err).Str("agent_id", agent.agentID).Msg("Failed to ensure heartbeat room for immediate wake")
			continue
		}
		if _, err := s.scheduleTickLocked(ctx, id.RoomID(state.RoomID), ScheduleTickContent{
			Kind:           scheduleTickKindHeartbeatRun,
			EntityID:       state.AgentID,
			Revision:       state.Revision,
			ScheduledForMs: nowMs,
			RunKey:         buildTickRunKey(state.Revision, "wake", nowMs),
			Reason:         strings.TrimSpace(reason),
		}, scheduleImmediateDelay); err != nil {
			s.client.log.Warn().Err(err).Str("agent_id", agent.agentID).Msg("Failed to schedule immediate heartbeat tick")
			continue
		}
		changed = true
	}
	if changed {
		if err := s.saveHeartbeatStoreLocked(ctx, store); err != nil {
			s.client.log.Warn().Err(err).Msg("Failed to save managed heartbeat store after wake")
		}
	}
}

func (s *schedulerRuntime) HandleScheduleTick(ctx context.Context, evt *event.Event, portal *bridgev2.Portal) {
	if s == nil || s.client == nil || evt == nil {
		return
	}
	var tick ScheduleTickContent
	if err := json.Unmarshal(evt.Content.VeryRaw, &tick); err != nil {
		s.client.log.Warn().Err(err).Stringer("event_id", evt.ID).Msg("Failed to decode schedule tick")
		return
	}
	switch tick.Kind {
	case scheduleTickKindCronPlan:
		if err := s.handleCronPlan(ctx, tick); err != nil {
			s.client.log.Warn().Err(err).Str("job_id", tick.EntityID).Msg("Failed to handle cron planner tick")
		}
	case scheduleTickKindCronRun:
		if err := s.handleCronRun(ctx, tick, false); err != nil {
			s.client.log.Warn().Err(err).Str("job_id", tick.EntityID).Msg("Failed to handle cron run tick")
		}
	case scheduleTickKindHeartbeatPlan:
		if err := s.handleHeartbeatPlan(ctx, tick); err != nil {
			s.client.log.Warn().Err(err).Str("agent_id", tick.EntityID).Msg("Failed to handle heartbeat planner tick")
		}
	case scheduleTickKindHeartbeatRun:
		if err := s.handleHeartbeatRun(ctx, tick); err != nil {
			s.client.log.Warn().Err(err).Str("agent_id", tick.EntityID).Msg("Failed to handle heartbeat run tick")
		}
	default:
		s.client.log.Debug().Str("kind", tick.Kind).Msg("Ignoring unknown schedule tick kind")
	}
}

func (s *schedulerRuntime) reconcile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.reconcileCronLocked(ctx); err != nil {
		return err
	}
	return s.reconcileHeartbeatLocked(ctx)
}

func (s *schedulerRuntime) reconcileCronLocked(ctx context.Context) error {
	store, err := s.loadCronStoreLocked(ctx)
	if err != nil {
		return err
	}
	nowMs := time.Now().UnixMilli()
	for idx := range store.Jobs {
		record := &store.Jobs[idx]
		if record.Revision <= 0 {
			record.Revision = 1
		}
		if err := s.ensureCronRoomLocked(ctx, record); err != nil {
			return err
		}
		s.scheduleCronRecordLocked(ctx, record, nowMs, true)
	}
	return s.saveCronStoreLocked(ctx, store)
}

func (s *schedulerRuntime) reconcileHeartbeatLocked(ctx context.Context) error {
	store, err := s.loadHeartbeatStoreLocked(ctx)
	if err != nil {
		return err
	}
	nowMs := time.Now().UnixMilli()
	active := make(map[string]struct{})
	for _, agent := range resolveHeartbeatAgents(&s.client.connector.Config) {
		active[agent.agentID] = struct{}{}
		state := upsertManagedHeartbeat(&store, agent.agentID, agent.heartbeat)
		if state == nil || !state.Enabled {
			continue
		}
		if err := s.ensureHeartbeatRoomLocked(ctx, state); err != nil {
			return err
		}
		s.scheduleHeartbeatStateLocked(ctx, state, nowMs, true)
	}
	for i := range store.Agents {
		state := &store.Agents[i]
		if _, ok := active[state.AgentID]; ok {
			continue
		}
		state.Enabled = false
		state.NextRunAtMs = 0
		if err := s.cancelPendingDelayLocked(ctx, state.PendingDelayID); err != nil {
			s.client.log.Warn().Err(err).Str("agent_id", state.AgentID).Msg("Failed to cancel disabled heartbeat delay")
		}
		state.PendingDelayID = ""
		state.PendingDelayKind = ""
		state.PendingRunKey = ""
	}
	return s.saveHeartbeatStoreLocked(ctx, store)
}

func (s *schedulerRuntime) handleCronPlan(ctx context.Context, tick ScheduleTickContent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.loadCronStoreLocked(ctx)
	if err != nil {
		return err
	}
	idx := findScheduledCronJob(store.Jobs, tick.EntityID)
	if idx < 0 {
		return nil
	}
	record := &store.Jobs[idx]
	if !record.Job.Enabled || tick.Revision != record.Revision || containsRunKey(record.ProcessedRunKeys, tick.RunKey) {
		return nil
	}
	record.PendingDelayID = ""
	record.PendingDelayKind = ""
	record.PendingRunKey = ""
	record.ProcessedRunKeys = appendRunKey(record.ProcessedRunKeys, tick.RunKey)
	s.scheduleCronRecordLocked(ctx, record, time.Now().UnixMilli(), false)
	return s.saveCronStoreLocked(ctx, store)
}

func (s *schedulerRuntime) handleCronRun(ctx context.Context, tick ScheduleTickContent, manual bool) error {
	s.mu.Lock()
	store, err := s.loadCronStoreLocked(ctx)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	idx := findScheduledCronJob(store.Jobs, tick.EntityID)
	if idx < 0 {
		s.mu.Unlock()
		return nil
	}
	record := store.Jobs[idx]
	if !record.Job.Enabled || tick.Revision != record.Revision || containsRunKey(record.ProcessedRunKeys, tick.RunKey) {
		s.mu.Unlock()
		return nil
	}
	nowMs := time.Now().UnixMilli()
	record.PendingDelayID = ""
	record.PendingDelayKind = ""
	record.PendingRunKey = ""
	record.Job.State.RunningAtMs = &nowMs
	if !manual {
		s.scheduleNextCronAfterRunLocked(ctx, &record, tick.ScheduledForMs, nowMs)
	}
	store.Jobs[idx] = record
	if err := s.saveCronStoreLocked(ctx, store); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	status, errText, preview := s.executeCronJob(ctx, &record)

	s.mu.Lock()
	defer s.mu.Unlock()
	store, err = s.loadCronStoreLocked(ctx)
	if err != nil {
		return err
	}
	idx = findScheduledCronJob(store.Jobs, tick.EntityID)
	if idx < 0 {
		return nil
	}
	record = store.Jobs[idx]
	finishedAt := time.Now().UnixMilli()
	record.Job.State.RunningAtMs = nil
	record.Job.State.LastRunAtMs = &finishedAt
	record.Job.State.LastStatus = status
	record.Job.State.LastError = errText
	record.Job.State.LastDurationMs = nil
	record.LastOutputPreview = preview
	record.ProcessedRunKeys = appendRunKey(record.ProcessedRunKeys, tick.RunKey)
	record.Job.UpdatedAtMs = finishedAt
	if strings.EqualFold(strings.TrimSpace(record.Job.Schedule.Kind), "at") {
		record.Job.Enabled = false
		record.Job.State.NextRunAtMs = nil
		record.PendingDelayID = ""
		record.PendingDelayKind = ""
		record.PendingRunKey = ""
	}
	store.Jobs[idx] = record
	return s.saveCronStoreLocked(ctx, store)
}

func (s *schedulerRuntime) handleHeartbeatPlan(ctx context.Context, tick ScheduleTickContent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	store, err := s.loadHeartbeatStoreLocked(ctx)
	if err != nil {
		return err
	}
	idx := findManagedHeartbeat(store.Agents, tick.EntityID)
	if idx < 0 {
		return nil
	}
	state := &store.Agents[idx]
	if !state.Enabled || state.Revision != tick.Revision || containsRunKey(state.ProcessedRunKeys, tick.RunKey) {
		return nil
	}
	state.PendingDelayID = ""
	state.PendingDelayKind = ""
	state.PendingRunKey = ""
	state.ProcessedRunKeys = appendRunKey(state.ProcessedRunKeys, tick.RunKey)
	s.scheduleHeartbeatStateLocked(ctx, state, time.Now().UnixMilli(), false)
	return s.saveHeartbeatStoreLocked(ctx, store)
}

func (s *schedulerRuntime) handleHeartbeatRun(ctx context.Context, tick ScheduleTickContent) error {
	s.mu.Lock()
	store, err := s.loadHeartbeatStoreLocked(ctx)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	idx := findManagedHeartbeat(store.Agents, tick.EntityID)
	if idx < 0 {
		s.mu.Unlock()
		return nil
	}
	state := store.Agents[idx]
	if !state.Enabled || state.Revision != tick.Revision || containsRunKey(state.ProcessedRunKeys, tick.RunKey) {
		s.mu.Unlock()
		return nil
	}
	nowMs := time.Now().UnixMilli()
	state.PendingDelayID = ""
	state.PendingDelayKind = ""
	state.PendingRunKey = ""
	s.scheduleNextHeartbeatAfterRunLocked(ctx, &state, nowMs)
	store.Agents[idx] = state
	if err := s.saveHeartbeatStoreLocked(ctx, store); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	reason := strings.TrimSpace(tick.Reason)
	if reason == "" {
		reason = "interval"
	}
	hb := resolveHeartbeatConfig(&s.client.connector.Config, state.AgentID)
	res := s.client.runHeartbeatOnce(state.AgentID, hb, reason)

	s.mu.Lock()
	defer s.mu.Unlock()
	store, err = s.loadHeartbeatStoreLocked(ctx)
	if err != nil {
		return err
	}
	idx = findManagedHeartbeat(store.Agents, tick.EntityID)
	if idx < 0 {
		return nil
	}
	state = store.Agents[idx]
	state.LastRunAtMs = time.Now().UnixMilli()
	state.LastResult = res.Status
	state.LastError = res.Reason
	state.ProcessedRunKeys = appendRunKey(state.ProcessedRunKeys, tick.RunKey)
	store.Agents[idx] = state
	return s.saveHeartbeatStoreLocked(ctx, store)
}

func (s *schedulerRuntime) executeCronJob(ctx context.Context, record *scheduledCronJob) (string, string, string) {
	if s == nil || s.client == nil || record == nil {
		return "error", "missing scheduler", ""
	}
	portal := s.client.portalByRoomID(ctx, id.RoomID(record.RoomID))
	if portal == nil || portal.MXID == "" {
		return "error", "cron room not found", ""
	}
	meta := clonePortalMetadata(portalMeta(portal))
	if meta == nil {
		meta = &PortalMetadata{}
	}
	meta.AgentID = normalizedCronAgentID(&record.Job.AgentID)
	if model := strings.TrimSpace(record.Job.Payload.Model); model != "" {
		meta.Model = model
	}
	if thinking := strings.TrimSpace(record.Job.Payload.Thinking); thinking != "" {
		meta.ReasoningEffort = thinking
	}
	if record.Job.Delivery != nil && record.Job.Delivery.Mode == integrationcron.DeliveryAnnounce {
		meta.DisabledTools = appendMissingDisabledTool(meta.DisabledTools, "message")
	}

	timeoutSeconds := resolveScheduledCronTimeoutSeconds(s.client, record.Job.Payload.TimeoutSeconds)
	runCtx, cancel := context.WithTimeout(s.client.backgroundContext(ctx), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	userTimezone, _ := s.client.resolveUserTimezone()
	message := integrationcron.BuildCronMessage(record.Job.ID, record.Job.Name, resolveCronPayloadMessage(record.Job.Payload), userTimezone)
	if record.Job.Payload.AllowUnsafeExternal == nil || !*record.Job.Payload.AllowUnsafeExternal {
		message = integrationcron.WrapSafeExternalPrompt(message)
	}
	lastID, lastTS := s.client.lastAssistantMessageInfo(runCtx, portal)
	if _, _, err := s.client.dispatchInternalMessage(runCtx, portal, meta, message, defaultScheduleEventSource, false); err != nil {
		return "error", err.Error(), ""
	}

	msg, found := s.client.waitForNewAssistantMessage(runCtx, portal, lastID, lastTS)
	if !found || msg == nil {
		return "error", "timed out waiting for cron response", ""
	}
	body := ""
	model := ""
	if meta := messageMeta(msg); meta != nil {
		body = strings.TrimSpace(meta.Body)
		model = strings.TrimSpace(meta.Model)
	}
	if body == "" {
		body = strings.TrimSpace(msg.MXID.String())
	}
	preview := truncateSchedulePreview(body)
	if record.Job.Delivery != nil && record.Job.Delivery.Mode == integrationcron.DeliveryAnnounce {
		target := s.resolveCronDeliveryTarget(record.Job.AgentID, record.Job.Delivery)
		if target.Portal == nil || strings.TrimSpace(target.RoomID) == "" {
			return "skipped", "delivery target unavailable", preview
		}
		if err := s.client.sendPlainAssistantMessageWithResult(runCtx, target.Portal.(*bridgev2.Portal), body); err != nil {
			return "error", err.Error(), preview
		}
	}
	_ = model
	return "success", "", preview
}

func (s *schedulerRuntime) resolveCronDeliveryTarget(agentID string, delivery *integrationcron.Delivery) integrationcron.DeliveryTarget {
	return integrationcron.ResolveCronDeliveryTarget(agentID, delivery, integrationcron.DeliveryResolverDeps{
		ResolveLastTarget: func(agentID string) (channel string, target string, ok bool) {
			ref, mainKey := s.client.resolveHeartbeatMainSessionRef(agentID)
			entry, found := s.client.getSessionEntry(context.Background(), ref, mainKey)
			if !found {
				return "", "", false
			}
			return entry.LastChannel, entry.LastTo, true
		},
		IsStaleTarget: func(roomID string, agentID string) bool {
			portal := s.client.portalByRoomID(context.Background(), id.RoomID(roomID))
			if portal == nil {
				return true
			}
			meta := portalMeta(portal)
			return meta != nil && normalizeAgentID(meta.AgentID) != normalizeAgentID(agentID)
		},
		LastActiveRoomID: func(agentID string) string {
			if portal := s.client.lastActivePortal(agentID); portal != nil && portal.MXID != "" {
				return portal.MXID.String()
			}
			return ""
		},
		DefaultChatRoomID: func() string {
			if portal := s.client.defaultChatPortal(); portal != nil && portal.MXID != "" {
				return portal.MXID.String()
			}
			return ""
		},
		ResolvePortalByRoom: func(roomID string) any {
			return s.client.portalByRoomID(context.Background(), id.RoomID(roomID))
		},
		IsLoggedIn: s.client.IsLoggedIn,
	})
}

func (s *schedulerRuntime) scheduleCronRecordLocked(ctx context.Context, record *scheduledCronJob, nowMs int64, validateExisting bool) {
	if record == nil {
		return
	}
	due := computeInitialCronDue(record.Job, nowMs)
	if due == nil || !record.Job.Enabled {
		record.Job.State.NextRunAtMs = nil
		record.PendingDelayID = ""
		record.PendingDelayKind = ""
		record.PendingRunKey = ""
		return
	}
	if validateExisting && record.PendingDelayID != "" && s.delayedEventExistsLocked(ctx, record.PendingDelayID) {
		record.Job.State.NextRunAtMs = due
		return
	}
	if record.PendingDelayID != "" {
		_ = s.cancelPendingDelayLocked(ctx, record.PendingDelayID)
	}
	s.scheduleCronDueLocked(ctx, record, *due)
}

func (s *schedulerRuntime) scheduleCronDueLocked(ctx context.Context, record *scheduledCronJob, dueAtMs int64) {
	if record == nil {
		return
	}
	nowMs := time.Now().UnixMilli()
	runAtMs := dueAtMs
	kind := scheduleTickKindCronRun
	if dueAtMs-nowMs > int64(schedulePlannerHorizon/time.Millisecond) {
		runAtMs = nowMs + int64(schedulePlannerHorizon/time.Millisecond)
		kind = scheduleTickKindCronPlan
	}
	resp, err := s.scheduleTickLocked(ctx, id.RoomID(record.RoomID), ScheduleTickContent{
		Kind:           kind,
		EntityID:       record.Job.ID,
		Revision:       record.Revision,
		ScheduledForMs: runAtMs,
		RunKey:         buildTickRunKey(record.Revision, shortTickKind(kind), runAtMs),
		Reason:         "interval",
	}, time.Duration(max64(runAtMs-nowMs, scheduleImmediateDelay.Milliseconds()))*time.Millisecond)
	if err != nil {
		s.client.log.Warn().Err(err).Str("job_id", record.Job.ID).Msg("Failed to schedule cron tick")
		record.Job.State.LastStatus = "error"
		record.Job.State.LastError = err.Error()
		return
	}
	record.Job.State.NextRunAtMs = &dueAtMs
	record.PendingDelayID = string(resp.UnstableDelayID)
	record.PendingDelayKind = shortTickKind(kind)
	record.PendingRunKey = buildTickRunKey(record.Revision, shortTickKind(kind), runAtMs)
}

func (s *schedulerRuntime) scheduleNextCronAfterRunLocked(ctx context.Context, record *scheduledCronJob, scheduledForMs, nowMs int64) {
	if record == nil {
		return
	}
	next := computeNextCronAfterRun(record.Job, scheduledForMs, nowMs)
	if next == nil {
		record.Job.State.NextRunAtMs = nil
		return
	}
	s.scheduleCronDueLocked(ctx, record, *next)
}

func (s *schedulerRuntime) scheduleHeartbeatStateLocked(ctx context.Context, state *managedHeartbeatState, nowMs int64, validateExisting bool) {
	if state == nil || !state.Enabled || state.IntervalMs <= 0 {
		if state != nil {
			state.NextRunAtMs = 0
			state.PendingDelayID = ""
			state.PendingDelayKind = ""
			state.PendingRunKey = ""
		}
		return
	}
	nextRun := computeManagedHeartbeatDue(s.client, *state, nowMs)
	if nextRun <= 0 {
		return
	}
	if validateExisting && state.PendingDelayID != "" && s.delayedEventExistsLocked(ctx, state.PendingDelayID) {
		state.NextRunAtMs = nextRun
		return
	}
	if state.PendingDelayID != "" {
		_ = s.cancelPendingDelayLocked(ctx, state.PendingDelayID)
	}
	kind := scheduleTickKindHeartbeatRun
	runAtMs := nextRun
	if nextRun-nowMs > int64(schedulePlannerHorizon/time.Millisecond) {
		kind = scheduleTickKindHeartbeatPlan
		runAtMs = nowMs + int64(schedulePlannerHorizon/time.Millisecond)
	}
	resp, err := s.scheduleTickLocked(ctx, id.RoomID(state.RoomID), ScheduleTickContent{
		Kind:           kind,
		EntityID:       state.AgentID,
		Revision:       state.Revision,
		ScheduledForMs: runAtMs,
		RunKey:         buildTickRunKey(state.Revision, shortTickKind(kind), runAtMs),
		Reason:         "interval",
	}, time.Duration(max64(runAtMs-nowMs, scheduleImmediateDelay.Milliseconds()))*time.Millisecond)
	if err != nil {
		s.client.log.Warn().Err(err).Str("agent_id", state.AgentID).Msg("Failed to schedule managed heartbeat tick")
		state.LastResult = "error"
		state.LastError = err.Error()
		return
	}
	state.NextRunAtMs = nextRun
	state.PendingDelayID = string(resp.UnstableDelayID)
	state.PendingDelayKind = shortTickKind(kind)
	state.PendingRunKey = buildTickRunKey(state.Revision, shortTickKind(kind), runAtMs)
}

func (s *schedulerRuntime) scheduleNextHeartbeatAfterRunLocked(ctx context.Context, state *managedHeartbeatState, nowMs int64) {
	if state == nil {
		return
	}
	state.NextRunAtMs = nowMs + state.IntervalMs
	s.scheduleHeartbeatStateLocked(ctx, state, nowMs, false)
}

func (s *schedulerRuntime) scheduleTickLocked(ctx context.Context, roomID id.RoomID, content ScheduleTickContent, delay time.Duration) (*mautrix.RespSendEvent, error) {
	intent := s.intentClient()
	if intent == nil {
		return nil, errors.New("matrix intent not available")
	}
	if delay < scheduleImmediateDelay {
		delay = scheduleImmediateDelay
	}
	resp, err := intent.SendMessageEvent(ctx, roomID, ScheduleTickEventType, content, mautrix.ReqSendEvent{UnstableDelay: delay})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (s *schedulerRuntime) delayedEventExistsLocked(ctx context.Context, delayID string) bool {
	intent := s.intentClient()
	if intent == nil || strings.TrimSpace(delayID) == "" {
		return false
	}
	resp, err := intent.DelayedEvents(ctx, &mautrix.ReqDelayedEvents{DelayID: id.DelayID(delayID)})
	if err != nil {
		return false
	}
	return resp != nil
}

func (s *schedulerRuntime) cancelPendingDelayLocked(ctx context.Context, delayID string) error {
	intent := s.intentClient()
	if intent == nil || strings.TrimSpace(delayID) == "" {
		return nil
	}
	_, err := intent.UpdateDelayedEvent(ctx, &mautrix.ReqUpdateDelayedEvent{
		DelayID: id.DelayID(delayID),
		Action:  event.DelayActionCancel,
	})
	return err
}

func (s *schedulerRuntime) ensureCronRoomLocked(ctx context.Context, record *scheduledCronJob) error {
	if record == nil {
		return nil
	}
	portalID := fmt.Sprintf("cron:%s:%s", normalizeAgentID(record.Job.AgentID), strings.TrimSpace(record.Job.ID))
	portal, err := s.getOrCreateScheduledPortal(ctx, portalID, fmt.Sprintf("Cron: %s", strings.TrimSpace(record.Job.Name)), func(meta *PortalMetadata) {
		meta.AgentID = normalizeAgentID(record.Job.AgentID)
		if meta.ModuleMeta == nil {
			meta.ModuleMeta = make(map[string]any)
		}
		meta.ModuleMeta["cron"] = map[string]any{
			"is_internal_room": true,
			"backend":          "hungry",
			"job_id":           record.Job.ID,
			"revision":         record.Revision,
			"managed":          true,
		}
	})
	if err != nil {
		return err
	}
	record.RoomID = portal.MXID.String()
	return nil
}

func (s *schedulerRuntime) ensureHeartbeatRoomLocked(ctx context.Context, state *managedHeartbeatState) error {
	if state == nil {
		return nil
	}
	portalID := fmt.Sprintf("heartbeat:%s", normalizeAgentID(state.AgentID))
	portal, err := s.getOrCreateScheduledPortal(ctx, portalID, fmt.Sprintf("Heartbeat: %s", state.AgentID), func(meta *PortalMetadata) {
		meta.AgentID = normalizeAgentID(state.AgentID)
		if meta.ModuleMeta == nil {
			meta.ModuleMeta = make(map[string]any)
		}
		meta.ModuleMeta["heartbeat"] = map[string]any{
			"is_internal_room": true,
			"backend":          "hungry",
			"agent_id":         state.AgentID,
			"revision":         state.Revision,
			"managed":          true,
		}
	})
	if err != nil {
		return err
	}
	state.RoomID = portal.MXID.String()
	return nil
}

func (s *schedulerRuntime) getOrCreateScheduledPortal(ctx context.Context, portalID, displayName string, setup func(meta *PortalMetadata)) (*bridgev2.Portal, error) {
	key := portalKeyFromParts(s.client, portalID, string(s.client.UserLogin.ID))
	portal, err := s.client.UserLogin.Bridge.GetPortalByKey(ctx, key)
	if err != nil {
		return nil, err
	}
	if portal.MXID != "" {
		meta := portalMeta(portal)
		if meta == nil {
			meta = &PortalMetadata{}
			portal.Metadata = meta
		}
		if setup != nil {
			setup(meta)
		}
		s.client.savePortalQuiet(ctx, portal, "scheduler metadata update")
		return portal, nil
	}
	meta := &PortalMetadata{}
	if setup != nil {
		setup(meta)
	}
	portal.Metadata = meta
	portal.Name = displayName
	portal.NameSet = true
	if err := portal.Save(ctx); err != nil {
		return nil, err
	}
	chatInfo := &bridgev2.ChatInfo{Name: &portal.Name}
	if err := portal.CreateMatrixRoom(ctx, s.client.UserLogin, chatInfo); err != nil {
		return nil, err
	}
	return portal, nil
}

func (s *schedulerRuntime) intentClient() *appservice.IntentAPI {
	if s == nil || s.client == nil || s.client.UserLogin == nil || s.client.UserLogin.Bridge == nil {
		return nil
	}
	bot, ok := s.client.UserLogin.Bridge.Bot.(*matrix.ASIntent)
	if !ok || bot == nil {
		return nil
	}
	return bot.Matrix
}

func normalizeCronCreateForScheduler(input *integrationcron.JobCreate) error {
	if input == nil {
		return errors.New("cron job is required")
	}
	if err := normalizeCronPayloadAlias(&input.Payload); err != nil {
		return err
	}
	input.Delivery = normalizeCronDelivery(input.Delivery)
	return nil
}

func normalizeCronPatchForScheduler(patch *integrationcron.JobPatch) error {
	if patch == nil {
		return errors.New("cron patch is required")
	}
	if patch.Payload != nil {
		payload := integrationcron.Payload{
			Kind:                patch.Payload.Kind,
			Model:               derefString(patch.Payload.Model),
			Text:                derefString(patch.Payload.Text),
			Message:             derefString(patch.Payload.Message),
			Thinking:            derefString(patch.Payload.Thinking),
			TimeoutSeconds:      patch.Payload.TimeoutSeconds,
			AllowUnsafeExternal: patch.Payload.AllowUnsafeExternal,
		}
		if err := normalizeCronPayloadAlias(&payload); err != nil {
			return err
		}
		patch.Payload.Kind = payload.Kind
		patch.Payload.Text = &payload.Text
		patch.Payload.Message = &payload.Message
	}
	return nil
}

func normalizeCronPayloadAlias(payload *integrationcron.Payload) error {
	if payload == nil {
		return errors.New("payload is required")
	}
	switch strings.ToLower(strings.TrimSpace(payload.Kind)) {
	case "systemevent":
		payload.Kind = "agentTurn"
		if strings.TrimSpace(payload.Message) == "" {
			payload.Message = strings.TrimSpace(payload.Text)
		}
		payload.Text = ""
	case "agentturn":
		payload.Kind = "agentTurn"
	default:
		return fmt.Errorf("unsupported cron payload kind: %s", strings.TrimSpace(payload.Kind))
	}
	if strings.TrimSpace(payload.Message) == "" {
		return errors.New("payload.message is required")
	}
	return nil
}

func normalizeCronDelivery(delivery *integrationcron.Delivery) *integrationcron.Delivery {
	if delivery == nil {
		return &integrationcron.Delivery{Mode: integrationcron.DeliveryAnnounce}
	}
	mode := delivery.Mode
	if strings.TrimSpace(string(mode)) == "" {
		mode = integrationcron.DeliveryAnnounce
	}
	copyDelivery := *delivery
	copyDelivery.Mode = mode
	return &copyDelivery
}

func resolveCronJobName(input integrationcron.JobCreate) string {
	name := strings.TrimSpace(input.Name)
	if name != "" {
		return name
	}
	if strings.TrimSpace(input.Payload.Message) != "" {
		return truncateSchedulePreview(strings.TrimSpace(input.Payload.Message))
	}
	switch strings.ToLower(strings.TrimSpace(input.Schedule.Kind)) {
	case "cron":
		return "Cron job"
	case "every":
		return "Recurring job"
	default:
		return "Scheduled job"
	}
}

func optionalCronDescription(raw *string) string {
	if raw == nil {
		return ""
	}
	return strings.TrimSpace(*raw)
}

func cronCreateEnabled(raw *bool) bool {
	if raw == nil {
		return true
	}
	return *raw
}

func cronDeleteAfterRun(input integrationcron.JobCreate) bool {
	if input.DeleteAfterRun != nil {
		return *input.DeleteAfterRun
	}
	return strings.EqualFold(strings.TrimSpace(input.Schedule.Kind), "at")
}

func normalizedCronAgentID(raw *string) string {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return normalizeAgentID("default")
	}
	return normalizeAgentID(*raw)
}

func resolveCronPayloadMessage(payload integrationcron.Payload) string {
	message := strings.TrimSpace(payload.Message)
	if message != "" {
		return message
	}
	return strings.TrimSpace(payload.Text)
}

func applyScheduledCronPatch(record scheduledCronJob, patch integrationcron.JobPatch, nowMs int64) (scheduledCronJob, error) {
	if patch.AgentID != nil {
		agentID := normalizeAgentID(strings.TrimSpace(*patch.AgentID))
		if agentID == "" {
			agentID = "default"
		}
		record.Job.AgentID = agentID
	}
	if patch.Name != nil {
		record.Job.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.Description != nil {
		record.Job.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.Enabled != nil {
		record.Job.Enabled = *patch.Enabled
	}
	if patch.DeleteAfterRun != nil {
		record.Job.DeleteAfterRun = *patch.DeleteAfterRun
	}
	if patch.Schedule != nil {
		if result := integrationcron.ValidateSchedule(*patch.Schedule); !result.Ok {
			return record, errors.New(result.Message)
		}
		if result := integrationcron.ValidateScheduleTimestamp(*patch.Schedule, nowMs); !result.Ok {
			return record, errors.New(result.Message)
		}
		record.Job.Schedule = *patch.Schedule
	}
	if patch.Payload != nil {
		if strings.TrimSpace(patch.Payload.Kind) != "" {
			record.Job.Payload.Kind = patch.Payload.Kind
		}
		if patch.Payload.Message != nil {
			record.Job.Payload.Message = strings.TrimSpace(*patch.Payload.Message)
		}
		if patch.Payload.Text != nil {
			record.Job.Payload.Text = strings.TrimSpace(*patch.Payload.Text)
		}
		if patch.Payload.Model != nil {
			record.Job.Payload.Model = strings.TrimSpace(*patch.Payload.Model)
		}
		if patch.Payload.Thinking != nil {
			record.Job.Payload.Thinking = strings.TrimSpace(*patch.Payload.Thinking)
		}
		if patch.Payload.TimeoutSeconds != nil {
			record.Job.Payload.TimeoutSeconds = patch.Payload.TimeoutSeconds
		}
		if patch.Payload.AllowUnsafeExternal != nil {
			record.Job.Payload.AllowUnsafeExternal = patch.Payload.AllowUnsafeExternal
		}
		if err := normalizeCronPayloadAlias(&record.Job.Payload); err != nil {
			return record, err
		}
	}
	if patch.Delivery != nil {
		delivery := normalizeCronDelivery(record.Job.Delivery)
		if patch.Delivery.Mode != nil {
			delivery.Mode = *patch.Delivery.Mode
		}
		if patch.Delivery.Channel != nil {
			delivery.Channel = strings.TrimSpace(*patch.Delivery.Channel)
		}
		if patch.Delivery.To != nil {
			delivery.To = strings.TrimSpace(*patch.Delivery.To)
		}
		if patch.Delivery.BestEffort != nil {
			delivery.BestEffort = patch.Delivery.BestEffort
		}
		record.Job.Delivery = normalizeCronDelivery(delivery)
	}
	record.Job.UpdatedAtMs = nowMs
	record.Revision++
	return record, nil
}

func computeInitialCronDue(job integrationcron.Job, nowMs int64) *int64 {
	switch strings.ToLower(strings.TrimSpace(job.Schedule.Kind)) {
	case "at":
		atMs, ok := parseScheduleAt(job.Schedule.At)
		if !ok {
			return nil
		}
		if job.State.LastRunAtMs != nil && *job.State.LastRunAtMs > 0 {
			return nil
		}
		if atMs <= nowMs {
			val := nowMs
			return &val
		}
		return &atMs
	default:
		return integrationcron.ComputeNextRunAtMs(job.Schedule, nowMs)
	}
}

func computeNextCronAfterRun(job integrationcron.Job, scheduledForMs, nowMs int64) *int64 {
	switch strings.ToLower(strings.TrimSpace(job.Schedule.Kind)) {
	case "at":
		return nil
	default:
		return integrationcron.ComputeNextRunAtMs(job.Schedule, max64(scheduledForMs, nowMs))
	}
}

func computeManagedHeartbeatDue(client *AIClient, state managedHeartbeatState, nowMs int64) int64 {
	if state.IntervalMs <= 0 {
		return 0
	}
	if state.LastRunAtMs > 0 {
		return state.LastRunAtMs + state.IntervalMs
	}
	ref, sessionKey := client.resolveHeartbeatMainSessionRef(state.AgentID)
	if entry, ok := client.getSessionEntry(context.Background(), ref, sessionKey); ok && entry.LastHeartbeatSentAt > 0 {
		return entry.LastHeartbeatSentAt + state.IntervalMs
	}
	return nowMs + state.IntervalMs
}

func upsertManagedHeartbeat(store *managedHeartbeatStore, agentID string, hb *HeartbeatConfig) *managedHeartbeatState {
	if store == nil {
		return nil
	}
	idx := findManagedHeartbeat(store.Agents, agentID)
	interval := resolveHeartbeatIntervalMs(nil, "", hb)
	if idx < 0 {
		state := managedHeartbeatState{
			AgentID:     normalizeAgentID(agentID),
			Enabled:     interval > 0,
			IntervalMs:  interval,
			ActiveHours: cloneHeartbeatActiveHours(hb),
			Revision:    1,
		}
		store.Agents = append(store.Agents, state)
		return &store.Agents[len(store.Agents)-1]
	}
	state := &store.Agents[idx]
	if state.Revision <= 0 {
		state.Revision = 1
	}
	if state.IntervalMs != interval || !equalHeartbeatActiveHours(state.ActiveHours, cloneHeartbeatActiveHours(hb)) {
		state.IntervalMs = interval
		state.ActiveHours = cloneHeartbeatActiveHours(hb)
		state.Revision++
		state.PendingDelayID = ""
		state.PendingDelayKind = ""
		state.PendingRunKey = ""
	}
	state.Enabled = interval > 0
	return state
}

func cloneHeartbeatActiveHours(hb *HeartbeatConfig) *HeartbeatActiveHoursConfig {
	if hb == nil || hb.ActiveHours == nil {
		return nil
	}
	copyCfg := *hb.ActiveHours
	return &copyCfg
}

func equalHeartbeatActiveHours(a, b *HeartbeatActiveHoursConfig) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Start == b.Start && a.End == b.End && a.Timezone == b.Timezone
}

func findScheduledCronJob(jobs []scheduledCronJob, jobID string) int {
	trimmed := strings.TrimSpace(jobID)
	for idx := range jobs {
		if strings.TrimSpace(jobs[idx].Job.ID) == trimmed {
			return idx
		}
	}
	return -1
}

func findManagedHeartbeat(states []managedHeartbeatState, agentID string) int {
	trimmed := normalizeAgentID(agentID)
	for idx := range states {
		if normalizeAgentID(states[idx].AgentID) == trimmed {
			return idx
		}
	}
	return -1
}

func appendRunKey(existing []string, runKey string) []string {
	trimmed := strings.TrimSpace(runKey)
	if trimmed == "" {
		return existing
	}
	if containsRunKey(existing, trimmed) {
		return existing
	}
	existing = append(existing, trimmed)
	if len(existing) > 8 {
		existing = existing[len(existing)-8:]
	}
	return existing
}

func containsRunKey(existing []string, runKey string) bool {
	for _, candidate := range existing {
		if strings.TrimSpace(candidate) == strings.TrimSpace(runKey) {
			return true
		}
	}
	return false
}

func resolveScheduledCronTimeoutSeconds(client *AIClient, override *int) int {
	if override != nil {
		if *override == 0 {
			return 30 * 24 * 60 * 60
		}
		if *override > 0 {
			return *override
		}
	}
	if client != nil && client.connector != nil && client.connector.Config.Agents != nil && client.connector.Config.Agents.Defaults != nil && client.connector.Config.Agents.Defaults.TimeoutSeconds > 0 {
		return client.connector.Config.Agents.Defaults.TimeoutSeconds
	}
	return defaultCronTimeoutSeconds
}

func truncateSchedulePreview(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if len(text) <= 160 {
		return text
	}
	return strings.TrimSpace(text[:159]) + "..."
}

func appendMissingDisabledTool(existing []string, toolName string) []string {
	for _, entry := range existing {
		if strings.EqualFold(strings.TrimSpace(entry), strings.TrimSpace(toolName)) {
			return existing
		}
	}
	return append(existing, toolName)
}

func parseScheduleAt(raw string) (int64, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false
	}
	if ts, err := time.Parse(time.RFC3339, normalizeScheduleAtString(trimmed)); err == nil {
		return ts.UTC().UnixMilli(), true
	}
	return 0, false
}

func normalizeScheduleAtString(raw string) string {
	if strings.HasSuffix(raw, "Z") || strings.Contains(raw, "+") || strings.LastIndex(raw, "-") > 9 {
		return raw
	}
	if strings.Contains(raw, "T") {
		return raw + "Z"
	}
	if len(raw) == len("2006-01-02") {
		return raw + "T00:00:00Z"
	}
	return raw
}

func buildTickRunKey(revision int, kind string, scheduledForMs int64) string {
	return fmt.Sprintf("rev%d:%s:%d", revision, strings.TrimSpace(kind), scheduledForMs)
}

func shortTickKind(kind string) string {
	switch kind {
	case scheduleTickKindCronPlan, scheduleTickKindHeartbeatPlan:
		return "plan"
	default:
		return "run"
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
