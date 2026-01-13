# ai-bridge

Multi-provider AI bridge for Beeper. Supports OpenAI, Anthropic, Google Gemini, and OpenRouter via the mautrix-go bridgev2 framework.

## Features

- **Multi-provider support** with intelligent model routing (prefix-based: `openai/gpt-4o`, `anthropic/claude-3-5-sonnet`, etc.)
- **Per-model contacts** - each model appears as a separate chat contact
- **Streaming responses** with debounced progressive rendering
- **Vision support** - automatic capability detection for image uploads
- **Per-room configuration** - model, temperature, system prompt, context length
- **User-provided API keys** - per-user billing via login flow
- **Chat history persistence** with prompt reconstruction

## Supported Providers

| Provider | Example Models |
|----------|----------------|
| OpenAI | gpt-4o, gpt-4o-mini, o1, o3-mini |
| Anthropic | claude-3-5-sonnet, claude-3-opus, claude-3-haiku |
| Google | gemini-2.0-flash, gemini-1.5-pro |
| OpenRouter | Various (pass-through) |

## Setup

Generate config with bbctl:

```bash
bbctl config --type bridgev2 sh-ai
```

Add the `network` section from `config.example.yaml`, then run:

```bash
./ai-bridge --config config.yaml
```

## Login Flows

- **Shared key** - Set `network.openai.api_key` in config or `OPENAI_API_KEY` env var
- **Personal key** - User selects provider and enters API key in Beeper
- **Beeper SDK** - Pre-configured credentials from Beeper infrastructure

## Configuration

Key settings under `network`:

```yaml
network:
  openai:
    api_key: ""                    # Shared API key (or use OPENAI_API_KEY env)
    default_model: gpt-4o-mini
    default_temperature: 0.3
    max_context_messages: 12
    max_completion_tokens: 512
    system_prompt: ""
    request_timeout: 45s
    enable_streaming: true
  bridge:
    command_prefix: "!ai"
    typing_notifications: true
```

See `config.example.yaml` for full options.

## Build

Requires libolm for encryption support.

```bash
./build.sh
```

Or use Docker:

```bash
docker build -t ai-bridge .
```

## Architecture

- `cmd/ai-bridge/` - mxmain bootstrap
- `pkg/connector/` - Provider implementations, message handling, login flows
