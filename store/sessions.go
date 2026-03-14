package store

type SessionRecord struct {
	SessionKey            string
	SessionID             string
	UpdatedAtMs           int64
	LastHeartbeatText     string
	LastHeartbeatSentAtMs int64
	LastChannel           string
	LastTo                string
	LastAccountID         string
	LastThreadID          string
	QueueMode             string
	QueueDebounceMs       *int
	QueueCap              *int
	QueueDrop             string
}

type SessionStore struct {
	scope *Scope
}
