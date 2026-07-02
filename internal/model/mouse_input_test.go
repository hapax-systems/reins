package model

import (
	"strings"
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

// A LEFT tap on a task row focuses that task — verified against the ACTUAL View() geometry (parse to find
// the row's Y, tap there, assert focus). This View-parsing guard fails if the layout chrome shifts, rather
// than silently mis-mapping taps. A tap never invokes a command.
func TestMouseTapFocusesTaskRow(t *testing.T) {
	m := mouseTestModel() // 120x40, PageTasks, tasks t1/t2/t3, initial focus t1
	lines := strings.Split(m.View(), "\n")
	targetY := -1
	for y, ln := range lines {
		if strings.Contains(ln, "t3") { // t3's task row (initial focus is t1, so the rail can't hold t3)
			targetY = y
			break
		}
	}
	if targetY < 0 {
		t.Fatal("t3's row not found in View() — cannot verify tap geometry")
	}
	nm, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 3, Y: targetY})
	ft, ok := nm.(Model).FocusedTask()
	if !ok || ft.TaskID != "t3" {
		t.Fatalf("tap at t3's row (Y=%d) must focus t3, got %q", targetY, ft.TaskID)
	}
	if nm.(Model).PendingCommand != nil {
		t.Fatal("a tap must NOT invoke a governed command")
	}
}

// A LEFT tap on a session (lane) row focuses that lane — the second primary touch surface. Verified
// against the ACTUAL View() geometry (parse the lane's Y, tap, assert focus).
func TestMouseTapFocusesSessionRow(t *testing.T) {
	sessions := []grammar.Session{
		{Role: "alpha", AIR: map[string]string{}},
		{Role: "beta", AIR: map[string]string{}},
		{Role: "gamma", AIR: map[string]string{}},
	}
	m := New("REINS").FoldSessions(sessions, false)
	m.Width, m.Height, m.Page = 120, 40, PageSessions
	lines := strings.Split(m.View(), "\n")
	targetY := -1
	for y, ln := range lines {
		if strings.Contains(ln, "gamma") {
			targetY = y
			break
		}
	}
	if targetY < 0 {
		t.Fatal("gamma lane not found in View() — cannot verify tap geometry")
	}
	nm, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 3, Y: targetY})
	fs, ok := nm.(Model).FocusedSession()
	if !ok || fs.Role != "gamma" {
		t.Fatalf("tap at gamma's row (Y=%d) must focus gamma, got %q", targetY, fs.Role)
	}
	if nm.(Model).PendingCommand != nil {
		t.Fatal("a tap must NOT invoke a governed command")
	}
}

// Taps off the task list (above the rows, or over the right-hand rail) are inert — no focus jump, no command.
func TestMouseTapOffListInert(t *testing.T) {
	m := mouseTestModel()
	start, _ := m.FocusedTask()
	// over the rail (x past the main area at 120 cols: mainW = 120-36-1 = 83)
	nm, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 110, Y: 7})
	if ft, _ := nm.(Model).FocusedTask(); ft.TaskID != start.TaskID {
		t.Fatalf("a tap over the rail must not move focus (%s -> %s)", start.TaskID, ft.TaskID)
	}
	// above the first row (in the title chrome)
	nm2, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 3, Y: 1})
	if ft, _ := nm2.(Model).FocusedTask(); ft.TaskID != start.TaskID {
		t.Fatalf("a tap in the title chrome must not move focus")
	}
	if nm.(Model).PendingCommand != nil || nm2.(Model).PendingCommand != nil {
		t.Fatal("off-list taps must not invoke a command")
	}
}
