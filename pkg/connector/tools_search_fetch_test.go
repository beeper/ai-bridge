package connector

import (
	"testing"

	"github.com/beeper/ai-bridge/pkg/search"
)

func TestApplyLoginTokensToSearchConfig_MagicProxyDisablesDDGFallback(t *testing.T) {
	oc := &OpenAIConnector{}
	meta := &UserLoginMetadata{
		Provider: ProviderMagicProxy,
		APIKey:   "magic-token",
		BaseURL:  "https://bai.bt.hn/team/proxy",
	}
	cfg := &search.Config{
		Fallbacks: []string{search.ProviderBrave, search.ProviderDuckDuckGo},
	}

	got := applyLoginTokensToSearchConfig(cfg, meta, oc)

	if got.Provider != search.ProviderExa {
		t.Fatalf("expected provider %q, got %q", search.ProviderExa, got.Provider)
	}
	if len(got.Fallbacks) != 1 || got.Fallbacks[0] != search.ProviderExa {
		t.Fatalf("expected exa-only fallbacks, got %#v", got.Fallbacks)
	}
	if got.DDG.Enabled == nil || *got.DDG.Enabled {
		t.Fatalf("expected ddg to be disabled, got %+v", got.DDG.Enabled)
	}
	if got.Exa.BaseURL != "https://bai.bt.hn/team/proxy/exa" {
		t.Fatalf("unexpected exa base URL: %q", got.Exa.BaseURL)
	}
	if got.Exa.APIKey != "magic-token" {
		t.Fatalf("unexpected exa API key: %q", got.Exa.APIKey)
	}
}

func TestApplyLoginTokensToSearchConfig_NoMetaLeavesDDGConfigUntouched(t *testing.T) {
	oc := &OpenAIConnector{}
	enabled := true
	cfg := &search.Config{
		Provider:  search.ProviderDuckDuckGo,
		Fallbacks: []string{search.ProviderDuckDuckGo},
		DDG: search.DDGConfig{
			Enabled: &enabled,
		},
	}

	got := applyLoginTokensToSearchConfig(cfg, nil, oc)

	if got.Provider != search.ProviderDuckDuckGo {
		t.Fatalf("unexpected provider: %q", got.Provider)
	}
	if len(got.Fallbacks) != 1 || got.Fallbacks[0] != search.ProviderDuckDuckGo {
		t.Fatalf("unexpected fallbacks: %#v", got.Fallbacks)
	}
	if got.DDG.Enabled == nil || !*got.DDG.Enabled {
		t.Fatalf("expected ddg enabled to stay true")
	}
}
