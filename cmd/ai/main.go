package main

import (
	"maunium.net/go/mautrix/bridgev2/matrix/mxmain"

	aibridge "github.com/beeper/agentremote/bridges/ai"
)

// Information to find out exactly which commit the bridge was built from.
// These are filled at build time with the -X linker flag.
var (
	Tag       = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

var m = mxmain.BridgeMain{
	Name:        "ai",
	Description: "A Matrix↔AI bridge for Beeper built on mautrix-go bridgev2.",
	URL:         "https://github.com/beeper/agentremote",
	Version:     "0.1.0",
	Connector:   aibridge.NewAIConnector(),
}

func main() {
	m.InitVersion(Tag, Commit, BuildTime)
	m.Run()
}
