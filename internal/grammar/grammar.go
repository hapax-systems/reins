package grammar

import (
	"fmt"
	"strings"
)

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
	return fmt.Sprintf("%s %s%s %s  %s", compactTS(ev.TS), ScoreBar(ev.Score), Glyph(ev.Kind), subj, summ)
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

func pad(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}
