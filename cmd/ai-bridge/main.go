package main

import (
	"maunium.net/go/mautrix/bridgev2/matrix/mxmain"

	"github.com/beeper/ai-bridge/pkg/connector"
)

func main() {
	br := &mxmain.BridgeMain{
		Name:        "beeper-ai-bridge",
		Description: "A Matrix↔AI bridge for Beeper built on mautrix-go bridgev2.",
		URL:         "https://github.com/beeper/ai-bridge",
		Version:     "0.1.0",
		Connector:   &connector.OpenAIConnector{},
	}
	br.Run()
}
