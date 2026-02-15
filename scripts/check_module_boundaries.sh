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

contracts_imports="$(imports_for_module modules/contracts)"
runtime_imports="$(imports_for_module modules/runtime)"
simple_imports="$(imports_for_module modules/simple)"
aiproxy_imports="$(imports_for_module modules/aiproxy)"

if echo "$contracts_imports" | rg -q '^github.com/beeper/ai-bridge/modules/(runtime|simple)'; then
  fail "modules/contracts must not import feature/runtime modules"
fi

if echo "$runtime_imports" | rg -q '^github.com/beeper/ai-bridge/modules/simple'; then
  fail "modules/runtime must not import modules/simple"
fi

for imports in "$runtime_imports" "$simple_imports" "$aiproxy_imports"; do
  if echo "$imports" | rg -q 'github.com/(batuhan/beeper-codex|batuhan/beeper-opencode|beeper/beep)'; then
    fail "shared modules must not import dedicated runtime repos"
  fi
done

ai_bridge_deps="$(go list -deps ./bridges/ai/cmd/bridge)"
if echo "$ai_bridge_deps" | rg -q 'github.com/beeper/ai-bridge/pkg/simpleruntime/simpledeps/'; then
  fail "ai bridge must not depend on pkg/simpleruntime/simpledeps/*"
fi

# pkg/core/ must never import mautrix (Matrix-free AI primitives).
if rg -q '"maunium.net/go/mautrix' pkg/core/; then
  fail "pkg/core/ must not import mautrix"
fi

# Disallow compatibility/shim files in active runtime paths.
if rg --files pkg modules bridges | rg -qi '(_compat\.go$|(^|/)compat_|(^|/)shim_|_shim\.go$)'; then
  fail "compatibility/shim filenames are not allowed in pkg/, modules/, or bridges/"
fi

# Disallow compatibility/shim markers in active runtime paths.
if rg -n 'compat|shim|alias wrapper|legacy adapter' pkg modules bridges -g'*.go' -g'*.md' >/dev/null; then
  fail "compatibility/shim markers are not allowed in active runtime paths"
fi

# Disallow type alias declarations in active runtime paths.
if rg -n "^[[:space:]]*type[[:space:]]+[A-Za-z_][A-Za-z0-9_]*[[:space:]]*=" pkg modules bridges -g'*.go' >/dev/null; then
  fail "type alias declarations are not allowed in active runtime paths"
fi
if rg --pcre2 -U -n "(?s)type\\s*\\([^)]*?\\b[A-Za-z_][A-Za-z0-9_]*\\s*=\\s*[^\\n]+\\)" pkg modules bridges -g'*.go' >/dev/null; then
  fail "type alias declarations are not allowed in active runtime paths"
fi

# pkg/matrixai/ must not import simpleruntime or bridge-specific code.
if rg -q 'github.com/beeper/ai-bridge/pkg/simpleruntime' pkg/matrixai/; then
  fail "pkg/matrixai/ must not import simpleruntime"
fi

# Shared pkg/* packages must not import simpleruntime or dedicated repos.
shared_pkgs=(core/aierrors core/aimedia core/aimodels core/aiprovider core/aiqueue core/aitokens core/aityping core/aiutil matrixai/linkpreview)
for pkg in "${shared_pkgs[@]}"; do
  pkg_imports="$(go list -f '{{join .Imports "\n"}}' "./pkg/$pkg/" 2>/dev/null || true)"
  if [ -n "$pkg_imports" ]; then
    if echo "$pkg_imports" | rg -q 'github.com/beeper/ai-bridge/pkg/simpleruntime'; then
      fail "pkg/$pkg must not import simpleruntime"
    fi
    if echo "$pkg_imports" | rg -q 'github.com/(batuhan/beeper-codex|batuhan/beeper-opencode|beeper/beep)'; then
      fail "pkg/$pkg must not import dedicated runtime repos"
    fi
  fi
done

echo "module boundary checks passed"
