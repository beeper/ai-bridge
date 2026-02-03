package connector

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestClonePortalMetadataDeepCopiesConfig(t *testing.T) {
	enabled := true
	orig := &PortalMetadata{
		ToolsConfig: ToolsConfig{
			Tools: map[string]*ToolEntry{
				"alpha": {
					Tool:    mcp.Tool{Name: "alpha"},
					Enabled: &enabled,
					Type:    "builtin",
				},
			},
		},
		PDFConfig: &PDFConfig{Engine: "mistral"},
	}

	clone := clonePortalMetadata(orig)
	if clone == nil {
		t.Fatal("expected clone to be non-nil")
	}
	if clone == orig {
		t.Fatal("expected clone to be a different pointer")
	}
	if clone.ToolsConfig.Tools["alpha"] == orig.ToolsConfig.Tools["alpha"] {
		t.Fatal("expected tool entry to be copied")
	}
	if clone.PDFConfig == orig.PDFConfig {
		t.Fatal("expected PDF config to be copied")
	}

	newEnabled := false
	clone.ToolsConfig.Tools["alpha"].Tool.Name = "changed"
	clone.ToolsConfig.Tools["alpha"].Enabled = &newEnabled
	clone.ToolsConfig.Tools["beta"] = &ToolEntry{Tool: mcp.Tool{Name: "beta"}, Type: "builtin"}
	clone.PDFConfig.Engine = "other"

	if orig.ToolsConfig.Tools["alpha"].Tool.Name != "alpha" {
		t.Fatalf("expected original tool name to remain, got %q", orig.ToolsConfig.Tools["alpha"].Tool.Name)
	}
	if orig.ToolsConfig.Tools["alpha"].Enabled == nil || !*orig.ToolsConfig.Tools["alpha"].Enabled {
		t.Fatal("expected original enabled flag to remain true")
	}
	if _, ok := orig.ToolsConfig.Tools["beta"]; ok {
		t.Fatal("expected original tools map to be unchanged")
	}
	if orig.PDFConfig.Engine != "mistral" {
		t.Fatalf("expected original PDF engine to remain, got %q", orig.PDFConfig.Engine)
	}
}
