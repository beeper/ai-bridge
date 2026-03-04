# AGENTS.md

Guidelines for coding agents working in this repository.

## Scope

These instructions apply to the entire repository unless a deeper, directory-local
AGENTS.md file overrides them.

## Session startup checklist

1. Read `README.md` for project overview and supported workflows.
2. Read relevant docs before coding:
   - `docs/matrix-ai-matrix-spec-v1.md` for Matrix transport behavior.
   - `docs/bridge-orchestrator.md` for bridge instance orchestration.
3. Inspect current workspace state (`git status`) before editing.
4. Keep changes focused to the user request.

## Build and test commands

- Build: `./build.sh`
- Run all tests: `go test ./...`
- Run package tests: `go test ./pkg/<package>`
- Run one test: `go test ./pkg/<package> -run <TestName>`

When you change behavior, add or update tests in the same area when feasible.

## Coding conventions

- Prefer small, targeted diffs over broad refactors.
- Follow existing Go style and naming in the touched package.
- Keep functions readable and avoid unnecessary abstraction.
- Do not introduce new dependencies unless required by the task.
- Update docs when user-visible behavior or commands change.

## Safety and operations

- Never commit secrets or credentials.
- Do not revert unrelated user changes.
- Avoid destructive commands unless explicitly requested.
- Prefer `rg` for search and keep command usage precise.

## Git workflow

- Create one commit per logical change.
- Use descriptive commit messages.
- Push commits after local verification is complete.

