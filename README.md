# WIP ai-bridge

This repository contains a reference Matrix ↔ ChatGPT bridge that closely follows the
[`bridgev2` megabridge template](https://mau.fi/blog/megabridge-twilio/)

Heavily AI assisted.

I imagine this being a multi network bridge able to be setup for all the different model providers, though, only one at a time due to
bridge limitations. This is why it is a generic ai bridge instead of chatgpt specifically

## Features

- gomatrix/mautrix `bridgev2` implementation with proper connector/login separation.
- Chat history persistence via `bridgev2`'s shared database + prompt reconstruction.
- Configurable OpenAI API key (config file or `OPENAI_API_KEY` environment variable).
- Deployment artifacts mirroring mautrix bridges (example config, registration skeleton,
  multi-stage Dockerfile, docker-compose service).

### WIP

- There is unused/not working logic to be able to start new chats with the bridge 


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

- **config-provided API key** – there is a spot in config.yaml to provide an api key if self hosting
- **WIP - setup provided API key** – this should present a login prompt in beeper to enter your api key on setup if the config api key is empty

### Multiple chats

- Immediately after login, the bridge bootstraps the first GPT DM (emitting
  `openai-chat-bootstrap` log entries) so the user can start chatting without extra
  steps.
- **WIP** Beeper’s “Start New Chat → ChatGPT” entry is backed by the bridge’s
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

## Architecture Notes

- `pkg/connector`: OpenAI connector implementation (config, login flows, metadata,
  NetworkAPI, remote message conversions).
- `cmd/openai-bridge`: thin mxmain bootstrap identical to other mautrix bridges.
- Prompt reconstruction uses stored message metadata (role/body) from the shared DB.
- GPT responses are queued as `RemoteMessage` events so they flow through the same
  portal machinery as external networks.
