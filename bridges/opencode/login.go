package opencode

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"

	openCodeAPI "github.com/beeper/ai-bridge/bridges/opencode/opencode"
	"github.com/beeper/ai-bridge/bridges/opencode/opencodebridge"
)

var (
	_ bridgev2.LoginProcess          = (*OpenCodeLogin)(nil)
	_ bridgev2.LoginProcessUserInput = (*OpenCodeLogin)(nil)
)

const (
	openCodeLoginStepCredentials = "io.ai-bridge.opencode.enter_credentials"
	defaultOpenCodeUsername      = "opencode"
)

type OpenCodeLogin struct {
	User      *bridgev2.User
	Connector *OpenCodeConnector
	FlowID    string
}

func (ol *OpenCodeLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	_ = ctx
	if ol.User == nil {
		return nil, errors.New("missing user context for login")
	}
	if ol.Connector == nil || ol.Connector.br == nil {
		return nil, errors.New("connector is not initialized")
	}
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       openCodeLoginStepCredentials,
		Instructions: "Enter your OpenCode server details.",
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{
				{
					Type:         bridgev2.LoginInputFieldTypeURL,
					ID:           "url",
					Name:         "Server URL",
					Description:  "OpenCode server URL, e.g. http://localhost:4096",
					DefaultValue: "http://localhost:4096",
				},
				{
					Type:         bridgev2.LoginInputFieldTypeUsername,
					ID:           "username",
					Name:         "Username",
					Description:  "Optional HTTP basic-auth username.",
					DefaultValue: defaultOpenCodeUsername,
				},
				{
					Type:        bridgev2.LoginInputFieldTypePassword,
					ID:          "password",
					Name:        "Password",
					Description: "Optional HTTP basic-auth password.",
				},
			},
		},
	}, nil
}

func (ol *OpenCodeLogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	if ol.User == nil {
		return nil, errors.New("missing user context for login")
	}
	if ol.Connector == nil || ol.Connector.br == nil {
		return nil, errors.New("connector is not initialized")
	}

	normalizedURL, err := openCodeAPI.NormalizeBaseURL(input["url"])
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	username := strings.TrimSpace(input["username"])
	if username == "" {
		username = defaultOpenCodeUsername
	}
	password := strings.TrimSpace(input["password"])
	instanceID := opencodebridge.OpenCodeInstanceID(normalizedURL, username)
	loginID := makeOpenCodeUserLoginID(ol.User.MXID, instanceID)
	remoteName := openCodeRemoteName(normalizedURL, username)

	instances := map[string]*opencodebridge.OpenCodeInstance{
		instanceID: {
			ID:       instanceID,
			URL:      normalizedURL,
			Username: username,
			Password: password,
		},
	}

	if existing, _ := ol.Connector.br.GetExistingUserLoginByID(ctx, loginID); existing != nil {
		existingMeta := loginMetadata(existing)
		existingMeta.Provider = ProviderOpenCode
		existingMeta.OpenCodeInstances = instances
		existing.Metadata = existingMeta
		existing.RemoteName = remoteName
		if err := existing.Save(ctx); err != nil {
			return nil, fmt.Errorf("failed to update existing login: %w", err)
		}
		if err := ol.Connector.LoadUserLogin(ctx, existing); err != nil {
			return nil, fmt.Errorf("failed to load client: %w", err)
		}
		go existing.Client.Connect(existing.Log.WithContext(ctx))
		return openCodeCompleteStep(existing), nil
	}

	login, err := ol.User.NewLogin(ctx, &database.UserLogin{
		ID:         loginID,
		RemoteName: remoteName,
		Metadata: &UserLoginMetadata{
			Provider:          ProviderOpenCode,
			OpenCodeInstances: instances,
		},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create login: %w", err)
	}
	if err := ol.Connector.LoadUserLogin(ctx, login); err != nil {
		return nil, fmt.Errorf("failed to load client: %w", err)
	}
	go login.Client.Connect(login.Log.WithContext(ctx))
	return openCodeCompleteStep(login), nil
}

func openCodeCompleteStep(login *bridgev2.UserLogin) *bridgev2.LoginStep {
	return &bridgev2.LoginStep{
		Type:   bridgev2.LoginStepTypeComplete,
		StepID: "io.ai-bridge.opencode.complete",
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLoginID: login.ID,
			UserLogin:   login,
		},
	}
}

func openCodeRemoteName(baseURL, username string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return "OpenCode"
	}
	if strings.EqualFold(strings.TrimSpace(username), defaultOpenCodeUsername) || strings.TrimSpace(username) == "" {
		return "OpenCode (" + parsed.Host + ")"
	}
	return fmt.Sprintf("OpenCode (%s@%s)", strings.TrimSpace(username), parsed.Host)
}

func (ol *OpenCodeLogin) Cancel() {}
