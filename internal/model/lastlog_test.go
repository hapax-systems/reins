package model

import (
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

// Inc 1b/3: the /lastlog backward-page. When a PgUp page lands (LastlogPageMsg), the older
// events prepend above the retained recent window (oldest->newest order preserved) and the
// in-flight paging flag clears.

func TestLastlogPageMsgPrependsOlderAndClearsPaging(t *testing.T) {
	m := New("t")
	m.EventScrollback.Cap = 8
	m.EventScrollback.Push(grammar.Event{TS: "t9", Subject: "recent"})
	m.LastlogPaging = true

	nm, _ := m.Update(LastlogPageMsg{Events: []grammar.Event{
		{TS: "t1", Subject: "old"},
		{TS: "t2", Subject: "older"},
	}})
	m = nm.(Model)

	if m.LastlogPaging {
		t.Fatal("LastlogPaging must clear after the backward page lands")
	}
	// API returns oldest->newest; prepending keeps that order so older sits above retained
	if len(m.LastlogOlder) != 2 || m.LastlogOlder[0].Subject != "old" || m.LastlogOlder[1].Subject != "older" {
		t.Fatalf("older prepended (oldest->newest): want [old older], got %+v", m.LastlogOlder)
	}
	// the retained ring is untouched by paging
	if len(m.EventScrollback.Rows) != 1 || m.EventScrollback.Rows[0].Subject != "recent" {
		t.Fatalf("retained ring must be untouched by paging: got %+v", m.EventScrollback.Rows)
	}
}
