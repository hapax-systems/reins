# Getting started with reins

Zero to first value in a couple of minutes. No estate, no account, no config.

## 1. See it (30 seconds)

**From a release (recommended):** download the prebuilt binary for your platform from
[Releases](https://github.com/hapax-systems/reins/releases), then:

```sh
reins --demo
```

**From source** (needs Go 1.26+):

```sh
git clone https://github.com/hapax-systems/reins && cd reins
go run ./cmd/reins --demo
```

Either way you land in a fully-populated cockpit backed by fixture data. Nothing is fetched, nothing is
live — it's the fastest way to see what reins is.

## 2. Read the screen (2 minutes)

You're looking at one AI-agent software-delivery estate as a single screen. A quick tour:

- The **title bar** is a live channel list — `events:n`, `tasks:n!blocked`, `caps:c!gaps` — not just tabs.
- Press number/letter keys to jump: **`[1]`** events, **`[2]`** tasks, **`[3]`** sessions,
  **`[C]`** capabilities, **`[L]`** lifecycles. `[←/→]` cycles windows.
- **`[j/k]`** selects a row, **`[↵]`** inspects it, **`[/]`** filters.
- **`[?]`** shows the legend for whatever page you're on — press it any time you're unsure of a glyph
  or key.
- **`[a]`** toggles the **AIR** lens (on-air redaction: fields not on the allowlist blank to `▒▒▒`).
- **`[q]`** quits.

Every glyph carries meaning: `▸` in-progress, `✓` ok, `✖` fail, `⚑` flag. Emptiness reads as all-clear.
For the full glossary (what "AIR", "capability routing", "lifecycle lens" mean), see
[`concepts.md`](concepts.md).

## 3. Point it at a real estate (later)

`--demo` is fixtures. To fold your *own* live estate, run the READ API against your substrate and drop
the `--demo` flag — see [the README's "Run it live"](../README.md#run-it-live). (Heads-up: a full live
experience today also needs a substrate *producer*, which ships separately; `--demo` remains the
estate-free path.)

## What reins does — and doesn't, yet

It **reads and previews**: it observes your live lifecycle and shows what a command *would* do (minting
nothing). It does **not yet mutate** the estate, and while it maps SDLC / RDLC / "your own n-DLC" as
navigation lenses, governed automation of an arbitrary lifecycle is on the roadmap, not shipped. We say
so plainly so you know exactly what you're adopting.

## Where next

- [`concepts.md`](concepts.md) — the glossary + full key reference.
- [README](../README.md) — architecture, the AIR redaction model, self-verification.
- `LICENSE` — Business Source License 1.1 (free for all non-competing use; → Apache-2.0 on the change date).
