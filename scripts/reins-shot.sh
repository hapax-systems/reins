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
}

bindir="$(mktemp -d)"
go -C "$REPO" build -o "$bindir/reins" ./cmd/reins
ansi="$(mktemp).ansi"
"$bindir/reins" --drive "$spec" "$size" ${extra+"${extra[@]}"} > "$ansi"
# freeze v0.2.2 traps — BOTH guards are mandatory for a reproducible AVSDLC raster:
#  1. main.go:149 `if config.Input == "-" || in.IsPipe(os.Stdin)` with input/input.go:26
#     `return (stat.Mode() & os.ModeCharDevice) == 0` means freeze SILENTLY IGNORES the file
#     argument and renders stdin whenever stdin is a pipe OR a redirected file (ssh, CI,
#     `bash -s < script`, heredocs). </dev/null is a char device, so the file arg is honoured.
#  2. png.go:14 prefers the host's rsvg-convert, which ignores the SVG's embedded WOFF2
#     @font-face and resolves via fontconfig; `fc-match "JetBrains Mono"` returns a
#     proportional fallback. Content then compresses leftward: left_wall/entity_core are
#     INFLATED and right_wall is VACATED. Measured: it minted PASS for identity-pane and
#     relational-pane that FAIL at a correct raster, and minted the dispatch-pane FAIL.
#     Only freeze's vendored resvg path loads the embedded font. Hiding PATH forces it.
# Do NOT pin geometry with --width/--height: main.go:130-139 sets scale=4 only when both
# are auto, so passing them silently quarters the raster and moves every metric.
env -i HOME="$HOME" PATH=/nonexistent "$freeze_bin" \
  "$ansi" --language ansi --output "$out" < /dev/null >/dev/null
echo "wrote $out  (spec: $spec  $size ${extra[*]:-})"
