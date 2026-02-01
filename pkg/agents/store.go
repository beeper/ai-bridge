package agents

import (
	"context"

	"github.com/beeper/ai-bridge/pkg/agents/tools"
)

// AgentStore interface for loading and saving agents.
// Implemented by the connector to store agents in Matrix state events.
type AgentStore interface {
	// LoadAgents returns all agents for the current user.
	LoadAgents(ctx context.Context) (map[string]*AgentDefinition, error)

	// SaveAgent creates or updates an agent.
	SaveAgent(ctx context.Context, agent *AgentDefinition) error

	// DeleteAgent removes a custom agent.
	DeleteAgent(ctx context.Context, agentID string) error

	// ListModels returns available AI models.
	ListModels(ctx context.Context) ([]ModelInfo, error)

	// ListAvailableTools returns available tools.
	ListAvailableTools(ctx context.Context) ([]tools.ToolInfo, error)
}

// InMemoryStore is a simple in-memory implementation for testing.
type InMemoryStore struct {
	agents map[string]*AgentDefinition
	models []ModelInfo
	tools  []tools.ToolInfo
}

// NewInMemoryStore creates a new in-memory store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		agents: make(map[string]*AgentDefinition),
	}
}

// LoadAgents implements AgentStore.
func (s *InMemoryStore) LoadAgents(_ context.Context) (map[string]*AgentDefinition, error) {
	result := make(map[string]*AgentDefinition)
	for k, v := range s.agents {
		result[k] = v.Clone()
	}
	return result, nil
}

// SaveAgent implements AgentStore.
func (s *InMemoryStore) SaveAgent(_ context.Context, agent *AgentDefinition) error {
	if err := agent.Validate(); err != nil {
		return err
	}
	s.agents[agent.ID] = agent.Clone()
	return nil
}

// DeleteAgent implements AgentStore.
func (s *InMemoryStore) DeleteAgent(_ context.Context, agentID string) error {
	if _, exists := s.agents[agentID]; !exists {
		return ErrAgentNotFound
	}
	if s.agents[agentID].IsPreset {
		return ErrAgentIsPreset
	}
	delete(s.agents, agentID)
	return nil
}

// ListModels implements AgentStore.
func (s *InMemoryStore) ListModels(_ context.Context) ([]ModelInfo, error) {
	return s.models, nil
}

// ListAvailableTools implements AgentStore.
func (s *InMemoryStore) ListAvailableTools(_ context.Context) ([]tools.ToolInfo, error) {
	return s.tools, nil
}

// SetModels sets the available models for testing.
func (s *InMemoryStore) SetModels(models []ModelInfo) {
	s.models = models
}

// SetTools sets the available tools for testing.
func (s *InMemoryStore) SetTools(toolInfos []tools.ToolInfo) {
	s.tools = toolInfos
}

// AddAgent adds an agent directly for testing.
func (s *InMemoryStore) AddAgent(agent *AgentDefinition) {
	s.agents[agent.ID] = agent
}

// GetAgent retrieves an agent by ID.
func (s *InMemoryStore) GetAgent(agentID string) *AgentDefinition {
	return s.agents[agentID]
}
