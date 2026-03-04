package providers

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrockdocument "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/beeper/ai-bridge/pkg/ai"
)

func streamBedrockConverse(model ai.Model, c ai.Context, options *ai.StreamOptions) *ai.AssistantMessageEventStream {
	stream := ai.NewAssistantMessageEventStream(128)
	go func() {
		var streamOptions ai.StreamOptions
		if options != nil {
			streamOptions = *options
		}
		runCtx := streamOptions.Ctx
		if runCtx == nil {
			runCtx = context.Background()
		}

		if ai.GetEnvAPIKey("amazon-bedrock") == "" {
			pushProviderError(stream, model, "missing AWS credentials for Amazon Bedrock runtime")
			return
		}
		cfg, err := loadBedrockAWSConfig(runCtx)
		if err != nil {
			pushProviderError(stream, model, err.Error())
			return
		}
		client := bedrockruntime.NewFromConfig(cfg)

		payload := BuildBedrockConverseInput(model, c, BedrockOptions{StreamOptions: streamOptions})
		if streamOptions.OnPayload != nil {
			streamOptions.OnPayload(payload)
		}

		input := buildBedrockConverseInput(model, c, streamOptions)
		resp, err := client.Converse(runCtx, input)
		if err != nil {
			pushProviderError(stream, model, err.Error())
			return
		}
		message := ai.Message{
			Role:       ai.RoleAssistant,
			API:        model.API,
			Provider:   model.Provider,
			Model:      model.ID,
			StopReason: mapBedrockStopReason(resp.StopReason),
			Timestamp:  time.Now().UnixMilli(),
		}
		if resp.Usage != nil {
			message.Usage = ai.Usage{
				Input:       int(aws.ToInt32(resp.Usage.InputTokens)),
				Output:      int(aws.ToInt32(resp.Usage.OutputTokens)),
				TotalTokens: int(aws.ToInt32(resp.Usage.TotalTokens)),
				CacheRead:   int(aws.ToInt32(resp.Usage.CacheReadInputTokens)),
				CacheWrite:  int(aws.ToInt32(resp.Usage.CacheWriteInputTokens)),
			}
			message.Usage.Cost = ai.CalculateCost(model, message.Usage)
		}
		if outputMessage, ok := resp.Output.(*bedrocktypes.ConverseOutputMemberMessage); ok {
			for _, block := range outputMessage.Value.Content {
				switch blockValue := block.(type) {
				case *bedrocktypes.ContentBlockMemberText:
					if strings.TrimSpace(blockValue.Value) == "" {
						continue
					}
					stream.Push(ai.AssistantMessageEvent{
						Type:  ai.EventTextDelta,
						Delta: blockValue.Value,
					})
					message.Content = append(message.Content, ai.ContentBlock{
						Type: ai.ContentTypeText,
						Text: blockValue.Value,
					})
				case *bedrocktypes.ContentBlockMemberReasoningContent:
					thinkingBlock := convertBedrockReasoningBlock(blockValue.Value)
					if thinkingBlock == nil {
						continue
					}
					stream.Push(ai.AssistantMessageEvent{
						Type:  ai.EventThinkingDelta,
						Delta: thinkingBlock.Thinking,
					})
					message.Content = append(message.Content, *thinkingBlock)
				case *bedrocktypes.ContentBlockMemberToolUse:
					toolCall := convertBedrockToolUseBlock(blockValue.Value)
					if toolCall == nil {
						continue
					}
					stream.Push(ai.AssistantMessageEvent{
						Type:     ai.EventToolCallEnd,
						ToolCall: toolCall,
					})
					message.Content = append(message.Content, *toolCall)
				}
			}
		}
		if message.StopReason == ai.StopReasonStop {
			for _, block := range message.Content {
				if block.Type == ai.ContentTypeToolCall {
					message.StopReason = ai.StopReasonToolUse
					break
				}
			}
		}
		stream.Push(ai.AssistantMessageEvent{
			Type:    ai.EventDone,
			Message: message,
			Reason:  message.StopReason,
		})
	}()
	return stream
}

func streamSimpleBedrockConverse(model ai.Model, c ai.Context, options *ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
	base := BuildBaseOptions(model, options, "")
	return streamBedrockConverse(model, c, &base)
}

func loadBedrockAWSConfig(ctx context.Context) (aws.Config, error) {
	if region := strings.TrimSpace(os.Getenv("AWS_REGION")); region != "" {
		return config.LoadDefaultConfig(ctx, config.WithRegion(region))
	}
	if region := strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION")); region != "" {
		return config.LoadDefaultConfig(ctx, config.WithRegion(region))
	}
	return config.LoadDefaultConfig(ctx)
}

func buildBedrockConverseInput(model ai.Model, c ai.Context, options ai.StreamOptions) *bedrockruntime.ConverseInput {
	input := &bedrockruntime.ConverseInput{
		ModelId:  aws.String(model.ID),
		Messages: convertContextToBedrockMessages(model, c),
	}
	if strings.TrimSpace(c.SystemPrompt) != "" {
		input.System = []bedrocktypes.SystemContentBlock{
			&bedrocktypes.SystemContentBlockMemberText{Value: c.SystemPrompt},
		}
	}
	if len(c.Tools) > 0 {
		input.ToolConfig = &bedrocktypes.ToolConfiguration{
			Tools: convertBedrockTools(c.Tools),
		}
		input.ToolConfig.ToolChoice = mapBedrockToolChoice("auto")
	}
	if options.MaxTokens > 0 || options.Temperature != nil {
		inference := &bedrocktypes.InferenceConfiguration{}
		if options.MaxTokens > 0 {
			inference.MaxTokens = aws.Int32(int32(options.MaxTokens))
		}
		if options.Temperature != nil {
			inference.Temperature = aws.Float32(float32(*options.Temperature))
		}
		input.InferenceConfig = inference
	}
	return input
}

func convertContextToBedrockMessages(model ai.Model, c ai.Context) []bedrocktypes.Message {
	normalized := TransformMessages(c.Messages, model, func(id string, _ ai.Model, _ ai.Message) string {
		return NormalizeBedrockToolCallID(id)
	})
	out := make([]bedrocktypes.Message, 0, len(normalized))
	for _, msg := range normalized {
		switch msg.Role {
		case ai.RoleUser:
			blocks := make([]bedrocktypes.ContentBlock, 0, max(1, len(msg.Content)))
			if strings.TrimSpace(msg.Text) != "" {
				blocks = append(blocks, &bedrocktypes.ContentBlockMemberText{Value: msg.Text})
			}
			for _, block := range msg.Content {
				if block.Type == ai.ContentTypeText && strings.TrimSpace(block.Text) != "" {
					blocks = append(blocks, &bedrocktypes.ContentBlockMemberText{Value: block.Text})
				}
			}
			if len(blocks) == 0 {
				continue
			}
			out = append(out, bedrocktypes.Message{
				Role:    bedrocktypes.ConversationRoleUser,
				Content: blocks,
			})
		case ai.RoleAssistant:
			blocks := make([]bedrocktypes.ContentBlock, 0, len(msg.Content))
			for _, block := range msg.Content {
				switch block.Type {
				case ai.ContentTypeText:
					if strings.TrimSpace(block.Text) == "" {
						continue
					}
					blocks = append(blocks, &bedrocktypes.ContentBlockMemberText{Value: block.Text})
				case ai.ContentTypeThinking:
					if strings.TrimSpace(block.Thinking) == "" {
						continue
					}
					reasoning := bedrocktypes.ReasoningTextBlock{
						Text: aws.String(block.Thinking),
					}
					if strings.TrimSpace(block.ThinkingSignature) != "" {
						reasoning.Signature = aws.String(block.ThinkingSignature)
					}
					blocks = append(blocks, &bedrocktypes.ContentBlockMemberReasoningContent{
						Value: &bedrocktypes.ReasoningContentBlockMemberReasoningText{Value: reasoning},
					})
				case ai.ContentTypeToolCall:
					blocks = append(blocks, &bedrocktypes.ContentBlockMemberToolUse{
						Value: bedrocktypes.ToolUseBlock{
							Name:      aws.String(block.Name),
							ToolUseId: aws.String(block.ID),
							Input:     bedrockdocument.NewLazyDocument(block.Arguments),
						},
					})
				}
			}
			if len(blocks) == 0 {
				continue
			}
			out = append(out, bedrocktypes.Message{
				Role:    bedrocktypes.ConversationRoleAssistant,
				Content: blocks,
			})
		case ai.RoleToolResult:
			content := make([]bedrocktypes.ToolResultContentBlock, 0, 1)
			resultText := msg.Text
			if strings.TrimSpace(resultText) == "" {
				var parts []string
				for _, block := range msg.Content {
					if block.Type == ai.ContentTypeText && strings.TrimSpace(block.Text) != "" {
						parts = append(parts, block.Text)
					}
				}
				resultText = strings.Join(parts, "\n")
			}
			if strings.TrimSpace(resultText) == "" {
				resultText = "(empty tool result)"
			}
			content = append(content, &bedrocktypes.ToolResultContentBlockMemberText{Value: resultText})
			status := bedrocktypes.ToolResultStatusSuccess
			if msg.IsError {
				status = bedrocktypes.ToolResultStatusError
			}
			out = append(out, bedrocktypes.Message{
				Role: bedrocktypes.ConversationRoleUser,
				Content: []bedrocktypes.ContentBlock{
					&bedrocktypes.ContentBlockMemberToolResult{
						Value: bedrocktypes.ToolResultBlock{
							ToolUseId: aws.String(msg.ToolCallID),
							Content:   content,
							Status:    status,
						},
					},
				},
			})
		}
	}
	return out
}

func convertBedrockTools(tools []ai.Tool) []bedrocktypes.Tool {
	out := make([]bedrocktypes.Tool, 0, len(tools))
	for _, tool := range tools {
		spec := bedrocktypes.ToolSpecification{
			Name:        aws.String(tool.Name),
			Description: aws.String(tool.Description),
			InputSchema: &bedrocktypes.ToolInputSchemaMemberJson{
				Value: bedrockdocument.NewLazyDocument(tool.Parameters),
			},
		}
		out = append(out, &bedrocktypes.ToolMemberToolSpec{Value: spec})
	}
	return out
}

func mapBedrockToolChoice(choice string) bedrocktypes.ToolChoice {
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "any":
		return &bedrocktypes.ToolChoiceMemberAny{Value: bedrocktypes.AnyToolChoice{}}
	case "none":
		return nil
	default:
		return &bedrocktypes.ToolChoiceMemberAuto{Value: bedrocktypes.AutoToolChoice{}}
	}
}

func mapBedrockStopReason(reason bedrocktypes.StopReason) ai.StopReason {
	switch reason {
	case bedrocktypes.StopReasonMaxTokens:
		return ai.StopReasonLength
	case bedrocktypes.StopReasonToolUse:
		return ai.StopReasonToolUse
	case bedrocktypes.StopReasonEndTurn, bedrocktypes.StopReasonStopSequence:
		return ai.StopReasonStop
	default:
		return ai.StopReasonError
	}
}

func convertBedrockToolUseBlock(block bedrocktypes.ToolUseBlock) *ai.ContentBlock {
	if strings.TrimSpace(aws.ToString(block.Name)) == "" {
		return nil
	}
	arguments := map[string]any{}
	if block.Input != nil {
		_ = block.Input.UnmarshalSmithyDocument(&arguments)
	}
	return &ai.ContentBlock{
		Type:      ai.ContentTypeToolCall,
		ID:        strings.TrimSpace(aws.ToString(block.ToolUseId)),
		Name:      strings.TrimSpace(aws.ToString(block.Name)),
		Arguments: arguments,
	}
}

func convertBedrockReasoningBlock(block bedrocktypes.ReasoningContentBlock) *ai.ContentBlock {
	switch value := block.(type) {
	case *bedrocktypes.ReasoningContentBlockMemberReasoningText:
		if value.Value.Text == nil || strings.TrimSpace(*value.Value.Text) == "" {
			return nil
		}
		out := &ai.ContentBlock{
			Type:     ai.ContentTypeThinking,
			Thinking: strings.TrimSpace(*value.Value.Text),
		}
		if value.Value.Signature != nil {
			out.ThinkingSignature = strings.TrimSpace(*value.Value.Signature)
		}
		return out
	case *bedrocktypes.ReasoningContentBlockMemberRedactedContent:
		if len(value.Value) == 0 {
			return nil
		}
		return &ai.ContentBlock{
			Type:     ai.ContentTypeThinking,
			Thinking: string(value.Value),
			Redacted: true,
		}
	default:
		return nil
	}
}
