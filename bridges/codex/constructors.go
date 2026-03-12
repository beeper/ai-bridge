package codex

import "github.com/beeper/agentremote"

func NewConnector() *CodexConnector {
	return &CodexConnector{
		BaseConnectorMethods: agentremote.BaseConnectorMethods{ProtocolID: "ai-codex"},
	}
}
