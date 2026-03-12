package openclaw

import (
	"context"
	"strings"
	"sync"

	"go.mau.fi/util/configupgrade"
	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"github.com/beeper/agentremote"
)

var (
	_ bridgev2.NetworkConnector               = (*OpenClawConnector)(nil)
	_ bridgev2.PortalBridgeInfoFillingNetwork = (*OpenClawConnector)(nil)
)

type OpenClawConnector struct {
	*agentremote.ConnectorBase
	br     *bridgev2.Bridge
	Config Config

	clientsMu sync.Mutex
	clients   map[networkid.UserLoginID]bridgev2.NetworkAPI
}

func NewConnector() *OpenClawConnector {
	oc := &OpenClawConnector{}
	oc.ConnectorBase = agentremote.NewConnector(agentremote.ConnectorSpec{
		ProtocolID: "ai-openclaw",
		Init: func(bridge *bridgev2.Bridge) {
			oc.br = bridge
			agentremote.EnsureClientMap(&oc.clientsMu, &oc.clients)
		},
		Start: func(context.Context) error {
			if oc.Config.Bridge.CommandPrefix == "" {
				oc.Config.Bridge.CommandPrefix = "!openclaw"
			}
			if oc.Config.OpenClaw.Enabled == nil {
				oc.Config.OpenClaw.Enabled = ptr.Ptr(true)
			}
			return nil
		},
		Stop: func(context.Context) {
			agentremote.StopClients(&oc.clientsMu, &oc.clients)
		},
		Name: func() bridgev2.BridgeName {
			return bridgev2.BridgeName{
				DisplayName:          "OpenClaw Bridge",
				NetworkURL:           "https://github.com/openclaw/openclaw",
				NetworkID:            "openclaw",
				BeeperBridgeType:     "openclaw",
				DefaultPort:          29348,
				DefaultCommandPrefix: oc.Config.Bridge.CommandPrefix,
			}
		},
		Config: func() (example string, data any, upgrader configupgrade.Upgrader) {
			return exampleNetworkConfig, &oc.Config, configupgrade.SimpleUpgrader(upgradeConfig)
		},
		DBMeta: func() database.MetaTypes {
			return database.MetaTypes{
				Portal:    func() any { return &PortalMetadata{} },
				Message:   func() any { return &MessageMetadata{} },
				UserLogin: func() any { return &UserLoginMetadata{} },
				Ghost:     func() any { return &GhostMetadata{} },
			}
		},
		Capabilities: func() *bridgev2.NetworkGeneralCapabilities {
			caps := agentremote.DefaultNetworkCapabilities()
			caps.DisappearingMessages = false
			return caps
		},
		LoadLogin: agentremote.TypedClientLoader(agentremote.TypedClientLoaderSpec[*OpenClawClient]{
			Accept: func(login *bridgev2.UserLogin) (bool, string) {
				meta := loginMetadata(login)
				return strings.EqualFold(strings.TrimSpace(meta.Provider), ProviderOpenClaw), "This bridge only supports OpenClaw logins."
			},
			LoadUserLoginConfig: agentremote.LoadUserLoginConfig[*OpenClawClient]{
				Mu:         &oc.clientsMu,
				Clients:    oc.clients,
				BridgeName: "OpenClaw",
				Update: func(e *OpenClawClient, l *bridgev2.UserLogin) {
					e.SetUserLogin(l)
				},
				Create: func(l *bridgev2.UserLogin) (*OpenClawClient, error) {
					return newOpenClawClient(l, oc)
				},
			},
		}),
		LoginFlows: func() []bridgev2.LoginFlow {
			return agentremote.SingleLoginFlow(oc.openClawEnabled(), bridgev2.LoginFlow{
				ID:          ProviderOpenClaw,
				Name:        "OpenClaw",
				Description: "Create a login for an OpenClaw gateway.",
			})
		},
		CreateLogin: func(_ context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
			if err := agentremote.ValidateSingleLoginFlow(flowID, ProviderOpenClaw, oc.openClawEnabled()); err != nil {
				return nil, err
			}
			return &OpenClawLogin{User: user, Connector: oc}, nil
		},
	})
	return oc
}

func (oc *OpenClawConnector) openClawEnabled() bool {
	return oc.Config.OpenClaw.Enabled == nil || *oc.Config.OpenClaw.Enabled
}
