package model

import "github.com/hapax-systems/reins/internal/grammar"

// Scrollback is a bounded per-window history of folded event rows — the BitchX/irssi
// scrollback / lastlog affordance. It retains events WITH their AIR provenance so a
// locally-captured private event cannot replay cleartext after the AIR lens turns on:
// the render always applies AIR; the ring never strips it.
type Scrollback struct {
	Cap  int
	Rows []grammar.Event
}

// Push appends ev (newest-last) and trims to Cap, dropping the oldest off the front.
func (s *Scrollback) Push(ev grammar.Event) {
	s.Rows = append(s.Rows, ev)
	if s.Cap > 0 && len(s.Rows) > s.Cap {
		s.Rows = s.Rows[len(s.Rows)-s.Cap:]
	}
}

// OldestTS returns the timestamp of the oldest retained row — the backward-page cursor
// for the /lastlog door (paired with the API's `before` param) — or "" when empty.
func (s *Scrollback) OldestTS() string {
	if len(s.Rows) == 0 {
		return ""
	}
	return s.Rows[0].TS
}

// Feed pushes newly-seen events (ts newer than the ring's newest, or all when empty) into
// the per-window history, bounded at Cap. Called from the stateful Update layer on each
// EventsMsg (NOT the pure Fold) — history is legitimate accumulating state. The READ API
// returns the newest 80-window each poll; Feed accumulates the union beyond that window so
// the cockpit retains scrollback across polls (the BitchX/irssi "what happened while I was
// elsewhere" affordance). AIR provenance rides on each event; the render applies the lens.
func (s *Scrollback) Feed(evs []grammar.Event) {
	if s.Cap <= 0 {
		s.Cap = 512
	}
	newest := ""
	if n := len(s.Rows); n > 0 {
		newest = s.Rows[n-1].TS
	}
	for _, ev := range evs {
		if ev.TS > newest {
			s.Rows = append(s.Rows, ev)
			newest = ev.TS
		}
	}
	if len(s.Rows) > s.Cap {
		s.Rows = s.Rows[len(s.Rows)-s.Cap:]
	}
}
