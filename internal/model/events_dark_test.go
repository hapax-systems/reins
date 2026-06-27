package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Regression for the codex-fugu events-migration review: a DARK events page (with the legacy split
// flag on, at wide width) must stay ALGEBRA-OWNED — it must NOT fall back into the legacy
// session-frozen split (which would also re-bind templates/yank to the session source via
// commandSelectionPage). It renders its own dark-hint body instead.
func TestDarkEventsStayAlgebraOwnedNoLegacySplit(t *testing.T) {
	m := New("R")
	m.Page = PageEvents
	m.EventsDark = true
	m.EventsError = "feed-down-marker"
	m.SplitContext = true
	m.Width, m.Height = 220, 40

	if m.splitContextActive() {
		t.Fatal("dark events must be algebra-owned (splitContextActive()==false), not the legacy split")
	}
	if m.commandSelectionPage() != PageEvents {
		t.Fatalf("dark events templates/yank must bind to PageEvents, not the session source")
	}
	v := ansi.Strip(m.View())
	if strings.Contains(v, "split sessions") || strings.Contains(v, "[j/k]source") {
		t.Fatalf("dark events must NOT render the legacy session-frozen split:\n%s", v)
	}
	if !strings.Contains(v, "feed-down-marker") {
		t.Fatalf("dark events must still show the dark-reason disclosure:\n%s", v)
	}
}

// The vital strip must not show the legacy "split:wide" (queued) chip on a migrated (algebra-owned)
// page — the page is ALREADY split, so the chip would lie.
func TestMigratedEventsSplitAffordanceIsHonest(t *testing.T) {
	m := New("R")
	m.Page = PageEvents
	m.SplitContext = true
	m.Width, m.Height = 220, 40
	if strings.Contains(ansi.Strip(m.viewVital(m.Width)), "split:wide") {
		t.Fatal("migrated events must not show the misleading legacy split:wide chip")
	}
}
