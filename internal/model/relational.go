package model

import (
	"fmt"

	"github.com/hapax-systems/reins/internal/config"
	"github.com/hapax-systems/reins/internal/grammar"
)

// consentFacets folds the A6 Relational pane: the four consent-posture facets — the broadcast FRAME
// (the operator's standing consent decision), AUTHORSHIP (the provenance ●◐◌○ distribution over the
// turn ladder — who wrote what), field GATING (the AIR policy — what default-denies), and STAKEHOLDERS
// (the ∅/unilateral-authority flag). Air-safe by construction: every line is a count / policy / glyph
// / frame-state — never a protected VALUE (the pane governs PII; it must not leak it). A6 is
// projection-pending: the consent ledger is not landed, so this is the access-control POSTURE.
func (m Model) consentFacets() []grammar.ConsentFacet {
	cur := "on-air (broadcast — default-deny)"
	if !m.AIR {
		cur = "present-at-hand (operator — cleartext)"
	}

	op, md, st, un, other := 0, 0, 0, 0, 0
	for _, t := range m.TurnLadder {
		switch t.Prov {
		case "operator":
			op++
		case "model":
			md++
		case "structured":
			st++
		case "untrusted":
			un++
		default:
			other++ // non-canonical / empty provenance — ACCOUNTED (never silently dropped)
		}
	}

	allow := len(config.Defaults().AIRAllowlist)
	n := len(m.identityRoster())
	authSummary := fmt.Sprintf("●%d ◐%d ◌%d ○%d", op, md, st, un)
	if other > 0 {
		authSummary += fmt.Sprintf(" ?%d", other) // unattributed bucket — totals always sum to len(ladder)
	}
	if len(m.TurnLadder) == 0 {
		authSummary = "no turns in the ladder (load :session)"
	}
	stakeFlag := "consent ledger NOT landed → no per-stakeholder consented/withheld/deferred states yet"
	dangerFlag := fmt.Sprintf("%d distinct principals act across the fleet (the WHO is the :identity roster)", n)
	if n == 0 {
		dangerFlag = "∅ NO principals — UNILATERAL authority (dangerous: no one to consent or contest)"
	}

	authLines := []string{
		fmt.Sprintf("● operator %d — free-text, ALWAYS denied on air", op),
		fmt.Sprintf("◐ model %d — model-authored, airs per field", md),
		fmt.Sprintf("◌ structured %d — registry/hardened, may air", st),
		fmt.Sprintf("○ untrusted %d — MCP/web, shape-only on air", un),
	}
	if other > 0 {
		authLines = append(authLines, fmt.Sprintf("? unattributed %d — non-canonical/empty provenance (accounted, never dropped)", other))
	}
	authLines = append(authLines, "the provenance glyph IS the consent-evidence channel (who authored each block)")

	return []grammar.ConsentFacet{
		{Key: "frame", Name: "broadcast frame", Summary: cur, Lines: []string{
			"current: " + cur,
			"present-at-hand ⟵ operator only · cleartext, all data",
			"on-air ⟵ broadcast · default-deny + N-sec hold + dump/kill (hold/kill NOT wired yet)",
			"the toggle is the operator's standing consent decision for the stream",
		}},
		{Key: "authorship", Name: "authorship", Summary: authSummary, Lines: authLines},
		{Key: "gating", Name: "field gating", Summary: fmt.Sprintf("%d air · rest deny", allow), Lines: []string{
			fmt.Sprintf("%d fields air by default (the structural skeleton)", allow),
			"every OTHER field default-denies on air (consent-protected)",
			"a value airs only if its per-item AIR map marks it ok — one redact() source, no drift",
		}},
		{Key: "stakeholders", Name: "stakeholders", Summary: fmt.Sprintf("%d principals", n), Lines: []string{
			dangerFlag,
			stakeFlag,
			"who is AFFECTED (not who acts) needs the consent ledger — projection-pending",
		}},
	}
}
