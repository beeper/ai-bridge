package connector

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/beeper/ai-bridge/pkg/textfs"
)

const (
	modelCatalogAgentID  = "__models__"
	modelCatalogStoreRef = "models.json"
	modelCatalogStoreAlt = "models/catalog.json"
)

type ModelCatalogEntry struct {
	ID            string   `json:"id"`
	Name          string   `json:"name,omitempty"`
	Provider      string   `json:"provider"`
	ContextWindow int      `json:"contextWindow,omitempty"`
	Reasoning     bool     `json:"reasoning,omitempty"`
	Input         []string `json:"input,omitempty"`
}

func (oc *AIClient) modelCatalogStore() (*textfs.Store, error) {
	if oc == nil || oc.UserLogin == nil {
		return nil, nil
	}
	bridgeID := string(oc.UserLogin.Bridge.DB.BridgeID)
	loginID := string(oc.UserLogin.ID)
	agentID := normalizeAgentID(strings.TrimSpace(modelCatalogAgentID))
	return textfs.NewStore(oc.UserLogin.Bridge.DB.Database, bridgeID, loginID, agentID), nil
}

func (oc *AIClient) loadModelCatalog(ctx context.Context, useCache bool) []ModelCatalogEntry {
	if oc == nil || oc.UserLogin == nil {
		return nil
	}
	if useCache {
		oc.modelCatalogMu.Lock()
		if oc.modelCatalogLoaded {
			cached := append([]ModelCatalogEntry(nil), oc.modelCatalogCache...)
			oc.modelCatalogMu.Unlock()
			return cached
		}
		oc.modelCatalogMu.Unlock()
	}

	store, err := oc.modelCatalogStore()
	if err != nil || store == nil {
		return nil
	}
	entry, found, err := store.Read(ctx, modelCatalogStoreRef)
	if err != nil || !found {
		if modelCatalogStoreAlt != "" {
			entry, found, err = store.Read(ctx, modelCatalogStoreAlt)
		}
	}
	if err != nil || !found {
		if useCache {
			oc.modelCatalogMu.Lock()
			oc.modelCatalogLoaded = true
			oc.modelCatalogCache = nil
			oc.modelCatalogMu.Unlock()
		}
		return nil
	}

	var raw any
	if err := json.Unmarshal([]byte(entry.Content), &raw); err != nil {
		return nil
	}
	entries := parseModelCatalog(raw)
	if useCache {
		oc.modelCatalogMu.Lock()
		oc.modelCatalogLoaded = true
		oc.modelCatalogCache = append([]ModelCatalogEntry(nil), entries...)
		oc.modelCatalogMu.Unlock()
	}
	return entries
}

func parseModelCatalog(raw any) []ModelCatalogEntry {
	if raw == nil {
		return nil
	}
	switch value := raw.(type) {
	case []any:
		return coerceModelEntries(value)
	case map[string]any:
		if models, ok := value["models"].([]any); ok {
			return coerceModelEntries(models)
		}
	}
	return nil
}

func coerceModelEntries(items []any) []ModelCatalogEntry {
	out := make([]ModelCatalogEntry, 0, len(items))
	for _, item := range items {
		entryMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := strings.TrimSpace(asString(entryMap["id"]))
		provider := strings.TrimSpace(asString(entryMap["provider"]))
		if id == "" || provider == "" {
			continue
		}
		name := strings.TrimSpace(asString(entryMap["name"]))
		if name == "" {
			name = id
		}
		out = append(out, ModelCatalogEntry{
			ID:            id,
			Name:          name,
			Provider:      provider,
			ContextWindow: asInt(entryMap["contextWindow"]),
			Reasoning:     asBool(entryMap["reasoning"]),
			Input:         asStringSlice(entryMap["input"]),
		})
	}
	return out
}

func asString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func asInt(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}

func asBool(value any) bool {
	if v, ok := value.(bool); ok {
		return v
	}
	return false
}

func asStringSlice(value any) []string {
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if str, ok := item.(string); ok {
			out = append(out, str)
		}
	}
	return out
}

func catalogInputIncludes(entry *ModelCatalogEntry, label string) bool {
	if entry == nil || label == "" {
		return false
	}
	for _, input := range entry.Input {
		if strings.EqualFold(input, label) {
			return true
		}
	}
	return false
}

func findModelCatalogEntry(catalog []ModelCatalogEntry, provider string, model string) *ModelCatalogEntry {
	if provider == "" || model == "" {
		return nil
	}
	needleProvider := strings.ToLower(strings.TrimSpace(provider))
	needleModel := strings.ToLower(strings.TrimSpace(model))
	for i := range catalog {
		entry := &catalog[i]
		if strings.ToLower(entry.Provider) == needleProvider && strings.ToLower(entry.ID) == needleModel {
			return entry
		}
	}
	return nil
}

func modelCatalogSupportsVision(entry *ModelCatalogEntry) bool {
	return catalogInputIncludes(entry, "image")
}

func (oc *AIClient) modelSupportsVision(ctx context.Context, meta *PortalMetadata) bool {
	if oc == nil || meta == nil {
		return false
	}
	modelID := strings.TrimSpace(oc.effectiveModel(meta))
	if modelID == "" {
		return false
	}
	caps := getModelCapabilities(modelID, oc.findModelInfo(modelID))
	if caps.SupportsVision {
		return true
	}
	catalog := oc.loadModelCatalog(ctx, true)
	if len(catalog) == 0 {
		return false
	}
	provider, model := splitModelProvider(modelID)
	if provider == "" {
		provider = normalizeMediaProviderID(loginMetadata(oc.UserLogin).Provider)
	}
	if provider == "" {
		return false
	}
	entry := findModelCatalogEntry(catalog, provider, model)
	return modelCatalogSupportsVision(entry)
}

func normalizeCatalogProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func normalizeCatalogModelID(entry ModelCatalogEntry) string {
	id := strings.TrimSpace(entry.ID)
	if id == "" {
		return ""
	}
	if strings.Contains(id, "/") {
		return id
	}
	provider := normalizeCatalogProvider(entry.Provider)
	if provider == ProviderOpenAI {
		return ProviderOpenAI + "/" + id
	}
	if provider == ProviderOpenRouter || provider == ProviderBeeper {
		return id
	}
	if provider != "" {
		return provider + "/" + id
	}
	return id
}

func (oc *AIClient) loadModelCatalogModels(ctx context.Context) []ModelInfo {
	entries := oc.loadModelCatalog(ctx, true)
	if len(entries) == 0 {
		return nil
	}
	models := make([]ModelInfo, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.Provider) == "" {
			continue
		}
		normalizedID := normalizeCatalogModelID(entry)
		if normalizedID == "" {
			continue
		}
		provider := normalizeCatalogProvider(entry.Provider)
		info := ModelInfo{
			ID:                  normalizedID,
			Name:                strings.TrimSpace(entry.Name),
			Provider:            provider,
			SupportsVision:      catalogInputIncludes(&entry, "image"),
			SupportsAudio:       catalogInputIncludes(&entry, "audio"),
			SupportsVideo:       catalogInputIncludes(&entry, "video"),
			SupportsPDF:         catalogInputIncludes(&entry, "pdf"),
			SupportsToolCalling: true,
			SupportsReasoning:   entry.Reasoning,
			ContextWindow:       entry.ContextWindow,
		}
		if info.Name == "" {
			info.Name = normalizedID
		}
		models = append(models, info)
	}
	return models
}

func (oc *AIClient) findModelInfoInCatalog(modelID string) *ModelInfo {
	if oc == nil || strings.TrimSpace(modelID) == "" {
		return nil
	}
	ctx := oc.backgroundContext(context.Background())
	entries := oc.loadModelCatalog(ctx, true)
	if len(entries) == 0 {
		return nil
	}
	normalizedTarget := strings.TrimSpace(modelID)
	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.Provider) == "" {
			continue
		}
		normalizedID := normalizeCatalogModelID(entry)
		if strings.EqualFold(normalizedTarget, normalizedID) ||
			strings.EqualFold(normalizedTarget, entry.ID) {
			info := ModelInfo{
				ID:                  normalizedID,
				Name:                strings.TrimSpace(entry.Name),
				Provider:            normalizeCatalogProvider(entry.Provider),
				SupportsVision:      catalogInputIncludes(&entry, "image"),
				SupportsAudio:       catalogInputIncludes(&entry, "audio"),
				SupportsVideo:       catalogInputIncludes(&entry, "video"),
				SupportsPDF:         catalogInputIncludes(&entry, "pdf"),
				SupportsToolCalling: true,
				SupportsReasoning:   entry.Reasoning,
				ContextWindow:       entry.ContextWindow,
			}
			if info.Name == "" {
				info.Name = normalizedID
			}
			return &info
		}
	}
	return nil
}
