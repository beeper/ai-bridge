package connector

import (
	"context"

	"github.com/beeper/ai-bridge/pkg/simpleruntime/agents/tools"
	"maunium.net/go/mautrix/bridgev2"
)

func (oc *AIClient) executeSessionsList(context.Context, *bridgev2.Portal, map[string]any) (*tools.Result, error) {
	return tools.ErrorResult("sessions_list", "sessions tools are not available in the simple bridge"), nil
}

func (oc *AIClient) executeSessionsHistory(context.Context, *bridgev2.Portal, map[string]any) (*tools.Result, error) {
	return tools.ErrorResult("sessions_history", "sessions tools are not available in the simple bridge"), nil
}

func (oc *AIClient) executeSessionsSend(context.Context, *bridgev2.Portal, map[string]any) (*tools.Result, error) {
	return tools.ErrorResult("sessions_send", "sessions tools are not available in the simple bridge"), nil
}
