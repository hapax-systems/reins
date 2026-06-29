package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// The classification show-WHY rail (affordance-loop seed): the event context exposes the live signal
// DRIVING the focused row — importance (the DOI/score allocator) → the state classification + the
// relation. AIR-safe: a denied score redacts in the rail (never the number).
func TestEventContextSignalRailShowsWhyAirSafe(t *testing.T) {
	m := New("R").Fold([]grammar.Event{
		{TS: "t0", Kind: "note", Subject: "a", Score: 0.52, AIR: map[string]string{"ts": "ok", "kind": "ok", "score": "ok", "subject": "ok"}},
	}, false)
	m.Page = PageEvents
	m.EFocus = 0

	off := ansi.Strip(m.eventContextPane(80))
	if !strings.Contains(off, "signal") || !strings.Contains(off, "importance 0.52") || !strings.Contains(off, "related") {
		t.Fatalf("the show-WHY signal rail must expose importance + relation:\n%s", off)
	}

	m.AIR = true
	m.Events[0].AIR = map[string]string{"ts": "ok", "kind": "ok"} // score DENIED on air
	on := ansi.Strip(m.eventContextPane(80))
	if strings.Contains(on, "0.52") {
		t.Fatalf("on air a denied score must not surface in the signal rail:\n%s", on)
	}
	if !strings.Contains(on, "signal") {
		t.Fatalf("the signal rail must still render (structurally) on air:\n%s", on)
	}
}
