package connector

import (
	"context"
	"fmt"
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
		key := strings.TrimSpace(ol.Connector.Config.OpenAI.APIKey)
		if key == "" {
			return nil, fmt.Errorf("bridge has no default API key configured; use the personal API key flow instead")
		}
		return ol.finishLogin(ctx, key)
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
	return ol.finishLogin(ctx, key)
}

func (ol *OpenAILogin) apiKeyStep() *bridgev2.LoginStep {
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       "io.beep.openai.enter_api_key",
		Instructions: "Enter the OpenAI API key you want this bridge login to use.",
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{{
				Type:        bridgev2.LoginInputFieldTypeToken,
				ID:          "api_key",
				Name:        "OpenAI API key",
				Description: "Generate one at https://platform.openai.com/account/api-keys",
			}},
		},
	}
}

func (ol *OpenAILogin) finishLogin(ctx context.Context, apiKey string) (*bridgev2.LoginStep, error) {
	loginID := makeUserLoginID(ol.User.MXID)
	meta := &UserLoginMetadata{
		APIKey: apiKey,
	}
	login, err := ol.User.NewLogin(ctx, &database.UserLogin{
		ID:         loginID,
		RemoteName: "ChatGPT",
		Metadata:   meta,
	}, nil)
	if err != nil {
		return nil, err
	}
	return &bridgev2.LoginStep{
		Type:   bridgev2.LoginStepTypeComplete,
		StepID: "io.beep.openai.complete",
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLoginID: login.ID,
			UserLogin:   login,
		},
	}, nil
}
