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

	"github.com/beeper/ai-bridge/pkg/connector"
)

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
	outputFile := flag.String("output", "pkg/connector/beeper_models_generated.go", "Output Go file")
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
	fmt.Printf("Generated %s with %d models\n", *outputFile, len(connector.BeeperModelList))

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

// BeeperModelsGenerated contains model definitions fetched from OpenRouter API.
// The model list is manually curated in cmd/generate-models/main.go.
// All capabilities are auto-detected from the OpenRouter API.
var BeeperModelsGenerated = []ModelInfo{
`)

	// Get sorted model IDs for deterministic output
	modelIDs := sortedKeys(connector.BeeperModelList)

	for _, modelID := range modelIDs {
		displayName := connector.BeeperModelList[modelID]
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
		SupportsAudio:       %t,
		SupportsVideo:       %t,
		SupportsPDF:         %t,
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
			caps.Audio,
			caps.Video,
			caps.PDF,
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
	SupportsAudio       bool     `json:"supports_audio,omitempty"`
	SupportsVideo       bool     `json:"supports_video,omitempty"`
	SupportsPDF         bool     `json:"supports_pdf,omitempty"`
	ContextWindow       int      `json:"context_window,omitempty"`
	MaxOutputTokens     int      `json:"max_output_tokens,omitempty"`
	AvailableTools      []string `json:"available_tools,omitempty"`
}

func generateJSONFile(apiModels map[string]OpenRouterModel, outputPath string) error {
	var models []JSONModelInfo

	modelIDs := sortedKeys(connector.BeeperModelList)

	for _, modelID := range modelIDs {
		displayName := connector.BeeperModelList[modelID]
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
