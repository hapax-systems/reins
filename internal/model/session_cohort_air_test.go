package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// sessionConstraints derives lane-state labels from raw fields; the mere PRESENCE of a label
// discloses the redacted readiness/state/blocker/stalled value (a back-door presence leak).
func TestSessionConstraintsAreAirSafe(t *testing.T) {
	s := grammar.Session{
		Readiness: "claim", State: "offline", Blocker: "merge-wedge", Stalled: true,
		RelayAgeS: 9999, ClaimedTask: "SECRET-TASK",
		AIR: map[string]string{"role": "ok"}, // every sensitive field default-denied
	}
	on := strings.Join(sessionConstraints(s, true), " | ")
	for _, leak := range []string{
		"claim-ready", "stale relay", "no live session surface",
		"needs operator attention", "merge-wedge",
	} {
		if strings.Contains(on, leak) {
			t.Fatalf("sessionConstraints leaked the denied derived constraint %q on air: %s", leak, on)
		}
	}
	if !strings.Contains(on, "no visible lane constraint") {
		t.Fatalf("the constraints block should fall through to the safe default: %s", on)
	}
	off := strings.Join(sessionConstraints(s, false), " | ")
	if !strings.Contains(off, "claim-ready") {
		t.Fatalf("off air the derived constraint renders: %s", off)
	}
}

// sessionConstraintPane's "fleet context" tally (claim/stale/off/stalled) classifies every session
// by raw readiness/state — a denied session must not be counted into the per-class count on air.
func TestSessionConstraintPaneFleetTallyIsAirSafe(t *testing.T) {
	denied := grammar.Session{Role: "a", Readiness: "claim", AIR: map[string]string{"role": "ok", "readiness": "deny"}}
	allowed := grammar.Session{Role: "b", Readiness: "claim", AIR: map[string]string{"role": "ok", "readiness": "ok"}}
	m := New("R").FoldSessions([]grammar.Session{denied, allowed}, false)

	m.AIR = true
	out := ansi.Strip(m.sessionConstraintPane(80))
	if !strings.Contains(out, "1 ready") || strings.Contains(out, "2 ready") {
		t.Fatalf("on air the fleet claim tally must exclude the denied session (want 1 ready):\n%s", out)
	}
	m.AIR = false
	if !strings.Contains(ansi.Strip(m.sessionConstraintPane(80)), "2 ready") {
		t.Fatalf("off air both sessions count (2 ready)")
	}
}

// sessionSlackRows' "fleet claim:N · stale:N · off:N" tally must not count denied sessions on air.
func TestSessionSlackRowsFleetTallyIsAirSafe(t *testing.T) {
	denied := grammar.Session{Role: "a", Readiness: "claim", AIR: map[string]string{"role": "ok", "readiness": "deny"}}
	allowed := grammar.Session{Role: "b", Readiness: "claim", AIR: map[string]string{"role": "ok", "readiness": "ok"}}
	m := New("R").FoldSessions([]grammar.Session{denied, allowed}, false)

	m.AIR = true
	out := strings.Join(m.sessionSlackRows(80), " ")
	if !strings.Contains(out, "claim:1") || strings.Contains(out, "claim:2") {
		t.Fatalf("on air the denied-readiness session must be excluded from the claim tally (want claim:1): %s", out)
	}
	m.AIR = false
	if !strings.Contains(strings.Join(m.sessionSlackRows(80), " "), "claim:2") {
		t.Fatalf("off air both count (claim:2)")
	}
}

// sessionRouteBindingSummary's "bound evidence:N" counts route_id presence + tallies binding state;
// a denied route_id/route_binding_state must not be counted on air.
func TestSessionRouteBindingSummaryIsAirSafe(t *testing.T) {
	denied := grammar.Session{Role: "a", RouteID: "r1", RouteBindingState: "bound",
		AIR: map[string]string{"role": "ok", "route_id": "deny", "route_binding_state": "deny"}}
	allowed := grammar.Session{Role: "b", RouteID: "r2", RouteBindingState: "bound",
		AIR: map[string]string{"role": "ok", "route_id": "ok", "route_binding_state": "ok"}}
	m := New("R").FoldSessions([]grammar.Session{denied, allowed}, false)

	m.AIR = true
	out := m.sessionRouteBindingSummary()
	if !strings.Contains(out, "bound evidence:1") || strings.Contains(out, "bound evidence:2") {
		t.Fatalf("on air the denied route_id must not be counted (want evidence:1): %s", out)
	}
	m.AIR = false
	if !strings.Contains(m.sessionRouteBindingSummary(), "bound evidence:2") {
		t.Fatalf("off air both route_ids count (evidence:2)")
	}
}
