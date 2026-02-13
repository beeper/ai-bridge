//lint:file-ignore U1000 Hard-cut compatibility: pending full dead-code deletion.
package connector

import (
	"context"

	"github.com/rs/zerolog"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/id"
)

type HeartbeatWake struct {
	log zerolog.Logger
}

func (h *HeartbeatWake) Request(string, int64) {}

type HeartbeatRunner struct{}

func NewHeartbeatRunner(*AIClient) *HeartbeatRunner { return &HeartbeatRunner{} }
func (h *HeartbeatRunner) Start()                   {}
func (h *HeartbeatRunner) Stop()                    {}
func (h *HeartbeatRunner) run(string) HeartbeatRunResult {
	return HeartbeatRunResult{Status: "skipped", Reason: "disabled"}
}

type HeartbeatRunConfig struct {
	StoreAgentID     string
	StorePath        string
	SessionKey       string
	TargetRoom       id.RoomID
	TargetReason     string
	Reason           string
	Channel          string
	ResponsePrefix   string
	ShowOk           bool
	ShowAlerts       bool
	UseIndicator     bool
	IncludeReasoning bool
	ExecEvent        bool
	SuppressSave     bool
	SuppressSend     bool
	PrevUpdatedAt    int64
	AckMaxChars      int
}

type HeartbeatRunOutcome struct {
	Status  string
	Reason  string
	Text    string
	Sent    bool
	Skipped bool
	Silent  bool
}

type HeartbeatRunResult struct {
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type heartbeatRunContext struct {
	Config   *HeartbeatRunConfig
	ResultCh chan HeartbeatRunOutcome
}

type HeartbeatIndicatorType string

const (
	HeartbeatIndicatorSent    HeartbeatIndicatorType = "sent"
	HeartbeatIndicatorSkipped HeartbeatIndicatorType = "skipped"
)

type HeartbeatEventPayload struct {
	TS            int64                   `json:"ts,omitempty"`
	Status        string                  `json:"status,omitempty"`
	Reason        string                  `json:"reason,omitempty"`
	To            string                  `json:"to,omitempty"`
	Preview       string                  `json:"preview,omitempty"`
	Channel       string                  `json:"channel,omitempty"`
	Silent        bool                    `json:"silent,omitempty"`
	HasMedia      bool                    `json:"has_media,omitempty"`
	DurationMs    int64                   `json:"duration_ms,omitempty"`
	IndicatorType *HeartbeatIndicatorType `json:"indicator_type,omitempty"`
}

func resolveIndicatorType(string) *HeartbeatIndicatorType {
	v := HeartbeatIndicatorSkipped
	return &v
}

func heartbeatRunFromContext(context.Context) *heartbeatRunContext { return nil }

func (oc *AIClient) emitHeartbeatEvent(evt *HeartbeatEventPayload) {
	if oc == nil || oc.UserLogin == nil || evt == nil {
		return
	}
	meta := loginMetadata(oc.UserLogin)
	meta.LastHeartbeatEvent = evt
	_ = oc.UserLogin.Save(context.Background())
}

func getLastHeartbeatEventForLogin(login *bridgev2.UserLogin) *HeartbeatEventPayload {
	if login == nil {
		return nil
	}
	meta := loginMetadata(login)
	return meta.LastHeartbeatEvent
}

type StateStoreEntry struct {
	Key  string
	Data []byte
}

type StateStoreBackend interface {
	Read(context.Context, string) ([]byte, bool, error)
	Write(context.Context, string, []byte) error
	List(context.Context, string) ([]StateStoreEntry, error)
}

type StateEvent struct {
	JobID       string
	Action      string
	Status      string
	Error       string
	Summary     string
	RunAtMs     int64
	DurationMs  int64
	NextRunAtMs int64
}

type StateRunLogEntry struct {
	TS          int64
	JobID       string
	Action      string
	Status      string
	Error       string
	Summary     string
	RunAtMs     int64
	DurationMs  int64
	NextRunAtMs int64
}
