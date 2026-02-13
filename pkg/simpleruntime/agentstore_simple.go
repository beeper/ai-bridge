package connector

import (
	"context"
	"errors"

	"github.com/beeper/ai-bridge/pkg/simpleruntime/simpledeps/agents"
	"github.com/beeper/ai-bridge/pkg/simpleruntime/simpledeps/agents/tools"
)

type AgentStoreAdapter struct{ client *AIClient }

func NewAgentStoreAdapter(client *AIClient) *AgentStoreAdapter {
	return &AgentStoreAdapter{client: client}
}

func (s *AgentStoreAdapter) LoadAgents(context.Context) (map[string]*agents.AgentDefinition, error) {
	return map[string]*agents.AgentDefinition{}, nil
}

func (s *AgentStoreAdapter) SaveAgent(context.Context, *agents.AgentDefinition) error {
	return errors.New("agents are not available in simple bridge")
}

func (s *AgentStoreAdapter) DeleteAgent(context.Context, string) error {
	return errors.New("agents are not available in simple bridge")
}

func (s *AgentStoreAdapter) ListModels(ctx context.Context) ([]agents.ModelInfo, error) {
	if s == nil || s.client == nil {
		return nil, nil
	}
	models, err := s.client.listAvailableModels(ctx, false)
	if err != nil {
		return nil, err
	}
	out := make([]agents.ModelInfo, 0, len(models))
	for _, m := range models {
		out = append(out, agents.ModelInfo{ID: m.ID, Name: m.Name, Provider: m.Provider, Description: m.Description, Primary: m.ID})
	}
	return out, nil
}

func (s *AgentStoreAdapter) ListAvailableTools(context.Context) ([]tools.ToolInfo, error) {
	return []tools.ToolInfo{{Name: ToolNameWebSearch, Description: "Search the web", Enabled: true}}, nil
}

func (s *AgentStoreAdapter) GetAgentByID(context.Context, string) (*agents.AgentDefinition, error) {
	return nil, agents.ErrAgentNotFound
}

func (s *AgentStoreAdapter) GetAgentForRoom(context.Context, *PortalMetadata) (*agents.AgentDefinition, error) {
	return nil, agents.ErrAgentNotFound
}

type BossStoreAdapter struct{}

func NewBossStoreAdapter(*AIClient) *BossStoreAdapter { return &BossStoreAdapter{} }
