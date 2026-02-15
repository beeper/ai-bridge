package main

import (
	"github.com/beeper/ai-bridge/pkg/matrixai/runtime"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/matrix/mxmain"
)

var (
	Tag       = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

var m = mxmain.BridgeMain{
	Name:        "ai",
	Description: "Matrix↔AI bridge.",
	URL:         "https://github.com/beeper/ai-bridge",
	Version:     "0.1.0",
	Connector: func() bridgev2.NetworkConnector {
		oc := &runtime.OpenAIConnector{}
		oc.SetPolicy(runtime.BridgePolicy{
			Name:                "Beeper AI (Simple)",
			NetworkID:           "ai-simple",
			BeeperBridgeType:    "ai-simple",
			ProvisioningEnabled: true,
			ResolveIdentifier: bridgev2.ResolveIdentifierCapabilities{
				CreateDM:       true,
				LookupUsername: true,
				ContactList:    true,
				Search:         true,
			},
		})
		return oc
	}(),
}

func main() {
	m.InitVersion(Tag, Commit, BuildTime)
	m.Run()
}
