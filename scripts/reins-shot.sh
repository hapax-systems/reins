#!/usr/bin/env bash
# reins-shot — render a Reins cockpit navigation frame to a PNG for visual inspection / AVSDLC
# capture, headless (no terminal, no human). The PNG is the AVSDLC visual witness artifact.
#
# Usage:
#   scripts/reins-shot.sh "<drive-spec>" <out.png> [size:WxH] [--air] [--live]
# where <drive-spec> is a ';'-separated step list fed to `reins --drive`, e.g.:
#   scripts/reins-shot.sh ":capabilities; j" /tmp/caps.png size:170x46
#   scripts/reins-shot.sh ":tasks; j; v; a" /tmp/arm.png size:160x44 --air
#
# Requires `freeze` (github.com/charmbracelet/freeze) on PATH or in ~/go/bin.
set -euo pipefail
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PATH="$HOME/go/bin:$PATH"

spec="${1:?usage: reins-shot.sh \"<drive-spec>\" <out.png> [size:WxH] [--air] [--live]}"
out="${2:?missing output .png path}"
shift 2

size="size:170x46"
extra=()
for a in "$@"; do
  case "$a" in
    size:*) size="$a" ;;
    *) extra+=("$a") ;;
  esac
done

command -v freeze >/dev/null 2>&1 || { echo "freeze not found (go install github.com/charmbracelet/freeze@latest)" >&2; exit 1; }

bindir="$(mktemp -d)"
go -C "$REPO" build -o "$bindir/reins" ./cmd/reins
ansi="$(mktemp).ansi"
"$bindir/reins" --drive "$spec" "$size" ${extra+"${extra[@]}"} > "$ansi"
freeze "$ansi" --language ansi --output "$out" >/dev/null
echo "wrote $out  (spec: $spec  $size ${extra[*]:-})"
