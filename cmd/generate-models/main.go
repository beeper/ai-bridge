package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"sort"
	"strings"
	"time"
)

// OpenRouterArchitecture contains model architecture information
type OpenRouterArchitecture struct {
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
	Modality         string   `json:"modality"` // Legacy field
	Tokenizer        string   `json:"tokenizer"`
	InstructType     string   `json:"instruct_type"`
}

// OpenRouterPricing contains model pricing information
type OpenRouterPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
	WebSearch  string `json:"web_search"`
}

// OpenRouterTopProvider contains top provider information
type OpenRouterTopProvider struct {
	MaxCompletionTokens int  `json:"max_completion_tokens"`
	IsModerated         bool `json:"is_moderated"`
}

// OpenRouterModel represents a model from OpenRouter API with full capability fields
type OpenRouterModel struct {
	ID                  string                 `json:"id"`
	Name                string                 `json:"name"`
	Description         string                 `json:"description"`
	ContextLength       int                    `json:"context_length"`
	Architecture        OpenRouterArchitecture `json:"architecture"`
	Pricing             OpenRouterPricing      `json:"pricing"`
	TopProvider         OpenRouterTopProvider  `json:"top_provider"`
	SupportedParameters []string               `json:"supported_parameters"`
}

// OpenRouterResponse represents the API response
type OpenRouterResponse struct {
	Data []OpenRouterModel `json:"data"`
}

// ModelCapabilities holds auto-detected capabilities for a model
type ModelCapabilities struct {
	Vision          bool
	ToolCalling     bool
	Reasoning       bool
	WebSearch       bool
	ImageGen        bool
	ContextWindow   int
	MaxOutputTokens int
}

// BeeperModelConfig defines which models to include for Beeper
// Simple map: model ID -> display name
// All capabilities are auto-detected from OpenRouter API
var BeeperModelConfig = map[string]string{
	// MiniMax
	"minimax/minimax-m2.1": "MiniMax M2.1",
	"minimax/minimax-m2":   "MiniMax M2",
	// GLM (Z.AI)
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

func main() {
	token := flag.String("openrouter-token", "", "OpenRouter API token")
	outputFile := flag.String("output", "pkg/connector/beeper_models_generated.go", "Output Go file")
	jsonFile := flag.String("json", "pkg/connector/beeper_models.json", "Output JSON file for clients")
	flag.Parse()

	if *token == "" {
		fmt.Fprintln(os.Stderr, "Error: --openrouter-token is required")
		os.Exit(1)
	}

	// Fetch models from OpenRouter
	models, err := fetchOpenRouterModels(*token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching models: %v\n", err)
		os.Exit(1)
	}

	// Generate Go file
	if err := generateGoFile(models, *outputFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Generated %s with %d models\n", *outputFile, len(BeeperModelConfig))

	// Generate JSON file for clients
	if err := generateJSONFile(models, *jsonFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating JSON file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Generated %s\n", *jsonFile)
}

func fetchOpenRouterModels(token string) (map[string]OpenRouterModel, error) {
	req, err := http.NewRequest(http.MethodGet, "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var apiResp OpenRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}

	// Index by ID
	result := make(map[string]OpenRouterModel)
	for _, model := range apiResp.Data {
		result[model.ID] = model
	}

	return result, nil
}

// detectCapabilities auto-detects all model capabilities from OpenRouter API response
func detectCapabilities(modelID string, apiModel OpenRouterModel, hasAPIData bool) ModelCapabilities {
	if !hasAPIData {
		// Fallback: can't detect capabilities without API data
		fmt.Fprintf(os.Stderr, "Warning: No API data for model %s, using defaults\n", modelID)
		return ModelCapabilities{ToolCalling: true}
	}

	caps := ModelCapabilities{}

	// Vision: "image" in architecture.input_modalities
	caps.Vision = slices.Contains(apiModel.Architecture.InputModalities, "image")
	// Legacy fallback: check modality field
	if !caps.Vision && strings.Contains(apiModel.Architecture.Modality, "image") {
		caps.Vision = true
	}

	// Image Generation: "image" in architecture.output_modalities
	caps.ImageGen = slices.Contains(apiModel.Architecture.OutputModalities, "image")

	// Tool Calling: "tools" in supported_parameters
	caps.ToolCalling = slices.Contains(apiModel.SupportedParameters, "tools")

	// Reasoning: "reasoning" in supported_parameters
	caps.Reasoning = slices.Contains(apiModel.SupportedParameters, "reasoning")

	// Web Search: pricing.web_search != "" OR "web_search_options" in supported_parameters
	caps.WebSearch = apiModel.Pricing.WebSearch != "" ||
		slices.Contains(apiModel.SupportedParameters, "web_search_options")

	// Context window and max output tokens from API
	caps.ContextWindow = apiModel.ContextLength
	caps.MaxOutputTokens = apiModel.TopProvider.MaxCompletionTokens

	return caps
}

// availableToolsGo returns the Go code representation of available tools
func availableToolsGo(caps ModelCapabilities) string {
	if caps.WebSearch && caps.ToolCalling {
		return "[]string{ToolWebSearch, ToolFunctionCalling}"
	} else if caps.ToolCalling {
		return "[]string{ToolFunctionCalling}"
	} else if caps.WebSearch {
		return "[]string{ToolWebSearch}"
	}
	return "[]string{}"
}

// availableToolsJSON returns the JSON representation of available tools
func availableToolsJSON(caps ModelCapabilities) []string {
	var tools []string
	if caps.WebSearch {
		tools = append(tools, "web_search")
	}
	if caps.ToolCalling {
		tools = append(tools, "function_calling")
	}
	return tools
}

func generateGoFile(apiModels map[string]OpenRouterModel, outputPath string) error {
	var buf strings.Builder

	buf.WriteString(`// Code generated by generate-models. DO NOT EDIT.
// Generated at: ` + time.Now().UTC().Format(time.RFC3339) + `

package connector

// BeeperModelsGenerated contains model definitions fetched from OpenRouter API.
// The model list is manually curated in cmd/generate-models/main.go.
// All capabilities are auto-detected from the OpenRouter API.
var BeeperModelsGenerated = []ModelInfo{
`)

	// Get sorted model IDs for deterministic output
	modelIDs := sortedKeys(BeeperModelConfig)

	for _, modelID := range modelIDs {
		displayName := BeeperModelConfig[modelID]
		apiModel, hasAPIData := apiModels[modelID]
		caps := detectCapabilities(modelID, apiModel, hasAPIData)

		buf.WriteString(fmt.Sprintf(`	{
		ID:                  %q,
		Name:                %q,
		Provider:            "openrouter",
		SupportsVision:      %t,
		SupportsToolCalling: %t,
		SupportsReasoning:   %t,
		SupportsWebSearch:   %t,
		SupportsImageGen:    %t,
		ContextWindow:       %d,
		MaxOutputTokens:     %d,
		AvailableTools:      %s,
	},
`,
			modelID,
			displayName,
			caps.Vision,
			caps.ToolCalling,
			caps.Reasoning,
			caps.WebSearch,
			caps.ImageGen,
			caps.ContextWindow,
			caps.MaxOutputTokens,
			availableToolsGo(caps),
		))
	}

	buf.WriteString(`}

// GetBeeperModelsGenerated returns the auto-generated Beeper models list.
func GetBeeperModelsGenerated() []ModelInfo {
	result := make([]ModelInfo, len(BeeperModelsGenerated))
	copy(result, BeeperModelsGenerated)
	return result
}
`)

	return os.WriteFile(outputPath, []byte(buf.String()), 0644)
}

// JSONModelInfo mirrors the connector.ModelInfo struct for JSON output
type JSONModelInfo struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Provider            string   `json:"provider"`
	SupportsVision      bool     `json:"supports_vision"`
	SupportsToolCalling bool     `json:"supports_tool_calling"`
	SupportsReasoning   bool     `json:"supports_reasoning"`
	SupportsWebSearch   bool     `json:"supports_web_search"`
	SupportsImageGen    bool     `json:"supports_image_gen,omitempty"`
	ContextWindow       int      `json:"context_window,omitempty"`
	MaxOutputTokens     int      `json:"max_output_tokens,omitempty"`
	AvailableTools      []string `json:"available_tools,omitempty"`
}

func generateJSONFile(apiModels map[string]OpenRouterModel, outputPath string) error {
	var models []JSONModelInfo

	modelIDs := sortedKeys(BeeperModelConfig)

	for _, modelID := range modelIDs {
		displayName := BeeperModelConfig[modelID]
		apiModel, hasAPIData := apiModels[modelID]
		caps := detectCapabilities(modelID, apiModel, hasAPIData)

		models = append(models, JSONModelInfo{
			ID:                  modelID,
			Name:                displayName,
			Provider:            "openrouter",
			SupportsVision:      caps.Vision,
			SupportsToolCalling: caps.ToolCalling,
			SupportsReasoning:   caps.Reasoning,
			SupportsWebSearch:   caps.WebSearch,
			SupportsImageGen:    caps.ImageGen,
			ContextWindow:       caps.ContextWindow,
			MaxOutputTokens:     caps.MaxOutputTokens,
			AvailableTools:      availableToolsJSON(caps),
		})
	}

	data, err := json.MarshalIndent(models, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, data, 0644)
}

// sortedKeys returns the keys of a map sorted alphabetically
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
