package graph

import (
	"sort"
	"testing"
)

func TestInferSignPrior(t *testing.T) {
	if InferSign("blocks") != SignNeg {
		t.Fatal("blocks should infer negative")
	}
	if InferSign("enables") != SignPos {
		t.Fatal("enables should infer positive")
	}
	if InferSign("co-occurs") != SignUnknown {
		t.Fatal("an unmapped relation type must stay unknown (never a wrong guess)")
	}
}

func TestAddInfersSignAndMarksProvenance(t *testing.T) {
	g := New()
	g.Add(Relation{Src: "a", Dst: "b", Type: "blocks"}) // no sign given -> inferred
	e := g.Edges[0]
	if e.Sign != SignNeg {
		t.Fatalf("sign should be inferred negative, got %d", e.Sign)
	}
	if e.Prov != Inferred {
		t.Fatalf("an inferred-sign edge must be marked Inferred (A6), got %q", e.Prov)
	}
}

func TestAssertedSignNotOverwritten(t *testing.T) {
	g := New()
	g.Add(Relation{Src: "a", Dst: "b", Type: "blocks", Sign: SignPos, Prov: Asserted})
	if g.Edges[0].Sign != SignPos || g.Edges[0].Prov != Asserted {
		t.Fatal("an asserted sign/provenance must not be overwritten by the type prior")
	}
}

func TestReinforcingLoop(t *testing.T) {
	g := New()
	g.Add(Relation{Src: "A", Dst: "B", Type: "feeds"})    // +
	g.Add(Relation{Src: "B", Dst: "A", Type: "produces"}) // +
	loops := g.CausalLoops()
	if len(loops) != 1 {
		t.Fatalf("expected 1 loop, got %d: %v", len(loops), loops)
	}
	if loops[0].Kind != Reinforcing {
		t.Fatalf("0 negative links ⇒ Reinforcing, got %s", loops[0].Kind)
	}
}

func TestBalancingLoop(t *testing.T) {
	g := New()
	g.Add(Relation{Src: "A", Dst: "B", Type: "feeds"})  // +
	g.Add(Relation{Src: "B", Dst: "A", Type: "blocks"}) // -
	loops := g.CausalLoops()
	if len(loops) != 1 || loops[0].Kind != Balancing {
		t.Fatalf("1 negative link ⇒ Balancing, got %v", loops)
	}
}

func TestThreeCycleAndParity(t *testing.T) {
	g := New()
	g.Add(Relation{Src: "A", Dst: "B", Type: "feeds"})  // +
	g.Add(Relation{Src: "B", Dst: "C", Type: "blocks"}) // -
	g.Add(Relation{Src: "C", Dst: "A", Type: "blocks"}) // -
	loops := g.CausalLoops()
	if len(loops) != 1 {
		t.Fatalf("expected 1 three-cycle, got %d", len(loops))
	}
	if loops[0].Kind != Reinforcing { // 2 negatives ⇒ even ⇒ Reinforcing
		t.Fatalf("2 negative links ⇒ Reinforcing, got %s", loops[0].Kind)
	}
}

func TestTwoDistinctLoops(t *testing.T) {
	g := New()
	g.Add(Relation{Src: "A", Dst: "B", Type: "feeds"})
	g.Add(Relation{Src: "B", Dst: "A", Type: "feeds"}) // R
	g.Add(Relation{Src: "C", Dst: "D", Type: "feeds"})
	g.Add(Relation{Src: "D", Dst: "C", Type: "blocks"}) // B
	loops := g.CausalLoops()
	if len(loops) != 2 {
		t.Fatalf("expected 2 loops, got %d: %v", len(loops), loops)
	}
	kinds := []string{string(loops[0].Kind), string(loops[1].Kind)}
	sort.Strings(kinds)
	if kinds[0] != "B" || kinds[1] != "R" {
		t.Fatalf("expected one R and one B loop, got %v", kinds)
	}
}

func TestAcyclicHasNoLoops(t *testing.T) {
	g := New()
	g.Add(Relation{Src: "A", Dst: "B", Type: "feeds"})
	g.Add(Relation{Src: "B", Dst: "C", Type: "feeds"})
	if loops := g.CausalLoops(); len(loops) != 0 {
		t.Fatalf("a DAG has no feedback loops, got %v", loops)
	}
}

func TestUnknownSignIsIndeterminate(t *testing.T) {
	g := New()
	g.Add(Relation{Src: "A", Dst: "B", Type: "feeds"})     // +
	g.Add(Relation{Src: "B", Dst: "A", Type: "co-occurs"}) // unknown sign
	loops := g.CausalLoops()
	if len(loops) != 1 || loops[0].Kind != Indeterminate {
		t.Fatalf("an unknown-sign link ⇒ Indeterminate (no guessing), got %v", loops)
	}
}

func TestDelayDetected(t *testing.T) {
	g := New()
	g.Add(Relation{Src: "A", Dst: "B", Type: "feeds"})
	g.Add(Relation{Src: "B", Dst: "A", Type: "blocks", Delay: true})
	loops := g.CausalLoops()
	if len(loops) != 1 || !loops[0].HasDelay {
		t.Fatalf("a delayed link must mark the loop HasDelay (oscillation), got %v", loops)
	}
}
