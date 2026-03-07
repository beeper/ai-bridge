package connector

import (
	"context"
	"fmt"
	"strings"

	"maunium.net/go/mautrix/bridgev2"

	integrationruntime "github.com/beeper/ai-bridge/pkg/integrations/runtime"
)

// ---- Core Host sub-adapters ----

type hostStoreBackend struct {
	backend *lazyStoreBackend
}

func (s *hostStoreBackend) Read(ctx context.Context, key string) ([]byte, bool, error) {
	if s == nil || s.backend == nil {
		return nil, false, fmt.Errorf("store not available")
	}
	return s.backend.Read(ctx, key)
}

func (s *hostStoreBackend) Write(ctx context.Context, key string, data []byte) error {
	if s == nil || s.backend == nil {
		return fmt.Errorf("store not available")
	}
	return s.backend.Write(ctx, key, data)
}

func (s *hostStoreBackend) List(ctx context.Context, prefix string) ([]integrationruntime.StoreEntry, error) {
	if s == nil || s.backend == nil {
		return nil, fmt.Errorf("store not available")
	}
	entries, err := s.backend.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]integrationruntime.StoreEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, integrationruntime.StoreEntry{Key: e.Key, Data: e.Data})
	}
	return out, nil
}

type hostPortalResolver struct {
	client *AIClient
}

func (r *hostPortalResolver) ResolvePortalByRoomID(ctx context.Context, roomID string) any {
	if r == nil || r.client == nil || strings.TrimSpace(roomID) == "" {
		return nil
	}
	return r.client.portalByRoomID(ctx, portalRoomIDFromString(roomID))
}

func (r *hostPortalResolver) ResolveDefaultPortal(ctx context.Context) any {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.defaultChatPortal()
}

func (r *hostPortalResolver) ResolveLastActivePortal(ctx context.Context, agentID string) any {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.lastActivePortal(agentID)
}

type hostDispatch struct {
	client *AIClient
}

func (d *hostDispatch) DispatchInternalMessage(ctx context.Context, portal any, meta any, message string, source string) error {
	if d == nil || d.client == nil {
		return fmt.Errorf("missing client")
	}
	p, _ := portal.(*bridgev2.Portal)
	if p == nil {
		return fmt.Errorf("missing portal")
	}
	m, _ := meta.(*PortalMetadata)
	if m == nil {
		m = &PortalMetadata{}
	}
	_, _, err := d.client.dispatchInternalMessage(ctx, p, m, message, source, false)
	return err
}

func (d *hostDispatch) SendAssistantMessage(ctx context.Context, portal any, body string) error {
	if d == nil || d.client == nil {
		return fmt.Errorf("missing client")
	}
	p, _ := portal.(*bridgev2.Portal)
	if p == nil {
		return fmt.Errorf("missing portal")
	}
	return d.client.sendPlainAssistantMessageWithResult(ctx, p, body)
}

type hostSessionStore struct {
	client *AIClient
}

func (s *hostSessionStore) Update(ctx context.Context, key string, updater func(raw map[string]any) map[string]any) {
	if s == nil || s.client == nil || updater == nil {
		return
	}
	backend := s.client.bridgeStateBackend()
	if backend == nil {
		return
	}
	updateSessionStoreEntry(ctx, backend, key, updater)
}

type hostHeartbeat struct {
	client *AIClient
}

func (hb *hostHeartbeat) RequestNow(ctx context.Context, reason string) {
	if hb == nil || hb.client == nil || hb.client.heartbeatWake == nil {
		return
	}
	hb.client.heartbeatWake.Request(reason, 0)
}

type hostToolExec struct {
	client *AIClient
}

func (t *hostToolExec) ToolDefinitionByName(name string) (integrationruntime.ToolDefinition, bool) {
	for _, def := range BuiltinTools() {
		if def.Name == name {
			return def, true
		}
	}
	return integrationruntime.ToolDefinition{}, false
}

func (t *hostToolExec) ExecuteBuiltinTool(ctx context.Context, scope integrationruntime.ToolScope, name string, rawArgsJSON string) (string, error) {
	if t == nil || t.client == nil {
		return "", fmt.Errorf("missing client")
	}
	portal, _ := scope.Portal.(*bridgev2.Portal)
	return t.client.executeBuiltinTool(ctx, portal, name, rawArgsJSON)
}

type hostPromptContext struct{}

func (p *hostPromptContext) ResolveWorkspaceDir() string {
	return resolvePromptWorkspaceDir()
}

type hostDBAccess struct {
	client *AIClient
}

func (d *hostDBAccess) BridgeDB() any {
	if d == nil || d.client == nil {
		return nil
	}
	return d.client.bridgeDB()
}

func (d *hostDBAccess) BridgeID() string {
	if d == nil || d.client == nil || d.client.UserLogin == nil || d.client.UserLogin.Bridge == nil || d.client.UserLogin.Bridge.DB == nil {
		return ""
	}
	return string(d.client.UserLogin.Bridge.DB.BridgeID)
}

func (d *hostDBAccess) LoginID() string {
	if d == nil || d.client == nil || d.client.UserLogin == nil {
		return ""
	}
	return string(d.client.UserLogin.ID)
}

// ---- Logger ----

type runtimeLogger struct {
	client *AIClient
}

func (l *runtimeLogger) emit(level string, msg string, fields map[string]any) {
	if l == nil || l.client == nil {
		return
	}
	logger := l.client.log.With().Fields(fields).Logger()
	switch level {
	case "debug":
		logger.Debug().Msg(msg)
	case "info":
		logger.Info().Msg(msg)
	case "warn":
		logger.Warn().Msg(msg)
	case "error":
		logger.Error().Msg(msg)
	}
}

func (l *runtimeLogger) Debug(msg string, fields map[string]any) { l.emit("debug", msg, fields) }
func (l *runtimeLogger) Info(msg string, fields map[string]any)  { l.emit("info", msg, fields) }
func (l *runtimeLogger) Warn(msg string, fields map[string]any)  { l.emit("warn", msg, fields) }
func (l *runtimeLogger) Error(msg string, fields map[string]any) { l.emit("error", msg, fields) }
