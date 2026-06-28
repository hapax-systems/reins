package model

import (
	"sort"
	"strings"

	"github.com/hapax-systems/reins/internal/grammar"
)

// identityRoster folds the A1 Identity pane's data: the deduped set of distinct principals across the
// fleet (session roles, event actors, task owners), each with its class + appearance counts. Pure —
// re-folding restores the view. The roster is alphabetical (deterministic); AIR redaction of the
// NAME happens at RENDER (grammar.RenderIdentityRow), never here — the fold keeps the real name so the
// pane works off-air. A1 is projection-pending: this is derived from existing fields, not a registry.
func (m Model) identityRoster() []grammar.Identity {
	type acc struct {
		sessions, events, tasks  int
		isLane, isActor, isOwner bool
	}
	idx := map[string]*acc{}
	bump := func(name, kind string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		a := idx[name]
		if a == nil {
			a = &acc{}
			idx[name] = a
		}
		switch kind {
		case "lane":
			a.sessions++
			a.isLane = true
		case "actor":
			a.events++
			a.isActor = true
		case "owner":
			a.tasks++
			a.isOwner = true
		}
	}
	for _, s := range m.Sessions {
		bump(s.Role, "lane")
	}
	for _, e := range m.Events {
		bump(e.Actor, "actor")
	}
	for _, t := range m.Tasks {
		bump(t.Owner, "owner")
	}

	names := make([]string, 0, len(idx))
	for n := range idx {
		names = append(names, n)
	}
	sort.Strings(names)

	roster := make([]grammar.Identity, 0, len(names))
	for _, n := range names {
		a := idx[n]
		roster = append(roster, grammar.Identity{
			Name:     n,
			Class:    identityClass(a.isLane, a.isActor, a.isOwner),
			Sessions: a.sessions,
			Events:   a.events,
			Tasks:    a.tasks,
		})
	}
	return roster
}

// identityClass names the principal's role-kind; a principal appearing in more than one is "mixed".
func identityClass(isLane, isActor, isOwner bool) string {
	n := 0
	for _, b := range []bool{isLane, isActor, isOwner} {
		if b {
			n++
		}
	}
	if n > 1 {
		return "mixed"
	}
	switch {
	case isLane:
		return "lane"
	case isActor:
		return "actor"
	case isOwner:
		return "owner"
	}
	return "actor"
}
