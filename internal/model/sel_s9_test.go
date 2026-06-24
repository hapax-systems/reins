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
