package grammar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func f(v float64) *float64 { return &v }

func bandOf(p EconPartition, cap string) (EconBand, bool) {
	for _, pl := range p.Placements {
		if pl.Cell.Capability == cap {
			return pl.Band, true
		}
	}
	return 0, false
}

// The classifier partitions (task×capability) cells into FRONTIER / DOMINATED / INCOMPARABLE over
// ⟨v̂↑, ĉ↓⟩ — a partial order, never a rank. Equal-on-both cells are mutual peers (both frontier);
// a strictly-covered cell is dominated with its cover relation; an unmeasured-value cell is incomparable.
func TestClassifyEconPartitionsByDominance(t *testing.T) {
	cells := []EconCell{
		{Capability: "glm", ValueStatus: "measured", ValueHat: f(42), CostUSD: f(0.0042)},   // frontier
		{Capability: "codex", ValueStatus: "measured", ValueHat: f(42), CostUSD: f(0.0089)}, // DOMINATED by glm (= v̂, > ĉ)
		{Capability: "fast", ValueStatus: "measured", ValueHat: f(31), CostUSD: f(0.0021)},  // frontier (cheaper, lower v̂ — incomparable to glm)
		{Capability: "agy", ValueStatus: "projected", ValueHat: f(50), CostUSD: f(0.003)},   // INCOMPARABLE (value projected)
		{Capability: "fugu", ValueStatus: "absent", CostUSD: nil},                           // INCOMPARABLE (value+cost absent)
	}
	p := ClassifyEcon(cells, false, nil)

	want := map[string]EconBand{"glm": BandFrontier, "fast": BandFrontier, "codex": BandDominated, "agy": BandIncomparable, "fugu": BandIncomparable}
	for cap, b := range want {
		if got, ok := bandOf(p, cap); !ok || got != b {
			t.Fatalf("%s: band %v (want %v) found=%v", cap, got, b, ok)
		}
	}
	if p.NFront != 2 || p.NDom != 1 || p.NIncomp != 2 {
		t.Fatalf("band counts: front=%d dom=%d incomp=%d (want 2/1/2)", p.NFront, p.NDom, p.NIncomp)
	}
	// the dominated cell carries the honest cover relation (which cell, which axis) — no scalar
	for _, pl := range p.Placements {
		if pl.Cell.Capability == "codex" {
			if len(pl.DominatedBy) != 1 || pl.DominatedBy[0] != "glm" || pl.BindingAxis[0] != "ĉ" {
				t.Fatalf("codex cover relation: by=%v axis=%v (want glm / ĉ)", pl.DominatedBy, pl.BindingAxis)
			}
		}
	}
}

// Honest-when-starved: with NO measured value (the live state — v̂ producer unbuilt), the FRONTIER is
// UNDEFINED (empty) and every cell falls to INCOMPARABLE — never a fabricated ranking.
func TestClassifyEconFrontierUndefinedWhenValueAbsent(t *testing.T) {
	cells := []EconCell{
		{Capability: "codex", ValueStatus: "absent", CostUSD: f(0.012)},
		{Capability: "glm", ValueStatus: "absent", CostUSD: nil},
	}
	p := ClassifyEcon(cells, false, nil)
	if p.NFront != 0 || p.NIncomp != 2 {
		t.Fatalf("value-absent → frontier UNDEFINED, all incomparable: front=%d incomp=%d", p.NFront, p.NIncomp)
	}
}

// AIR derived-channel discipline: if an economic axis is DENIED on air, the partition SEALS — it is NOT
// computed over the denied axis, so band membership cannot disclose the redacted magnitude. Proof: with
// cost denied, the classification is BYTE-IDENTICAL whether cost is high or low.
func TestClassifyEconSealsWhenAxisDeniedOnAir(t *testing.T) {
	costAllow := func(axis string) bool { return axis == "value" } // cost DENIED
	build := func(cost float64) EconPartition {
		return ClassifyEcon([]EconCell{
			{Capability: "glm", ValueStatus: "measured", ValueHat: f(42), CostUSD: f(cost)},
			{Capability: "codex", ValueStatus: "measured", ValueHat: f(42), CostUSD: f(0.0089)},
		}, true, costAllow)
	}
	hi, lo := build(0.99), build(0.0001)
	if !hi.Sealed || hi.NFront != 0 || hi.NIncomp != 2 {
		t.Fatalf("denied cost must SEAL → no frontier, all incomparable: %+v", hi)
	}
	// the denied cost must not steer membership: identical partition regardless of its value
	for i := range hi.Placements {
		if hi.Placements[i].Band != lo.Placements[i].Band || hi.Placements[i].Cell.Capability != lo.Placements[i].Cell.Capability {
			t.Fatalf("a denied cost leaked through the partition: %v vs %v", hi.Placements, lo.Placements)
		}
	}
}

// The scan renders the three named bands with raw data (no scalar/rank), and on air with $cost denied
// the partition SEALS — byte-identical regardless of the withheld magnitudes (the derived-channel floor).
func TestRenderEconPartitionBandsAndAirSeal(t *testing.T) {
	cells := []EconCell{
		{Capability: "glm", ValueStatus: "measured", ValueHat: f(42), CostUSD: f(0.0042)},
		{Capability: "codex", ValueStatus: "measured", ValueHat: f(42), CostUSD: f(0.0089)},
		{Capability: "agy", ValueStatus: "absent", CostUSD: f(0.0033)},
	}
	off := ansi.Strip(RenderEconPartition(ClassifyEcon(cells, false, nil), false, true, 80))
	for _, w := range []string{"FRONTIER", "DOMINATED", "INCOMPARABLE", "not ranked", "glm", "codex", "agy"} {
		if !strings.Contains(off, w) {
			t.Fatalf("off-air partition must show %q:\n%s", w, off)
		}
	}
	for _, scalar := range []string{"0%", "100%", "priority", "rank "} {
		if strings.Contains(off, scalar) {
			t.Fatalf("partition must not show a scalar/rank (%q):\n%s", scalar, off)
		}
	}

	deny := func(axis string) bool { return axis == "value" } // $cost DENIED on air
	mk := func(cost float64) string {
		return ansi.Strip(RenderEconPartition(ClassifyEcon([]EconCell{
			{Capability: "glm", ValueStatus: "measured", ValueHat: f(42), CostUSD: f(cost)},
			{Capability: "codex", ValueStatus: "measured", ValueHat: f(42), CostUSD: f(0.0089)},
		}, true, deny), true, false, 80))
	}
	hi, lo := mk(0.99), mk(0.0001)
	if !strings.Contains(hi, "SEALED") || strings.Contains(hi, "0.99") {
		t.Fatalf("on air with $cost denied the partition must SEAL and never show the cost:\n%s", hi)
	}
	if hi != lo {
		t.Fatalf("on air a denied $cost must not steer the rendered partition (byte-identical):\nHI:\n%s\nLO:\n%s", hi, lo)
	}
}
