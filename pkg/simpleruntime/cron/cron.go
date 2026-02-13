package cron

import (
	"context"
	"errors"
	"time"
)

type StoreEntry struct {
	Key string
	Data []byte
}

type StoreBackend interface {
	Read(ctx context.Context, key string) ([]byte, error)
	Write(ctx context.Context, key string, data []byte) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]StoreEntry, error)
}

type CronService struct{}

type CronRunResult struct {
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type HeartbeatRunResult = CronRunResult

type CronDelivery struct {
	Mode string `json:"mode,omitempty"`
}

const (
	CronDeliveryAnnounce = "announce"
	CronSessionIsolated = "isolated"
)

type CronPayload struct {
	Kind string `json:"kind,omitempty"`
}

type CronJobCreate struct {
	Schedule string `json:"schedule,omitempty"`
	Payload CronPayload `json:"payload,omitempty"`
	Delivery *CronDelivery `json:"delivery,omitempty"`
	SessionTarget string `json:"session_target,omitempty"`
	AgentID *string `json:"agent_id,omitempty"`
}

type CronJobPatch struct {
	Schedule *string `json:"schedule,omitempty"`
}

type CronJob struct {
	ID string `json:"id,omitempty"`
}

type CronEvent struct {
	JobID string `json:"job_id,omitempty"`
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
	TS int64 `json:"ts,omitempty"`
}

type CronRunLogEntry struct {
	JobID string `json:"job_id,omitempty"`
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
	StartedAt int64 `json:"started_at,omitempty"`
	EndedAt int64 `json:"ended_at,omitempty"`
}

func NewCronService(_ context.Context, _ any, _ any, _ any, _ string) (*CronService, error) { return &CronService{}, nil }
func (c *CronService) Close() error { return nil }
func (c *CronService) Start() error { return nil }
func (c *CronService) Stop() error { return nil }
func (c *CronService) Status() (bool, string, int, int64, error) { return false, "", 0, 0, nil }
func (c *CronService) List(_ bool) ([]CronJob, error) { return nil, nil }
func (c *CronService) Add(_ CronJobCreate) (*CronJob, error) { return nil, errors.New("disabled") }
func (c *CronService) Update(_ string, _ CronJobPatch) (*CronJob, error) { return nil, errors.New("disabled") }
func (c *CronService) Remove(_ string) (bool, error) { return false, nil }
func (c *CronService) Run(_ string, _ string) (bool, string, error) { return false, "disabled", nil }
func (c *CronService) Wake(_ string, _ string) (bool, error) { return false, nil }

func ResolveCronRunLogPath(base, jobID string) string { return base + "/" + jobID }
func ResolveCronRunLogDir(base string) string { return base }
func ReadCronRunLogEntries(_ context.Context, _ StoreBackend, _ string, _ int, _ string) ([]CronRunLogEntry, error) { return nil, nil }
func ParseCronRunLogEntries(_ string, _ int, _ string) []CronRunLogEntry { return nil }

type ValidationResult struct { Ok bool; Error string }
func ValidateSchedule(_ string) ValidationResult { return ValidationResult{Ok: true} }
func ValidateScheduleTimestamp(_ string, _ int64) ValidationResult { return ValidationResult{Ok: true} }
func NormalizeCronJobCreateRaw(v any) (CronJobCreate, error) { _ = v; return CronJobCreate{}, nil }
func NormalizeCronJobPatchRaw(v any) (CronJobPatch, error) { _ = v; return CronJobPatch{}, nil }
func CronNowMillis() int64 { return time.Now().UnixMilli() }
