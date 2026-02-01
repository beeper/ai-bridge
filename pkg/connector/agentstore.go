package connector

import (
	"context"
	"fmt"

	"github.com/beeper/ai-bridge/pkg/agents"
	"github.com/beeper/ai-bridge/pkg/agents/tools"
)

// AgentStoreAdapter implements agents.AgentStore using the connector's data sources.
type AgentStoreAdapter struct {
	client *AIClient
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

	// Load custom agents from UserLoginMetadata
	customAgents := s.loadCustomAgentsFromMetadata()
	for id, data := range customAgents {
		result[id] = customAgentDataToDefinition(data)
	}

	return result, nil
}

// loadCustomAgentsFromMetadata retrieves custom agents from UserLoginMetadata.
func (s *AgentStoreAdapter) loadCustomAgentsFromMetadata() map[string]*CustomAgentData {
	meta := loginMetadata(s.client.UserLogin)
	if meta.CustomAgents == nil {
		return make(map[string]*CustomAgentData)
	}
	return meta.CustomAgents
}

// saveCustomAgentsToMetadata persists custom agents to UserLoginMetadata.
func (s *AgentStoreAdapter) saveCustomAgentsToMetadata(ctx context.Context, agentsMap map[string]*CustomAgentData) error {
	meta := loginMetadata(s.client.UserLogin)
	meta.CustomAgents = agentsMap
	return s.client.UserLogin.Save(ctx)
}

// customAgentDataToDefinition converts CustomAgentData to AgentDefinition.
func customAgentDataToDefinition(data *CustomAgentData) *agents.AgentDefinition {
	return &agents.AgentDefinition{
		ID:          data.ID,
		Name:        data.Name,
		Description: data.Description,
		AvatarURL:   data.AvatarURL,
		Model: agents.ModelConfig{
			Primary:   data.Model,
			Fallbacks: data.ModelFallback,
		},
		SystemPrompt:  data.SystemPrompt,
		PromptMode:    agents.PromptMode(data.PromptMode),
		ToolProfile:   agents.ToolProfile(data.ToolProfile),
		ToolOverrides: data.ToolOverrides,
		Temperature:   data.Temperature,
		IsPreset:      false,
		CreatedAt:     data.CreatedAt,
		UpdatedAt:     data.UpdatedAt,
	}
}

// agentDefinitionToCustomData converts AgentDefinition to CustomAgentData.
func agentDefinitionToCustomData(agent *agents.AgentDefinition) *CustomAgentData {
	return &CustomAgentData{
		ID:            agent.ID,
		Name:          agent.Name,
		Description:   agent.Description,
		AvatarURL:     agent.AvatarURL,
		Model:         agent.Model.Primary,
		ModelFallback: agent.Model.Fallbacks,
		SystemPrompt:  agent.SystemPrompt,
		PromptMode:    string(agent.PromptMode),
		ToolProfile:   string(agent.ToolProfile),
		ToolOverrides: agent.ToolOverrides,
		Temperature:   agent.Temperature,
		CreatedAt:     agent.CreatedAt,
		UpdatedAt:     agent.UpdatedAt,
	}
}

// SaveAgent implements agents.AgentStore.
// It saves an agent to the UserLoginMetadata.
func (s *AgentStoreAdapter) SaveAgent(ctx context.Context, agent *agents.AgentDefinition) error {
	if err := agent.Validate(); err != nil {
		return err
	}

	if agent.IsPreset {
		return agents.ErrAgentIsPreset
	}

	// Load existing custom agents
	customAgents := s.loadCustomAgentsFromMetadata()

	// Add/update this agent
	customAgents[agent.ID] = agentDefinitionToCustomData(agent)

	// Save back to metadata
	if err := s.saveCustomAgentsToMetadata(ctx, customAgents); err != nil {
		return fmt.Errorf("failed to save agents: %w", err)
	}

	s.client.log.Info().Str("agent_id", agent.ID).Str("name", agent.Name).Msg("Saved custom agent")
	return nil
}

// DeleteAgent implements agents.AgentStore.
func (s *AgentStoreAdapter) DeleteAgent(ctx context.Context, agentID string) error {
	if agents.IsPreset(agentID) || agents.IsBossAgent(agentID) {
		return agents.ErrAgentIsPreset
	}

	// Load existing custom agents
	customAgents := s.loadCustomAgentsFromMetadata()

	// Check if agent exists
	if _, exists := customAgents[agentID]; !exists {
		return agents.ErrAgentNotFound
	}

	// Remove this agent
	delete(customAgents, agentID)

	// Save back to metadata
	if err := s.saveCustomAgentsToMetadata(ctx, customAgents); err != nil {
		return fmt.Errorf("failed to save agents: %w", err)
	}

	s.client.log.Info().Str("agent_id", agentID).Msg("Deleted custom agent")
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
// Falls back to the general assistant if no specific agent is set.
func (s *AgentStoreAdapter) GetAgentForRoom(ctx context.Context, meta *PortalMetadata) (*agents.AgentDefinition, error) {
	agentID := meta.AgentID
	if agentID == "" {
		agentID = meta.DefaultAgentID
	}
	if agentID == "" {
		agentID = "general" // Default to general assistant
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
	return &AgentDefinitionContent{
		ID:            agent.ID,
		Name:          agent.Name,
		Description:   agent.Description,
		AvatarURL:     agent.AvatarURL,
		Model:         agent.Model.Primary,
		ModelFallback: agent.Model.Fallbacks,
		SystemPrompt:  agent.SystemPrompt,
		PromptMode:    string(agent.PromptMode),
		ToolProfile:   string(agent.ToolProfile),
		ToolOverrides: agent.ToolOverrides,
		Temperature:   agent.Temperature,
		IsPreset:      agent.IsPreset,
		CreatedAt:     agent.CreatedAt,
		UpdatedAt:     agent.UpdatedAt,
	}
}

// FromAgentDefinitionContent converts a Matrix event form to AgentDefinition.
func FromAgentDefinitionContent(content *AgentDefinitionContent) *agents.AgentDefinition {
	return &agents.AgentDefinition{
		ID:          content.ID,
		Name:        content.Name,
		Description: content.Description,
		AvatarURL:   content.AvatarURL,
		Model: agents.ModelConfig{
			Primary:   content.Model,
			Fallbacks: content.ModelFallback,
		},
		SystemPrompt:  content.SystemPrompt,
		PromptMode:    agents.PromptMode(content.PromptMode),
		ToolProfile:   agents.ToolProfile(content.ToolProfile),
		ToolOverrides: content.ToolOverrides,
		Temperature:   content.Temperature,
		IsPreset:      content.IsPreset,
		CreatedAt:     content.CreatedAt,
		UpdatedAt:     content.UpdatedAt,
	}
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
		Temperature:   data.Temperature,
		IsPreset:      data.IsPreset,
		CreatedAt:     data.CreatedAt,
		UpdatedAt:     data.UpdatedAt,
	}
}
