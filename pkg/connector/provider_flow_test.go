package connector

import "testing"

func TestProviderToFlowIDCanonicalizesLegacyBeeper(t *testing.T) {
	if got := providerToFlowID(ProviderBeeper); got != ProviderMagicProxy {
		t.Fatalf("expected beeper provider to map to %q flow, got %q", ProviderMagicProxy, got)
	}
	if got := providerToFlowID(ProviderMagicProxy); got != ProviderMagicProxy {
		t.Fatalf("expected magic proxy provider to map to %q flow, got %q", ProviderMagicProxy, got)
	}
}
