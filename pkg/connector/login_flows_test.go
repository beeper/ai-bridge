package connector

import "testing"

func TestSimpleLoginFlows(t *testing.T) {
	oc := &OpenAIConnector{}
	flows := oc.GetLoginFlows()
	if len(flows) != 2 {
		t.Fatalf("expected exactly 2 login flows, got %d", len(flows))
	}
	if flows[0].ID != ProviderMagicProxy {
		t.Fatalf("expected first flow %q, got %q", ProviderMagicProxy, flows[0].ID)
	}
	if flows[1].ID != FlowCustom {
		t.Fatalf("expected second flow %q, got %q", FlowCustom, flows[1].ID)
	}
}
