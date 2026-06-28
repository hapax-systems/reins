package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func axesPageModel() Model {
	m := New("REINS")
	m.Width, m.Height, m.Page, m.SplitContext = 180, 44, PageAxes, true
	return m
}

func TestAxesComposesViaAlgebraNativeBinding(t *testing.T) {
	m := axesPageModel()
	m = m.switchPage(PageAxes)
	if !m.composesViaAlgebra() {
		t.Fatal("PageAxes must compose via the view algebra")
	}
	if m.splitContextActive() {
		t.Fatal("PageAxes must not activate the legacy session-frozen split")
	}
	if m.commandSelectionPage() != PageAxes {
		t.Fatalf("templates/yank context must bind to PageAxes, got %d", m.commandSelectionPage())
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"CASE-ROLE AXES", "axis -> five-tuple contract", "A1", "A6", "blind-spot"} {
		if !strings.Contains(v, want) {
			t.Fatalf("axes page should render %q via algebra split:\n%s", want, v)
		}
	}
}

func TestAxesJMovesAxisFocus(t *testing.T) {
	m := axesPageModel()
	before := m.AxisFocus
	m = step(m, "j")
	if m.AxisFocus != before+1 {
		t.Fatalf("j must move AxisFocus natively (%d→%d)", before, m.AxisFocus)
	}
	if !strings.Contains(m.Status, "axis A2 · Process") {
		t.Fatalf("j should announce the focused axis, got status %q", m.Status)
	}
}

func TestAxisListBodyRendersAllSixIDsAndStatusGlyphs(t *testing.T) {
	m := axesPageModel()
	body := ansi.Strip(m.axisListBody(120, 14))
	for _, want := range []string{"A1", "A2", "A3", "A4", "A5", "A6", "●", "◐", "○"} {
		if !strings.Contains(body, want) {
			t.Fatalf("axis list must render %q (ids + honest status glyphs):\n%s", want, body)
		}
	}
}

func TestAxisDetailBodyRendersFocusedFiveTuple(t *testing.T) {
	m := axesPageModel()
	m.AxisFocus = 3 // A4 Authority+Capability
	body := ansi.Strip(m.axisDetailBody(96))
	for _, want := range []string{"A4", "Authority+Capability", "question", "state ∅", "controls", "provenance", "blind-spot"} {
		if !strings.Contains(body, want) {
			t.Fatalf("axis detail must render focused five-tuple field %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "A1 · Identity") {
		t.Fatalf("axis detail should render the focused axis, not A1:\n%s", body)
	}
}
