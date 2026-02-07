package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"

	"github.com/beeper/ai-bridge/pkg/codexrpc"
)

var (
	_ bridgev2.LoginProcess               = (*CodexLogin)(nil)
	_ bridgev2.LoginProcessUserInput      = (*CodexLogin)(nil)
	_ bridgev2.LoginProcessDisplayAndWait = (*CodexLogin)(nil)
)

// CodexLogin provisions a provider=codex user login backed by a local `codex app-server` process.
// Tokens are persisted by Codex itself under an isolated CODEX_HOME per login.
type CodexLogin struct {
	User      *bridgev2.User
	Connector *OpenAIConnector
	FlowID    string

	rpc        *codexrpc.Client
	codexHome  string
	instanceID string
	authMode   string
	loginID    string
	authURL    string
	waitUntil  time.Time

	loginDoneCh chan codexLoginDone
}

type codexLoginDone struct {
	success bool
	errText string
}

func (cl *CodexLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	cmd := cl.resolveCodexCommand()
	if _, err := exec.LookPath(cmd); err != nil {
		return &bridgev2.LoginStep{
			Type:         bridgev2.LoginStepTypeUserInput,
			StepID:       "io.ai-bridge.codex.install",
			Instructions: fmt.Sprintf("Codex CLI (%q) not found on PATH. Install Codex, then submit this step again.", cmd),
			UserInputParams: &bridgev2.LoginUserInputParams{
				Fields: []bridgev2.LoginInputDataField{
					{
						Type:         bridgev2.LoginInputFieldTypeURL,
						ID:           "install_url",
						Name:         "Install Codex",
						Description:  "Install Codex and retry. (Input is ignored; this field is just a reminder.)",
						DefaultValue: "https://github.com/openai/codex",
					},
				},
			},
		}, nil
	}

	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       "io.ai-bridge.codex.enter_credentials",
		Instructions: "Choose Codex auth mode (ChatGPT browser flow or API key).",
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{
				{
					Type:         bridgev2.LoginInputFieldTypeSelect,
					ID:           "auth_mode",
					Name:         "Auth mode",
					Description:  "ChatGPT uses a browser login flow; API key uses a pasted OpenAI API key.",
					DefaultValue: "chatgpt",
					Options:      []string{"chatgpt", "apiKey"},
				},
				{
					Type:        bridgev2.LoginInputFieldTypeToken,
					ID:          "api_key",
					Name:        "OpenAI API Key",
					Description: "Required only when auth_mode=apiKey.",
				},
			},
		},
	}, nil
}

func (cl *CodexLogin) Cancel() {
	if cl.rpc != nil {
		_ = cl.rpc.Close()
	}
}

func (cl *CodexLogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	log := zerolog.Ctx(ctx)
	if log == nil || log.GetLevel() == zerolog.Disabled {
		base := zerolog.Nop()
		if cl != nil && cl.User != nil {
			base = cl.User.Log
		}
		l := base.With().Str("component", "codex_login").Logger()
		log = &l
	}

	cmd := cl.resolveCodexCommand()
	if _, err := exec.LookPath(cmd); err != nil {
		return nil, fmt.Errorf("codex CLI not found (%q): %w", cmd, err)
	}
	mode := strings.TrimSpace(input["auth_mode"])
	if mode == "" {
		mode = "chatgpt"
	}
	if mode != "chatgpt" && mode != "apiKey" {
		return nil, fmt.Errorf("invalid auth_mode: %s", mode)
	}

	apiKey := strings.TrimSpace(input["api_key"])
	if mode == "apiKey" && apiKey == "" {
		return nil, fmt.Errorf("api_key is required for auth_mode=apiKey")
	}

	homeBase := cl.resolveCodexHomeBaseDir()
	instanceID := generateShortID()
	codexHome := filepath.Join(homeBase, instanceID)
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create CODEX_HOME: %w", err)
	}

	// IMPORTANT: Do not bind the Codex app-server process lifetime to the HTTP request context.
	// The provisioning API cancels r.Context() after the response is written; using it would kill
	// the child process and cause the login to hang forever in Wait().
	procCtx := context.Background()
	if cl.Connector != nil && cl.Connector.br != nil && cl.Connector.br.BackgroundCtx != nil {
		procCtx = cl.Connector.br.BackgroundCtx
	}
	rpc, err := codexrpc.StartProcess(procCtx, codexrpc.ProcessConfig{
		Command: cmd,
		Args:    []string{"app-server", "--listen", "stdio://"},
		Env:     []string{"CODEX_HOME=" + codexHome},
	})
	if err != nil {
		return nil, err
	}
	cl.rpc = rpc
	cl.codexHome = codexHome
	cl.instanceID = instanceID
	cl.authMode = mode

	initCtx, cancelInit := context.WithTimeout(ctx, 45*time.Second)
	defer cancelInit()
	_, err = rpc.Initialize(initCtx, codexrpc.ClientInfo{
		Name:    cl.Connector.Config.Codex.ClientInfo.Name,
		Title:   cl.Connector.Config.Codex.ClientInfo.Title,
		Version: cl.Connector.Config.Codex.ClientInfo.Version,
	}, false)
	if err != nil {
		log.Warn().Err(err).Msg("Codex initialize failed")
		_ = rpc.Close()
		cl.rpc = nil
		return nil, err
	}

	// Subscribe to account/login/completed so Wait() can resolve.
	cl.loginDoneCh = make(chan codexLoginDone, 1)
	rpc.OnNotification(func(method string, params json.RawMessage) {
		if method != "account/login/completed" {
			return
		}
		var evt struct {
			Success bool    `json:"success"`
			LoginID *string `json:"loginId"`
			Error   *string `json:"error"`
		}
		_ = json.Unmarshal(params, &evt)
		if cl.loginID != "" && (evt.LoginID == nil || strings.TrimSpace(*evt.LoginID) != cl.loginID) {
			return
		}
		errText := ""
		if evt.Error != nil {
			errText = strings.TrimSpace(*evt.Error)
		}
		select {
		case cl.loginDoneCh <- codexLoginDone{success: evt.Success, errText: errText}:
		default:
		}
	})

	if mode == "apiKey" {
		var loginResp struct {
			Type string `json:"type"`
		}
		loginCtx, cancelLogin := context.WithTimeout(ctx, 60*time.Second)
		defer cancelLogin()
		if err := rpc.Call(loginCtx, "account/login/start", map[string]any{"type": "apiKey", "apiKey": apiKey}, &loginResp); err != nil {
			log.Warn().Err(err).Msg("Codex apiKey login start failed")
			return nil, err
		}
		return cl.finishLogin(ctx)
	}

	var loginResp struct {
		Type    string `json:"type"`
		LoginID string `json:"loginId"`
		AuthURL string `json:"authUrl"`
	}
	loginCtx, cancelLogin := context.WithTimeout(ctx, 60*time.Second)
	defer cancelLogin()
	if err := rpc.Call(loginCtx, "account/login/start", map[string]any{"type": "chatgpt"}, &loginResp); err != nil {
		log.Warn().Err(err).Msg("Codex chatgpt login start failed")
		return nil, err
	}
	cl.loginID = strings.TrimSpace(loginResp.LoginID)
	authURL := strings.TrimSpace(loginResp.AuthURL)
	if authURL == "" || cl.loginID == "" {
		return nil, fmt.Errorf("codex returned empty authUrl/loginId")
	}
	cl.authURL = authURL
	cl.waitUntil = time.Now().Add(10 * time.Minute)

	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeDisplayAndWait,
		StepID:       "io.ai-bridge.codex.chatgpt",
		Instructions: "Open this URL in a browser and complete login, then wait here.",
		DisplayAndWaitParams: &bridgev2.LoginDisplayAndWaitParams{
			Type: bridgev2.LoginDisplayTypeCode,
			Data: authURL,
		},
	}, nil
}

func (cl *CodexLogin) Wait(ctx context.Context) (*bridgev2.LoginStep, error) {
	if cl.rpc == nil {
		return nil, fmt.Errorf("login not started")
	}
	if cl.loginDoneCh == nil {
		return nil, fmt.Errorf("login wait unavailable")
	}

	overallTimeout := time.Until(cl.waitUntil)
	if overallTimeout <= 0 {
		return nil, fmt.Errorf("timed out waiting for Codex login to complete")
	}
	deadline := time.NewTimer(overallTimeout)
	defer deadline.Stop()

	// Poll account/read as a fallback in case the notification is dropped.
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()

	// Avoid holding a single Wait() request open indefinitely; returning periodically
	// allows polling callers and prevents head-of-line blocking in single-threaded callers.
	returnAfter := time.NewTimer(25 * time.Second)
	defer returnAfter.Stop()

	for {
		select {
		case done := <-cl.loginDoneCh:
			if !done.success {
				if done.errText == "" {
					done.errText = "login failed"
				}
				return nil, fmt.Errorf("%s", done.errText)
			}
			return cl.finishLogin(ctx)
		case <-tick.C:
			if cl.rpc == nil {
				return nil, fmt.Errorf("codex login process stopped")
			}
			readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			var resp struct {
				Account *struct {
					Type  string `json:"type"`
					Email string `json:"email"`
				} `json:"account"`
			}
			err := cl.rpc.Call(readCtx, "account/read", map[string]any{"refreshToken": false}, &resp)
			cancel()
			if err == nil && resp.Account != nil && strings.TrimSpace(resp.Account.Type) != "" {
				return cl.finishLogin(ctx)
			}
		case <-returnAfter.C:
			return &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeDisplayAndWait,
				StepID:       "io.ai-bridge.codex.chatgpt",
				Instructions: "Still waiting for Codex login to complete. Keep this screen open after completing the browser login.",
				DisplayAndWaitParams: &bridgev2.LoginDisplayAndWaitParams{
					Type: bridgev2.LoginDisplayTypeCode,
					Data: strings.TrimSpace(cl.authURL),
				},
			}, nil
		case <-deadline.C:
			return nil, fmt.Errorf("timed out waiting for Codex login to complete")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (cl *CodexLogin) finishLogin(ctx context.Context) (*bridgev2.LoginStep, error) {
	if cl.User == nil {
		return nil, fmt.Errorf("missing user")
	}
	loginID := makeCodexUserLoginID(cl.User.MXID, cl.instanceID)
	remoteName := "Codex"
	if cl.User != nil {
		dupCount := 0
		for _, existing := range cl.User.GetUserLogins() {
			if existing == nil || existing.Metadata == nil {
				continue
			}
			meta, ok := existing.Metadata.(*UserLoginMetadata)
			if !ok || meta == nil {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(meta.Provider), ProviderCodex) && existing.ID != loginID {
				dupCount++
			}
		}
		if dupCount > 0 {
			remoteName = fmt.Sprintf("%s (%d)", remoteName, dupCount+1)
		}
	}

	// Best-effort read account email (chatgpt mode).
	accountEmail := ""
	if cl.rpc != nil {
		readCtx, cancelRead := context.WithTimeout(ctx, 10*time.Second)
		defer cancelRead()
		var acct struct {
			Account *struct {
				Type  string `json:"type"`
				Email string `json:"email"`
			} `json:"account"`
		}
		_ = cl.rpc.Call(readCtx, "account/read", map[string]any{"refreshToken": false}, &acct)
		if acct.Account != nil && strings.TrimSpace(acct.Account.Email) != "" {
			accountEmail = strings.TrimSpace(acct.Account.Email)
		}
	}

	meta := &UserLoginMetadata{
		Provider:          ProviderCodex,
		CodexHome:         cl.codexHome,
		CodexAuthMode:     cl.authMode,
		CodexAccountEmail: accountEmail,
	}
	login, err := cl.User.NewLogin(ctx, &database.UserLogin{
		ID:         loginID,
		RemoteName: remoteName,
		Metadata:   meta,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create login: %w", err)
	}
	if err := cl.Connector.LoadUserLogin(ctx, login); err != nil {
		return nil, fmt.Errorf("failed to load client: %w", err)
	}
	go login.Client.Connect(login.Log.WithContext(context.Background()))

	if cl.rpc != nil {
		_ = cl.rpc.Close()
		cl.rpc = nil
	}

	return &bridgev2.LoginStep{
		Type:   bridgev2.LoginStepTypeComplete,
		StepID: "io.ai-bridge.codex.complete",
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLoginID: login.ID,
			UserLogin:   login,
		},
	}, nil
}

func (cl *CodexLogin) resolveCodexCommand() string {
	if cl.Connector != nil && cl.Connector.Config.Codex != nil {
		if cmd := strings.TrimSpace(cl.Connector.Config.Codex.Command); cmd != "" {
			return cmd
		}
	}
	return "codex"
}

func (cl *CodexLogin) resolveCodexHomeBaseDir() string {
	base := ""
	if cl.Connector != nil && cl.Connector.Config.Codex != nil {
		base = strings.TrimSpace(cl.Connector.Config.Codex.HomeBaseDir)
	}
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
			base = filepath.Join(home, ".local", "share", "ai-bridge", "codex")
		} else {
			base = filepath.Join(os.TempDir(), "ai-bridge-codex")
		}
	}
	if strings.HasPrefix(base, "~"+string(os.PathSeparator)) {
		if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
			base = filepath.Join(home, strings.TrimPrefix(base, "~"+string(os.PathSeparator)))
		}
	}
	abs, err := filepath.Abs(base)
	if err == nil {
		return abs
	}
	return base
}
