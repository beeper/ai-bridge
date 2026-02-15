# ai-bridge Workspace Layout

This repository contains the shared AI/Matrix libraries and the simple bridge product.

## Local Bridge Binary

- `bridges/ai`

The simple bridge has its own `go.mod`, `cmd/bridge/main.go`, and `config.example.yaml`.

## Shared Modules

- `modules/contracts`: shared Matrix AI event contract/schema surfaces (`com.beeper.ai.*`).
- `modules/runtime`: shared command registry/runtime interfaces.
- `modules/aiproxy`: shared proxy clients for search/fetch/media.

## Shared Library Packages

- `pkg/core/*`: provider-agnostic AI primitives (no Matrix imports).
- `pkg/matrixai/*`: Matrix event/storage/runtime helpers used by bridges.
- `pkg/airuntime/*`: bridge runtime streaming/tool plumbing.

## Dedicated Runtime Repos

Dedicated runtimes live in sibling repositories:

- `../beep` (agentic runtime)
- `../beeper-codex` (codex-only)
- `../beeper-opencode` (opencode-only)

Downstream repos pin `github.com/beeper/ai-bridge` by SHA-based pseudo versions.
Local `replace` directives are development-only.

## Boundary Guardrails

`./scripts/check_module_boundaries.sh` validates:

- shared modules do not import dedicated runtime repos.
- retired `pkg/simpleruntime` paths are not referenced.
- type aliases and compat/shim layers are disallowed in active runtime paths.
