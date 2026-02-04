package cron

import "encoding/json"

// CronSchedule defines when a cron job should run.
type CronSchedule struct {
	Kind     string `json:"kind"`
	AtMs     int64  `json:"atMs,omitempty"`
	EveryMs  int64  `json:"everyMs,omitempty"`
	AnchorMs *int64 `json:"anchorMs,omitempty"`
	Expr     string `json:"expr,omitempty"`
	TZ       string `json:"tz,omitempty"`
}

// CronSessionTarget defines where a cron job runs.
type CronSessionTarget string

const (
	CronSessionMain     CronSessionTarget = "main"
	CronSessionIsolated CronSessionTarget = "isolated"
)

// CronWakeMode defines how the heartbeat is triggered after a main job.
type CronWakeMode string

const (
	CronWakeNextHeartbeat CronWakeMode = "next-heartbeat"
	CronWakeNow           CronWakeMode = "now"
)

// CronMessageChannel defines delivery channel hints for isolated jobs.
type CronMessageChannel string

// CronPayload defines the job action.
type CronPayload struct {
	Kind                     string `json:"kind"`
	Text                     string `json:"text,omitempty"`
	Message                  string `json:"message,omitempty"`
	Model                    string `json:"model,omitempty"`
	Thinking                 string `json:"thinking,omitempty"`
	TimeoutSeconds           *int   `json:"timeoutSeconds,omitempty"`
	AllowUnsafeExternal      *bool  `json:"allowUnsafeExternalContent,omitempty"`
	Deliver                  *bool  `json:"deliver,omitempty"`
	Channel                  string `json:"channel,omitempty"`
	To                       string `json:"to,omitempty"`
	BestEffortDeliver        *bool  `json:"bestEffortDeliver,omitempty"`
	LegacyProviderDeprecated string `json:"provider,omitempty"`
}

// CronPayloadPatch defines partial payload updates.
type CronPayloadPatch struct {
	Kind                string  `json:"kind"`
	Text                *string `json:"text,omitempty"`
	Message             *string `json:"message,omitempty"`
	Model               *string `json:"model,omitempty"`
	Thinking            *string `json:"thinking,omitempty"`
	TimeoutSeconds      *int    `json:"timeoutSeconds,omitempty"`
	AllowUnsafeExternal *bool   `json:"allowUnsafeExternalContent,omitempty"`
	Deliver             *bool   `json:"deliver,omitempty"`
	Channel             *string `json:"channel,omitempty"`
	To                  *string `json:"to,omitempty"`
	BestEffortDeliver   *bool   `json:"bestEffortDeliver,omitempty"`
}

// CronIsolation controls how isolated runs report back to main.
type CronIsolation struct {
	PostToMainPrefix   string `json:"postToMainPrefix,omitempty"`
	PostToMainMode     string `json:"postToMainMode,omitempty"` // summary|full
	PostToMainMaxChars *int   `json:"postToMainMaxChars,omitempty"`
}

// CronJobState tracks runtime state.
type CronJobState struct {
	NextRunAtMs    *int64 `json:"nextRunAtMs,omitempty"`
	RunningAtMs    *int64 `json:"runningAtMs,omitempty"`
	LastRunAtMs    *int64 `json:"lastRunAtMs,omitempty"`
	LastStatus     string `json:"lastStatus,omitempty"`
	LastError      string `json:"lastError,omitempty"`
	LastDurationMs *int64 `json:"lastDurationMs,omitempty"`
}

// CronJob defines a stored job.
type CronJob struct {
	ID             string            `json:"id"`
	AgentID        string            `json:"agentId,omitempty"`
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	Enabled        bool              `json:"enabled"`
	DeleteAfterRun bool              `json:"deleteAfterRun,omitempty"`
	CreatedAtMs    int64             `json:"createdAtMs"`
	UpdatedAtMs    int64             `json:"updatedAtMs"`
	Schedule       CronSchedule      `json:"schedule"`
	SessionTarget  CronSessionTarget `json:"sessionTarget"`
	WakeMode       CronWakeMode      `json:"wakeMode"`
	Payload        CronPayload       `json:"payload"`
	Isolation      *CronIsolation    `json:"isolation,omitempty"`
	State          CronJobState      `json:"state"`
}

// CronStoreFile defines the JSON store format.
type CronStoreFile struct {
	Version int       `json:"version"`
	Jobs    []CronJob `json:"jobs"`
}

// CronJobCreate is input for creating jobs.
type CronJobCreate struct {
	AgentID        *string           `json:"agentId,omitempty"`
	Name           string            `json:"name,omitempty"`
	Description    *string           `json:"description,omitempty"`
	Enabled        *bool             `json:"enabled,omitempty"`
	DeleteAfterRun *bool             `json:"deleteAfterRun,omitempty"`
	Schedule       CronSchedule      `json:"schedule"`
	SessionTarget  CronSessionTarget `json:"sessionTarget"`
	WakeMode       CronWakeMode      `json:"wakeMode,omitempty"`
	Payload        CronPayload       `json:"payload"`
	Isolation      *CronIsolation    `json:"isolation,omitempty"`
	State          *CronJobState     `json:"state,omitempty"`
}

// CronJobPatch defines partial updates.
type CronJobPatch struct {
	AgentID        *string            `json:"agentId,omitempty"`
	Name           *string            `json:"name,omitempty"`
	Description    *string            `json:"description,omitempty"`
	Enabled        *bool              `json:"enabled,omitempty"`
	DeleteAfterRun *bool              `json:"deleteAfterRun,omitempty"`
	Schedule       *CronSchedule      `json:"schedule,omitempty"`
	SessionTarget  *CronSessionTarget `json:"sessionTarget,omitempty"`
	WakeMode       *CronWakeMode      `json:"wakeMode,omitempty"`
	Payload        *CronPayloadPatch  `json:"payload,omitempty"`
	Isolation      *CronIsolation     `json:"isolation,omitempty"`
	State          *CronJobState      `json:"state,omitempty"`
}

// MarshalJSON ensures payload patches include kind when set.
func (p CronPayloadPatch) MarshalJSON() ([]byte, error) {
	type alias CronPayloadPatch
	return json.Marshal(alias(p))
}
