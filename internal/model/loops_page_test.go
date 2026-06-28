package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
	"github.com/hapax-systems/reins/internal/graph"
)

func loopsAIROK() map[string]string {
	return map[string]string{
		"id": "ok", "label": "ok", "kind": "ok", "layer": "ok", "status": "ok", "res": "ok",
		"source": "ok", "target": "ok", "relation": "ok", "sign": "ok", "delay": "ok", "prov": "ok",
	}
}

func loopsPageModel(edges []grammar.Edge, nodes ...grammar.Node) Model {
	m := New("REINS").FoldDynamics(grammar.Graph{Nodes: nodes, Edges: edges}, false)
	m.Width, m.Height, m.Page = 180, 44, PageLoops
	return m
}

func TestLoopsComposesViaAlgebraNativeBinding(t *testing.T) {
	m := loopsPageModel([]grammar.Edge{
		{Source: "A", Target: "B", Relation: "supports", AIR: loopsAIROK()},
		{Source: "B", Target: "A", Relation: "supports", AIR: loopsAIROK()},
	},
		grammar.Node{ID: "A", AIR: loopsAIROK()},
		grammar.Node{ID: "B", AIR: loopsAIROK()},
	)
	if !m.composesViaAlgebra() {
		t.Fatal("PageLoops must compose via the view algebra")
	}
	if isSessionAnchoredPage(m.Page) {
		t.Fatal("PageLoops must not activate the legacy session-frozen split")
	}
	if m.commandSelectionPage() != PageLoops {
		t.Fatalf("templates/yank context must bind to PageLoops, got %d", m.commandSelectionPage())
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"CAUSAL LOOPS", "LOOP STRUCTURE", "loop -> structure", "no simulation"} {
		if !strings.Contains(v, want) {
			t.Fatalf("loops page should render %q via algebra split:\n%s", want, v)
		}
	}
}

func TestLoopsJMovesLoopFocus(t *testing.T) {
	m := loopsPageModel([]grammar.Edge{
		{Source: "A", Target: "B", Relation: "supports", AIR: loopsAIROK()},
		{Source: "B", Target: "A", Relation: "supports", AIR: loopsAIROK()},
		{Source: "C", Target: "D", Relation: "supports", AIR: loopsAIROK()},
		{Source: "D", Target: "C", Relation: "supports", AIR: loopsAIROK()},
	},
		grammar.Node{ID: "A", AIR: loopsAIROK()}, grammar.Node{ID: "B", AIR: loopsAIROK()},
		grammar.Node{ID: "C", AIR: loopsAIROK()}, grammar.Node{ID: "D", AIR: loopsAIROK()},
	)
	if got := len(m.loopRows()); got < 2 {
		t.Fatalf("test requires at least two loops, got %d", got)
	}
	before := m.LoopFocus
	m = step(m, "j")
	if m.LoopFocus != before+1 {
		t.Fatalf("j must move LoopFocus natively (%d→%d)", before, m.LoopFocus)
	}
}

func TestLoopsKnownTwoCycleParity(t *testing.T) {
	m := loopsPageModel([]grammar.Edge{
		{Source: "demand", Target: "load", Relation: "supports", AIR: loopsAIROK()},
		{Source: "load", Target: "demand", Relation: "blocks", AIR: loopsAIROK()},
	},
		grammar.Node{ID: "demand", AIR: loopsAIROK()},
		grammar.Node{ID: "load", AIR: loopsAIROK()},
	)
	rows := m.loopRows()
	if len(rows) != 1 {
		t.Fatalf("expected one 2-cycle loop, got %d", len(rows))
	}
	if rows[0].Kind != graph.Balancing {
		t.Fatalf("one negative edge must classify as Balancing, got %s", rows[0].Kind)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "⊖B") || !strings.Contains(v, "sign=−") {
		t.Fatalf("balancing loop should render a shape glyph and negative dominant sign:\n%s", v)
	}
}

func TestLoopsHonestEmptyOnAcyclicGraph(t *testing.T) {
	m := loopsPageModel([]grammar.Edge{
		{Source: "source", Target: "sink", Relation: "supports", AIR: loopsAIROK()},
	},
		grammar.Node{ID: "source", AIR: loopsAIROK()},
		grammar.Node{ID: "sink", AIR: loopsAIROK()},
	)
	if got := len(m.loopRows()); got != 0 {
		t.Fatalf("acyclic graph must have no causal loops, got %d", got)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "no causal loops in the current graph — acyclic or single-node") {
		t.Fatalf("acyclic graph should render honest-empty text:\n%s", v)
	}
	if strings.Contains(v, "fixture fallback") {
		t.Fatalf("non-empty acyclic graph must not fall back to fixture:\n%s", v)
	}
}

func TestLoopsAIRRedactsDeniedNodeIDs(t *testing.T) {
	denied := loopsAIROK()
	denied["id"] = "deny"
	m := loopsPageModel([]grammar.Edge{
		{Source: "SECRET-NODE", Target: "public-node", Relation: "supports", AIR: loopsAIROK()},
		{Source: "public-node", Target: "SECRET-NODE", Relation: "supports", AIR: loopsAIROK()},
	},
		grammar.Node{ID: "SECRET-NODE", AIR: denied},
		grammar.Node{ID: "public-node", AIR: loopsAIROK()},
	)
	m.AIR = true
	v := ansi.Strip(m.View())
	if strings.Contains(v, "SECRET-NODE") {
		t.Fatalf("on-air loops page leaked a denied dynamics node id:\n%s", v)
	}
	if !strings.Contains(v, "▒▒▒") || !strings.Contains(v, "⟳R") {
		t.Fatalf("on-air loops page should redact identity while preserving structural loop type:\n%s", v)
	}
}
