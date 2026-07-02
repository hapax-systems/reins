#!/usr/bin/env sh
# reins install.sh — install the reins cockpit from a checked-out repo. The FROM-SOURCE path: works today,
# no release channel required (a `curl … | sh` release installer lands when GoReleaser publishes — see
# .goreleaser.yml). Under scripts/ so the engine/instance guard (test_engine_instance_guard.py) scans it —
# this installer carries NO baked home paths (only $HOME / computed $REPO), the kit-packaging invariant.
#
# Usage:  sh scripts/install.sh            # install the cockpit + materialize config
#         PREFIX=/opt sh scripts/install.sh
set -eu

REPO="$(cd "$(dirname "$0")/.." && pwd)"       # scripts/ -> repo root
PREFIX="${PREFIX:-$HOME/.local}"
VERSION="$(cat "$REPO/VERSION" 2>/dev/null || echo dev)"

echo "reins $VERSION — installing from $REPO"

# 1. the cockpit binary (VERSION-stamped; rename-over so a RUNNING cockpit survives the swap, ETXTBSY-safe)
command -v go >/dev/null 2>&1 || { echo "  ! the Go toolchain is required (https://go.dev/dl)" >&2; exit 1; }
make -C "$REPO" install PREFIX="$PREFIX"

# 2. the instance config — materialize from the example, NEVER clobber an existing one (engine/instance:
#    the example carries ~-relative placeholders; a real instance edits its own config)
CONF="${REINS_CONFIG:-$HOME/.config/reins/config.toml}"
if [ ! -f "$CONF" ]; then
  mkdir -p "$(dirname "$CONF")"
  cp "$REPO/config.example.toml" "$CONF"
  echo "  wrote $CONF  (edit it for your instance)"
else
  echo "  kept existing $CONF"
fi

# 3. the read API is only needed for a LIVE instance — the demo needs NONE. Provision best-effort with uv.
if command -v uv >/dev/null 2>&1; then
  if uv venv "$REPO/api/.venv" >/dev/null 2>&1 && \
     "$REPO/api/.venv/bin/python" -m ensurepip >/dev/null 2>&1 && \
     uv pip install --python "$REPO/api/.venv/bin/python" fastapi uvicorn >/dev/null 2>&1; then
    echo "  read API deps provisioned -> $REPO/api/.venv (uv, never pip)"
  else
    echo "  (read API deps not auto-provisioned — for a live instance: cd $REPO/api && uv venv && uv pip install fastapi uvicorn)"
  fi
else
  echo "  (uv not found — the DEMO below needs nothing; for a live instance install uv + the read API deps)"
fi

echo ""
echo "reins $VERSION installed -> $PREFIX/bin/reins"
echo "  try it now, no estate needed:   reins --demo"
echo "  a live instance:                make -C $REPO up   (starts the read API + cockpit)"
