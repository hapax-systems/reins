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

// renderObserve — E11.7 whole-system awareness, LIVE from /read/observe (read-only projection; never mints
// authority). Per-dimension honest-dark (a dimension whose source is unreachable shows ·dark, no fabricated
// count). AIR: whole-system state is operator-private → the VALUES (summary/count) SEAL on air while the
// dimension KEY + live/dark STATUS air (structural skeleton). Honest-dark when the whole endpoint is down.
func (m Model) renderObserve(w int) string {
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	b.WriteString(" " + grammar.C("brt", "OBSERVE") + grammar.C("mut", "  whole-system awareness — read-only projection, never minting authority") + "\n")
	b.WriteString(" " + rule + "\n")
	if m.ObserveDark || len(m.Observe) == 0 {
		b.WriteString(" " + grammar.C("mut", "(observe dark — the cockpit read endpoint is unreachable; no fabricated state)"))
		return strings.TrimRight(b.String(), "\n")
	}
	for _, dim := range m.Observe {
		g, tok := "·", "mut"
		if dim.Status == "live" {
			g, tok = "●", "grn"
		}
		val := grammar.C("mut", "▒▒▒")
		if !m.AIR {
			summary := dim.Summary
			if dim.Count != nil {
				summary = fmt.Sprintf("%s (%d)", strings.TrimSpace(summary), *dim.Count)
			}
			if strings.TrimSpace(summary) == "" {
				summary = "—"
			}
			val = grammar.C("mut", clipRunes(summary, maxVisible(8, w-20)))
		}
		b.WriteString(" " + grammar.C(tok, g) + " " + grammar.C("2nd", fmt.Sprintf("%-12s", dim.Key)) + " " + val + "\n")
	}
	if m.AIR {
		b.WriteString(" " + rule + "\n")
		b.WriteString(" " + grammar.C("mut", "▒ values sealed on air — whole-system state is operator-private (status airs, values do not)"))
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderVault — E11.5b Obsidian research/planning navigation, LIVE from /read/vault (titles + obsidian://
// links; bodies default-deny, never fetched). AIR: the vault is operator-private life-planning (LDLC
// air_class "private-life") → the list SEALS on air (the count airs, the titles do not). Honest-dark when
// the endpoint is unreachable / vault_root unset.
func (m Model) renderVault(w int) string {
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	b.WriteString(" " + grammar.C("brt", "VAULT") + grammar.C("mut", "  Obsidian research/planning — titles + obsidian:// links (bodies default-deny)") + "\n")
	b.WriteString(" " + rule + "\n")
	if m.AIR {
		b.WriteString(" " + grammar.C("mut", fmt.Sprintf("▒▒▒ vault SEALED on air — %d notes (operator-private life-planning, not for the wire)", len(m.Vault))))
		return strings.TrimRight(b.String(), "\n")
	}
	if m.VaultDark || len(m.Vault) == 0 {
		b.WriteString(" " + grammar.C("mut", "(vault dark — set vault_root / REINS_VAULT_ROOT in config; metadata only, bodies never read)"))
		return strings.TrimRight(b.String(), "\n")
	}
	for _, n := range m.Vault {
		folder := n.Folder
		if folder == "" {
			folder = "·"
		}
		row := fmt.Sprintf("%-16s %s", clipRunes(folder, 16), n.Title)
		b.WriteString(" " + grammar.C("2nd", "▸ ") + grammar.C("mut", clipRunes(row, maxVisible(10, w-4))) + "\n")
	}
	b.WriteString(" " + rule + "\n")
	b.WriteString(" " + grammar.C("mut", fmt.Sprintf("%d notes · obsidian:// deep-links · bodies default-deny ([J] scrolls)", len(m.Vault))))
	return strings.TrimRight(b.String(), "\n")
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
