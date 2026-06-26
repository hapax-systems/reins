package model

import (
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

// The activity ladder (audit §2 #1): the hotlist must flag windows whose state CHANGED
// since the operator last visited — "what changed," not just "what is" (AUTO-SURFACE).
// A window is snapshotted when you leave it (switchPage); windowActive is true when its
// current signature differs from that snapshot. The current page is never active.

func TestWindowActiveFlagsChangedSinceVisit(t *testing.T) {
	m := New("t")
	m.Page = PageEvents
	m.Tasks = []grammar.Task{{TaskID: "a", Criticality: "warn", AIR: map[string]string{"task_id": "ok"}}}
	// visit tasks, then return to events -> tasks snapshotted at 1 task
	m = m.switchPage(PageTasks)
	m = m.switchPage(PageEvents)
	if m.windowActive(PageTasks) {
		t.Fatal("tasks unchanged since visit: must NOT be active")
	}
	// tasks grow while the operator is away on events -> tasks becomes active
	m.Tasks = append(m.Tasks, grammar.Task{TaskID: "b", Criticality: "warn", AIR: map[string]string{"task_id": "ok"}})
	if !m.windowActive(PageTasks) {
		t.Fatal("tasks changed since visit: must be active")
	}
	// the current page is never active (you are looking at it)
	if m.windowActive(PageEvents) {
		t.Fatal("the current page must not be active")
	}
	// a never-visited window has no baseline -> not active (no "change since seen")
	if m.windowActive(PageDynamics) {
		t.Fatal("a never-visited window has no baseline: must not be active")
	}
}
