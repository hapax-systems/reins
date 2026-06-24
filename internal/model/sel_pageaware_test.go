package model

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/grammar"
)

func evFixture() []grammar.Event {
	return []grammar.Event{
		{TS: "10:00", Kind: "task.updated", Subject: "first", Actor: "alpha", Summary: "did a thing", AIR: map[string]string{"subject": "ok", "actor": "ok", "summary": "ok"}},
		{TS: "10:05", Kind: "pr.opened", Subject: "second", Actor: "beta", Summary: "opened pr", AIR: map[string]string{"subject": "ok", "actor": "ok", "summary": "ok"}},
		{TS: "10:09", Kind: "review.pass", Subject: "third", Actor: "gamma", Summary: "approved", AIR: map[string]string{"subject": "ok", "actor": "ok", "summary": "ok"}},
	}
}

// The :events page is a first-class selectable surface: the cursor defaults to newest + [j/k] moves it.
func TestEventsAreSelectable(t *testing.T) {
	m := New("REINS").Fold(evFixture(), false)
	m.Width, m.Height, m.Page = 120, 40, PageEvents
	if ev, ok := m.FocusedEvent(); !ok || ev.Subject != "third" {
		t.Fatalf("the events cursor should default to the NEWEST event, got %+v", ev)
	}
	m = step(m, "k") // up one → 2nd-newest
	if ev, _ := m.FocusedEvent(); ev.Subject != "second" {
		t.Fatalf("[k] should move the events cursor up, got %q", ev.Subject)
	}
	m = step(m, "g") // top → oldest
	if ev, _ := m.FocusedEvent(); ev.Subject != "first" {
		t.Fatalf("[g] should jump to the oldest event, got %q", ev.Subject)
	}
}

// [y] on :events yanks an EVENT field (page-aware), not a task field.
func TestEventYank(t *testing.T) {
	m := New("REINS").Fold(evFixture(), false)
	m.Width, m.Height, m.Page = 120, 40, PageEvents
	m = step(m, "y") // enter yank mode over the focused (newest) event
	if m.Mode != ModeYank {
		t.Fatal("[y] on :events should enter yank mode")
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")}) // pick subject
	m = nm.(Model)
	if m.Input != "third" || m.Mode != ModeCommand {
		t.Fatalf("yanking the subject should pre-seed the command line with 'third', got %q (mode %d)", m.Input, m.Mode)
	}
	if len(m.Ring) != 1 || m.Ring[0].Field != "subject" {
		t.Fatalf("the event yank should push to the kill-ring, got %+v", m.Ring)
	}
}

// task-only verbs are inert on :events (no door, no field-rank descent) — the dead-end is closed.
func TestTaskVerbsInertOnEvents(t *testing.T) {
	m := New("REINS").Fold(evFixture(), false).FoldTasks([]grammar.Task{{TaskID: "t", AIR: map[string]string{}}}, false)
	m.Width, m.Height, m.Page = 120, 40, PageEvents
	if m = step(m, "enter"); m.DoorOpen {
		t.Fatal("[enter] must not open the whois door on :events")
	}
	if m = step(m, "tab"); m.Sel.Rank == RankField {
		t.Fatal("[tab] must not descend into task fields on :events")
	}
	if m = step(m, "V"); len(m.Sel.Members) > 0 {
		t.Fatal("[V] class-select must be inert on :events")
	}
}
