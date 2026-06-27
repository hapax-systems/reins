package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// The :dispatch page composes via the view-algebra (ledger │ measurement) and carries the
// measurement-first honesty end to end: a null cost renders UNMEASURED (never $0.00), the utilization
// rollup shows, and the cc_task id redacts on air.
func TestDispatchPageRendersLedgerAndMeasurement(t *testing.T) {
	cost := 0.0123
	m := New("R")
	m.Page = PageDispatch
	m.Width, m.Height = 170, 30
	m.DispatchRecords = []grammar.DispatchRecord{
		{Capability: "glm-via-cc", RouteID: "claude.full", CCTask: "cc-task-secret", Launched: true, DispatchLatencyMs: 1180},
		{Capability: "codex.full", RouteID: "codex.spark.full", CCTask: "t2", Launched: true, DispatchLatencyMs: 940, CostUSD: &cost},
	}

	out := ansi.Strip(m.View())
	if strings.Contains(out, "$0.00") {
		t.Fatalf("a null cost must not render $0.00 on the dispatch page:\n%s", out)
	}
	if !strings.Contains(out, "UNMEASURED") {
		t.Fatalf("the dispatch page must show UNMEASURED for the null cost:\n%s", out)
	}
	if !strings.Contains(out, "UTILIZATION") {
		t.Fatalf("the dispatch page must show the utilization rollup (secondary):\n%s", out)
	}
	if !strings.Contains(out, "glm-via-cc") {
		t.Fatalf("the dispatch page must show the ledger rows (primary):\n%s", out)
	}

	m.AIR = true
	on := ansi.Strip(m.View())
	if strings.Contains(on, "cc-task-secret") {
		t.Fatalf("the dispatch page must redact cc_task on air:\n%s", on)
	}
	if !strings.Contains(on, "glm-via-cc") {
		t.Fatalf("the capability (structural) still airs:\n%s", on)
	}
}

// An empty ledger reads honestly — the page says so rather than faking activity.
func TestDispatchPageEmptyLedgerIsHonest(t *testing.T) {
	m := New("R")
	m.Page = PageDispatch
	m.Width, m.Height = 170, 30
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "ledger empty") {
		t.Fatalf("an empty dispatch ledger must say so:\n%s", out)
	}
}
