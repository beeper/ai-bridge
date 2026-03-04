package connector

import (
	"context"
	"os"
	"strings"

	"github.com/openai/openai-go/v3"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
)

type streamingRuntimePath string

const (
	streamingRuntimePkgAI           streamingRuntimePath = "pkg_ai"
	streamingRuntimeChatCompletions streamingRuntimePath = "chat_completions"
	streamingRuntimeResponses       streamingRuntimePath = "responses"
)

func pkgAIRuntimeEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("PI_USE_PKG_AI_RUNTIME")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func chooseStreamingRuntimePath(hasAudio bool, modelAPI ModelAPI, preferPkgAI bool) streamingRuntimePath {
	if hasAudio {
		return streamingRuntimeChatCompletions
	}
	if preferPkgAI {
		return streamingRuntimePkgAI
	}
	if modelAPI == ModelAPIChatCompletions {
		return streamingRuntimeChatCompletions
	}
	return streamingRuntimeResponses
}

func (oc *AIClient) streamWithPkgAIBridge(
	ctx context.Context,
	evt *event.Event,
	portal *bridgev2.Portal,
	meta *PortalMetadata,
	prompt []openai.ChatCompletionMessageParamUnion,
) (bool, *ContextLengthError, error) {
	oc.loggerForContext(ctx).Debug().Msg("pkg/ai runtime bridge flag enabled; delegating to existing runtime path")
	switch oc.resolveModelAPI(meta) {
	case ModelAPIChatCompletions:
		return oc.streamChatCompletions(ctx, evt, portal, meta, prompt)
	default:
		return oc.streamingResponseWithToolSchemaFallback(ctx, evt, portal, meta, prompt)
	}
}
