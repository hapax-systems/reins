package model

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/grammar"
)

// A yank fires a flash confirmation + arms a clearing tick; the matching FlashClearMsg clears it,
// but a STALE one (older seq) must not wipe a newer flash.
func TestYankFlashesAndClears(t *testing.T) {
	m := New("REINS").FoldTasks([]grammar.Task{{TaskID: "t1", Stage: "S5", AIR: map[string]string{"stage": "ok"}}}, false)
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	m.Sel.Rank, m.Sel.Field = RankField, "stage"

	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = nm.(Model)
	if m.Flash == "" {
		t.Fatal("yank should set a flash confirmation")
	}
	if cmd == nil {
		t.Fatal("yank should arm a clearing tick")
	}
	seq := m.FlashSeq

	// a stale tick (older seq) must NOT clear
	nm, _ = m.Update(FlashClearMsg{Seq: seq - 1})
	m = nm.(Model)
	if m.Flash == "" {
		t.Fatal("a stale FlashClearMsg must not clear a newer flash")
	}
	// the matching tick clears
	nm, _ = m.Update(FlashClearMsg{Seq: seq})
	m = nm.(Model)
	if m.Flash != "" {
		t.Fatalf("the matching FlashClearMsg should clear the flash, got %q", m.Flash)
	}
}
