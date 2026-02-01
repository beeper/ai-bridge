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

// providerConfig defines login configuration for a provider
type providerConfig struct {
	baseURL  string // API base URL (empty if provided at runtime)
	keyName  string // Display name for API key field
	keyDesc  string // Description for API key field
	needsURL bool   // Whether provider needs URL input
	urlName  string // Display name for URL field
	urlDesc  string // Description for URL field
}

// providerConfigs maps providers to their unified configuration
var providerConfigs = map[string]providerConfig{
	ProviderBeeper:     {"", "Beeper Access Token", "Your Beeper Matrix access token", true, "Beeper AI Proxy URL", "Your Beeper homeserver AI proxy endpoint"},
	ProviderOpenAI:     {"https://api.openai.com/v1", "OpenAI API Key", "Generate one at https://platform.openai.com/account/api-keys", false, "", ""},
	ProviderOpenRouter: {"https://openrouter.ai/api/v1", "OpenRouter API Key", "Generate one at https://openrouter.ai/keys", false, "", ""},
	ProviderCustom:     {"", "API Key", "API key for authentication", true, "Base URL", "OpenAI-compatible API endpoint (e.g., https://api.example.com/v1)"},
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
		return nil, &ErrAPIKeyRequired
	}

	cfg := providerConfigs[ol.Provider]
	var baseURL string
	if cfg.needsURL {
		baseURL = strings.TrimSpace(input["base_url"])
		if baseURL == "" {
			return nil, &ErrBaseURLRequired
		}
	} else {
		baseURL = cfg.baseURL
	}

	return ol.finishLogin(ctx, key, baseURL)
}

func (ol *OpenAILogin) credentialsStep() *bridgev2.LoginStep {
	cfg, ok := providerConfigs[ol.Provider]
	if !ok {
		cfg = providerConfigs[ProviderCustom]
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
	loginID := makeUserLoginID(ol.User.MXID, ol.Provider, apiKey)
	meta := &UserLoginMetadata{
		Provider: ol.Provider,
		APIKey:   apiKey,
		BaseURL:  baseURL,
	}
	login, err := ol.User.NewLogin(ctx, &database.UserLogin{
		ID:         loginID,
		RemoteName: formatRemoteName(ol.Provider, apiKey, baseURL),
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

	// Trigger connection in background with a long-lived context
	// (the request context gets cancelled after login returns)
	go login.Client.Connect(login.Log.WithContext(context.Background()))

	return &bridgev2.LoginStep{
		Type:   bridgev2.LoginStepTypeComplete,
		StepID: "io.ai-bridge.openai.complete",
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLoginID: login.ID,
			UserLogin:   login,
		},
	}, nil
}

// formatRemoteName generates a display name for the account based on provider.
func formatRemoteName(provider, apiKey, baseURL string) string {
	switch provider {
	case ProviderBeeper:
		return "Beeper AI"
	case ProviderOpenAI:
		return fmt.Sprintf("OpenAI (%s)", maskAPIKey(apiKey))
	case ProviderOpenRouter:
		return fmt.Sprintf("OpenRouter (%s)", maskAPIKey(apiKey))
	case ProviderCustom:
		return fmt.Sprintf("Custom (%s, %s)", baseURL, maskAPIKey(apiKey))
	default:
		return "AI Bridge"
	}
}

// maskAPIKey returns a masked version of the API key showing first 3 and last 3 chars.
func maskAPIKey(key string) string {
	if len(key) <= 6 {
		return "***"
	}
	return fmt.Sprintf("%s...%s", key[:3], key[len(key)-3:])
}
