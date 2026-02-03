package connector

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go/v3"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/id"

	"github.com/beeper/ai-bridge/pkg/agents"
	"github.com/beeper/ai-bridge/pkg/cron"
	"github.com/beeper/ai-bridge/pkg/textfs"
)

type heartbeatAgentState struct {
	agentID    string
	heartbeat  *HeartbeatConfig
	intervalMs int64
	lastRunMs  int64
	nextDueMs  int64
}

type HeartbeatRunner struct {
	client  *AIClient
	agents  map[string]*heartbeatAgentState
	timer   *time.Timer
	stopped bool
	mu      sync.Mutex
}

func NewHeartbeatRunner(client *AIClient) *HeartbeatRunner {
	return &HeartbeatRunner{
		client: client,
		agents: make(map[string]*heartbeatAgentState),
	}
}

func (r *HeartbeatRunner) Start() {
	if r == nil || r.client == nil {
		return
	}
	r.updateConfig(&r.client.connector.Config)
	if r.client.heartbeatWake != nil {
		r.client.heartbeatWake.SetHandler(func(reason string) cron.HeartbeatRunResult {
			return r.run(reason)
		})
	}
}

func (r *HeartbeatRunner) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.stopped = true
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	r.mu.Unlock()
	if r.client != nil && r.client.heartbeatWake != nil {
		r.client.heartbeatWake.SetHandler(nil)
	}
}

func (r *HeartbeatRunner) updateConfig(cfg *Config) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return
	}
	now := time.Now().UnixMilli()
	prev := r.agents
	next := make(map[string]*heartbeatAgentState)
	intervals := make([]int64, 0)
	for _, agent := range resolveHeartbeatAgents(cfg) {
		intervalMs := resolveHeartbeatIntervalMs(cfg, "", agent.heartbeat)
		if intervalMs <= 0 {
			continue
		}
		intervals = append(intervals, intervalMs)
		prevState := prev[agent.agentID]
		nextDue := now + intervalMs
		lastRun := int64(0)
		if prevState != nil {
			lastRun = prevState.lastRunMs
			if prevState.lastRunMs > 0 {
				nextDue = prevState.lastRunMs + intervalMs
			} else if prevState.intervalMs == intervalMs && prevState.nextDueMs > now {
				nextDue = prevState.nextDueMs
			}
		}
		next[agent.agentID] = &heartbeatAgentState{
			agentID:    agent.agentID,
			heartbeat:  agent.heartbeat,
			intervalMs: intervalMs,
			lastRunMs:  lastRun,
			nextDueMs:  nextDue,
		}
	}
	r.agents = next
	r.scheduleNextLocked()
}

func (r *HeartbeatRunner) scheduleNextLocked() {
	if r.stopped {
		return
	}
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	if len(r.agents) == 0 || r.client == nil || r.client.heartbeatWake == nil {
		return
	}
	now := time.Now().UnixMilli()
	nextDue := int64(0)
	for _, agent := range r.agents {
		if nextDue == 0 || agent.nextDueMs < nextDue {
			nextDue = agent.nextDueMs
		}
	}
	if nextDue == 0 {
		return
	}
	delay := nextDue - now
	if delay < 0 {
		delay = 0
	}
	r.timer = time.AfterFunc(time.Duration(delay)*time.Millisecond, func() {
		r.client.heartbeatWake.Request("interval", 0)
	})
}

func (r *HeartbeatRunner) run(reason string) cron.HeartbeatRunResult {
	r.mu.Lock()
	if r.stopped || len(r.agents) == 0 {
		r.mu.Unlock()
		return cron.HeartbeatRunResult{Status: "skipped", Reason: "disabled"}
	}
	agents := make([]*heartbeatAgentState, 0, len(r.agents))
	for _, agent := range r.agents {
		agents = append(agents, agent)
	}
	r.mu.Unlock()

	now := time.Now().UnixMilli()
	isInterval := reason == "interval"
	ran := false
	for _, agent := range agents {
		if isInterval && now < agent.nextDueMs {
			continue
		}
		res := r.client.runHeartbeatOnce(agent.agentID, agent.heartbeat, reason)
		if res.Status == "skipped" && res.Reason == "requests-in-flight" {
			return res
		}
		if res.Status != "skipped" || res.Reason != "disabled" {
			r.mu.Lock()
			agent.lastRunMs = now
			agent.nextDueMs = now + agent.intervalMs
			r.mu.Unlock()
		}
		if res.Status == "ran" {
			ran = true
		}
	}
	r.mu.Lock()
	r.scheduleNextLocked()
	r.mu.Unlock()
	if ran {
		return cron.HeartbeatRunResult{Status: "ran"}
	}
	if isInterval {
		return cron.HeartbeatRunResult{Status: "skipped", Reason: "not-due"}
	}
	return cron.HeartbeatRunResult{Status: "skipped", Reason: "disabled"}
}

type heartbeatAgent struct {
	agentID   string
	heartbeat *HeartbeatConfig
}

func resolveHeartbeatAgents(cfg *Config) []heartbeatAgent {
	list := []heartbeatAgent{}
	if cfg == nil {
		return list
	}
	if hasExplicitHeartbeatAgents(cfg) {
		for _, entry := range cfg.Agents.List {
			if entry.Heartbeat == nil {
				continue
			}
			id := normalizeAgentID(entry.ID)
			if id == "" {
				continue
			}
			list = append(list, heartbeatAgent{agentID: id, heartbeat: resolveHeartbeatConfig(cfg, id)})
		}
		return list
	}
	list = append(list, heartbeatAgent{agentID: normalizeAgentID(agents.DefaultAgentID), heartbeat: resolveHeartbeatConfig(cfg, agents.DefaultAgentID)})
	return list
}

func (oc *AIClient) runHeartbeatOnce(agentID string, heartbeat *HeartbeatConfig, reason string) cron.HeartbeatRunResult {
	if oc == nil || oc.connector == nil {
		return cron.HeartbeatRunResult{Status: "skipped", Reason: "disabled"}
	}
	cfg := &oc.connector.Config
	if !isHeartbeatEnabledForAgent(cfg, agentID) {
		return cron.HeartbeatRunResult{Status: "skipped", Reason: "disabled"}
	}
	if resolveHeartbeatIntervalMs(cfg, "", heartbeat) <= 0 {
		return cron.HeartbeatRunResult{Status: "skipped", Reason: "disabled"}
	}

	now := time.Now().UnixMilli()
	if !isWithinActiveHours(oc, heartbeat, now) {
		return cron.HeartbeatRunResult{Status: "skipped", Reason: "quiet-hours"}
	}

	sessionPortal, sessionKey, err := oc.resolveHeartbeatPortal(agentID, heartbeat)
	if err != nil || sessionPortal == nil || sessionPortal.MXID == "" {
		reason := "no-session"
		if errors.Is(err, errHeartbeatNoTarget) || errors.Is(err, errHeartbeatTargetNotFound) {
			reason = "no-target"
		}
		return cron.HeartbeatRunResult{Status: "skipped", Reason: reason}
	}
	if oc.isRoomBusy(sessionPortal.MXID) {
		return cron.HeartbeatRunResult{Status: "skipped", Reason: "requests-in-flight"}
	}

	// Skip when HEARTBEAT.md exists but is effectively empty.
	if !oc.shouldRunHeartbeatForFile(agentID, reason) {
		emitHeartbeatEvent(&HeartbeatEventPayload{
			TS:     time.Now().UnixMilli(),
			Status: "skipped",
			Reason: "empty-heartbeat-file",
		})
		return cron.HeartbeatRunResult{Status: "skipped", Reason: "empty-heartbeat-file"}
	}

	visibility := resolveHeartbeatVisibility(cfg, "matrix")
	hbCfg := &HeartbeatRunConfig{
		Reason:           reason,
		AckMaxChars:      resolveHeartbeatAckMaxChars(cfg, heartbeat),
		ShowOk:           visibility.ShowOk,
		ShowAlerts:       visibility.ShowAlerts,
		UseIndicator:     visibility.UseIndicator,
		IncludeReasoning: heartbeat != nil && heartbeat.IncludeReasoning != nil && *heartbeat.IncludeReasoning,
		TargetRoom:       sessionPortal.MXID,
		AgentID:          agentID,
		Channel:          "matrix",
		SuppressSave:     true,
	}
	var agentDef *agents.AgentDefinition
	store := NewAgentStoreAdapter(oc)
	if agent, err := store.GetAgentByID(context.Background(), agentID); err == nil {
		agentDef = agent
	}
	prompt := resolveHeartbeatPrompt(cfg, heartbeat, agentDef)
	systemEvents := formatSystemEvents(drainSystemEventEntries(sessionKey))
	if systemEvents != "" {
		prompt = systemEvents + "\n\n" + prompt
	}

	promptMessages, err := oc.buildPromptWithHeartbeat(context.Background(), sessionPortal, portalMeta(sessionPortal), prompt)
	if err != nil {
		return cron.HeartbeatRunResult{Status: "failed", Reason: err.Error()}
	}

	resultCh := make(chan HeartbeatRunOutcome, 1)
	runCtx := withHeartbeatRun(oc.backgroundContext(context.Background()), hbCfg, resultCh)
	done := make(chan struct{})
	go func() {
		oc.streamingResponseWithRetry(runCtx, nil, sessionPortal, portalMeta(sessionPortal), promptMessages)
		close(done)
	}()

	select {
	case res := <-resultCh:
		return cron.HeartbeatRunResult{Status: res.Status, Reason: res.Reason}
	case <-done:
		return cron.HeartbeatRunResult{Status: "failed", Reason: "heartbeat failed"}
	case <-time.After(2 * time.Minute):
		return cron.HeartbeatRunResult{Status: "failed", Reason: "heartbeat timed out"}
	}
}

func (oc *AIClient) buildPromptWithHeartbeat(ctx context.Context, portal *bridgev2.Portal, meta *PortalMetadata, prompt string) ([]openai.ChatCompletionMessageParamUnion, error) {
	base, err := oc.buildBasePrompt(ctx, portal, meta)
	if err != nil {
		return nil, err
	}
	base = oc.injectMemoryContext(ctx, portal, meta, base)
	message := appendMessageIDHint(prompt, "")
	return append(base, openai.UserMessage(message)), nil
}

var (
	errHeartbeatNoTarget       = errors.New("heartbeat target disabled")
	errHeartbeatTargetNotFound = errors.New("heartbeat target not found")
	errHeartbeatNoSession      = errors.New("heartbeat session not available")
)

func (oc *AIClient) resolveHeartbeatSessionPortal(agentID string, heartbeat *HeartbeatConfig) (*bridgev2.Portal, string, error) {
	session := ""
	if heartbeat != nil {
		session = strings.TrimSpace(heartbeat.Session)
	}
	if session == "" {
		portal := oc.lastActivePortal(agentID)
		if portal != nil {
			return portal, portal.MXID.String(), nil
		}
		if portal = oc.defaultChatPortal(); portal != nil {
			return portal, portal.MXID.String(), nil
		}
		return nil, "", fmt.Errorf("no session")
	}
	if strings.EqualFold(session, "main") {
		if portal := oc.defaultChatPortal(); portal != nil {
			return portal, portal.MXID.String(), nil
		}
	}
	if strings.HasPrefix(session, "!") {
		if portal, err := oc.UserLogin.Bridge.GetPortalByMXID(context.Background(), id.RoomID(session)); err == nil && portal != nil {
			return portal, portal.MXID.String(), nil
		}
	}
	if portal := oc.lastActivePortal(agentID); portal != nil {
		return portal, portal.MXID.String(), nil
	}
	return nil, "", fmt.Errorf("no session")
}

func (oc *AIClient) resolveHeartbeatPortal(agentID string, heartbeat *HeartbeatConfig) (*bridgev2.Portal, string, error) {
	if oc == nil || oc.UserLogin == nil {
		return nil, "", errHeartbeatNoSession
	}
	cfg := &oc.connector.Config
	target := resolveHeartbeatTarget(cfg, heartbeat)
	if strings.EqualFold(strings.TrimSpace(target), "none") {
		return nil, "", errHeartbeatNoTarget
	}
	if heartbeat != nil {
		if strings.TrimSpace(heartbeat.To) != "" {
			room := strings.TrimSpace(heartbeat.To)
			if strings.HasPrefix(room, "!") {
				if portal, err := oc.UserLogin.Bridge.GetPortalByMXID(context.Background(), id.RoomID(room)); err == nil && portal != nil {
					return portal, portal.MXID.String(), nil
				}
			}
			return nil, "", errHeartbeatTargetNotFound
		}
	}
	trimmedTarget := strings.TrimSpace(target)
	if trimmedTarget != "" && !strings.EqualFold(trimmedTarget, "last") {
		if strings.HasPrefix(trimmedTarget, "!") {
			if portal, err := oc.UserLogin.Bridge.GetPortalByMXID(context.Background(), id.RoomID(trimmedTarget)); err == nil && portal != nil {
				return portal, portal.MXID.String(), nil
			}
		}
		return nil, "", errHeartbeatTargetNotFound
	}
	portal, key, err := oc.resolveHeartbeatSessionPortal(agentID, heartbeat)
	if err != nil || portal == nil || portal.MXID == "" {
		return nil, "", errHeartbeatNoSession
	}
	return portal, key, nil
}

func (oc *AIClient) resolveHeartbeatDeliveryPortal(agentID string, heartbeat *HeartbeatConfig) (*bridgev2.Portal, id.RoomID) {
	if heartbeat != nil {
		if strings.TrimSpace(heartbeat.Target) == "none" {
			return nil, ""
		}
		if strings.TrimSpace(heartbeat.To) != "" {
			if portal, err := oc.UserLogin.Bridge.GetPortalByMXID(context.Background(), id.RoomID(strings.TrimSpace(heartbeat.To))); err == nil && portal != nil {
				return portal, portal.MXID
			}
		}
		if strings.TrimSpace(heartbeat.Target) != "" && strings.ToLower(strings.TrimSpace(heartbeat.Target)) != "last" {
			target := strings.TrimSpace(heartbeat.Target)
			if strings.HasPrefix(target, "!") {
				if portal, err := oc.UserLogin.Bridge.GetPortalByMXID(context.Background(), id.RoomID(target)); err == nil && portal != nil {
					return portal, portal.MXID
				}
			}
		}
	}
	if portal := oc.lastActivePortal(agentID); portal != nil {
		return portal, portal.MXID
	}
	if portal := oc.defaultChatPortal(); portal != nil {
		return portal, portal.MXID
	}
	return nil, ""
}

func (oc *AIClient) shouldRunHeartbeatForFile(agentID string, reason string) bool {
	store := textfs.NewStore(oc.UserLogin.Bridge.DB.Database, string(oc.UserLogin.Bridge.DB.BridgeID), string(oc.UserLogin.ID), normalizeAgentID(agentID))
	entry, found, err := store.Read(context.Background(), agents.DefaultHeartbeatFilename)
	if err != nil || !found {
		return true
	}
	if agents.IsHeartbeatContentEffectivelyEmpty(entry.Content) && reason != "exec-event" {
		return false
	}
	return true
}

func isWithinActiveHours(oc *AIClient, heartbeat *HeartbeatConfig, nowMs int64) bool {
	if heartbeat == nil || heartbeat.ActiveHours == nil {
		return true
	}
	startMin := parseActiveHoursTime(heartbeat.ActiveHours.Start, false)
	endMin := parseActiveHoursTime(heartbeat.ActiveHours.End, true)
	if startMin == nil || endMin == nil {
		return true
	}
	loc := resolveActiveHoursTimezone(oc, heartbeat.ActiveHours.Timezone)
	if loc == nil {
		return true
	}
	now := time.UnixMilli(nowMs).In(loc)
	currentMin := now.Hour()*60 + now.Minute()
	if *endMin > *startMin {
		return currentMin >= *startMin && currentMin < *endMin
	}
	return currentMin >= *startMin || currentMin < *endMin
}

func parseActiveHoursTime(raw string, allow24 bool) *int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if !activeHoursPattern.MatchString(trimmed) {
		return nil
	}
	parts := strings.Split(trimmed, ":")
	if len(parts) != 2 {
		return nil
	}
	hour, err1 := strconv.Atoi(parts[0])
	minute, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return nil
	}
	if hour == 24 {
		if !allow24 || minute != 0 {
			return nil
		}
		val := 24 * 60
		return &val
	}
	val := hour*60 + minute
	return &val
}

var activeHoursPattern = regexp.MustCompile(`^([01]\d|2[0-3]|24):([0-5]\d)$`)

func resolveActiveHoursTimezone(oc *AIClient, raw string) *time.Location {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.EqualFold(trimmed, "user") {
		_, loc := oc.resolveUserTimezone()
		return loc
	}
	if strings.EqualFold(trimmed, "local") {
		return time.Local
	}
	if loc, err := time.LoadLocation(trimmed); err == nil {
		return loc
	}
	_, loc := oc.resolveUserTimezone()
	return loc
}

func formatSystemEvents(events []SystemEvent) string {
	if len(events) == 0 {
		return ""
	}
	lines := make([]string, 0, len(events))
	for _, evt := range events {
		text := strings.TrimSpace(evt.Text)
		if text == "" {
			continue
		}
		ts := time.UnixMilli(evt.TS).UTC().Format(time.RFC3339)
		lines = append(lines, fmt.Sprintf("System: [%s] %s", ts, text))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}
