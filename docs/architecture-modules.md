# ai-bridge Workspace Layout

This repository contains shared modules and the simple bridge product.

## Local Bridge Binary

- `bridges/simple`

The simple bridge has its own `go.mod`, `cmd/bridge/main.go`, and `config.example.yaml`.

## Shared Modules

- `modules/contracts`: shared Matrix AI contract/schema surfaces (`com.beeper.ai.*`).
- `modules/core`: shared bridge kernel and policy composition used by simple and downstream repos.

## Dedicated Runtime Repos

Dedicated runtimes were split out into sibling repositories:

- `../beep` (agentic)
- `../beeper-codex` (codex)
- `../beeper-opencode` (opencode)

Downstream repos pin `github.com/beeper/ai-bridge` modules by SHA, with local `replace` directives only for development.

## Boundary Guardrails

`./scripts/check_module_boundaries.sh` validates:

- contracts do not import core modules.
- core does not import dedicated runtime repos.
