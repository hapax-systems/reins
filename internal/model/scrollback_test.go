package model

import (
	"fmt"
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

// Scrollback is the per-window event-history ring (the BitchX/irssi scrollback /
// lastlog affordance). These tests pin the two load-bearing invariants: it bounds at
// Cap (oldest drops), and it preserves AIR provenance so a locally-captured private
// event cannot replay cleartext after the AIR lens turns on.

func TestScrollbackCapsAtCapacity(t *testing.T) {
	s := Scrollback{Cap: 3}
	for _, e := range []grammar.Event{
		{TS: "T10", Subject: "A"},
		{TS: "T11", Subject: "B"},
		{TS: "T12", Subject: "C"},
		{TS: "T13", Subject: "D"},
		{TS: "T14", Subject: "E"},
	} {
		s.Push(e)
	}
	if len(s.Rows) != 3 {
		t.Fatalf("cap: want 3 rows, got %d", len(s.Rows))
	}
	if s.Rows[0].Subject != "C" {
		t.Fatalf("oldest retained after trim: want C, got %q", s.Rows[0].Subject)
	}
	if s.Rows[2].Subject != "E" {
		t.Fatalf("newest retained: want E, got %q", s.Rows[2].Subject)
	}
}

func TestScrollbackPreservesAirProvenance(t *testing.T) {
	s := Scrollback{Cap: 8}
	s.Push(grammar.Event{Subject: "4284", AIR: map[string]string{"subject": "deny", "kind": "ok"}})
	if len(s.Rows) != 1 || s.Rows[0].AIR["subject"] != "deny" {
		t.Fatal("AIR provenance must survive the ring so the lens redacts on replay")
	}
}

func TestScrollbackOldestTSIsBackwardCursor(t *testing.T) {
	s := Scrollback{Cap: 8}
	if got := s.OldestTS(); got != "" {
		t.Fatalf("empty oldest ts: want empty, got %q", got)
	}
	s.Push(grammar.Event{TS: "t1"})
	s.Push(grammar.Event{TS: "t2"})
	if got := s.OldestTS(); got != "t1" {
		t.Fatalf("oldest ts (backward cursor): want t1, got %q", got)
	}
}

func TestScrollbackFeedAccumulatesNewOnly(t *testing.T) {
	s := Scrollback{Cap: 512}
	s.Feed([]grammar.Event{{TS: "t1"}, {TS: "t2"}, {TS: "t3"}})
	if len(s.Rows) != 3 {
		t.Fatalf("first feed: want 3 rows, got %d", len(s.Rows))
	}
	// re-feed an overlapping window (t2,t3) + one new (t4): only t4 accumulates
	s.Feed([]grammar.Event{{TS: "t2"}, {TS: "t3"}, {TS: "t4"}})
	if len(s.Rows) != 4 {
		t.Fatalf("re-feed: want 4 rows (t1..t4), got %d", len(s.Rows))
	}
	if s.Rows[len(s.Rows)-1].TS != "t4" {
		t.Fatalf("newest after feed: want t4, got %s", s.Rows[len(s.Rows)-1].TS)
	}
}

func TestScrollbackFeedDefaultsCapWhenZero(t *testing.T) {
	var s Scrollback // Cap 0 -> Feed defaults to 512
	for i := 0; i < 4; i++ {
		s.Feed([]grammar.Event{{TS: fmt.Sprintf("t%d", i)}})
	}
	if s.Cap != 512 || len(s.Rows) != 4 {
		t.Fatalf("default cap + retain: want cap=512 rows=4, got cap=%d rows=%d", s.Cap, len(s.Rows))
	}
}
