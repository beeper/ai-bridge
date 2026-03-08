package bridgeadapter

import "strings"

type ApprovalDecisionPayload struct {
	ApprovalID string
	Approved   bool
	Always     bool
	Reason     string
}

func ParseApprovalDecision(raw map[string]any) (ApprovalDecisionPayload, bool) {
	if raw == nil {
		return ApprovalDecisionPayload{}, false
	}

	approvalID, _ := raw["approvalId"].(string)
	approvalID = strings.TrimSpace(approvalID)
	if approvalID == "" {
		return ApprovalDecisionPayload{}, false
	}

	approved, ok := raw["approved"].(bool)
	if !ok {
		return ApprovalDecisionPayload{}, false
	}
	always, _ := raw["always"].(bool)

	reason, _ := raw["reason"].(string)

	return ApprovalDecisionPayload{
		ApprovalID: approvalID,
		Approved:   approved,
		Always:     always,
		Reason:     strings.TrimSpace(reason),
	}, true
}
