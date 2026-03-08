package openclaw

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"sort"
	"strings"
	"sync"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"github.com/beeper/ai-bridge/pkg/bridgeadapter"
	"github.com/beeper/ai-bridge/pkg/connector/msgconv"
	"github.com/beeper/ai-bridge/pkg/matrixevents"
	"github.com/beeper/ai-bridge/pkg/shared/jsonutil"
	"github.com/beeper/ai-bridge/pkg/shared/openclawconv"
)

type openClawManager struct {
	client *OpenClawClient

	mu        sync.RWMutex
	gateway   *gatewayWSClient
	sessions  map[string]gatewaySessionRow
	approvals *bridgeadapter.ApprovalManager[string]

	cancel context.CancelFunc
}

func newOpenClawManager(client *OpenClawClient) *openClawManager {
	return &openClawManager{
		client:    client,
		sessions:  make(map[string]gatewaySessionRow),
		approvals: bridgeadapter.NewApprovalManager[string](),
	}
}

func (m *openClawManager) Start(ctx context.Context) error {
	meta := loginMetadata(m.client.UserLogin)
	cfg := gatewayConnectConfig{
		URL:         meta.GatewayURL,
		Token:       meta.GatewayToken,
		Password:    meta.GatewayPassword,
		DeviceToken: meta.DeviceToken,
	}
	gw := newGatewayWSClient(cfg)
	deviceToken, err := gw.Connect(ctx)
	if err != nil {
		return err
	}
	if deviceToken != "" && deviceToken != meta.DeviceToken {
		meta.DeviceToken = deviceToken
		_ = m.client.UserLogin.Save(ctx)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.gateway = gw
	m.cancel = cancel
	m.mu.Unlock()
	if err = m.syncSessions(ctx); err != nil {
		return err
	}
	go m.eventLoop(runCtx, gw.Events())
	return nil
}

func (m *openClawManager) Stop() {
	m.mu.Lock()
	cancel := m.cancel
	gateway := m.gateway
	m.cancel = nil
	m.gateway = nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if gateway != nil {
		gateway.Close()
	}
}

func (m *openClawManager) syncSessions(ctx context.Context) error {
	gateway := m.gatewayClient()
	if gateway == nil {
		return errors.New("gateway client is unavailable")
	}
	sessions, err := gateway.ListSessions(ctx, openClawDefaultSessionLimit)
	if err != nil {
		return err
	}
	m.mu.Lock()
	for _, session := range sessions {
		m.sessions[session.Key] = session
	}
	m.mu.Unlock()
	for _, session := range sessions {
		m.client.UserLogin.QueueRemoteEvent(&OpenClawSessionResyncEvent{client: m.client, session: session})
	}
	meta := loginMetadata(m.client.UserLogin)
	meta.SessionsSynced = true
	meta.LastSyncAt = time.Now().UnixMilli()
	return m.client.UserLogin.Save(ctx)
}

func (m *openClawManager) gatewayClient() *gatewayWSClient {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.gateway
}

func (m *openClawManager) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	meta := portalMeta(msg.Portal)
	body := strings.TrimSpace(msg.Content.Body)
	if isOpenClawAbortCommand(body, msg.Content.MsgType, msg.Event.Type) {
		if err := m.gatewayClient().AbortRun(ctx, meta.OpenClawSessionKey, ""); err != nil {
			return nil, err
		}
		return &bridgev2.MatrixMessageResponse{Pending: false}, nil
	}

	attachments, text, err := m.buildOutboundPayload(ctx, msg)
	if err != nil {
		return nil, err
	}
	if text == "" && len(attachments) == 0 {
		return &bridgev2.MatrixMessageResponse{Pending: false}, nil
	}
	_, err = m.gatewayClient().SendMessage(
		ctx,
		meta.OpenClawSessionKey,
		text,
		attachments,
		meta.ThinkingLevel,
		meta.VerboseLevel,
		string(msg.Event.ID),
	)
	if err != nil {
		return nil, err
	}
	return &bridgev2.MatrixMessageResponse{Pending: true}, nil
}

func (m *openClawManager) buildOutboundPayload(ctx context.Context, msg *bridgev2.MatrixMessage) ([]map[string]any, string, error) {
	content := msg.Content
	msgType := content.MsgType
	if msg.Event.Type == event.EventSticker {
		msgType = event.MsgImage
	}
	switch msgType {
	case event.MsgText, event.MsgNotice, event.MsgEmote:
		return nil, strings.TrimSpace(content.Body), nil
	case event.MsgImage, event.MsgVideo, event.MsgAudio, event.MsgFile:
		mediaURL := string(content.URL)
		if mediaURL == "" && content.File != nil {
			mediaURL = string(content.File.URL)
		}
		if mediaURL == "" {
			return nil, "", errors.New("missing media URL")
		}
		encoded, mimeType, err := m.client.DownloadAndEncodeMedia(ctx, mediaURL, content.File, 50)
		if err != nil {
			return nil, "", err
		}
		if content.Info != nil && strings.TrimSpace(content.Info.MimeType) != "" {
			mimeType = strings.TrimSpace(content.Info.MimeType)
		}
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		fileName := strings.TrimSpace(content.FileName)
		if fileName == "" {
			exts, _ := mime.ExtensionsByType(mimeType)
			if len(exts) > 0 {
				fileName = "file" + exts[0]
			} else {
				fileName = "file"
			}
		}
		text := strings.TrimSpace(content.Body)
		if text == fileName {
			text = ""
		}
		return []map[string]any{{
			"type":     "file",
			"mimeType": mimeType,
			"fileName": fileName,
			"content":  encoded,
		}}, text, nil
	default:
		return nil, "", fmt.Errorf("unsupported message type %s", msgType)
	}
}

func isOpenClawAbortCommand(body string, msgType event.MessageType, evtType event.Type) bool {
	if evtType == event.EventSticker || msgType == event.MsgImage || msgType == event.MsgVideo || msgType == event.MsgAudio || msgType == event.MsgFile {
		return false
	}
	body = strings.ToLower(strings.TrimSpace(body))
	switch body {
	case "stop", "/stop", "stop run", "stop action", "please stop", "stop openclaw":
		return true
	default:
		return false
	}
}

func (m *openClawManager) ResolveApprovalDecision(ctx context.Context, portal *bridgev2.Portal, decision bridgeadapter.ApprovalDecisionPayload) error {
	pending := m.approvals.Get(strings.TrimSpace(decision.ApprovalID))
	if pending == nil {
		return bridgeadapter.ErrApprovalUnknown
	}
	if pending.Data != nil {
		data, _ := pending.Data.(string)
		if strings.TrimSpace(data) != strings.TrimSpace(portalMeta(portal).OpenClawSessionKey) {
			return bridgeadapter.ErrApprovalWrongRoom
		}
	}
	upstreamDecision := "deny"
	if decision.Approved {
		upstreamDecision = "allow-once"
		if decision.Always {
			upstreamDecision = "allow-always"
		}
	}
	if err := m.gatewayClient().ResolveApproval(ctx, decision.ApprovalID, upstreamDecision); err != nil {
		return err
	}
	m.approvals.Drop(decision.ApprovalID)
	return nil
}

func (m *openClawManager) FetchMessages(ctx context.Context, params bridgev2.FetchMessagesParams) (*bridgev2.FetchMessagesResponse, error) {
	meta := portalMeta(params.Portal)
	history, err := m.gatewayClient().RecentHistory(ctx, meta.OpenClawSessionKey, normalizeHistoryLimit(params.Count))
	if err != nil {
		return nil, err
	}
	messages := make([]map[string]any, 0, len(history.Messages))
	for _, message := range history.Messages {
		if message != nil {
			messages = append(messages, message)
		}
	}
	sort.SliceStable(messages, func(i, j int) bool {
		return extractMessageTimestamp(messages[i]).Before(extractMessageTimestamp(messages[j]))
	})
	backfill := make([]*bridgev2.BackfillMessage, 0, len(messages))
	for _, message := range messages {
		converted, sender, messageID := m.convertHistoryMessage(ctx, params.Portal, meta, message)
		if converted == nil || messageID == "" {
			continue
		}
		ts := extractMessageTimestamp(message)
		backfill = append(backfill, &bridgev2.BackfillMessage{
			ConvertedMessage: converted,
			Sender:           sender,
			ID:               messageID,
			TxnID:            networkid.TransactionID(messageID),
			Timestamp:        ts,
			StreamOrder:      ts.UnixMilli(),
		})
	}
	meta.LastHistorySyncAt = time.Now().UnixMilli()
	_ = params.Portal.Save(ctx)
	return &bridgev2.FetchMessagesResponse{
		Messages:                backfill,
		HasMore:                 false,
		Forward:                 params.Forward,
		AggressiveDeduplication: true,
		ApproxTotalCount:        len(history.Messages),
	}, nil
}

func normalizeHistoryLimit(count int) int {
	if count <= 0 || count > openClawDefaultSessionLimit {
		return openClawDefaultSessionLimit
	}
	return count
}

func (m *openClawManager) convertHistoryMessage(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, message map[string]any) (*bridgev2.ConvertedMessage, bridgev2.EventSender, networkid.MessageID) {
	role := strings.ToLower(strings.TrimSpace(stringValue(message["role"])))
	text := extractMessageText(message)
	attachmentBlocks := extractAttachmentMetadata(message)
	if role == "toolresult" && strings.TrimSpace(text) == "" {
		if details, ok := message["details"]; ok && details != nil {
			if data, err := json.Marshal(details); err == nil {
				text = string(data)
			}
		}
	}
	if strings.TrimSpace(text) == "" && len(attachmentBlocks) == 0 && role != "toolresult" {
		return nil, bridgev2.EventSender{}, ""
	}
	agentID := resolveOpenClawAgentID(meta, meta.OpenClawSessionKey, message)
	sender := m.client.senderForAgent(agentID, false)
	if role == "user" {
		sender = m.client.senderForAgent("", true)
	}
	ts := extractMessageTimestamp(message)
	messageID := historyFingerprintMessageID(meta.OpenClawSessionKey, role, ts, text, message)
	parts := make([]*bridgev2.ConvertedMessagePart, 0, 1+len(attachmentBlocks))
	uiParts := openClawHistoryUIParts(message, role)
	if strings.TrimSpace(text) != "" {
		parts = append(parts, &bridgev2.ConvertedMessagePart{
			ID:      networkid.PartID("0"),
			Type:    event.EventMessage,
			Content: &event.MessageEventContent{MsgType: event.MsgText, Body: text},
			Extra:   map[string]any{"msgtype": event.MsgText, "body": text, "m.mentions": map[string]any{}},
		})
		if len(uiParts) == 0 || strings.ToLower(strings.TrimSpace(stringValue(uiParts[0]["type"]))) != "text" {
			uiParts = append([]map[string]any{{"type": "text", "text": text, "state": "done"}}, uiParts...)
		}
	}
	for idx, block := range attachmentBlocks {
		uploaded, err := m.client.buildOpenClawAttachmentContent(ctx, portal, block)
		if err != nil {
			fallbackText := openClawAttachmentFallbackText(block, err)
			parts = append(parts, &bridgev2.ConvertedMessagePart{
				ID:      networkid.PartID(fmt.Sprintf("attachment-fallback-%d", idx)),
				Type:    event.EventMessage,
				Content: &event.MessageEventContent{MsgType: event.MsgNotice, Body: fallbackText},
				Extra:   map[string]any{"msgtype": event.MsgNotice, "body": fallbackText, "m.mentions": map[string]any{}},
			})
			uiParts = append(uiParts, map[string]any{"type": "text", "text": fallbackText, "state": "done"})
			continue
		}
		parts = append(parts, &bridgev2.ConvertedMessagePart{
			ID:      networkid.PartID(fmt.Sprintf("attachment-%d", idx)),
			Type:    event.EventMessage,
			Content: uploaded.Content,
			Extra:   uploaded.Metadata,
		})
		if uploaded.MatrixURL != "" {
			uiParts = append(uiParts, map[string]any{
				"type":      "file",
				"url":       uploaded.MatrixURL,
				"mediaType": uploaded.Content.Info.MimeType,
				"filename":  uploaded.Content.FileName,
			})
		}
	}
	converted := &bridgev2.ConvertedMessage{
		Parts: parts,
	}
	if len(converted.Parts) > 0 {
		converted.Parts[0].DBMetadata = &MessageMetadata{
			Role:        role,
			Body:        text,
			SessionID:   meta.OpenClawSessionID,
			SessionKey:  meta.OpenClawSessionKey,
			Attachments: attachmentBlocks,
		}
	}
	if role == "assistant" || role == "toolresult" {
		uiMessage := msgconv.BuildUIMessage(msgconv.UIMessageParams{
			TurnID:   string(messageID),
			Role:     "assistant",
			Metadata: msgconv.BuildUIMessageMetadata(msgconv.UIMessageMetadataParams{TurnID: string(messageID), AgentID: agentID}),
			Parts:    uiParts,
		})
		if len(converted.Parts) > 0 {
			converted.Parts[0].Extra[matrixevents.BeeperAIKey] = uiMessage
			converted.Parts[0].DBMetadata.(*MessageMetadata).CanonicalSchema = "ai-sdk-ui-message-v1"
			converted.Parts[0].DBMetadata.(*MessageMetadata).CanonicalUIMessage = uiMessage
		}
	} else if len(converted.Parts) > 0 {
		uiMessage := msgconv.BuildUIMessage(msgconv.UIMessageParams{
			TurnID:   string(messageID),
			Role:     "user",
			Metadata: msgconv.BuildUIMessageMetadata(msgconv.UIMessageMetadataParams{TurnID: string(messageID)}),
			Parts:    uiParts,
		})
		converted.Parts[0].Extra[matrixevents.BeeperAIKey] = uiMessage
	}
	return converted, sender, messageID
}

func historyFingerprintMessageID(sessionKey, role string, ts time.Time, text string, raw map[string]any) networkid.MessageID {
	hashSource := map[string]any{
		"sessionKey":  sessionKey,
		"role":        role,
		"timestamp":   ts.UnixMilli(),
		"text":        text,
		"attachments": extractAttachmentMetadata(raw),
	}
	data, _ := json.Marshal(hashSource)
	sum := sha256.Sum256(data)
	return networkid.MessageID("openclaw:" + hex.EncodeToString(sum[:12]))
}

func extractAttachmentMetadata(message map[string]any) []map[string]any {
	return openclawconv.ExtractAttachmentBlocks(message)
}

func (m *openClawManager) eventLoop(ctx context.Context, events <-chan gatewayEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			m.handleEvent(ctx, evt)
		}
	}
}

func (m *openClawManager) handleEvent(ctx context.Context, evt gatewayEvent) {
	switch evt.Name {
	case "chat":
		var payload gatewayChatEvent
		if err := json.Unmarshal(evt.Payload, &payload); err == nil {
			m.handleChatEvent(ctx, payload)
		}
	case "agent":
		var payload gatewayAgentEvent
		if err := json.Unmarshal(evt.Payload, &payload); err == nil {
			m.handleAgentEvent(ctx, payload)
		}
	case "exec.approval.requested":
		var payload gatewayApprovalRequestEvent
		if err := json.Unmarshal(evt.Payload, &payload); err == nil {
			m.handleApprovalRequest(ctx, payload)
		}
	}
}

func (m *openClawManager) handleApprovalRequest(ctx context.Context, payload gatewayApprovalRequestEvent) {
	sessionKey := strings.TrimSpace(stringValue(payload.Request["sessionKey"]))
	if sessionKey == "" {
		return
	}
	portal := m.resolvePortal(ctx, sessionKey)
	if portal == nil || portal.MXID == "" {
		return
	}
	m.approvals.Register(payload.ID, time.Until(time.UnixMilli(payload.ExpiresAtMs)), sessionKey)
	body := "Tool approval required"
	if command := strings.TrimSpace(stringValue(payload.Request["command"])); command != "" {
		body = "Tool approval required: " + command
	}
	m.client.sendApprovalRequestFallbackEvent(ctx, portal, payload.ID, body)
}

func (m *openClawManager) handleChatEvent(ctx context.Context, payload gatewayChatEvent) {
	if strings.TrimSpace(payload.SessionKey) == "" {
		return
	}
	portal := m.resolvePortal(ctx, payload.SessionKey)
	if portal == nil || portal.MXID == "" {
		return
	}
	meta := portalMeta(portal)
	agentID := resolveOpenClawAgentID(meta, payload.SessionKey, payload.Message)
	maybePersistPortalAgentID(ctx, portal, meta, agentID)
	turnID := stringsTrimDefault(payload.RunID, "openclaw:"+payload.SessionKey)
	if payload.State == "delta" {
		m.ensureStreamStart(ctx, portal, meta, turnID, payload.RunID)
		text := extractMessageText(payload.Message)
		delta := m.client.computeVisibleDelta(turnID, text)
		if delta != "" {
			m.client.EmitStreamPart(ctx, portal, turnID, agentID, payload.SessionKey, map[string]any{
				"type":  "text-delta",
				"id":    "text-" + turnID,
				"delta": delta,
			})
		}
		return
	}
	if payload.State == "final" || payload.State == "aborted" || payload.State == "error" {
		m.ensureStreamStart(ctx, portal, meta, turnID, payload.RunID)
		text := extractMessageText(payload.Message)
		if delta := m.client.computeVisibleDelta(turnID, text); delta != "" {
			m.client.EmitStreamPart(ctx, portal, turnID, agentID, payload.SessionKey, map[string]any{
				"type":  "text-delta",
				"id":    "text-" + turnID,
				"delta": delta,
			})
		}
		if payload.State == "error" {
			m.client.EmitStreamPart(ctx, portal, turnID, agentID, payload.SessionKey, map[string]any{"type": "error", "errorText": stringsTrimDefault(payload.StopReason, "OpenClaw run failed")})
		} else if payload.State == "aborted" {
			m.client.EmitStreamPart(ctx, portal, turnID, agentID, payload.SessionKey, map[string]any{"type": "abort", "reason": stringsTrimDefault(payload.StopReason, "aborted")})
		}
		m.client.EmitStreamPart(ctx, portal, turnID, agentID, payload.SessionKey, map[string]any{
			"type":            "finish",
			"messageMetadata": msgconv.BuildUIMessageMetadata(msgconv.UIMessageMetadataParams{TurnID: turnID, AgentID: agentID, FinishReason: payload.State}),
		})
		m.client.FinishStream(turnID, payload.State)
		meta.LastLiveSeq = payload.Seq
		_ = portal.Save(ctx)
	}
}

func (m *openClawManager) ensureStreamStart(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, turnID, runID string) {
	agentID := resolveOpenClawAgentID(meta, meta.OpenClawSessionKey, nil)
	m.client.EmitStreamPart(ctx, portal, turnID, agentID, meta.OpenClawSessionKey, map[string]any{
		"type":      "start",
		"messageId": turnID,
		"messageMetadata": msgconv.BuildUIMessageMetadata(msgconv.UIMessageMetadataParams{
			TurnID:       turnID,
			AgentID:      agentID,
			CompletionID: runID,
		}),
	})
}

func (m *openClawManager) handleAgentEvent(ctx context.Context, payload gatewayAgentEvent) {
	if strings.TrimSpace(payload.SessionKey) == "" {
		return
	}
	portal := m.resolvePortal(ctx, payload.SessionKey)
	if portal == nil || portal.MXID == "" {
		return
	}
	meta := portalMeta(portal)
	agentID := resolveOpenClawAgentID(meta, payload.SessionKey, payload.Data)
	maybePersistPortalAgentID(ctx, portal, meta, agentID)
	turnID := stringsTrimDefault(payload.RunID, stringsTrimDefault(payload.SourceRunID, "openclaw:"+payload.SessionKey))
	m.ensureStreamStart(ctx, portal, meta, turnID, payload.RunID)
	stream := strings.ToLower(strings.TrimSpace(payload.Stream))
	switch stream {
	case "reasoning":
		if text := stringsTrimDefault(stringValue(payload.Data["text"]), stringValue(payload.Data["delta"])); text != "" {
			m.client.EmitStreamPart(ctx, portal, turnID, agentID, payload.SessionKey, map[string]any{"type": "reasoning-delta", "id": "reasoning-" + turnID, "delta": text})
		}
	case "tool":
		toolCallID := stringsTrimDefault(stringValue(payload.Data["toolCallId"]), stringsTrimDefault(stringValue(payload.Data["toolUseId"]), stringValue(payload.Data["id"])))
		toolName := stringsTrimDefault(stringValue(payload.Data["toolName"]), stringsTrimDefault(stringValue(payload.Data["name"]), "tool"))
		if toolCallID != "" {
			if input, ok := payload.Data["input"]; ok {
				m.client.EmitStreamPart(ctx, portal, turnID, agentID, payload.SessionKey, map[string]any{"type": "tool-input-available", "toolCallId": toolCallID, "toolName": toolName, "input": input, "providerExecuted": true})
			}
			if approvalID := strings.TrimSpace(stringValue(payload.Data["approvalId"])); approvalID != "" {
				m.client.EmitStreamPart(ctx, portal, turnID, agentID, payload.SessionKey, map[string]any{"type": "tool-approval-request", "approvalId": approvalID, "toolCallId": toolCallID})
			}
			if output, ok := payload.Data["output"]; ok {
				m.client.EmitStreamPart(ctx, portal, turnID, agentID, payload.SessionKey, map[string]any{"type": "tool-output-available", "toolCallId": toolCallID, "output": output, "providerExecuted": true})
			} else if result, ok := payload.Data["result"]; ok {
				m.client.EmitStreamPart(ctx, portal, turnID, agentID, payload.SessionKey, map[string]any{"type": "tool-output-available", "toolCallId": toolCallID, "output": result, "providerExecuted": true})
			}
			if errText := strings.TrimSpace(stringValue(payload.Data["error"])); errText != "" {
				m.client.EmitStreamPart(ctx, portal, turnID, agentID, payload.SessionKey, map[string]any{"type": "tool-output-error", "toolCallId": toolCallID, "errorText": errText, "providerExecuted": true})
			}
			return
		}
		fallthrough
	default:
		m.client.EmitStreamPart(ctx, portal, turnID, agentID, payload.SessionKey, map[string]any{
			"type": "data-openclaw-" + stream,
			"id":   fmt.Sprintf("openclaw-%s-%d", stream, payload.Seq),
			"data": map[string]any{"stream": payload.Stream, "data": payload.Data},
		})
	}
}

func (m *openClawManager) resolvePortal(ctx context.Context, sessionKey string) *bridgev2.Portal {
	if strings.TrimSpace(sessionKey) == "" {
		return nil
	}
	key := m.client.portalKeyForSession(sessionKey)
	portal, err := m.client.UserLogin.Bridge.GetPortalByKey(ctx, key)
	if err == nil && portal != nil {
		return portal
	}
	m.mu.RLock()
	session, ok := m.sessions[sessionKey]
	m.mu.RUnlock()
	if !ok {
		session = gatewaySessionRow{Key: sessionKey, SessionID: sessionKey}
	}
	m.client.UserLogin.QueueRemoteEvent(&OpenClawSessionResyncEvent{client: m.client, session: session})
	portal, _ = m.client.UserLogin.Bridge.GetPortalByKey(ctx, key)
	return portal
}

func extractMessageTimestamp(message map[string]any) time.Time {
	if ts, ok := message["timestamp"].(float64); ok && ts > 0 {
		return time.UnixMilli(int64(ts))
	}
	if ts, ok := message["timestamp"].(int64); ok && ts > 0 {
		return time.UnixMilli(ts)
	}
	return time.Now()
}

func extractMessageText(message map[string]any) string {
	return openclawconv.ExtractMessageText(message)
}

func contentBlocks(message map[string]any) []map[string]any {
	return openclawconv.ContentBlocks(message)
}

func stringValue(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func openClawAttachmentFallbackText(block map[string]any, err error) string {
	name := openClawBlockFilename(block)
	if name == "" {
		name = "attachment"
	}
	if err == nil {
		return "[Attachment: " + name + "]"
	}
	return fmt.Sprintf("[Attachment unavailable: %s (%v)]", name, err)
}

func openClawHistoryUIParts(message map[string]any, role string) []map[string]any {
	if role == "toolresult" {
		toolCallID := strings.TrimSpace(stringsTrimDefault(stringValue(message["toolCallId"]), stringValue(message["toolUseId"])))
		toolName := strings.TrimSpace(stringValue(message["toolName"]))
		if toolCallID == "" {
			toolCallID = "tool-result"
		}
		part := map[string]any{
			"type":       "dynamic-tool",
			"toolCallId": toolCallID,
			"toolName":   stringsTrimDefault(toolName, "tool"),
		}
		if isError, _ := message["isError"].(bool); isError {
			part["state"] = "output-error"
			part["errorText"] = extractMessageText(message)
		} else {
			part["state"] = "output-available"
			if details, ok := message["details"]; ok && details != nil {
				part["output"] = jsonutil.DeepCloneAny(details)
			} else {
				part["output"] = extractMessageText(message)
			}
		}
		return []map[string]any{part}
	}
	blocks := contentBlocks(message)
	uiParts := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		blockType := strings.ToLower(strings.TrimSpace(stringValue(block["type"])))
		switch blockType {
		case "text", "input_text", "output_text":
			text := strings.TrimSpace(stringsTrimDefault(stringValue(block["text"]), stringValue(block["content"])))
			if text != "" {
				uiParts = append(uiParts, map[string]any{"type": "text", "text": text, "state": "done"})
			}
		case "toolcall", "tooluse", "functioncall":
			toolCallID := strings.TrimSpace(stringsTrimDefault(stringValue(block["id"]), stringValue(block["call_id"])))
			if toolCallID == "" {
				toolCallID = "tool-call"
			}
			toolName := strings.TrimSpace(stringsTrimDefault(stringValue(block["name"]), stringValue(block["toolName"])))
			input := jsonutil.ToMap(block["arguments"])
			if len(input) == 0 {
				input = jsonutil.ToMap(block["input"])
			}
			uiParts = append(uiParts, map[string]any{
				"type":       "dynamic-tool",
				"toolCallId": toolCallID,
				"toolName":   stringsTrimDefault(toolName, "tool"),
				"state":      "input-available",
				"input":      input,
			})
		}
	}
	return uiParts
}

func isOpenClawAttachmentBlock(block map[string]any) bool {
	return openclawconv.IsAttachmentBlock(block)
}

func resolveOpenClawAgentID(meta *PortalMetadata, sessionKey string, payload map[string]any) string {
	for _, key := range []string{"agentId", "agent_id", "agent"} {
		if payload != nil {
			if value := strings.TrimSpace(stringValue(payload[key])); value != "" {
				return value
			}
		}
	}
	if meta != nil && strings.TrimSpace(meta.OpenClawAgentID) != "" {
		return strings.TrimSpace(meta.OpenClawAgentID)
	}
	if value := openClawAgentIDFromSessionKey(sessionKey); value != "" {
		return value
	}
	return "gateway"
}

func maybePersistPortalAgentID(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, agentID string) {
	agentID = strings.TrimSpace(agentID)
	if portal == nil || meta == nil || agentID == "" || meta.OpenClawAgentID == agentID {
		return
	}
	meta.OpenClawAgentID = agentID
	_ = portal.Save(ctx)
}
