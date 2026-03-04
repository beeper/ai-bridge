package connector

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"

	integrationruntime "github.com/beeper/ai-bridge/pkg/integrations/runtime"
	"github.com/beeper/ai-bridge/pkg/textfs"
)

// runtimeIntegrationHost implements integrationruntime.Host and all optional
// capability interfaces, bridging integration modules to the AIClient.
type runtimeIntegrationHost struct {
	client *AIClient
}

func newRuntimeIntegrationHost(client *AIClient) *runtimeIntegrationHost {
	return &runtimeIntegrationHost{client: client}
}

// hasClient is a consolidated nil guard for the two most common checks.
func (h *runtimeIntegrationHost) hasClient() bool {
	return h != nil && h.client != nil
}

// ---- Core Host interface ----

func (h *runtimeIntegrationHost) Logger() integrationruntime.Logger {
	return &runtimeLogger{client: h.client}
}

func (h *runtimeIntegrationHost) Now() time.Time { return time.Now() }

func (h *runtimeIntegrationHost) StoreBackend() integrationruntime.StoreBackend {
	if !h.hasClient() {
		return nil
	}
	return &hostStoreBackend{backend: &lazyStoreBackend{client: h.client}}
}

func (h *runtimeIntegrationHost) PortalResolver() integrationruntime.PortalResolver {
	if !h.hasClient() {
		return nil
	}
	return &hostPortalResolver{client: h.client}
}

func (h *runtimeIntegrationHost) Dispatch() integrationruntime.Dispatch {
	if !h.hasClient() {
		return nil
	}
	return &hostDispatch{client: h.client}
}

func (h *runtimeIntegrationHost) SessionStore() integrationruntime.SessionStore {
	if !h.hasClient() {
		return nil
	}
	return &hostSessionStore{client: h.client}
}

func (h *runtimeIntegrationHost) Heartbeat() integrationruntime.Heartbeat {
	if !h.hasClient() {
		return nil
	}
	return &hostHeartbeat{client: h.client}
}

func (h *runtimeIntegrationHost) ToolExec() integrationruntime.ToolExec {
	if !h.hasClient() {
		return nil
	}
	return &hostToolExec{client: h.client}
}

func (h *runtimeIntegrationHost) PromptContext() integrationruntime.PromptContext {
	return &hostPromptContext{}
}

func (h *runtimeIntegrationHost) DBAccess() integrationruntime.DBAccess {
	if !h.hasClient() {
		return nil
	}
	return &hostDBAccess{client: h.client}
}

func (h *runtimeIntegrationHost) ConfigLookup() integrationruntime.ConfigLookup { return h }

// ---- ConfigLookup ----

func (h *runtimeIntegrationHost) ModuleEnabled(name string) bool {
	if !h.hasClient() || h.client.connector == nil {
		return true
	}
	cfg := h.client.connector.Config.Integrations
	if cfg == nil || cfg.Modules == nil {
		return true
	}
	normalized := strings.ToLower(strings.TrimSpace(name))
	raw, exists := cfg.Modules[normalized]
	if !exists {
		return true
	}
	switch v := raw.(type) {
	case bool:
		return v
	case map[string]any:
		if enabled, ok := v["enabled"]; ok {
			if b, ok := enabled.(bool); ok {
				return b
			}
		}
		return true
	default:
		return true
	}
}

func (h *runtimeIntegrationHost) ModuleConfig(name string) map[string]any {
	if !h.hasClient() || h.client.connector == nil {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(name))
	// Check integrations-level module config first.
	if cfg := h.client.connector.Config.Integrations; cfg != nil && cfg.Modules != nil {
		if raw := cfg.Modules[normalized]; raw != nil {
			if typed, ok := raw.(map[string]any); ok {
				return typed
			}
		}
	}
	// Fall back to top-level module config.
	if h.client.connector.Config.Modules != nil {
		if raw := h.client.connector.Config.Modules[normalized]; raw != nil {
			if typed, ok := raw.(map[string]any); ok {
				return typed
			}
		}
	}
	return nil
}

func (h *runtimeIntegrationHost) AgentModuleConfig(agentID string, module string) map[string]any {
	if !h.hasClient() || h.client.connector == nil {
		return nil
	}
	store := NewAgentStoreAdapter(h.client)
	agent, err := store.GetAgentByID(h.client.backgroundContext(context.TODO()), agentID)
	if err != nil || agent == nil {
		return nil
	}
	// Marshal the entire agent to a generic map and extract the module key.
	raw, err := json.Marshal(agent)
	if err != nil {
		return nil
	}
	var agentMap map[string]any
	if err := json.Unmarshal(raw, &agentMap); err != nil {
		return nil
	}
	moduleName := strings.ToLower(strings.TrimSpace(module))
	moduleData, ok := agentMap[moduleName].(map[string]any)
	if !ok {
		return nil
	}
	return moduleData
}

// ---- AIClient message helpers (called from sessions_tools.go) ----

func (oc *AIClient) lastAssistantMessageInfo(ctx context.Context, portal *bridgev2.Portal) (string, int64) {
	if portal == nil || oc == nil || oc.UserLogin == nil || oc.UserLogin.Bridge == nil || oc.UserLogin.Bridge.DB == nil || oc.UserLogin.Bridge.DB.Message == nil {
		return "", 0
	}
	messages, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, 20)
	if err != nil {
		return "", 0
	}
	bestID := ""
	bestTS := int64(0)
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		meta := messageMeta(msg)
		if meta == nil || meta.Role != "assistant" {
			continue
		}
		ts := msg.Timestamp.UnixMilli()
		if bestID == "" || ts > bestTS {
			bestID = msg.MXID.String()
			bestTS = ts
		}
	}
	return bestID, bestTS
}

func (oc *AIClient) waitForNewAssistantMessage(ctx context.Context, portal *bridgev2.Portal, lastID string, lastTimestamp int64) (*database.Message, bool) {
	if portal == nil || oc == nil || oc.UserLogin == nil || oc.UserLogin.Bridge == nil || oc.UserLogin.Bridge.DB == nil || oc.UserLogin.Bridge.DB.Message == nil {
		return nil, false
	}
	messages, err := oc.UserLogin.Bridge.DB.Message.GetLastNInPortal(ctx, portal.PortalKey, 20)
	if err != nil {
		return nil, false
	}
	var candidate *database.Message
	candidateTS := lastTimestamp
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		meta := messageMeta(msg)
		if meta == nil || meta.Role != "assistant" {
			continue
		}
		idStr := msg.MXID.String()
		ts := msg.Timestamp.UnixMilli()
		if ts < lastTimestamp {
			continue
		}
		if ts == lastTimestamp && idStr == lastID {
			continue
		}
		if candidate == nil || ts > candidateTS {
			candidate = msg
			candidateTS = ts
		}
	}
	if candidate == nil {
		return nil, false
	}
	return candidate, true
}

// ---- Helpers ----

func textStoreForAgent(client *AIClient, agentID string) *textfs.Store {
	if client == nil || client.UserLogin == nil || client.UserLogin.Bridge == nil || client.UserLogin.Bridge.DB == nil {
		return nil
	}
	db := client.bridgeDB()
	if db == nil {
		return nil
	}
	return textfs.NewStore(
		db,
		string(client.UserLogin.Bridge.DB.BridgeID),
		string(client.UserLogin.ID),
		agentID,
	)
}

func resolveEmbeddingConfigGeneric(client *AIClient, apiKey string, baseURL string, headers map[string]string, serviceName string, proxyPath string) (string, string, map[string]string) {
	if strings.TrimSpace(apiKey) == "" && client != nil && client.connector != nil {
		meta := loginMetadata(client.UserLogin)
		apiKey = strings.TrimSpace(client.connector.resolveOpenAIAPIKey(meta))
		if meta != nil {
			if apiKey == "" && meta.Provider == ProviderMagicProxy {
				apiKey = strings.TrimSpace(meta.APIKey)
			}
			if apiKey == "" && meta.Provider == ProviderBeeper {
				services := client.connector.resolveServiceConfig(meta)
				if svc, ok := services[serviceName]; ok {
					apiKey = strings.TrimSpace(svc.APIKey)
					if baseURL == "" {
						baseURL = strings.TrimSpace(svc.BaseURL)
					}
				}
			}
		}
	}
	if strings.TrimSpace(baseURL) == "" && client != nil && client.connector != nil {
		if meta := loginMetadata(client.UserLogin); meta != nil {
			if meta.Provider == ProviderMagicProxy {
				base := normalizeMagicProxyBaseURL(meta.BaseURL)
				if base != "" {
					baseURL = joinProxyPath(base, proxyPath)
				}
			} else if meta.Provider == ProviderBeeper {
				services := client.connector.resolveServiceConfig(meta)
				if svc, ok := services[serviceName]; ok && strings.TrimSpace(svc.BaseURL) != "" {
					baseURL = strings.TrimSpace(svc.BaseURL)
				}
			}
		}
		if baseURL == "" {
			baseURL = client.connector.resolveOpenAIBaseURL()
		}
	}
	return apiKey, baseURL, headers
}

// ---- Small helpers used by host sub-adapters ----

func portalKeyFromParts(client *AIClient, portalID string, receiver string) networkid.PortalKey {
	key := networkid.PortalKey{ID: networkid.PortalID(portalID)}
	if receiver != "" {
		key.Receiver = networkid.UserLoginID(receiver)
	} else if client != nil && client.UserLogin != nil {
		key.Receiver = client.UserLogin.ID
	}
	return key
}

func portalRoomIDFromString(roomID string) id.RoomID {
	return id.RoomID(roomID)
}

func updateSessionStoreEntry(ctx context.Context, backend bridgeStoreBackend, key string, updater func(raw map[string]any) map[string]any) {
	if backend == nil || updater == nil || strings.TrimSpace(key) == "" {
		return
	}
	storeKey := "session:" + key
	existing := make(map[string]any)
	if data, ok, err := backend.Read(ctx, storeKey); err == nil && ok && len(data) > 0 {
		_ = json.Unmarshal(data, &existing)
	}
	updated := updater(existing)
	if updated == nil {
		return
	}
	data, err := json.Marshal(updated)
	if err != nil {
		return
	}
	_ = backend.Write(ctx, storeKey, data)
}
