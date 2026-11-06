# reins — Concepts and Key Reference

Glossary of architectural terms and keyboard bindings used across the `reins`
terminal cockpit. Press `[?]` on any page in the application to display the
page-specific legend.

## Core Model

`reins` renders an AI-agent delivery substrate as a stateless terminal fold. It
reads an append-only event ledger and associated registries over HTTP, scores
and projects rows into fixed-cell views, and renders preview envelopes for
candidate commands without mutating underlying state.

## Glossary

| Term | Definition |
|---|---|
| **substrate** | Append-only event ledger and configuration registries read by `api/reins_read.py`. `reins` is a read/preview client over the substrate rather than a primary database. |
| **cell / cell-grammar** | Single character-cell layout unit. Semantic glyphs encode state class (`▸` in-progress, `✓` succeeded, `✖` failed, `⚑` flagged) and eighth-block bars encode magnitude, preserving legibility in monochrome terminals, screen readers, and re-encoded video streams. |
| **AIR** | Default-deny redaction render mode. When `AIR` is active, every field not explicitly listed on the allowlist renders as `▒▒▒`. Toggle with `[a]` or `:air on\|off`. |
| **honest-dark** | Explicit `DARK` indicator shown whenever an upstream read surface is absent, unreachable, or stale, avoiding synthetic green status placeholders. |
| **capability routing** | Mapping from task demands to evaluated model, agent, or tool routes, surfaced with fitness and evidence metadata on `:capabilities`. |
| **lifecycle lens** | Structured stage model for software (`SDLC`), research (`RDLC`), or custom domain delivery lifecycles. `reins` currently renders declared lifecycles as navigation and inspection lenses rather than automated write-side executors. |
| **yard** | Delivery-coordination projection tracking work items across lifecycle stages. |
| **readiness** | Operational availability summary across sessions, queues, and active blockers. |
| **command preview** | Non-mutating envelope rendered when a write-shaped command is entered, recording the requested intent without issuing state changes. |
| **lens** | Pure functional projection over fetched read-API payloads; re-fetching and re-folding reproduces the view deterministically. |

## Pages

`:events` live event stream · `:tasks` work items · `:sessions` agent-session roster · `:yard`
delivery coordination · `:readiness` operational readiness · `:capabilities` capability routing ·
`:lifecycles` declared lifecycle lenses · `:domains` domain lenses · `:intake`
inbound requests · `:dynamics` and `:epistemics` derived summaries · `:commands`, `:windows`, and `:surfaces`
self-describing registries · `:help` and `:legend`.

## Keyboard Reference

- **Switch windows:** `[←/→]` cycle windows · `[1]` events, `[2]` tasks, `[3]` sessions, `[C]` capabilities, `[L]` lifecycles · `[|]` split context.
- **Within a page:** `[j/k]` move cursor · `[g/G]` jump top/bottom · `[/]` filter · `[↵]` inspect row · `[Tab]` rank fields · `[V]` select class.
- **Lenses and command bar:** `[a]` toggle `AIR` redaction · `[:]` open command prompt · `[?]` open legend.
- **Exit:** `[q]`.

Every page is reachable both by hotkey and by `:command`.

---

*HACA-C v0.3.15, the lab's draft rule for labelling machine-made text, not an external certification. No request to a natural person to approve the final wording, and no such approval, was found in hrl-han's sending, review and coordination records when the text was released. hrl-han releases a text once its checking is complete, under a standing rule that hrl-han's principal, who is not named in this text, adopted before the text was drafted. Review record: https://hapaxresearch.com/review-record/ (open to anyone). This label records how the text was made; it does not certify that the text is true. An unaltered record can contain false claims or miss relevant events.*
