package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// E4.3→live: the chat-pane flagship consumes the live FetchTurns read-wire (the turn-receipt page),
// and falls back to the demo fixture HONESTLY (labeled, never silently passing canned data as live).

func TestFoldTurnsLiveReplacesFixture(t *testing.T) {
	m := New("REINS").loadTurns() // seeds the fixture
	if len(m.TurnLadder) == 0 {
		t.Fatal("precondition: fixture should seed a non-empty ladder")
	}
	live := []grammar.Turn{
		{Role: "cc-x", Kind: "user", Summary: "live row one"},
		{Role: "cc-x", Kind: "assistant", Summary: "live row two"},
	}
	m = m.FoldTurns(live, false)
	if len(m.TurnLadder) != 2 || m.TurnLadder[0].Summary != "live row one" {
		t.Fatalf("live turns must replace the fixture; got %d rows", len(m.TurnLadder))
	}
	if m.TurnsDark {
		t.Fatal("a successful live fold must clear TurnsDark")
	}
}

func TestFoldTurnsDarkKeepsFixtureButFlagsIt(t *testing.T) {
	m := New("REINS").loadTurns()
	seeded := len(m.TurnLadder)
	m = m.FoldTurns(nil, true)
	if len(m.TurnLadder) != seeded {
		t.Fatalf("a dark feed must keep the fixture ladder legible; got %d want %d", len(m.TurnLadder), seeded)
	}
	if !m.TurnsDark {
		t.Fatal("a dark feed must set TurnsDark so the source can be labeled honestly")
	}
}

func TestSwitchToTurnsSetsRoleFromFocusedSession(t *testing.T) {
	m := New("REINS")
	m.Sessions = []grammar.Session{{Role: "cc-alpha"}, {Role: "cc-beta"}}
	m.SFocus = 1
	m = m.switchPage(PageSessionTurns)
	if m.TurnRole != "cc-beta" {
		t.Fatalf("entering the chat pane must target the focused lane's role; got %q", m.TurnRole)
	}
}

func TestStaleLiveTurnsNotMislabeledAsFixture(t *testing.T) {
	// live fold, then a transient dark blip: the prior LIVE rows are kept but must be labeled STALE,
	// not "demo fixture" (the honesty mechanism must not misattribute real data to the fixture).
	m := New("REINS").loadTurns()
	m.TurnRole = "cc-x"
	m = m.FoldTurns([]grammar.Turn{{Role: "cc-x", Kind: "user", Summary: "real live row"}}, false)
	if m.TurnsFixture || m.TurnsDark {
		t.Fatal("after a live fold the ladder is live, not fixture/dark")
	}
	m = m.FoldTurns(nil, true) // the feed blips dark; live rows are kept
	if m.TurnLadder[0].Summary != "real live row" {
		t.Fatal("a dark blip must keep the prior live rows")
	}
	if m.TurnsFixture {
		t.Fatal("kept-stale LIVE rows must NOT be flagged as the fixture")
	}
	m.Page = PageSessionTurns
	label := m.turnSourceLabel()
	if strings.Contains(label, "demo fixture") {
		t.Fatalf("stale live data must not be labeled the demo fixture: %q", label)
	}
	if !strings.Contains(label, "stale") {
		t.Fatalf("kept-stale live data must be labeled stale: %q", label)
	}
}

func TestTurnListBodyLabelsSourceHonestly(t *testing.T) {
	m := New("REINS").loadTurns()
	m.Page = PageSessionTurns
	// dark → the body must NOT imply the canned ladder is live; it must say so
	m = m.FoldTurns(nil, true)
	dark := ansi.Strip(m.turnListBody(120, 24))
	if !strings.Contains(strings.ToLower(dark), "fixture") && !strings.Contains(strings.ToLower(dark), "dark") {
		t.Fatalf("a dark turn feed must be labeled as the demo fixture, not shown as live:\n%s", dark)
	}
	// live → the body labels the live role
	m.TurnRole = "cc-alpha"
	m = m.FoldTurns([]grammar.Turn{{Role: "cc-alpha", Kind: "user", Summary: "hi"}}, false)
	live := ansi.Strip(m.turnListBody(120, 24))
	if !strings.Contains(strings.ToLower(live), "live") || !strings.Contains(live, "cc-alpha") {
		t.Fatalf("a live turn feed must label the live role:\n%s", live)
	}
}
