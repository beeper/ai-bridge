package opencodebridge

import (
	"testing"

	"github.com/beeper/agentremote/bridges/opencode/opencode"
)

func TestBuildOpenCodeApprovalPresentation(t *testing.T) {
	p := buildOpenCodeApprovalPresentation(opencode.PermissionRequest{
		Permission: "filesystem.write",
		Patterns:   []string{"src/**", "pkg/**"},
		Metadata: map[string]any{
			"cwd": "/repo",
		},
	})
	if p.Title == "" {
		t.Fatalf("expected title")
	}
	if !p.AllowAlways {
		t.Fatalf("expected OpenCode approvals to allow always")
	}
	if len(p.Details) == 0 {
		t.Fatalf("expected details")
	}
}
