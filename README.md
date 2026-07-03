# reins

**A single-screen cockpit for governing an AI-agent software-delivery estate** — the frontdoor to the
Hapax n-DLC automation system. It folds your whole live delivery lifecycle — events, tasks, agent
sessions, capability routing, readiness — into one dense terminal screen built to survive a livestream,
a 16-color terminal, and a screen-reader alike.

**Try it right now — no setup, no estate, no account:**

```sh
reins --demo          # prebuilt binary (see Releases), or from source:
go run ./cmd/reins --demo
```

`--demo` drops you into a fully-populated cockpit backed by fixture data. It's the fastest way to see
what this is — everything below is what you're looking at.

## What it is — and what it isn't yet (stated honestly)

reins is the **read + command-preview frontdoor** to a governed agent-automation system. Today it lets
you **observe** the whole live lifecycle (folded from an append-only ledger into one scored screen) and
**navigate** the estate's registries — what capabilities exist, what's routed where, what's stalled — as
*lenses*, not a hard-coded ontology.

What it does **not** do yet, said plainly:
- It **reads and previews; it does not yet mutate.** Write commands render a governed preview envelope
  that **mints nothing** — the governed write side is on the roadmap.
- It maps SDLC / RDLC / capability-routing / *your own n-DLC* as lenses, but that is **declare-and-navigate
  today** — the "governed automation for any lifecycle" generality is being built, not shipped. We don't
  claim what isn't there.

## Why it looks like this — the cell

The **cell** (one character position) is the unit of design, not the widget. A glyph carries *kind* by
semantic class (`▸` in-progress · `✓` success · `✖` failure · `⇡` advance · `⚑` flag · `◆` task), an
eighth-block bar carries *magnitude*, and emptiness reads as all-clear. It's **monochrome-safe first**
(color is redundant reinforcement, never the only signal), so the same screen survives a livestream
re-encode, a 16-color terminal, and a screen-reader pass.

## AIR — default-deny redaction

reins is built to be visible on a livestream, so redaction is **default-deny** and lives at the renderer,
not a post-hoc filter. One model, two render targets: **LOCAL** (full fidelity, your eyes) and **AIR**
(every field not on the allowlist renders as `▒▒▒` — a new field is denied until explicitly allowed, so
you cannot leak by forgetting). Toggle with `[a]` or `:air on|off`.

## Run it live

`--demo` needs nothing. To point reins at a **real** estate, it reads from a substrate you provide — so
the live path needs that substrate importable + a small instance config:

```sh
# 1. the READ API (Python; folds your live ledger — needs fastapi + uvicorn + your substrate)
python api/reins_read.py            # reads $REINS_CONFIG, else ~/.config/reins/config.toml

# 2. the cockpit (Go)
go run ./cmd/reins
```

Instance config (`config.example.toml` → `~/.config/reins/config.toml`) carries every deployment fact —
the substrate root, the API URL, the on-air allowlist, the palette. The cockpit binary itself bakes no
paths; the config is the instance.

*Honest caveats:* the config key for the substrate root is currently `council_root` (being generalized to
`substrate_root`), and a full live experience today also needs a substrate *producer* that ships
separately — so **`--demo` is the estate-free path** and the surest first-value.

## Architecture

```
  SUBSTRATE            UNIFIED READ API           COCKPIT
  (event spine,     ─▶ (thin, scored,        ─▶  (Go / Bubble Tea,
   append-only)        air-classified HTTP)       stateless fold)
                       api/reins_read.py          cmd/reins + internal/*
```

- **The language boundary is the API boundary.** The Go cockpit never imports the substrate; it speaks
  HTTP to a thin Python service. Swap either side without touching the other.
- **Stateless fold (Elm model-update-view).** The view is a pure function of fetched data — re-fetch +
  re-fold restores the exact view, so a code change hot-reloads and a crash re-exec is lossless. The
  cockpit is a *lens*, never a source of truth.

## Self-verifying (headless — no terminal, no human)

The cockpit can be driven, rendered to a PNG, and confirmed by an agent — so a visual change verifies
itself:

```sh
make smoke                                     # visit every page headless; a panic is the finding
go run ./cmd/reins --drive ":tasks; j; enter"  # drive a key sequence, print the end frame
```

## Concepts & keys

The cockpit has many pages and a rich key grammar. Rather than a wall here, see **`docs/concepts.md`**
for the glossary (what "AIR", "capability routing", "lifecycle lens", "yard" mean) and the full key
reference. In-app, press `[?]` for the legend on any page.

## Test

```sh
go test ./...                      # Go: config, grammar, model, cmd
( cd api && python -m pytest -q )  # Python: the READ service
```

## Status

Early, but live end-to-end against a real substrate. Working today: the vital frame + window hotlist,
the read pages (`:events` `:tasks` `:sessions` `:yard` `:readiness` `:capabilities` `:lifecycles` and
more), the cell-grammar, the AIR lens, hot-reload, the command line with completion, and the governed
command **preview** (mints nothing). Next: the governed *write* side, and the lifecycle-adapter +
generality work that turns "declare a lifecycle" from a navigation lens into governed automation.

## License

Source-available under the **Business Source License 1.1** (see `LICENSE`): free for all non-competing
use — self-host it, build on it, run your own instance — converting to Apache-2.0 on the change date.
Only offering it as a competing hosted service is reserved.
