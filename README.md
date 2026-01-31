# Bridge AI chats into Beeper

Highly experimental Matrix ↔︎ AI bridge for Beeper, built on top of [mautrix/bridgev2](https://pkg.go.dev/maunium.net/go/mautrix/bridgev2). Supports any OpenAI-compatible provider, including OpenRouter.

Currently best works with alpha versions of Beeper Desktop. Beeper Plus users can use it without providing their own keys by picking the Beeper AI provider when adding an account.

## Highlights

- **Multi-provider routing** with prefixed model IDs (for example `openai/o3-mini`, `openai/gpt-5.2`, `anthropic/claude-sonnet-4.5` via OpenRouter)
- **Per-model contacts** so each model appears as its own chat contact
- **Streaming responses** with status updates
- **Multimodal input** (images, PDFs, audio, video) when the selected model supports it
- **Per-room settings** for model, temperature, system prompt, context limits, and tools
- **User-managed keys** via login flow, plus optional Beeper-managed credentials

## When to use what

Common to all methods:
- **One-click setup** for all Beeper Plus users (uses Beeper AI servers, rate limits apply)
- **BYOK** for any OpenAI Responses API compatible provider
  - Supports local LLM providers when running with On-Device or Self-Hosted

| Method | Sync | Where requests run | Local LLMs | Notes |
| --- | --- | --- | --- | --- |
| Beeper Cloud | Syncs to all devices | Beeper servers | No | Good default when you want full sync and zero setup |
| Beeper On-Device | No sync | Your device | Yes | Everything stays on-device; Beeper never sees your messages |
| Beeper Self-Hosted | Syncs to all devices, encrypted | Your bridge host | Yes* | Beeper never sees your messages |

* Local LLMs require an OpenAI-compatible provider endpoint.

## Build

Requires libolm for encryption support.

```bash
./build.sh
```

Or use Docker:

```bash
docker build -t ai-bridge .
```
