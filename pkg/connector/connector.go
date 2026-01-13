package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/util/configupgrade"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/matrix"
	"maunium.net/go/mautrix/event"
)

const (
	defaultTemperature        = 0.4
	defaultMaxContextMessages = 12
	defaultMaxTokens          = 512
)

var (
	_ bridgev2.NetworkConnector               = (*OpenAIConnector)(nil)
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
	if oc.Config.ModelCacheDuration == 0 {
		oc.Config.ModelCacheDuration = 6 * time.Hour
	}
	if oc.Config.Bridge.CommandPrefix == "" {
		oc.Config.Bridge.CommandPrefix = "!ai"
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
		return fmt.Errorf("no API key available for this login; please login again")
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

// AutoCreateBeeperLogin creates a Beeper login for a user using config credentials.
// Returns nil if Beeper config is not set or user already has a login.
func (oc *OpenAIConnector) AutoCreateBeeperLogin(ctx context.Context, user *bridgev2.User) (*bridgev2.UserLogin, error) {
	if !oc.hasBeeperConfig() {
		return nil, nil
	}

	// Check if user already has a login
	logins := user.GetUserLogins()
	if len(logins) > 0 {
		return logins[0], nil
	}

	// Create login with Beeper credentials from config
	loginID := makeUserLoginID(user.MXID)
	meta := &UserLoginMetadata{
		Provider: ProviderBeeper,
		APIKey:   oc.Config.Beeper.Token,
		BaseURL:  oc.Config.Beeper.BaseURL,
	}
	login, err := user.NewLogin(ctx, &database.UserLogin{
		ID:         loginID,
		RemoteName: "Beeper AI",
		Metadata:   meta,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create beeper login: %w", err)
	}

	// Load the login
	err = oc.LoadUserLogin(ctx, login)
	if err != nil {
		return nil, fmt.Errorf("failed to load beeper login: %w", err)
	}

	// Connect in background
	if login.Client != nil {
		go login.Client.Connect(oc.br.Log.WithContext(context.Background()))
	}

	oc.br.Log.Info().Str("user", user.MXID.String()).Msg("Auto-created Beeper login from config")
	return login, nil
}

// hasBeeperConfig returns true if Beeper credentials are configured in the bridge config.
func (oc *OpenAIConnector) hasBeeperConfig() bool {
	return oc.Config.Beeper.BaseURL != "" && oc.Config.Beeper.Token != ""
}

func (oc *OpenAIConnector) GetLoginFlows() []bridgev2.LoginFlow {
	flows := []bridgev2.LoginFlow{}

	// Only show Beeper provider if NOT configured in config
	// (if configured, users get auto-login and don't need to manually login)
	if !oc.hasBeeperConfig() {
		flows = append(flows, bridgev2.LoginFlow{
			ID:          LoginFlowIDLocalBeeper,
			Name:        "Local Beeper",
			Description: "Use Beeper's AI proxy (automatic setup for SDK).",
		})
	}

	// Always show other providers
	flows = append(flows,
		bridgev2.LoginFlow{
			ID:          LoginFlowIDOpenAI,
			Name:        "OpenAI",
			Description: "Use your own OpenAI API key.",
		},
		bridgev2.LoginFlow{
			ID:          LoginFlowIDAnthropic,
			Name:        "Anthropic",
			Description: "Use your own Anthropic API key.",
		},
		bridgev2.LoginFlow{
			ID:          LoginFlowIDGemini,
			Name:        "Gemini",
			Description: "Use your own Google Gemini API key.",
		},
		bridgev2.LoginFlow{
			ID:          LoginFlowIDOpenRouter,
			Name:        "OpenRouter",
			Description: "Use your own OpenRouter API key.",
		},
		bridgev2.LoginFlow{
			ID:          LoginFlowIDCustom,
			Name:        "Custom OpenAI-compatible",
			Description: "Use a custom OpenAI-compatible API endpoint.",
		},
	)

	return flows
}

func (oc *OpenAIConnector) CreateLogin(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
	login := &OpenAILogin{User: user, Connector: oc, FlowID: flowID}

	switch flowID {
	case LoginFlowIDLocalBeeper:
		login.Provider = ProviderBeeper
	case LoginFlowIDOpenAI:
		login.Provider = ProviderOpenAI
	case LoginFlowIDAnthropic:
		login.Provider = ProviderAnthropic
	case LoginFlowIDGemini:
		login.Provider = ProviderGemini
	case LoginFlowIDOpenRouter:
		login.Provider = ProviderOpenRouter
	case LoginFlowIDCustom:
		login.Provider = ProviderCustom
	default:
		return nil, fmt.Errorf("unknown login flow: %s", flowID)
	}
	return login, nil
}
