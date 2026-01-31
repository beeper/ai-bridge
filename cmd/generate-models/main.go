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

// modelConfig is the SINGLE SOURCE OF TRUTH for which models are available.
// Only model IDs and optional display name overrides are defined here.
// ALL capabilities are fetched from the OpenRouter API.
//
// Map format: model_id -> display_name (empty string = use API name)
var modelConfig = struct {
	// Models to fetch from OpenRouter - ID -> display name override (empty = use API name)
	Models map[string]string
	// Aliases for stable references
	Aliases map[string]string
	// Direct OpenAI models (not via OpenRouter)
	OpenAIModels []OpenAIModel
}{
	Models: map[string]string{
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
	},
	Aliases: map[string]string{
		// Default alias
		"beeper/default": "anthropic/claude-sonnet-4.5",

		// Stable aliases that can be remapped
		"beeper/fast":      "openai/gpt-5-mini",
		"beeper/smart":     "openai/gpt-5.2",
		"beeper/reasoning": "openai/gpt-5.2", // Uses reasoning effort parameter
	},
	OpenAIModels: []OpenAIModel{
		{
			ID:                  "openai/o1",
			Name:                "O1",
			Description:         "Advanced reasoning model",
			SupportsVision:      true,
			SupportsToolCalling: true,
			SupportsReasoning:   true,
			SupportsWebSearch:   true,
			ContextWindow:       200000,
			MaxOutputTokens:     100000,
		},
		{
			ID:                  "openai/o1-mini",
			Name:                "O1 Mini",
			Description:         "Fast reasoning model",
			SupportsVision:      true,
			SupportsToolCalling: true,
			SupportsReasoning:   true,
			SupportsWebSearch:   true,
			ContextWindow:       128000,
			MaxOutputTokens:     65536,
		},
		{
			ID:                  "openai/o3-mini",
			Name:                "O3 Mini",
			Description:         "Latest reasoning model",
			SupportsVision:      true,
			SupportsToolCalling: true,
			SupportsReasoning:   true,
			SupportsWebSearch:   true,
			ContextWindow:       200000,
			MaxOutputTokens:     100000,
		},
		{
			ID:                  "openai/gpt-4-turbo",
			Name:                "GPT 4 Turbo",
			Description:         "Previous generation GPT-4 with vision",
			SupportsVision:      true,
			SupportsToolCalling: true,
			SupportsReasoning:   false,
			SupportsWebSearch:   true,
			ContextWindow:       128000,
			MaxOutputTokens:     4096,
		},
	},
}

// OpenAIModel represents a direct OpenAI model (not via OpenRouter)
type OpenAIModel struct {
	ID                  string
	Name                string
	Description         string
	SupportsVision      bool
	SupportsToolCalling bool
	SupportsReasoning   bool
	SupportsWebSearch   bool
	ContextWindow       int
	MaxOutputTokens     int
}

// OpenRouterArchitecture contains model architecture information
type OpenRouterArchitecture struct {
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
	Modality         string   `json:"modality"`
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
	Audio           bool
	Video           bool
	PDF             bool
	ContextWindow   int
	MaxOutputTokens int
}

func main() {
	token := flag.String("openrouter-token", "", "OpenRouter API token")
	outputFile := flag.String("output", "pkg/connector/models_generated.go", "Output Go file")
	jsonFile := flag.String("json", "pkg/connector/beeper_models.json", "Output JSON file for clients")
	flag.Parse()

	if *token == "" {
		fmt.Fprintln(os.Stderr, "Error: --openrouter-token is required")
		os.Exit(1)
	}

	models, err := fetchOpenRouterModels(*token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching models: %v\n", err)
		os.Exit(1)
	}

	if err := generateGoFile(models, *outputFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Generated %s with %d models\n", *outputFile, len(modelConfig.Models)+len(modelConfig.OpenAIModels))

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

	// Audio: "audio" in architecture.input_modalities
	caps.Audio = slices.Contains(apiModel.Architecture.InputModalities, "audio")

	// Video: "video" in architecture.input_modalities
	caps.Video = slices.Contains(apiModel.Architecture.InputModalities, "video")

	// PDF: "file" in architecture.input_modalities (OpenRouter uses "file" for document support)
	// Also check for specific PDF-capable models
	caps.PDF = slices.Contains(apiModel.Architecture.InputModalities, "file") ||
		strings.Contains(modelID, "claude") || // Claude models support PDFs
		strings.Contains(modelID, "gemini") // Gemini models support PDFs

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

// ModelManifest contains all model definitions and aliases.
// Models are fetched from OpenRouter API, aliases are defined in the generator config.
var ModelManifest = struct {
	Models  map[string]ModelInfo
	Aliases map[string]string
}{
	Models: map[string]ModelInfo{
`)

	// Get sorted model IDs for deterministic output
	modelIDs := sortedKeys(modelConfig.Models)

	for _, modelID := range modelIDs {
		displayName := modelConfig.Models[modelID]
		apiModel, hasAPIData := apiModels[modelID]
		caps := detectCapabilities(modelID, apiModel, hasAPIData)

		buf.WriteString(fmt.Sprintf(`		%q: {
			ID:                  %q,
			Name:                %q,
			Provider:            "openrouter",
			SupportsVision:      %t,
			SupportsToolCalling: %t,
			SupportsReasoning:   %t,
			SupportsWebSearch:   %t,
			SupportsImageGen:    %t,
			SupportsAudio:       %t,
			SupportsVideo:       %t,
			SupportsPDF:         %t,
			ContextWindow:       %d,
			MaxOutputTokens:     %d,
			AvailableTools:      %s,
		},
`,
			modelID,
			modelID,
			displayName,
			caps.Vision,
			caps.ToolCalling,
			caps.Reasoning,
			caps.WebSearch,
			caps.ImageGen,
			caps.Audio,
			caps.Video,
			caps.PDF,
			caps.ContextWindow,
			caps.MaxOutputTokens,
			availableToolsGo(caps),
		))
	}

	// Add direct OpenAI models
	for _, model := range modelConfig.OpenAIModels {
		caps := ModelCapabilities{
			Vision:      model.SupportsVision,
			ToolCalling: model.SupportsToolCalling,
			Reasoning:   model.SupportsReasoning,
			WebSearch:   model.SupportsWebSearch,
		}
		buf.WriteString(fmt.Sprintf(`		%q: {
			ID:                  %q,
			Name:                %q,
			Provider:            "openai",
			Description:         %q,
			SupportsVision:      %t,
			SupportsToolCalling: %t,
			SupportsReasoning:   %t,
			SupportsWebSearch:   %t,
			SupportsImageGen:    false,
			SupportsAudio:       false,
			SupportsVideo:       false,
			SupportsPDF:         false,
			ContextWindow:       %d,
			MaxOutputTokens:     %d,
			AvailableTools:      %s,
		},
`,
			model.ID,
			model.ID,
			model.Name,
			model.Description,
			model.SupportsVision,
			model.SupportsToolCalling,
			model.SupportsReasoning,
			model.SupportsWebSearch,
			model.ContextWindow,
			model.MaxOutputTokens,
			availableToolsGo(caps),
		))
	}

	buf.WriteString(`	},
	Aliases: map[string]string{
`)

	// Add aliases
	aliasKeys := sortedKeys(modelConfig.Aliases)
	for _, alias := range aliasKeys {
		target := modelConfig.Aliases[alias]
		buf.WriteString(fmt.Sprintf(`		%q: %q,
`, alias, target))
	}

	buf.WriteString(`	},
}
`)

	return os.WriteFile(outputPath, []byte(buf.String()), 0644)
}

// JSONModelInfo mirrors the connector.ModelInfo struct for JSON output
type JSONModelInfo struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Provider            string   `json:"provider"`
	Description         string   `json:"description,omitempty"`
	SupportsVision      bool     `json:"supports_vision"`
	SupportsToolCalling bool     `json:"supports_tool_calling"`
	SupportsReasoning   bool     `json:"supports_reasoning"`
	SupportsWebSearch   bool     `json:"supports_web_search"`
	SupportsImageGen    bool     `json:"supports_image_gen,omitempty"`
	SupportsAudio       bool     `json:"supports_audio,omitempty"`
	SupportsVideo       bool     `json:"supports_video,omitempty"`
	SupportsPDF         bool     `json:"supports_pdf,omitempty"`
	ContextWindow       int      `json:"context_window,omitempty"`
	MaxOutputTokens     int      `json:"max_output_tokens,omitempty"`
	AvailableTools      []string `json:"available_tools,omitempty"`
}

// JSONManifest is the full manifest structure for JSON output
type JSONManifest struct {
	Models  []JSONModelInfo   `json:"models"`
	Aliases map[string]string `json:"aliases"`
}

func generateJSONFile(apiModels map[string]OpenRouterModel, outputPath string) error {
	var models []JSONModelInfo

	// Add OpenRouter models
	modelIDs := sortedKeys(modelConfig.Models)
	for _, modelID := range modelIDs {
		displayName := modelConfig.Models[modelID]
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
			SupportsAudio:       caps.Audio,
			SupportsVideo:       caps.Video,
			SupportsPDF:         caps.PDF,
			ContextWindow:       caps.ContextWindow,
			MaxOutputTokens:     caps.MaxOutputTokens,
			AvailableTools:      availableToolsJSON(caps),
		})
	}

	// Add direct OpenAI models
	for _, model := range modelConfig.OpenAIModels {
		caps := ModelCapabilities{
			Vision:      model.SupportsVision,
			ToolCalling: model.SupportsToolCalling,
			Reasoning:   model.SupportsReasoning,
			WebSearch:   model.SupportsWebSearch,
		}
		models = append(models, JSONModelInfo{
			ID:                  model.ID,
			Name:                model.Name,
			Provider:            "openai",
			Description:         model.Description,
			SupportsVision:      model.SupportsVision,
			SupportsToolCalling: model.SupportsToolCalling,
			SupportsReasoning:   model.SupportsReasoning,
			SupportsWebSearch:   model.SupportsWebSearch,
			ContextWindow:       model.ContextWindow,
			MaxOutputTokens:     model.MaxOutputTokens,
			AvailableTools:      availableToolsJSON(caps),
		})
	}

	manifest := JSONManifest{
		Models:  models,
		Aliases: modelConfig.Aliases,
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
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
