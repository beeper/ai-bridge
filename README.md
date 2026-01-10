# ai-bridge

This repository contains a reference Matrix ↔ OpenAI bridge that closely follows the
[`bridgev2` megabridge template](https://mau.fi/blog/megabridge-twilio/)

This is a generic AI bridge able to be setup for different model providers. Currently it supports OpenAI's chat completion API.

## Features

- gomatrix/mautrix `bridgev2` implementation with proper connector/login separation.
- **Model selection** – Each OpenAI model appears as a separate contact; create separate chats with different models.
- **Dynamic capabilities** – Rooms automatically adjust based on model (vision support, etc.).
- **Streaming responses** – Progressive rendering with debounced edits.
- Chat history persistence via `bridgev2`'s shared database + prompt reconstruction.
- Configurable OpenAI API key (config file or `OPENAI_API_KEY` environment variable).
- Support for user-provided API keys (per-user billing).
- Deployment artifacts mirroring mautrix bridges (example config, registration skeleton, multi-stage Dockerfile).


## Getting Started

Use bridge manager to generate your config:

```
If you have a 3rd party bridge that's built on top of mautrix-go's bridgev2 framework, you can have bbctl generate a mostly-complete config file:

Run bbctl config --type bridgev2 <name> to generate a bridgev2 config with everything except the network section.
<name> is a short name for the bridge (a-z, 0-9, -). The name should start with sh-. The bridge user ID namespace will be @<name>_.+:beeper.local and the bridge bot will be @<name>bot:beeper.local.
Add the network section containing the bridge-specific configuration if necessary, then run the bridge normally.
```

`./ai-bridge --config config.yaml --registration registration.yaml`

### Build notes

Its setup to use libolm.

### Login flows

- **config-provided API key** – Shared API key in config.yaml for all users.
- **user-provided API key** – Login prompt in Beeper to enter your own API key.

### Multiple chats & model selection

- Use "Start New Chat" in Beeper to view all available OpenAI models as contacts.
- Each model appears as a separate contact (e.g., "GPT-4o (Vision)", "GPT-4o Mini", "GPT-4 Turbo").
- Create multiple conversations with different models.
- Each room automatically adjusts capabilities based on model (vision models support image uploads).
- Room-specific settings (model, temperature, system prompt, etc.).

## Configuration Overview

`config.example.yaml` mirrors mautrix defaults. Key sections:

- `homeserver` / `appservice`: standard mautrix fields.
- `network.openai`: API access + defaults for model, temperature, context length,
  timeout, and system prompt.
- `network.bridge`: bridge-specific UX toggles (command prefix, typing mirroring).

For connector-only deployments (e.g. mx-puppet), see `pkg/connector/example-config.yaml`.

## Architecture Notes

- `pkg/connector`: OpenAI connector implementation (config, login flows, metadata,
  NetworkAPI, remote message conversions).
- `cmd/openai-bridge`: thin mxmain bootstrap identical to other mautrix bridges.
- Prompt reconstruction uses stored message metadata (role/body) from the shared DB.
- GPT responses are queued as `RemoteMessage` events so they flow through the same
  portal machinery as external networks.
