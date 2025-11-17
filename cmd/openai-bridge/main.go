package main

import (
	"maunium.net/go/mautrix/bridgev2/matrix/mxmain"

	"github.com/beepai/matrix-openai-bridge/pkg/connector"
)

func main() {
	br := &mxmain.BridgeMain{
		Name:        "beepai-openai-bridge",
		Description: "A Matrix↔OpenAI ChatGPT bridge built on mautrix-go bridgev2.",
		URL:         "https://github.com/beepai/matrix-openai-bridge",
		Version:     "0.1.0",
		Connector:   &connector.OpenAIConnector{},
	}
	br.Run()
}
