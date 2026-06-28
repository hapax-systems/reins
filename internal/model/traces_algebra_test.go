package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// Inc 3 TRANSFORM — PageTraces migrates onto the view-algebra as a SELF-ANCHORED page (own TFocus),
// ABOLISHING the legacy session-frozen reference split (sessions │ trace feed). The primary IS the
// trace list; j moves the trace row (TFocus) NATIVELY; the secondary is the focused trace's spend +
// latency breakdown (authored — the narrow list clips trailing columns); the connector is EMERGENT.

func tracesAlgebraModel() Model {
	m := New("REINS").FoldTraces([]grammar.Trace{
		{TS: "2026-06-26T12:00:00Z", TraceID: "trace-alpha", Model: "claude-opus-4", PromptTok: 100, CompletionTok: 50, TotalTok: 150, Cost: 0.012345, LatencyMs: 2500,
			AIR: map[string]string{"ts": "ok", "trace_id": "ok", "model": "ok", "latency_ms": "ok", "cost": "ok", "total_tok": "ok"}},
		{TS: "2026-06-26T12:01:00Z", TraceID: "trace-beta", Model: "claude-haiku-4", PromptTok: 20, CompletionTok: 10, TotalTok: 30, Cost: 0.000123, LatencyMs: 300,
			AIR: map[string]string{"ts": "ok", "trace_id": "ok", "model": "ok", "latency_ms": "ok", "cost": "ok", "total_tok": "ok"}},
	}, false)
	m.Width, m.Height, m.Page = 180, 44, PageTraces
	m.TFocus = 0
	return m
}

func TestTracesComposesViaAlgebraNativeBinding(t *testing.T) {
	m := tracesAlgebraModel()
	if !m.composesViaAlgebra() {
		t.Fatal("PageTraces must compose via the view-algebra (only-split)")
	}
	if m.splitContextActive() {
		t.Fatal("a migrated page must NOT be session-frozen (splitContextActive==false)")
	}
	if m.commandSelectionPage() != PageTraces {
		t.Fatalf("templates/yank must bind to PageTraces, got page %d", m.commandSelectionPage())
	}
}

func TestTracesJMovesTraceRowNatively(t *testing.T) {
	m := tracesAlgebraModel()
	before := m.TFocus
	m = step(m, "j")
	if m.TFocus != before+1 {
		t.Fatalf("j must move the trace row natively (TFocus %d→%d)", before, m.TFocus)
	}
}

func TestTracesFocusTemplateBindsToRow(t *testing.T) {
	m := tracesAlgebraModel()
	got := m.resolveTemplate("trace {{focus}}")
	if !strings.Contains(got, "trace-alpha") {
		t.Fatalf("{{focus}} must resolve to the focused trace_id, got %q", got)
	}
	if strings.Contains(got, "{{focus}}") {
		t.Fatalf("{{focus}} must not render literally, got %q", got)
	}
	m = step(m, "j")
	if got := m.resolveTemplate("{{focus}}"); !strings.Contains(got, "trace-beta") {
		t.Fatalf("{{focus}} must track the moved row, got %q", got)
	}
}

func TestTracesFocusTemplateIsAirSafe(t *testing.T) {
	m := New("REINS").FoldTraces([]grammar.Trace{
		{TS: "2026-06-26T12:00:00Z", TraceID: "SECRET-TRACE", Model: "m", LatencyMs: 100,
			AIR: map[string]string{"ts": "ok", "trace_id": "deny", "model": "ok", "latency_ms": "ok"}},
	}, false)
	m.Width, m.Height, m.Page = 180, 44, PageTraces
	m.AIR = true
	if got := m.resolveTemplate("{{focus}}"); strings.Contains(got, "SECRET-TRACE") {
		t.Fatalf("on air {{focus}} must not leak a denied trace_id, got %q", got)
	}
}

func TestTracesViewIsAlgebraSplitWithDetailSecondary(t *testing.T) {
	m := tracesAlgebraModel()
	v := ansi.Strip(m.View())
	if strings.Contains(v, "split sessions") || strings.Contains(v, "[j/k]source") {
		t.Fatalf("migrated traces must NOT render the legacy session-frozen split:\n%s", v)
	}
	if !strings.Contains(v, "TRACE DETAIL") {
		t.Fatalf("the secondary must show the focused trace detail:\n%s", v)
	}
	// the trailing cost clips in the narrow primary list — it must render in the secondary
	if !strings.Contains(v, "$0.012345") {
		t.Fatalf("the focused trace's cost must render (in the detail secondary):\n%s", v)
	}
}

// The emergent connector must not derive over a denied facet: two traces on the same DENIED model must
// not surface "shares model" on air (traceEntity omits denied facets before relate.Derive).
func TestTracesEmergentRelationOmitsDeniedFacetOnAir(t *testing.T) {
	m := New("REINS").FoldTraces([]grammar.Trace{
		{TS: "t1", TraceID: "a", Model: "secret-model", LatencyMs: 100,
			AIR: map[string]string{"ts": "ok", "trace_id": "ok", "model": "deny", "latency_ms": "ok"}},
		{TS: "t2", TraceID: "b", Model: "secret-model", LatencyMs: 200,
			AIR: map[string]string{"ts": "ok", "trace_id": "ok", "model": "deny", "latency_ms": "ok"}},
	}, false)
	m.Width, m.Height, m.Page, m.TFocus = 180, 44, PageTraces, 0

	m.AIR = false
	if rel := ansi.Strip(m.tracesEmergentRelation()); !strings.Contains(rel, "model") {
		t.Fatalf("off air the shared model IS the relation: %q", rel)
	}
	m.AIR = true
	if rel := ansi.Strip(m.tracesEmergentRelation()); strings.Contains(rel, "model") {
		t.Fatalf("on air a denied model must not appear in the emergent connector relation: %q", rel)
	}
}
