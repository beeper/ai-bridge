package core

import "github.com/beeper/ai-bridge/pkg/connector"

func NewConnector(kind BridgeKind) *connector.OpenAIConnector {
	oc := &connector.OpenAIConnector{}
	oc.SetPolicy(PolicyFor(kind))
	return oc
}
