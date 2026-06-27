package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// coordinatorSelectionContext renders on the LIVE coordinator: every per-field value must redact on
// air (#7). Stage is allowlisted (structural) and survives; prior/predicted/owner/criticality/rel_count deny.
func TestCoordinatorSelectionContextIsAirSafe(t *testing.T) {
	task := grammar.Task{
		TaskID: "pub", Stage: "S7", PriorStage: "S6", PredictedStage: "hold", Owner: "owner-lane",
		Criticality: "crit", RelCount: 7,
		AIR: map[string]string{
			"task_id": "ok", "stage": "ok", "prior_stage": "deny", "predicted_stage": "deny",
			"owner": "deny", "criticality": "deny", "rel_count": "deny",
		},
	}
	m := New("R").FoldTasks([]grammar.Task{task}, false)
	m.AIR = true
	out := ansi.Strip(m.coordinatorSelectionContext(120))
	for _, leak := range []string{"S6", "hold", "owner-lane", "7 ties"} {
		if strings.Contains(out, leak) {
			t.Fatalf("coordinatorSelectionContext leaked %q on air:\n%s", leak, out)
		}
	}
	if !strings.Contains(out, "S7") { // an allowlisted structural field still airs
		t.Fatalf("the allowlisted stage S7 should still air:\n%s", out)
	}
}

// The GLOBAL chrome aggregates — the vital strip's hold·risk breakdown (blockedBreakdown) and the
// coordinator throughput's held tally — render on EVERY page (incl. the live coordinator), so a
// denied criticality/predicted_stage classified into those counts leaks on the livestream.
func TestGlobalAggregatesAreAirSafe(t *testing.T) {
	denied := grammar.Task{
		TaskID: "x", Criticality: "crit", PredictedStage: "hold", Freshness: 0.9,
		AIR: map[string]string{"criticality": "deny", "predicted_stage": "deny", "freshness": "deny"},
	}
	allowed := grammar.Task{
		TaskID: "y", Criticality: "crit", PredictedStage: "hold", Freshness: 0.9,
		AIR: map[string]string{"criticality": "ok", "predicted_stage": "ok", "freshness": "ok"},
	}
	m := New("R").FoldTasks([]grammar.Task{denied, allowed}, false)

	// #5 — blockedBreakdown excludes the denied task's classification on air, counts both off air.
	m.AIR = true
	if h, r := m.blockedBreakdown([]int{0, 1}); h+r != 1 {
		t.Fatalf("on air a denied predicted_stage must not be classified: hold=%d risk=%d (want total 1)", h, r)
	}
	m.AIR = false
	if h, r := m.blockedBreakdown([]int{0, 1}); h+r != 2 {
		t.Fatalf("off air both classify: hold=%d risk=%d (want 2)", h, r)
	}

	// #6 — the throughput held tally drops the denied item on air (would otherwise read "2 held").
	m.AIR = true
	if strings.Contains(m.coordinatorThroughputLine(120), "2 held") {
		t.Fatalf("on air the held tally must not count a denied-criticality/predicted task:\n%s", m.coordinatorThroughputLine(120))
	}
}
