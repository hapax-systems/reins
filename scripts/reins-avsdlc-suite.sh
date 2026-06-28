#!/usr/bin/env bash
# reins-avsdlc-suite — render + AVSDLC-confirm every pane that has a pre-authored intent, in one
# pass. The reproducible VISUAL regression check: a pane whose realized frame stops satisfying its
# intent (legibility / dark-theme / structure) fails here. Headless, no human.
#
# Usage: scripts/reins-avsdlc-suite.sh [--live]   (default: the deterministic offline seed)
set -euo pipefail
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PATH="$HOME/go/bin:$PATH"
LIVE="${1:-}"
TMP="$(mktemp -d)"

# pane drive-spec : intent file (under docs/avsdlc/intents/)
PANES=(
  ":coordinator|cockpit-legibility"
  ":axes|axes-pane"
  ":identity; j|identity-pane"
  ":relational; j|relational-pane"
)

command -v freeze >/dev/null 2>&1 || { echo "freeze not found (go install github.com/charmbracelet/freeze@latest)" >&2; exit 1; }
pass=0; fail=0; head="$(git -C "$REPO" rev-parse --short HEAD)"
printf 'reins AVSDLC suite @ %s%s\n' "$head" "${LIVE:+ (live)}"
bin="$TMP/reins"
go -C "$REPO" build -o "$bin" ./cmd/reins   # build ONCE — the renders reuse it (was: rebuild per pane)
for entry in "${PANES[@]}"; do
  spec="${entry%%|*}"; intent="${entry##*|}"
  png="$TMP/${intent}.png"
  "$bin" --drive "$spec" size:160x44 $LIVE > "$TMP/frame.ansi" 2>/dev/null
  freeze "$TMP/frame.ansi" --language ansi --output "$png" >/dev/null 2>&1
  if python3 "$REPO/scripts/reins-avsdlc-witness.py" --frame "$png" \
       --intent "$REPO/docs/avsdlc/intents/${intent}.json" --pov local-terminal \
       --source-head "$head" >/dev/null 2>&1; then
    printf '  PASS  %-14s %s\n' "$intent" "$spec"; pass=$((pass+1))
  else
    printf '  FAIL  %-14s %s\n' "$intent" "$spec"; fail=$((fail+1))
  fi
done
printf '%d passed · %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
