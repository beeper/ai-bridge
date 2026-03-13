package sdk

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.mau.fi/util/configupgrade"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"github.com/beeper/agentremote"
)

type sdkConnector struct {
	*agentremote.ConnectorBase
	cfg *Config
}

func newSDKConnector(cfg *Config) *sdkConnector {
	sc := &sdkConnector{
		cfg:           cfg,
		ConnectorBase: NewConnectorBase(cfg),
	}
	return sc
}

// NewConnectorBase builds an SDK-backed connector base that can be embedded by custom bridges.
func NewConnectorBase(cfg *Config) *agentremote.ConnectorBase {
	var localMu sync.Mutex
	var localClients map[networkid.UserLoginID]bridgev2.NetworkAPI
	var br *bridgev2.Bridge
	mu := &localMu
	clientsRef := &localClients
	if cfg.ClientCacheMu != nil {
		mu = cfg.ClientCacheMu
	}
	if cfg.ClientCache != nil {
		clientsRef = cfg.ClientCache
	}

	protocolID := cfg.ProtocolID
	if protocolID == "" {
		protocolID = "sdk-" + cfg.Name
	}
	return agentremote.NewConnector(agentremote.ConnectorSpec{
		ProtocolID: protocolID,
		Init: func(bridge *bridgev2.Bridge) {
			br = bridge
			agentremote.EnsureClientMap(mu, clientsRef)
			if cfg.InitConnector != nil {
				cfg.InitConnector(bridge)
			}
		},
		Start: func(ctx context.Context) error {
			registerCommands(br, cfg)
			if cfg.StartConnector != nil {
				return cfg.StartConnector(ctx, br)
			}
			return nil
		},
		Stop: func(ctx context.Context) {
			agentremote.StopClients(mu, clientsRef)
			if cfg.StopConnector != nil {
				cfg.StopConnector(ctx, br)
			}
		},
		Name: func() bridgev2.BridgeName {
			if cfg.BridgeName != nil {
				return cfg.BridgeName()
			}
			desc := cfg.Description
			if desc == "" {
				desc = fmt.Sprintf("A Matrix↔%s bridge for Beeper.", cfg.Name)
			}
			port := cfg.Port
			if port == 0 {
				port = 29400
			}
			return bridgev2.BridgeName{
				DisplayName:      cfg.Name,
				NetworkURL:       "https://github.com/beeper/agentremote",
				NetworkID:        cfg.Name,
				BeeperBridgeType: cfg.Name,
				DefaultPort:      uint16(port),
			}
		},
		Config: func() (string, any, configupgrade.Upgrader) {
			example := cfg.ExampleConfig
			if example == "" {
				example = "{}"
			}
			return example, cfg.ConfigData, cfg.ConfigUpgrader
		},
		DBMeta: func() database.MetaTypes {
			if cfg.DBMeta != nil {
				return cfg.DBMeta()
			}
			return database.MetaTypes{
				Portal:    func() any { return &map[string]any{} },
				Message:   func() any { return &map[string]any{} },
				UserLogin: func() any { return &map[string]any{} },
				Ghost:     func() any { return &map[string]any{} },
			}
		},
		Capabilities: func() *bridgev2.NetworkGeneralCapabilities {
			if cfg.NetworkCapabilities != nil {
				return cfg.NetworkCapabilities()
			}
			return agentremote.DefaultNetworkCapabilities()
		},
		BridgeInfoVersion: func() (info, capabilities int) {
			if cfg.BridgeInfoVersion != nil {
				return cfg.BridgeInfoVersion()
			}
			return agentremote.DefaultBridgeInfoVersion()
		},
		FillBridgeInfo: func(portal *bridgev2.Portal, content *event.BridgeEventContent) {
			if cfg.FillBridgeInfo != nil {
				cfg.FillBridgeInfo(portal, content)
				return
			}
			if portal == nil || content == nil || protocolID == "" {
				return
			}
			agentremote.ApplyAIBridgeInfo(content, protocolID, portal.RoomType, agentremote.AIRoomKindAgent)
		},
		LoadLogin: func(_ context.Context, login *bridgev2.UserLogin) error {
			if cfg.AcceptLogin != nil {
				ok, reason := cfg.AcceptLogin(login)
				if !ok {
					if strings.TrimSpace(reason) == "" {
						reason = "This login is not supported."
					}
					makeBroken := cfg.MakeBrokenLogin
					if makeBroken == nil {
						makeBroken = func(l *bridgev2.UserLogin, msg string) *agentremote.BrokenLoginClient {
							return agentremote.NewBrokenLoginClient(l, msg)
						}
					}
					login.Client = makeBroken(login, reason)
					return nil
				}
			}
			return agentremote.LoadUserLogin(login, agentremote.LoadUserLoginConfig[bridgev2.NetworkAPI]{
				Mu:         mu,
				Clients:    *clientsRef,
				BridgeName: cfg.Name,
				MakeBroken: cfg.MakeBrokenLogin,
				Update: func(client bridgev2.NetworkAPI, login *bridgev2.UserLogin) {
					if cfg.UpdateClient != nil {
						cfg.UpdateClient(client, login)
						return
					}
					if typed, ok := client.(*sdkClient); ok {
						typed.SetUserLogin(login)
					}
				},
				Create: func(login *bridgev2.UserLogin) (bridgev2.NetworkAPI, error) {
					if cfg.CreateClient != nil {
						return cfg.CreateClient(login)
					}
					return newSDKClient(login, cfg), nil
				},
				AfterLoad: func(client bridgev2.NetworkAPI) {
					if cfg.AfterLoadClient != nil {
						cfg.AfterLoadClient(client)
					}
				},
			})
		},
		LoginFlows: func() []bridgev2.LoginFlow {
			if len(cfg.LoginFlows) > 0 {
				return cfg.LoginFlows
			}
			return []bridgev2.LoginFlow{{
				ID:          "sdk-default",
				Name:        cfg.Name,
				Description: fmt.Sprintf("Login to %s", cfg.Name),
			}}
		},
		CreateLogin: func(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
			if cfg.CreateLogin != nil {
				return cfg.CreateLogin(ctx, user, flowID)
			}
			if flowID == "sdk-default" {
				return &sdkAutoLogin{user: user}, nil
			}
			return nil, bridgev2.ErrInvalidLoginFlowID
		},
	})
}
