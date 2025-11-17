# BeepAI OpenAI Matrix Bridge

This repository contains a reference Matrix ↔ ChatGPT bridge that closely follows the
[`bridgev2` megabridge template](https://mau.fi/blog/megabridge-twilio/) from Tulir
while keeping the deployment workflow on par with [`mautrix/whatsapp`](https://github.com/mautrix/whatsapp).

The bridge translates Matrix direct messages into OpenAI Chat Completions using the
official [`openai-go`](https://github.com/openai/openai-go) SDK and streams the responses
back into the originating Matrix room. It never initiates conversations on its own,
which keeps the threading model deterministic and predictable.

## Features

- gomatrix/mautrix `bridgev2` implementation with proper connector/login separation.
- Chat history persistence via `bridgev2`'s shared database + prompt reconstruction.
- Configurable OpenAI API key (config file or `OPENAI_API_KEY` environment variable).
- Deployment artifacts mirroring mautrix bridges (example config, registration skeleton,
  multi-stage Dockerfile, docker-compose service).
- Multi-chat UX: the bridge auto-creates the first ChatGPT DM after login (see logs from
  the `openai-chat-bootstrap` component), and every “Start new chat” action in Beeper
  spawns a fresh GPT room with isolated history.

## Requirements

- Go 1.22+
- A Matrix homeserver with appservice support.
- An OpenAI API key with access to the desired chat model.

## Getting Started

1. Copy `config.example.yaml` → `data/config.yaml` and adjust the homeserver,
   appservice, and network settings.
2. Copy `registration.yaml.example` → `data/registration.yaml`, edit tokens and upload
   it to your homeserver following its appservice registration procedure.
3. Provide the OpenAI API key either in the config under `network.openai.api_key`
   or via `OPENAI_API_KEY`.
4. Build and run:

```bash
docker compose up --build -d
# or
go build ./cmd/openai-bridge && ./openai-bridge --config config.yaml --registration registration.yaml
```

5. Invite the bridge bot to a DM, run `!gpt login`, and start chatting.

### Login flows

- **Bridge-provided API key** – if the admin configured `network.openai.api_key` (or
  `OPENAI_API_KEY`), users can pick the “Bridge-provided API key” flow. The login completes
  immediately using the shared key.
- **Personal API key** – selecting “Personal OpenAI API key” presents a text field in
  supporting clients (Element, Beeper, etc.) where users can paste their own key. The
  bridge stores it in the login metadata and uses it for all subsequent conversations.

### Multiple chats

- Immediately after login, the bridge bootstraps the first GPT DM (emitting
  `openai-chat-bootstrap` log entries) so the user can start chatting without extra
  steps.
- Beeper’s “Start New Chat → ChatGPT” entry is backed by the bridge’s
  `ResolveIdentifier` implementation, which always creates a brand-new portal. Each room
  keeps its own title, prompt, and history, so users can juggle multiple threads in
  parallel without context collisions.

## Configuration Overview

`config.example.yaml` mirrors mautrix defaults. Key sections:

- `homeserver` / `appservice`: standard mautrix fields.
- `network.openai`: API access + defaults for model, temperature, context length,
  timeout, and system prompt.
- `network.bridge`: bridge-specific UX toggles (command prefix, typing mirroring).

For connector-only deployments (e.g. mx-puppet), see `pkg/connector/example-config.yaml`.

## Development & Deployment

- `Dockerfile`: multi-stage build producing a distroless image identical to other
  mautrix bridges.
- `docker-compose.yaml`: single-service stack exposing the HTTP listener (29345 by default).
- `go.mod` pins `maunium.net/go/mautrix@main` to stay in sync with bridgev2 changes.

When upgrading mautrix, re-run `go get maunium.net/go/mautrix@main` and rebuild.

## Architecture Notes

- `pkg/connector`: OpenAI connector implementation (config, login flows, metadata,
  NetworkAPI, remote message conversions).
- `cmd/openai-bridge`: thin mxmain bootstrap identical to other mautrix bridges.
- Prompt reconstruction uses stored message metadata (role/body) from the shared DB.
- GPT responses are queued as `RemoteMessage` events so they flow through the same
  portal machinery as external networks.

## License

MPL-2.0, matching mautrix core licensing.
