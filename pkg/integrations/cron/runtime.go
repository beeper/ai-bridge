package cron

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/rs/zerolog"

	croncore "github.com/beeper/ai-bridge/pkg/cron"
)

type StoreEntry struct {
	Key  string
	Data []byte
}

type StoreBackend interface {
	Read(ctx context.Context, key string) ([]byte, bool, error)
	Write(ctx context.Context, key string, data []byte) error
	List(ctx context.Context, prefix string) ([]StoreEntry, error)
}

type storeBackendAdapter struct {
	backend StoreBackend
}

func NewStoreBackendAdapter(backend StoreBackend) croncore.StoreBackend {
	return &storeBackendAdapter{backend: backend}
}

func (a *storeBackendAdapter) Read(ctx context.Context, key string) ([]byte, bool, error) {
	if a == nil || a.backend == nil {
		return nil, false, errors.New("bridge state store not available")
	}
	return a.backend.Read(ctx, key)
}

func (a *storeBackendAdapter) Write(ctx context.Context, key string, data []byte) error {
	if a == nil || a.backend == nil {
		return errors.New("bridge state store not available")
	}
	return a.backend.Write(ctx, key, data)
}

func (a *storeBackendAdapter) List(ctx context.Context, prefix string) ([]croncore.StoreEntry, error) {
	if a == nil || a.backend == nil {
		return nil, errors.New("bridge state store not available")
	}
	entries, err := a.backend.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]croncore.StoreEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, croncore.StoreEntry{Key: entry.Key, Data: entry.Data})
	}
	return out, nil
}

func ResolveCronStorePath(raw string) string {
	return croncore.ResolveCronStorePath(raw)
}

func ResolveCronMaxConcurrentRuns(maxConcurrentRuns int) int {
	return max(maxConcurrentRuns, 1)
}

func ResolveCronJobTimeoutMs(job croncore.CronJob, defaultTimeoutSeconds int) int64 {
	if job.SessionTarget != croncore.CronSessionIsolated {
		return int64(10 * time.Minute / time.Millisecond)
	}
	if defaultTimeoutSeconds <= 0 {
		defaultTimeoutSeconds = 600
	}
	timeoutSeconds := defaultTimeoutSeconds
	if job.Payload.TimeoutSeconds != nil {
		switch override := *job.Payload.TimeoutSeconds; {
		case override == 0:
			return int64(30 * 24 * time.Hour / time.Millisecond)
		case override > 0:
			timeoutSeconds = override
		}
	}
	return int64(max(timeoutSeconds, 1)) * 1000
}

type ServiceBuildDeps struct {
	NowMs             func() int64
	Log               zerolog.Logger
	StorePath         string
	Store             StoreBackend
	MaxConcurrentRuns int
	CronEnabled       bool

	ResolveJobTimeoutMs func(job croncore.CronJob) int64
	EnqueueSystemEvent  func(ctx context.Context, text string, agentID string) error
	RequestHeartbeatNow func(ctx context.Context, reason string)
	RunHeartbeatOnce    func(ctx context.Context, reason string) croncore.HeartbeatRunResult
	RunIsolatedAgentJob func(ctx context.Context, job croncore.CronJob, message string) (status string, summary string, outputText string, err error)
	OnEvent             func(evt croncore.CronEvent)
}

func BuildCronService(deps ServiceBuildDeps) *croncore.CronService {
	coreDeps := croncore.CronServiceDeps{
		NowMs:               deps.NowMs,
		Log:                 NewZeroLogger(deps.Log),
		StorePath:           deps.StorePath,
		Store:               NewStoreBackendAdapter(deps.Store),
		MaxConcurrentRuns:   deps.MaxConcurrentRuns,
		CronEnabled:         deps.CronEnabled,
		ResolveJobTimeoutMs: deps.ResolveJobTimeoutMs,
		EnqueueSystemEvent:  deps.EnqueueSystemEvent,
		RequestHeartbeatNow: deps.RequestHeartbeatNow,
		RunHeartbeatOnce:    deps.RunHeartbeatOnce,
		RunIsolatedAgentJob: deps.RunIsolatedAgentJob,
		OnEvent:             deps.OnEvent,
	}
	return croncore.NewCronService(coreDeps)
}

type EventLogDeps struct {
	StorePath    string
	Log          croncore.Logger
	AppendRunLog func(ctx context.Context, path string, entry croncore.CronRunLogEntry) error
	NowMs        func() int64
}

func HandleCronEvent(evt croncore.CronEvent, deps EventLogDeps) {
	if strings.TrimSpace(evt.JobID) == "" {
		return
	}
	if deps.Log != nil {
		deps.Log.Debug("Cron event received", map[string]any{
			"job_id":      evt.JobID,
			"action":      evt.Action,
			"status":      evt.Status,
			"duration_ms": evt.DurationMs,
		})
	}
	if evt.Action != "finished" || deps.AppendRunLog == nil {
		return
	}
	path := croncore.ResolveCronRunLogPath(deps.StorePath, evt.JobID)
	entry := CronRunLogEntryFromEvent(evt, deps.NowMs)
	if err := deps.AppendRunLog(context.Background(), path, entry); err != nil && deps.Log != nil {
		deps.Log.Warn("cron event: failed to append run log", map[string]any{"job_id": evt.JobID, "error": err.Error()})
	}
}

func CronRunLogEntryFromEvent(evt croncore.CronEvent, nowMs func() int64) croncore.CronRunLogEntry {
	var ts int64
	if nowMs != nil {
		ts = nowMs()
	} else {
		ts = time.Now().UnixMilli()
	}
	return croncore.CronRunLogEntry{
		TS:          ts,
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
