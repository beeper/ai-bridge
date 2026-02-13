package main

import (
	core "github.com/beeper/ai-bridge/modules/core"
	"maunium.net/go/mautrix/bridgev2/matrix/mxmain"
)

var (
	Tag       = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

var m = mxmain.BridgeMain{
	Name:        "ai-simple",
	Description: "Simple Matrix↔AI bridge.",
	URL:         "https://github.com/beeper/ai-bridge",
	Version:     "0.1.0",
	Connector:   core.NewConnector(core.BridgeSimple),
}

func main() {
	m.InitVersion(Tag, Commit, BuildTime)
	m.Run()
}
