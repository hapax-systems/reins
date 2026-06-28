package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// Audit wzjr5kfq7, LEAK 1 (count-aggregation): splitSessionContext tallies "%d fail" over the session's
// related events by classifying ev.Kind. A denied-kind fail event must NOT be counted on air — the
// magnitude would disclose the redacted kind. Mirrors the eventContextPane / intake-lane siblings.
func TestSplitSessionContextFailCountIsAirSafe(t *testing.T) {
	sess := grammar.Session{Role: "cx-p0", AIR: map[string]string{"role": "ok"}}
	failEv := grammar.Event{Actor: "cx-p0", Kind: "deploy.fail", AIR: map[string]string{"kind": "deny", "actor": "ok"}}
	m := New("R").FoldSessions([]grammar.Session{sess}, false).Fold([]grammar.Event{failEv}, false)
	m.Page = PageEvents

	m.AIR = false
	if !strings.Contains(ansi.Strip(m.splitSessionContext(sess)), "1 fail") {
		t.Fatalf("off air the related fail event counts (1 fail): %q", m.splitSessionContext(sess))
	}
	m.AIR = true
	if strings.Contains(ansi.Strip(m.splitSessionContext(sess)), "fail") {
		t.Fatalf("on air a denied-kind fail must not be counted: %q", m.splitSessionContext(sess))
	}
}

// Audit wzjr5kfq7, LEAK 2 (conditional-presence): fallbackSplitSessionContext selects the blocker
// branch on a raw non-empty Blocker. On air with blocker denied it must fall through (like a
// blocker-empty session) — diverting to the redacted ▒▒▒ would disclose that a blocker is present.
func TestFallbackSplitContextBlockerPresenceIsAirSafe(t *testing.T) {
	blocked := grammar.Session{Blocker: "stale_relay", State: "active", AIR: map[string]string{"state": "ok"}} // blocker DENIED
	clear := grammar.Session{Blocker: "", State: "active", AIR: map[string]string{"state": "ok"}}
	on := ansi.Strip(fallbackSplitSessionContext(blocked, true))
	onClear := ansi.Strip(fallbackSplitSessionContext(clear, true))
	if on != onClear {
		t.Fatalf("on air a denied blocker must not divert the branch (presence leak): blocked=%q clear=%q", on, onClear)
	}
}

// Audit wzjr5kfq7, LEAK 3 (count-aggregation): yardFleetCounts increments f.stalled from the raw
// Stalled bool; the hide-guard gates readiness/state/platform but NOT stalled, so a denied-stalled
// session still leaks through the per-class count on air. Mirrors the fleet-matrix sibling.
func TestYardFleetCountsStalledIsAirSafe(t *testing.T) {
	s := grammar.Session{Stalled: true, Readiness: "green", State: "active", Platform: "claude",
		AIR: map[string]string{"readiness": "ok", "state": "ok", "platform": "ok"}} // stalled DENIED
	m := New("R").FoldSessions([]grammar.Session{s}, false)

	m.AIR = false
	if got := m.yardFleetCounts().stalled; got != 1 {
		t.Fatalf("off air a stalled session counts, got stalled=%d", got)
	}
	m.AIR = true
	if got := m.yardFleetCounts().stalled; got != 0 {
		t.Fatalf("on air a denied-stalled session must not be counted, got stalled=%d", got)
	}
}
