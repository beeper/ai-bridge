package agentremote

import "testing"

func TestNewApprovalManagerWrapsFlow(t *testing.T) {
	manager := NewApprovalManager[map[string]any](ApprovalFlowConfig[map[string]any]{})
	if manager == nil {
		t.Fatal("expected approval manager")
	}
	if manager.ApprovalFlow == nil {
		t.Fatal("expected approval flow to be initialized")
	}
}

func TestNewRuntimeInitializesServices(t *testing.T) {
	runtime := NewRuntime(RuntimeConfig{AgentID: " agent "})
	if runtime == nil {
		t.Fatal("expected runtime")
	}
	if runtime.AgentID != "agent" {
		t.Fatalf("expected trimmed agent id, got %q", runtime.AgentID)
	}
	if runtime.Turns == nil {
		t.Fatal("expected turn manager")
	}
	if runtime.Approvals == nil {
		t.Fatal("expected approval manager")
	}
}
