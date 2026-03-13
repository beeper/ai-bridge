package sdk

import (
	"maunium.net/go/mautrix/bridgev2/matrix/mxmain"

	"github.com/beeper/agentremote"
)

// Bridge is the SDK bridge handle.
type Bridge struct {
	config    *Config
	connector *agentremote.ConnectorBase
	main      *mxmain.BridgeMain
}

// New creates a new SDK bridge instance.
func New(cfg Config) *Bridge {
	conn := NewConnectorBase(&cfg)
	desc := cfg.Description
	if desc == "" {
		desc = "A Matrix↔" + cfg.Name + " bridge for Beeper built on agentremote SDK."
	}
	return &Bridge{
		config:    &cfg,
		connector: conn,
		main: &mxmain.BridgeMain{
			Name:        cfg.Name,
			Description: desc,
			URL:         "https://github.com/beeper/agentremote",
			Version:     "0.1.0",
			Connector:   conn,
		},
	}
}

// Run starts the bridge and blocks until it exits.
func (b *Bridge) Run() {
	b.main.InitVersion("0.1.0", "unknown", "unknown")
	b.main.Run()
}

// Stop stops the bridge.
func (b *Bridge) Stop() {
	// Bridge stop is handled by mxmain's signal handling
}

// Connector returns the underlying ConnectorBase.
func (b *Bridge) Connector() *agentremote.ConnectorBase { return b.connector }

// BridgeMain returns the underlying mxmain.BridgeMain.
func (b *Bridge) BridgeMain() *mxmain.BridgeMain { return b.main }
