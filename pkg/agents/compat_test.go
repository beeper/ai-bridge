package agents

import (
	"encoding/json"
	"testing"
)

func TestGetPresetByIDLegacyPlaygroundAlias(t *testing.T) {
	preset := GetPresetByID("playground")
	if preset == nil {
		t.Fatalf("expected legacy preset id to resolve")
	}
	if preset.ID != SimpleAgent.ID {
		t.Fatalf("expected preset id %q, got %q", SimpleAgent.ID, preset.ID)
	}
}

func TestResponseModeUnmarshalLegacyRawAlias(t *testing.T) {
	var decoded struct {
		ResponseMode ResponseMode `json:"response_mode"`
	}

	if err := json.Unmarshal([]byte(`{"response_mode":"raw"}`), &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.ResponseMode != ResponseModeSimple {
		t.Fatalf("expected %q, got %q", ResponseModeSimple, decoded.ResponseMode)
	}
}
