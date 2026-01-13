package connector

import (
	"context"
	"fmt"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
)

// Provider constants
const (
	ProviderBeeper     = "beeper"
	ProviderOpenAI     = "openai"
	ProviderGemini     = "gemini"
	ProviderAnthropic  = "anthropic"
	ProviderOpenRouter = "openrouter"
	ProviderCustom     = "custom"
)

// Login flow IDs
const (
	LoginFlowIDLocalBeeper = "local-beeper"
	LoginFlowIDOpenAI      = "openai"
	LoginFlowIDAnthropic   = "anthropic"
	LoginFlowIDGemini      = "gemini"
	LoginFlowIDOpenRouter  = "openrouter"
	LoginFlowIDCustom      = "custom"
)

// providerBaseURLs maps provider names to their API base URLs
// For Beeper provider, the base URL is provided by the SDK (hungryserv endpoint)
var providerBaseURLs = map[string]string{
	ProviderOpenAI:     "https://api.openai.com/v1",
	ProviderOpenRouter: "https://openrouter.ai/api/v1",
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

	return ol.finishLogin(ctx, key, baseURL)
}

func (ol *OpenAILogin) credentialsStep() *bridgev2.LoginStep {
	var fields []bridgev2.LoginInputDataField

	switch ol.Provider {
	case ProviderBeeper:
		fields = []bridgev2.LoginInputDataField{
			{
				Type:        bridgev2.LoginInputFieldTypeURL,
				ID:          "base_url",
				Name:        "Beeper AI Proxy URL",
				Description: "Your Beeper homeserver AI proxy endpoint",
			},
			{
				Type:        bridgev2.LoginInputFieldTypeToken,
				ID:          "api_key",
				Name:        "Beeper Access Token",
				Description: "Your Beeper Matrix access token",
			},
		}
	case ProviderOpenAI:
		fields = []bridgev2.LoginInputDataField{
			{
				Type:        bridgev2.LoginInputFieldTypeToken,
				ID:          "api_key",
				Name:        "OpenAI API Key",
				Description: "Generate one at https://platform.openai.com/account/api-keys",
			},
		}
	case ProviderGemini:
		fields = []bridgev2.LoginInputDataField{
			{
				Type:        bridgev2.LoginInputFieldTypeToken,
				ID:          "api_key",
				Name:        "Gemini API Key",
				Description: "Generate one at https://aistudio.google.com/apikey",
			},
		}
	case ProviderAnthropic:
		fields = []bridgev2.LoginInputDataField{
			{
				Type:        bridgev2.LoginInputFieldTypeToken,
				ID:          "api_key",
				Name:        "Anthropic API Key",
				Description: "Generate one at https://console.anthropic.com/settings/keys",
			},
		}
	case ProviderOpenRouter:
		fields = []bridgev2.LoginInputDataField{
			{
				Type:        bridgev2.LoginInputFieldTypeToken,
				ID:          "api_key",
				Name:        "OpenRouter API Key",
				Description: "Generate one at https://openrouter.ai/keys",
			},
		}
	case ProviderCustom:
		fields = []bridgev2.LoginInputDataField{
			{
				Type:        bridgev2.LoginInputFieldTypeURL,
				ID:          "base_url",
				Name:        "Base URL",
				Description: "OpenAI-compatible API endpoint (e.g., https://api.example.com/v1)",
			},
			{
				Type:        bridgev2.LoginInputFieldTypeToken,
				ID:          "api_key",
				Name:        "API Key",
				Description: "API key for authentication",
			},
		}
	}

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
		return nil, err
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
