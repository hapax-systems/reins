package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Multi-instance safety (operator 2026-06-28): it must ALWAYS be possible to run more than one Reins at
// once — some ON-AIR (broadcasting, redacted) and some LOCAL (private, cleartext). Two guarantees:
//  1. AIR state is strictly PER-INSTANCE — the m.AIR field, in memory. A second instance going on-air
//     must not flip this instance's lens. (There is NO shared AIR file and NO singleton lock; this test
//     pins the in-memory independence so a regression to shared/persisted AIR fails loudly.)
//  2. Each instance's frame UNMISTAKABLY shows its OWN state, so the operator never captures the wrong
//     terminal for the broadcast: ON-AIR reads "ON-AIR"; LOCAL reads "LOCAL" and never "ON-AIR".
func TestMultiInstanceAIRIsPerInstanceAndUnmistakable(t *testing.T) {
	priv := New("REINS")
	priv.Width, priv.Height = 170, 44 // LOCAL (m.AIR defaults false)
	live := New("REINS")
	live.Width, live.Height = 170, 44
	live.AIR = true // a SEPARATE instance goes on-air

	if priv.AIR {
		t.Fatal("AIR must be per-instance: a second instance going on-air must not flip this one")
	}

	privFrame := ansi.Strip(priv.View())
	liveFrame := ansi.Strip(live.View())

	if !strings.Contains(privFrame, "LOCAL") || strings.Contains(privFrame, "ON-AIR") {
		t.Fatalf("the LOCAL instance must read LOCAL and never ON-AIR:\n%s", privFrame)
	}
	if !strings.Contains(liveFrame, "ON-AIR") {
		t.Fatalf("the on-air instance must unmistakably read ON-AIR:\n%s", liveFrame)
	}
}
