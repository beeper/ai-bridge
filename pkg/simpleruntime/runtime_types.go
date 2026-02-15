package connector

// HeartbeatEventPayload is a serialization-only struct stored in
// UserLoginMetadata.LastHeartbeatEvent. The simple bridge never writes
// it; downstream agentic bridges do via metadata.
type HeartbeatEventPayload struct {
	TS            int64   `json:"ts,omitempty"`
	Status        string  `json:"status,omitempty"`
	Reason        string  `json:"reason,omitempty"`
	To            string  `json:"to,omitempty"`
	Preview       string  `json:"preview,omitempty"`
	Channel       string  `json:"channel,omitempty"`
	Silent        bool    `json:"silent,omitempty"`
	HasMedia      bool    `json:"has_media,omitempty"`
	DurationMs    int64   `json:"duration_ms,omitempty"`
	IndicatorType *string `json:"indicator_type,omitempty"`
}
