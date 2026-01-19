package connector

import (
	"context"
	"fmt"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
)

// Provider constants - all use OpenAI SDK with different base URLs
const (
	ProviderBeeper     = "beeper"     // Beeper's OpenRouter proxy
	ProviderOpenAI     = "openai"     // Direct OpenAI API
	ProviderOpenRouter = "openrouter" // Direct OpenRouter API
	ProviderCustom     = "custom"     // Custom OpenAI-compatible endpoint
)

// Login flow IDs
const (
	LoginFlowIDBeeper     = "beeper"     // Cloud mode (auto-completes using config credentials)
	LoginFlowIDOpenAI     = "openai"     // Self-hosted
	LoginFlowIDOpenRouter = "openrouter" // Self-hosted
	LoginFlowIDCustom     = "custom"     // Self-hosted
)

// providerBaseURLs maps provider names to their API base URLs
// For Beeper provider, the base URL is provided by the SDK (hungryserv endpoint)
var providerBaseURLs = map[string]string{
	ProviderOpenAI:     "https://api.openai.com/v1",
	ProviderOpenRouter: "https://openrouter.ai/api/v1",
}

// providerFieldConfig defines the login form fields for a provider
type providerFieldConfig struct {
	keyName, keyDesc string
	needsURL         bool
	urlName, urlDesc string
}

// providerFieldConfigs maps providers to their login field configuration
var providerFieldConfigs = map[string]providerFieldConfig{
	ProviderBeeper:     {"Beeper Access Token", "Your Beeper Matrix access token", true, "Beeper AI Proxy URL", "Your Beeper homeserver AI proxy endpoint"},
	ProviderOpenAI:     {"OpenAI API Key", "Generate one at https://platform.openai.com/account/api-keys", false, "", ""},
	ProviderOpenRouter: {"OpenRouter API Key", "Generate one at https://openrouter.ai/keys", false, "", ""},
	ProviderCustom:     {"API Key", "API key for authentication", true, "Base URL", "OpenAI-compatible API endpoint (e.g., https://api.example.com/v1)"},
}

var (
	_ bridgev2.LoginProcess          = (*OpenAILogin)(nil)
	_ bridgev2.LoginProcessUserInput = (*OpenAILogin)(nil)
)

// OpenAILogin maps a Matrix user to a synthetic OpenAI "login".
type OpenAILogin struct {
	User      *bridgev2.User
	Connector *OpenAIConnector
	FlowID    string
	Provider  string // Set by CreateLogin based on flow ID
}

func (ol *OpenAILogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	// If Beeper provider and config has credentials, complete immediately (zero-step login)
	if ol.Provider == ProviderBeeper && ol.Connector.hasBeeperConfig() {
		return ol.finishLogin(ctx, ol.Connector.Config.Beeper.Token, ol.Connector.Config.Beeper.BaseURL)
	}
	return ol.credentialsStep(), nil
}

func (ol *OpenAILogin) Cancel() {}

func (ol *OpenAILogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	key := strings.TrimSpace(input["api_key"])
	if key == "" {
		return nil, fmt.Errorf("please enter an API key")
	}

	var baseURL string
	switch ol.Provider {
	case ProviderBeeper, ProviderCustom:
		baseURL = strings.TrimSpace(input["base_url"])
		if baseURL == "" {
			return nil, fmt.Errorf("please enter a base URL")
		}
	default:
		baseURL = providerBaseURLs[ol.Provider]
	}

	if baseURL == "" && (ol.Provider == ProviderOpenAI || ol.Provider == ProviderOpenRouter) {
		return nil, fmt.Errorf("no base URL configured for provider %s", ol.Provider)
	}

	return ol.finishLogin(ctx, key, baseURL)
}

func (ol *OpenAILogin) credentialsStep() *bridgev2.LoginStep {
	cfg, ok := providerFieldConfigs[ol.Provider]
	if !ok {
		cfg = providerFieldConfigs[ProviderCustom]
	}

	var fields []bridgev2.LoginInputDataField
	if cfg.needsURL {
		fields = append(fields, bridgev2.LoginInputDataField{
			Type:        bridgev2.LoginInputFieldTypeURL,
			ID:          "base_url",
			Name:        cfg.urlName,
			Description: cfg.urlDesc,
		})
	}
	fields = append(fields, bridgev2.LoginInputDataField{
		Type:        bridgev2.LoginInputFieldTypeToken,
		ID:          "api_key",
		Name:        cfg.keyName,
		Description: cfg.keyDesc,
	})

	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       "io.ai-bridge.openai.enter_credentials",
		Instructions: "Enter your API credentials",
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: fields,
		},
	}
}

func (ol *OpenAILogin) finishLogin(ctx context.Context, apiKey, baseURL string) (*bridgev2.LoginStep, error) {
	loginID := makeUserLoginID(ol.User.MXID)
	meta := &UserLoginMetadata{
		Provider: ol.Provider,
		APIKey:   apiKey,
		BaseURL:  baseURL,
	}
	login, err := ol.User.NewLogin(ctx, &database.UserLogin{
		ID:         loginID,
		RemoteName: "Beeper AI",
		Metadata:   meta,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create login: %w", err)
	}

	// Load login (which validates and caches the client internally)
	err = ol.Connector.LoadUserLogin(ctx, login)
	if err != nil {
		return nil, fmt.Errorf("failed to load client: %w", err)
	}

	// Get the client from login.Client field (set by LoadUserLogin)
	client := login.Client
	if client == nil {
		return nil, fmt.Errorf("failed to load client: client is nil")
	}

	// Trigger connection in background with a long-lived context
	// (the request context gets cancelled after login returns)
	go client.Connect(login.Log.WithContext(context.Background()))

	return &bridgev2.LoginStep{
		Type:   bridgev2.LoginStepTypeComplete,
		StepID: "io.ai-bridge.openai.complete",
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLoginID: login.ID,
			UserLogin:   login,
		},
	}, nil
}
