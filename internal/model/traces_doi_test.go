package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

func traceAIROK() map[string]string {
	return map[string]string{"ts": "ok", "trace_id": "ok", "model": "ok", "cost": "ok", "latency_ms": "ok", "total_tok": "ok"}
}

// The traces feed folds through the SAME DOI mechanism as events (framework: every feed pane folds
// identically). An expensive/slow OLD trace is summoned over routine-but-recent ones instead of being
// dropped off the top, and the dropped remainder is marked honestly.
func TestTraceDoiSelectionSurfacesExpensiveOverRecency(t *testing.T) {
	traces := []grammar.Trace{
		{TS: "t0", TraceID: "beacon", Cost: 0.50, LatencyMs: 3000, AIR: traceAIROK()},
		{TS: "t1", TraceID: "m1", Cost: 0.001, LatencyMs: 50, AIR: traceAIROK()},
		{TS: "t2", TraceID: "m2", Cost: 0.001, LatencyMs: 50, AIR: traceAIROK()},
		{TS: "t3", TraceID: "m3", Cost: 0.001, LatencyMs: 50, AIR: traceAIROK()},
		{TS: "t4", TraceID: "m4", Cost: 0.001, LatencyMs: 50, AIR: traceAIROK()},
		{TS: "t5", TraceID: "now", Cost: 0.001, LatencyMs: 50, AIR: traceAIROK()},
	}
	m := New("R").FoldTraces(traces, false)
	m.TFocus = 5 // attending to the newest

	order, folded := m.traceDoiSelection(3)
	has := func(idx int) bool {
		for _, o := range order {
			if o == idx {
				return true
			}
		}
		return false
	}
	if !has(0) {
		t.Fatalf("the expensive oldest trace must be summoned over recency, order=%v", order)
	}
	if !has(5) {
		t.Fatalf("the focused trace must be pinned visible, order=%v", order)
	}
	if has(1) || has(2) || has(3) {
		t.Fatalf("routine recent traces should fold under the expensive beacon, order=%v", order)
	}
	if folded != 3 {
		t.Fatalf("3 traces must fold (6 − budget 3), got %d", folded)
	}
}

// AIR derived-channel discipline: a denied cost/latency must NOT steer the visible set — else the
// membership discloses the redacted magnitude. Two feeds identical but for a DENIED cost must select
// the same set.
func TestTraceDoiSelectionDoesNotLeakDeniedCost(t *testing.T) {
	build := func(beaconCost float64) Model {
		air := map[string]string{"ts": "ok", "trace_id": "ok"} // cost + latency DENIED
		mm := New("R").FoldTraces([]grammar.Trace{
			{TS: "t0", TraceID: "beacon", Cost: beaconCost, LatencyMs: 9000, AIR: air},
			{TS: "t1", TraceID: "m1", Cost: 0.001, LatencyMs: 50, AIR: air},
			{TS: "t2", TraceID: "m2", Cost: 0.001, LatencyMs: 50, AIR: air},
			{TS: "t3", TraceID: "m3", Cost: 0.001, LatencyMs: 50, AIR: air},
			{TS: "t4", TraceID: "m4", Cost: 0.001, LatencyMs: 50, AIR: air},
			{TS: "t5", TraceID: "now", Cost: 0.001, LatencyMs: 50, AIR: air},
		}, false)
		mm.AIR = true
		mm.TFocus = 5
		return mm
	}
	hiOrder, _ := build(0.99).traceDoiSelection(3)
	loOrder, _ := build(0.0).traceDoiSelection(3)
	if len(hiOrder) != len(loOrder) {
		t.Fatalf("a denied cost must not change selection size: %v vs %v", hiOrder, loOrder)
	}
	for i := range hiOrder {
		if hiOrder[i] != loOrder[i] {
			t.Fatalf("a denied cost must not steer the visible set: %v vs %v", hiOrder, loOrder)
		}
	}
}

// The traces feed gets the same honest "+N folded" marker when it overflows the cell budget.
func TestTracesListBodyMarksFoldedRemainder(t *testing.T) {
	traces := make([]grammar.Trace, 6)
	for i := range traces {
		traces[i] = grammar.Trace{TS: "t", TraceID: "tr", Cost: 0.001, AIR: traceAIROK()}
	}
	m := New("R").FoldTraces(traces, false)
	m.TFocus = 5
	out := ansi.Strip(m.tracesListBody(120, 6)) // visible 4 → reserve → 3 shown, 3 folded
	if !strings.Contains(out, "folded") {
		t.Fatalf("an overflowing traces feed must mark the folded remainder:\n%s", out)
	}
}
