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

// Glyph: the closed, learned alphabet — the cell carries the kind (monochrome-safe).
var glyphs = map[string]string{
	"pr.merged": "↟", "task.closed": "◆", "session.ended": "⚙",
	"review.fail": "✖", "stage": "▸", "status": "·",
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

// air-or-redact: allowlisted field passes; denied field becomes a fixed-width redaction token.
func air(ev Event, field, val string, on bool) string {
	if on && ev.AIR[field] != "ok" {
		return "▒▒▒"
	}
	return val
}

// RenderEventRow: one row of the grammar. `airOn` toggles the AIR lens (default-deny redaction).
// Format (formats.toml row-kind "event"): TS │ scorebar glyph │ subject(6) │ summary
func RenderEventRow(ev Event, airOn bool) string {
	subj := air(ev, "subject", pad(ev.Subject, 6), airOn)
	summ := air(ev, "summary", ev.Summary, airOn)
	return fmt.Sprintf("%-5s %s%s %s  %s", ev.TS, ScoreBar(ev.Score), Glyph(ev.Kind), subj, summ)
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}
