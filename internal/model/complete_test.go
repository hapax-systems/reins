package model

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/grammar"
)

func tab(m Model) Model  { nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab}); return nm.(Model) }
func ent(m Model) Model  { nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); return nm.(Model) }
func rght(m Model) Model { nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight}); return nm.(Model) }

// A verb with args descends into a sub-menu on accept; the arg leaf then runs the full command.
func TestSubMenuDescentAndRun(t *testing.T) {
	g := grammar.Graph{Nodes: []grammar.Node{{ID: "n", Layer: "domain"}}}
	m := New("REINS").FoldDynamics(g, false)
	m.Width, m.Height = 120, 40
	m = step(m, ":") // command line
	m = tab(m)       // events(0) -> tasks(1)
	m = tab(m)       // tasks(1) -> dynamics(2), which has a sub-menu
	c, _ := m.curCandidate()
	if c.Label != "dynamics" || len(c.Sub) == 0 {
		t.Fatalf("expected to highlight the dynamics sub-menu node, got %q (sub=%d)", c.Label, len(c.Sub))
	}
	m = ent(m) // accept -> DESCEND (input becomes "dynamics ")
	if m.Input != "dynamics " || m.Mode != ModeCommand {
		t.Fatalf("accepting a sub-menu node should descend (input=%q mode=%d)", m.Input, m.Mode)
	}
	cands := m.completionTree()
	if len(cands) != 6 || cands[0].Label != "overview" {
		t.Fatalf("descent should reveal the dynamics args, got %d (%v)", len(cands), cands)
	}
	m = tab(m) // overview(0) -> domain(1)
	m = ent(m) // accept the leaf -> RUN "dynamics domain"
	if m.Mode != ModeNormal || m.Page != PageDynamics {
		t.Fatalf("accepting an arg leaf should run it: mode=%d page=%v", m.Mode, m.Page)
	}
}

// [→] fills the line (descends or fills) but never executes.
func TestFillDoesNotRun(t *testing.T) {
	m := New("REINS")
	m.Width, m.Height = 120, 40
	m = step(m, ":")
	m = tab(m) // -> tasks (a leaf verb)
	m = rght(m)
	if m.Mode != ModeCommand {
		t.Fatal("[→] must NOT leave the command line")
	}
	if m.Input != "tasks" {
		t.Fatalf("[→] should fill the input with the leaf value, got %q", m.Input)
	}
}

// dynamic-on-selection: a selected field leads with a paste candidate; accepting injects, not runs.
func TestPasteCandidateInjects(t *testing.T) {
	m := New("REINS").FoldTasks([]grammar.Task{{TaskID: "abc", Stage: "S5", AIR: map[string]string{}}}, false)
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	m.Mode = ModeCommand
	m.Sel.Rank, m.Sel.Field = RankField, "stage"
	cands := m.completionTree()
	if len(cands) == 0 || cands[0].Label != "paste stage" {
		t.Fatalf("a selected field should lead with a paste candidate, got %v", cands)
	}
	m = ent(m) // accept the paste -> inject the value, stay in command mode
	if m.Mode != ModeCommand || m.Input != "S5" {
		t.Fatalf("paste should inject the field value (input=%q mode=%d)", m.Input, m.Mode)
	}
}

// the filter surface uses the SAME engine — candidates are the live task ids.
func TestFilterCandidates(t *testing.T) {
	m := New("REINS").FoldTasks([]grammar.Task{
		{TaskID: "alpha", AIR: map[string]string{}},
		{TaskID: "beta", AIR: map[string]string{}},
	}, false)
	m.Mode = ModeFilter
	m.Filter = "al"
	cands := m.completionTree()
	if len(cands) != 1 || cands[0].Value != "alpha" {
		t.Fatalf("filter completion should offer matching ids, got %v", cands)
	}
}

// the [/] filter navigates + fills via the SAME engine (autocomplete genuinely everywhere).
func TestFilterNavigateAndFill(t *testing.T) {
	m := New("REINS").FoldTasks([]grammar.Task{
		{TaskID: "ant", AIR: map[string]string{}},
		{TaskID: "ace", AIR: map[string]string{}},
	}, false)
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	m = step(m, "/") // enter filter mode
	if m.Mode != ModeFilter {
		t.Fatal("/ should open the filter")
	}
	m = step(m, "a") // narrows to ant, ace
	m = tab(m)       // highlight the 2nd id
	if m.CompIdx != 1 {
		t.Fatalf("Tab should advance the filter candidate, got %d", m.CompIdx)
	}
	m = rght(m) // [→] fills the filter with the highlighted id
	if m.Filter != "ace" || len(m.visibleTasks()) != 1 {
		t.Fatalf("[→] should fill the filter to 'ace' (filter=%q n=%d)", m.Filter, len(m.visibleTasks()))
	}
}
