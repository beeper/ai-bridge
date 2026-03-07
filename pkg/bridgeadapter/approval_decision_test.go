package bridgeadapter

import "testing"

func TestParseApprovalDecision_AcceptsCanonicalShape(t *testing.T) {
	payload, ok := ParseApprovalDecision(map[string]any{
		"approvalId": "approval-1",
		"approved":   true,
		"always":     true,
		"reason":     "approved",
	})
	if !ok {
		t.Fatalf("expected payload to parse")
	}
	if payload.ApprovalID != "approval-1" {
		t.Fatalf("unexpected approval id: %q", payload.ApprovalID)
	}
	if !payload.Approved || !payload.Always || payload.Reason != "approved" {
		t.Fatalf("unexpected parsed payload: %#v", payload)
	}
}

func TestParseApprovalDecision_RejectsLegacyShape(t *testing.T) {
	for name, raw := range map[string]map[string]any{
		"snake_case": {
			"approval_id": "approval-1",
			"approved":    true,
		},
		"string_decision": {
			"approvalId": "approval-1",
			"decision":   "allow",
		},
	} {
		if _, ok := ParseApprovalDecision(raw); ok {
			t.Fatalf("%s payload should be rejected", name)
		}
	}
}
