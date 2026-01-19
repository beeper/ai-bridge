package connector

// BeeperModelList is the SINGLE SOURCE OF TRUTH for which models are available in Beeper mode.
// Only model IDs and optional display name overrides are defined here.
// ALL capabilities are fetched from the OpenRouter API via generate-models.sh.
//
// To add/remove models:
// 1. Add/remove the model ID in this map
// 2. Run: ./generate-models.sh --openrouter-token=YOUR_TOKEN
// 3. Commit both this file and the generated beeper_models_generated.go
//
// Map format: model_id -> display_name (empty string = use API name)
var BeeperModelList = map[string]string{
	// MiniMax
	"minimax/minimax-m2.1": "MiniMax M2.1",
	"minimax/minimax-m2":   "MiniMax M2",

	// GLM (Z.AI) - supports reasoning via parameter
	"z-ai/glm-4.7":  "GLM 4.7",
	"z-ai/glm-4.6v": "GLM 4.6V",

	// Kimi (Moonshot)
	"moonshotai/kimi-k2-0905":     "Kimi K2 (0905)",
	"moonshotai/kimi-k2-thinking": "Kimi K2 (Thinking)",

	// Qwen
	"qwen/qwen3-235b-a22b-thinking-2507": "Qwen 3 235B (Thinking)",
	"qwen/qwen3-235b-a22b":               "Qwen 3 235B",

	// Grok (xAI)
	"x-ai/grok-4.1-fast": "Grok 4.1 Fast",

	// DeepSeek
	"deepseek/deepseek-v3.2": "DeepSeek v3.2",

	// Llama (Meta)
	"meta-llama/llama-4-scout":    "Llama 4 Scout",
	"meta-llama/llama-4-maverick": "Llama 4 Maverick",

	// Gemini (Google) via OpenRouter
	"google/gemini-2.5-flash-image":     "Nano Banana",
	"google/gemini-3-flash-preview":     "Gemini 3 Flash",
	"google/gemini-3-pro-image-preview": "Nano Banana Pro",
	"google/gemini-3-pro-preview":       "Gemini 3 Pro",

	// Claude (Anthropic) via OpenRouter
	"anthropic/claude-sonnet-4.5": "Claude Sonnet 4.5",
	"anthropic/claude-opus-4.5":   "Claude Opus 4.5",
	"anthropic/claude-haiku-4.5":  "Claude Haiku 4.5",

	// GPT (OpenAI) via OpenRouter
	"openai/gpt-5-image":  "GPT ImageGen 1.5",
	"openai/gpt-5.2":      "GPT-5.2",
	"openai/gpt-5-mini":   "GPT-5 mini",
	"openai/gpt-oss-20b":  "GPT OSS 20B",
	"openai/gpt-oss-120b": "GPT OSS 120B",
}

// GetBeeperModelIDs returns all model IDs configured for Beeper mode.
func GetBeeperModelIDs() []string {
	ids := make([]string, 0, len(BeeperModelList))
	for id := range BeeperModelList {
		ids = append(ids, id)
	}
	return ids
}

// GetBeeperModelDisplayName returns the display name for a model ID.
// If a custom display name is configured, it's returned.
// Otherwise, returns empty string (caller should use API name).
func GetBeeperModelDisplayName(modelID string) string {
	return BeeperModelList[modelID]
}

// IsBeeperModel checks if a model ID is in the Beeper model list.
func IsBeeperModel(modelID string) bool {
	_, ok := BeeperModelList[modelID]
	return ok
}
