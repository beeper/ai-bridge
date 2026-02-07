package connector

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/beeper/ai-bridge/pkg/openclawws"
)

type openclawNodesConfig struct {
	GatewayURL string
	Token      string
	Timeout    time.Duration
}

func (oc *AIClient) openclawNodesConfig() (openclawNodesConfig, bool, string) {
	if oc == nil || oc.connector == nil {
		return openclawNodesConfig{}, false, "missing bridge context"
	}
	cfg := oc.connector.Config.Tools.OpenClaw
	if cfg == nil || cfg.Enabled == nil || !*cfg.Enabled {
		return openclawNodesConfig{}, false, "OpenClaw nodes integration is disabled (tools.openclaw.enabled=false)"
	}
	url := strings.TrimSpace(cfg.GatewayURL)
	if url == "" {
		return openclawNodesConfig{}, false, "OpenClaw gateway URL is missing (tools.openclaw.gateway_url)"
	}
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return openclawNodesConfig{}, false, "OpenClaw gateway token is missing (tools.openclaw.token)"
	}
	timeout := 30 * time.Second
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	return openclawNodesConfig{
		GatewayURL: url,
		Token:      token,
		Timeout:    timeout,
	}, true, ""
}

func stableNodeKey(value string) string {
	s := strings.ToLower(strings.TrimSpace(value))
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	lastDash := false
	for _, r := range s {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	return out
}

type nodeListNode struct {
	NodeID      string   `json:"nodeId"`
	DisplayName string   `json:"displayName,omitempty"`
	RemoteIP    string   `json:"remoteIp,omitempty"`
	Commands    []string `json:"commands,omitempty"`
	Connected   bool     `json:"connected,omitempty"`
}

func resolveNodeIDFromList(nodes []nodeListNode, query string, allowPrefix bool) (string, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return "", fmt.Errorf("node required")
	}
	qNorm := stableNodeKey(q)
	matches := make([]nodeListNode, 0, 2)
	for _, n := range nodes {
		if n.NodeID == q {
			matches = append(matches, n)
			continue
		}
		if n.RemoteIP != "" && n.RemoteIP == q {
			matches = append(matches, n)
			continue
		}
		if n.DisplayName != "" && stableNodeKey(n.DisplayName) == qNorm {
			matches = append(matches, n)
			continue
		}
		if allowPrefix && len(q) >= 6 && strings.HasPrefix(n.NodeID, q) {
			matches = append(matches, n)
			continue
		}
	}
	if len(matches) == 1 {
		return matches[0].NodeID, nil
	}
	if len(matches) == 0 {
		known := make([]string, 0, len(nodes))
		for _, n := range nodes {
			label := strings.TrimSpace(n.DisplayName)
			if label == "" {
				label = strings.TrimSpace(n.RemoteIP)
			}
			if label == "" {
				label = n.NodeID
			}
			if label != "" {
				known = append(known, label)
			}
		}
		sort.Strings(known)
		if len(known) > 0 {
			return "", fmt.Errorf("unknown node: %s (known: %s)", q, strings.Join(known, ", "))
		}
		return "", fmt.Errorf("unknown node: %s", q)
	}
	labels := make([]string, 0, len(matches))
	for _, n := range matches {
		label := strings.TrimSpace(n.DisplayName)
		if label == "" {
			label = strings.TrimSpace(n.RemoteIP)
		}
		if label == "" {
			label = n.NodeID
		}
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return "", fmt.Errorf("ambiguous node: %s (matches: %s)", q, strings.Join(labels, ", "))
}

func parseEnvPairs(raw any) map[string]string {
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	env := map[string]string{}
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			continue
		}
		idx := strings.IndexByte(s, '=')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(s[:idx])
		if key == "" {
			continue
		}
		env[key] = s[idx+1:]
	}
	if len(env) == 0 {
		return nil
	}
	return env
}

func parseTimeoutMs(raw any) *int {
	switch v := raw.(type) {
	case nil:
		return nil
	case float64:
		if v <= 0 || v != v {
			return nil
		}
		i := int(v)
		return &i
	case int:
		if v <= 0 {
			return nil
		}
		i := v
		return &i
	case string:
		t := strings.TrimSpace(v)
		if t == "" {
			return nil
		}
		var n int
		_, err := fmt.Sscanf(t, "%d", &n)
		if err != nil || n <= 0 {
			return nil
		}
		return &n
	default:
		return nil
	}
}

func executeNodes(ctx context.Context, args map[string]any) (string, error) {
	btc := GetBridgeToolContext(ctx)
	if btc == nil || btc.Client == nil {
		return "", fmt.Errorf("missing bridge context")
	}
	client := btc.Client

	cfg, ok, reason := client.openclawNodesConfig()
	if !ok {
		return "", fmt.Errorf("%s", reason)
	}

	action, _ := args["action"].(string)
	action = strings.TrimSpace(action)
	if action == "" {
		return "", fmt.Errorf("action is required")
	}

	timeout := cfg.Timeout
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	gw, err := openclawws.DialAndConnect(callCtx, cfg.GatewayURL, cfg.Token, timeout)
	if err != nil {
		return "", err
	}
	defer gw.Close()

	switch action {
	case "status":
		out, err := gw.Call(callCtx, "node.list", map[string]any{})
		if err != nil {
			return "", err
		}
		return string(out), nil
	case "describe": {
		node, _ := args["node"].(string)
		node = strings.TrimSpace(node)
		if node == "" {
			return "", fmt.Errorf("node is required")
		}
		// Resolve by list first (supports displayName/ip/prefix).
		listRaw, err := gw.Call(callCtx, "node.list", map[string]any{})
		if err != nil {
			return "", err
		}
		var listObj struct {
			Nodes []nodeListNode `json:"nodes"`
		}
		_ = json.Unmarshal(listRaw, &listObj)
		nodeID, err := resolveNodeIDFromList(listObj.Nodes, node, true)
		if err != nil {
			return "", err
		}
		out, err := gw.Call(callCtx, "node.describe", map[string]any{"nodeId": nodeID})
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	case "pending":
		out, err := gw.Call(callCtx, "node.pair.list", map[string]any{})
		if err != nil {
			return "", err
		}
		return string(out), nil
	case "approve", "reject": {
		reqID, _ := args["requestId"].(string)
		reqID = strings.TrimSpace(reqID)
		if reqID == "" {
			return "", fmt.Errorf("requestId is required")
		}
		method := "node.pair.approve"
		if action == "reject" {
			method = "node.pair.reject"
		}
		out, err := gw.Call(callCtx, method, map[string]any{"requestId": reqID})
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	case "notify": {
		node, _ := args["node"].(string)
		node = strings.TrimSpace(node)
		if node == "" {
			return "", fmt.Errorf("node is required")
		}
		title, _ := args["title"].(string)
		body, _ := args["body"].(string)
		title = strings.TrimSpace(title)
		body = strings.TrimSpace(body)
		if title == "" && body == "" {
			return "", fmt.Errorf("title or body required")
		}

		listRaw, err := gw.Call(callCtx, "node.list", map[string]any{})
		if err != nil {
			return "", err
		}
		var listObj struct{ Nodes []nodeListNode `json:"nodes"` }
		_ = json.Unmarshal(listRaw, &listObj)
		nodeID, err := resolveNodeIDFromList(listObj.Nodes, node, true)
		if err != nil {
			return "", err
		}

		priority, _ := args["priority"].(string)
		delivery, _ := args["delivery"].(string)
		sound, _ := args["sound"].(string)
		params := map[string]any{
			"title": title,
			"body":  body,
		}
		if strings.TrimSpace(sound) != "" {
			params["sound"] = strings.TrimSpace(sound)
		}
		if strings.TrimSpace(priority) != "" {
			params["priority"] = strings.TrimSpace(priority)
		}
		if strings.TrimSpace(delivery) != "" {
			params["delivery"] = strings.TrimSpace(delivery)
		}

		_, err = gw.Call(callCtx, "node.invoke", map[string]any{
			"nodeId":         nodeID,
			"command":        "system.notify",
			"params":         params,
			"idempotencyKey": uuid.NewString(),
		})
		if err != nil {
			return "", err
		}
		return `{"ok":true}`, nil
	}
	case "run": {
		node, _ := args["node"].(string)
		node = strings.TrimSpace(node)
		if node == "" {
			return "", fmt.Errorf("node is required")
		}
		cmdRaw, ok := args["command"].([]any)
		if !ok || len(cmdRaw) == 0 {
			return "", fmt.Errorf("command is required (argv array)")
		}
		command := make([]string, 0, len(cmdRaw))
		for _, part := range cmdRaw {
			command = append(command, fmt.Sprint(part))
		}

		listRaw, err := gw.Call(callCtx, "node.list", map[string]any{})
		if err != nil {
			return "", err
		}
		var listObj struct{ Nodes []nodeListNode `json:"nodes"` }
		_ = json.Unmarshal(listRaw, &listObj)
		if len(listObj.Nodes) == 0 {
			return "", fmt.Errorf("no nodes available (pair a node host or companion app)")
		}
		nodeID, err := resolveNodeIDFromList(listObj.Nodes, node, true)
		if err != nil {
			return "", err
		}
		var nodeInfo *nodeListNode
		for i := range listObj.Nodes {
			if listObj.Nodes[i].NodeID == nodeID {
				nodeInfo = &listObj.Nodes[i]
				break
			}
		}
		supportsSystemRun := false
		if nodeInfo != nil {
			for _, c := range nodeInfo.Commands {
				if c == "system.run" {
					supportsSystemRun = true
					break
				}
			}
		}
		if !supportsSystemRun {
			return "", fmt.Errorf("selected node does not support system.run")
		}

		cwd, _ := args["cwd"].(string)
		cwd = strings.TrimSpace(cwd)
		env := parseEnvPairs(args["env"])

		commandTimeoutMs := parseTimeoutMs(args["commandTimeoutMs"])
		invokeTimeoutMs := parseTimeoutMs(args["invokeTimeoutMs"])
		needsScreenRecording, _ := args["needsScreenRecording"].(bool)

		params := map[string]any{
			"command": command,
		}
		if cwd != "" {
			params["cwd"] = cwd
		}
		if env != nil {
			params["env"] = env
		}
		if commandTimeoutMs != nil {
			params["timeoutMs"] = *commandTimeoutMs
		}
		// Helpful audit hints (best-effort; node host may ignore).
		params["agentId"] = resolveAgentID(btc.Meta)
		params["sessionKey"] = "" // ai-bridge doesn't have OpenClaw session keys
		if needsScreenRecording {
			params["needsScreenRecording"] = true
		}

		callParams := map[string]any{
			"nodeId":         nodeID,
			"command":        "system.run",
			"params":         params,
			"idempotencyKey": uuid.NewString(),
		}
		if invokeTimeoutMs != nil {
			callParams["timeoutMs"] = *invokeTimeoutMs
		}

		out, err := gw.Call(callCtx, "node.invoke", callParams)
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	case "invoke": {
		node, _ := args["node"].(string)
		node = strings.TrimSpace(node)
		if node == "" {
			return "", fmt.Errorf("node is required")
		}
		invokeCommand, _ := args["invokeCommand"].(string)
		invokeCommand = strings.TrimSpace(invokeCommand)
		if invokeCommand == "" {
			return "", fmt.Errorf("invokeCommand is required")
		}
		invokeParamsJSON, _ := args["invokeParamsJson"].(string)
		invokeParamsJSON = strings.TrimSpace(invokeParamsJSON)
		var invokeParams any = map[string]any{}
		if invokeParamsJSON != "" {
			if err := json.Unmarshal([]byte(invokeParamsJSON), &invokeParams); err != nil {
				return "", fmt.Errorf("invokeParamsJson must be valid JSON: %v", err)
			}
		}
		invokeTimeoutMs := parseTimeoutMs(args["invokeTimeoutMs"])

		listRaw, err := gw.Call(callCtx, "node.list", map[string]any{})
		if err != nil {
			return "", err
		}
		var listObj struct{ Nodes []nodeListNode `json:"nodes"` }
		_ = json.Unmarshal(listRaw, &listObj)
		nodeID, err := resolveNodeIDFromList(listObj.Nodes, node, true)
		if err != nil {
			return "", err
		}

		callParams := map[string]any{
			"nodeId":         nodeID,
			"command":        invokeCommand,
			"params":         invokeParams,
			"idempotencyKey": uuid.NewString(),
		}
		if invokeTimeoutMs != nil {
			callParams["timeoutMs"] = *invokeTimeoutMs
		}
		out, err := gw.Call(callCtx, "node.invoke", callParams)
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	default:
		return "", fmt.Errorf("unknown nodes action: %s", action)
	}
}

func hashToken(token string) string {
	h := sha1.Sum([]byte(token))
	return hex.EncodeToString(h[:])
}

// openclawNodesDebugString is used for logs only (avoid leaking tokens).
func (oc *AIClient) openclawNodesDebugString() string {
	cfg := oc.connector.Config.Tools.OpenClaw
	if cfg == nil {
		return "openclaw:none"
	}
	return fmt.Sprintf("openclaw:url=%s token=%s os=%s arch=%s", strings.TrimSpace(cfg.GatewayURL), hashToken(cfg.Token), runtime.GOOS, runtime.GOARCH)
}
