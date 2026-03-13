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
	bridgesdk "github.com/beeper/agentremote/sdk"
	"github.com/beeper/agentremote/pkg/aidb"
)

func NewConnector() *CodexConnector {
	cc := &CodexConnector{}
	cc.sdkConfig = &bridgesdk.Config{
		Name:             "codex",
		Description:      "A Matrix↔Codex bridge built on mautrix-go bridgev2.",
		ProtocolID:       "ai-codex",
		Agent:            codexSDKAgent(),
		ProviderIdentity: bridgesdk.ProviderIdentity{IDPrefix: "codex", LogKey: "codex_msg_id", StatusNetwork: "codex"},
		ClientCacheMu:    &cc.clientsMu,
		ClientCache:      &cc.clients,
		InitConnector: func(bridge *bridgev2.Bridge) {
			cc.br = bridge
			if bridge != nil && bridge.DB != nil && bridge.DB.Database != nil {
				cc.db = aidb.NewChild(
					bridge.DB.Database,
					dbutil.ZeroLogger(bridge.Log.With().Str("db_section", "codex_bridge").Logger()),
				)
			}
		},
		StartConnector: func(ctx context.Context, _ *bridgev2.Bridge) error {
			db := cc.bridgeDB()
			if err := aidb.Upgrade(ctx, db, "codex_bridge", "codex bridge database not initialized"); err != nil {
				return err
			}
			cc.applyRuntimeDefaults()
			agentremote.PrimeUserLoginCache(ctx, cc.br)
			cc.reconcileHostAuthLogins(ctx)
			return nil
		},
		BridgeName: func() bridgev2.BridgeName {
			return bridgev2.BridgeName{
				DisplayName:          "Codex Bridge",
				NetworkURL:           "https://github.com/openai/codex",
				NetworkID:            "codex",
				BeeperBridgeType:     "codex",
				DefaultPort:          29346,
				DefaultCommandPrefix: cc.Config.Bridge.CommandPrefix,
			}
		},
		ExampleConfig: exampleNetworkConfig,
		ConfigData:   &cc.Config,
		ConfigUpgrader: configupgrade.SimpleUpgrader(upgradeConfig),
		DBMeta: func() database.MetaTypes {
			return agentremote.BuildMetaTypes(
				func() any { return &PortalMetadata{} },
				func() any { return &MessageMetadata{} },
				func() any { return &UserLoginMetadata{} },
				func() any { return &GhostMetadata{} },
			)
		},
		AcceptLogin: func(login *bridgev2.UserLogin) (bool, string) {
			meta := loginMetadata(login)
			if !strings.EqualFold(strings.TrimSpace(meta.Provider), ProviderCodex) {
				return false, "This bridge only supports Codex logins."
			}
			if !cc.codexEnabled() {
				return false, "Codex integration is disabled in the configuration."
			}
			return true, ""
		},
		MakeBrokenLogin: func(l *bridgev2.UserLogin, reason string) *agentremote.BrokenLoginClient {
			return newBrokenLoginClient(l, cc, reason)
		},
		CreateClient: func(l *bridgev2.UserLogin) (bridgev2.NetworkAPI, error) {
			return newCodexClient(l, cc)
		},
		UpdateClient: func(client bridgev2.NetworkAPI, login *bridgev2.UserLogin) {
			if c, ok := client.(*CodexClient); ok {
				c.SetUserLogin(login)
			}
		},
		AfterLoadClient: func(client bridgev2.NetworkAPI) {
			if c, ok := client.(*CodexClient); ok {
				c.scheduleBootstrap()
			}
		},
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
	}
	cc.ConnectorBase = bridgesdk.NewConnectorBase(cc.sdkConfig)
	return cc
}
