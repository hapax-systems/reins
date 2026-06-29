package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// Projection-2 readout: the coordinator selection-context resolves the lane bound to the FOCUSED task
// (via ClaimedTask, the pane's own cursor) and shows its route + eval signals. AIR: on air the binding is
// disclosed only when claimed_task + task_id BOTH air — else honest-dark (no association leak).
func TestCoordContextReadoutResolvesSessionAndAirGatesBinding(t *testing.T) {
	m := New("R")
	m.Tasks = []grammar.Task{{TaskID: "task-1", AIR: map[string]string{"task_id": "ok"}}}
	m.Sessions = []grammar.Session{{
		Role: "cx-alpha", ClaimedTask: "task-1", Readiness: "claim", Attention: 0.8, Blocker: "none",
		AIR: map[string]string{"role": "ok", "claimed_task": "ok", "readiness": "ok", "attention": "ok", "blocker": "ok"},
	}}

	off := ansi.Strip(m.coordContextReadout(m.Tasks[0], 120))
	if !strings.Contains(off, "coord ctx") || !strings.Contains(off, "cx-alpha") || !strings.Contains(off, "ready") {
		t.Fatalf("must resolve the bound lane + show its eval signals:\n%s", off)
	}

	m.AIR = true
	m.Sessions[0].AIR = map[string]string{"role": "ok", "readiness": "ok"} // claimed_task DENIED on air
	on := ansi.Strip(m.coordContextReadout(m.Tasks[0], 120))
	if strings.Contains(on, "cx-alpha") || !strings.Contains(on, "no lane bound") {
		t.Fatalf("on air a denied claimed_task must not disclose the binding:\n%s", on)
	}
}
