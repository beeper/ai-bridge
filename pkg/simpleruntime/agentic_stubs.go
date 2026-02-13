package connector

import (
	"context"

	"github.com/beeper/ai-bridge/pkg/simpleruntime/cron"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/id"
)

type subagentRun struct{}

type HeartbeatWake struct {
	log zerolog.Logger
}

type HeartbeatRunner struct{}

type HeartbeatRunConfig struct {
	StoreAgentID     string
	StorePath        string
	AckMaxChars      int
	ExecEvent        bool
	ResponsePrefix   string
	IncludeReasoning bool
	TargetRoom       id.RoomID
	TargetReason     string
	SessionKey       string
	PrevUpdatedAt    int64
	ShowOk           bool
	ShowAlerts       bool
	UseIndicator     bool
	Reason           string
	Channel          string
	SuppressSave     bool
	SuppressSend     bool
}

type HeartbeatRunOutcome struct {
	Skipped bool
	Reason  string
	Status  string
	Silent  bool
	Text    string
	Sent    bool
}

type HeartbeatEventPayload struct {
	TS            int64
	Status        string
	To            string
	Reason        string
	Channel       string
	Preview       string
	Silent        bool
	HasMedia      bool
	DurationMs    int64
	IndicatorType *HeartbeatIndicatorType
}

type HeartbeatIndicatorType string

type sessionEntry struct {
	SessionKey string
	UpdatedAt  int64
	QueueMode       string
	QueueDebounceMs *int
	QueueCap        *int
	QueueDrop       string
}

func NewHeartbeatRunner(_ *AIClient) *HeartbeatRunner { return &HeartbeatRunner{} }
func (r *HeartbeatRunner) Run(context.Context, HeartbeatRunConfig) (*HeartbeatRunOutcome, error) {
	_ = r
	return &HeartbeatRunOutcome{}, nil
}
func (r *HeartbeatRunner) Start() {}
func (r *HeartbeatRunner) Stop()  {}
func (r *HeartbeatRunner) RunNow(_ context.Context, _ string) cron.HeartbeatRunResult {
	return cron.HeartbeatRunResult{Status: "skipped", Reason: "disabled"}
}
func (r *HeartbeatRunner) LastHeartbeat() *HeartbeatEventPayload { return nil }

func (w *HeartbeatWake) Wake(_ string) {}
func (w *HeartbeatWake) SetHandler(_ func(string) cron.HeartbeatRunResult) {}
func (w *HeartbeatWake) Close() error { return nil }
