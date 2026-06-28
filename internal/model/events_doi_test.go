package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// E1 — the DOI fold lights up IN-LANE in the events pane. When there are more events than cells, the
// pane no longer silently drops the OLDEST (the chronological-tail behaviour); it SELECTS by
// Degree-of-Interest (importance − distance-from-focus) so a high-interest OLD event is summoned over
// more-recent-but-dull ones, and it marks the dropped remainder with an honest "+N folded" line.
// Selection decides WHAT is shown; the reading order stays chronological.
func TestEventsListDoiFoldRetainsHighInterestOverRecencyAndMarksFolded(t *testing.T) {
	m := New("REINS").Fold([]grammar.Event{
		{TS: "t0", Kind: "note", Subject: "beacon", Score: 0.95, AIR: map[string]string{"ts": "ok", "kind": "ok", "score": "ok", "subject": "ok"}},
		{TS: "t1", Kind: "note", Subject: "mid1", Score: 0.10, AIR: map[string]string{"ts": "ok", "kind": "ok", "score": "ok", "subject": "ok"}},
		{TS: "t2", Kind: "note", Subject: "mid2", Score: 0.10, AIR: map[string]string{"ts": "ok", "kind": "ok", "score": "ok", "subject": "ok"}},
		{TS: "t3", Kind: "note", Subject: "mid3", Score: 0.10, AIR: map[string]string{"ts": "ok", "kind": "ok", "score": "ok", "subject": "ok"}},
		{TS: "t4", Kind: "note", Subject: "mid4", Score: 0.10, AIR: map[string]string{"ts": "ok", "kind": "ok", "score": "ok", "subject": "ok"}},
		{TS: "t5", Kind: "note", Subject: "now", Score: 0.10, AIR: map[string]string{"ts": "ok", "kind": "ok", "score": "ok", "subject": "ok"}},
	}, false)
	m.EFocus = 5 // the operator is attending to the newest event

	// h = 6 → visible budget = 4, one cell reserved for the marker ⇒ 3 rows shown, 3 folded.
	out := ansi.Strip(m.eventsListBody(120, 6))

	// the high-interest OLDEST event is retained — the discriminating proof of DOI over chronological
	// tail: a tail-drop would shed "beacon" first; DOI summons it OVER the more-recent dull rows.
	if !strings.Contains(out, "beacon") {
		t.Fatalf("DOI must summon the high-interest oldest event over recency:\n%s", out)
	}
	// the focused row is always retained even at low score
	if !strings.Contains(out, "now") {
		t.Fatalf("the focused row must always be retained:\n%s", out)
	}
	// the more-recent-but-dull middling rows are the ones that fold
	for _, dull := range []string{"mid1", "mid2", "mid3"} {
		if strings.Contains(out, dull) {
			t.Fatalf("a dull mid row %q should have folded under the high-interest beacon:\n%s", dull, out)
		}
	}
	// honest "+N folded" marker for the dropped remainder (here 3)
	if !strings.Contains(out, "folded") || !strings.Contains(out, "3") {
		t.Fatalf("the dropped remainder must be marked '+3 folded':\n%s", out)
	}
}

// Steady state — when everything fits, every event renders and NO folded marker appears (recede).
func TestEventsListNoFoldedMarkerWhenAllFit(t *testing.T) {
	m := New("REINS").Fold([]grammar.Event{
		{TS: "t0", Kind: "note", Subject: "a", Score: 0.5, AIR: map[string]string{"ts": "ok", "kind": "ok", "score": "ok", "subject": "ok"}},
		{TS: "t1", Kind: "note", Subject: "b", Score: 0.5, AIR: map[string]string{"ts": "ok", "kind": "ok", "score": "ok", "subject": "ok"}},
	}, false)
	out := ansi.Strip(m.eventsListBody(120, 20))
	if strings.Contains(out, "folded") {
		t.Fatalf("nothing folds when all events fit — the marker must recede:\n%s", out)
	}
}

// AIR derived-channel discipline: when an event's score is DENIED on air, it must not steer selection.
// Otherwise the ORDER/membership of the visible set discloses the redacted score (the aggregation-key
// leak class). Proof: two models identical but for a DENIED score (0.95 vs 0.0) must render byte-for-
// byte the same — the hidden score changed nothing observable.
func TestEventsListDoiFoldDoesNotLeakDeniedScoreViaSelection(t *testing.T) {
	build := func(beaconScore float64) Model {
		mm := New("REINS").Fold([]grammar.Event{
			{TS: "t0", Kind: "note", Subject: "beacon", Score: beaconScore, AIR: map[string]string{"ts": "ok", "kind": "ok"}},
			{TS: "t1", Kind: "note", Subject: "mid1", Score: 0.10, AIR: map[string]string{"ts": "ok", "kind": "ok"}},
			{TS: "t2", Kind: "note", Subject: "mid2", Score: 0.10, AIR: map[string]string{"ts": "ok", "kind": "ok"}},
			{TS: "t3", Kind: "note", Subject: "mid3", Score: 0.10, AIR: map[string]string{"ts": "ok", "kind": "ok"}},
			{TS: "t4", Kind: "note", Subject: "mid4", Score: 0.10, AIR: map[string]string{"ts": "ok", "kind": "ok"}},
			{TS: "t5", Kind: "note", Subject: "now", Score: 0.10, AIR: map[string]string{"ts": "ok", "kind": "ok"}},
		}, false)
		mm.AIR = true
		mm.EFocus = 5
		return mm
	}
	hi := ansi.Strip(build(0.95).eventsListBody(120, 6))
	lo := ansi.Strip(build(0.00).eventsListBody(120, 6))
	if hi != lo {
		t.Fatalf("a DENIED score must not steer the visible set (derived-channel leak):\nHI:\n%s\nLO:\n%s", hi, lo)
	}
}
