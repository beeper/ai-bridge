# Bridge AI chats into Beeper

AI bridge is a Matrix <-> AI bridge for Beeper built on mautrix-go bridgev2. It routes chats to models via OpenAI and OpenRouter (which includes Anthropic, Google, Meta, and more). Designed for Beeper Desktop.
Rich features only work with alpha versions of Beeper Desktop. This bridge is highly experimental.

## Highlights

- **Multi-provider routing** with prefixed model IDs (for example `openai/o3-mini`, `openai/gpt-5.2`, `anthropic/claude-sonnet-4.5` via OpenRouter)
- **Per-model contacts** so each model appears as its own chat contact
- **Streaming responses** with status updates
- **Multimodal input** (images, PDFs, audio, video) when the selected model supports it
- **Per-room settings** for model, temperature, system prompt, context limits, and tools
- **User-managed keys** via login flow, plus optional Beeper-managed credentials

## Providers and example models

- **OpenAI**: `openai/o1`, `openai/o1-mini`, `openai/o3-mini`, `openai/gpt-4-turbo`
- **OpenRouter**: `openai/gpt-5.2`, `anthropic/claude-sonnet-4.5`, `google/gemini-3-pro-preview`, `meta-llama/llama-4-maverick`, `qwen/qwen3-235b-a22b`
- **Beeper aliases**: `beeper/default`, `beeper/fast`, `beeper/smart`, `beeper/reasoning`

## Build

Requires libolm for encryption support.

```bash
./build.sh
```

Or use Docker:

```bash
docker build -t ai-bridge .
```
