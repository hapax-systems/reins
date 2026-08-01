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

command -v freeze >/dev/null 2>&1 || { echo "freeze not found (go install github.com/charmbracelet/freeze@v0.2.2)" >&2; exit 1; }
freeze_bin="$(command -v freeze)"
# PIN AND VERIFY. The two guards below encode defects MEASURED in freeze v0.2.2 (the stdin trap and
# the host-rsvg font substitution). A different build may fix, move or reintroduce them, so a raster
# taken with an unverified freeze is not evidence — fail loud, never render.
#
# `--version` alone is only what the binary CLAIMS. `go version -m` reads the module identity and the
# go.sum checksum stamped into the executable at build time, which `go install` verified against the
# checksum database. That is a real provenance check and, unlike a per-platform binary sha256, it
# does not vary with the local toolchain. `|| true` on the probes so `set -euo pipefail` cannot exit
# before the explicit guard below reports WHICH check failed.
freeze_want_ver="v0.2.2"
freeze_want_sum="h1:pBXkyFXcj8UBCZZTQ7qNy4Xgv/w4RBcOiWtdxvHeJco="
command -v go >/dev/null 2>&1 || { echo "go is required to verify freeze's provenance" >&2; exit 1; }
freeze_mod="$(go version -m "$freeze_bin" 2>/dev/null | awk '$1=="mod" && $2=="github.com/charmbracelet/freeze" {print $3" "$4; exit}')" || true
freeze_have_ver="${freeze_mod%% *}"
freeze_have_sum="${freeze_mod##* }"
[ "$freeze_have_ver" = "$freeze_want_ver" ] && [ "$freeze_have_sum" = "$freeze_want_sum" ] || {
  echo "freeze provenance check FAILED — refusing to render an unattributable raster" >&2
  echo "  want: $freeze_want_ver $freeze_want_sum" >&2
  echo "  have: ${freeze_have_ver:-unknown} ${freeze_have_sum:-unknown}" >&2
  echo "  go install github.com/charmbracelet/freeze@$freeze_want_ver" >&2
  exit 1
}   # see the trap note in reins-shot.sh
pass=0; fail=0; head="$(git -C "$REPO" rev-parse --short HEAD)"
printf 'reins AVSDLC suite @ %s%s\n' "$head" "${LIVE:+ (live)}"
bin="$TMP/reins"
go -C "$REPO" build -o "$bin" ./cmd/reins   # build ONCE — the renders reuse it (was: rebuild per pane)
for entry in "${PANES[@]}"; do
  spec="${entry%%|*}"; intent="${entry##*|}"
  png="$TMP/${intent}.png"
  "$bin" --drive "$spec" size:160x44 $LIVE > "$TMP/frame.ansi" 2>/dev/null
  env -i HOME="$HOME" PATH=/nonexistent "$freeze_bin" \
    "$TMP/frame.ansi" --language ansi --output "$png" < /dev/null >/dev/null 2>&1
  # persist the dossier (G7: the cockpit-legibility receipt previously never reached disk — the suite
  # ran the witness but discarded it; a witness that leaves no receipt is prose, not evidence)
  if python3 "$REPO/scripts/reins-avsdlc-witness.py" --frame "$png" \
       --intent "$REPO/docs/avsdlc/intents/${intent}.json" --pov local-terminal \
       --source-head "$head" --out "$REPO/docs/releases/${intent}-witness" >/dev/null 2>&1; then
    printf '  PASS  %-14s %s\n' "$intent" "$spec"; pass=$((pass+1))
  else
    printf '  FAIL  %-14s %s\n' "$intent" "$spec"; fail=$((fail+1))
  fi
done
printf '%d passed · %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
