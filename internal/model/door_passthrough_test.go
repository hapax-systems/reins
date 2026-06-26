package model

import (
	"strings"
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

// Inc 1c: doors pass window-nav keys through (cycle/jump) instead of silently swallowing
// them. A window key pressed inside an open door jumps the page AND closes the door — the
// page-jump modal-trap the audit verified at the door handlers' default branch.

func TestDoorPassesThroughWindowJump(t *testing.T) {
	tasks := []grammar.Task{{TaskID: "d-1", Stage: "S7_RELEASE", PredictedStage: "hold",
		Criticality: "warn", AIR: map[string]string{"task_id": "ok", "stage": "ok"}}}
	m := New("REINS").FoldTasks(tasks, false)
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	m = step(m, "enter") // open the /whois door
	if !m.DoorOpen {
		t.Fatal("[enter] should open the /whois door")
	}
	// "1" is the events window key — it must pass THROUGH the door (jump + close),
	// not be silently swallowed as it was before Inc 1c.
	m = step(m, "1")
	if m.DoorOpen {
		t.Fatal("window key [1] must pass through the door (close it), not be swallowed")
	}
	if m.Page != PageEvents {
		t.Fatalf("window key [1] must jump the page: want PageEvents, got %d", m.Page)
	}
	if strings.Contains(m.View(), "DOOR /whois") {
		t.Fatal("the /whois door must not render after a pass-through jump")
	}
}
