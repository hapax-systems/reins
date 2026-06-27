// Package graph is the Reins general relation/graph primitive (framework §5b): ONE typed graph over
// the relation-derivation edge stream, reused by every relational surface (systems-dynamics,
// task-deps, capability routing, dispatch trees, implication edges, the selection lattice).
//
// This file is the A5 Tier-1 STRUCTURAL CAUSAL-LOOP layer — the highest-leverage finding: the
// QUALITATIVE systems-dynamics layer (signed links + Reinforcing/Balancing feedback loops) is
// COMPUTABLE from the edge store with NO simulation, given a polarity (sign) facet on each edge.
// Loop type = parity of negative-sign links around an elementary cycle (even ⇒ Reinforcing,
// odd ⇒ Balancing) — Sterman/CLD canon. Sign is INFERRED (operator-ratified) from a relation-type
// prior; inferred edges carry Provenance so they render distinct from asserted ones (A6).
package graph

import "sort"

// Sign is the polarity of a causal link: do cause and effect move together (+) or oppositely (-)?
type Sign int

const (
	SignUnknown Sign = 0
	SignPos     Sign = 1
	SignNeg     Sign = -1
)

// Provenance distinguishes how an edge's facts (esp. its sign) were established — A6 demands the
// operator can always tell an inferred edge from an asserted one.
type Provenance string

const (
	Asserted Provenance = "asserted" // operator/source stated it
	Inferred Provenance = "inferred" // derived from a type prior / extraction (default for sign)
	Derived  Provenance = "derived"  // computed (e.g. near-embed, co-occurrence)
)

// Relation is one typed edge in the relation-derivation stream (framework §3).
type Relation struct {
	Src, Dst string
	Type     string  // is-a · shares-attr · co-occurs · near-embed · links-to · causes · blocks · feeds …
	Weight   float64 // strength (the relation stage emits this)
	Sign     Sign    // +/-/unknown — polarity (inferred from Type unless asserted)
	Delay    bool    // ‖ — a delayed link (the source of oscillation in feedback)
	Prov     Provenance
}

// TypedGraph = nodes + typed edges. The universal substrate for every Reins graph surface.
type TypedGraph struct {
	nodes map[string]struct{}
	Edges []Relation
}

func New() *TypedGraph { return &TypedGraph{nodes: map[string]struct{}{}} }

// Add inserts an edge (and its endpoints). If the edge's Sign is unknown, it is INFERRED from the
// relation type and marked Inferred provenance (unless already asserted) — A5/A6.
func (g *TypedGraph) Add(r Relation) {
	g.nodes[r.Src] = struct{}{}
	g.nodes[r.Dst] = struct{}{}
	if r.Sign == SignUnknown {
		if s := InferSign(r.Type); s != SignUnknown {
			r.Sign = s
			if r.Prov == "" {
				r.Prov = Inferred
			}
		}
	}
	if r.Prov == "" {
		r.Prov = Asserted
	}
	g.Edges = append(g.Edges, r)
}

// InferSign is the relation-type → polarity PRIOR (the operator-ratified inferred sign — the cheapest
// of the three inference channels; LLM-extraction and time-series correlation refine it later).
// Anything not in the prior returns SignUnknown (→ loop type Indeterminate, never a wrong guess).
func InferSign(relType string) Sign {
	switch relType {
	case "blocks", "refutes", "reduces", "guards", "guards_release", "inhibits", "consumes", "drains":
		return SignNeg
	case "feeds", "enables", "grounds", "produces", "produces_evidence", "complements",
		"feeds_or_complements", "reinforces", "amplifies", "increases", "supports":
		return SignPos
	}
	return SignUnknown
}

func (g *TypedGraph) Nodes() []string {
	out := make([]string, 0, len(g.nodes))
	for n := range g.nodes {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// adjacency: src -> outgoing relations.
func (g *TypedGraph) adjacency() map[string][]Relation {
	a := map[string][]Relation{}
	for _, e := range g.Edges {
		a[e.Src] = append(a[e.Src], e)
	}
	return a
}

// ElementaryCycles enumerates the simple directed cycles (each a feedback loop). DFS with a
// canonical minimum-start to avoid rotational duplicates. Reins graphs are small (tens of nodes), so
// the worst-case exponential enumeration is acceptable; a Johnson upgrade is a noted follow-up.
// Each cycle is returned as the ordered node list [n0, n1, …, nk] with the closing edge nk→n0.
func (g *TypedGraph) ElementaryCycles() [][]string {
	nodes := g.Nodes()
	idx := map[string]int{}
	for i, n := range nodes {
		idx[n] = i
	}
	adj := g.adjacency()
	var cycles [][]string
	for i, start := range nodes {
		onPath := map[string]bool{start: true}
		var dfs func(cur string, path []string)
		dfs = func(cur string, path []string) {
			for _, e := range adj[cur] {
				if e.Dst == start {
					cyc := make([]string, len(path))
					copy(cyc, path)
					cycles = append(cycles, cyc)
					continue
				}
				if onPath[e.Dst] || idx[e.Dst] < i { // canonical: start is the min-index node
					continue
				}
				onPath[e.Dst] = true
				dfs(e.Dst, append(path, e.Dst))
				onPath[e.Dst] = false
			}
		}
		dfs(start, []string{start})
	}
	return cycles
}

// LoopKind — Reinforcing (amplifying), Balancing (goal-seeking), or Indeterminate (a sign is unknown,
// so we refuse to guess the type — honest over wrong).
type LoopKind string

const (
	Reinforcing   LoopKind = "R"
	Balancing     LoopKind = "B"
	Indeterminate LoopKind = "?"
)

// edgeBetween returns the (first) edge a→b.
func (g *TypedGraph) edgeBetween(a, b string) (Relation, bool) {
	for _, e := range g.Edges {
		if e.Src == a && e.Dst == b {
			return e, true
		}
	}
	return Relation{}, false
}

// LoopType classifies a cycle by the PARITY of its negative-sign links (even ⇒ R, odd ⇒ B). If any
// link's sign is unknown, the type is Indeterminate (no simulation, no guessing).
func (g *TypedGraph) LoopType(cycle []string) LoopKind {
	if len(cycle) < 1 {
		return Indeterminate
	}
	neg := 0
	for i := range cycle {
		a := cycle[i]
		b := cycle[(i+1)%len(cycle)]
		e, ok := g.edgeBetween(a, b)
		if !ok || e.Sign == SignUnknown {
			return Indeterminate
		}
		if e.Sign == SignNeg {
			neg++
		}
	}
	if neg%2 == 0 {
		return Reinforcing
	}
	return Balancing
}

// Loop is a classified feedback loop, ready for the cell-grammar (⟲R / ⟳B badge, ‖ delay marker).
type Loop struct {
	Nodes    []string
	Kind     LoopKind
	HasDelay bool
}

// CausalLoops detects + classifies every feedback loop in the graph — the Tier-1 systems-dynamics
// readout, computable now from the relation store. (Loops with any unknown-sign link are marked
// Indeterminate; the operator can pin a sign to resolve them — A6 operator-correctable.)
func (g *TypedGraph) CausalLoops() []Loop {
	var loops []Loop
	for _, c := range g.ElementaryCycles() {
		delay := false
		for i := range c {
			if e, ok := g.edgeBetween(c[i], c[(i+1)%len(c)]); ok && e.Delay {
				delay = true
			}
		}
		loops = append(loops, Loop{Nodes: c, Kind: g.LoopType(c), HasDelay: delay})
	}
	return loops
}
