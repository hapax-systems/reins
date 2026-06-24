# reins

A single-surface terminal cockpit for an operator's entire software/research delivery
lifecycle — observe the live system, navigate its registries, and (soon) drive it, all
from one BBS/ASCII-grammar screen. Built to consolidate scattered dashboards, CLIs, and
session windows into one place.

```
REINS  LOCAL  :events  n:80
────────────────────────────────────────────────────────────────
01:53:07 ▏▸ reques
02:15:50 ▎✖ p0-inc
02:31:49 ▏▸ reques
────────────────────────────────────────────────────────────────
[:]cmd [1]events [2]tasks  [a]AIR  [q]quit
```

## Why it looks like this

The **cell** — one character position — is the unit of design, not the widget. A glyph
carries *kind* by semantic class (`▸` in-progress · `✓` success · `✖` failure · `⇡`
advance · `⚑` flag · `◆` task), an eighth-block bar carries *magnitude*, and emptiness
reads as all-clear. The grammar is monochrome-safe first (color is redundant
reinforcement, never the only signal) so it survives a livestream re-encode, a 16-color
terminal, and a screen-reader pass alike.

## Architecture

Three layers, one boundary that matters:

```
  SUBSTRATE            UNIFIED READ API           COCKPIT
  (HOS event spine) ─▶ (thin, scored,        ─▶  (Go / Bubble Tea,
   append-only,         air-classified            stateless fold,
   foldable)            HTTP/JSON)                 hot-reloadable)
                        │                          │
                  api/reins_read.py          cmd/reins + internal/*
                  (Python; folds the         (Go; holds NO
                   live ledger)               authoritative state)
```

- **The language boundary is the API boundary.** The Go cockpit never imports the
  substrate; it speaks HTTP to a thin Python service that folds the live event log. Swap
  either side without touching the other.
- **Stateless fold (Elm M-U-V).** The cockpit's view is a pure function of fetched data.
  Re-fetch + re-fold restores the exact view — so a code change hot-reloads and a crash
  re-exec is lossless. The cockpit is a *lens*, never a source of truth.
- **Command-as-effect.** Every typed command (`:tasks`, `:air on`, …) is one pure model
  transition. Read-verbs today; write-verbs will route through the same grammar onto the
  API's command surface tomorrow.

## Engine vs. instance

This repo is the **engine** — zero baked paths, zero operator-specific knowledge. All
deployment facts (where the substrate lives, the API URL, the on-air allowlist, the
palette) are **instance config**, injected at runtime. That separation is what lets the
same binary package for someone else's lifecycle, not just this one.

```toml
# config.toml  (see config.example.toml)
api_url       = "http://127.0.0.1:8811"
council_root  = "~/projects/hapax-spine"   # substrate root (READ API side)
ledger_path   = "~/.cache/hapax/coord/ledger.db"
palette       = "gruvbox"
air_allowlist = ["kind","subject","score","ts","task_id","stage","no_go"]
```

Config is resolved from `$REINS_CONFIG`, else `~/.config/reins/config.toml`.

## PII-safe / on-air mode

Reins is meant to be visible on a livestream, so redaction is **default-deny** and lives
at the renderer, not in a post-hoc filter. Two render targets from one model:

- **LOCAL** — full fidelity, operator's eyes only.
- **AIR** — every cell whose field is not on the `air_allowlist` renders as `▒▒▒`. New
  fields are denied until explicitly allowed; there is no way to leak by forgetting to
  redact.

Toggle with `[a]` or `:air on|off`. The status bar shows `AIR ▮` whenever the lens is on.

## Run it

```sh
# 1. the READ API (Python; needs the substrate importable — fastapi + uvicorn)
REINS_PORT=8811 REINS_COUNCIL_ROOT=~/projects/hapax-spine \
  python api/reins_read.py

# 2. the cockpit (Go)
go run ./cmd/reins                 # interactive (alt-screen TUI)
go run ./cmd/reins --probe         # headless: fetch, fold, print one frame, exit
go run ./cmd/reins --probe tasks --air   # the :tasks page through the AIR lens
go run ./cmd/reins --probe "cmd:air on"  # exercise the command path headless
```

Keys: `[:]` command line · `[1]` events · `[2]` tasks · `[a]` AIR lens · `[q]` quit.

## Test

```sh
go test ./...                      # Go: config, grammar, model, cmd
( cd api && python -m pytest -q )  # Python: the READ service
```

## Layout

```
cmd/reins/          the binary — config load, poll/re-fold loop, --probe acceptance
internal/config/    instance config loader (engine/instance separation)
internal/grammar/   the cell-grammar — glyph alphabet, score-bar, AIR redaction, row renderers
internal/model/     the Bubble Tea model — pure fold, page-switch, command-as-effect (Exec)
internal/api/       the READ client (Go side of the boundary)
api/reins_read.py   the thin READ service (Python side — folds the live spine)
```

## Status

Early, but live end-to-end against a real substrate. Working today: the vital frame,
`:events` and `:tasks` pages over the live event log, the cell-grammar, the AIR lens,
hot-reload, and the command line. Next: an ASCII dependency-graph page, a multi-session
agent client (so the operator's working sessions live *in* the cockpit), and the write
side of the command surface.
