package model

import (
	"fmt"
	"strings"

	"github.com/hapax-systems/reins/internal/grammar"
)

// scaffoldRow is one dimension of a parity-floor surface with its honest readiness.
type scaffoldRow struct {
	label  string
	status string // "live" | "pending" | "dark"
	detail string
}

func scaffoldStatusGlyph(status string) (glyph, tok string) {
	switch status {
	case "live":
		return "●", "grn"
	case "pending":
		return "◌", "yel"
	default: // dark
		return "·", "mut"
	}
}

// renderScaffoldPage is the honest-dark-with-dignity surface (E11 parity floor): a consolidated page that
// EXISTS and declares WHAT it will surface and its honest readiness, WITHOUT fabricating data. The wide
// space is the context rail (each dimension + why dark + what lands it) per the negative-space rule —
// never decorative filler, never a fake-actionable control.
func renderScaffoldPage(w int, title, tagline string, rows []scaffoldRow, note string) string {
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	b.WriteString(" " + grammar.C("brt", title) + grammar.C("mut", "  "+tagline) + "\n")
	b.WriteString(" " + rule + "\n")
	for _, r := range rows {
		g, tok := scaffoldStatusGlyph(r.status)
		b.WriteString(" " + grammar.C(tok, g) + " " + grammar.C("2nd", fmt.Sprintf("%-14s", r.label)) + " " + grammar.C("mut", r.detail) + "\n")
	}
	if strings.TrimSpace(note) != "" {
		b.WriteString(" " + rule + "\n")
		b.WriteString(fitWidth(" "+grammar.C("mut", note), w))
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderObserve — E11.7 whole-system awareness (read-only projection; never mints authority).
func (m Model) renderObserve(w int) string {
	return renderScaffoldPage(w, "OBSERVE", "whole-system awareness — read-only projection, never minting authority",
		[]scaffoldRow{
			{"health", "pending", "service/unit health roll-up (cockpit /health)"},
			{"drift", "pending", "config/state drift items"},
			{"nudges", "pending", "actionable nudges + dismissals"},
			{"agents", "pending", "fleet roster + lane state (→ :yard)"},
			{"governance", "pending", "gate stack + outcomes (→ :govern)"},
			{"consent", "pending", "HKP consent/egress posture (→ :relational)"},
			{"profile", "pending", "operator profile dimensions"},
			{"cost", "pending", "per-route spend (→ :dispatch economics)"},
			{"gpu", "pending", "dual-rig VRAM / utilization"},
			{"stimmung", "pending", "ambient operator stance"},
		},
		"honest-dark: aggregated from the cockpit read endpoint (localhost:8051) — the read-projection wire is pending. No shadow model, no fabricated state.")
}

// renderVault — E11.5b Obsidian research/planning navigation (titles + obsidian:// links; bodies deny).
func (m Model) renderVault(w int) string {
	return renderScaffoldPage(w, "VAULT", "Obsidian research/planning navigation — metadata + obsidian:// links",
		[]scaffoldRow{
			{"notes", "pending", "research/planning note titles + obsidian:// open"},
			{"observations", "pending", "intake research observations (→ :intake)"},
			{"backlinks", "dark", "backlink graph (deferred per spec)"},
			{"search", "dark", "full-text body search (deferred — bodies default-deny)"},
		},
		"honest-dark: the vault read endpoint is pending. Bodies stay default-deny (AIR); this surfaces titles + obsidian:// deep-links + research-observation metadata, never raw note bodies.")
}

// renderRdlc — E11.4 Research Development Lifecycle (Labrack) — honest-DARK by design (no fabricated cockpit).
func (m Model) renderRdlc(w int) string {
	return renderScaffoldPage(w, "RDLC · CLAIMS", "Research Development Lifecycle (Labrack) — honest-DARK by design",
		[]scaffoldRow{
			{"claims", "dark", "research claims + status"},
			{"observations", "dark", "evidence observations"},
			{"validation", "dark", "validation verdicts"},
			{"evidence", "dark", "provenance + sources"},
		},
		"honest-dark: the RDLC model is not yet defined — no fabricated claim cockpit is shown. When the RDLC substrate exists this surfaces claims/observations/validation/evidence at the SDLC parity floor.")
}

// renderDeck — E8.3 the DECK: the non-evicting operator-readout history (no-loss), vs the windowed event
// STREAM. AIR-safe: the deck holds rendered readouts captured OFF-air (possibly cleartext), so on air it
// SEALS — the count airs, the content does not (an operator-private history must not replay on the wire).
func (m Model) renderDeck(w int) string {
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	b.WriteString(" " + grammar.C("brt", "DECK") + grammar.C("mut", "  operator-readout history — no-loss (survives the event STREAM window)") + "\n")
	b.WriteString(" " + rule + "\n")
	if m.AIR {
		b.WriteString(" " + grammar.C("mut", fmt.Sprintf("▒▒▒ deck SEALED on air — %d readouts (operator-private history, not for the wire)", len(m.Deck))))
		return strings.TrimRight(b.String(), "\n")
	}
	if len(m.Deck) == 0 {
		b.WriteString(" " + grammar.C("mut", "(no readouts yet — operator-facing notices accumulate here, newest last)"))
		return strings.TrimRight(b.String(), "\n")
	}
	for _, r := range m.Deck {
		b.WriteString(" " + grammar.C("2nd", "▸ ") + grammar.C("mut", clipRunes(r, maxVisible(10, w-4))) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderPresence — E11.8 presence-plane binder (figure/control vs ground/presence) — honest-dark pending agy.
func (m Model) renderPresence(w int) string {
	return renderScaffoldPage(w, "PRESENCE", "Section / Figure-Ground / Concourse — the presence-plane binder",
		[]scaffoldRow{
			{"figure", "dark", "operator-acted control surface (foreground)"},
			{"ground", "dark", "ambient presence / state (background)"},
			{"concourse", "dark", "shared presence-plane binder"},
		},
		"honest-dark-with-dignity: the presence-plane design (agy) is pending. This separates figure/control (what the operator acts on) from ground/presence (ambient state), rendered honestly empty until the design lands.")
}
