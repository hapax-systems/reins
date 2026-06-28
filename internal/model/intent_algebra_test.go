package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Inc 3 TRANSFORM — PageIntent migrates onto the view-algebra as a SELF-ANCHORED page (own IntentFocus),
// ABOLISHING the legacy session-frozen reference split (sessions │ intent review). The primary IS the
// governed-route TARGETS list; j moves the target (IntentFocus) NATIVELY; the secondary is the selected
// target's review ladder. The connector is the honest "selected target → governed route review"
// elucidation (the targets are a static catalog, so relate.Derive does not apply — like :dispatch).

func intentAlgebraModel() Model {
	m := New("REINS")
	m.Width, m.Height, m.Page = 200, 48, PageIntent
	m.IntentFocus = 0
	return m
}

func TestIntentComposesViaAlgebraNativeBinding(t *testing.T) {
	m := intentAlgebraModel()
	if !m.composesViaAlgebra() {
		t.Fatal("PageIntent must compose via the view-algebra (only-split)")
	}
	if isSessionAnchoredPage(m.Page) {
		t.Fatal("a migrated page must NOT be session-frozen (not session-anchored)")
	}
	if m.commandSelectionPage() != PageIntent {
		t.Fatalf("templates/yank must bind to PageIntent, got page %d", m.commandSelectionPage())
	}
}

func TestIntentJMovesTargetNatively(t *testing.T) {
	m := intentAlgebraModel()
	before := m.IntentFocus
	m = step(m, "j")
	if m.IntentFocus != before+1 {
		t.Fatalf("j must move the intent target natively (IntentFocus %d→%d)", before, m.IntentFocus)
	}
}

func TestIntentFocusTemplateBindsToTarget(t *testing.T) {
	m := intentAlgebraModel()
	first := lookupIntentArgs()[0].Label
	got := m.resolveTemplate("route {{focus}}")
	if !strings.Contains(got, first) {
		t.Fatalf("{{focus}} must resolve to the focused target %q, got %q", first, got)
	}
	if strings.Contains(got, "{{focus}}") {
		t.Fatalf("{{focus}} must not render literally, got %q", got)
	}
	m = step(m, "j")
	second := lookupIntentArgs()[1].Label
	if got := m.resolveTemplate("{{focus}}"); !strings.Contains(got, second) {
		t.Fatalf("{{focus}} must track the moved target %q, got %q", second, got)
	}
}

func TestIntentViewIsAlgebraSplitWithReviewSecondary(t *testing.T) {
	m := intentAlgebraModel()
	m = m.Exec("intent dispatch")
	v := ansi.Strip(m.View())
	if strings.Contains(v, "split sessions") || strings.Contains(v, "[j/k]source") {
		t.Fatalf("migrated intent must NOT render the legacy session-frozen split:\n%s", v)
	}
	flat := flattenSplitColumns(v)
	for _, want := range []string{"INTENT REVIEW", "ROUTE PREVIEW LADDER", "dispatch", "no effect emitted"} {
		if !strings.Contains(flat, want) {
			t.Fatalf("migrated intent must render the targets list + review secondary, missing %q:\n%s", want, v)
		}
	}
}
