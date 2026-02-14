package simpleconnector

import (
	base "github.com/beeper/ai-bridge/pkg/simpleruntime"
)

type OpenAIConnector = base.OpenAIConnector

// New builds the simple bridge connector with the given policy.
func New(policy base.BridgePolicy) *base.OpenAIConnector {
	oc := &base.OpenAIConnector{}
	oc.SetPolicy(policy)
	return oc
}
