package model

import (
	"strings"
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

// E4.9 synergy-1 (cross-session yank → inject) is EMERGENT, not a feature to build: the yank kill-ring
// (m.Ring) is GLOBAL and cycleLane (the lane-rail [←/→] focus move) does not clear it, so a value
// yanked while coordinating lane A still resolves via {{ring.0}} after focus moves to lane B — the
// operator can carry one lane's datum into another lane's composed command. This locks the emergent
// capability against a future cycleLane that might reset more state than it should.
func TestCrossLaneYankRingSurvivesLaneCycle(t *testing.T) {
	m := New("R").FoldSessions([]grammar.Session{
		{Role: "cx-a", AIR: map[string]string{"role": "ok"}},
		{Role: "cx-b", AIR: map[string]string{"role": "ok"}},
	}, false)
	m.Page = PageSessionTurns
	m.TurnRole = "cx-a"
	// simulate a yank performed while coordinating lane A
	m.Ring = pushRing(m.Ring, RingEntry{Value: "from-lane-a", Field: "summary", Page: "turns", AIR: "ok"})

	m = m.cycleLane(1) // [→] move the coordinating focus to the next lane
	if m.TurnRole != "cx-b" {
		t.Fatalf("precondition: cycle must move focus to cx-b, got %q", m.TurnRole)
	}
	if got := m.resolveTemplate("inject {{ring.0}}"); !strings.Contains(got, "from-lane-a") {
		t.Fatalf("cross-lane: {{ring.0}} must still resolve to lane A's yank after cycling to lane B: %q", got)
	}
	if len(m.Ring) == 0 || m.Ring[0].Value != "from-lane-a" {
		t.Fatalf("cycleLane must not clear the global yank ring (emergent cross-lane carry)")
	}
}
