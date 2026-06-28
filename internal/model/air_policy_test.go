package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/config"
	"github.com/hapax-systems/reins/internal/grammar"
)

// Operator AIR bar (2026-06-28): "air structural, DENY $cost". The structural skeleton (severity,
// trajectory, served model, latency, tokens, ids, gate) airs on the livestream per session-pane design
// §2; the financial $cost denies. This pins the live default allowlist to that bar.
func TestAirPolicyAirsStructuralDeniesCost(t *testing.T) {
	al := config.Defaults().AIRAllowlist
	has := func(f string) bool {
		for _, x := range al {
			if x == f {
				return true
			}
		}
		return false
	}
	for _, f := range []string{"criticality", "predicted_stage", "prior_stage", "model", "latency_ms", "total_tok", "trace_id", "gate"} {
		if !has(f) {
			t.Fatalf("structural field %q must air on the livestream (operator: air structural)", f)
		}
	}
	if has("cost") {
		t.Fatalf("$cost must DENY on air (operator confidentiality bar: deny $cost)")
	}
}

// At the render, a trace on air shows its structural skeleton (latency/tokens) but DENIES $cost.
func TestTraceRowOnAirDeniesCostAirsStructural(t *testing.T) {
	tr := grammar.Trace{TS: "t", TraceID: "x", Model: "claude-opus-4", Cost: 0.0123, LatencyMs: 1200, TotalTok: 3000,
		AIR: map[string]string{"ts": "ok", "trace_id": "ok", "model": "ok", "latency_ms": "ok", "total_tok": "ok"}} // cost DENIED
	on := ansi.Strip(grammar.RenderTraceRow(tr, true))
	if strings.Contains(on, "0.0123") {
		t.Fatalf("on air the $cost must deny:\n%s", on)
	}
	if !strings.Contains(on, "3000") {
		t.Fatalf("on air the structural token magnitude must air:\n%s", on)
	}
}
