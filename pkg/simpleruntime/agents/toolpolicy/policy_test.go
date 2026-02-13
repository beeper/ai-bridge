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

func TestNormalizeToolNameBashAlias(t *testing.T) {
	if got := NormalizeToolName("bash"); got != "exec" {
		t.Fatalf("expected bash alias to normalize to exec, got %q", got)
	}
}

func TestExpandToolGroups_Runtime(t *testing.T) {
	got := ExpandToolGroups([]string{"group:runtime"})
	if len(got) != 2 || got[0] != "exec" || got[1] != "process" {
		t.Fatalf("unexpected group:runtime expansion: %#v", got)
	}
}

func TestExpandToolGroups_OpenClawIsStrict(t *testing.T) {
	got := ExpandToolGroups([]string{"group:openclaw"})
	mustContain := []string{
		"message",
		"cron",
		"sessions_list",
		"sessions_send",
		"web_search",
		"web_fetch",
		"image",
		"browser",
		"canvas",
		"nodes",
		"gateway",
	}
	for _, name := range mustContain {
		found := false
		for _, entry := range got {
			if entry == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected group:openclaw to include %q, got %#v", name, got)
		}
	}

	// ai-bridge extras must NOT be part of strict group:openclaw.
	mustNotContain := []string{"beeper_docs", "gravatar_fetch", "gravatar_set", "tts", "image_generate", "calculator"}
	for _, name := range mustNotContain {
		for _, entry := range got {
			if entry == name {
				t.Fatalf("expected group:openclaw to be strict and exclude %q, got %#v", name, got)
			}
		}
	}
}

func TestExpandToolGroups_AIBridgeExtras(t *testing.T) {
	got := ExpandToolGroups([]string{"group:ai-bridge"})
	mustContain := []string{"beeper_docs", "gravatar_fetch", "gravatar_set", "tts", "image_generate", "calculator"}
	for _, name := range mustContain {
		found := false
		for _, entry := range got {
			if entry == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected group:ai-bridge to include %q, got %#v", name, got)
		}
	}
}
