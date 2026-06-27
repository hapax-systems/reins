package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// viewVital's "‼ N task-gated" headline counts m.blockedIndices(), which gates a task purely on raw
// criticality/predicted_stage with no AIR check — so a task gated solely by a DENIED field still
// increments the count, disclosing it on air. Must route through the AIR-aware yardBlockedIndices.
func TestVitalBlockedCountHonorsAIR(t *testing.T) {
	denied := grammar.Task{TaskID: "x", Criticality: "crit", PredictedStage: "hold",
		AIR: map[string]string{"task_id": "ok", "stage": "ok", "criticality": "deny", "predicted_stage": "deny"}}
	allowed := grammar.Task{TaskID: "y", Criticality: "crit", PredictedStage: "hold",
		AIR: map[string]string{"task_id": "ok", "stage": "ok", "criticality": "ok", "predicted_stage": "ok"}}
	m := New("R").FoldTasks([]grammar.Task{denied, allowed}, false)

	m.AIR = true
	out := ansi.Strip(m.viewVital(160))
	if !strings.Contains(out, "1 task-gated") || strings.Contains(out, "2 task-gated") {
		t.Fatalf("on air the gated count must exclude the denied task (want 1 task-gated):\n%s", out)
	}
	m.AIR = false
	if !strings.Contains(ansi.Strip(m.viewVital(160)), "2 task-gated") {
		t.Fatalf("off air the full count airs (want 2 task-gated)")
	}
}

// blockedBreakdown's guard only checked predicted_stage; a task gated by a DENIED criticality (with
// predicted_stage ok) still falls through to risk++, disclosing a crit/major task on air.
func TestBlockedBreakdownExcludesCriticalityOnlyDenialOnAir(t *testing.T) {
	critOnly := grammar.Task{TaskID: "z", Criticality: "major", PredictedStage: "run",
		AIR: map[string]string{"criticality": "deny", "predicted_stage": "ok"}}
	m := New("R").FoldTasks([]grammar.Task{critOnly}, false)

	m.AIR = true
	if h, r := m.blockedBreakdown([]int{0}); h+r != 0 {
		t.Fatalf("on air a criticality-only denial must not be classified: hold=%d risk=%d (want 0)", h, r)
	}
	m.AIR = false
	if h, r := m.blockedBreakdown([]int{0}); h != 0 || r != 1 {
		t.Fatalf("off air a major non-hold task is 1 risk: hold=%d risk=%d", h, r)
	}
}

// The window/tab signal counts (page tabs rendered on EVERY page, on air) must not tally denied items.
func TestWindowSignalTasksBlockedIsAirSafe(t *testing.T) {
	denied := grammar.Task{TaskID: "x", Criticality: "crit", PredictedStage: "hold",
		AIR: map[string]string{"task_id": "ok", "stage": "ok", "criticality": "deny", "predicted_stage": "deny"}}
	m := New("R").FoldTasks([]grammar.Task{denied}, false)
	m.AIR = true
	if label, _ := m.windowSignal(PageTasks); strings.Contains(label, "!") {
		t.Fatalf("on air a denied-gated task must not show in the tasks tab count, got %q", label)
	}
	m.AIR = false
	if label, _ := m.windowSignal(PageTasks); !strings.Contains(label, "!1") {
		t.Fatalf("off air the gated task shows !1, got %q", label)
	}
}

func TestWindowSignalSessionsHotTallyIsAirSafe(t *testing.T) {
	hot := grammar.Session{Role: "cx", Readiness: "claim", Blocker: "stale_relay", Attention: 0.88,
		AIR: map[string]string{"role": "ok", "readiness": "deny", "blocker": "deny", "attention": "deny"}}
	m := New("R").FoldSessions([]grammar.Session{hot}, false)
	m.AIR = false
	if label, _ := m.windowSignal(PageSessions); !strings.Contains(label, "!1") {
		t.Fatalf("off air a hot session shows the !hot suffix, got %q", label)
	}
	m.AIR = true
	if label, _ := m.windowSignal(PageSessions); strings.Contains(label, "!") {
		t.Fatalf("on air a denied hot session must not be tallied, got %q", label)
	}
}

func TestWindowSignalReadinessBlockerIsAirSafe(t *testing.T) {
	s := grammar.Session{Role: "cx", Blocker: "merge-wedge",
		AIR: map[string]string{"role": "ok", "blocker": "deny"}}
	m := New("R").FoldSessions([]grammar.Session{s}, false)
	m.AIR = false
	if label, _ := m.windowSignal(PageReadiness); !strings.Contains(label, "!1") {
		t.Fatalf("off air the blocker session counts (want !1), got %q", label)
	}
	m.AIR = true
	if label, _ := m.windowSignal(PageReadiness); strings.Contains(label, "!1") {
		t.Fatalf("on air a denied-blocker session must not count in readiness hot, got %q", label)
	}
}

// coordinatorThroughputLine's "N hot" counts attention>=0.5 with no AIR check (live coordinator).
func TestCoordinatorThroughputHotHonorsAIR(t *testing.T) {
	s := grammar.Session{Role: "cx", Attention: 0.88,
		AIR: map[string]string{"role": "ok", "attention": "deny"}}
	m := New("R").FoldSessions([]grammar.Session{s}, false)
	m.AIR = true
	if strings.Contains(ansi.Strip(m.coordinatorThroughputLine(160)), "1 hot") {
		t.Fatalf("on air a denied-attention session must not count as hot")
	}
	m.AIR = false
	if !strings.Contains(ansi.Strip(m.coordinatorThroughputLine(160)), "1 hot") {
		t.Fatalf("off air the attention-hot session counts")
	}
}

// The coordinator lens-pane Z3 crumb shows criticality·stage·owner; criticality is sensitive
// (redacted everywhere else) and must redact on air.
func TestCoordinatorLensCrumbRedactsCriticality(t *testing.T) {
	task := grammar.Task{TaskID: "x", Criticality: "major", Stage: "S7", Owner: "owner-lane",
		AIR: map[string]string{"task_id": "ok", "stage": "ok", "criticality": "deny", "owner": "deny"}}
	m := New("R").FoldTasks([]grammar.Task{task}, false)
	m.Page = PageCoordinator
	m.AIR = true
	out := ansi.Strip(m.coordinatorLensPane(120, 24))
	if strings.Contains(out, "major·S7") {
		t.Fatalf("the lens crumb leaked the denied criticality value on air:\n%s", out)
	}
}

// criticality is sensitive — the emergent-relation facet allowlist must NOT air its value.
func TestAirSafeFacetWithholdsCriticality(t *testing.T) {
	if airSafeFacet("crit") {
		t.Fatal("criticality value must NOT air — it is redacted everywhere else in the cockpit")
	}
	if !airSafeFacet("stage") || !airSafeFacet("score") {
		t.Fatal("stage/score are structural facets and should air")
	}
}
