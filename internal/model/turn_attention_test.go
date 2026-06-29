package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// E4.9 synergy 2: across the ladder, the turn that needs the operator (approval awaiting / gate DENY) is
// scored and SURFACED — even when scrolled out of the visible window.
func TestTurnAttentionScoresAndSurfacesTheBlockingTurn(t *testing.T) {
	m := New("R")
	m.TurnLadder = []grammar.Turn{
		{TS: "t0", Kind: "assistant", Summary: "ok"},
		{TS: "t1", Kind: "approval", Gate: "pending"}, // a decision is required
		{TS: "t2", Kind: "tool_result", Gate: "deny"}, // a gate denied
		{TS: "t3", Kind: "assistant"},
	}
	idx, score, _ := m.turnTopAttention()
	if score == 0 || idx < 0 {
		t.Fatalf("an approval/deny turn must score attention; got idx=%d score=%d", idx, score)
	}
	if idx != 1 {
		t.Fatalf("the approval turn (first of the tied-highest) should win; got idx=%d", idx)
	}
	// the window starts at turn 3 (1 row) → turn 1 is ABOVE the fold and must still surface
	ptr := ansi.Strip(m.turnAttentionPointer(60, 3, 1))
	if !strings.Contains(ptr, "ATTENTION") || !strings.Contains(ptr, "above") {
		t.Fatalf("the pointer must surface the off-screen attention turn with its location: %q", ptr)
	}
}

// AIR-safety: a denied gate/kind on air must NOT raise attention — no false alarm, no derived-channel leak.
func TestTurnAttentionAirDeniedNoFalseAlarm(t *testing.T) {
	m := New("R")
	m.AIR = true
	m.TurnLadder = []grammar.Turn{
		{TS: "t0", Kind: "approval", Gate: "deny", AIR: map[string]string{}}, // both DENIED on air
	}
	if _, score, _ := m.turnTopAttention(); score != 0 {
		t.Fatalf("on air a denied gate+kind must not raise attention; got %d", score)
	}
}
