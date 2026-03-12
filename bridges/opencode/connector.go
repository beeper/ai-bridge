package opencode

import (
	"context"
	"slices"
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
	_ bridgev2.NetworkConnector               = (*OpenCodeConnector)(nil)
	_ bridgev2.PortalBridgeInfoFillingNetwork = (*OpenCodeConnector)(nil)
)

type OpenCodeConnector struct {
	*agentremote.ConnectorBase
	br     *bridgev2.Bridge
	Config Config

	clientsMu sync.Mutex
	clients   map[networkid.UserLoginID]bridgev2.NetworkAPI
}

func NewConnector() *OpenCodeConnector {
	oc := &OpenCodeConnector{}
	oc.ConnectorBase = agentremote.NewConnector(agentremote.ConnectorSpec{
		ProtocolID: "ai-opencode",
		Init: func(bridge *bridgev2.Bridge) {
			oc.br = bridge
			agentremote.EnsureClientMap(&oc.clientsMu, &oc.clients)
		},
		Start: func(context.Context) error {
			if oc.Config.Bridge.CommandPrefix == "" {
				oc.Config.Bridge.CommandPrefix = "!opencode"
			}
			if oc.Config.OpenCode.Enabled == nil {
				oc.Config.OpenCode.Enabled = ptr.Ptr(true)
			}
			return nil
		},
		Stop: func(context.Context) {
			agentremote.StopClients(&oc.clientsMu, &oc.clients)
		},
		Name: func() bridgev2.BridgeName {
			return bridgev2.BridgeName{
				DisplayName:          "OpenCode Bridge",
				NetworkURL:           "https://api.ai",
				NetworkID:            "opencode",
				BeeperBridgeType:     "opencode",
				DefaultPort:          29347,
				DefaultCommandPrefix: oc.Config.Bridge.CommandPrefix,
			}
		},
		Config: func() (example string, data any, upgrader configupgrade.Upgrader) {
			return exampleNetworkConfig, &oc.Config, configupgrade.SimpleUpgrader(upgradeConfig)
		},
		DBMeta: func() database.MetaTypes {
			return agentremote.BuildMetaTypes(
				func() any { return &PortalMetadata{} },
				func() any { return &MessageMetadata{} },
				func() any { return &UserLoginMetadata{} },
				func() any { return &GhostMetadata{} },
			)
		},
		LoadLogin: agentremote.TypedClientLoader(agentremote.TypedClientLoaderSpec[*OpenCodeClient]{
			Accept: func(login *bridgev2.UserLogin) (bool, string) {
				meta := loginMetadata(login)
				return strings.EqualFold(strings.TrimSpace(meta.Provider), ProviderOpenCode), "This bridge only supports OpenCode logins."
			},
			LoadUserLoginConfig: agentremote.LoadUserLoginConfig[*OpenCodeClient]{
				Mu:         &oc.clientsMu,
				Clients:    oc.clients,
				BridgeName: "OpenCode",
				Update: func(e *OpenCodeClient, l *bridgev2.UserLogin) {
					e.SetUserLogin(l)
				},
				Create: func(l *bridgev2.UserLogin) (*OpenCodeClient, error) {
					return newOpenCodeClient(l, oc)
				},
			},
		}),
		LoginFlows: func() []bridgev2.LoginFlow {
			if !oc.openCodeEnabled() {
				return nil
			}
			return []bridgev2.LoginFlow{
				{
					ID:          FlowOpenCodeRemote,
					Name:        "Remote OpenCode",
					Description: "Connect to an already running OpenCode server.",
				},
				{
					ID:          FlowOpenCodeManaged,
					Name:        "Managed OpenCode",
					Description: "Let the bridge spawn and manage OpenCode processes for you.",
				},
			}
		},
		CreateLogin: func(_ context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
			if !oc.openCodeEnabled() {
				return nil, bridgev2.ErrNotLoggedIn
			}
			if !slices.ContainsFunc(oc.GetLoginFlows(), func(flow bridgev2.LoginFlow) bool {
				return flow.ID == flowID
			}) {
				return nil, bridgev2.ErrInvalidLoginFlowID
			}
			return &OpenCodeLogin{User: user, Connector: oc, FlowID: flowID}, nil
		},
	})
	return oc
}

func (oc *OpenCodeConnector) openCodeEnabled() bool {
	return oc.Config.OpenCode.Enabled == nil || *oc.Config.OpenCode.Enabled
}
