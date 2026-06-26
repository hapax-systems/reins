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
[:]cmd [1]events [2]tasks [3]sessions  [a]AIR  [q]quit
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
- **Command catalog, then command-as-effect.** Every typed command (`:tasks`, `:air on`,
  `:intent dispatch`, …) is registered metadata first, then one pure model transition.
  Read-verbs today; mutation remains an intent preview stub until it can route through a
  governed API command surface with preflight and receipts.

## Engine vs. instance

This repo is the **engine** — zero baked paths, zero operator-specific knowledge. All
deployment facts (where the substrate lives, the API URL, the on-air allowlist, the
palette) are **instance config**, injected at runtime. That separation is what lets the
same binary package for someone else's lifecycle, not just this one.

```toml
# config.toml  (see config.example.toml)
api_url       = "http://127.0.0.1:8799"
council_root  = "~/projects/hapax-spine"   # substrate root (READ API side)
ledger_path   = "~/.cache/hapax/coord/ledger.db"
cc_tasks_active = "~/Documents/Personal/20-projects/hapax-cc-tasks/active"
session_transcript_roots = []  # metadata refs only; raw transcript contents are not read
orchestration_ledger_dir = "~/.cache/hapax/orchestration"  # route decision/dispatch receipts
lifecycle_registry_paths = ["~/projects/reins/docs/lifecycle-registries/hapax-lifecycle-registry-20260625.json"]  # optional authority-aware SDLC/RDLC/n-DLC lifecycle contracts
domain_pack_paths = ["~/projects/reins/docs/domain-packs/hapax-domain-pack-20260625.json"]  # optional source-backed SDLC/RDLC/n-DLC domain packs
capability_surface_pack_paths = ["~/projects/reins/docs/capability-surface-packs/hapax-capability-surface-pack-20260625.json"]  # optional source-backed capability-surface discovery packs
palette       = "gruvbox"
air_allowlist = ["kind","score","ts","task_id","stage","no_go","id","layer","status","source","target","relation","res","role","platform","state","alive","idle","stalled","output_age_s","relay_age_s","readiness","blocker","attention","evidence_count","resume_ready","evidence_summary","by_kind","transcript_roots_observed","transcript_roots_missing","truncated","count","age_bucket","coverage","task_link_state","severity","privacy","raw_access","exists","capability_id","capability_class","surface_family","spend_model","egress_class","receipt_requirement","route_count","ok_count","blocked_count","hkp_posture","source_refs","source_ref_labels","route_id","mode","profile","model_id","effort","context_mode","fast_mode","quantization","capacity_pool","demand_vector","hardening","eval_plane","review_obligation","learning_eligibility","benchmark_coverage","fixed_overhead","route_state","authority_ceiling","freshness_ok","quota_state","receipt_count","blockers","authority","route_binding_state","tool_id","available","authority_use","observed_at","stale_after","gate_id","domain","evidence","missing","action","detail","generated_at","package_hash","default_lens","domain_id","lifecycle","terrain","depth","scope","claim_ceiling","windows","surfaces","parity","source_refs","lifecycle_id","owner","plant","posture","maturity","adapter_id","claim_surface","mutation_surface","dark_policy","freshness_policy","air_class","commands","receipt_contracts","next_evidence"]
```

Config is resolved from `$REINS_CONFIG`, else `~/.config/reins/config.toml`.
The included starter packs are metadata-only and `support_non_authoritative`: they shape navigation
over Trainyard, Labrack/RDLC, Section-Figure, intake, capability surfaces, and retired Logos design
archaeology without granting dispatch, claim, publication, provider-spend, or source-truth authority.

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
REINS_PORT=8799 REINS_COUNCIL_ROOT=~/projects/hapax-spine \
  python api/reins_read.py

# 2. the cockpit (Go)
go run ./cmd/reins                 # interactive (alt-screen TUI)
go run ./cmd/reins --probe         # headless: fetch, fold, print one frame, exit
go run ./cmd/reins --probe tasks --air   # the :tasks page through the AIR lens
go run ./cmd/reins --probe sessions --air # the :sessions roster through the AIR lens
go run ./cmd/reins --probe "cmd:air on"  # exercise the command path headless
go run ./cmd/reins --probe split size:170x46 capabilities # deterministic split layout frame
go run ./cmd/reins --probe split size:159x34 events       # queued split, no hidden j/k capture
```

The title bar is a channel/window hotlist, not just tabs: `events:n`,
`tasks:n!blocked`, `sess:n!hot`, `yard:n!blocked`, `caps:c!gaps`, `dyn:n`, or `DARK` when a read surface is dark.
When the full registry does not fit, the hotlist stays centered on the active window and uses `+N` markers for hidden windows that remain reachable with `[←/→]`.
Keys: `[:]` command line · `[1]` events · `[2]` tasks · `[3]` sessions · `[Y]` yard · `[R]` readiness · `[I]` intake · `[C]` capabilities · `[4]` dynamics · `[E]` epistemics · `[5]` help · `[6]` commands · `[7]` windows · `[8]` intent · `[9]` surfaces · `[0]` domains · `[L]` lifecycles · `[?]` legend · `[←/→]` cycle windows · `[|]` split context · `[a]` AIR lens · `[q]` quit.
Row pages use `[j/k]` and `[g/G]` for selection. `:tasks` adds `[/]` filter, `[f]` hint/select,
`[Tab]` field rank, `[V]` class-select, and `[↵]` inspect. `:sessions` adds `[↵]`
for a roster-only lane card and `[r]` as a governed `resume-intent` stub only: no transcript,
PTY, stdin bridge, or dispatch path. `:dynamics` adds `[,/.]` scale cycling.
Reference pages use `[j/k]` to scroll and use wide screens for a context/contract rail instead
of a single narrow document. `:capabilities` is cursorful: `[j/k]` selects the capability row and
the context rail follows selected status, class, family, spend, egress, receipt, authority, and missing evidence. No screen is command-only: `:yard`, `:readiness`, `:intake`, `:capabilities`, `:dynamics`, `:epistemics`, `:commands`,
`:windows`, `:intent`, `:surfaces`, `:domains`, and `:lifecycles` are registered pages, and the title hotlist plus
`[←/→]` cycle traverses the full window registry. The surfaces page names transient
doors and modes such as `/whois`, `/session`, yank, filter, hint/select, field-rank, and
note compose so they are not buried as command lore. It also registers selection templates
(`{{focus}}`, `{{sel.status}}`, `{{sel.family}}`, `{{sel.receipt}}`, `{{sel.*}}`, `{{ring.0}}`) for context injection without copy-paste, plus the
split-context surface for session-plus-context composition. The domains page maps SDLC, RDLC,
capability routing, intake, research, tool sessions, and future n-DLCs as lenses over
windows/surfaces rather than hard-coded operator ontology.
The lifecycles page separates source-backed tenant lifecycle contracts from compiled navigation fallback, so SDLC/RDLC/LDLC/n-DLC rows can be inspected without assuming one user's taxonomy is everyone else's lifecycle ontology.
Split context makes the live `:sessions` roster the left-hand source and renders the active
window as a declared relation/context on the right, so `[j/k]` moves the visible lane source while
`[J/K]` scrolls overflowing right-side context and `[←/→]` cycles from capability routing to
yard/windows/help without dropping lane awareness.
It is layout only: no transcript, PTY, stdin bridge, dispatch, claim, or close path.
The capabilities page is capability-first: route classes, concrete tools/providers/connectors,
worker/reviewer surfaces, verifier floors, publication/media/infra surfaces, route contracts,
registry score dimensions, HKP support context, and route/tool evidence all get explicit status rows.
Platforms are evidence, not the capability ontology.

Command probes:

```sh
go run ./cmd/reins --probe commands              # command catalog page
go run ./cmd/reins --probe windows               # lifecycle/window registry page
go run ./cmd/reins --probe intent                # intent review page
go run ./cmd/reins --probe surfaces              # transient surface/mode registry page
go run ./cmd/reins --probe domains               # domain/terrain lens registry page
go run ./cmd/reins --probe lifecycles            # tenant lifecycle contract registry page
go run ./cmd/reins --probe yard                  # Trainyard SDLC cockpit projection
go run ./cmd/reins --probe capabilities          # capability-routing fit/admission projection
go run ./cmd/reins --probe split size:170x46 capabilities # session source + capability context
go run ./cmd/reins --probe "cmd:commands"        # alias path opens the catalog page
go run ./cmd/reins --probe "cmd:windows"         # alias path opens the registry page
go run ./cmd/reins --probe "cmd:surfaces"        # alias path opens the surface page
go run ./cmd/reins --probe "cmd:domains"         # command path opens the domains page
go run ./cmd/reins --probe "cmd:terrain"         # terrain remains a domains alias
go run ./cmd/reins --probe "cmd:n-dlc"           # n-DLC alias opens lifecycle contracts
go run ./cmd/reins --probe "cmd:yard"            # command path opens the yard cockpit
go run ./cmd/reins --probe "cmd:capabilities"    # command path opens capability routing
go run ./cmd/reins --probe "cmd:intent dispatch" # opens review-before-run pane; no effect emitted
```

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

Early, but live end-to-end against a real substrate. Working today: the vital frame and
window hotlist, `:events`, `:tasks`, `:sessions`, `:yard`, `:readiness`, `:intake`, `:capabilities`, `:dynamics`, `:epistemics`, `:help`, `:commands`,
`:windows`, `:intent`, `:surfaces`, `:domains`, `:lifecycles`, and `:legend`,
the cell-grammar, page-aware selection/yank, roster-only session detail cards, the AIR
lens, hot-reload, and the command line with completion/templates plus a command catalog.
Next: richer command intent doors, deeper split-screen/session composition, transcript/compose
for the session pane, and the governed write side of the command surface.
