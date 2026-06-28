package grammar

import (
	"fmt"
	"strings"
)

// A1 IDENTITY (the case-role "who is acting"). The identity pane folds a deduped ROSTER of the
// distinct principals across the fleet — session roles (lanes), event actors, task owners — each with
// its class + where it appears. AIR (HARD): a principal's NAME is identity = sensitive → deny on air;
// the CLASS + appearance COUNTS are the structural skeleton (anonymous activity shape) and survive —
// the bimodal pattern (who is denied, that-there-is-activity is shown). A1 is PROJECTION-PENDING:
// derived from role/actor/owner fields, NOT a dedicated identity registry — the pane badges that.
type Identity struct {
	Name     string // role / actor / owner — SENSITIVE, redacts on air
	Class    string // "lane" | "actor" | "owner" | "mixed"
	Sessions int    // appearances as a session role
	Events   int    // appearances as an event actor
	Tasks    int    // appearances as a task owner
}

func identityClassGlyph(class string) string {
	switch class {
	case "lane":
		return C("grn", "▰")
	case "actor":
		return C("2nd", "◇")
	case "owner":
		return C("yel", "◆")
	default:
		return C("pri", "◈") // mixed — appears in more than one role
	}
}

// RenderIdentityHeader situates the roster columns.
func RenderIdentityHeader() string {
	return C("mut", fmt.Sprintf(" %-1s %-22s %-7s %s", "·", "PRINCIPAL", "CLASS", "s·e·t (sessions·events·tasks)"))
}

// RenderIdentityRow is one roster row. The NAME redacts on air (identity is sensitive); the class
// glyph + class word + the s·e·t appearance counts are the structural skeleton and survive on air.
func RenderIdentityRow(id Identity, airOn bool, w int) string {
	if w < 24 {
		w = 24
	}
	name := Redact(nil, "label", id.Name, airOn) // who → ▒▒▒ on air (default-deny)
	counts := C("mut", fmt.Sprintf("s%d·e%d·t%d", id.Sessions, id.Events, id.Tasks))
	row := fmt.Sprintf(" %s %-22s %-7s %s",
		identityClassGlyph(id.Class), C("pri", clipRunes(name, 22)), C("2nd", id.Class), counts)
	return clipRunes(row, w)
}

// RenderIdentityDetail renders the focused principal: its class + appearance breakdown (name redacts
// on air) + the A1 five-tuple contract reminder (so the pane situates itself as the Identity axis).
func RenderIdentityDetail(id Identity, airOn bool, w int) string {
	if w < 28 {
		w = 28
	}
	bw := w - 2
	if bw < 10 {
		bw = 10
	}
	var b strings.Builder
	name := Redact(nil, "label", id.Name, airOn)
	b.WriteString(" " + identityClassGlyph(id.Class) + " " + C("brt", clipRunes(name, w-6)) + C("mut", "  · "+id.Class) + "\n")
	b.WriteString(" " + C("border", strings.Repeat("─", bw)) + "\n")
	field := func(label, val string) {
		b.WriteString(" " + C("mut", fmt.Sprintf("%-12s", label)) + wrapInto(val, w-14, 14) + "\n")
	}
	field("appears as", fmt.Sprintf("%d session role(s) · %d event actor(s) · %d task owner(s)", id.Sessions, id.Events, id.Tasks))
	a1 := Axes()[0] // the A1 contract — the same five-tuple the :axes pane shows, situated here
	field("question", a1.Question)
	field("controls", a1.Controls)
	field("blind-spot", a1.BlindSpot)
	b.WriteString(" " + C("border", strings.Repeat("─", bw)) + "\n")
	b.WriteString(" " + axisStatusGlyph(a1.Status) + C("mut", " A1 Identity is "+axisStatusWord(a1.Status)+" — a roster derived from role/actor/owner fields, not a dedicated identity registry") + "\n")
	return strings.TrimRight(b.String(), "\n")
}
