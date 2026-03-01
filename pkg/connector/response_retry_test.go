package connector

import (
	"testing"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

func newPruningTestClient(pruning *PruningConfig, provider string) *AIClient {
	login := &database.UserLogin{
		ID:       networkid.UserLoginID("login"),
		Metadata: &UserLoginMetadata{Provider: provider},
	}
	return &AIClient{
		UserLogin: &bridgev2.UserLogin{
			UserLogin: login,
			Log:       zerolog.Nop(),
		},
		connector: &OpenAIConnector{
			Config: Config{
				Pruning: pruning,
			},
		},
		log: zerolog.Nop(),
	}
}

func TestPruningReserveTokens_UsesConfigValue(t *testing.T) {
	client := newPruningTestClient(&PruningConfig{ReserveTokens: 777}, ProviderOpenAI)
	if got := client.pruningReserveTokens(); got != 777 {
		t.Fatalf("expected reserve tokens 777, got %d", got)
	}
}

func TestPruningReserveTokens_DefaultsWhenUnset(t *testing.T) {
	client := newPruningTestClient(&PruningConfig{}, ProviderOpenAI)
	if got := client.pruningReserveTokens(); got != 2000 {
		t.Fatalf("expected default reserve tokens 2000, got %d", got)
	}
}

func TestPruningOverflowFlushConfig_ReadsFromPruning(t *testing.T) {
	enabled := true
	client := newPruningTestClient(&PruningConfig{
		OverflowFlush: &OverflowFlushConfig{
			Enabled:             &enabled,
			SoftThresholdTokens: 1234,
			Prompt:              "flush",
			SystemPrompt:        "sys",
		},
	}, ProviderOpenAI)
	cfg := client.pruningOverflowFlushConfig()
	if cfg == nil {
		t.Fatal("expected overflow flush config")
	}
	if cfg.Enabled == nil || !*cfg.Enabled {
		t.Fatal("expected overflow flush enabled")
	}
	if cfg.SoftThresholdTokens != 1234 {
		t.Fatalf("expected threshold 1234, got %d", cfg.SoftThresholdTokens)
	}
	if cfg.Prompt != "flush" || cfg.SystemPrompt != "sys" {
		t.Fatalf("unexpected prompts: %#v", cfg)
	}
}
