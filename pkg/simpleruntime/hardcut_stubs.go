//lint:file-ignore U1000 Hard-cut compatibility shim after invasive deletions.
package connector

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

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

func appendMessageIDHint(text string, _ any) string { return text }

func stripMessageIDHintLines(text string) string { return text }

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
