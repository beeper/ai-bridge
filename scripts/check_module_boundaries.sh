#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

fail() {
  echo "boundary check failed: $1" >&2
  exit 1
}

imports_for_module() {
  local mod_dir="$1"
  (cd "$mod_dir" && go list -f '{{join .Imports "\n"}}' ./...)
}

core_imports="$(imports_for_module modules/core)"
contracts_imports="$(imports_for_module modules/contracts)"
runtime_imports="$(imports_for_module modules/runtime)"
simple_imports="$(imports_for_module modules/simple)"
aiproxy_imports="$(imports_for_module modules/aiproxy)"

if echo "$contracts_imports" | rg -q '^github.com/beeper/ai-bridge/modules/core'; then
  fail "modules/contracts must not import modules/core"
fi

if echo "$contracts_imports" | rg -q '^github.com/beeper/ai-bridge/modules/(runtime|simple)'; then
  fail "modules/contracts must not import feature/runtime modules"
fi

if echo "$runtime_imports" | rg -q '^github.com/beeper/ai-bridge/modules/simple'; then
  fail "modules/runtime must not import modules/simple"
fi

for imports in "$core_imports" "$runtime_imports" "$simple_imports" "$aiproxy_imports"; do
  if echo "$imports" | rg -q 'github.com/(batuhan/beeper-codex|batuhan/beeper-opencode|beeper/beep)'; then
    fail "shared modules must not import dedicated runtime repos"
  fi
done

echo "module boundary checks passed"
