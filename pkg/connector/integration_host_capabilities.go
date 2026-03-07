package connector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"

	"github.com/beeper/ai-bridge/pkg/agents"
	integrationruntime "github.com/beeper/ai-bridge/pkg/integrations/runtime"
	airuntime "github.com/beeper/ai-bridge/pkg/runtime"
)

// ---- Optional Host capability: RawLoggerAccess ----

func (h *runtimeIntegrationHost) RawLogger() any {
	if !h.hasClient() {
		return zerolog.Logger{}
	}
	return h.client.log
}

// ---- Optional Host capability: PortalManager ----

func (h *runtimeIntegrationHost) GetOrCreatePortal(ctx context.Context, portalID string, receiver string, displayName string, setupMeta func(meta any)) (portal any, roomID string, err error) {
	if !h.hasClient() || h.client.UserLogin == nil {
		return nil, "", fmt.Errorf("missing login")
	}
	portalKey := portalKeyFromParts(h.client, portalID, receiver)
	p, err := h.client.UserLogin.Bridge.GetPortalByKey(ctx, portalKey)
	if err != nil {
		return nil, "", err
	}
	if p.MXID != "" {
		return p, p.MXID.String(), nil
	}
	meta := &PortalMetadata{}
	if setupMeta != nil {
		setupMeta(meta)
	}
	p.Metadata = meta
	p.Name = displayName
	p.NameSet = true
	if err := p.Save(ctx); err != nil {
		return nil, "", fmt.Errorf("failed to save portal: %w", err)
	}
	chatInfo := &bridgev2.ChatInfo{Name: &p.Name}
	if err := p.CreateMatrixRoom(ctx, h.client.UserLogin, chatInfo); err != nil {
		return nil, "", fmt.Errorf("failed to create Matrix room: %w", err)
	}
	return p, p.MXID.String(), nil
}

func (h *runtimeIntegrationHost) SavePortal(ctx context.Context, portal any, reason string) error {
	if !h.hasClient() {
		return nil
	}
	p, _ := portal.(*bridgev2.Portal)
	if p == nil {
		return nil
	}
	h.client.savePortalQuiet(ctx, p, reason)
	return nil
}

func (h *runtimeIntegrationHost) PortalRoomID(portal any) string {
	p, _ := portal.(*bridgev2.Portal)
	if p == nil {
		return ""
	}
	return p.MXID.String()
}

func (h *runtimeIntegrationHost) PortalKeyString(portal any) string {
	p, _ := portal.(*bridgev2.Portal)
	if p == nil {
		return ""
	}
	return p.PortalKey.String()
}

// ---- Optional Host capability: MetadataAccess ----

func (h *runtimeIntegrationHost) GetModuleMeta(meta any, key string) any {
	m, _ := meta.(*PortalMetadata)
	if m == nil || m.ModuleMeta == nil {
		return nil
	}
	return m.ModuleMeta[key]
}

func (h *runtimeIntegrationHost) SetModuleMeta(meta any, key string, value any) {
	m, _ := meta.(*PortalMetadata)
	if m == nil {
		return
	}
	if m.ModuleMeta == nil {
		m.ModuleMeta = make(map[string]any)
	}
	m.ModuleMeta[key] = value
}

func (h *runtimeIntegrationHost) IsSimpleMode(meta any) bool {
	m, _ := meta.(*PortalMetadata)
	return isSimpleMode(m)
}

func (h *runtimeIntegrationHost) AgentIDFromMeta(meta any) string {
	m, _ := meta.(*PortalMetadata)
	return resolveAgentID(m)
}

func (h *runtimeIntegrationHost) CompactionCount(meta any) int {
	m, _ := meta.(*PortalMetadata)
	if m == nil {
		return 0
	}
	return m.CompactionCount
}

func (h *runtimeIntegrationHost) IsGroupChat(ctx context.Context, portal any) bool {
	if !h.hasClient() {
		return false
	}
	p, _ := portal.(*bridgev2.Portal)
	if p == nil {
		return false
	}
	return h.client.isGroupChat(ctx, p)
}

func (h *runtimeIntegrationHost) IsInternalRoom(meta any) bool {
	m, _ := meta.(*PortalMetadata)
	if m == nil {
		return false
	}
	return m.IsBuilderRoom || isModuleInternalRoom(m)
}

func (h *runtimeIntegrationHost) PortalMeta(portal any) any {
	p, _ := portal.(*bridgev2.Portal)
	return portalMeta(p)
}

func (h *runtimeIntegrationHost) CloneMeta(portal any) any {
	p, _ := portal.(*bridgev2.Portal)
	return clonePortalMetadata(portalMeta(p))
}

func (h *runtimeIntegrationHost) SetMetaField(meta any, key string, value any) {
	m, _ := meta.(*PortalMetadata)
	if m == nil {
		return
	}
	switch key {
	case "AgentID":
		if v, ok := value.(string); ok {
			m.AgentID = v
		}
	case "Model":
		if v, ok := value.(string); ok {
			m.Model = strings.TrimSpace(v)
		}
	case "ReasoningEffort":
		if v, ok := value.(string); ok {
			m.ReasoningEffort = strings.TrimSpace(v)
		}
	case "DisabledTools":
		if v, ok := value.([]string); ok {
			m.DisabledTools = v
		}
	}
}

// ---- Optional Host capability: MessageHelper ----

func (h *runtimeIntegrationHost) RecentMessages(ctx context.Context, portal any, count int) []integrationruntime.MessageSummary {
	if !h.hasClient() {
		return nil
	}
	p, _ := portal.(*bridgev2.Portal)
	if p == nil || count <= 0 || h.client.UserLogin == nil || h.client.UserLogin.Bridge == nil || h.client.UserLogin.Bridge.DB == nil {
		return nil
	}
	maxMessages := count
	if maxMessages > 10 {
		maxMessages = 10
	}
	history, err := h.client.UserLogin.Bridge.DB.Message.GetLastNInPortal(h.client.backgroundContext(ctx), p.PortalKey, maxMessages)
	if err != nil || len(history) == 0 {
		return nil
	}
	out := make([]integrationruntime.MessageSummary, 0, len(history))
	for i := len(history) - 1; i >= 0; i-- {
		meta := messageMeta(history[i])
		if meta == nil {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(meta.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		text := strings.TrimSpace(meta.Body)
		if text == "" {
			continue
		}
		out = append(out, integrationruntime.MessageSummary{Role: role, Body: text})
	}
	return out
}

func (h *runtimeIntegrationHost) LastAssistantMessage(ctx context.Context, portal any) (id string, timestamp int64) {
	if !h.hasClient() {
		return "", 0
	}
	p, _ := portal.(*bridgev2.Portal)
	return h.client.lastAssistantMessageInfo(ctx, p)
}

func (h *runtimeIntegrationHost) WaitForAssistantMessage(ctx context.Context, portal any, afterID string, afterTS int64) (*integrationruntime.AssistantMessageInfo, bool) {
	if !h.hasClient() {
		return nil, false
	}
	p, _ := portal.(*bridgev2.Portal)
	msg, found := h.client.waitForNewAssistantMessage(ctx, p, afterID, afterTS)
	if !found || msg == nil {
		return nil, false
	}
	meta := messageMeta(msg)
	if meta == nil {
		return nil, false
	}
	return &integrationruntime.AssistantMessageInfo{
		Body:             strings.TrimSpace(meta.Body),
		Model:            strings.TrimSpace(meta.Model),
		PromptTokens:     meta.PromptTokens,
		CompletionTokens: meta.CompletionTokens,
	}, true
}

// ---- Optional Host capability: HeartbeatHelper ----

func (h *runtimeIntegrationHost) RunHeartbeatOnce(ctx context.Context, reason string) (status string, reasonMsg string) {
	if !h.hasClient() || h.client.heartbeatRunner == nil {
		return "skipped", "disabled"
	}
	res := h.client.heartbeatRunner.run(reason)
	return res.Status, res.Reason
}

func (h *runtimeIntegrationHost) ResolveHeartbeatSessionPortal(agentID string) (portal any, sessionKey string, err error) {
	if !h.hasClient() {
		return nil, "", fmt.Errorf("missing client")
	}
	hb := resolveHeartbeatConfig(&h.client.connector.Config, agentID)
	p, sk, e := h.client.resolveHeartbeatSessionPortal(agentID, hb)
	return p, sk, e
}

func (h *runtimeIntegrationHost) ResolveHeartbeatSessionKey(agentID string) string {
	if !h.hasClient() {
		return ""
	}
	hb := resolveHeartbeatConfig(&h.client.connector.Config, agentID)
	return strings.TrimSpace(h.client.resolveHeartbeatSession(agentID, hb).SessionKey)
}

func (h *runtimeIntegrationHost) HeartbeatAckMaxChars(agentID string) int {
	if !h.hasClient() {
		return 0
	}
	hb := resolveHeartbeatConfig(&h.client.connector.Config, agentID)
	return resolveHeartbeatAckMaxChars(&h.client.connector.Config, hb)
}

func (h *runtimeIntegrationHost) EnqueueSystemEvent(sessionKey string, text string, agentID string) {
	enqueueSystemEvent(sessionKey, text, agentID)
}

func (h *runtimeIntegrationHost) PersistSystemEvents() {
	if !h.hasClient() {
		return
	}
	persistSystemEventsSnapshot(h.client.bridgeStateBackend(), h.client.Log())
}

func (h *runtimeIntegrationHost) ResolveLastTarget(agentID string) (channel string, target string, ok bool) {
	if !h.hasClient() {
		return "", "", false
	}
	storeRef, mainKey := h.client.resolveHeartbeatMainSessionRef(agentID)
	entry, found := h.client.getSessionEntry(context.Background(), storeRef, mainKey)
	if !found {
		return "", "", false
	}
	return entry.LastChannel, entry.LastTo, true
}

// ---- Optional Host capability: AgentHelper ----

func (h *runtimeIntegrationHost) ResolveAgentID(raw string, fallbackDefault string) string {
	if !h.hasClient() {
		return agents.DefaultAgentID
	}
	normalized := normalizeAgentID(raw)
	if normalized == "" || !h.agentExists(normalized) {
		if fallbackDefault != "" {
			return normalizeAgentID(fallbackDefault)
		}
		return agents.DefaultAgentID
	}
	return normalized
}

func (h *runtimeIntegrationHost) NormalizeAgentID(raw string) string {
	return normalizeAgentID(raw)
}

func (h *runtimeIntegrationHost) AgentExists(normalizedID string) bool {
	return h.agentExists(normalizedID)
}

func (h *runtimeIntegrationHost) agentExists(normalizedID string) bool {
	if !h.hasClient() || h.client.connector == nil {
		return false
	}
	cfg := &h.client.connector.Config
	if cfg.Agents == nil {
		return false
	}
	for _, entry := range cfg.Agents.List {
		if normalizeAgentID(entry.ID) == strings.TrimSpace(normalizedID) {
			return true
		}
	}
	return false
}

func (h *runtimeIntegrationHost) DefaultAgentID() string {
	return agents.DefaultAgentID
}

func (h *runtimeIntegrationHost) AgentTimeoutSeconds() int {
	if !h.hasClient() || h.client.connector == nil {
		return 600
	}
	cfg := &h.client.connector.Config
	if cfg.Agents != nil && cfg.Agents.Defaults != nil && cfg.Agents.Defaults.TimeoutSeconds > 0 {
		return cfg.Agents.Defaults.TimeoutSeconds
	}
	return 600
}

func (h *runtimeIntegrationHost) UserTimezone() (tz string, loc *time.Location) {
	if !h.hasClient() {
		return "", time.UTC
	}
	tz, loc = h.client.resolveUserTimezone()
	if loc == nil {
		loc = time.UTC
	}
	return tz, loc
}

func (h *runtimeIntegrationHost) NormalizeThinkingLevel(raw string) (string, bool) {
	return normalizeThinkingLevel(raw)
}

// ---- Optional Host capability: ModelHelper ----

func (h *runtimeIntegrationHost) EffectiveModel(meta any) string {
	if !h.hasClient() {
		return ""
	}
	m, _ := meta.(*PortalMetadata)
	return h.client.effectiveModel(m)
}

func (h *runtimeIntegrationHost) ContextWindow(meta any) int {
	if !h.hasClient() {
		return 0
	}
	m, _ := meta.(*PortalMetadata)
	return h.client.getModelContextWindow(m)
}

// ---- Optional Host capability: ContextHelper ----

func (h *runtimeIntegrationHost) MergeDisconnectContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if !h.hasClient() {
		return context.WithCancel(ctx)
	}
	var base context.Context
	if h.client.disconnectCtx != nil {
		base = h.client.disconnectCtx
	} else if h.client.UserLogin != nil && h.client.UserLogin.Bridge != nil && h.client.UserLogin.Bridge.BackgroundCtx != nil {
		base = h.client.UserLogin.Bridge.BackgroundCtx
	} else {
		base = context.Background()
	}
	if model, ok := modelOverrideFromContext(ctx); ok {
		base = withModelOverride(base, model)
	}
	var merged context.Context
	var cancel context.CancelFunc
	if deadline, ok := ctx.Deadline(); ok {
		merged, cancel = context.WithDeadline(base, deadline)
	} else {
		merged, cancel = context.WithCancel(base)
	}
	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-merged.Done():
		}
	}()
	return h.client.loggerForContext(ctx).WithContext(merged), cancel
}

func (h *runtimeIntegrationHost) BackgroundContext(ctx context.Context) context.Context {
	if !h.hasClient() {
		return ctx
	}
	return h.client.backgroundContext(ctx)
}

// ---- Optional Host capability: ChatCompletionAPI ----

func (h *runtimeIntegrationHost) NewCompletion(ctx context.Context, model string, messages []openai.ChatCompletionMessageParamUnion, toolParams any) (*integrationruntime.CompletionResult, error) {
	if !h.hasClient() {
		return nil, fmt.Errorf("missing client")
	}
	params, _ := toolParams.([]openai.ChatCompletionToolUnionParam)
	req := openai.ChatCompletionNewParams{
		Model:    model,
		Messages: messages,
		Tools:    params,
	}
	resp, err := h.client.api.Chat.Completions.New(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return &integrationruntime.CompletionResult{Done: true}, nil
	}
	msg := resp.Choices[0].Message
	assistant := msg.ToAssistantMessageParam()
	result := &integrationruntime.CompletionResult{
		AssistantMessage: openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant},
	}
	if len(msg.ToolCalls) == 0 {
		result.Done = true
	} else {
		calls := make([]integrationruntime.CompletionToolCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			calls = append(calls, integrationruntime.CompletionToolCall{
				ID:       tc.ID,
				Name:     strings.TrimSpace(tc.Function.Name),
				ArgsJSON: tc.Function.Arguments,
			})
		}
		result.ToolCalls = calls
	}
	return result, nil
}

// ---- Optional Host capability: ToolPolicyHelper ----

func (h *runtimeIntegrationHost) IsToolEnabled(meta any, toolName string) bool {
	if !h.hasClient() {
		return true
	}
	m, _ := meta.(*PortalMetadata)
	if m == nil {
		return true
	}
	return h.client.isToolEnabled(m, toolName)
}

func (h *runtimeIntegrationHost) AllToolDefinitions() []integrationruntime.ToolDefinition {
	tools := BuiltinTools()
	out := make([]integrationruntime.ToolDefinition, 0, len(tools))
	out = append(out, tools...)
	return out
}

func (h *runtimeIntegrationHost) ExecuteToolInContext(ctx context.Context, portal any, meta any, name string, argsJSON string) (string, error) {
	if !h.hasClient() {
		return "", fmt.Errorf("missing client")
	}
	p, _ := portal.(*bridgev2.Portal)
	m, _ := meta.(*PortalMetadata)
	if m != nil && !h.client.isToolEnabled(m, name) {
		return "", fmt.Errorf("tool %s is disabled", name)
	}
	toolCtx := WithBridgeToolContext(ctx, &BridgeToolContext{
		Client: h.client,
		Portal: p,
		Meta:   m,
	})
	return h.client.executeBuiltinTool(toolCtx, p, name, argsJSON)
}

func (h *runtimeIntegrationHost) ToolsToOpenAIParams(tools []integrationruntime.ToolDefinition) any {
	if !h.hasClient() {
		return nil
	}
	bridgeTools := make([]ToolDefinition, 0, len(tools))
	bridgeTools = append(bridgeTools, tools...)
	params := ToOpenAIChatTools(bridgeTools, &h.client.log)
	return dedupeChatToolParams(params)
}

// ---- Optional Host capability: TextFileHelper ----

func (h *runtimeIntegrationHost) ReadTextFile(ctx context.Context, agentID string, path string) (content string, filePath string, found bool, err error) {
	if !h.hasClient() {
		return "", "", false, fmt.Errorf("storage unavailable")
	}
	store := textStoreForAgent(h.client, agentID)
	if store == nil {
		return "", "", false, fmt.Errorf("storage unavailable")
	}
	entry, ok, e := store.Read(ctx, path)
	if e != nil {
		return "", "", false, e
	}
	if !ok {
		return "", "", false, nil
	}
	return entry.Content, entry.Path, true, nil
}

func (h *runtimeIntegrationHost) WriteTextFile(ctx context.Context, portal any, meta any, agentID string, mode string, path string, content string, maxBytes int) (finalPath string, err error) {
	if !h.hasClient() {
		return "", fmt.Errorf("storage unavailable")
	}
	store := textStoreForAgent(h.client, agentID)
	if store == nil {
		return "", fmt.Errorf("storage unavailable")
	}
	if len([]byte(content)) > maxBytes {
		return "", fmt.Errorf("content exceeds %d bytes", maxBytes)
	}
	if strings.EqualFold(strings.TrimSpace(mode), "append") {
		if existing, ok, e := store.Read(ctx, path); e != nil {
			return "", fmt.Errorf("failed to read existing file for append: %w", e)
		} else if ok {
			sep := "\n"
			if strings.HasSuffix(existing.Content, "\n") || existing.Content == "" {
				sep = ""
			}
			content = existing.Content + sep + content
			if len([]byte(content)) > maxBytes {
				return "", fmt.Errorf("content exceeds %d bytes after append", maxBytes)
			}
		}
	}
	entry, e := store.Write(ctx, path, content)
	if e != nil {
		return "", e
	}
	if entry != nil {
		p, _ := portal.(*bridgev2.Portal)
		m, _ := meta.(*PortalMetadata)
		toolCtx := WithBridgeToolContext(ctx, &BridgeToolContext{
			Client: h.client,
			Portal: p,
			Meta:   m,
		})
		notifyIntegrationFileChanged(toolCtx, entry.Path)
		maybeRefreshAgentIdentity(toolCtx, entry.Path)
		return entry.Path, nil
	}
	return path, nil
}

// ---- Optional Host capability: EmbeddingHelper ----

func (h *runtimeIntegrationHost) ResolveOpenAIEmbeddingConfig(apiKey string, baseURL string, headers map[string]string) (string, string, map[string]string) {
	if !h.hasClient() {
		return apiKey, baseURL, headers
	}
	return resolveEmbeddingConfigGeneric(h.client, apiKey, baseURL, headers, serviceOpenRouter, "/openrouter/v1")
}

func (h *runtimeIntegrationHost) ResolveDirectOpenAIEmbeddingConfig(apiKey string, baseURL string, headers map[string]string) (string, string, map[string]string) {
	if !h.hasClient() {
		return apiKey, baseURL, headers
	}
	return resolveEmbeddingConfigGeneric(h.client, apiKey, baseURL, headers, serviceOpenAI, "/openai/v1")
}

func (h *runtimeIntegrationHost) ResolveGeminiEmbeddingConfig(apiKey string, baseURL string, headers map[string]string) (string, string, map[string]string) {
	return apiKey, baseURL, headers
}

// ---- Optional Host capability: OverflowHelper ----

func (h *runtimeIntegrationHost) SmartTruncatePrompt(prompt []openai.ChatCompletionMessageParamUnion, ratio float64) []openai.ChatCompletionMessageParamUnion {
	return airuntime.SmartTruncatePrompt(prompt, ratio)
}

func (h *runtimeIntegrationHost) EstimateTokens(prompt []openai.ChatCompletionMessageParamUnion, model string) int {
	if len(prompt) == 0 {
		return 0
	}
	if count, err := EstimateTokens(prompt, model); err == nil && count > 0 {
		return count
	}
	return estimatePromptTokensFallback(prompt)
}

func (h *runtimeIntegrationHost) CompactorReserveTokens() int {
	if !h.hasClient() {
		return airuntime.DefaultPruningConfig().ReserveTokens
	}
	return h.client.pruningReserveTokens()
}

func (h *runtimeIntegrationHost) SilentReplyToken() string {
	return agents.SilentReplyToken
}

func (h *runtimeIntegrationHost) OverflowFlushConfig() (enabled *bool, softThresholdTokens int, prompt string, systemPrompt string) {
	if !h.hasClient() {
		return nil, 0, "", ""
	}
	cfg := h.client.pruningOverflowFlushConfig()
	if cfg == nil {
		return nil, 0, "", ""
	}
	return cfg.Enabled, cfg.SoftThresholdTokens, cfg.Prompt, cfg.SystemPrompt
}

// ---- Optional Host capability: LoginHelper ----

func (h *runtimeIntegrationHost) IsLoggedIn() bool {
	if !h.hasClient() {
		return false
	}
	return h.client.IsLoggedIn()
}

func (h *runtimeIntegrationHost) SessionPortals(ctx context.Context, loginID string, agentID string) ([]integrationruntime.SessionPortalInfo, error) {
	if !h.hasClient() || h.client.UserLogin == nil || h.client.UserLogin.Bridge == nil || h.client.UserLogin.Bridge.DB == nil {
		return nil, nil
	}
	if strings.TrimSpace(loginID) == "" {
		loginID = string(h.client.UserLogin.ID)
	}
	targetAgentID := h.ResolveAgentID(agentID, h.DefaultAgentID())
	targetAgentID = h.NormalizeAgentID(targetAgentID)

	allowedShared := map[string]struct{}{}
	ups, err := h.client.UserLogin.Bridge.DB.UserPortal.GetAllForLogin(ctx, h.client.UserLogin.UserLogin)
	if err != nil {
		return nil, err
	}
	for _, up := range ups {
		if up == nil || up.Portal.Receiver != "" {
			continue
		}
		allowedShared[up.Portal.String()] = struct{}{}
	}

	portals, err := h.client.UserLogin.Bridge.DB.Portal.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]integrationruntime.SessionPortalInfo, 0, len(portals))
	for _, portal := range portals {
		if portal == nil || portal.MXID == "" {
			continue
		}
		if portal.Receiver != "" && string(portal.Receiver) != loginID {
			continue
		}
		if portal.Receiver == "" {
			if len(allowedShared) == 0 {
				continue
			}
			if _, ok := allowedShared[portal.PortalKey.String()]; !ok {
				continue
			}
		}
		meta, ok := portal.Metadata.(*PortalMetadata)
		if !ok || meta == nil || isModuleInternalRoom(meta) {
			continue
		}
		portalAgentID := h.ResolveAgentID(resolveAgentID(meta), h.DefaultAgentID())
		portalAgentID = h.NormalizeAgentID(portalAgentID)
		if portalAgentID != targetAgentID {
			continue
		}
		key := portal.PortalKey.String()
		if key == "" {
			continue
		}
		out = append(out, integrationruntime.SessionPortalInfo{Key: key, PortalKey: portal.PortalKey})
	}
	return out, nil
}

func (h *runtimeIntegrationHost) LoginDB() any {
	if !h.hasClient() {
		return nil
	}
	return h.client.bridgeDB()
}
