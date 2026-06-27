package model

import (
	"strings"
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

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
