package opencodebridge

import (
	"strings"

	"github.com/beeper/ai-bridge/bridges/opencode/opencode"
)

func buildTurnStartMetadata(msg *opencode.MessageWithParts, agentID string) map[string]any {
	if msg == nil {
		return nil
	}
	metadata := map[string]any{
		"role":       strings.TrimSpace(msg.Info.Role),
		"session_id": strings.TrimSpace(msg.Info.SessionID),
		"message_id": strings.TrimSpace(msg.Info.ID),
		"agent_id":   strings.TrimSpace(agentID),
	}
	if msg.Info.ParentID != "" {
		metadata["parent_message_id"] = strings.TrimSpace(msg.Info.ParentID)
	}
	if msg.Info.Agent != "" {
		metadata["agent"] = strings.TrimSpace(msg.Info.Agent)
	}
	if msg.Info.ModelID != "" {
		metadata["model_id"] = strings.TrimSpace(msg.Info.ModelID)
	}
	if msg.Info.ProviderID != "" {
		metadata["provider_id"] = strings.TrimSpace(msg.Info.ProviderID)
	}
	if msg.Info.Mode != "" {
		metadata["mode"] = strings.TrimSpace(msg.Info.Mode)
	}
	if msg.Info.Time.Created > 0 {
		metadata["started_at"] = int64(msg.Info.Time.Created)
	}
	return metadata
}

func buildTurnFinishMetadata(msg *opencode.MessageWithParts, agentID, finishReason string) map[string]any {
	metadata := buildTurnStartMetadata(msg, agentID)
	if metadata == nil {
		metadata = map[string]any{"agent_id": strings.TrimSpace(agentID)}
	}
	if finishReason != "" {
		metadata["finish_reason"] = strings.TrimSpace(finishReason)
	} else if msg != nil && msg.Info.Finish != "" {
		metadata["finish_reason"] = strings.TrimSpace(msg.Info.Finish)
	}
	if msg != nil && msg.Info.Time.Completed > 0 {
		metadata["completed_at"] = int64(msg.Info.Time.Completed)
	}
	if msg != nil && msg.Info.Cost != 0 {
		metadata["cost"] = msg.Info.Cost
	}
	if msg != nil && msg.Info.Tokens != nil {
		metadata["prompt_tokens"] = int64(msg.Info.Tokens.Input)
		metadata["completion_tokens"] = int64(msg.Info.Tokens.Output)
		metadata["reasoning_tokens"] = int64(msg.Info.Tokens.Reasoning)
		total := int64(msg.Info.Tokens.Input + msg.Info.Tokens.Output + msg.Info.Tokens.Reasoning)
		if msg.Info.Tokens.Cache != nil {
			total += int64(msg.Info.Tokens.Cache.Read + msg.Info.Tokens.Cache.Write)
		}
		metadata["total_tokens"] = total
	}
	for _, part := range msg.Parts {
		if part.Type != "step-finish" {
			continue
		}
		if part.Cost != 0 {
			metadata["cost"] = part.Cost
		}
		if part.Tokens != nil {
			metadata["prompt_tokens"] = int64(part.Tokens.Input)
			metadata["completion_tokens"] = int64(part.Tokens.Output)
			metadata["reasoning_tokens"] = int64(part.Tokens.Reasoning)
			total := int64(part.Tokens.Input + part.Tokens.Output + part.Tokens.Reasoning)
			if part.Tokens.Cache != nil {
				total += int64(part.Tokens.Cache.Read + part.Tokens.Cache.Write)
			}
			metadata["total_tokens"] = total
		}
	}
	return metadata
}
