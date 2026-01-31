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
	if content.Temperature != nil {
		changes = append(changes, fmt.Sprintf("temperature=%.2f", *content.Temperature))
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
	if content.ToolsEnabled != nil {
		if *content.ToolsEnabled {
			changes = append(changes, "tools=on")
		} else {
			changes = append(changes, "tools=off")
		}
	}
	if content.WebSearchEnabled != nil {
		if *content.WebSearchEnabled {
			changes = append(changes, "web_search=on")
		} else {
			changes = append(changes, "web_search=off")
		}
	}
	if content.FileSearchEnabled != nil {
		if *content.FileSearchEnabled {
			changes = append(changes, "file_search=on")
		} else {
			changes = append(changes, "file_search=off")
		}
	}
	if content.CodeInterpreterEnabled != nil {
		if *content.CodeInterpreterEnabled {
			changes = append(changes, "code_interpreter=on")
		} else {
			changes = append(changes, "code_interpreter=off")
		}
	}

	if len(changes) > 0 {
		client.sendSystemNotice(ctx, portal, fmt.Sprintf("Configuration updated: %s", strings.Join(changes, ", ")))
	}

	logEvent := log.Info().Str("model", content.Model)
	if content.Temperature != nil {
		logEvent = logEvent.Float64("temperature", *content.Temperature)
	}
	logEvent.Msg("Updated room configuration from state event")
}

func (oc *OpenAIConnector) GetCapabilities() *bridgev2.NetworkGeneralCapabilities {
	return &bridgev2.NetworkGeneralCapabilities{
		// Enable disappearing messages - we just delete from Matrix and DB
		DisappearingMessages: true,
		Provisioning: bridgev2.ProvisioningCapabilities{
			ResolveIdentifier: bridgev2.ResolveIdentifierCapabilities{
				CreateDM:       true,
				LookupUsername: true,
				ContactList:    true,
				Search:         true,
			},
		},
	}
}

func (oc *OpenAIConnector) GetBridgeInfoVersion() (info, capabilities int) {
	// Bump capabilities version when room features change.
	// v2: Added UpdateBridgeInfo call on model switch to properly broadcast capability changes
	return 1, 2
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
		Ghost: func() any {
			return &GhostMetadata{}
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

// hasBeeperConfig returns true if Beeper credentials are configured in the bridge config.
func (oc *OpenAIConnector) hasBeeperConfig() bool {
	return oc.Config.Beeper.BaseURL != "" && oc.Config.Beeper.Token != ""
}

// Package-level flow definitions
var (
	beeperFlow = bridgev2.LoginFlow{
		ID:          LoginFlowIDBeeper,
		Name:        "Beeper AI",
		Description: "Connect to Beeper AI (automatic)",
	}
	baseFlows = []bridgev2.LoginFlow{
		{ID: LoginFlowIDOpenAI, Name: "OpenAI", Description: "Use your own OpenAI API key."},
		{ID: LoginFlowIDOpenRouter, Name: "OpenRouter", Description: "Use your own OpenRouter API key."},
		{ID: LoginFlowIDCustom, Name: "Custom OpenAI-compatible", Description: "Use a custom OpenAI-compatible API endpoint."},
	}
)

func (oc *OpenAIConnector) GetLoginFlows() []bridgev2.LoginFlow {
	if oc.hasBeeperConfig() {
		// When Beeper credentials are available, show Beeper as first option plus self-hosted options
		return append([]bridgev2.LoginFlow{beeperFlow}, baseFlows...)
	}
	return baseFlows
}

func (oc *OpenAIConnector) CreateLogin(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
	// Validate by checking if flowID is in available flows
	flows := oc.GetLoginFlows()
	valid := false
	for _, f := range flows {
		if f.ID == flowID {
			valid = true
			break
		}
	}
	if !valid {
		return nil, fmt.Errorf("login flow %s is not available", flowID)
	}
	return &OpenAILogin{User: user, Connector: oc, FlowID: flowID, Provider: flowID}, nil
}
