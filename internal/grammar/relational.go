package grammar

import (
	"fmt"
	"strings"
)

// A6 RELATIONAL/CONSENT (the case-role "who is affected / what consent + accountability lines exist").
// The pane is the LENS ON CONSENT ITSELF — it shows the access-control POSTURE, not the principals
// (that is A1). Four consent facets: the broadcast FRAME (the operator's consent toggle), AUTHORSHIP
// (who wrote what — the provenance ●◐◌○ ladder), field GATING (the AIR policy — what default-denies),
// and STAKEHOLDERS (the ∅/unilateral-authority flag). AIR: the pane shows POLICY + COUNTS + GLYPHS +
// the frame state — never a protected VALUE — so it is air-safe by construction (it governs PII, it
// must not leak it). A6 is PROJECTION-PENDING: the consent ledger is not landed; this is the posture.
type ConsentFacet struct {
	Key     string   // "frame" | "authorship" | "gating" | "stakeholders"
	Name    string   // the facet label
	Summary string   // the one-line list summary (air-safe)
	Lines   []string // the detail lines (air-safe — counts/policy/glyphs, never a PII value)
}

func consentFacetGlyph(key string) string {
	switch key {
	case "frame":
		return C("brt", "◹") // the broadcast frame
	case "authorship":
		return C("2nd", "✎") // who wrote
	case "gating":
		return C("yel", "⊟") // field gating
	default:
		return C("pri", "⚖") // stakeholders / accountability
	}
}

// RenderConsentFacetHeader situates the consent-posture columns.
func RenderConsentFacetHeader() string {
	return C("mut", fmt.Sprintf(" %-1s %-16s %s", "·", "CONSENT FACET", "POSTURE"))
}

// RenderConsentFacetRow is one facet row: glyph · name · the air-safe summary.
func RenderConsentFacetRow(f ConsentFacet, w int) string {
	if w < 24 {
		w = 24
	}
	row := fmt.Sprintf(" %s %-16s %s", consentFacetGlyph(f.Key), C("pri", f.Name), C("2nd", f.Summary))
	return clipRunes(row, w)
}

// RenderConsentFacetDetail renders the focused facet's air-safe breakdown + the A6 five-tuple contract
// reminder + the projection-pending badge (the consent ledger is not landed).
func RenderConsentFacetDetail(f ConsentFacet, w int) string {
	if w < 28 {
		w = 28
	}
	bw := w - 2
	if bw < 10 {
		bw = 10
	}
	var b strings.Builder
	b.WriteString(" " + consentFacetGlyph(f.Key) + " " + C("brt", f.Name) + "\n")
	b.WriteString(" " + C("border", strings.Repeat("─", bw)) + "\n")
	for _, ln := range f.Lines {
		b.WriteString(" " + wrapInto(ln, w-2, 2) + "\n")
	}
	a6 := Axes()[5] // the A6 contract — the same five-tuple :axes shows, situated here
	b.WriteString(" " + C("border", strings.Repeat("─", bw)) + "\n")
	field := func(label, val string) {
		b.WriteString(" " + C("mut", fmt.Sprintf("%-11s", label)) + wrapInto(val, w-13, 13) + "\n")
	}
	field("controls", a6.Controls)
	field("blind-spot", a6.BlindSpot)
	b.WriteString(" " + axisStatusGlyph(a6.Status) + C("mut", " A6 Relational is "+axisStatusWord(a6.Status)+" — the consent ledger is not landed; this is the access-control POSTURE, not a stakeholder roster") + "\n")
	return strings.TrimRight(b.String(), "\n")
}
