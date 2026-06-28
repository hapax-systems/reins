package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// taskWorkDomainPane (the tasks secondary) renders derived channels — constraints, the rel-count,
// the freshness value — that must consult per-field AIR: a denied criticality/predicted/freshness/
// rel_count may not air, directly OR through a derived constraint. (Latent until the always-split
// migration rendered this pane at widths where it never appeared.)
func TestTaskWorkDomainPaneIsAirSafe(t *testing.T) {
	task := grammar.Task{
		TaskID: "public-task", Stage: "S7", PredictedStage: "hold", Owner: "owner-lane",
		Criticality: "crit", Freshness: 0.9, RelCount: 7,
		AIR: map[string]string{
			"task_id": "ok", "stage": "ok", "predicted_stage": "deny", "owner": "deny",
			"criticality": "deny", "freshness": "deny", "rel_count": "deny",
		},
	}
	m := New("R").FoldTasks([]grammar.Task{task}, false)
	m.AIR = true
	out := ansi.Strip(m.taskWorkDomainPane(80))
	for _, leak := range []string{"critical exception", "release blocked", "owner-lane", "●7", "0.90"} {
		if strings.Contains(out, leak) {
			t.Fatalf("taskWorkDomainPane leaked denied derived channel %q on air:\n%s", leak, out)
		}
	}
	if !strings.Contains(out, "▒▒▒") {
		t.Fatalf("the pane should keep structure with redaction:\n%s", out)
	}
}

// rel_count==0 must NOT disclose itself on air through the "task-edge source absent" advisory: the
// advisory's PRESENCE is a 1-bit derived channel (class-c) revealing rel_count==0 while the field is
// denied. Off air the advisory is a legitimate honesty cue and must still render.
func TestTaskWorkDomainPaneRelCountZeroDoesNotLeakViaMessagePresence(t *testing.T) {
	task := grammar.Task{
		TaskID: "public-task", Stage: "S7", RelCount: 0,
		AIR: map[string]string{"task_id": "ok", "stage": "ok", "rel_count": "deny"},
	}
	m := New("R").FoldTasks([]grammar.Task{task}, false)
	m.AIR = true
	if out := ansi.Strip(m.taskWorkDomainPane(80)); strings.Contains(out, "task-edge source absent") {
		t.Fatalf("rel_count==0 must not disclose itself via the source-absent advisory on air:\n%s", out)
	}
	m.AIR = false
	if off := ansi.Strip(m.taskWorkDomainPane(80)); !strings.Contains(off, "task-edge source absent") {
		t.Fatalf("off air the source-absent advisory should still render:\n%s", off)
	}
}
