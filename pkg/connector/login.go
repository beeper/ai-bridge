package connector

import (
	"context"
	"fmt"
	"os"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
)

var (
	_ bridgev2.LoginProcess          = (*OpenAILogin)(nil)
	_ bridgev2.LoginProcessUserInput = (*OpenAILogin)(nil)
)

// OpenAILogin maps a Matrix user to a synthetic OpenAI "login".
type OpenAILogin struct {
	User             *bridgev2.User
	Connector        *OpenAIConnector
	RequireUserInput bool
}

func (ol *OpenAILogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	if !ol.RequireUserInput {
		// Check for shared API key from environment variable
		key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		if key == "" {
			return nil, fmt.Errorf("bridge has no default API key configured; use the personal API key flow instead")
		}
		return ol.finishLogin(ctx, key, "", "", "")
	}
	return ol.apiKeyStep(), nil
}

func (ol *OpenAILogin) Cancel() {}

func (ol *OpenAILogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	if !ol.RequireUserInput {
		return nil, fmt.Errorf("user input not expected for this login flow")
	}
	key := strings.TrimSpace(input["api_key"])
	if key == "" {
		return nil, fmt.Errorf("please enter an OpenAI API key")
	}
	baseURL := strings.TrimSpace(input["base_url"])
	orgID := strings.TrimSpace(input["org_id"])
	projectID := strings.TrimSpace(input["project_id"])
	return ol.finishLogin(ctx, key, baseURL, orgID, projectID)
}

func (ol *OpenAILogin) apiKeyStep() *bridgev2.LoginStep {
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       "io.ai-bridge.openai.enter_credentials",
		Instructions: "Enter your OpenAI API key and optional configuration.",
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{
				{
					Type:        bridgev2.LoginInputFieldTypeToken,
					ID:          "api_key",
					Name:        "OpenAI API key",
					Description: "Generate one at https://platform.openai.com/account/api-keys",
				},
				{
					Type:        bridgev2.LoginInputFieldTypeToken,
					ID:          "base_url",
					Name:        "Base URL (optional)",
					Description: "Leave blank for https://api.openai.com/v1 (also supports Azure, proxies)",
				},
				{
					Type:        bridgev2.LoginInputFieldTypeToken,
					ID:          "org_id",
					Name:        "Organization ID (optional)",
					Description: "For org-scoped API keys",
				},
				{
					Type:        bridgev2.LoginInputFieldTypeToken,
					ID:          "project_id",
					Name:        "Project ID (optional)",
					Description: "For project-scoped API keys",
				},
			},
		},
	}
}

func (ol *OpenAILogin) finishLogin(ctx context.Context, apiKey, baseURL, orgID, projectID string) (*bridgev2.LoginStep, error) {
	loginID := makeUserLoginID(ol.User.MXID)
	meta := &UserLoginMetadata{
		APIKey:    apiKey,
		BaseURL:   baseURL,
		OrgID:     orgID,
		ProjectID: projectID,
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
