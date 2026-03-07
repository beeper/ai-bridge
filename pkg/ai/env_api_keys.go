package ai

import (
	"os"
	"path/filepath"
)

func GetEnvAPIKey(provider string) string {
	switch provider {
	case "github-copilot":
		if v := os.Getenv("COPILOT_GITHUB_TOKEN"); v != "" {
			return v
		}
		if v := os.Getenv("GH_TOKEN"); v != "" {
			return v
		}
		return os.Getenv("GITHUB_TOKEN")
	case "anthropic":
		if v := os.Getenv("ANTHROPIC_OAUTH_TOKEN"); v != "" {
			return v
		}
		return os.Getenv("ANTHROPIC_API_KEY")
	case "openai":
		return os.Getenv("OPENAI_API_KEY")
	case "azure-openai-responses":
		return os.Getenv("AZURE_OPENAI_API_KEY")
	case "google-vertex":
		if hasVertexADCCredentials() && hasVertexProject() && os.Getenv("GOOGLE_CLOUD_LOCATION") != "" {
			return "<authenticated>"
		}
		return ""
	case "amazon-bedrock":
		if os.Getenv("AWS_PROFILE") != "" ||
			(os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "") ||
			os.Getenv("AWS_BEARER_TOKEN_BEDROCK") != "" ||
			os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI") != "" ||
			os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI") != "" ||
			os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") != "" {
			return "<authenticated>"
		}
		return ""
	case "google":
		return os.Getenv("GEMINI_API_KEY")
	case "groq":
		return os.Getenv("GROQ_API_KEY")
	case "cerebras":
		return os.Getenv("CEREBRAS_API_KEY")
	case "xai":
		return os.Getenv("XAI_API_KEY")
	case "openrouter":
		return os.Getenv("OPENROUTER_API_KEY")
	case "vercel-ai-gateway":
		return os.Getenv("AI_GATEWAY_API_KEY")
	case "zai":
		return os.Getenv("ZAI_API_KEY")
	case "mistral":
		return os.Getenv("MISTRAL_API_KEY")
	case "minimax":
		return os.Getenv("MINIMAX_API_KEY")
	case "minimax-cn":
		return os.Getenv("MINIMAX_CN_API_KEY")
	case "huggingface":
		return os.Getenv("HF_TOKEN")
	case "opencode", "opencode-go":
		return os.Getenv("OPENCODE_API_KEY")
	case "kimi-coding":
		return os.Getenv("KIMI_API_KEY")
	default:
		return ""
	}
}

func hasVertexProject() bool {
	return os.Getenv("GOOGLE_CLOUD_PROJECT") != "" || os.Getenv("GCLOUD_PROJECT") != ""
}

func hasVertexADCCredentials() bool {
	if path := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	adcPath := filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
	if _, err := os.Stat(adcPath); err == nil {
		return true
	}
	return false
}
