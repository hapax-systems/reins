package grammar

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hapax-systems/reins/internal/palette"
)

// pal is the cockpit's active palette (mode-keyed). SetPalette swaps it on a working-mode flip;
// color is a redundant amplifier over the glyph grammar, so callers never depend on it for meaning.
var pal = palette.For("gruvbox")

// SetPalette switches the color grammar for the working mode ("gruvbox"/"solarized").
func SetPalette(mode string) { pal = palette.For(mode) }

// C colorizes text with a palette token (the cockpit's one coloring entry point for zones/widgets).
func C(token, text string) string { return pal.Colorize(token, text) }

// SeverityToken / LaneToken re-exported so callers color by meaning without importing palette.
func SeverityToken(sev string) string { return palette.SeverityToken(sev) }
func LaneToken(owner string) string   { return palette.LaneToken(owner) }

// kindSeverity maps an event kind to a severity word for its heat color ("" = neutral/ground).
func kindSeverity(kind string) string {
	switch {
	case strings.Contains(kind, "fail"):
		return "failed"
	case strings.Contains(kind, "succeed"), strings.Contains(kind, "merged"):
		return "done"
	case strings.Contains(kind, "flip"):
		return "urgent"
	case strings.Contains(kind, "started"), strings.Contains(kind, "transition"), strings.Contains(kind, "claim"):
		return "review"
	}
	return ""
}

// Event is the unified-API READ contract for one stream row (mirrors reins_read.to_event).
type Event struct {
	TS, Kind, Subject, Actor, Summary string
	Score                             float64
	AIR                               map[string]string // field -> "ok"|"deny"
}

// Glyph: the closed, learned alphabet — the cell carries the kind by semantic class
// (▸ in-progress · ✓ success · ✖ failure · ⇡ advance · ⚑ flag · ◆ task · ↟ PR), monochrome-safe.
var glyphs = map[string]string{
	"pr.merged": "↟", "task.closed": "◆", "task.claim": "◆", "session.ended": "⚙",
	"review.fail": "✖", "stage": "▸", "status": "·",
	"coord_dispatch.launch_started":   "▸",
	"coord_dispatch.launch_failed":    "✖",
	"coord_dispatch.launch_succeeded": "✓",
	"sdlc.stage_transition":           "⇡",
	"sdlc.authorization_flip":         "⚑",
}

func Glyph(kind string) string {
	if g, ok := glyphs[kind]; ok {
		return g
	}
	return "✶" // generic event
}

// ScoreBar: eighth-block magnitude (the bar IS the magnitude; no severity glyph here).
var eighths = []rune(" ▏▎▍▌▋▊▉█")

func ScoreBar(score float64) string {
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	idx := int(score * 8)
	if idx > 8 {
		idx = 8
	}
	return string(eighths[idx])
}

// redact: generic AIR helper. Allowlisted field (air[field]=="ok") passes; denied field
// becomes a fixed-width redaction token (default-deny). Used by every row-kind.
func redact(airMap map[string]string, field, val string, on bool) string {
	if on && airMap[field] != "ok" {
		return "▒▒▒"
	}
	return val
}

// compactTS: presentation-only — the API returns canonical ISO (full precision, the data
// contract); the cockpit compacts to HH:MM:SS for grid density. Unparseable -> first 8 chars.
func compactTS(ts string) string {
	if i := strings.IndexByte(ts, 'T'); i >= 0 {
		t := strings.TrimSuffix(ts[i+1:], "Z")
		if j := strings.IndexByte(t, '.'); j >= 0 { // drop sub-seconds
			t = t[:j]
		}
		if k := strings.IndexByte(t, '+'); k >= 0 { // drop tz offset
			t = t[:k]
		}
		return pad(t, 8)
	}
	return pad(ts, 8)
}

// RenderEventRow: one row of the grammar. `airOn` toggles the AIR lens (default-deny redaction).
// Format (formats.toml row-kind "event"): TS │ scorebar glyph │ subject(6) │ summary
func RenderEventRow(ev Event, airOn bool) string {
	subj := redact(ev.AIR, "subject", pad(ev.Subject, 6), airOn)
	summ := redact(ev.AIR, "summary", ev.Summary, airOn)
	bar, glyph := ScoreBar(ev.Score), Glyph(ev.Kind) // single runes — colorized AFTER width math
	if sev := kindSeverity(ev.Kind); sev != "" {
		tok := palette.SeverityToken(sev)
		bar, glyph = pal.Colorize(tok, bar), pal.Colorize(tok, glyph)
	}
	return fmt.Sprintf("%s %s%s %s  %s", compactTS(ev.TS), bar, glyph, subj, summ)
}

// Task is the unified-API READ contract for one registry row (mirrors reins_read.to_task).
type Task struct {
	TaskID        string            `json:"task_id"`
	Stage         string            `json:"stage"`
	AuthorityCase string            `json:"authority_case"`
	NoGo          string            `json:"no_go"`
	AIR           map[string]string `json:"air"`
}

// RenderTaskHeader: the frozen header for the :tasks registry page.
func RenderTaskHeader() string {
	return fmt.Sprintf("  %-28s %-5s %s", "TASK", "STAGE", "NO-GO")
}

// dotsOr: structured-silence — an empty cell is dots at full width (the grid never jitters).
func dotsOr(s string, n int) string {
	if strings.TrimSpace(s) == "" {
		return strings.Repeat("·", n)
	}
	return pad(s, n)
}

// RenderTaskRow: one registry row (row-kind "task"). Leading glyph carries the kind (◆=task);
// task_id is the frozen id-gutter / cross-pane address; empties render as structured-silence dots.
func RenderTaskRow(t Task, airOn bool) string {
	id := redact(t.AIR, "task_id", pad(t.TaskID, 28), airOn)
	stage := redact(t.AIR, "stage", dotsOr(t.Stage, 5), airOn)
	nogo := redact(t.AIR, "no_go", dotsOr(t.NoGo, 4), airOn)
	return fmt.Sprintf("%s %s %s %s", Glyph("task.closed"), id, stage, nogo)
}

// --- :dynamics — the system-dynamics map (obsoletes the standalone :8765 cytoscape viewer) ---

// Layer / Node / Edge mirror reins_read's to_node/to_edge + the seed's layer list.
type Layer struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type Node struct {
	ID     string            `json:"id"`
	Label  string            `json:"label"`
	Kind   string            `json:"kind"`
	Layer  string            `json:"layer"`
	Status string            `json:"status"`
	Res    string            `json:"res"`
	AIR    map[string]string `json:"air"`
}
type Edge struct {
	Source   string            `json:"source"`
	Target   string            `json:"target"`
	Relation string            `json:"relation"`
	Status   string            `json:"status"`
	AIR      map[string]string `json:"air"`
}
type Graph struct {
	MapID  string  `json:"map_id"`
	Thesis string  `json:"thesis"`
	Layers []Layer `json:"layers"`
	Nodes  []Node  `json:"nodes"`
	Edges  []Edge  `json:"edges"`
}

// AtResolution returns the sub-graph at view-scale maxRes (the seed's view_scales: 1=overview …
// 5=evidence); maxRes<=0 means "all". Nodes with res>maxRes drop; edges to dropped nodes drop with
// them. Pure transform — the cell-grammar's "resolution" / zoom principle, using the map's own model.
func (g Graph) AtResolution(maxRes int) Graph {
	if maxRes <= 0 {
		return g
	}
	keep := make(map[string]bool, len(g.Nodes))
	var nodes []Node
	for _, n := range g.Nodes {
		r, _ := strconv.Atoi(n.Res)
		if r == 0 || r <= maxRes { // unknown res (0) is always kept
			nodes = append(nodes, n)
			keep[n.ID] = true
		}
	}
	var edges []Edge
	for _, e := range g.Edges {
		if keep[e.Source] && keep[e.Target] {
			edges = append(edges, e)
		}
	}
	return Graph{MapID: g.MapID, Thesis: g.Thesis, Layers: g.Layers, Nodes: nodes, Edges: edges}
}

// statusGlyph: provenance as a confidence ladder — filled = solid, open = tentative (the seed's
// status_kinds). The glyph IS the status field, so it is redacted when status is denied on air.
var statusGlyphs = map[string]string{
	"asserted": "●", "observed": "◉", "inferred": "◐",
	"simulated": "◍", "rendered": "◌", "candidate": "○",
}

func statusGlyph(status string, air map[string]string, airOn bool) string {
	if airOn && air["status"] != "ok" {
		return "▒"
	}
	if g, ok := statusGlyphs[status]; ok {
		return g
	}
	return "·"
}

// RenderDynamics: the system-dynamics map as layered ASCII adjacency. Bands = layers (in seed
// order); each node shows its provenance glyph + id + label, with outgoing edges as an indented
// adjacency tree (├→ / └→ target (relation)). Deterministic — seed order preserved, no sort.
// The research-recommended stage-aware column-rail 2D layout is the planned aesthetic iteration;
// this is the honest, complete v1 (every node + every edge, obsoleting the :8765 viewer).
func RenderDynamics(g Graph, airOn bool) string {
	if len(g.Nodes) == 0 {
		return "  (no map)"
	}
	out := map[string][]Edge{} // outgoing edges indexed by source id
	for _, e := range g.Edges {
		out[e.Source] = append(out[e.Source], e)
	}
	var b strings.Builder
	for _, L := range g.Layers {
		dashes := 54 - len(L.Label)
		if dashes < 1 {
			dashes = 1
		}
		b.WriteString("── " + strings.ToUpper(L.Label) + " " + strings.Repeat("─", dashes) + "\n")
		for _, n := range g.Nodes {
			if n.Layer != L.ID {
				continue
			}
			id := redact(n.AIR, "id", pad(n.ID, 22), airOn)
			label := redact(n.AIR, "label", n.Label, airOn)
			b.WriteString(fmt.Sprintf("%s %s  %s\n", statusGlyph(n.Status, n.AIR, airOn), id, label))
			es := out[n.ID]
			for i, e := range es {
				conn := "├→"
				if i == len(es)-1 {
					conn = "└→"
				}
				tgt := redact(e.AIR, "target", pad(e.Target, 20), airOn)
				rel := redact(e.AIR, "relation", e.Relation, airOn)
				b.WriteString(fmt.Sprintf("   %s %s (%s)\n", conn, tgt, rel))
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// RenderHelp: the static discoverability page — every page, verb, and key on one screen.
// Discoverability is a cockpit principle; the cockpit documents itself.
func RenderHelp() string {
	return strings.Join([]string{
		"REINS — one cockpit for the whole delivery lifecycle.",
		"",
		"PAGES",
		"  :events            live coord event stream (scored, glyph grammar)",
		"  :tasks             the task registry (live projection)",
		"  :dynamics [scale]  the system-dynamics map as layered ASCII",
		"                     scale = overview|domain|artifact|runtime|evidence|1..5|all",
		"  :help              this page",
		"",
		"COMMAND   ([:] opens the command line — Enter runs, Esc cancels)",
		"  :air on|off        the PII-safe on-air lens (default-deny redaction)",
		"  :quit              leave",
		"",
		"KEYS",
		"  [:] command   [1] events  [2] tasks  [3] dynamics  [4] help",
		"  [a] AIR lens  [q] quit",
		"",
		"On AIR, every non-allowlisted cell renders ▒▒▒ — safe for the livestream.",
	}, "\n")
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}
