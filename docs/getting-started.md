# Getting Started with reins

`reins` runs offline on bundled fixture data without external accounts, network
services, or instance configuration.

## 1. Run Offline Demo (`--demo`)

**From GitHub Releases:** download the archive for your operating system and
architecture from [GitHub Releases](https://github.com/hapax-systems/reins/releases),
extract `reins`, and run:

```sh
reins --demo
```

**From source** (requires Go 1.26+):

```sh
git clone https://github.com/hapax-systems/reins && cd reins
go run ./cmd/reins --demo
```

Both paths open the terminal cockpit populated from deterministic fixture data
without making external network calls.

## 2. Navigate the Cockpit

`reins` folds events, tasks, sessions, capability routing, and lifecycle state
into a single terminal screen:

- **Title bar:** displays live channel counts and blocker flags (`events:n`, `tasks:n!blocked`, `caps:c!gaps`).
- **Window navigation:** press `[1]` events, `[2]` tasks, `[3]` sessions, `[C]` capabilities, `[L]` lifecycles, or `[←/→]` to cycle windows.
- **Row inspection:** `[j/k]` moves the cursor, `[↵]` opens row detail, and `[/]` filters visible rows.
- **In-app legend:** `[?]` opens the glyph and key legend for the active page.
- **Redaction toggle:** `[a]` switches the `AIR` default-deny redaction lens (fields outside the allowlist render as `▒▒▒`).
- **Exit:** `[q]` quits the cockpit.

Semantic glyphs remain legible without color: `▸` in-progress, `✓` succeeded,
`✖` failed, `⚑` flagged, and blank cells indicate no active exception. See
[`concepts.md`](concepts.md) for the full glossary and key reference.

## 3. Connect to a Live Read API

`--demo` reads embedded fixtures. To observe a live event substrate, run
`api/reins_read.py` against a configured substrate directory and start `reins`
without `--demo` (see [Live Read API Setup in `README.md`](../README.md#live-read-api-setup)).
Producing live substrate events requires a separate upstream event producer;
`--demo` provides complete standalone inspection of the cockpit and preview
grammar.

## Scope Boundary

`reins` implements **read observation and governed command preview**. Write
commands render a preview envelope describing what an action would request while
mutating no state. Lifecycle definitions (`SDLC`, `RDLC`, and custom domain
lifecycles) operate today as read and navigation lenses rather than general
write-side workflow executors.

## Related Documentation

- [`concepts.md`](concepts.md) — glossary, page catalog, and keyboard reference.
- [`han.md`](han.md) — `Human-Agentic Network` (`HAN`) front command dispatcher (`cmd/han`).
- [`README.md`](../README.md) — architecture, `AIR` redaction model, and headless verification.
- [`LICENSE`](../LICENSE) — Business Source License 1.1 (converts to Apache-2.0 on the Change Date).

---

*HACA-C v0.3.15, the lab's draft rule for labelling machine-made text, not an external certification. No request to a natural person to approve the final wording, and no such approval, was found in hrl-han's sending, review and coordination records when the text was released. hrl-han releases a text once its checking is complete, under a standing rule that hrl-han's principal, who is not named in this text, adopted before the text was drafted. Review record: https://hapaxresearch.com/review-record/ (open to anyone). This label records how the text was made; it does not certify that the text is true. An unaltered record can contain false claims or miss relevant events.*
