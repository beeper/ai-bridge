package connector

import (
	"errors"
	"slices"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/rs/zerolog"
)

func logResponsesFailure(log zerolog.Logger, err error, params responses.ResponseNewParams, meta *PortalMetadata, messages []openai.ChatCompletionMessageParamUnion, stage string) {
	event := log.Error().Err(err).Str("stage", stage)
	addRequestSummary(event, meta, messages)
	addResponsesParamsSummary(event, params)
	addOpenAIErrorFields(event, err)
	event.Msg("Responses API failure")
}

func logChatCompletionsFailure(log zerolog.Logger, err error, params openai.ChatCompletionNewParams, meta *PortalMetadata, messages []openai.ChatCompletionMessageParamUnion, stage string) {
	event := log.Error().Err(err).Str("stage", stage)
	addRequestSummary(event, meta, messages)
	addChatParamsSummary(event, params)
	addOpenAIErrorFields(event, err)
	event.Msg("Chat Completions failure")
}

func addRequestSummary(event *zerolog.Event, meta *PortalMetadata, messages []openai.ChatCompletionMessageParamUnion) {
	if event == nil {
		return
	}
	event.Int("message_count", len(messages))
	event.Bool("has_audio", hasAudioContent(messages))
	event.Bool("has_multimodal", hasMultimodalContent(messages))
	if meta != nil {
		event.Bool("tool_calling", meta.Capabilities.SupportsToolCalling)
		event.Str("conversation_mode", meta.ConversationMode)
	}
}

func addResponsesParamsSummary(event *zerolog.Event, params responses.ResponseNewParams) {
	if event == nil {
		return
	}
	if params.Model != "" {
		event.Str("model", string(params.Model))
	}
	if params.PreviousResponseID.Valid() {
		event.Bool("uses_previous_response_id", true)
	}
	if params.MaxOutputTokens.Valid() {
		event.Int64("max_output_tokens", params.MaxOutputTokens.Value)
	}
	if params.Temperature.Valid() {
		event.Float64("temperature", params.Temperature.Value)
	}
	inputKind := "none"
	inputCount := 0
	if params.Input.OfInputItemList != nil {
		inputKind = "items"
		inputCount = len(params.Input.OfInputItemList)
	} else if params.Input.OfString.Valid() {
		inputKind = "string"
	}
	event.Str("input_kind", inputKind).Int("input_items", inputCount)

	toolNames := responsesToolNames(params.Tools)
	if len(toolNames) > 0 {
		event.Int("tool_count", len(toolNames)).Strs("tools", toolNames)
	}
}

func addChatParamsSummary(event *zerolog.Event, params openai.ChatCompletionNewParams) {
	if event == nil {
		return
	}
	if params.Model != "" {
		event.Str("model", params.Model)
	}
	if params.MaxCompletionTokens.Valid() {
		event.Int64("max_completion_tokens", params.MaxCompletionTokens.Value)
	}
	if params.Temperature.Valid() {
		event.Float64("temperature", params.Temperature.Value)
	}
	toolNames := chatToolNames(params.Tools)
	if len(toolNames) > 0 {
		event.Int("tool_count", len(toolNames)).Strs("tools", toolNames)
	}
}

func addOpenAIErrorFields(event *zerolog.Event, err error) {
	if event == nil || err == nil {
		return
	}
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return
	}
	if apiErr.StatusCode != 0 {
		event.Int("status_code", apiErr.StatusCode)
	}
	if apiErr.Code != "" {
		event.Str("error_code", apiErr.Code)
	}
	if apiErr.Type != "" {
		event.Str("error_type", apiErr.Type)
	}
	if apiErr.Param != "" {
		event.Str("error_param", apiErr.Param)
	}
	if apiErr.Message != "" {
		event.Str("error_message", apiErr.Message)
	}
	if resp := apiErr.Response; resp != nil {
		addOptionalHeader := func(header, logKey string) {
			if v := resp.Header.Get(header); v != "" {
				event.Str(logKey, v)
			}
		}
		addOptionalHeader("x-request-id", "request_id")
		addOptionalHeader("x-openai-request-id", "openai_request_id")
		addOptionalHeader("x-ai-proxy-provider", "proxy_provider")
		addOptionalHeader("x-ai-proxy-upstream-ray", "proxy_upstream_ray")
		addOptionalHeader("cf-ray", "cf_ray")
		addOptionalHeader("server", "response_server")
		addOptionalHeader("Content-Type", "response_content_type")
	}
	if req := apiErr.Request; req != nil && req.URL != nil {
		event.
			Str("request_method", req.Method).
			Str("request_url", req.URL.String()).
			Str("request_host", req.URL.Host)
		if v := req.Header.Get("x-request-id"); v != "" {
			event.Str("client_request_id", v)
		}
	}
	if raw := apiErr.RawJSON(); raw != "" {
		event.Str("error_raw", raw)
	}
}

func responsesToolNames(tools []responses.ToolUnionParam) []string {
	if len(tools) == 0 {
		return nil
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool.OfFunction != nil && tool.OfFunction.Name != "" {
			names = append(names, tool.OfFunction.Name)
		} else if tool.OfWebSearch != nil {
			names = append(names, ToolNameWebSearch)
		} else if tool.OfFileSearch != nil {
			names = append(names, "file_search")
		} else if tool.OfCodeInterpreter != nil {
			names = append(names, "code_interpreter")
		} else if tool.OfComputerUsePreview != nil {
			names = append(names, "computer")
		} else if tool.OfImageGeneration != nil {
			names = append(names, "image_generation")
		} else if tool.OfLocalShell != nil || tool.OfShell != nil {
			names = append(names, "shell")
		} else if tool.OfMcp != nil {
			names = append(names, "mcp")
		} else if tool.OfApplyPatch != nil {
			names = append(names, "apply_patch")
		}
	}
	slices.Sort(names)
	return names
}

func chatToolNames(tools []openai.ChatCompletionToolUnionParam) []string {
	if len(tools) == 0 {
		return nil
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool.OfFunction != nil && tool.OfFunction.Function.Name != "" {
			names = append(names, tool.OfFunction.Function.Name)
		}
	}
	slices.Sort(names)
	return names
}
