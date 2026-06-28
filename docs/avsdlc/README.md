# Reins self-verification — smoke navigation, visual inspection, AVSDLC witness

This is how a Reins visual change is verified **headlessly, with no human at a terminal** — so the
cockpit can be navigated, looked at, and AVSDLC-confirmed by an agent. Three layers:

## 1. Automated navigation smoke (`internal/smoke`)

`internal/smoke` drives key sequences through the live `Model.Update` loop and captures the rendered
frame after each step (panics recovered + recorded — a crash is the finding, not an abort). The tests
visit **every page** (no panic, non-empty frame), exercise the in-page gestures (door / verb-menu /
yank / filter / command), and assert on-air navigation redacts. Run:

```
go test ./internal/smoke/ -v
```

## 2. Visual inspection — render a frame to PNG (`scripts/reins-shot.sh`)

The cockpit is a terminal surface; its rendered frame **is** the visual. `reins --drive "<steps>"`
folds the seed (or `--live` data), feeds the steps, and prints the end-state frame; `freeze
--language ansi` renders that ANSI to a PNG an agent can open + look at.

```
scripts/reins-shot.sh ":capabilities; j"     /tmp/a4-caps.png  size:170x46 --live
scripts/reins-shot.sh ":tasks; j; v; a"      /tmp/arm.png      size:160x44 --air
```

`<steps>` is `;`-separated; each step is a space-separated key list (`j k enter esc v a space`), and
`:word` types a command. Requires `freeze` (`go install github.com/charmbracelet/freeze@latest`).

## 3. AVSDLC predict-then-confirm (`scripts/reins-avsdlc-witness.py`)

Follows the council AVSDLC: **pre-author** a falsifiable `VisualIntentRecord` (predicates over
per-region `luma` / `edge_energy`), **capture** the frame (layer 2), then **confirm** with the
canonical council eval (`shared/avsdlc_realized_vector.py` + `avsdlc_visual_intent.py`). It emits a
witness receipt (content-hash · intent-hash · perceptual-digest · verdict) + per-predicate detail.

```
scripts/reins-shot.sh ":coordinator" /tmp/coord.png size:150x42 --live
python3 scripts/reins-avsdlc-witness.py \
  --frame /tmp/coord.png --intent docs/avsdlc/intents/cockpit-legibility.json \
  --pov local-terminal --source-head "$(git rev-parse --short HEAD)" --out docs/releases/<change>-witness
```

Exit 0 = intent PASS, 1 = FAIL, 2 = malformed intent. It **discriminates** — a blank frame FAILS the
legibility intent. The 6 regions (`ceiling/left_wall/right_wall/floor/entity_core/negative_space`)
are the council ROIs; for a TUI they partition the frame, `entity_core` = the center pane, `floor` =
the bottom legend bar, etc. `luma` = brightness (text-on-dark stays low), `edge_energy` = glyph/border
structure (sparse panes measure low — e.g. the empty CHAT pane right_wall ≈ 0.3 vs the dense center ≈ 3.3).

**Witness mode:** this is a *local-terminal self-witness* — the verdict is the deterministic council
metric eval (not a minted assertion). The signed-receipt / OBS-moving fields are the operator's
release gate, not this self-witness; the PR release gate verifies the dossier at merge.

### Per-pane intents

Each visual pane carries a pre-authored intent under `docs/avsdlc/intents/`. When you build/change a
pane: author the intent FIRST (the prediction), build, capture, confirm, and store the witness
receipt in the release dossier. A pane whose five-tuple contract is incomplete is **projection-pending**
and must be badged — the AVSDLC verdict does not waive that.
