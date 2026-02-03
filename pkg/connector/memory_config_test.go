package connector

import "testing"

func TestMergeMemorySearchConfig_DefaultsEnabled(t *testing.T) {
	cfg := mergeMemorySearchConfig(nil, nil, nil, "")
	if cfg == nil {
		t.Fatal("expected memory search to be enabled by default")
	}
	if !cfg.Enabled {
		t.Fatal("expected memory search Enabled=true by default")
	}
	if cfg.Provider != "auto" {
		t.Fatalf("expected provider auto, got %q", cfg.Provider)
	}
}
