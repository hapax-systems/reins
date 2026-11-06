# han — Human-Agentic Network Front Command Dispatcher

`han` is a thin front-command dispatcher for the **Human-Agentic Network (`HAN`)**
published by **Hapax Research Lab**. It contains no state store or execution
engine of its own; each subcommand resolves and launches a bound companion
program on `PATH`:

| Subcommand | Dispatched program | Role |
|---|---|---|
| `han scope` | `reins --scope` | Two-frame read-only task and event monitor (`./cmd/reins` in this repository). |
| `han docket` | `reins` | Full terminal cockpit for work, readiness, and governed command preview (`./cmd/reins` in this repository). |
| `han forum` | `hapx` | Companion terminal conversation client (resolved on `PATH` when installed separately). |
| `han locus` | `stead` | Companion spatial-workspace client (resolved on `PATH` when installed separately). |

`han --version` prints the dispatcher identity (`hapax-systems/reins`) so
callers can distinguish this binary from an unrelated `han` executable on
`PATH`. Invoking `han` with no subcommand or an unrecognized subcommand prints
usage to standard error and exits with status `2`; if the target executable is
absent from `PATH`, `han` exits with status `127`.

## Scope Frame Behavior

`han scope` opens with tasks in frame 1 and events in frame 2:

- `Tab` switches focus between the two frames.
- `t` or `e` selects the task or event lens within the focused frame.
- `j`/`k` (or arrow keys) moves the row cursor; `g`/`G` jumps to the top or bottom row.
- `a` toggles `AIR` default-deny field redaction; `q` quits.
- `han scope` exposes read-only observation only (no command-line prompt or write preview).

On viewports narrower than 145 columns, the focused frame occupies the terminal
body while the second frame remains reachable via `Tab`. On wider viewports,
both frames render side by side. Frame linkage derives strictly from
`AIR`-permitted fields on the selected rows (shared facets or exact task
identifier / event subject equality); partial name matches are never inferred,
and unmatched selections render `unlinked · no join`.

Missing, failed, or locally stale reads render as `DARK`; the local poll receipt
expires after 15 seconds without a successful refresh. That timer measures local
read delivery rather than upstream source age.

## Offline Fixture and Headless Verification

Run `han scope --demo` (once `reins` is on `PATH`) for offline fixture data, or
capture a headless frame witness directly from source:

```sh
bash scripts/reins-shot.sh 'j; tab; e; j' /tmp/scope.png size:170x46 --scope
bash scripts/reins-shot.sh 'tab; j' /tmp/scope-mobile.png size:60x24 --scope --air
```

These commands exercise local fixture navigation and terminal rendering; frame
selections are ephemeral and are not persisted across runs.

---

*HACA-C v0.3.15, the lab's draft rule for labelling machine-made text, not an external certification. No request to a natural person to approve the final wording, and no such approval, was found in hrl-han's sending, review and coordination records when the text was released. hrl-han releases a text once its checking is complete, under a standing rule that hrl-han's principal, who is not named in this text, adopted before the text was drafted. Review record: https://hapaxresearch.com/review-record/ (open to anyone). This label records how the text was made; it does not certify that the text is true. An unaltered record can contain false claims or miss relevant events.*
