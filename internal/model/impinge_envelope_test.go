package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// E4.4 (preview-only, §4): the impinge affordances NAME the governed gate_output event each decision
// maps to (impinge(<verb>)) — the "x → gate_output" legibility a bare verb lacks — and stay NOT wired
// (egress permanently always-gate; never mint). The preview is GENERIC (the decision class only); the
// turn body never appears, on air or off.
func TestTurnImpingeNamesGovernedEventAndStaysNotWired(t *testing.T) {
	turn := func() grammar.Turn {
		return grammar.Turn{Kind: "approval", Role: "cx-secret", Summary: "approve dropping the prod table", AIR: map[string]string{}}
	}
	m := New("REINS")

	off := ansi.Strip(m.turnImpingeAffordances(turn()))
	if !strings.Contains(off, "gate_output") || !strings.Contains(off, "impinge(accept)") {
		t.Fatalf("the impinge preview must name the governed event for the primary decision:\n%s", off)
	}
	if !strings.Contains(off, "NOT wired") {
		t.Fatalf("the gate_output preview must be NOT wired:\n%s", off)
	}
	if strings.Contains(off, "prod table") {
		t.Fatalf("the preview must be GENERIC — the turn body must never appear:\n%s", off)
	}

	m.AIR = true
	on := ansi.Strip(m.turnImpingeAffordances(turn()))
	if !strings.Contains(on, "impinge(accept)") || !strings.Contains(on, "NOT wired") {
		t.Fatalf("on air the governed shape survives and stays NOT wired:\n%s", on)
	}
	if strings.Contains(on, "prod table") {
		t.Fatalf("the turn body must never appear in the preview on air:\n%s", on)
	}
}
