package ai

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetEnvAPIKey_GoogleVertexAuthenticated(t *testing.T) {
	home := t.TempDir()
	adcPath := filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
	if err := os.MkdirAll(filepath.Dir(adcPath), 0o755); err != nil {
		t.Fatalf("failed to create ADC directory: %v", err)
	}
	if err := os.WriteFile(adcPath, []byte(`{"type":"authorized_user"}`), 0o600); err != nil {
		t.Fatalf("failed to write ADC file: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("GOOGLE_CLOUD_LOCATION", "us-central1")

	if got := GetEnvAPIKey("google-vertex"); got != "<authenticated>" {
		t.Fatalf("expected <authenticated> for google-vertex, got %q", got)
	}
}

func TestGetEnvAPIKey_GoogleVertexMissingContext(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GCLOUD_PROJECT", "")
	t.Setenv("GOOGLE_CLOUD_LOCATION", "")
	if got := GetEnvAPIKey("google-vertex"); got != "" {
		t.Fatalf("expected empty key when google-vertex env incomplete, got %q", got)
	}
}

func TestGetEnvAPIKey_AmazonBedrockAuthenticated(t *testing.T) {
	t.Setenv("AWS_PROFILE", "default")
	if got := GetEnvAPIKey("amazon-bedrock"); got != "<authenticated>" {
		t.Fatalf("expected <authenticated> for amazon-bedrock profile auth, got %q", got)
	}

	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIA123")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	if got := GetEnvAPIKey("amazon-bedrock"); got != "<authenticated>" {
		t.Fatalf("expected <authenticated> for amazon-bedrock IAM auth, got %q", got)
	}
}
