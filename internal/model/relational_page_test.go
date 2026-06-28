package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

func relationalPageModel() Model {
	m := New("REINS")
	m.Width, m.Height, m.Page, m.SplitContext = 180, 44, PageRelational, true
	m = m.FoldSessions([]grammar.Session{
		{Role: "cc-secret-lane"},
		{Role: "cc-beta"},
	}, false)
	m.TurnLadder = []grammar.Turn{
		{Prov: "operator"},
		{Prov: "model"},
		{Prov: "structured"},
		{Prov: "untrusted"},
	}
	return m
}

func TestRelationalComposesViaAlgebraNativeBinding(t *testing.T) {
	m := relationalPageModel()
	m = m.switchPage(PageRelational)
	if !m.composesViaAlgebra() {
		t.Fatal("PageRelational must compose via the view algebra")
	}
	if m.splitContextActive() {
		t.Fatal("PageRelational must not activate the legacy session-frozen split")
	}
	if m.commandSelectionPage() != PageRelational {
		t.Fatalf("templates/yank context must bind to PageRelational, got %d", m.commandSelectionPage())
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"CONSENT POSTURE", "consent facet -> posture", "A6", "projection-pending", "broadcast frame"} {
		if !strings.Contains(v, want) {
			t.Fatalf("relational page should render %q via algebra split:\n%s", want, v)
		}
	}
}

func TestRelationalJMovesRelationalFocus(t *testing.T) {
	m := relationalPageModel()
	before := m.RelationalFocus
	m = step(m, "j")
	if m.RelationalFocus != before+1 {
		t.Fatalf("j must move RelationalFocus natively (%d→%d)", before, m.RelationalFocus)
	}
	if !strings.Contains(m.Status, "consent facet 2/4") {
		t.Fatalf("j should announce the focused consent facet, got status %q", m.Status)
	}
}

func TestRelationalListBodyRendersAllFacetNames(t *testing.T) {
	m := relationalPageModel()
	body := ansi.Strip(m.relationalListBody(140, 12))
	for _, want := range []string{"CONSENT POSTURE", "broadcast frame", "authorship", "field gating", "stakeholders"} {
		if !strings.Contains(body, want) {
			t.Fatalf("relational list must render facet name %q:\n%s", want, body)
		}
	}
}
