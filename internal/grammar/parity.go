package grammar

import "strings"

// ParityKind is how a native-session interactive verb is honored in reins.
type ParityKind int

const (
	ParityReins       ParityKind = iota // a native reins verb/surface covers it
	ParityPassthrough                   // forwarded verbatim to the backend session (reins is the conduit)
	ParityNA                            // structurally N/A — excised by the direction-only doctrine, not missing
	ParityGap                           // UNCOVERED lossless hole — flagged, never hidden
)

func (k ParityKind) Label() string {
	switch k {
	case ParityReins:
		return "reins"
	case ParityPassthrough:
		return "pass"
	case ParityNA:
		return "N/A"
	default:
		return "GAP"
	}
}

// ParityRow maps one native interactive capability to its reins projection.
type ParityRow struct {
	Native string     // the native-session verb (session-pane design §6)
	Kind   ParityKind // how reins honors it
	Reins  string     // the reins verb/surface, or the N/A reason, or the gap description
}

// ChatParityManifest is the E4.7 chat-UX-bar SSOT: the native interactive verb set (session-pane design
// §6) × its reins projection. The bar is "≥ native given the in-reins capability layers" — so most verbs
// map to a richer reins surface; the direction-only doctrine makes lane capability-selection an explicit
// N/A (the operator steers DIRECTION, routing picks capability); the terminal forces ONE honest GAP
// (no inline image render). Hand-authored; the gate is ParityGaps + the never-silent-coverage test.
func ChatParityManifest() []ParityRow {
	return []ParityRow{
		{"send-prompt", ParityReins, "coord chat — [:] compose, {{sel}}/{{ring}} injection"},
		{"/clear", ParityReins, "/clear"},
		{"/compact", ParityReins, "/compact + the continuity substrate (lossless distill)"},
		{"/help", ParityReins, ":help · :legend (progressive disclosure)"},
		{"slash-commands", ParityReins, ":commands catalog (legal verbs + templates)"},
		{"custom-skills", ParityReins, ":commands — skill/template grammar"},
		{"plan-mode", ParityReins, "impinge graduated approval (present-at-hand)"},
		{"auto-accept-mode", ParityReins, "approval policy on the impinge gate"},
		{"tool-approval", ParityReins, "impinge accept / deny / edit (out-of-band)"},
		{"interrupt", ParityReins, "impinge interrupt — out-of-band cancel"},
		{"attach-resume", ParityReins, "session resume (role-keyed event log)"},
		{"paste-files", ParityReins, "injection basket (path refs, AIR-audited)"},
		{"scroll-history", ParityReins, "turn ladder scroll + /lastlog"},
		{"token-cost-readout", ParityReins, "turn tokens + :traces / :dispatch economics"},
		{"context-readout", ParityReins, "compressed-FSM context view + lenses"},
		{"MCP-tool-list", ParityReins, ":capabilities — harnessed/routed/measured state"},
		{"coordinator-swap", ParityReins, "context-preserving capability HOT-SWAP (canonical Block union)"},
		// excised by the direction-only doctrine — N/A, not missing:
		{"model-switch", ParityNA, "lane capability is ROUTING-decided; the operator steers DIRECTION, not capability"},
		// terminal constraint — the one honest lossless hole:
		{"paste-images", ParityGap, "no inline image render (terminal); basket carries the path ref only"},
	}
}

// ParityGaps returns only the UNCOVERED rows — the lossless holes the chat-UX-bar must own openly.
func ParityGaps(rows []ParityRow) []ParityRow {
	var out []ParityRow
	for _, r := range rows {
		if r.Kind == ParityGap {
			out = append(out, r)
		}
	}
	return out
}

// RenderChatParity is the scan — native verb · kind · reins projection, grouped so GAPs are conspicuous
// (never folded into a green wash). Raw mapping, no scalar "% parity" (that would re-collapse the holes).
func RenderChatParity(rows []ParityRow, w int) string {
	rule := strings.Repeat("─", clampW(w-2, 10, 120))
	var b strings.Builder
	b.WriteString(" " + C("brt", "chat-UX parity") + C("mut", "   native verb → reins projection (≥ native; holes shown)") + "\n")
	b.WriteString(" " + C("border", rule) + "\n")
	tok := map[ParityKind]string{ParityReins: "grn", ParityPassthrough: "blu", ParityNA: "mut", ParityGap: "red"}
	emit := func(only ParityKind) {
		for _, r := range rows {
			if r.Kind != only {
				continue
			}
			b.WriteString("  " + C(tok[r.Kind], pad(r.Kind.Label(), 5)) + " " + C("2nd", pad(r.Native, 18)) + " " + C("mut", r.Reins) + "\n")
		}
	}
	emit(ParityReins)
	emit(ParityPassthrough)
	emit(ParityNA)
	if len(ParityGaps(rows)) > 0 {
		b.WriteString(" " + C("red", "GAP") + C("mut", "  lossless holes — owned, not hidden") + "\n")
		emit(ParityGap)
	}
	return strings.TrimRight(b.String(), "\n")
}
