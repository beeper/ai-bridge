package connector

import "testing"

func TestBuiltinToolApprovalRequirement_Write_MemoryPathDoesNotRequireApproval(t *testing.T) {
	oc := &AIClient{connector: &OpenAIConnector{}}

	required, action := oc.builtinToolApprovalRequirement("write", map[string]any{
		"path": "MEMORY.md",
	})
	if required {
		t.Fatalf("expected required=false for memory write")
	}
	if action != "memory" {
		t.Fatalf("expected action=memory, got %q", action)
	}
}

func TestBuiltinToolApprovalRequirement_Write_WorkspacePathRequiresApproval(t *testing.T) {
	oc := &AIClient{connector: &OpenAIConnector{}}

	required, action := oc.builtinToolApprovalRequirement("write", map[string]any{
		"path": "notes/a.txt",
	})
	if !required {
		t.Fatalf("expected required=true for workspace write")
	}
	if action != "workspace" {
		t.Fatalf("expected action=workspace, got %q", action)
	}
}

func TestBuiltinToolApprovalRequirement_Edit_MemoryPathDoesNotRequireApproval(t *testing.T) {
	oc := &AIClient{connector: &OpenAIConnector{}}

	required, action := oc.builtinToolApprovalRequirement("edit", map[string]any{
		"path": "memory/2026-02-07.md",
	})
	if required {
		t.Fatalf("expected required=false for memory edit")
	}
	if action != "memory" {
		t.Fatalf("expected action=memory, got %q", action)
	}
}

func TestBuiltinToolApprovalRequirement_Edit_WorkspacePathRequiresApproval(t *testing.T) {
	oc := &AIClient{connector: &OpenAIConnector{}}

	required, action := oc.builtinToolApprovalRequirement("edit", map[string]any{
		"path": "notes/a.txt",
	})
	if !required {
		t.Fatalf("expected required=true for workspace edit")
	}
	if action != "workspace" {
		t.Fatalf("expected action=workspace, got %q", action)
	}
}
