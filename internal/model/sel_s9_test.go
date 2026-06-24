package model

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/grammar"
)

func TestCountSelectFiltersByClass(t *testing.T) {
	tasks := []grammar.Task{
		{TaskID: "a", Criticality: "crit", AIR: map[string]string{}},
		{TaskID: "b", Criticality: "warn", AIR: map[string]string{}},
		{TaskID: "c", Criticality: "crit", AIR: map[string]string{}},
	}
	m := New("REINS").FoldTasks(tasks, false)
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	m = step(m, "f") // hint overlay (rows + counts labeled)
	if m.Mode != ModeHint {
		t.Fatal("f -> hint mode")
	}
	// 'C' is the crit-count label -> filter to crit class
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("C")})
	m = nm.(Model)
	if m.CritFilter != "crit" || len(m.visibleTasks()) != 2 {
		t.Fatalf("selecting the crit count should filter to the 2 crit tasks: cf=%q n=%d", m.CritFilter, len(m.visibleTasks()))
	}
}

func TestActItemJumpsToBlocker(t *testing.T) {
	tasks := []grammar.Task{
		{TaskID: "calm", Criticality: "ok", PredictedStage: "S5", AIR: map[string]string{}},
		{TaskID: "stuck", Criticality: "ok", PredictedStage: "hold", AIR: map[string]string{}},
		{TaskID: "hot", Criticality: "crit", PredictedStage: "S5", AIR: map[string]string{}},
	}
	m := New("REINS").FoldTasks(tasks, false)
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	bi := m.blockedIndices() // expect [1 (hold), 2 (crit)]
	if len(bi) != 2 || bi[0] != 1 || bi[1] != 2 {
		t.Fatalf("blockedIndices = %v, want [1 2]", bi)
	}
	m = step(m, "f") // hint overlay (Act items labeled 1/2)
	m = step(m, "2") // jump to the 2nd blocker (hot/crit, index 2)
	if m.Mode != ModeNormal {
		t.Fatal("jump should return to normal mode")
	}
	if got := m.visibleTasks()[m.Focus].TaskID; got != "hot" {
		t.Fatalf("Act-item 2 should focus 'hot', got %q (focus=%d)", got, m.Focus)
	}
}
