package toolspec

import "testing"

func TestTTSSchemaIncludesVoiceAndModel(t *testing.T) {
	schema := TTSSchema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tts schema properties missing")
	}
	if _, ok := props["voice"]; !ok {
		t.Fatalf("expected tts schema to include voice property")
	}
	if _, ok := props["model"]; !ok {
		t.Fatalf("expected tts schema to include model property")
	}
}
