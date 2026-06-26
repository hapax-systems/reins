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
