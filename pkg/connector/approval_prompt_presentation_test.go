package connector

import "testing"

func TestBuildBuiltinApprovalPresentation(t *testing.T) {
	presentation := buildBuiltinApprovalPresentation("commandExecution", "run", map[string]any{
		"command": "ls -la",
		"cwd":     "/tmp",
	})
	if !presentation.AllowAlways {
		t.Fatalf("expected builtin approvals to allow always")
	}
	if presentation.Title == "" {
		t.Fatalf("expected title")
	}
	if len(presentation.Details) == 0 {
		t.Fatalf("expected details")
	}
}

func TestBuildMCPApprovalPresentation(t *testing.T) {
	presentation := buildMCPApprovalPresentation("filesystem", "read_file", map[string]any{
		"path": "/tmp/demo.txt",
	})
	if !presentation.AllowAlways {
		t.Fatalf("expected MCP approvals to allow always")
	}
	if presentation.Title == "" {
		t.Fatalf("expected title")
	}
	if len(presentation.Details) == 0 {
		t.Fatalf("expected details")
	}
}
