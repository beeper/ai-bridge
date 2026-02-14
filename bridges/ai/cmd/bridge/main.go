package main

import (
	"github.com/beeper/ai-bridge/modules/simple"
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
	Connector:   simple.NewConnector(),
}

func main() {
	m.InitVersion(Tag, Commit, BuildTime)
	m.Run()
}
