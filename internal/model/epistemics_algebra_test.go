package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// Inc 3 TRANSFORM — PageEpistemics migrates onto the view-algebra as a SELF-ANCHORED page (like tasks/
// events), ABOLISHING the legacy session-frozen split-pair (sessions │ evidence). The primary IS the
// epistemic posture list; j moves the epistemic row (EpiFocus) NATIVELY; the secondary is the focused
// row's evidence path; the pane-to-pane connector is EMERGENT, never an authored split-pair.

func epistemicsAlgebraModel() Model {
	ep := grammar.EpistemicsSummary{Rows: []grammar.EpistemicReadRow{
		{RowID: "r1", Family: "claim", Subject: "authority-ceiling", Status: "asserted", Authority: "platform",
			EvidenceCount: 2, Source: "dynamics", Freshness: "<1h", Privacy: "metadata-only",
			AIR: map[string]string{"row_id": "ok", "family": "ok", "subject": "ok", "status": "ok", "authority": "ok", "evidence_count": "ok", "source": "ok", "freshness": "ok", "privacy": "ok"}},
		{RowID: "r2", Family: "observation", Subject: "session-pressure", Status: "observed", Authority: "world",
			EvidenceCount: 1, Source: "intake", Freshness: "<1h", Privacy: "metadata-only",
			AIR: map[string]string{"row_id": "ok", "family": "ok", "subject": "ok", "status": "ok", "authority": "ok", "evidence_count": "ok", "source": "ok", "freshness": "ok", "privacy": "ok"}},
	}}
	m := New("REINS").FoldEpistemics(ep, false)
	m.Width, m.Height, m.Page = 180, 44, PageEpistemics
	return m
}

// The page composes through the algebra and binds templates/yank to ITS OWN focus (not a session source).
func TestEpistemicsComposesViaAlgebraNativeBinding(t *testing.T) {
	m := epistemicsAlgebraModel()
	if !m.composesViaAlgebra() {
		t.Fatal("PageEpistemics must compose via the view-algebra (only-split)")
	}
	if m.splitContextActive() {
		t.Fatal("a migrated page must NOT be session-frozen (splitContextActive==false)")
	}
	if m.commandSelectionPage() != PageEpistemics {
		t.Fatalf("templates/yank must bind to PageEpistemics, got page %d", m.commandSelectionPage())
	}
}

// j moves the EPISTEMIC row natively (the abolished session-frozen split made j move the session lane).
func TestEpistemicsJMovesEvidenceRowNatively(t *testing.T) {
	m := epistemicsAlgebraModel()
	before := m.EpiFocus
	m = step(m, "j")
	if m.EpiFocus != before+1 {
		t.Fatalf("j must move the epistemic row natively (EpiFocus %d→%d)", before, m.EpiFocus)
	}
}

// {{focus}} resolves to the focused epistemic subject — the native binding the migration unblocks.
func TestEpistemicsFocusTemplateBindsToRow(t *testing.T) {
	m := epistemicsAlgebraModel()
	got := m.resolveTemplate("note {{focus}}")
	if !strings.Contains(got, "authority-ceiling") {
		t.Fatalf("{{focus}} must resolve to the focused epistemic subject, got %q", got)
	}
	if strings.Contains(got, "{{focus}}") {
		t.Fatalf("{{focus}} must not render literally, got %q", got)
	}
	// after moving, {{focus}} tracks the new row
	m = step(m, "j")
	if got := m.resolveTemplate("{{focus}}"); !strings.Contains(got, "session-pressure") {
		t.Fatalf("{{focus}} must track the moved row, got %q", got)
	}
}

// On air, {{focus}} over a DENIED subject must redact (the rows are pre-projected with AIR, so the
// resolver returns the already-redacted value — never the private subject).
func TestEpistemicsFocusTemplateIsAirSafe(t *testing.T) {
	ep := grammar.EpistemicsSummary{Rows: []grammar.EpistemicReadRow{
		{RowID: "r1", Family: "claim", Subject: "PRIVATE-SUBJECT", Status: "asserted", Authority: "platform",
			AIR: map[string]string{"row_id": "ok", "family": "ok", "subject": "deny", "status": "ok", "authority": "ok"}},
	}}
	m := New("REINS").FoldEpistemics(ep, false)
	m.Width, m.Height, m.Page = 180, 44, PageEpistemics
	m.AIR = true
	if got := m.resolveTemplate("{{focus}}"); strings.Contains(got, "PRIVATE-SUBJECT") {
		t.Fatalf("on air {{focus}} must not leak a denied subject, got %q", got)
	}
}

// The wide View renders the algebra split (a secondary evidence path), NOT the legacy session-frozen
// split (no "split sessions" source pane, no session-lane nav hint).
func TestEpistemicsViewIsAlgebraSplitNotSessionFrozen(t *testing.T) {
	m := epistemicsAlgebraModel()
	v := ansi.Strip(m.View())
	if strings.Contains(v, "split sessions") || strings.Contains(v, "[j/k]source") {
		t.Fatalf("migrated epistemics must NOT render the legacy session-frozen split:\n%s", v)
	}
	if !strings.Contains(v, "SELECTED EVIDENCE PATH") {
		t.Fatalf("the secondary must show the focused row's evidence path:\n%s", v)
	}
	if !strings.Contains(v, "authority-ceiling") {
		t.Fatalf("the primary list must show the epistemic rows:\n%s", v)
	}
}

// The emergent connector must NOT derive over a denied facet. Two rows whose ONLY shared facet is a
// DENIED status: off air the connector derives "shares status"; on air the status projects to the
// redaction token, so epistemicEntity omits it and the connector can never name the denied dimension
// (the derived-channel invariant — mirrors the tasks-cohort TestEmergentRelationOmitsDeniedFacetOnAir).
// (Family is intentionally NOT used to seed this: it is a non-PII category, never redacted.)
func TestEpistemicsEmergentRelationOmitsDeniedFacetOnAir(t *testing.T) {
	ep := grammar.EpistemicsSummary{Rows: []grammar.EpistemicReadRow{
		{RowID: "r1", Family: "claim", Subject: "a", Status: "asserted", Authority: "platform", Privacy: "metadata-only",
			AIR: map[string]string{"row_id": "ok", "family": "ok", "subject": "ok", "status": "deny", "authority": "ok", "privacy": "ok"}},
		{RowID: "r2", Family: "observation", Subject: "b", Status: "asserted", Authority: "world", Privacy: "internal-only",
			AIR: map[string]string{"row_id": "ok", "family": "ok", "subject": "ok", "status": "deny", "authority": "ok", "privacy": "ok"}},
	}}
	m := New("REINS").FoldEpistemics(ep, false)
	m.Width, m.Height, m.Page, m.EpiFocus = 180, 44, PageEpistemics, 0

	m.AIR = false
	if rel := ansi.Strip(m.epistemicsEmergentRelation()); !strings.Contains(rel, "status") {
		t.Fatalf("off air the shared status IS the strongest relation: %q", rel)
	}
	m.AIR = true
	if rel := ansi.Strip(m.epistemicsEmergentRelation()); strings.Contains(rel, "status") {
		t.Fatalf("on air a denied status must not appear in the emergent connector relation: %q", rel)
	}
}
