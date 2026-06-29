package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// E0.3: the traces rollup aggregates spend/tokens across the loaded traces; AIR-safe — $cost is denied on
// the livestream (not in the allowlist) so its aggregate SEALS on air, while the allowlisted token
// aggregate still shows.
func TestTraceCostRollupSumsAndSealsCostOnAir(t *testing.T) {
	m := New("R")
	m.Traces = []grammar.Trace{
		{TraceID: "t1", TotalTok: 100, Cost: 0.01, LatencyMs: 200, AIR: map[string]string{"total_tok": "ok", "latency_ms": "ok"}},
		{TraceID: "t2", TotalTok: 300, Cost: 0.03, LatencyMs: 400, AIR: map[string]string{"total_tok": "ok", "latency_ms": "ok"}},
	}
	off := ansi.Strip(m.traceCostRollup(80))
	if !strings.Contains(off, "2 traces") || !strings.Contains(off, "400 tok") || !strings.Contains(off, "$0.0400") {
		t.Fatalf("off-air rollup must sum tokens + cost: %q", off)
	}
	m.AIR = true
	on := ansi.Strip(m.traceCostRollup(80))
	if strings.Contains(on, "0.04") || !strings.Contains(on, "$ ▒▒▒") {
		t.Fatalf("on air the $cost aggregate must SEAL (deny-$cost policy): %q", on)
	}
	if !strings.Contains(on, "400 tok") {
		t.Fatalf("tokens are allowlisted → the token aggregate shows on air: %q", on)
	}
}
