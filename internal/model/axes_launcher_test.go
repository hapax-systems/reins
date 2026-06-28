package model

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The :axes overview is a LAUNCHER — [enter] on an axis row jumps to that axis's dedicated pane
// (its Maps command). This is the navigation story that ties the framework overview to the panes.
func TestAxesLauncherJumpsToAxisPane(t *testing.T) {
	send := func(m Model, v tea.KeyMsg) Model { nm, _ := m.Update(v); return nm.(Model) }
	enter := tea.KeyMsg{Type: tea.KeyEnter}

	cases := []struct {
		focus int
		want  int
		label string
	}{
		{0, PageIdentity, "A1 → :identity"},
		{5, PageRelational, "A6 → :relational"},
		{1, PageReadiness, "A2 → :readiness"},
		{3, PageCaps, "A4 → :capabilities"},
		{4, PageDynamics, "A5 → :dynamics"},
	}
	for _, c := range cases {
		m := New("REINS")
		m.Width, m.Height = 150, 40
		m = m.switchPage(PageAxes)
		m.AxisFocus = c.focus
		m = send(m, enter)
		if m.Page != c.want {
			t.Fatalf("%s: enter must launch page %d; got %d", c.label, c.want, m.Page)
		}
	}
}
