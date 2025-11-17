package connector

import (
	"testing"

	"go.mau.fi/util/configupgrade"
	"gopkg.in/yaml.v3"
)

func TestUpgradeConfigKeepsAPIKey(t *testing.T) {
	const baseYAML = `
network:
  openai:
    api_key: ""
`
	const cfgYAML = `
network:
  openai:
    api_key: "sk-test"
`

	var baseNode, cfgNode yaml.Node
	if err := yaml.Unmarshal([]byte(baseYAML), &baseNode); err != nil {
		t.Fatalf("failed to parse base yaml: %v", err)
	}
	if err := yaml.Unmarshal([]byte(cfgYAML), &cfgNode); err != nil {
		t.Fatalf("failed to parse config yaml: %v", err)
	}

	helper := configupgrade.NewHelper(&baseNode, &cfgNode)
	proxy := &configupgrade.ProxyHelper{
		Target: helper,
		Prefix: []string{"network"},
	}
	upgradeConfig(proxy)

	networkNode, ok := helper.Base.Map["network"]
	if !ok {
		t.Fatalf("network node missing after upgrade")
	}
	openaiNode, ok := networkNode.Map["openai"]
	if !ok {
		t.Fatalf("openai node missing after upgrade")
	}
	apiKeyNode, ok := openaiNode.Map["api_key"]
	if !ok {
		t.Fatalf("api_key node missing after upgrade")
	}
	if apiKeyNode.Value != "sk-test" {
		t.Fatalf("expected api_key to stay set, got %q", apiKeyNode.Value)
	}
}
