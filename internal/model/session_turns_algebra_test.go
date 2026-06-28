package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func sessionTurnsModel() Model {
	m := New("REINS")
	m.Width, m.Height = 180, 44
	m = m.switchPage(PageSessionTurns)
	return m
}

func TestSessionTurnsComposesViaAlgebraNativeBinding(t *testing.T) {
	m := sessionTurnsModel()
	if !m.composesViaAlgebra() {
		t.Fatal("PageSessionTurns must compose via the view-algebra (only-split)")
	}
	if isSessionAnchoredPage(m.Page) {
		t.Fatal("PageSessionTurns must not activate the legacy session-frozen split")
	}
	if m.commandSelectionPage() != PageSessionTurns {
		t.Fatalf("templates/yank must bind to PageSessionTurns, got page %d", m.commandSelectionPage())
	}
	if len(m.TurnLadder) < 8 || len(m.TurnBlocks) == 0 {
		t.Fatalf("switchPage(PageSessionTurns) must load the fixture ladder and blocks, got %d/%d", len(m.TurnLadder), len(m.TurnBlocks))
	}
}

func TestSessionTurnsJMovesTurnFocus(t *testing.T) {
	m := sessionTurnsModel()
	before := m.TurnFocus
	m = step(m, "j")
	if m.TurnFocus != before+1 {
		t.Fatalf("j must move TurnFocus natively (%d→%d)", before, m.TurnFocus)
	}
	for i := 0; i < 20; i++ {
		m = step(m, "j")
	}
	if m.TurnFocus != len(m.TurnLadder)-1 {
		t.Fatalf("TurnFocus must clamp at bottom, got %d of %d", m.TurnFocus, len(m.TurnLadder))
	}
}

func TestSessionTurnsRendersLadderAndDetail(t *testing.T) {
	m := sessionTurnsModel()
	m.TurnFocus = 3 // tool_call has an expanded block
	v := ansi.Strip(m.View())
	for _, want := range []string{"LANE-RAIL", "TIME", "cc-reins", "go test ./... -run Trace"} {
		if !strings.Contains(v, want) {
			t.Fatalf("session turns page should render %q in ladder/detail:\n%s", want, v)
		}
	}
	if strings.Contains(v, "split sessions") || strings.Contains(v, "[j/k]source") {
		t.Fatalf("session turns must not render the legacy session-frozen split:\n%s", v)
	}
}

func TestSessionTurnsAIRRedactsBodiesKeepsSkeleton(t *testing.T) {
	m := sessionTurnsModel()
	m.AIR = true
	m.TurnFocus = 0 // operator-provenance prompt body must never air
	v := ansi.Strip(m.View())
	for _, leak := range []string{"fix the flaky trace test", "widen the 3s timeout", "ok  internal/grammar"} {
		if strings.Contains(v, leak) {
			t.Fatalf("on air turn bodies must redact %q while skeleton survives:\n%s", leak, v)
		}
	}
	for _, want := range []string{"LANE-RAIL", "TIME", "operator", "cc-reins", "▒▒▒"} {
		if !strings.Contains(v, want) {
			t.Fatalf("on air skeleton should survive with redactions, missing %q:\n%s", want, v)
		}
	}
}

func TestSessionTurnsTemplateFocusAndPasteAreAirSafe(t *testing.T) {
	m := sessionTurnsModel()
	id := TurnID(m.TurnLadder[0])
	if got := m.resolveTemplate("{{focus}}"); got != id {
		t.Fatalf("{{focus}} must resolve to the focused turn id, got %q want %q", got, id)
	}
	m.Sel.Rank, m.Sel.Field = RankField, "summary"
	if field, value, ok := m.selectedPasteValue(); !ok || field != "summary" || !strings.Contains(value, "fix the flaky trace test") {
		t.Fatalf("selectedPasteValue should bind summary off-air, got %q %q ok=%v", field, value, ok)
	}
	m.AIR = true
	if got := m.resolveTemplate("{{sel.summary}}"); strings.Contains(got, "fix the flaky trace test") || got == "" {
		t.Fatalf("on air summary template must redact, got %q", got)
	}
}
