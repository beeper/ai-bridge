package store

type ApprovalRecord struct {
	ApprovalID  string
	Kind        string
	RoomID      string
	TurnID      string
	ToolCallID  string
	ToolName    string
	RequestJSON string
	Status      string
	Reason      string
	ExpiresAtMs int64
	CreatedAtMs int64
	UpdatedAtMs int64
}

type ApprovalStore struct{}
