package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"go.mau.fi/util/configupgrade"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/matrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

const (
	defaultTemperature            = 0.4
	defaultMaxContextMessages     = 12
	defaultMaxTokens              = 512
	defaultRequestTimeout         = 45 * time.Second
	reasoningModelRequestTimeout  = 5 * time.Minute // Extended timeout for O1/O3 models
)

var (
	_ bridgev2.NetworkConnector                = (*OpenAIConnector)(nil)
	_ bridgev2.PortalBridgeInfoFillingNetwork = (*OpenAIConnector)(nil)
)

// OpenAIConnector wires mautrix bridgev2 to the OpenAI chat APIs.
type OpenAIConnector struct {
	br     *bridgev2.Bridge
	Config Config
}

func (oc *OpenAIConnector) Init(bridge *bridgev2.Bridge) {
	oc.br = bridge
}

func (oc *OpenAIConnector) Stop(ctx context.Context) {
	// Future: cleanup background tasks if needed
	// For now, OpenAI connector has no background loops to stop
}

func (oc *OpenAIConnector) Start(ctx context.Context) error {
	if oc.Config.OpenAI.RequestTimeout == 0 {
		oc.Config.OpenAI.RequestTimeout = defaultRequestTimeout
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
	if oc.Config.OpenAI.EditDebounceMs == 0 {
		oc.Config.OpenAI.EditDebounceMs = 200
	}
	if oc.Config.OpenAI.TransientDebounceMs == 0 {
		oc.Config.OpenAI.TransientDebounceMs = 50
	}
	if oc.Config.Bridge.CommandPrefix == "" {
		oc.Config.Bridge.CommandPrefix = "!gpt"
	}
	// Check for default OpenAI API key from environment variable
	sharedAPIKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if sharedAPIKey == "" {
		oc.br.Log.Warn().Msg("No default OpenAI API key configured; users must supply their own during login")
	} else {
		go oc.ensureSharedKeyLogins(sharedAPIKey)
	}

	// Register custom Matrix event handlers
	oc.registerCustomEventHandlers()

	return nil
}

// registerCustomEventHandlers registers handlers for custom Matrix state events
func (oc *OpenAIConnector) registerCustomEventHandlers() {
	// Type assert the Matrix connector to get the concrete type with EventProcessor
	matrixConnector, ok := oc.br.Matrix.(*matrix.Connector)
	if !ok {
		oc.br.Log.Warn().Msg("Cannot register custom event handlers: Matrix connector type assertion failed")
		return
	}

	// Register handler for our custom room config event
	matrixConnector.EventProcessor.On(RoomConfigEventType, oc.handleRoomConfigEvent)
	oc.br.Log.Info().Msg("Registered custom room config event handler")
}

// handleRoomConfigEvent processes Matrix room config state events
func (oc *OpenAIConnector) handleRoomConfigEvent(ctx context.Context, evt *event.Event) {
	log := oc.br.Log.With().
		Str("component", "room_config_handler").
		Str("room_id", evt.RoomID.String()).
		Str("sender", evt.Sender.String()).
		Logger()

	// Look up portal by Matrix room ID
	portal, err := oc.br.GetPortalByMXID(ctx, evt.RoomID)
	if err != nil {
		log.Err(err).Msg("Failed to get portal for room config event")
		return
	}
	if portal == nil {
		log.Debug().Msg("No portal found for room, ignoring config event")
		return
	}

	// Parse event content
	var content RoomConfigEventContent
	if err := json.Unmarshal(evt.Content.VeryRaw, &content); err != nil {
		log.Warn().Err(err).Msg("Failed to parse room config event content")
		return
	}

	// Get the user who sent the event and their login
	user, err := oc.br.GetUserByMXID(ctx, evt.Sender)
	if err != nil || user == nil {
		log.Warn().Err(err).Msg("Failed to get user for room config event")
		return
	}

	login := user.GetDefaultLogin()
	if login == nil {
		log.Warn().Msg("User has no active login, cannot process config")
		return
	}

	client, ok := login.Client.(*AIClient)
	if !ok || client == nil {
		log.Warn().Msg("Invalid client type for user login")
		return
	}

	// Validate model if specified
	if content.Model != "" {
		valid, err := client.validateModel(ctx, content.Model)
		if err != nil {
			log.Warn().Err(err).Str("model", content.Model).Msg("Failed to validate model")
		} else if !valid {
			log.Warn().Str("model", content.Model).Msg("Invalid model specified, ignoring")
			client.sendSystemNotice(ctx, portal, fmt.Sprintf("Invalid model: %s. Configuration not applied.", content.Model))
			return
		}
	}

	// Update portal metadata
	client.updatePortalConfig(ctx, portal, &content)

	// Send confirmation notice
	var changes []string
	if content.Model != "" {
		changes = append(changes, fmt.Sprintf("model=%s", content.Model))
	}
	if content.Temperature > 0 {
		changes = append(changes, fmt.Sprintf("temperature=%.2f", content.Temperature))
	}
	if content.MaxContextMessages > 0 {
		changes = append(changes, fmt.Sprintf("context=%d messages", content.MaxContextMessages))
	}
	if content.MaxCompletionTokens > 0 {
		changes = append(changes, fmt.Sprintf("max_tokens=%d", content.MaxCompletionTokens))
	}
	if content.SystemPrompt != "" {
		changes = append(changes, "system_prompt updated")
	}
	if content.ReasoningEffort != "" {
		changes = append(changes, fmt.Sprintf("reasoning_effort=%s", content.ReasoningEffort))
	}
	if content.ConversationMode != "" {
		changes = append(changes, fmt.Sprintf("conversation_mode=%s", content.ConversationMode))
	}
	if content.ToolsEnabled {
		changes = append(changes, "tools=on")
	}
	if content.WebSearchEnabled {
		changes = append(changes, "web_search=on")
	}
	if content.CodeInterpreterEnabled {
		changes = append(changes, "code_interpreter=on")
	}

	if len(changes) > 0 {
		client.sendSystemNotice(ctx, portal, fmt.Sprintf("Configuration updated: %s", strings.Join(changes, ", ")))
	}

	log.Info().
		Str("model", content.Model).
		Float64("temperature", content.Temperature).
		Msg("Updated room configuration from state event")
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

// FillPortalBridgeInfo sets custom room type for AI rooms
func (oc *OpenAIConnector) FillPortalBridgeInfo(portal *bridgev2.Portal, content *event.BridgeEventContent) {
	content.BeeperRoomTypeV2 = "ai"
}

func (oc *OpenAIConnector) GetName() bridgev2.BridgeName {
	return bridgev2.BridgeName{
		DisplayName:          "Beeper AI",
		NetworkURL:           "https://www.beeper.com/ai",
		NetworkIcon:          "mxc://maunium.net/HPiAFz2uVH54camzatoorkWY",
		NetworkID:            "beeper-ai",
		BeeperBridgeType:     "beeper-ai",
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
		// Check for shared API key from environment variable
		key = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if key == "" {
		return fmt.Errorf("no OpenAI API key available for this login; please relogin using the personal API key flow")
	}
	client, err := newAIClient(login, oc, key)
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
	// Check for shared API key from environment variable
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" {
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
		// Check for shared API key from environment variable
		if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" {
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

func (oc *OpenAIConnector) ensureSharedKeyLogins(apiKey string) {
	if oc.br == nil || oc.br.Config == nil {
		return
	}
	key := strings.TrimSpace(apiKey)
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
		// For shared-key login, use the key without optional per-user settings
		step, err := loginProc.finishLogin(ctx, key, "")
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
