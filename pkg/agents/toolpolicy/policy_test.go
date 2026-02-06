package toolpolicy

import "testing"

func TestNormalizeToolNameRemovesAnalyzeImageAlias(t *testing.T) {
	if got := NormalizeToolName("analyze_image"); got != "analyze_image" {
		t.Fatalf("expected analyze_image to stay unchanged, got %q", got)
	}
}

func TestNormalizeToolNameKeepsApplyPatchAlias(t *testing.T) {
	if got := NormalizeToolName("apply-patch"); got != "apply_patch" {
		t.Fatalf("expected apply-patch alias to normalize to apply_patch, got %q", got)
	}
}
