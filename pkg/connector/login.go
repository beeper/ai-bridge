package connector

import (
	"context"
	"fmt"
	"os"
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

// providerBaseURLs maps provider names to their API base URLs
var providerBaseURLs = map[string]string{
	ProviderBeeper:     "https://ai-proxy.beeper.com/v1",
	ProviderOpenAI:     "https://api.openai.com/v1",
	ProviderOpenRouter: "https://openrouter.ai/api/v1",
}

var (
	_ bridgev2.LoginProcess          = (*OpenAILogin)(nil)
	_ bridgev2.LoginProcessUserInput = (*OpenAILogin)(nil)
)

// OpenAILogin maps a Matrix user to a synthetic OpenAI "login".
type OpenAILogin struct {
	User             *bridgev2.User
	Connector        *OpenAIConnector
	RequireUserInput bool
	Provider         string // Selected provider (set after step 1)
	Step             int    // Current step: 0 = provider selection, 1 = credentials
}

func (ol *OpenAILogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	if !ol.RequireUserInput {
		// Check for shared API key from environment variable
		key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		if key == "" {
			return nil, fmt.Errorf("bridge has no default API key configured; use the personal API key flow instead")
		}
		return ol.finishLogin(ctx, key, "")
	}
	return ol.providerStep(), nil
}

func (ol *OpenAILogin) Cancel() {}

func (ol *OpenAILogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	if !ol.RequireUserInput {
		return nil, fmt.Errorf("user input not expected for this login flow")
	}

	// Step 0: Provider selection
	if ol.Step == 0 {
		provider := strings.TrimSpace(input["provider"])
		if provider == "" {
			return nil, fmt.Errorf("please select a provider")
		}
		// Validate provider
		switch provider {
		case ProviderBeeper, ProviderOpenAI, ProviderGemini, ProviderAnthropic, ProviderOpenRouter, ProviderCustom:
			// Valid
		default:
			return nil, fmt.Errorf("invalid provider: %s", provider)
		}
		ol.Provider = provider
		ol.Step = 1
		return ol.credentialsStep(), nil
	}

	// Step 1: Credentials
	key := strings.TrimSpace(input["api_key"])
	if key == "" {
		return nil, fmt.Errorf("please enter an API key")
	}

	// Determine base URL based on provider
	var baseURL string
	if ol.Provider == ProviderCustom {
		baseURL = strings.TrimSpace(input["base_url"])
		if baseURL == "" {
			return nil, fmt.Errorf("please enter a base URL")
		}
	} else {
		baseURL = providerBaseURLs[ol.Provider]
	}

	return ol.finishLogin(ctx, key, baseURL)
}

func (ol *OpenAILogin) providerStep() *bridgev2.LoginStep {
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       "io.ai-bridge.openai.select_provider",
		Instructions: "Select your AI provider",
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{
				{
					Type:    bridgev2.LoginInputFieldTypeSelect,
					ID:      "provider",
					Name:    "Provider",
					Options: []string{ProviderBeeper, ProviderOpenAI, ProviderGemini, ProviderAnthropic, ProviderOpenRouter, ProviderCustom},
				},
			},
		},
	}
}

func (ol *OpenAILogin) credentialsStep() *bridgev2.LoginStep {
	var fields []bridgev2.LoginInputDataField

	switch ol.Provider {
	case ProviderBeeper:
		fields = []bridgev2.LoginInputDataField{
			{
				Type:        bridgev2.LoginInputFieldTypeToken,
				ID:          "api_key",
				Name:        "Beeper Access Token",
				Description: "Your Beeper access token",
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
		RemoteName: "ChatGPT",
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

	// Trigger connection in background
	go client.Connect(ctx)

	return &bridgev2.LoginStep{
		Type:   bridgev2.LoginStepTypeComplete,
		StepID: "io.ai-bridge.openai.complete",
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLoginID: login.ID,
			UserLogin:   login,
		},
	}, nil
}
