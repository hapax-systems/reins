package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// E0.2 brushing: the lens highlights (├) the rows that participate in the FOCUSED row's emergent
// relation (relate.Derive Peers) — visualizing the connector's "shares … (N)" as the actual N rows,
// WITHOUT authoring the relation (it stays derived from facets). The brush is positional + AIR-safe.

func TestCoordinatorBrushesRelatedTasks(t *testing.T) {
	// distinct stages/crits so owner=alpha is the UNIQUE strongest shared facet (no tie to break).
	tasks := []grammar.Task{
		{TaskID: "t1", Owner: "alpha", Stage: "build", Criticality: "ok"},
		{TaskID: "t2", Owner: "alpha", Stage: "review", Criticality: "warn"},
		{TaskID: "t3", Owner: "beta", Stage: "ship", Criticality: "major"},
		{TaskID: "t4", Owner: "alpha", Stage: "plan", Criticality: "crit"},
	}
	m := New("REINS").FoldTasks(tasks, false)
	m.Page = PageCoordinator
	m.Focus = 0 // focus t1 (owner=alpha); the strongest shared facet is owner=alpha → t2,t4

	brushed := m.coordinatorBrushedTasks()
	if !brushed["t2"] || !brushed["t4"] {
		t.Fatalf("the owner=alpha siblings must be brushed; got %v", brushed)
	}
	if brushed["t3"] {
		t.Fatalf("a non-sharing task must not be brushed; got %v", brushed)
	}
	if brushed["t1"] {
		t.Fatalf("the focused task itself must not be in the brush set; got %v", brushed)
	}

	out := ansi.Strip(m.coordinatorLensPane(120, 30))
	if strings.Count(out, "├") < 2 {
		t.Fatalf("the lens must draw a ├ gutter for each brushed row:\n%s", out)
	}
	// in-band decode: the connector label must announce the brush glyph
	if !strings.Contains(m.coordinatorEmergentRelation(), "├") {
		t.Fatalf("the connector label must decode ├ in-band; got %q", m.coordinatorEmergentRelation())
	}
}

func TestEventsBrushRelatedRows(t *testing.T) {
	// distinct subjects/kinds so actor=alpha is the UNIQUE strongest shared facet (no tie).
	evs := []grammar.Event{
		{TS: "t1", Kind: "k1", Subject: "s1", Actor: "alpha"},
		{TS: "t2", Kind: "k2", Subject: "s2", Actor: "alpha"},
		{TS: "t3", Kind: "k3", Subject: "s3", Actor: "beta"},
		{TS: "t4", Kind: "k4", Subject: "s4", Actor: "alpha"},
	}
	m := New("REINS").Fold(evs, false)
	m.Page = PageEvents
	m.EFocus = 0 // focus e1 (actor=alpha); the strongest shared facet is actor=alpha → e2,e4

	brushed := m.brushedEvents()
	id := func(e grammar.Event) string { return eventEntity(e, false).ID }
	if !brushed[id(evs[1])] || !brushed[id(evs[3])] {
		t.Fatalf("the actor=alpha siblings must be brushed; got %v", brushed)
	}
	if brushed[id(evs[2])] || brushed[id(evs[0])] {
		t.Fatalf("non-sharers and the focused event must not be brushed; got %v", brushed)
	}
	out := ansi.Strip(m.eventsListBody(140, 30))
	if strings.Count(out, "├") < 2 {
		t.Fatalf("the events list must draw a ├ gutter for each brushed row:\n%s", out)
	}
	if !strings.Contains(m.eventsEmergentRelation(), "├") {
		t.Fatalf("the events connector label must decode ├ in-band; got %q", m.eventsEmergentRelation())
	}
}

func TestCoordinatorBrushRedactedFacetDoesNotForm(t *testing.T) {
	// when the shared facet's source field is DENIED on air, the relation must not form over it,
	// so no rows are brushed on that facet (no derived-channel leak).
	air := map[string]string{"owner": "deny"}
	tasks := []grammar.Task{
		{TaskID: "t1", Owner: "alpha", AIR: air},
		{TaskID: "t2", Owner: "alpha", AIR: air},
	}
	m := New("REINS").FoldTasks(tasks, false)
	m.Page = PageCoordinator
	m.Focus = 0
	m.AIR = true
	if b := m.coordinatorBrushedTasks(); b["t2"] {
		t.Fatalf("a relation over a redacted facet must not brush; got %v", b)
	}
}
