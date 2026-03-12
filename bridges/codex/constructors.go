package codex

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"go.mau.fi/util/configupgrade"
	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"

	"github.com/beeper/agentremote"
	"github.com/beeper/agentremote/pkg/aidb"
)

func NewConnector() *CodexConnector {
	cc := &CodexConnector{}
	cc.ConnectorBase = agentremote.NewConnector(agentremote.ConnectorSpec{
		ProtocolID: "ai-codex",
		Init: func(bridge *bridgev2.Bridge) {
			cc.br = bridge
			if bridge != nil && bridge.DB != nil && bridge.DB.Database != nil {
				cc.db = aidb.NewChild(
					bridge.DB.Database,
					dbutil.ZeroLogger(bridge.Log.With().Str("db_section", "codex_bridge").Logger()),
				)
			}
			agentremote.EnsureClientMap(&cc.clientsMu, &cc.clients)
		},
		Start: func(ctx context.Context) error {
			db := cc.bridgeDB()
			if err := aidb.Upgrade(ctx, db, "codex_bridge", "codex bridge database not initialized"); err != nil {
				return err
			}
			cc.applyRuntimeDefaults()
			agentremote.PrimeUserLoginCache(ctx, cc.br)
			cc.reconcileHostAuthLogins(ctx)
			return nil
		},
		Stop: func(context.Context) {
			agentremote.StopClients(&cc.clientsMu, &cc.clients)
		},
		Name: func() bridgev2.BridgeName {
			return bridgev2.BridgeName{
				DisplayName:          "Codex Bridge",
				NetworkURL:           "https://github.com/openai/codex",
				NetworkID:            "codex",
				BeeperBridgeType:     "codex",
				DefaultPort:          29346,
				DefaultCommandPrefix: cc.Config.Bridge.CommandPrefix,
			}
		},
		Config: func() (example string, data any, upgrader configupgrade.Upgrader) {
			return exampleNetworkConfig, &cc.Config, configupgrade.SimpleUpgrader(upgradeConfig)
		},
		DBMeta: func() database.MetaTypes {
			return agentremote.BuildMetaTypes(
				func() any { return &PortalMetadata{} },
				func() any { return &MessageMetadata{} },
				func() any { return &UserLoginMetadata{} },
				func() any { return &GhostMetadata{} },
			)
		},
		LoadLogin: agentremote.TypedClientLoader(agentremote.TypedClientLoaderSpec[*CodexClient]{
			Accept: func(login *bridgev2.UserLogin) (bool, string) {
				meta := loginMetadata(login)
				if !strings.EqualFold(strings.TrimSpace(meta.Provider), ProviderCodex) {
					return false, "This bridge only supports Codex logins."
				}
				if !cc.codexEnabled() {
					return false, "Codex integration is disabled in the configuration."
				}
				return true, ""
			},
			LoadUserLoginConfig: agentremote.LoadUserLoginConfig[*CodexClient]{
				Mu:         &cc.clientsMu,
				Clients:    cc.clients,
				BridgeName: "Codex",
				MakeBroken: func(l *bridgev2.UserLogin, reason string) *agentremote.BrokenLoginClient {
					return newBrokenLoginClient(l, cc, reason)
				},
				Update: func(e *CodexClient, l *bridgev2.UserLogin) {
					e.SetUserLogin(l)
				},
				Create: func(l *bridgev2.UserLogin) (*CodexClient, error) {
					return newCodexClient(l, cc)
				},
				AfterLoad: func(c *CodexClient) {
					c.scheduleBootstrap()
				},
			},
		}),
		LoginFlows: func() []bridgev2.LoginFlow {
			if !cc.codexEnabled() {
				return nil
			}
			return []bridgev2.LoginFlow{
				{
					ID:          FlowCodexAPIKey,
					Name:        "API Key",
					Description: "Sign in with an OpenAI API key using codex app-server.",
				},
				{
					ID:          FlowCodexChatGPT,
					Name:        "ChatGPT",
					Description: "Open browser login and authenticate with your ChatGPT account.",
				},
				{
					ID:          FlowCodexChatGPTExternalTokens,
					Name:        "ChatGPT external tokens",
					Description: "Provide externally managed ChatGPT id/access tokens.",
				},
			}
		},
		CreateLogin: func(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
			if !cc.codexEnabled() {
				return nil, fmt.Errorf("login flow %s is not available", flowID)
			}
			if !slices.ContainsFunc(cc.GetLoginFlows(), func(f bridgev2.LoginFlow) bool { return f.ID == flowID }) {
				return nil, fmt.Errorf("login flow %s is not available", flowID)
			}
			if err := cc.ensureHostAuthLoginForUser(ctx, user); err != nil && cc.br != nil {
				cc.br.Log.Debug().Err(err).Stringer("mxid", user.MXID).Msg("Host-auth reconcile: create-login reconcile failed")
			}
			return &CodexLogin{User: user, Connector: cc, FlowID: flowID}, nil
		},
	})
	return cc
}
