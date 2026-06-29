package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// The :dispatch MEASUREMENT surface is the (task×capability) value readout — dev2's invariant is that it
// renders an HONEST measured-vs-absent partial order, NEVER a display scalar and NEVER 0/100% for an
// absent value (the quota_spend_ledger 0-on-missing trap). It must NOT scalar-fold (no doi.Fold). This
// guard pins that: with partial data it shows gap-counts (N/M priced · rest UNMEASURED), so a future
// "universalize DOI" that pushed the scalar fold onto this surface fails loudly. (Both 2026-06-28 digs
// flagged this as the cardinal contradiction; the dispatch path is scalar-free today — keep it so.)
func TestDispatchMeasurementIsHonestPartialOrderNotScalar(t *testing.T) {
	cost := 0.012
	qual := "pass"
	m := New("R")
	m.DispatchRecords = []grammar.DispatchRecord{
		{Capability: "codex", CostUSD: &cost, QualitySignal: &qual}, // measured
		{Capability: "glm"},  // cost + quality ABSENT (nil), not 0/asserted
		{Capability: "fugu"}, // ABSENT
	}
	out := ansi.Strip(m.dispatchMeasurementPane(120))

	if !strings.Contains(out, "1/3 priced") || !strings.Contains(out, "UNMEASURED") {
		t.Fatalf("dispatch must show honest measured-vs-absent gap-counts (dev2 value_status), got:\n%s", out)
	}
	if !strings.Contains(out, "1/3 verified") || !strings.Contains(out, "asserted") {
		t.Fatalf("dispatch must name the quality gap honestly, not assert it:\n%s", out)
	}
	for _, scalar := range []string{"0%", "100%", "$0.00", "score 0", "priority"} {
		if strings.Contains(out, scalar) {
			t.Fatalf("dispatch must NOT collapse to a scalar / 0-on-missing (%q):\n%s", scalar, out)
		}
	}
}
