package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

func TestIdentityRosterDedupsAndCountsAndClasses(t *testing.T) {
	m := New("REINS")
	m = m.FoldSessions([]grammar.Session{{Role: "cc-alpha"}, {Role: "cc-beta"}, {Role: "cc-alpha"}}, false)
	m = m.Fold([]grammar.Event{{Actor: "cc-alpha", TS: "t1", Kind: "k", Subject: "s"}, {Actor: "watcher", TS: "t2", Kind: "k", Subject: "s2"}}, false)
	m = m.FoldTasks([]grammar.Task{{TaskID: "x", Owner: "cc-beta"}}, false)

	roster := m.identityRoster()
	byName := map[string]grammar.Identity{}
	for _, id := range roster {
		byName[id.Name] = id
	}
	// cc-alpha: 2 sessions + 1 event, no task → mixed (lane + actor)
	a := byName["cc-alpha"]
	if a.Sessions != 2 || a.Events != 1 || a.Tasks != 0 || a.Class != "mixed" {
		t.Fatalf("cc-alpha roster wrong: %+v", a)
	}
	// cc-beta: 1 session + 1 task → mixed (lane + owner)
	b := byName["cc-beta"]
	if b.Sessions != 1 || b.Tasks != 1 || b.Class != "mixed" {
		t.Fatalf("cc-beta roster wrong: %+v", b)
	}
	// watcher: 1 event only → actor (single class)
	w := byName["watcher"]
	if w.Events != 1 || w.Class != "actor" {
		t.Fatalf("watcher roster wrong: %+v", w)
	}
	// deterministic alphabetical order, no empties
	if len(roster) != 3 || roster[0].Name != "cc-alpha" {
		t.Fatalf("roster must be deduped + sorted alphabetically; got %v", roster)
	}
}

func TestIdentityRosterDropsEmptyNames(t *testing.T) {
	m := New("REINS")
	m = m.FoldSessions([]grammar.Session{{Role: ""}, {Role: "  "}, {Role: "real"}}, false)
	if r := m.identityRoster(); len(r) != 1 || r[0].Name != "real" {
		t.Fatalf("empty/blank principal names must be dropped; got %v", r)
	}
}

// AIR (HARD): the identity NAME is sensitive PII and MUST redact on air; the class + appearance
// counts are the structural skeleton and survive (who is denied, that-there-is-activity is shown).
func TestIdentityRowRedactsNameOnAirKeepsSkeleton(t *testing.T) {
	id := grammar.Identity{Name: "cc-secret-lane", Class: "lane", Sessions: 3, Events: 1, Tasks: 0}
	on := ansi.Strip(grammar.RenderIdentityRow(id, true, 80))
	if strings.Contains(on, "cc-secret-lane") {
		t.Fatalf("on-air identity row must redact the principal name:\n%s", on)
	}
	if !strings.Contains(on, "▒▒▒") {
		t.Fatalf("on-air identity row must show the redaction token:\n%s", on)
	}
	// skeleton survives: the class + the s·e·t counts (anonymous activity shape)
	if !strings.Contains(on, "lane") || !strings.Contains(on, "s3·e1·t0") {
		t.Fatalf("on-air identity row must keep the class + appearance counts (skeleton):\n%s", on)
	}
	// off air the real name renders
	off := ansi.Strip(grammar.RenderIdentityRow(id, false, 80))
	if !strings.Contains(off, "cc-secret-lane") {
		t.Fatalf("off-air identity row must show the real principal:\n%s", off)
	}
}

func TestIdentityDetailRedactsNameOnAir(t *testing.T) {
	id := grammar.Identity{Name: "cc-secret-lane", Class: "lane", Sessions: 3}
	on := ansi.Strip(grammar.RenderIdentityDetail(id, true, 90))
	if strings.Contains(on, "cc-secret-lane") {
		t.Fatalf("on-air identity detail must redact the principal name:\n%s", on)
	}
	// the A1 contract + projection-pending badge still situate the pane
	if !strings.Contains(on, "blind-spot") || !strings.Contains(strings.ToLower(on), "projection-pending") {
		t.Fatalf("identity detail must show the A1 contract + projection-pending badge:\n%s", on)
	}
}
