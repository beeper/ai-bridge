package connector

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.mau.fi/util/configupgrade"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/id"
)

const (
	defaultModel              = "gpt-4.1-mini"
	defaultTemperature        = 0.4
	defaultMaxContextMessages = 12
	defaultMaxTokens          = 512
	defaultRequestTimeout     = 45 * time.Second
)

var (
	_ bridgev2.NetworkConnector = (*OpenAIConnector)(nil)
)

// OpenAIConnector wires mautrix bridgev2 to the OpenAI chat APIs.
type OpenAIConnector struct {
	br     *bridgev2.Bridge
	Config Config
}

func (oc *OpenAIConnector) Init(bridge *bridgev2.Bridge) {
	oc.br = bridge
}

func (oc *OpenAIConnector) Start(ctx context.Context) error {
	if oc.Config.OpenAI.RequestTimeout == 0 {
		oc.Config.OpenAI.RequestTimeout = defaultRequestTimeout
	}
	if oc.Config.OpenAI.DefaultModel == "" {
		oc.Config.OpenAI.DefaultModel = defaultModel
	}
	if oc.Config.OpenAI.DefaultTemperature == 0 {
		oc.Config.OpenAI.DefaultTemperature = defaultTemperature
	}
	if oc.Config.OpenAI.MaxContextMessages == 0 {
		oc.Config.OpenAI.MaxContextMessages = defaultMaxContextMessages
	}
	if oc.Config.OpenAI.MaxCompletionTokens == 0 {
		oc.Config.OpenAI.MaxCompletionTokens = defaultMaxTokens
	}
	if oc.Config.Bridge.CommandPrefix == "" {
		oc.Config.Bridge.CommandPrefix = "!gpt"
	}
	if oc.Config.OpenAI.APIKey == "" {
		oc.Config.OpenAI.APIKey = strings.TrimSpace(getEnvOrEmpty("OPENAI_API_KEY"))
	}
	if oc.Config.OpenAI.APIKey == "" {
		oc.br.Log.Warn().Msg("No default OpenAI API key configured; users must supply their own during login")
	} else {
		go oc.ensureSharedKeyLogins()
	}
	return nil
}

func (oc *OpenAIConnector) GetCapabilities() *bridgev2.NetworkGeneralCapabilities {
	return &bridgev2.NetworkGeneralCapabilities{
		Provisioning: bridgev2.ProvisioningCapabilities{
			ResolveIdentifier: bridgev2.ResolveIdentifierCapabilities{
				CreateDM:       true,
				LookupUsername: true,
				ContactList:    true,
			},
		},
	}
}

func (oc *OpenAIConnector) GetBridgeInfoVersion() (info, capabilities int) {
	return 1, 1
}

func (oc *OpenAIConnector) GetName() bridgev2.BridgeName {
	return bridgev2.BridgeName{
		DisplayName:          "ChatGPT",
		NetworkURL:           "https://chat.openai.com",
		NetworkIcon:          "mxc://maunium.net/HPiAFz2uVH54camzatoorkWY",
		NetworkID:            "openai",
		BeeperBridgeType:     "github.com/beepai/matrix-openai-bridge",
		DefaultPort:          29345,
		DefaultCommandPrefix: oc.Config.Bridge.CommandPrefix,
	}
}

func (oc *OpenAIConnector) GetConfig() (example string, data any, upgrader configupgrade.Upgrader) {
	return exampleNetworkConfig, &oc.Config, configupgrade.SimpleUpgrader(upgradeConfig)
}

func (oc *OpenAIConnector) GetDBMetaTypes() database.MetaTypes {
	return database.MetaTypes{
		Portal: func() any {
			return &PortalMetadata{}
		},
		Message: func() any {
			return &MessageMetadata{}
		},
		UserLogin: func() any {
			return &UserLoginMetadata{}
		},
	}
}

func (oc *OpenAIConnector) LoadUserLogin(ctx context.Context, login *bridgev2.UserLogin) error {
	meta := loginMetadata(login)
	key := strings.TrimSpace(meta.APIKey)
	if key == "" {
		key = strings.TrimSpace(oc.Config.OpenAI.APIKey)
	}
	if key == "" {
		return fmt.Errorf("no OpenAI API key available for this login; please relogin using the personal API key flow")
	}
	client, err := newOpenAIClient(login, oc, key)
	if err != nil {
		return err
	}
	login.Client = client
	meta.APIKey = key
	client.scheduleBootstrap()
	return nil
}

func (oc *OpenAIConnector) GetLoginFlows() []bridgev2.LoginFlow {
	var flows []bridgev2.LoginFlow
	if strings.TrimSpace(oc.Config.OpenAI.APIKey) != "" {
		flows = append(flows, bridgev2.LoginFlow{
			ID:          "shared-api-key",
			Name:        "Bridge-provided API key",
			Description: "Use the shared OpenAI API key configured by the bridge admin.",
		})
	}
	flows = append(flows, bridgev2.LoginFlow{
		ID:          "user-api-key",
		Name:        "Personal OpenAI API key",
		Description: "Provide your own OpenAI API key (displayed as a text box in supporting clients).",
	})
	return flows
}

func (oc *OpenAIConnector) CreateLogin(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
	switch flowID {
	case "shared-api-key":
		if strings.TrimSpace(oc.Config.OpenAI.APIKey) == "" {
			return nil, fmt.Errorf("bridge is not configured with a default API key")
		}
		return &OpenAILogin{
			User:      user,
			Connector: oc,
		}, nil
	case "user-api-key":
		return &OpenAILogin{
			User:             user,
			Connector:        oc,
			RequireUserInput: true,
		}, nil
	default:
		return nil, fmt.Errorf("unknown login flow: %s", flowID)
	}
}

func getEnvOrEmpty(key string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return ""
}

func (oc *OpenAIConnector) ensureSharedKeyLogins() {
	if oc.br == nil || oc.br.Config == nil {
		return
	}
	key := strings.TrimSpace(oc.Config.OpenAI.APIKey)
	if key == "" {
		return
	}
	ctx := oc.br.BackgroundCtx
	if ctx == nil {
		ctx = context.Background()
	}
	log := oc.br.Log.With().Str("component", "openai-auto-login").Logger()
	for rawUser, perms := range oc.br.Config.Permissions {
		if !strings.HasPrefix(rawUser, "@") {
			continue
		}
		if perms != nil && !perms.Login {
			continue
		}
		userID := id.UserID(rawUser)
		user, err := oc.br.GetUserByMXID(ctx, userID)
		if err != nil {
			log.Warn().Err(err).Str("user_mxid", rawUser).Msg("Failed to load user for shared-key auto login")
			continue
		}
		if user == nil {
			continue
		}
		if user.GetDefaultLogin() != nil {
			continue
		}
		log.Info().Str("user_mxid", rawUser).Msg("Provisioning shared-key OpenAI login automatically")
		loginProc := &OpenAILogin{
			User:      user,
			Connector: oc,
		}
		step, err := loginProc.finishLogin(ctx, key)
		if step != nil && step.CompleteParams != nil && step.CompleteParams.UserLogin != nil {
			login := step.CompleteParams.UserLogin
			if client := login.Client; client != nil {
				go client.Connect(ctx)
			}
		}
		if err != nil {
			log.Warn().Err(err).Str("user_mxid", rawUser).Msg("Failed to auto-provision OpenAI login")
		}
	}
}
