# reins — concepts & keys

A glossary for the terms you'll meet in the cockpit, in plain language, and the full key reference.
In-app, press `[?]` on any page for its legend.

## The core idea

reins shows you a live AI-agent software-delivery estate as one screen. It **reads** the estate's
append-only event ledger, folds it into scored rows, and lets you navigate the estate's registries. It
does not (yet) change anything — it observes and *previews*.

## Glossary

| Term | Plain meaning |
|---|---|
| **substrate** | The system reins reads from — the event ledger + registries of your estate. reins is the frontend; the substrate is the backend it folds. |
| **cell / cell-grammar** | The design unit is one character position, not a widget. A glyph = *kind* (`▸` in-progress, `✓` ok, `✖` fail…), a bar = *magnitude*. Monochrome-safe so it survives a livestream / 16-color / screen-reader. |
| **AIR** | "On-air" redaction. A default-deny render lens: on-air, any field not on the allowlist shows as `▒▒▒`. You can't leak by forgetting. Toggle `[a]`. |
| **honest-dark** | When a read surface has no data (the producer isn't running), reins shows `DARK` rather than faking content. Honesty over false green. |
| **capability routing** | Which *capability* (a model/agent/tool, identified by measured behavior) should serve a given demand. The `:capabilities` page shows the routes, their fitness, and their evidence. |
| **lifecycle / n-DLC** | A domain's delivery lifecycle. SDLC (software), RDLC (research), MDLC (management) are instances of a general "n-DLC". reins maps them as **navigation lenses** — today it lets you *declare and navigate* a lifecycle; governed automation of an arbitrary lifecycle is on the roadmap. |
| **yard** | The delivery-coordination cockpit projection (tasks moving through the lifecycle). |
| **readiness** | Whether the estate/agents are ready to take work (live/idle/stalled, blockers). |
| **command preview / intent** | Write commands don't mutate — they render a governed *preview* envelope showing what *would* happen, minting nothing. The governed write side is roadmap. |
| **lens** | reins holds no authoritative state; every view is a pure fold of fetched data. A "lens" is a view over the substrate, never a source of truth. |

## Pages

`:events` live event stream · `:tasks` work items · `:sessions` agent-session roster · `:yard`
delivery-coordination · `:readiness` estate readiness · `:capabilities` capability routing ·
`:lifecycles` the lifecycle contracts (SDLC/RDLC/… as lenses) · `:domains` domain lenses · `:intake`
inbound requests · `:dynamics` / `:epistemics` derived views · `:commands` / `:windows` / `:surfaces`
self-describing registries · `:help` / `:legend`.

## Keys

- **Navigate windows:** `[←/→]` cycle windows · number/letter hotkeys jump (`[1]`events `[2]`tasks
  `[3]`sessions `[C]`capabilities `[L]`lifecycles …) · `[|]` split context.
- **Within a page:** `[j/k]` select · `[g/G]` top/bottom · `[/]` filter · `[↵]` inspect · `[Tab]` rank
  fields · `[V]` class-select.
- **Lenses & modes:** `[a]` AIR redaction · `[:]` command line · `[?]` legend.
- **Quit:** `[q]`.

Every page is reachable both by hotkey and by `:command`. No screen is command-only, and the title-bar
hotlist (`events:n`, `tasks:n!blocked`, …) is a live channel list, not just tabs.
