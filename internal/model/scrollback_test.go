package model

import (
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
