package agents

import (
	"strings"
	"testing"
)

func TestBuildBootstrapContextFiles_MissingFile(t *testing.T) {
	files := []WorkspaceBootstrapFile{{
		Name:    "AGENTS.md",
		Path:    "AGENTS.md",
		Missing: true,
	}}
	ctxFiles := BuildBootstrapContextFiles(files, 200, nil)
	if len(ctxFiles) != 1 {
		t.Fatalf("expected 1 context file, got %d", len(ctxFiles))
	}
	if !strings.Contains(ctxFiles[0].Content, "[MISSING] Expected at: AGENTS.md") {
		t.Fatalf("unexpected missing content: %q", ctxFiles[0].Content)
	}
}

func TestBuildBootstrapContextFiles_Truncation(t *testing.T) {
	content := strings.Repeat("a", 200)
	files := []WorkspaceBootstrapFile{{
		Name:    "AGENTS.md",
		Path:    "AGENTS.md",
		Content: content,
		Missing: false,
	}}
	ctxFiles := BuildBootstrapContextFiles(files, 50, nil)
	if len(ctxFiles) != 1 {
		t.Fatalf("expected 1 context file, got %d", len(ctxFiles))
	}
	if !strings.Contains(ctxFiles[0].Content, "[...truncated, read AGENTS.md for full content...") {
		t.Fatalf("expected truncation marker, got %q", ctxFiles[0].Content)
	}
}
