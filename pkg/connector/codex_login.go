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

	startCh chan error
}

type codexLoginDone struct {
	success bool
	errText string
}

func (cl *CodexLogin) logger(ctx context.Context) *zerolog.Logger {
	if ctx != nil {
		if ctxLog := zerolog.Ctx(ctx); ctxLog != nil && ctxLog.GetLevel() != zerolog.Disabled {
			return ctxLog
		}
	}
	if cl != nil && cl.User != nil {
		l := cl.User.Log.With().Str("component", "codex_login").Logger()
		return &l
	}
	l := zerolog.Nop()
	return &l
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
	log := cl.logger(ctx)

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

	// Keep SubmitUserInput responsive: callers may have ~30-40s request timeouts.
	initCtx, cancelInit := context.WithTimeout(ctx, 20*time.Second)
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
		switch method {
		case "account/login/completed":
			var evt struct {
				Success bool    `json:"success"`
				LoginID *string `json:"loginId"`
				Error   *string `json:"error"`
			}
			_ = json.Unmarshal(params, &evt)
			// Some Codex builds omit loginId; only filter when it's present.
			if cl.loginID != "" && evt.LoginID != nil && strings.TrimSpace(*evt.LoginID) != cl.loginID {
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
		case "account/updated":
			// Some Codex builds only emit account/updated after login.
			var evt struct {
				AuthMode *string `json:"authMode"`
			}
			_ = json.Unmarshal(params, &evt)
			if evt.AuthMode != nil && strings.TrimSpace(*evt.AuthMode) != "" {
				select {
				case cl.loginDoneCh <- codexLoginDone{success: true}:
				default:
				}
			}
		}
	})

	if mode == "apiKey" {
		// Start apiKey login in the background and let Wait() poll/resolve. This avoids
		// blocking the provisioning request (which can freeze clients).
		cl.waitUntil = time.Now().Add(5 * time.Minute)
		cl.apiKeyStartCh = make(chan error, 1)
		go func() {
			var loginResp struct {
				Type string `json:"type"`
			}
			startCtx, cancel := context.WithTimeout(cl.persistContext(ctx), 20*time.Second)
			err := rpc.Call(startCtx, "account/login/start", map[string]any{"type": "apiKey", "apiKey": apiKey}, &loginResp)
			cancel()
			if err != nil {
				log.Warn().Err(err).Msg("Codex apiKey login start failed")
			}
			select {
			case cl.apiKeyStartCh <- err:
			default:
			}
		}()

		return &bridgev2.LoginStep{
			Type:         bridgev2.LoginStepTypeDisplayAndWait,
			StepID:       "io.ai-bridge.codex.validating",
			Instructions: "Validating the API key with Codex. Keep this screen open.",
			DisplayAndWaitParams: &bridgev2.LoginDisplayAndWaitParams{
				Type: bridgev2.LoginDisplayTypeNothing,
			},
		}, nil
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
	log.Info().Str("instance_id", cl.instanceID).Str("login_id", cl.loginID).Msg("Codex browser login started")

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
	log := cl.logger(ctx)
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
	returnAfter := time.NewTimer(20 * time.Second)
	defer returnAfter.Stop()

	for {
		select {
		case err := <-cl.apiKeyStartCh:
			// apiKey mode: surface start failure early instead of waiting until timeout.
			if err != nil {
				return nil, err
			}
			// Prevent repeated handling if the channel is re-used.
			cl.apiKeyStartCh = nil
		case done := <-cl.loginDoneCh:
			if !done.success {
				if done.errText == "" {
					done.errText = "login failed"
				}
				log.Warn().Str("login_id", cl.loginID).Str("error", done.errText).Msg("Codex login failed")
				return nil, fmt.Errorf("%s", done.errText)
			}
			log.Info().Str("login_id", cl.loginID).Msg("Codex login completed (notification)")
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
				RequiresOpenaiAuth bool `json:"requiresOpenaiAuth"`
			}
			// Try a few variants for compatibility with different Codex versions.
			// Use refreshToken=true during login to force Codex to re-check auth state when supported.
			err := cl.rpc.Call(readCtx, "account/read", map[string]any{"refreshToken": true}, &resp)
			if err != nil {
				err = cl.rpc.Call(readCtx, "account/read", map[string]any{"refreshToken": false}, &resp)
			}
			if err != nil {
				err = cl.rpc.Call(readCtx, "account/read", nil, &resp)
			}
			cancel()
			if err == nil && (resp.Account != nil || !resp.RequiresOpenaiAuth) {
				log.Info().Str("login_id", cl.loginID).Msg("Codex login completed (account/read)")
				return cl.finishLogin(cl.persistContext(ctx))
			}
		case <-returnAfter.C:
			log.Debug().Str("login_id", cl.loginID).Msg("Codex login still waiting")
			stepID := "io.ai-bridge.codex.chatgpt"
			instr := "Still waiting for Codex login to complete. Keep this screen open."
			displayType := bridgev2.LoginDisplayTypeCode
			data := strings.TrimSpace(cl.authURL)
			if cl.authMode == "apiKey" {
				stepID = "io.ai-bridge.codex.validating"
				instr = "Still validating the API key with Codex. Keep this screen open."
				displayType = bridgev2.LoginDisplayTypeNothing
				data = ""
			}
			return &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeDisplayAndWait,
				StepID:       stepID,
				Instructions: instr,
				DisplayAndWaitParams: &bridgev2.LoginDisplayAndWaitParams{
					Type: displayType,
					Data: data,
				},
			}, nil
		case <-deadline.C:
			log.Warn().Str("login_id", cl.loginID).Msg("Codex login timed out")
			return nil, fmt.Errorf("timed out waiting for Codex login to complete")
		case <-ctx.Done():
			// Most callers will have their own HTTP/gRPC deadlines. Returning the same waiting
			// step allows the client to poll again without the login process being marked as failed.
			log.Debug().Str("login_id", cl.loginID).Msg("Codex login wait context ended; returning still-waiting step")
			stepID := "io.ai-bridge.codex.chatgpt"
			instr := "Still waiting for Codex login to complete. Keep this screen open after completing the browser login."
			displayType := bridgev2.LoginDisplayTypeCode
			data := strings.TrimSpace(cl.authURL)
			if cl.authMode == "apiKey" {
				stepID = "io.ai-bridge.codex.validating"
				instr = "Still validating the API key with Codex. Keep this screen open."
				displayType = bridgev2.LoginDisplayTypeNothing
				data = ""
			}
			return &bridgev2.LoginStep{
				Type:         bridgev2.LoginStepTypeDisplayAndWait,
				StepID:       stepID,
				Instructions: instr,
				DisplayAndWaitParams: &bridgev2.LoginDisplayAndWaitParams{
					Type: displayType,
					Data: data,
				},
			}, nil
		}
	}
}

func (cl *CodexLogin) persistContext(ctx context.Context) context.Context {
	if cl != nil && cl.Connector != nil && cl.Connector.br != nil && cl.Connector.br.BackgroundCtx != nil {
		return cl.Connector.br.BackgroundCtx
	}
	_ = ctx
	return context.Background()
}

func (cl *CodexLogin) finishLogin(ctx context.Context) (*bridgev2.LoginStep, error) {
	if cl.User == nil {
		return nil, fmt.Errorf("missing user")
	}
	persistCtx := cl.persistContext(ctx)
	log := cl.logger(persistCtx)

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
		readCtx, cancelRead := context.WithTimeout(persistCtx, 10*time.Second)
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

	if cl.Connector != nil && cl.Connector.br != nil {
		if existing, _ := cl.Connector.br.GetExistingUserLoginByID(persistCtx, loginID); existing != nil {
			existingMeta, ok := existing.Metadata.(*UserLoginMetadata)
			if !ok || existingMeta == nil {
				existingMeta = &UserLoginMetadata{}
			}
			*existingMeta = *meta
			existing.Metadata = existingMeta
			existing.RemoteName = remoteName
			if err := existing.Save(persistCtx); err != nil {
				return nil, fmt.Errorf("failed to update existing login: %w", err)
			}
			log.Info().Str("user_login_id", string(existing.ID)).Msg("Updated existing Codex login")
			if err := cl.Connector.LoadUserLogin(persistCtx, existing); err != nil {
				return nil, fmt.Errorf("failed to load client: %w", err)
			}
			go existing.Client.Connect(existing.Log.WithContext(cl.persistContext(ctx)))
			if cl.rpc != nil {
				_ = cl.rpc.Close()
				cl.rpc = nil
			}
			return &bridgev2.LoginStep{
				Type:   bridgev2.LoginStepTypeComplete,
				StepID: "io.ai-bridge.codex.complete",
				CompleteParams: &bridgev2.LoginCompleteParams{
					UserLoginID: existing.ID,
					UserLogin:   existing,
				},
			}, nil
		}
	}

	login, err := cl.User.NewLogin(persistCtx, &database.UserLogin{
		ID:         loginID,
		RemoteName: remoteName,
		Metadata:   meta,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create login: %w", err)
	}
	log.Info().Str("user_login_id", string(login.ID)).Msg("Created new Codex login")
	if err := cl.Connector.LoadUserLogin(persistCtx, login); err != nil {
		return nil, fmt.Errorf("failed to load client: %w", err)
	}
	go login.Client.Connect(login.Log.WithContext(cl.persistContext(ctx)))

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
