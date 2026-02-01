package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/matrix"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"github.com/beeper/ai-bridge/pkg/agents"
	"github.com/beeper/ai-bridge/pkg/agents/tools"
)

// AgentStoreAdapter implements agents.AgentStore using Matrix state events.
type AgentStoreAdapter struct {
	client *AIClient
	mu     sync.Mutex // protects read-modify-write operations on custom agents
}

// NewAgentStoreAdapter creates a new agent store adapter.
func NewAgentStoreAdapter(client *AIClient) *AgentStoreAdapter {
	return &AgentStoreAdapter{client: client}
}

// LoadAgents implements agents.AgentStore.
// It loads agents from the Builder room's Matrix state events.
func (s *AgentStoreAdapter) LoadAgents(ctx context.Context) (map[string]*agents.AgentDefinition, error) {
	// Start with preset agents
	result := make(map[string]*agents.AgentDefinition)

	// Add all presets
	for _, preset := range agents.PresetAgents {
		result[preset.ID] = preset.Clone()
	}

	// Add boss agent
	result[agents.BossAgent.ID] = agents.BossAgent.Clone()

	// Load custom agents from Matrix state event in Builder room
	customAgents, err := s.loadCustomAgentsFromState(ctx)
	if err != nil {
		s.client.log.Warn().Err(err).Msg("Failed to load custom agents from Matrix state")
		// Continue with just presets if state load fails
	}
	for id, content := range customAgents {
		result[id] = FromAgentDefinitionContent(content)
	}

	return result, nil
}

// loadCustomAgentsFromState loads custom agents from the Builder room's Matrix state event.
func (s *AgentStoreAdapter) loadCustomAgentsFromState(ctx context.Context) (map[string]*AgentDefinitionContent, error) {
	// Get Builder room
	portal, err := s.getBuilderPortal(ctx)
	if err != nil || portal == nil || portal.MXID == "" {
		return nil, nil // No Builder room yet, return empty
	}

	// Get the Matrix connector to read state events
	matrixConn, ok := s.client.UserLogin.Bridge.Matrix.(*matrix.Connector)
	if !ok {
		return nil, fmt.Errorf("matrix connector not available")
	}

	// Read the custom agents state event
	evt, err := matrixConn.GetStateEvent(ctx, portal.MXID, CustomAgentsEventType, "")
	if err != nil {
		// State event doesn't exist yet - this is normal for new users
		return nil, nil
	}

	// Parse the content
	var content CustomAgentsEventContent
	if err := json.Unmarshal(evt.Content.VeryRaw, &content); err != nil {
		return nil, fmt.Errorf("failed to parse custom agents event: %w", err)
	}

	if content.Agents == nil {
		return make(map[string]*AgentDefinitionContent), nil
	}
	return content.Agents, nil
}

// saveCustomAgentsToState saves custom agents to the Builder room's Matrix state event.
func (s *AgentStoreAdapter) saveCustomAgentsToState(ctx context.Context, agentsMap map[string]*AgentDefinitionContent) error {
	// Get Builder room
	portal, err := s.getBuilderPortal(ctx)
	if err != nil {
		return fmt.Errorf("failed to get builder portal: %w", err)
	}
	if portal == nil || portal.MXID == "" {
		return fmt.Errorf("builder room not created yet")
	}

	// Send the custom agents state event
	content := &CustomAgentsEventContent{
		Agents: agentsMap,
	}

	bot := s.client.UserLogin.Bridge.Bot
	_, err = bot.SendState(ctx, portal.MXID, CustomAgentsEventType, "", &event.Content{
		Parsed: content,
	}, time.Time{})
	if err != nil {
		return fmt.Errorf("failed to send state event: %w", err)
	}

	return nil
}

// getBuilderPortal returns the Builder room portal if it exists.
func (s *AgentStoreAdapter) getBuilderPortal(ctx context.Context) (*bridgev2.Portal, error) {
	meta := loginMetadata(s.client.UserLogin)
	if meta.BuilderRoomID == "" {
		return nil, nil
	}

	return s.client.UserLogin.Bridge.GetPortalByKey(ctx, networkid.PortalKey{
		ID:       meta.BuilderRoomID,
		Receiver: s.client.UserLogin.ID,
	})
}

// modifyCustomAgents handles the common load-modify-save pattern for custom agents.
// The operation function receives the current agents map and can modify it.
func (s *AgentStoreAdapter) modifyCustomAgents(ctx context.Context, operation func(map[string]*AgentDefinitionContent) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	customAgents, err := s.loadCustomAgentsFromState(ctx)
	if err != nil {
		return fmt.Errorf("failed to load existing agents: %w", err)
	}
	if customAgents == nil {
		customAgents = make(map[string]*AgentDefinitionContent)
	}

	if err := operation(customAgents); err != nil {
		return err
	}

	if err := s.saveCustomAgentsToState(ctx, customAgents); err != nil {
		return fmt.Errorf("failed to save agents: %w", err)
	}

	return nil
}

// SaveAgent implements agents.AgentStore.
// It saves an agent to the Builder room's Matrix state event.
func (s *AgentStoreAdapter) SaveAgent(ctx context.Context, agent *agents.AgentDefinition) error {
	if err := agent.Validate(); err != nil {
		return err
	}
	if agent.IsPreset {
		return agents.ErrAgentIsPreset
	}

	err := s.modifyCustomAgents(ctx, func(customAgents map[string]*AgentDefinitionContent) error {
		customAgents[agent.ID] = ToAgentDefinitionContent(agent)
		return nil
	})
	if err != nil {
		return err
	}

	s.client.log.Info().Str("agent_id", agent.ID).Str("name", agent.Name).Msg("Saved custom agent to Matrix state")
	return nil
}

// DeleteAgent implements agents.AgentStore.
func (s *AgentStoreAdapter) DeleteAgent(ctx context.Context, agentID string) error {
	if agents.IsPreset(agentID) || agents.IsBossAgent(agentID) {
		return agents.ErrAgentIsPreset
	}

	err := s.modifyCustomAgents(ctx, func(customAgents map[string]*AgentDefinitionContent) error {
		if customAgents[agentID] == nil {
			return agents.ErrAgentNotFound
		}
		delete(customAgents, agentID)
		return nil
	})
	if err != nil {
		return err
	}

	s.client.log.Info().Str("agent_id", agentID).Msg("Deleted custom agent from Matrix state")
	return nil
}

// ListModels implements agents.AgentStore.
func (s *AgentStoreAdapter) ListModels(ctx context.Context) ([]agents.ModelInfo, error) {
	models, err := s.client.listAvailableModels(ctx, false)
	if err != nil {
		return nil, err
	}

	result := make([]agents.ModelInfo, 0, len(models))
	for _, m := range models {
		result = append(result, agents.ModelInfo{
			ID:          m.ID,
			Name:        m.Name,
			Provider:    m.Provider,
			Description: m.Description,
		})
	}
	return result, nil
}

// ListAvailableTools implements agents.AgentStore.
func (s *AgentStoreAdapter) ListAvailableTools(_ context.Context) ([]tools.ToolInfo, error) {
	registry := tools.DefaultRegistry()

	var result []tools.ToolInfo
	for _, tool := range registry.All() {
		result = append(result, tools.ToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
			Type:        tool.Type,
			Group:       tool.Group,
			Enabled:     true, // All tools are available, policy determines which are enabled
		})
	}
	return result, nil
}

// Verify interface compliance
var _ agents.AgentStore = (*AgentStoreAdapter)(nil)

// GetAgentByID looks up an agent by ID, returning preset or custom agents.
func (s *AgentStoreAdapter) GetAgentByID(ctx context.Context, agentID string) (*agents.AgentDefinition, error) {
	agentsMap, err := s.LoadAgents(ctx)
	if err != nil {
		return nil, err
	}

	agent, ok := agentsMap[agentID]
	if !ok {
		return nil, agents.ErrAgentNotFound
	}
	return agent, nil
}

// GetAgentForRoom returns the agent assigned to a room.
// Falls back to the Quick Chatter if no specific agent is set.
func (s *AgentStoreAdapter) GetAgentForRoom(ctx context.Context, meta *PortalMetadata) (*agents.AgentDefinition, error) {
	agentID := meta.AgentID
	if agentID == "" {
		agentID = meta.DefaultAgentID
	}
	if agentID == "" {
		agentID = "quick" // Default to Quick Chatter
	}

	return s.GetAgentByID(ctx, agentID)
}

// CreateExecutorForAgent creates a tools.Executor configured for an agent.
func (s *AgentStoreAdapter) CreateExecutorForAgent(agent *agents.AgentDefinition) *tools.Executor {
	registry := tools.DefaultRegistry()

	// Register boss tools if this is the boss agent
	if agent.ID == "boss" {
		for _, tool := range tools.BossTools() {
			registry.Register(tool)
		}
	}

	policy := agents.CreatePolicyFromProfile(agent, registry)
	return tools.NewExecutor(registry, policy)
}

// ToAgentDefinitionContent converts an AgentDefinition to its Matrix event form.
func ToAgentDefinitionContent(agent *agents.AgentDefinition) *AgentDefinitionContent {
	content := &AgentDefinitionContent{
		ID:              agent.ID,
		Name:            agent.Name,
		Description:     agent.Description,
		AvatarURL:       agent.AvatarURL,
		Model:           agent.Model.Primary,
		ModelFallback:   agent.Model.Fallbacks,
		SystemPrompt:    agent.SystemPrompt,
		PromptMode:      string(agent.PromptMode),
		ToolProfile:     string(agent.ToolProfile),
		ToolOverrides:   agent.ToolOverrides,
		ToolAlsoAllow:   agent.ToolAlsoAllow,
		Temperature:     agent.Temperature,
		ReasoningEffort: agent.ReasoningEffort,
		IsPreset:        agent.IsPreset,
		CreatedAt:       agent.CreatedAt,
		UpdatedAt:       agent.UpdatedAt,
	}

	// Include Identity if present
	if agent.Identity != nil {
		content.IdentityName = agent.Identity.Name
		content.IdentityPersona = agent.Identity.Persona
	}

	return content
}

// FromAgentDefinitionContent converts a Matrix event form to AgentDefinition.
func FromAgentDefinitionContent(content *AgentDefinitionContent) *agents.AgentDefinition {
	def := &agents.AgentDefinition{
		ID:          content.ID,
		Name:        content.Name,
		Description: content.Description,
		AvatarURL:   content.AvatarURL,
		Model: agents.ModelConfig{
			Primary:   content.Model,
			Fallbacks: content.ModelFallback,
		},
		SystemPrompt:    content.SystemPrompt,
		PromptMode:      agents.PromptMode(content.PromptMode),
		ToolProfile:     agents.ToolProfile(content.ToolProfile),
		ToolOverrides:   content.ToolOverrides,
		ToolAlsoAllow:   content.ToolAlsoAllow,
		Temperature:     content.Temperature,
		ReasoningEffort: content.ReasoningEffort,
		IsPreset:        content.IsPreset,
		CreatedAt:       content.CreatedAt,
		UpdatedAt:       content.UpdatedAt,
	}

	// Restore Identity if present
	if content.IdentityName != "" || content.IdentityPersona != "" {
		def.Identity = &agents.Identity{
			Name:    content.IdentityName,
			Persona: content.IdentityPersona,
		}
	}

	return def
}

// BossStoreAdapter implements tools.AgentStoreInterface for boss tool execution.
// This adapter converts between our agent types and the tools package types.
type BossStoreAdapter struct {
	store *AgentStoreAdapter
}

// NewBossStoreAdapter creates a new boss store adapter.
func NewBossStoreAdapter(client *AIClient) *BossStoreAdapter {
	return &BossStoreAdapter{
		store: NewAgentStoreAdapter(client),
	}
}

// LoadAgents implements tools.AgentStoreInterface.
func (b *BossStoreAdapter) LoadAgents(ctx context.Context) (map[string]tools.AgentData, error) {
	agentsMap, err := b.store.LoadAgents(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]tools.AgentData, len(agentsMap))
	for id, agent := range agentsMap {
		result[id] = agentToToolsData(agent)
	}
	return result, nil
}

// SaveAgent implements tools.AgentStoreInterface.
func (b *BossStoreAdapter) SaveAgent(ctx context.Context, agent tools.AgentData) error {
	def := toolsDataToAgent(agent)
	return b.store.SaveAgent(ctx, def)
}

// DeleteAgent implements tools.AgentStoreInterface.
func (b *BossStoreAdapter) DeleteAgent(ctx context.Context, agentID string) error {
	return b.store.DeleteAgent(ctx, agentID)
}

// ListModels implements tools.AgentStoreInterface.
func (b *BossStoreAdapter) ListModels(ctx context.Context) ([]tools.ModelData, error) {
	models, err := b.store.ListModels(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]tools.ModelData, 0, len(models))
	for _, m := range models {
		result = append(result, tools.ModelData{
			ID:          m.ID,
			Name:        m.Name,
			Provider:    m.Provider,
			Description: m.Description,
		})
	}
	return result, nil
}

// ListAvailableTools implements tools.AgentStoreInterface.
func (b *BossStoreAdapter) ListAvailableTools(ctx context.Context) ([]tools.ToolInfo, error) {
	return b.store.ListAvailableTools(ctx)
}

// CreateRoom implements tools.AgentStoreInterface.
func (b *BossStoreAdapter) CreateRoom(ctx context.Context, room tools.RoomData) (string, error) {
	// Get the agent to verify it exists
	agent, err := b.store.GetAgentByID(ctx, room.AgentID)
	if err != nil {
		return "", fmt.Errorf("agent '%s' not found: %w", room.AgentID, err)
	}

	// Create the portal via createAgentChat
	resp, err := b.store.client.createAgentChat(ctx, agent)
	if err != nil {
		return "", fmt.Errorf("failed to create room: %w", err)
	}

	// Get the portal to apply any overrides
	portal, err := b.store.client.UserLogin.Bridge.GetPortalByKey(ctx, resp.PortalKey)
	if err != nil {
		return "", fmt.Errorf("failed to get created portal: %w", err)
	}

	// Apply custom name and system prompt if provided
	pm := portalMeta(portal)
	if room.Name != "" {
		pm.Title = room.Name
		portal.Name = room.Name
		portal.NameSet = true
		if resp.PortalInfo != nil {
			resp.PortalInfo.Name = &room.Name
		}
	}
	if room.SystemPrompt != "" {
		pm.SystemPrompt = room.SystemPrompt
		// Note: portal.Topic is NOT set to SystemPrompt - they are separate concepts
		// Topic is for display only, SystemPrompt is for LLM context
	}

	if err := portal.Save(ctx); err != nil {
		b.store.client.log.Warn().Err(err).Msg("Failed to save room overrides")
	}

	// Create the Matrix room
	if err := portal.CreateMatrixRoom(ctx, b.store.client.UserLogin, resp.PortalInfo); err != nil {
		return "", fmt.Errorf("failed to create Matrix room: %w", err)
	}

	// Send welcome message (excluded from LLM history)
	b.store.client.sendWelcomeMessage(ctx, portal)

	if room.Name != "" {
		if err := b.store.client.setRoomName(ctx, portal, room.Name); err != nil {
			b.store.client.log.Warn().Err(err).Msg("Failed to set Matrix room name")
		}
	}
	if room.SystemPrompt != "" {
		if err := b.store.client.setRoomSystemPrompt(ctx, portal, room.SystemPrompt); err != nil {
			b.store.client.log.Warn().Err(err).Msg("Failed to set room system prompt")
		}
	}

	return string(portal.PortalKey.ID), nil
}

// ModifyRoom implements tools.AgentStoreInterface.
func (b *BossStoreAdapter) ModifyRoom(ctx context.Context, roomID string, updates tools.RoomData) error {
	// Find the portal by listing all and matching ID
	portals, err := b.store.client.listAllChatPortals(ctx)
	if err != nil {
		return fmt.Errorf("failed to list portals: %w", err)
	}

	var portal *bridgev2.Portal
	for _, p := range portals {
		if string(p.PortalKey.ID) == roomID {
			portal = p
			break
		}
	}
	if portal == nil {
		return fmt.Errorf("room '%s' not found", roomID)
	}

	pm := portalMeta(portal)

	// Apply updates
	if updates.Name != "" {
		portal.Name = updates.Name
		pm.Title = updates.Name
		portal.NameSet = true
	}
	if updates.AgentID != "" {
		// Verify agent exists
		agent, err := b.store.GetAgentByID(ctx, updates.AgentID)
		if err != nil {
			return fmt.Errorf("agent '%s' not found: %w", updates.AgentID, err)
		}
		pm.AgentID = agent.ID
		pm.DefaultAgentID = agent.ID
		portal.OtherUserID = agentUserID(agent.ID)
	}
	if updates.SystemPrompt != "" {
		pm.SystemPrompt = updates.SystemPrompt
		// Note: portal.Topic is NOT set to SystemPrompt - they are separate concepts
	}

	if updates.Name != "" && portal.MXID != "" {
		if err := b.store.client.setRoomName(ctx, portal, updates.Name); err != nil {
			b.store.client.log.Warn().Err(err).Msg("Failed to set Matrix room name")
		}
	}
	if updates.SystemPrompt != "" && portal.MXID != "" {
		if err := b.store.client.setRoomSystemPrompt(ctx, portal, updates.SystemPrompt); err != nil {
			b.store.client.log.Warn().Err(err).Msg("Failed to set room system prompt")
		}
	}

	return portal.Save(ctx)
}

// ListRooms implements tools.AgentStoreInterface.
func (b *BossStoreAdapter) ListRooms(ctx context.Context) ([]tools.RoomData, error) {
	portals, err := b.store.client.listAllChatPortals(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list rooms: %w", err)
	}

	var rooms []tools.RoomData
	for _, portal := range portals {
		pm := portalMeta(portal)
		name := portal.Name
		if name == "" {
			name = pm.Title
		}
		rooms = append(rooms, tools.RoomData{
			ID:      string(portal.PortalKey.ID),
			Name:    name,
			AgentID: pm.AgentID,
		})
	}

	return rooms, nil
}

// Verify interface compliance
var _ tools.AgentStoreInterface = (*BossStoreAdapter)(nil)

// agentToToolsData converts an AgentDefinition to tools.AgentData.
func agentToToolsData(agent *agents.AgentDefinition) tools.AgentData {
	return tools.AgentData{
		ID:            agent.ID,
		Name:          agent.Name,
		Description:   agent.Description,
		Model:         agent.Model.Primary,
		SystemPrompt:  agent.SystemPrompt,
		ToolProfile:   string(agent.ToolProfile),
		ToolOverrides: agent.ToolOverrides,
		ToolAlsoAllow: agent.ToolAlsoAllow,
		Temperature:   agent.Temperature,
		IsPreset:      agent.IsPreset,
		CreatedAt:     agent.CreatedAt,
		UpdatedAt:     agent.UpdatedAt,
	}
}

// toolsDataToAgent converts tools.AgentData to an AgentDefinition.
func toolsDataToAgent(data tools.AgentData) *agents.AgentDefinition {
	return &agents.AgentDefinition{
		ID:          data.ID,
		Name:        data.Name,
		Description: data.Description,
		Model: agents.ModelConfig{
			Primary: data.Model,
		},
		SystemPrompt:  data.SystemPrompt,
		ToolProfile:   agents.ToolProfile(data.ToolProfile),
		ToolOverrides: data.ToolOverrides,
		ToolAlsoAllow: data.ToolAlsoAllow,
		Temperature:   data.Temperature,
		IsPreset:      data.IsPreset,
		CreatedAt:     data.CreatedAt,
		UpdatedAt:     data.UpdatedAt,
	}
}
