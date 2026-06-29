package grammar

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/palette"
)

// pal is the cockpit's active palette (mode-keyed). SetPalette swaps it on a working-mode flip;
// color is a redundant amplifier over the glyph grammar, so callers never depend on it for meaning.
var pal = palette.For("gruvbox")

// SetPalette switches the color grammar for the working mode ("gruvbox"/"solarized").
func SetPalette(mode string) { pal = palette.For(mode) }

// C colorizes text with a palette token (the cockpit's one coloring entry point for zones/widgets).
func C(token, text string) string { return pal.Colorize(token, text) }

// Hex resolves a palette token to its raw hex (for callers that need a background, not just fg).
func Hex(token string) string { return pal.Hex(token) }

// SelLabel renders a SELECTION swatch — the reserved selection channel. It rides SHAPE/CONTRAST
// (a bright glyph on a neutral grey block), never a hue, so it cannot collide with the data's
// criticality-hue / freshness-brightness / ownership-family, and stays legible in grayscale + on-air.
func SelLabel(text string) string {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(pal.Hex("border"))).
		Foreground(lipgloss.Color(pal.Hex("brt"))).
		Bold(true).Render(text)
}

// FlashLabel: a transient effect-confirmation chip (Norman feedback). Distinct CHANNEL from SelLabel
// — a green block (success hue) rather than the neutral selection swatch — so a flash reads as "an
// action just landed", never as a persistent selection. Lives ~900ms then clears.
func FlashLabel(text string) string {
	return lipgloss.NewStyle().
		Background(lipgloss.Color(pal.Hex("grn"))).
		Foreground(lipgloss.Color(pal.Hex("bg"))).
		Bold(true).Render(text)
}

// SeverityToken / LaneToken re-exported so callers color by meaning without importing palette.
func SeverityToken(sev string) string { return palette.SeverityToken(sev) }
func DimSeverityToken(sev string, freshness float64) string {
	return palette.DimSeverityToken(sev, freshness)
}
func LaneToken(owner string) string { return palette.LaneToken(owner) }

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

// Redact is the exported default-deny lens for callers OUTSIDE the row renderers (the context rail,
// the door) that must honor the SAME on-air policy — one source, no drift. A denied field blanks to
// the redaction token; structure is kept, the value never airs.
func Redact(airMap map[string]string, field, val string, on bool) string {
	return redact(airMap, field, val, on)
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

// RenderEventHeader: the column header that SITUATES the event stream (what each column is).
func RenderEventHeader() string {
	return C("mut", fmt.Sprintf(" %-8s %-2s %-26s %-10s %s", "TIME", "k", "SUBJECT", "WHO", "WHAT")) // k=kind glyph
}

// shortKind strips the noisy event-type prefix for a readable "what" (launch_failed, stage_transition).
func shortKind(k string) string {
	for _, p := range []string{"coord_dispatch.", "sdlc.", "coord."} {
		k = strings.TrimPrefix(k, p)
	}
	return k
}

// RenderEventRow: one self-explaining stream row — TIME · score-bar · kind-glyph · subject · WHO ·
// WHAT. Subject is wide enough to read; actor is lane-colored; the "what" falls back to the kind
// when there is no summary (so a row is never just a truncated stub). `airOn` redacts per field.
func RenderEventRow(ev Event, airOn bool) string {
	ts := compactTS(ev.TS)
	if airOn && ev.AIR["ts"] != "ok" {
		ts = pad("▒▒▒▒▒▒▒▒", 8)
	}
	bar := ScoreBar(ev.Score)
	if airOn && ev.AIR["score"] != "ok" {
		bar = C("mut", "▒▒▒▒")
	}
	glyph := Glyph(ev.Kind) // single rune — colorized AFTER width math
	if airOn && ev.AIR["kind"] != "ok" {
		glyph = C("mut", "▒")
	} else if sev := kindSeverity(ev.Kind); sev != "" {
		tok := palette.SeverityToken(sev)
		if !(airOn && ev.AIR["score"] != "ok") {
			bar = pal.Colorize(tok, bar)
		}
		glyph = pal.Colorize(tok, glyph)
	}
	subj := C("pri", dotsOr(redact(ev.AIR, "subject", ev.Subject, airOn), 26))
	whoTok := LaneToken(ev.Actor)
	if airOn && ev.AIR["actor"] != "ok" {
		whoTok = "mut"
	}
	who := C(whoTok, dotsOr(redact(ev.AIR, "actor", ev.Actor, airOn), 10))
	what := ev.Summary
	if strings.TrimSpace(what) == "" {
		what = shortKind(ev.Kind)
	}
	what = C("mut", redact(ev.AIR, "summary", what, airOn))
	return fmt.Sprintf("%s %s%s %s %s %s", ts, bar, glyph, subj, who, what)
}

// Trace is the unified-API READ contract for one LLM-observability row (mirrors
// reins_read.to_trace_row). Every field is operational metadata (model/tokens/cost/latency);
// the operator's prompt/completion (PII) NEVER enter the row — the livestream-safe projection
// of an LLM call. On-air every field is safe, so air[field]=="ok" for all.
type Trace struct {
	TS            string            `json:"ts"`
	TraceID       string            `json:"trace_id"`
	Model         string            `json:"model"`
	PromptTok     int               `json:"prompt_tok"`
	CompletionTok int               `json:"completion_tok"`
	TotalTok      int               `json:"total_tok"`
	Cost          float64           `json:"cost"`
	LatencyMs     int               `json:"latency_ms"`
	AIR           map[string]string `json:"air"`
}

// latencyBucket maps a latency (ms) to a 1..6 fill count on a scale tuned to LLM-call durations
// (sub-quarter-second … tens of seconds). Latency is a MAGNITUDE — carried by bar SHAPE, never a
// hue — so it cannot collide with the criticality channel (slow ≠ broken).
func latencyBucket(ms int) int {
	switch {
	case ms < 250:
		return 1
	case ms < 750:
		return 2
	case ms < 1500:
		return 3
	case ms < 3000:
		return 4
	case ms < 6000:
		return 5
	default:
		return 6
	}
}

// latencyHistogram: a fixed-width monochrome magnitude bar (█ fill · ░ remainder) + the precise ms
// label. Token is "mut" — shared ground, excluded from the channel-disjoint invariant — so the bar
// touches none of the three meaning-channels; the magnitude lives entirely in SHAPE. The numeric
// label is the Peirce redundant signal: text reads in grayscale where the bar cannot vary by hue.
func latencyHistogram(ms int) string {
	n := latencyBucket(ms)
	bar := strings.Repeat("█", n) + strings.Repeat("░", 6-n)
	return C("mut", bar) + " " + C("mut", fmt.Sprintf("%5dms", ms))
}

// RenderTraceHeader: the column header that SITUATES the LLM-spend stream; pads align under
// RenderTraceRow so the grid never jitters.
func RenderTraceHeader() string {
	return C("mut", fmt.Sprintf(" %-8s %-13s %-16s %-14s %-11s %s", "TIME", "LATENCY", "TRACE", "MODEL", "TOKENS p/c/t", "COST"))
}

// traceGlyphLabel projects a glyph-bearing encoded cell (time / measure) back into the legacy trace
// column envelope. The semantic channel still comes from EncodeCell; the leading channel glyph is
// withheld here only because RenderTraceRow's operator-vetted columns already carry latency shape in
// the six-block histogram and must remain byte/width stable.
func traceGlyphLabel(reg FacetRegistry, facet string, v CellValue, airOn bool) string {
	return traceEncodedGlyphLabel(EncodeCell(reg, facet, v, airOn))
}

func traceEncodedGlyphLabel(c Cell) string {
	plain := ansi.Strip(c.Rendered)
	r := []rune(plain)
	if len(r) >= 2 && r[1] == ' ' {
		return string(r[2:])
	}
	return plain
}

// traceTextCell keeps the legacy trace ink while delegating padding, truncation, structured silence,
// and AIR redaction to EncodeCell's text-channel grammar.
func traceTextCell(reg FacetRegistry, facet, token string, v CellValue, airOn bool) string {
	return C(token, ansi.Strip(EncodeCell(reg, facet, v, airOn).Rendered))
}

// RenderTraceRow: one self-explaining LLM-call row — TIME · LATENCY(histogram) · TRACE · MODEL ·
// TOKENS(prompt/completion/total) · COST. Magnitudes are SHAPE (bar) + TEXT (literal), never hue,
// so the row is channel-pure and monochrome-legible; `airOn` redacts per field (structure kept).
func RenderTraceRow(tr Trace, airOn bool) string {
	reg := FacetRegistry{} // built-in default binding (same offline path RenderTaskRow uses)
	d := func(field string) bool { return tr.AIR[field] != "ok" }

	tsText := compactTS(tr.TS)
	tsDenied := d("ts")
	if airOn && tsDenied {
		tsText = "▒▒▒▒▒▒▒▒" // legacy full-width timestamp redaction, not the generic ▒▒▒ token
		tsDenied = false
	}
	ts := traceGlyphLabel(reg, "time", CellValue{Text: tsText, Denied: tsDenied, Width: 8}, airOn)

	latDenied := d("latency_ms")
	n := latencyBucket(tr.LatencyMs)
	latCell := EncodeCell(reg, "measure", CellValue{
		Text:      fmt.Sprintf("%5dms", tr.LatencyMs),
		Magnitude: float64(n) / 6.0,
		Denied:    latDenied,
	}, airOn)
	var lat string
	if airOn && latDenied {
		// Keep the screenshot-vetted 9-cell latency redaction while still routing the field through
		// the measure facet; the visible legacy histogram shape must not gain the encoder's extra pip.
		lat = C("mut", "▒▒▒▒▒▒▒▒▒")
	} else {
		bar := strings.Repeat("█", n) + strings.Repeat("░", 6-n)
		label := traceEncodedGlyphLabel(latCell)
		lat = C("mut", bar) + " " + C("mut", label)
	}

	id := traceTextCell(reg, "identity", "pri", CellValue{Text: tr.TraceID, Denied: d("trace_id"), Width: 16}, airOn)
	model := traceTextCell(reg, "variant", "mut", CellValue{Text: tr.Model, Denied: d("model"), Width: 14}, airOn)
	tok := C("mut", traceGlyphLabel(reg, "measure", CellValue{
		Text:   fmt.Sprintf("%d/%d/%d", tr.PromptTok, tr.CompletionTok, tr.TotalTok),
		Denied: d("total_tok"),
		Width:  11,
	}, airOn))
	cost := C("mut", traceGlyphLabel(reg, "measure", CellValue{
		Text:   fmt.Sprintf("$%.6f", tr.Cost),
		Denied: d("cost"),
	}, airOn))
	return fmt.Sprintf("%s %s %s %s %s %s", ts, lat, id, model, tok, cost)
}

// --- session-pane turn grammar (§9 step-1: the row-level projection of a CapabilityIO turn-receipt;
// reins-design-ref). A turn is NOT a text blob — it is a typed, provenance-
// tagged unit normalized across Claude Code / Codex / Agy. This row is the RECEDED (turn-ladder)
// projection; the expanded block-tree is a later increment. ---

// Turn is the unified-API READ contract for one session-turn row. BIMODAL AIR (the governing rule):
// the SKELETON (ts/kind/role/model/route/gate/magnitude — operational metadata, the "x via y"
// provenance) airs; the BODY (summary — the turn content: operator free-text, model prose, tool
// output) DEFAULT-DENIES — so on air a turn collapses to its structurally-intact redacted skeleton
// (the Trace-row livestream-safe pattern generalized to a conversation turn).
type Turn struct {
	TS        string            `json:"ts"`
	Role      string            `json:"role"`      // lane/actor — LaneToken identity (skeleton)
	Kind      string            `json:"kind"`      // turn-kind key into turnGlyph (skeleton)
	Prov      string            `json:"prov"`      // provenance: operator|model|structured|untrusted (glyph channel)
	Summary   string            `json:"summary"`   // the BODY — default-deny on air
	Magnitude float64           `json:"magnitude"` // normalized 0..1 size (tokens/lines) -> ScoreBar
	Model     string            `json:"model"`     // served model ("x via y") (skeleton)
	Route     string            `json:"route"`     // route_id (skeleton)
	Gate      string            `json:"gate"`      // pass|deny|pending|"" — gate_output result (skeleton)
	Streaming bool              `json:"streaming"` // in-flight (still generating) — drives the two-frame broadcast (E4.6)
	Tokens    int               `json:"tokens"`    // tokens produced so far (the [N tok] shape; skeleton magnitude)
	AIR       map[string]string `json:"air"`
}

// provGlyph: the PROVENANCE channel — a first-class width-1 mark distinguishing WHO authored a turn's
// body, so the on-air policy applies BEFORE airing (the "distinguish free-text from structured" rule).
// operator-free-text ● (can contain anything); model ◐; structured/registry ◌; untrusted-external
// (MCP/web) ○. SHAPE, not hue.
func provGlyph(prov string) string {
	switch prov {
	case "operator":
		return "●"
	case "untrusted":
		return "○"
	case "structured":
		return "◌"
	default:
		return "◐" // model (default)
	}
}

// turnProvGloss decodes the turn AUTHORSHIP-provenance channel (distinct from the confidence-ladder
// provGloss). The glyphs intentionally echo the confidence ladder (no new vocabulary); the legend
// situates both readings.
var turnProvGloss = map[string]string{
	"operator":   "operator free-text — ALWAYS denied on air (can contain anything)",
	"model":      "model-authored text",
	"structured": "structured / registry value (may air per field)",
	"untrusted":  "untrusted external (MCP/web) — shape-only on air regardless",
}

// provDeniesOnAir: operator-free-text and untrusted-external bodies NEVER air, the per-field AIR map
// notwithstanding — the AIR allowlist cannot be trusted for a free-text or external-origin body.
func provDeniesOnAir(prov string) bool { return prov == "operator" || prov == "untrusted" }

// turnBodyForAir gates a turn/block BODY by BOTH the per-field AIR map AND the provenance override
// (operator/untrusted always redact on air); the redaction token stays structurally intact.
func turnBodyForAir(airMap map[string]string, prov, body string, airOn bool) string {
	if airOn && provDeniesOnAir(prov) {
		return redactToken
	}
	return redact(airMap, "summary", body, airOn)
}

var _ = turnProvGloss // legended via RenderLegend follow-up; referenced to keep the gloss authored

// turnGlyph: the closed turn-kind alphabet (one width-1 mark per turn-primitive, normalized across
// the three harnesses). PROVISIONAL marks — the operator vets the aesthetic on stream; the legend
// (RenderLegend) decodes every one (class-closure: TestLegendCoversTurnGlyphs). Some marks
// intentionally echo the event/criticality alphabet (◆ · ); the legend situates each reading.
var turnGlyph = map[string]string{
	"user": "❯", "attach": "▤", "slash": "/", "file": "@",
	"assistant": "✦", "reasoning": "⟂",
	"tool_call": "◆", "tool_result": "⎿", "diff": "±",
	"plan": "⊞", "todo": "▢", "approval": "⊜",
	"dispatch": "⇉", "interrupt": "⊘", "refusal": "⊗",
	"status": "·", "usage": "$", "mcp": "⊕", "web": "⊙", "lifecycle": "◴",
}

var turnKindOrder = []string{"user", "attach", "slash", "file", "assistant", "reasoning",
	"tool_call", "tool_result", "diff", "plan", "todo", "approval", "dispatch", "interrupt",
	"refusal", "status", "usage", "mcp", "web", "lifecycle"}

var turnKindGloss = map[string]string{
	"user": "operator prompt (free-text — denied on air)", "attach": "attachment / pasted image (denied on air)",
	"slash": "slash-command + output", "file": "@file mention",
	"assistant": "agent message",
	"reasoning": "thinking / reasoning (denied on air)", "tool_call": "tool invoked (name + args)",
	"tool_result": "tool output (stdout/exit — body denied on air)", "diff": "file change (hunks)",
	"plan": "plan — review before execute", "todo": "task / checklist item",
	"approval": "approval / gate awaiting decision", "dispatch": "dispatched to a lane / sub-agent",
	"interrupt": "interrupted / cancelled", "refusal": "refused (with category)",
	"status": "status / heartbeat", "usage": "usage — tokens / cost / context%",
	"mcp": "MCP tool (external — untrusted)", "web": "web / search result (external)",
	"lifecycle": "turn lifecycle (start / end / refresh)",
}

func turnGlyphOr(kind string) string {
	if g, ok := turnGlyph[kind]; ok {
		return g
	}
	return "✶" // generic turn (legended under EVENTS)
}

// gateSeverity maps a gate_output result to a criticality word so a denied gate reads as critical
// (gate is genuinely a criticality-bearing state, so it legitimately rides the hue channel).
func gateSeverity(gate string) string {
	switch gate {
	case "deny":
		return "crit"
	case "pending":
		return "warn"
	case "pass":
		return "ok"
	}
	return ""
}

// RenderTurnHeader: the column header that SITUATES the turn-ladder (k = magnitude-bar + kind glyph).
func RenderTurnHeader() string {
	return C("mut", fmt.Sprintf(" %-8s %-2s %-10s %-14s %-9s %-1s %s", "TIME", "k", "WHO", "MODEL", "GATE", "p", "WHAT"))
}

// RenderTurnRow: one self-explaining session-turn row — TIME · magnitude-bar · kind-glyph · WHO(lane)
// · MODEL(x-via-y) · GATE · WHAT(body). The body default-denies on air; the skeleton stays legible —
// the structurally-intact, redacted, livestream-safe projection of a turn. Magnitude is SHAPE (bar),
// never hue; the kind is SHAPE (glyph), never hue; only GATE rides the criticality hue (it is a state).
func RenderTurnRow(t Turn, airOn bool) string {
	ts := compactTS(t.TS)
	if airOn && t.AIR["ts"] != "ok" {
		ts = pad("▒▒▒▒▒▒▒▒", 8)
	}
	bar := ScoreBar(t.Magnitude)
	if airOn && t.AIR["magnitude"] != "ok" {
		bar = "▒"
	}
	bar = C("mut", bar)
	glyph := turnGlyphOr(t.Kind)
	if airOn && t.AIR["kind"] != "ok" {
		glyph = "▒"
	}
	glyph = C("mut", glyph) // turn-kind is SHAPE, not a meaning-channel hue
	roleTok := LaneToken(t.Role)
	if airOn && t.AIR["role"] != "ok" {
		roleTok = "mut"
	}
	who := C(roleTok, dotsOr(redact(t.AIR, "role", t.Role, airOn), 10))
	model := C("mut", dotsOr(redact(t.AIR, "model", t.Model, airOn), 14))
	gate := pad("", 9)
	if t.Gate != "" {
		g := redact(t.AIR, "gate", t.Gate, airOn)
		if sev := gateSeverity(t.Gate); sev != "" && !(airOn && t.AIR["gate"] != "ok") {
			gate = C(SeverityToken(sev), pad("["+g+"]", 9))
		} else {
			gate = C("mut", pad("["+g+"]", 9))
		}
	}
	prov := C("mut", provGlyph(t.Prov)) // provenance is SHAPE, decoded by the legend
	what := C("mut", turnBodyForAir(t.AIR, t.Prov, t.Summary, airOn))
	return fmt.Sprintf("%s %s%s %s %s %s %s %s", ts, bar, glyph, who, model, gate, prov, what)
}

// TurnBlock is one sub-unit inside a turn (reasoning / tool_call / tool_result / diff / …) — the
// expanded-detail granularity of the normalized turn-receipt. BIMODAL AIR holds per block: META
// (exit code · line count · tool name · model — operational skeleton) airs; the BODY default-denies.
type TurnBlock struct {
	Kind      string            `json:"kind"`      // sub-unit turn-kind (key into turnGlyph)
	Prov      string            `json:"prov"`      // provenance (operator|model|structured|untrusted)
	Summary   string            `json:"summary"`   // the BODY — default-deny on air
	Magnitude float64           `json:"magnitude"` // normalized 0..1 size -> ScoreBar
	Meta      string            `json:"meta"`      // air-safe skeleton (exit/lines/tool/model)
	AIR       map[string]string `json:"air"`
}

// RenderTurnDetail: the EXPANDED (summoned / present-at-hand) projection of one turn — its header
// row plus an indented ASCII tree of its blocks. The complement of RenderTurnRow's receded one-liner
// (progressive disclosure: recede -> summon). LAYOUT-AGNOSTIC by construction: it returns a standalone
// string; WHERE it mounts (inline unfold vs detail pane vs split) is the cockpit-LAYOUT concern and is
// deliberately NOT decided here (the operator's split-screen / Yard-coordinator strategy owns that).
func RenderTurnDetail(t Turn, blocks []TurnBlock, airOn bool) string {
	var b strings.Builder
	b.WriteString(RenderTurnRow(t, airOn) + "\n")
	for i, blk := range blocks {
		conn := "├─"
		if i == len(blocks)-1 {
			conn = "└─"
		}
		glyph := turnGlyphOr(blk.Kind)
		if airOn && blk.AIR["kind"] != "ok" {
			glyph = "▒"
		}
		bar := ScoreBar(blk.Magnitude)
		if airOn && blk.AIR["magnitude"] != "ok" {
			bar = "▒"
		}
		bar = C("mut", bar)
		meta := ""
		if blk.Meta != "" {
			meta = C("mut", redact(blk.AIR, "meta", blk.Meta, airOn)) + "  "
		}
		// tool_result stdout + MCP/web bodies are the highest-blast-radius blocks → provenance-gate them
		// like the top-level body (untrusted/operator never air, AIR map notwithstanding).
		body := C("mut", turnBodyForAir(blk.AIR, blk.Prov, blk.Summary, airOn))
		b.WriteString("   " + C("border", conn) + " " + bar + C("mut", glyph) + " " + meta + body + "\n")
	}
	return b.String()
}

// Task is the unified-API READ contract for one registry row (mirrors reins_read.to_task).
type Task struct {
	TaskID         string            `json:"task_id"`
	Stage          string            `json:"stage"`
	AuthorityCase  string            `json:"authority_case"`
	NoGo           string            `json:"no_go"`
	PriorStage     string            `json:"prior_stage"`     // D6 — the stage transitioned FROM
	PredictedStage string            `json:"predicted_stage"` // D7 — the expected next stage
	Owner          string            `json:"owner"`           // who — last actor / lane
	Freshness      float64           `json:"freshness"`       // exp(-age/τ)
	Criticality    string            `json:"criticality"`     // D4 — ok|warn|major|crit
	RelCount       int               `json:"rel_count"`       // D2 — live relationship ties
	AIR            map[string]string `json:"air"`
}

// RenderTaskHeader: the column header for the encoder-composed task row (posture carries criticality
// as glyph+word, so there is no separate state/crit column anymore; relations stay as the edge cell).
func RenderTaskHeader() string {
	return C("mut", fmt.Sprintf("%-7s %-3s %-22s %-4s %-5s %-5s %-8s %s",
		"state", "rel", "TASK", "STG", "was◀", "→next", "who", "fr"))
}

// dotsOr: structured-silence — an empty cell is dots at full width (the grid never jitters).
func dotsOr(s string, n int) string {
	if strings.TrimSpace(s) == "" {
		return strings.Repeat("·", n)
	}
	return pad(s, n)
}

// shortStage strips the SDLC stage suffix: "S7_RELEASE" -> "S7".
func shortStage(s string) string {
	if i := strings.IndexByte(s, '_'); i >= 0 {
		return s[:i]
	}
	return s
}

// critGlyph: the state glyph carries criticality monochrome-safe (✖ crit · ‼ major · ▸ warn · ✓ ok).
var critGlyph = map[string]string{"crit": "✖", "major": "‼", "warn": "▸", "ok": "✓"}

// critBar: criticality as a fixed-width fill bar (magnitude via fill only, per the grammar rule).
func critBar(crit string) string {
	n := map[string]int{"ok": 1, "warn": 2, "major": 3, "crit": 4}[crit]
	if n == 0 {
		n = 1
	}
	return strings.Repeat("█", n) + strings.Repeat("░", 4-n)
}

// freshGlyph: freshness (0..1) -> an eighth-block + brightness token (recent=bright, stale=muted).
func freshGlyph(f float64) (string, string) {
	bars := []rune("▁▂▃▄▅▆▇█")
	i := int(f * 8)
	if i > 7 {
		i = 7
	} else if i < 0 {
		i = 0
	}
	switch {
	case f > 0.6:
		return string(bars[i]), "grn"
	case f > 0.2:
		return string(bars[i]), "pri"
	}
	return string(bars[i]), "mut"
}

// relCell: the relations edge SUMMARY cell (●N) — NOT a facet (a relation is a property of a PAIR, the
// edge layer §3), kept in the row alongside the facet cells; blue, count-bearing, redacts under AIR.
func relCell(n int, denied, airOn bool) string {
	if airOn && denied {
		return C("mut", pad("▒▒▒", 3))
	}
	if n > 0 {
		return C("blu", pad(fmt.Sprintf("●%d", n), 3))
	}
	return C("mut", pad("●0", 3))
}

// RenderTaskRow: the task roster row, now COMPOSED THROUGH THE ENCODER (framework §1 Layer-2 —
// "every pane renders the same way"). Each facet cell (posture · id · stage · ◀was · →next · who ·
// freshness) is the encoder's uniform channel grammar — the SAME grammar every kind uses — plus the
// relations edge cell (●N). The old hand-rolled per-cell strip is dissolved into EncodeCell; the
// redundant criticality BAR is dropped (posture already carries criticality as glyph+word+hue, and
// "blocked" reads from the posture cell, not a colored predicted-stage). Empties are structured-
// silence dots; denied cells redact in place — both now properties of the encoder, not this renderer.
func RenderTaskRow(t Task, airOn bool) string {
	crit := t.Criticality
	if crit == "" {
		crit = "ok"
	}
	reg := FacetRegistry{} // built-in default binding (the row renderer has no live registry handle)
	d := func(field string) bool { return t.AIR[field] != "ok" }
	// the ◀ travels WITH the prior-stage value (self-distinguishing even if the header is cropped).
	prior := shortStage(t.PriorStage)
	if strings.TrimSpace(prior) != "" {
		prior += "◀"
	}
	pred := t.PredictedStage // the →next chip ("·ship" = terminal/arrived; "→X" = pending move)
	switch pred {
	case "", "ship":
		if pred == "ship" {
			pred = "·ship"
		}
	default:
		pred = "→" + pred
	}
	cell := func(facet, text string, field string, width int) string {
		return EncodeCell(reg, facet, CellValue{Text: text, Denied: d(field), Width: width}, airOn).Rendered
	}
	posture := cell("posture", crit, "criticality", 5)
	rel := relCell(t.RelCount, d("rel_count"), airOn)
	id := cell("identity", t.TaskID, "task_id", 22)
	stg := cell("action", shortStage(t.Stage), "stage", 4)
	was := cell("action", prior, "prior_stage", 5)
	nxt := cell("action", pred, "predicted_stage", 5)
	who := cell("ownership", t.Owner, "owner", 8)
	fr := EncodeCell(reg, "time", CellValue{Magnitude: t.Freshness, Denied: d("freshness")}, airOn).Rendered
	return strings.Join([]string{posture, rel, id, stg, was, nxt, who, fr}, " ")
}

// Session is the unified-API READ contract for one live agent/session lane. It is deliberately a
// roster/health projection, not a transcript or PTY stream: raw session content stays outside AIR
// until the governed command surface exists.
type Session struct {
	Role              string            `json:"role"`
	Session           string            `json:"session"`
	Platform          string            `json:"platform"`
	State             string            `json:"state"`
	Readiness         string            `json:"readiness"`
	Blocker           string            `json:"blocker"`
	Attention         float64           `json:"attention"`
	Alive             bool              `json:"alive"`
	Idle              bool              `json:"idle"`
	Stalled           bool              `json:"stalled"`
	ClaimedTask       string            `json:"claimed_task"`
	RouteID           string            `json:"route_id"`
	RouteMode         string            `json:"mode"`
	RouteProfile      string            `json:"profile"`
	RouteBindingState string            `json:"route_binding_state"`
	RouteEvidenceRef  string            `json:"route_evidence_ref"`
	OutputAgeS        float64           `json:"output_age_s"`
	RelayAgeS         float64           `json:"relay_age_s"`
	AIR               map[string]string `json:"air"`
}

type SessionHealth struct {
	Alive      bool    `json:"alive"`
	Idle       bool    `json:"idle"`
	Stalled    bool    `json:"stalled"`
	OutputAgeS float64 `json:"output_age_s"`
	RelayAgeS  float64 `json:"relay_age_s"`
}

type SessionTmux struct {
	Session      string  `json:"session"`
	Exists       bool    `json:"exists"`
	Attached     bool    `json:"attached"`
	ActivityAgeS float64 `json:"activity_age_s"`
}

type SessionTaskDetail struct {
	TaskID          string `json:"task_id"`
	Status          string `json:"status"`
	AssignedTo      string `json:"assigned_to"`
	AuthorityCase   string `json:"authority_case"`
	ParentSpec      string `json:"parent_spec"`
	MutationSurface string `json:"mutation_surface"`
	UpdatedAt       string `json:"updated_at"`
}

type EvidenceRef struct {
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	MTime     string `json:"mtime"`
	Size      int    `json:"size"`
	Privacy   string `json:"privacy"`
	RawAccess bool   `json:"raw_access"`
}

type ResumeContext struct {
	Intent         string   `json:"intent"`
	Ready          bool     `json:"ready"`
	Authority      string   `json:"authority"`
	BlockedReasons []string `json:"blocked_reasons"`
}

type SessionEvidenceSummary struct {
	Total                   int            `json:"total"`
	ByKind                  map[string]int `json:"by_kind"`
	TranscriptRootsObserved int            `json:"transcript_roots_observed"`
	TranscriptRootsMissing  int            `json:"transcript_roots_missing"`
	Truncated               bool           `json:"truncated"`
	Privacy                 string         `json:"privacy"`
	RawAccess               bool           `json:"raw_access"`
}

type SessionDetail struct {
	Role            string                 `json:"role"`
	Platform        string                 `json:"platform"`
	State           string                 `json:"state"`
	Readiness       string                 `json:"readiness"`
	Blocker         string                 `json:"blocker"`
	Attention       float64                `json:"attention"`
	Health          SessionHealth          `json:"health"`
	Tmux            SessionTmux            `json:"tmux"`
	Task            SessionTaskDetail      `json:"task"`
	EvidenceRefs    []EvidenceRef          `json:"evidence_refs"`
	EvidenceSummary SessionEvidenceSummary `json:"evidence_summary"`
	Resume          ResumeContext          `json:"resume"`
	AIR             map[string]string      `json:"air"`
}

// IntakeSource is a bounded metadata row for one durable intake source. It names the source,
// freshness, count, and evidence ref posture without reading raw inbox/note/notification bodies.
type IntakeSource struct {
	ID        string            `json:"id"`
	Path      string            `json:"path"`
	Exists    bool              `json:"exists"`
	MTime     string            `json:"mtime"`
	AgeBucket string            `json:"age_bucket"`
	Status    string            `json:"status"`
	Count     int               `json:"count"`
	Privacy   string            `json:"privacy"`
	RawAccess bool              `json:"raw_access"`
	AIR       map[string]string `json:"air"`
}

// IntakeRow is an aggregate observation/demand row. Request IDs, note bodies, notification messages,
// URLs, and raw evidence refs stay outside this first-order read model.
type IntakeRow struct {
	ID            string            `json:"id"`
	Source        string            `json:"source"`
	Kind          string            `json:"kind"`
	Status        string            `json:"status"`
	Severity      string            `json:"severity"`
	Count         int               `json:"count"`
	Blocker       string            `json:"blocker"`
	Coverage      string            `json:"coverage"`
	TaskLinkState string            `json:"task_link_state"`
	EvidenceCount int               `json:"evidence_count"`
	AgeBucket     string            `json:"age_bucket"`
	Authority     string            `json:"authority"`
	Evidence      string            `json:"evidence"`
	Missing       string            `json:"missing"`
	Action        string            `json:"action"`
	Detail        string            `json:"detail"`
	SourceRefs    string            `json:"source_refs"`
	NextEvidence  string            `json:"next_evidence"`
	AIR           map[string]string `json:"air"`
}

type IntakeSummary struct {
	Sources []IntakeSource `json:"sources"`
	Rows    []IntakeRow    `json:"rows"`
	Totals  map[string]int `json:"totals"`
}

// CapabilitySource is a metadata-only source row for capability routing. Paths and details are
// evidence pointers, not execution authority.
type CapabilitySource struct {
	ID        string            `json:"id"`
	Path      string            `json:"path"`
	Exists    bool              `json:"exists"`
	MTime     string            `json:"mtime"`
	AgeBucket string            `json:"age_bucket"`
	Status    string            `json:"status"`
	Count     int               `json:"count"`
	Detail    string            `json:"detail"`
	Privacy   string            `json:"privacy"`
	RawAccess bool              `json:"raw_access"`
	AIR       map[string]string `json:"air"`
}

// CapabilityRow is capability-first. Platform routes are evidence below this level, never the
// ontology of the capability page.
type CapabilityRow struct {
	CapabilityID       string            `json:"capability_id"`
	Status             string            `json:"status"`
	Authority          string            `json:"authority"`
	CapabilityClass    string            `json:"capability_class"`
	SurfaceFamily      string            `json:"surface_family"`
	SpendModel         string            `json:"spend_model"`
	EgressClass        string            `json:"egress_class"`
	ReceiptRequirement string            `json:"receipt_requirement"`
	RouteCount         int               `json:"route_count"`
	OKCount            int               `json:"ok_count"`
	BlockedCount       int               `json:"blocked_count"`
	EvidenceCount      int               `json:"evidence_count"`
	Blocker            string            `json:"blocker"`
	HKPPosture         string            `json:"hkp_posture"`
	SourceRefs         string            `json:"source_refs"`
	SourceRefLabels    []string          `json:"source_ref_labels"`
	AIR                map[string]string `json:"air"`
}

// CapabilityRoute is route evidence for a capability. It does not select or launch the route.
type CapabilityRoute struct {
	RouteID             string            `json:"route_id"`
	CapabilityID        string            `json:"capability_id"`
	Platform            string            `json:"platform"`
	Mode                string            `json:"mode"`
	Profile             string            `json:"profile"`
	ModelID             string            `json:"model_id"`
	Effort              string            `json:"effort"`
	ContextMode         string            `json:"context_mode"`
	FastMode            string            `json:"fast_mode"`
	Quantization        string            `json:"quantization"`
	CapacityPool        string            `json:"capacity_pool"`
	DemandVector        string            `json:"demand_vector"`
	Hardening           string            `json:"hardening"`
	EvalPlane           string            `json:"eval_plane"`
	ReviewObligation    string            `json:"review_obligation"`
	LearningEligibility string            `json:"learning_eligibility"`
	BenchmarkCoverage   string            `json:"benchmark_coverage"`
	FixedOverhead       string            `json:"fixed_overhead"`
	RouteState          string            `json:"route_state"`
	AuthorityCeiling    string            `json:"authority_ceiling"`
	FreshnessOK         bool              `json:"freshness_ok"`
	QuotaState          string            `json:"quota_state"`
	ReceiptCount        int               `json:"receipt_count"`
	Blockers            []string          `json:"blockers"`
	EvidenceCount       int               `json:"evidence_count"`
	AIR                 map[string]string `json:"air"`
}

// CapabilityTool is route-level tool evidence. It is not bound to an individual live session until
// session records expose a route id, so renderers must label it as candidate route evidence.
type CapabilityTool struct {
	RouteID      string            `json:"route_id"`
	Platform     string            `json:"platform"`
	ToolID       string            `json:"tool_id"`
	Status       string            `json:"status"`
	Available    bool              `json:"available"`
	AuthorityUse string            `json:"authority_use"`
	ObservedAt   string            `json:"observed_at"`
	StaleAfter   string            `json:"stale_after"`
	EvidenceRef  string            `json:"evidence_ref"`
	Privacy      string            `json:"privacy"`
	RawAccess    bool              `json:"raw_access"`
	AIR          map[string]string `json:"air"`
}

type CapabilitySummary struct {
	Sources []CapabilitySource `json:"sources"`
	Rows    []CapabilityRow    `json:"rows"`
	Routes  []CapabilityRoute  `json:"routes"`
	Tools   []CapabilityTool   `json:"tools"`
	Totals  map[string]int     `json:"totals"`
}

// GateSource is a metadata-only source row for readiness/gate truth.
type GateSource struct {
	ID        string            `json:"id"`
	Status    string            `json:"status"`
	Count     int               `json:"count"`
	Detail    string            `json:"detail"`
	AgeBucket string            `json:"age_bucket"`
	Path      string            `json:"path"`
	Privacy   string            `json:"privacy"`
	RawAccess bool              `json:"raw_access"`
	AIR       map[string]string `json:"air"`
}

// GateRow preserves the exact gate/blocker name that is currently stopping action.
type GateRow struct {
	GateID    string            `json:"gate_id"`
	Domain    string            `json:"domain"`
	Source    string            `json:"source"`
	Subject   string            `json:"subject"`
	State     string            `json:"state"`
	Severity  string            `json:"severity"`
	Authority string            `json:"authority"`
	Evidence  string            `json:"evidence"`
	Missing   string            `json:"missing"`
	Action    string            `json:"action"`
	AIR       map[string]string `json:"air"`
}

type GateSummary struct {
	Sources []GateSource   `json:"sources"`
	Rows    []GateRow      `json:"rows"`
	Totals  map[string]int `json:"totals"`
}

// DomainSource is a metadata-only source row for optional source-backed lifecycle/domain packs.
type DomainSource struct {
	ID        string            `json:"id"`
	Path      string            `json:"path"`
	Exists    bool              `json:"exists"`
	Status    string            `json:"status"`
	Count     int               `json:"count"`
	AgeBucket string            `json:"age_bucket"`
	Authority string            `json:"authority"`
	Detail    string            `json:"detail"`
	Privacy   string            `json:"privacy"`
	RawAccess bool              `json:"raw_access"`
	AIR       map[string]string `json:"air"`
}

// DomainRow is an extensible SDLC/RDLC/n-DLC row. It is evidence/navigation, not route authority.
type DomainRow struct {
	DomainID         string            `json:"domain_id"`
	Label            string            `json:"label"`
	Lifecycle        string            `json:"lifecycle"`
	Terrain          string            `json:"terrain"`
	Depth            string            `json:"depth"`
	Scope            string            `json:"scope"`
	State            string            `json:"state"`
	AuthorityCeiling string            `json:"authority_ceiling"`
	ClaimCeiling     string            `json:"claim_ceiling"`
	Windows          string            `json:"windows"`
	Surfaces         string            `json:"surfaces"`
	Parity           string            `json:"parity"`
	EvidenceCount    int               `json:"evidence_count"`
	Blocker          string            `json:"blocker"`
	SourceRefs       string            `json:"source_refs"`
	AIR              map[string]string `json:"air"`
}

type DomainRelation struct {
	Source           string            `json:"source"`
	Target           string            `json:"target"`
	Relation         string            `json:"relation"`
	AuthorityCeiling string            `json:"authority_ceiling"`
	SourceRefs       string            `json:"source_refs"`
	AIR              map[string]string `json:"air"`
}

// LifecycleRow is an authority-aware tenant lifecycle contract. SDLC/RDLC/LDLC are instance
// examples, not product enums; future n-DLC rows should load through the same shape.
type LifecycleRow struct {
	LifecycleID      string            `json:"lifecycle_id"`
	Label            string            `json:"label"`
	Owner            string            `json:"owner"`
	Scope            string            `json:"scope"`
	Plant            string            `json:"plant"`
	Posture          string            `json:"posture"`
	State            string            `json:"state"`
	Maturity         string            `json:"maturity"`
	AdapterID        string            `json:"adapter_id"`
	AuthorityCeiling string            `json:"authority_ceiling"`
	ClaimSurface     string            `json:"claim_surface"`
	MutationSurface  string            `json:"mutation_surface"`
	DarkPolicy       string            `json:"dark_policy"`
	FreshnessPolicy  string            `json:"freshness_policy"`
	AIRClass         string            `json:"air_class"`
	Windows          string            `json:"windows"`
	Surfaces         string            `json:"surfaces"`
	Commands         string            `json:"commands"`
	ReceiptContracts string            `json:"receipt_contracts"`
	EvidenceCount    int               `json:"evidence_count"`
	Blocker          string            `json:"blocker"`
	NextEvidence     string            `json:"next_evidence"`
	SourceRefs       string            `json:"source_refs"`
	AIR              map[string]string `json:"air"`
}

type DomainSummary struct {
	Sources              []DomainSource   `json:"sources"`
	Rows                 []DomainRow      `json:"rows"`
	Relations            []DomainRelation `json:"relations"`
	Totals               map[string]int   `json:"totals"`
	Authority            string           `json:"authority"`
	GeneratedAt          string           `json:"generated_at"`
	PackageHash          string           `json:"package_hash"`
	DefaultLens          string           `json:"default_lens"`
	LifecycleSources     []DomainSource   `json:"lifecycle_sources"`
	Lifecycles           []LifecycleRow   `json:"lifecycles"`
	LifecycleTotals      map[string]int   `json:"lifecycle_totals"`
	LifecycleAuthority   string           `json:"lifecycle_authority"`
	LifecycleGeneratedAt string           `json:"lifecycle_generated_at"`
	LifecyclePackageHash string           `json:"lifecycle_package_hash"`
	LifecycleDefaultLens string           `json:"lifecycle_default_lens"`
}

// RenderSessionHeader: a compact live-lane roster. The first cell is a health glyph; task is AIR
// denied by default because task ids can carry incident text.
func RenderSessionHeader() string {
	return C("mut", fmt.Sprintf(" %-1s %-5s %-13s %-7s %-8s %-5s %-7s %-7s %s",
		"h", "RDY", "ROLE", "PLAT", "STATE", "ATTN", "OUT", "RELAY", "TASK"))
}

func sessionToken(state string) string {
	switch state {
	case "active":
		return "grn"
	case "idle":
		return "yel"
	case "stalled", "offline":
		return "red"
	}
	return "mut"
}

func readinessToken(readiness string) string {
	switch readiness {
	case "claim", "live":
		return "grn"
	case "idle", "stale":
		return "yel"
	case "stall", "off", "offline":
		return "red"
	}
	return "mut"
}

func sessionHealthVisible(s Session, airOn bool) bool {
	if !airOn {
		return true
	}
	for _, f := range []string{"state", "alive", "idle", "stalled"} {
		if s.AIR[f] != "ok" {
			return false
		}
	}
	return true
}

func sessionGlyph(s Session, airOn bool) string {
	if !sessionHealthVisible(s, airOn) {
		return "▒"
	}
	switch {
	case s.Stalled:
		return "!"
	case !s.Alive:
		return "○"
	case s.Idle:
		return "·"
	default:
		return "●"
	}
}

func compactAge(age float64) string {
	if age <= 0 {
		return "·"
	}
	switch {
	case age < 60:
		return fmt.Sprintf("%.0fs", age)
	case age < 3600:
		return fmt.Sprintf("%.0fm", age/60)
	case age < 86400:
		return fmt.Sprintf("%.1fh", age/3600)
	}
	return fmt.Sprintf("%.1fd", age/86400)
}

func compactAttention(score float64) string {
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return fmt.Sprintf("%s%.2f", ScoreBar(score), score)
}

// RenderSessionRow renders one lane without exposing raw output. AIR gates every value by field,
// including the derived health ages; default config leaves claimed_task redacted.
func RenderSessionRow(s Session, airOn bool) string {
	tok := sessionToken(s.State)
	if !sessionHealthVisible(s, airOn) {
		tok = "mut"
	}
	glyph := C(tok, sessionGlyph(s, airOn))
	roleTok := LaneToken(s.Role)
	if airOn && s.AIR["role"] != "ok" {
		roleTok = "mut"
	}
	rdyTok := readinessToken(s.Readiness)
	if airOn && s.AIR["readiness"] != "ok" {
		rdyTok = "mut"
	}
	rdy := C(rdyTok, dotsOr(redact(s.AIR, "readiness", s.Readiness, airOn), 5))
	role := C(roleTok, dotsOr(redact(s.AIR, "role", s.Role, airOn), 13))
	plat := C("2nd", dotsOr(redact(s.AIR, "platform", s.Platform, airOn), 7))
	state := C(tok, dotsOr(redact(s.AIR, "state", s.State, airOn), 8))
	attn := C("pri", dotsOr(redact(s.AIR, "attention", compactAttention(s.Attention), airOn), 5))
	out := C("mut", dotsOr(redact(s.AIR, "output_age_s", compactAge(s.OutputAgeS), airOn), 7))
	relay := C("mut", dotsOr(redact(s.AIR, "relay_age_s", compactAge(s.RelayAgeS), airOn), 7))
	taskTok := "2nd"
	if airOn && s.AIR["claimed_task"] != "ok" {
		taskTok = "mut"
	}
	taskVal := redact(s.AIR, "claimed_task", s.ClaimedTask, airOn)
	if strings.TrimSpace(taskVal) == "" {
		taskVal = "·····"
	}
	task := C(taskTok, dotsOr(taskVal, 48))
	return fmt.Sprintf("%s %s %s %s %s %s %s %s %s", glyph, rdy, role, plat, state, attn, out, relay, task)
}

// --- :dynamics — the system-dynamics map (obsoletes the standalone :8765 cytoscape viewer) ---

// Layer / Node / Edge mirror reins_read's to_node/to_edge + the seed's layer list.
type Layer struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type Node struct {
	ID              string            `json:"id"`
	Label           string            `json:"label"`
	Kind            string            `json:"kind"`
	Layer           string            `json:"layer"`
	Status          string            `json:"status"`
	Res             string            `json:"res"`
	Summary         string            `json:"summary"`
	Context         string            `json:"context"`
	Docs            string            `json:"docs"`
	HardeningNotes  string            `json:"hardening_notes"`
	Aliases         string            `json:"aliases"`
	Tags            string            `json:"tags"`
	SourceRefs      string            `json:"source_refs"`
	SourceRefLabels []string          `json:"source_ref_labels"`
	AIR             map[string]string `json:"air"`
}
type Edge struct {
	ID              string            `json:"id"`
	Source          string            `json:"source"`
	Target          string            `json:"target"`
	Relation        string            `json:"relation"`
	Sign            string            `json:"sign"`
	Delay           bool              `json:"delay"`
	Prov            string            `json:"prov"`
	Status          string            `json:"status"`
	Layer           string            `json:"layer"`
	Res             string            `json:"res"`
	Confidence      string            `json:"confidence"`
	Summary         string            `json:"summary"`
	Docs            string            `json:"docs"`
	SourceRefs      string            `json:"source_refs"`
	SourceRefLabels []string          `json:"source_ref_labels"`
	AIR             map[string]string `json:"air"`
}
type DynamicsSource struct {
	ID        string            `json:"id"`
	Status    string            `json:"status"`
	Count     int               `json:"count"`
	Detail    string            `json:"detail"`
	AgeBucket string            `json:"age_bucket"`
	Path      string            `json:"path"`
	Privacy   string            `json:"privacy"`
	RawAccess bool              `json:"raw_access"`
	AIR       map[string]string `json:"air"`
}
type DynamicsRow struct {
	Kind     string            `json:"kind"`
	ID       string            `json:"id"`
	Source   string            `json:"source"`
	Status   string            `json:"status"`
	Severity string            `json:"severity"`
	Count    int               `json:"count"`
	Detail   string            `json:"detail"`
	AIR      map[string]string `json:"air"`
}
type DynamicsWorkbenchDefaults struct {
	InquiryMode     string            `json:"inquiry_mode"`
	AudienceMode    string            `json:"audience_mode"`
	ExplanationPath string            `json:"explanation_path"`
	AIR             map[string]string `json:"air"`
}
type DynamicsWorkbenchInquiry struct {
	ID           string            `json:"id"`
	Label        string            `json:"label"`
	Lens         string            `json:"lens"`
	Prompt       string            `json:"prompt"`
	AnswerShape  []string          `json:"answer_shape"`
	FocusNodeIDs []string          `json:"focus_node_ids"`
	FocusEdgeIDs []string          `json:"focus_edge_ids"`
	AIR          map[string]string `json:"air"`
}
type DynamicsWorkbenchAudience struct {
	ID       string            `json:"id"`
	Label    string            `json:"label"`
	Emphasis string            `json:"emphasis"`
	AIR      map[string]string `json:"air"`
}
type DynamicsWorkbenchScene struct {
	Title          string            `json:"title"`
	Lens           string            `json:"lens"`
	SelectionGroup string            `json:"selection_group"`
	SelectionID    string            `json:"selection_id"`
	Takeaway       string            `json:"takeaway"`
	Caveat         string            `json:"caveat"`
	AIR            map[string]string `json:"air"`
}
type DynamicsWorkbenchExplanation struct {
	ID          string                   `json:"id"`
	Label       string                   `json:"label"`
	Summary     string                   `json:"summary"`
	MustInclude []string                 `json:"must_include"`
	SceneCount  int                      `json:"scene_count"`
	Scenes      []DynamicsWorkbenchScene `json:"scenes"`
	AIR         map[string]string        `json:"air"`
}
type DynamicsWorkbench struct {
	Status           string                         `json:"status"`
	Missing          string                         `json:"missing"`
	Defaults         DynamicsWorkbenchDefaults      `json:"defaults"`
	InquiryModes     []DynamicsWorkbenchInquiry     `json:"inquiry_modes"`
	AudienceModes    []DynamicsWorkbenchAudience    `json:"audience_modes"`
	ExplanationPaths []DynamicsWorkbenchExplanation `json:"explanation_paths"`
	FollowOnTranches []string                       `json:"follow_on_tranches"`
	AIR              map[string]string              `json:"air"`
}
type DynamicsPackage struct {
	Sources      []DynamicsSource  `json:"sources"`
	Validation   []DynamicsRow     `json:"validation"`
	Lenses       []DynamicsRow     `json:"lenses"`
	Claims       []DynamicsRow     `json:"claims"`
	Observations []DynamicsRow     `json:"observations"`
	Relations    []DynamicsRow     `json:"relations"`
	Totals       map[string]int    `json:"totals"`
	Authority    string            `json:"authority_case"`
	GeneratedAt  string            `json:"generated_at"`
	PackageHash  string            `json:"package_hash"`
	DefaultLens  string            `json:"default_lens"`
	Workbench    DynamicsWorkbench `json:"workbench_contract"`
}

// EpistemicSource is a metadata-only source row for the typed epistemics read model.
// It names evidence channels and source health without exposing source bodies.
type EpistemicSource struct {
	ID        string            `json:"id"`
	Status    string            `json:"status"`
	Count     int               `json:"count"`
	Detail    string            `json:"detail"`
	AgeBucket string            `json:"age_bucket"`
	Path      string            `json:"path"`
	Privacy   string            `json:"privacy"`
	RawAccess bool              `json:"raw_access"`
	AIR       map[string]string `json:"air"`
}

// EpistemicReadRow is the typed source-backed reference row used by :epistemics.
// Map identity fields are structural joins; subject/detail/source bodies remain AIR-gated.
type EpistemicReadRow struct {
	RowID           string            `json:"row_id"`
	Family          string            `json:"family"`
	SubjectKind     string            `json:"subject_kind"`
	SubjectRef      string            `json:"subject_ref"`
	Subject         string            `json:"subject"`
	Status          string            `json:"status"`
	Posture         string            `json:"posture"`
	Authority       string            `json:"authority"`
	AuthorityCase   string            `json:"authority_case"`
	EvidenceCount   int               `json:"evidence_count"`
	Evidence        string            `json:"evidence"`
	Source          string            `json:"source"`
	SourceRefs      string            `json:"source_refs"`
	SourceRefLabels []string          `json:"source_ref_labels"`
	Freshness       string            `json:"freshness"`
	Privacy         string            `json:"privacy"`
	RawAccess       bool              `json:"raw_access"`
	Missing         string            `json:"missing"`
	Action          string            `json:"action"`
	Detail          string            `json:"detail"`
	MapKind         string            `json:"map_kind"`
	MapID           string            `json:"map_id"`
	MapSource       string            `json:"map_source"`
	MapTarget       string            `json:"map_target"`
	MapRelation     string            `json:"map_relation"`
	AIR             map[string]string `json:"air"`
}

type EpistemicsSummary struct {
	SchemaVersion string             `json:"schema_version"`
	Scope         string             `json:"scope"`
	AuthorityCase string             `json:"authority_case"`
	GeneratedAt   string             `json:"generated_at"`
	PackageHash   string             `json:"package_hash"`
	Sources       []EpistemicSource  `json:"sources"`
	Rows          []EpistemicReadRow `json:"rows"`
	Totals        map[string]int     `json:"totals"`
}

type Graph struct {
	MapID   string          `json:"map_id"`
	Thesis  string          `json:"thesis"`
	Layers  []Layer         `json:"layers"`
	Nodes   []Node          `json:"nodes"`
	Edges   []Edge          `json:"edges"`
	Package DynamicsPackage `json:"package"`
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
	return Graph{MapID: g.MapID, Thesis: g.Thesis, Layers: g.Layers, Nodes: nodes, Edges: edges, Package: g.Package}
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
		"      “…let reason hold the reins.”      — Benjamin Franklin",
		"",
		"PAGES",
		"  [Z] :coordinator     the Yard Coordinator — selection lens + steer-chat + cockpit (the landing page)",
		"  [1] :events          live coord event stream; [j/k] select, [y] yank event fields",
		"  [2] :tasks           task registry; [/] filter, [f] hint/select, [V] class-select",
		"  [3] :sessions        live lane roster; [j/k] select, [Enter] detail, [r] resume stub",
		"  [Y] :yard            Trainyard SDLC cockpit; ladder/attention/fleet/gates",
		"  [R] :readiness       gates/readiness projection; sources, lane blockers, route receipts",
		"  [I] :intake          source-backed intake observations; snapshots, buckets, gaps",
		"  [C] :capabilities    routing fit/admission projection; [j/k] select capability",
		"  [4] :dynamics [scale] system-dynamics map; [j/k] focus, [J/K] scroll, [E] epistemics",
		"                       scale = overview|domain|artifact|runtime|evidence|1..5|all",
		"  [A] :loops           A5 causal-loop analysis over :dynamics (computed, no simulation)",
		"  [E] :epistemics      evidence/provenance posture; [j/k] select derived rows",
		"  [U] :session         the chat-pane turn ladder — live turns, AIR-bimodal (skeleton airs, bodies deny)",
		"      :traces          LLM-observability — latency shape · cost text · model, per dispatch",
		"      :dispatch        the capability-dispatch ledger + measurement (cost/quality, honest gaps)",
		"  [5] :help            this page; [j/k] scroll when clipped",
		"  [6] :commands        unified command catalog; authority/preflight/receipt/UI delta",
		"  [7] :windows         lifecycle/window registry; every screen is jumpable/cycleable",
		"  [8] :intent          review-before-run pane; target/subject/preflight/receipt",
		"  [9] :surfaces        transient doors/modes registry; no buried modal lore",
		"  [0] :domains         domain/terrain lens registry; extensible SDLC/RDLC/n-DLC map",
		"  [L] :lifecycles      tenant lifecycle contracts; SDLC/RDLC/LDLC/n-DLC rows",
		"  [?] :legend          decode every glyph/color/cell; [g/G] top/bottom",
		"",
		"CASE-ROLE AXES   (the representational framework — :axes is a launcher to each pane)",
		"  :axes            the six axes + their five-tuple contracts; [enter] jumps to an axis's pane",
		"  :identity        A1 — who is acting (the live principal roster; the name denies on air)",
		"  :relational      A6 — who is affected (the consent / access-control posture)",
		"                   A2→:readiness · A4→:capabilities · A5→:dynamics · A3→:domains  (●live ◐part ○proj)",
		"",
		"COMMAND   ([:] opens the command line — Tab navigates, → fills, Enter accepts, Esc cancels)",
		"  :air on|off        the PII-safe on-air lens (default-deny redaction)",
		"  :note <text>       local note sink; free text is hidden while AIR is on",
		"  :quit              leave",
		"",
		"TASKS",
		"  [↵] inspect        open /whois — SDLC ladder + 7 dims + governed verb stubs",
		"  [Tab] field rank   [h/l] steer fields; [y] yanks the selected field",
		"  [y] yank           navigable grab mode: [j/k] rows, [Tab/←/→] fields, [Enter] grabs",
		"  [/] filter         id substring filter; completion offers visible ids",
		"  [f] hint/select    row letters, O/W/M/C class filters, 1/2 Act-jumps",
		"  [V] class-select   select visible siblings with the same criticality; [Esc] clears",
		"  [v] verbs         object-verb menu: the focused task's STATE-LEGAL governed verbs (mints nothing)",
		"",
		"EVENTS",
		"  [j/k]/[g/G]        move the event cursor",
		"  [y] yank           [t] time [K] kind [s] subject [a] actor [m] summary",
		"",
		"SESSIONS",
		"  [j/k]/[g/G]        move the session cursor",
		"  [y] yank           [r] role [p] platform [d] readiness [b] blocker [s] session [c] task",
		"  [Enter] detail     full-screen lane card; roster facts only",
		"  [r] resume-intent  governed route stub only; no transcript, PTY, or dispatch path",
		"",
		"REFERENCE PAGES",
		"  [j/k] scroll       [g/G] top/bottom; wide screens add a context/contract rail",
		"                       :capabilities is cursorful: [j/k] capability, [g/G] first/last",
		"  [←/→] cycle        next/previous registered window; [ and ] also work; title +N = hidden windows",
		"  [|] split context  session source left; active window becomes declared relation/context",
		"                       linked split: [j/k] source, [J/K] right context scroll",
		"                       source-only split: [j/k] lane anchor, [J/K] right context scroll; [Enter]/[y] source",
		"",
		"SPLIT PAIRS",
		"  linked             events, tasks, sessions, yard, readiness, intake, capabilities; [j/k] source updates context",
		"  source-only        dynamics, epistemics, intent, help, commands, windows, surfaces, domains, lifecycles, legend",
		"  panes              left is sessions source; right is active window relation/context; [J/K] scrolls right",
		"",
		"COMPOSITION",
		"  {{focus}}          inject the current row identity into a command",
		"  {{sel.*}}          inject field/value/status/family/receipt/missing refs; completion previews values",
		"  {{ring.0}}         replay the most recent AIR-safe yank without copy-paste",
		"  split context      session source plus relation context is a registered layout surface",
		"",
		"KEYS",
		"  global: [:] command  [1-9/0/Y/R/I/C/L/?] pages  [←/→] windows  [|] split  [a] AIR lens  [q] quit",
		"  rows:   [j/k] select  [g/G] top/bottom  [y] yank",
		"",
		"On AIR, every non-allowlisted cell renders ▒▒▒ — safe for the livestream.",
	}, "\n")
}

// pad clips/pads to exactly n RUNES (not bytes — multibyte glyphs like →/◀/✖ must not split).
// --- legend (the on-demand decoder for the whole grammar — never alienate, always situate) ---

// ordered so the legend is deterministic; the drift test asserts these cover the live glyph maps.
var critOrder = []string{"crit", "major", "warn", "ok"}
var provOrder = []string{"asserted", "observed", "inferred", "simulated", "rendered", "candidate"}

var critStateGloss = map[string]string{
	"crit": "critical — blocked / failed", "major": "major issue",
	"warn": "needs attention / in review", "ok": "healthy / on track",
}
var provGloss = map[string]string{
	"asserted": "stated as fact", "observed": "seen in telemetry", "inferred": "derived",
	"simulated": "modeled", "rendered": "a generated view", "candidate": "proposed / tentative",
}

// eventGlyphLegend decodes the EVENT-kind glyph alphabet (the leading glyph on :events rows) so a
// cold / on-air viewer can decode every mark Glyph() can emit — knowledge-in-the-world. Ordered for
// stable reading; covers every distinct value in the glyphs map plus the generic fallback. Several
// of these marks are intentionally shared with the criticality STATE alphabet (▸/✓/✖); the legend
// situates BOTH readings rather than hiding the overload. Drift-guarded by TestLegendCoversAllGlyphMaps.
var eventGlyphLegend = []struct{ glyph, gloss string }{
	{"▸", "in-progress · launch started · stage event"},
	{"✓", "succeeded"},
	{"✖", "failed (launch / review)"},
	{"⇡", "stage advanced (transition)"},
	{"⚑", "authorization changed (flag)"},
	{"◆", "task — claimed / closed"},
	{"↟", "PR merged"},
	{"⚙", "session ended"},
	{"·", "status · heartbeat"},
	{"✶", "other / unrecognized event"},
}

// DynamicsHeader situates the :dynamics graph for a viewer who cannot interact (the livestream
// audience): the map THESIS (macro reading before the micro) + an always-present inline provenance
// key so the node confidence-ladder decodes without focusing anything.
func DynamicsHeader(g Graph, w int) string {
	var b strings.Builder
	if g.Thesis != "" {
		th := g.Thesis
		if r := []rune(th); len(r) > w-3 {
			th = string(r[:w-4]) + "…"
		}
		b.WriteString(" " + C("2nd", th) + "\n")
	}
	key := " " + C("mut", "nodes  ")
	for _, k := range provOrder {
		key += C(palette.ProvToken(k), statusGlyphs[k]) + C("mut", " "+k+"   ")
	}
	b.WriteString(key + "\n")
	return b.String()
}

// RenderLegend decodes every mark in the grammar — glyph + plain-language gloss + a LIVE palette
// swatch — by iterating the SAME maps the renderers use, so it can never drift. The cure for
// unsituated novelty: a [?]/:legend keystroke answers "what am I looking at" from any page.
func RenderLegend() string {
	var b strings.Builder
	hd := func(s string) { b.WriteString("\n " + C("brt", s) + "\n") }
	row := func(swatch, gloss string) { b.WriteString("   " + swatch + "   " + C("mut", gloss) + "\n") }

	b.WriteString(" " + C("brt", "LEGEND") + C("mut", " — what the marks mean    open with [?] or :legend") + "\n")

	hd("STATE  (leading glyph, colored by criticality)")
	for _, k := range critOrder {
		row(C(SeverityToken(k), critGlyph[k]+" "+pad(k, 6)), critStateGloss[k])
	}
	hd("EVENTS  (leading glyph by kind — :events rows)")
	{ // compact, wrapped: keeps the core legend one-screen-with-slack; class-closure guarded.
		var parts []string
		for _, e := range eventGlyphLegend {
			parts = append(parts, e.glyph+" "+legendShort(e.gloss))
		}
		for _, ln := range wrapMarks(parts, 10) {
			b.WriteString("   " + C("mut", ln) + "\n")
		}
	}
	hd("SESSION TURNS  (turn-kind glyph — session-pane rows)")
	{
		var parts []string
		for _, k := range turnKindOrder {
			parts = append(parts, turnGlyph[k]+" "+legendShort(turnKindGloss[k]))
		}
		for _, ln := range wrapMarks(parts, 8) {
			b.WriteString("   " + C("mut", ln) + "\n")
		}
	}
	row(C("grn", "live")+C("mut", " / ")+C("yel", "stale")+C("mut", " / demo fixture"), "chat pane names its feed source honestly (streaming lane / last-live page / no live data)")
	hd("CRITICALITY BAR  (more fill = worse)")
	for _, k := range critOrder {
		row(C(SeverityToken(k), critBar(k)), k)
	}
	hd("FRESHNESS  (brightness = recency)")
	fb := func(f float64) string { g, tk := freshGlyph(f); return C(tk, g) }
	row(fb(0.9)+fb(0.5)+fb(0.1), "▇ recent … ▁ stale  (age since the task's last event)")
	hd("TRAJECTORY  (◀ where it was · → where it's going)")
	row(C("mut", "S6◀"), "the stage it came FROM (prior)")
	row(C("grn", "→S7"), "pending transition to the next stage")
	row(C("red", "→hold"), "blocked — release-gated, will not advance")
	row(C("grn", "·ship"), "terminal / arrived (no pending move)")
	hd("RELATIONS")
	row(C("blu", "●N"), "N live ties (deps / governance / tied)")
	row(C("2nd", "├"), "brushed row: related to the focused row's emergent relation")
	hd("LAYOUT  (pane relationship and scroll grammar)")
	row(C("yel", "split:ctx"), "split is active; left source and right context are paired")
	row(C("mut", "split:wide"), "wide contextual rail is active without split source")
	row(C("yel", "▶"), "active source row; navigation rebinds the context")
	row(C("pri", "◆"), "independent source row; right context does not rebind")
	row(C("brt", "▣"), "file staged in the injection basket (files-zone gutter; distinct from ✓ ok)")
	row(C("2nd", "▤"), "attachment chip / attached turn payload")
	row(C("2nd", "» / «»"), "path breadcrumb marker (path text may redact on air)")
	row(C("border", "│"), "pane divider; panes should explain each other")
	row(C("mut", "… N more"), "overflow rows exist below the current viewport")
	hd("CASE-ROLE AXES  (:axes · A1 :identity · A6 :relational)")
	row(C("grn", "●")+C("yel", "◐")+C("mut", "○"), "axis build status: ● live (folds) · ◐ partial · ○ projection-pending (badged)")
	row(C("grn", "▰")+C("2nd", "◇")+C("yel", "◆")+C("pri", "◈"), "A1 identity class: ▰ lane · ◇ actor · ◆ owner · ◈ mixed")
	row(C("brt", "◹")+C("2nd", "✎")+C("yel", "⊟")+C("pri", "⚖"), "A6 consent facet: ◹ broadcast-frame · ✎ authorship · ⊟ field-gating · ⚖ stakeholders")
	hd("PROVENANCE  (:dynamics nodes — a confidence ladder)")
	for _, k := range provOrder {
		row(C(palette.ProvToken(k), statusGlyphs[k]+" "+pad(k, 9)), provGloss[k])
	}
	hd("TRACES  (LLM spend — latency shape · cost text · neither is a hue)")
	row(latencyHistogram(1200), "█▆▄▁ latency magnitude (mut; NOT criticality — magnitude by SHAPE)")
	row(C("mut", "$0.012345"), "USD cost at 6dp (mut; NOT ownership — a literal, not a family)")
	hd("COLOR  (three independent channels — each one meaning)")
	row(C("grn", "█")+C("yel", "█")+C("org", "█")+C("red", "█"), "hue = CRITICALITY   (ok → crit)")
	row(C("brt", "█")+C("pri", "█")+C("mut", "█"), "brightness = FRESHNESS   (recent → stale)")
	row(C("blu", "█")+C("fch", "█")+C("eme", "█")+C("2nd", "█"), "family = OWNERSHIP   (cc · gov · alpha · other)")
	b.WriteString("\n " + C("mut", "gray is ground; color blooms only where it means something.") + "\n")
	return b.String()
}

func pad(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return string(r[:n])
	}
	return s + strings.Repeat(" ", n-len(r))
}

// legendShort: the compact label for a dense legend line — the gloss up to its first qualifier
// (so a one-screen legend can list a whole glyph alphabet without one row per mark).
func legendShort(gloss string) string {
	for _, sep := range []string{" (", " ·", " —", " /"} {
		if i := strings.Index(gloss, sep); i > 0 {
			return gloss[:i]
		}
	}
	return gloss
}

// wrapMarks: pack "glyph label" parts into at most n-per-line for a compact, scannable decoder block.
func wrapMarks(parts []string, n int) []string {
	var out []string
	for i := 0; i < len(parts); i += n {
		end := i + n
		if end > len(parts) {
			end = len(parts)
		}
		out = append(out, strings.Join(parts[i:end], "  "))
	}
	return out
}
