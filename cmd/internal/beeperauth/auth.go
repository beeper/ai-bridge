package beeperauth

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/beeper/bridge-manager/api/beeperapi"
	"maunium.net/go/mautrix"
)

var envDomains = map[string]string{
	"prod":    "beeper.com",
	"staging": "beeper-staging.com",
	"dev":     "beeper-dev.com",
	"local":   "beeper.localtest.me",
}

type Config struct {
	Env      string `json:"env"`
	Domain   string `json:"domain"`
	Username string `json:"username"`
	Token    string `json:"token"`
}

type Store struct {
	Path         string
	MissingError func() error
}

type LoginParams struct {
	Env               string
	Email             string
	Code              string
	DeviceDisplayName string
	Prompt            func(string) (string, error)
}

func DomainForEnv(env string) (string, error) {
	domain, ok := envDomains[env]
	if !ok {
		return "", fmt.Errorf("invalid env %q", env)
	}
	return domain, nil
}

func EnvNames() []string {
	return slices.Collect(maps.Keys(envDomains))
}

func Login(ctx context.Context, params LoginParams) (Config, error) {
	domain, err := DomainForEnv(params.Env)
	if err != nil {
		return Config{}, err
	}
	email := strings.TrimSpace(params.Email)
	if email == "" {
		if params.Prompt == nil {
			return Config{}, fmt.Errorf("email is required")
		}
		email, err = params.Prompt("Email: ")
		if err != nil {
			return Config{}, err
		}
		email = strings.TrimSpace(email)
	}
	if email == "" {
		return Config{}, fmt.Errorf("email is required")
	}

	start, err := beeperapi.StartLogin(domain)
	if err != nil {
		return Config{}, err
	}
	if err = beeperapi.SendLoginEmail(domain, start.RequestID, email); err != nil {
		return Config{}, err
	}

	code := strings.TrimSpace(params.Code)
	if code == "" {
		if params.Prompt == nil {
			return Config{}, fmt.Errorf("code is required")
		}
		code, err = params.Prompt("Code: ")
		if err != nil {
			return Config{}, err
		}
		code = strings.TrimSpace(code)
	}
	if code == "" {
		return Config{}, fmt.Errorf("code is required")
	}

	resp, err := beeperapi.SendLoginCode(domain, start.RequestID, code)
	if err != nil {
		return Config{}, err
	}
	matrixClient, err := mautrix.NewClient(fmt.Sprintf("https://matrix.%s", domain), "", "")
	if err != nil {
		return Config{}, fmt.Errorf("failed to create matrix client: %w", err)
	}
	loginCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	loginResp, err := matrixClient.Login(loginCtx, &mautrix.ReqLogin{
		Type:                     "org.matrix.login.jwt",
		Token:                    resp.LoginToken,
		InitialDeviceDisplayName: params.DeviceDisplayName,
	})
	if err != nil {
		return Config{}, fmt.Errorf("matrix login failed: %w", err)
	}
	username := ""
	if resp.Whoami != nil {
		username = strings.TrimSpace(resp.Whoami.UserInfo.Username)
	}
	if username == "" {
		username = loginResp.UserID.Localpart()
	}
	return Config{
		Env:      params.Env,
		Domain:   domain,
		Username: username,
		Token:    loginResp.AccessToken,
	}, nil
}

func ResolveFromEnvOrStore(store Store) (Config, error) {
	if tok := os.Getenv("BEEPER_ACCESS_TOKEN"); tok != "" {
		env := os.Getenv("BEEPER_ENV")
		if env == "" {
			env = "prod"
		}
		domain, err := DomainForEnv(env)
		if err != nil {
			return Config{}, fmt.Errorf("invalid BEEPER_ENV %q", env)
		}
		return Config{
			Env:      env,
			Domain:   domain,
			Username: os.Getenv("BEEPER_USERNAME"),
			Token:    tok,
		}, nil
	}
	return Load(store)
}

func Load(store Store) (Config, error) {
	data, err := os.ReadFile(store.Path)
	if err != nil {
		if store.MissingError != nil {
			return Config{}, store.MissingError()
		}
		return Config{}, err
	}
	var cfg Config
	if err = json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Token == "" || cfg.Domain == "" {
		return Config{}, fmt.Errorf("invalid auth config at %s", store.Path)
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if cfg.Domain == "" && cfg.Env != "" {
		domain, err := DomainForEnv(cfg.Env)
		if err != nil {
			return err
		}
		cfg.Domain = domain
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
