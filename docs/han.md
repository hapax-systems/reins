# han — the System I front dispatcher

`han` is a thin front command (no engine of its own) that dispatches the System I
role words to today's programs (row
`system-i-han-front-command-dispatcher-slice-1-20261031`):

| role | program today |
|---|---|
| `han scope` | `reins --scope` (two read-only task/event frames) |
| `han docket` | `reins` (work and governance) |
| `han forum` | `hapx` (conversation) |
| `han locus` | `stead` (the spatial lab) |

`han --version` prints the estate dispatcher identity (`hapax-systems/reins`) so a
collision probe can tell it from a foreign `han`. `han` with no subcommand, or an
unknown one, prints usage and exits non-zero.

Scope starts with tasks in frame 1 and events in frame 2. `Tab` moves focus;
`t` or `e` chooses the focused frame's lens; `j/k` (or arrows) moves its row
cursor; `g/G` moves to the ends. Each frame retains its own task and event
positions. `a` toggles the existing AIR redaction and `q` quits. Scope has no
command or conversation mode. `han docket` keeps the existing cockpit launch.

On a compact device or a viewport narrower than 145 columns, the focused frame
fills the body; the other frame remains reachable with `Tab`. On wider devices
both frames appear through the existing view algebra. Their connector derives
from selected rows' AIR-permitted facts: shared facets, or exact task ID/event
subject equality. It never guesses from partial names. No supported fact means
`unlinked · no join`. Missing, failed or locally stale reads are dark; the local
receipt expires after 15 seconds without a successful poll. This measures read
delivery, not the age of the provider's underlying source. Source freshness cells
retain their existing meaning.

For an offline fixture (labelled DEMO), use `han scope --demo`, or capture a
headless navigation witness through the established path:

```sh
bash scripts/reins-shot.sh 'j; tab; e; j' /tmp/scope.png size:170x46 --scope
bash scripts/reins-shot.sh 'tab; j' /tmp/scope-mobile.png size:60x24 --scope --air
```

These exercise local fixture navigation and rendering; they do not demonstrate
an installed client or actual operator use. Frame choices are not persisted.

Not in this slice: the legacy aliases, any rename, README or product-name changes,
the collision probe, any spoken or voice use.
