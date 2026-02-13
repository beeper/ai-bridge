//lint:file-ignore U1000 Hard-cut compatibility shim after invasive deletions.
package connector

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	beeperdesktopapi "github.com/beeper/desktop-api-go"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/id"
)

type ToolStrictMode string

const (
	ToolStrictModeOff ToolStrictMode = "off"
	ToolStrictModeOn  ToolStrictMode = "on"
)

func resolveToolStrictMode(openRouter bool) ToolStrictMode {
	if openRouter {
		return ToolStrictModeOff
	}
	return ToolStrictModeOn
}

type noopStateStoreBackend struct{}

func (noopStateStoreBackend) Read(context.Context, string) ([]byte, bool, error) {
	return nil, false, nil
}
func (noopStateStoreBackend) Write(context.Context, string, []byte) error { return nil }
func (noopStateStoreBackend) List(context.Context, string) ([]StateStoreEntry, error) {
	return nil, nil
}

func (oc *AIClient) bridgeStateBackend() StateStoreBackend {
	return noopStateStoreBackend{}
}

type desktopInstanceConfig struct {
	BaseURL string
}

const desktopDefaultInstance = "default"

func (oc *AIClient) desktopAPIInstanceNames() []string {
	if oc == nil || oc.UserLogin == nil || oc.UserLogin.Metadata == nil {
		return nil
	}
	meta := loginMetadata(oc.UserLogin)
	if meta == nil || meta.ServiceTokens == nil || len(meta.ServiceTokens.DesktopAPIInstances) == 0 {
		return nil
	}
	names := make([]string, 0, len(meta.ServiceTokens.DesktopAPIInstances))
	for k := range meta.ServiceTokens.DesktopAPIInstances {
		if strings.TrimSpace(k) != "" {
			names = append(names, k)
		}
	}
	return names
}

func (oc *AIClient) desktopAPIInstances() map[string]DesktopAPIInstance {
	if oc == nil || oc.UserLogin == nil || oc.UserLogin.Metadata == nil {
		return map[string]DesktopAPIInstance{}
	}
	meta := loginMetadata(oc.UserLogin)
	if meta == nil || meta.ServiceTokens == nil || len(meta.ServiceTokens.DesktopAPIInstances) == 0 {
		return map[string]DesktopAPIInstance{}
	}
	out := make(map[string]DesktopAPIInstance, len(meta.ServiceTokens.DesktopAPIInstances))
	for k, v := range meta.ServiceTokens.DesktopAPIInstances {
		out[k] = v
	}
	return out
}

func (oc *AIClient) listDesktopAccounts(context.Context, string) ([]beeperdesktopapi.Account, error) {
	return nil, nil
}

func (oc *AIClient) desktopAPIInstanceConfig(name string) (desktopInstanceConfig, bool) {
	if oc == nil || oc.UserLogin == nil || oc.UserLogin.Metadata == nil {
		return desktopInstanceConfig{}, false
	}
	meta := loginMetadata(oc.UserLogin)
	if meta == nil || meta.ServiceTokens == nil {
		return desktopInstanceConfig{}, false
	}
	cfg, ok := meta.ServiceTokens.DesktopAPIInstances[strings.TrimSpace(name)]
	if !ok {
		return desktopInstanceConfig{}, false
	}
	return desktopInstanceConfig{BaseURL: cfg.BaseURL}, true
}

func canonicalDesktopNetwork(network string) string {
	return normalizeDesktopNetworkToken(network)
}

func normalizeDesktopNetworkToken(network string) string {
	n := strings.TrimSpace(strings.ToLower(network))
	n = strings.ReplaceAll(n, " ", "_")
	n = strings.ReplaceAll(n, "-", "_")
	return n
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func appendMessageIDHint(text string, _ any) string { return text }

func stripMessageIDHintLines(text string) string { return text }

func normalizeDesktopInstanceName(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return sanitizeDesktopInstanceKey(trimmed)
}

func parsePositiveInt(value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, fmt.Errorf("value must be positive")
	}
	return n, nil
}

type MatrixRoomInfo struct {
	Name string
}

func getMatrixRoomInfo(context.Context, *BridgeToolContext) (*MatrixRoomInfo, error) {
	return nil, nil
}

func sanitizeToolSchemaWithReport(schema map[string]any) (map[string]any, []string) {
	return schema, nil
}

func logSchemaSanitization(*zerolog.Logger, string, []string) {}

func shouldUseStrictMode(mode ToolStrictMode, _ map[string]any) bool {
	return mode == ToolStrictModeOn
}

func normalizeMessageID(value string) string { return strings.TrimSpace(value) }

func fileURLToPath(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	return parsed.Path, nil
}

func resolvePromptWorkspaceDir() string { return "" }

func executeMessageRead(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("message read is not available in the simple bridge")
}

func executeMessageMemberInfo(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("message member-info is not available in the simple bridge")
}

func executeMessageChannelInfo(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("message channel-info is not available in the simple bridge")
}

func executeMessageChannelEdit(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("message channel-edit is not available in the simple bridge")
}

func executeMessageFocus(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("message focus is not available in the simple bridge")
}

func executeMessageReactions(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("message reactions is not available in the simple bridge")
}

func executeMessageReactRemove(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("message reaction removal is not available in the simple bridge")
}

func executeMessageDesktopListChats(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("desktop list-chats is not available in the simple bridge")
}

func executeMessageDesktopSearchChats(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("desktop search-chats is not available in the simple bridge")
}

func executeMessageDesktopSearchMessages(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("desktop search-messages is not available in the simple bridge")
}

func executeMessageDesktopCreateChat(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("desktop create-chat is not available in the simple bridge")
}

func executeMessageDesktopArchiveChat(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("desktop archive-chat is not available in the simple bridge")
}

func executeMessageDesktopSetReminder(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("desktop set-reminder is not available in the simple bridge")
}

func executeMessageDesktopClearReminder(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("desktop clear-reminder is not available in the simple bridge")
}

func executeMessageDesktopUploadAsset(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("desktop upload-asset is not available in the simple bridge")
}

func executeMessageDesktopDownloadAsset(context.Context, map[string]any, *BridgeToolContext) (string, error) {
	return "", errors.New("desktop download-asset is not available in the simple bridge")
}

func maybeExecuteMessageSendDesktop(context.Context, map[string]any, *BridgeToolContext) (bool, string, error) {
	return false, "", nil
}

func maybeExecuteMessageEditDesktop(context.Context, map[string]any, *BridgeToolContext) (bool, string, error) {
	return false, "", nil
}

func maybeExecuteMessageReplyDesktop(context.Context, map[string]any, *BridgeToolContext) (bool, string, error) {
	return false, "", nil
}

func maybeExecuteMessageSearchDesktop(context.Context, map[string]any, *BridgeToolContext) (bool, string, error) {
	return false, "", nil
}

func executeWebFetchWithProviders(context.Context, map[string]any) (string, error) {
	return "", errors.New("web_fetch is not available in the simple bridge")
}

func executeWebSearchWithProviders(context.Context, map[string]any) (string, error) {
	return "", errors.New("web_search provider stack is unavailable")
}

func (oc *AIClient) sendReaction(context.Context, *bridgev2.Portal, id.EventID, string) {}

func sendFormattedMessage(context.Context, *BridgeToolContext, string, map[string]any, string) (id.EventID, error) {
	return "", errors.New("message sending helpers are not available in the simple bridge")
}

func getPinnedEventIDs(context.Context, *BridgeToolContext) []string { return nil }

func isAllowedValue(value string, allowed map[string]bool) bool {
	_, ok := allowed[strings.TrimSpace(value)]
	return ok
}
