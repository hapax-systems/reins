package model

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/grammar"
)

func mouseTestModel() Model {
	tasks := []grammar.Task{
		{TaskID: "t1", AIR: map[string]string{}},
		{TaskID: "t2", AIR: map[string]string{}},
		{TaskID: "t3", AIR: map[string]string{}},
	}
	m := New("REINS").FoldTasks(tasks, false)
	m.Width, m.Height, m.Page = 120, 40, PageTasks
	return m
}

// The mouse WHEEL drives focus through the EXACT j/k key path (same result), and mouse input NEVER
// invokes a governed command (mutations stay behind the apply seam).
func TestMouseWheelMatchesKeyNav(t *testing.T) {
	base := mouseTestModel()

	kd, _ := base.Update(tea.KeyMsg{Type: tea.KeyDown})
	md, _ := base.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if kd.(Model).View() != md.(Model).View() {
		t.Fatal("wheel-down must produce the identical result to key-down")
	}
	if md.(Model).View() == base.View() {
		t.Fatal("wheel-down did not move the focus (nav is a no-op in the fixture)")
	}
	if md.(Model).PendingCommand != nil {
		t.Fatal("a mouse wheel must NOT invoke a governed command")
	}

	ku, _ := base.Update(tea.KeyMsg{Type: tea.KeyUp})
	mu, _ := base.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if ku.(Model).View() != mu.(Model).View() {
		t.Fatal("wheel-up must produce the identical result to key-up")
	}
}

// A non-wheel mouse event (click/motion) is inert — no command staged, no panic.
func TestMouseClickIsInert(t *testing.T) {
	base := mouseTestModel()
	nm, _ := base.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 5, Y: 5})
	if nm.(Model).PendingCommand != nil {
		t.Fatal("a click must not invoke a governed command")
	}
	if nm.(Model).View() != base.View() {
		t.Fatal("an inert click should not change the view")
	}
}
