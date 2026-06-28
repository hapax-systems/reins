// Package relate derives the EMERGENT relationship between two split panes — the operator's ruling
// (2026-06-27): "any split is an implicit pair; the implicit relationship between them should be
// emergent and based on our information-processing systems." There is no authored split-pair
// contract; the relation is COMPUTED by consuming what those systems already produce and Reins
// already reads — the council-derived graph EDGES (relation{src,dst,type,weight}) and the per-item
// FACETS — never re-run here ("consume, don't rederive").
//
// v1 consumes edges + facet-overlap (computable from the live read model). The full semantic
// relation (embedding-cosine / LLM-KG over arbitrary pane contents) is the cross-repo
// relation-derivation PRODUCER (framework step 5); when it lands it simply emits richer Edges that
// this same Derive consumes.
package relate

import (
	"fmt"
	"sort"
)

// Entity is one item shown in a pane: its id + the facets the info-processing systems tagged it with.
type Entity struct {
	ID     string
	Facets map[string]string
}

// Edge is a council-derived relation between two entities (graph.Relation, adapted): the Type is
// the emergent relationship (causes·blocks·feeds·shares-attr·near-embed·links-to·…), Weight its strength.
type Edge struct {
	Src, Dst string
	Type     string
	Weight   float64
}

// Relation is the emergent pane-to-pane relationship. Label is the off-air default; the structured
// fields let a caller render it AIR-aware (withhold a sensitive facet Value or edge Peer on air
// while keeping the structural shape).
type Relation struct {
	Kind     string   // "edge" | "facet" | "none"
	Type     string   // edge type (Kind=="edge"): blocks/causes/feeds/...
	Peer     string   // strongest peer entity id (Kind=="edge")
	Peers    []string // ALL pane-B ids participating in this relation — the brush set (edge-typed or facet-sharing)
	Facet    string   // shared facet name (Kind=="facet")
	Value    string   // shared facet value (Kind=="facet")
	Count    int      // how many others share it (Kind=="facet")
	Strength float64
	Label    string
}

// Derive computes the emergent relationship FROM an anchor entity (e.g. pane A's selection) TO a set
// of pane-B entities. Precedence: a council-derived graph EDGE first (the real, typed relation),
// else the strongest SHARED FACET, else an honest empty relation. Pure.
func Derive(anchor Entity, others []Entity, edges []Edge) Relation {
	if r, ok := strongestEdge(anchor, others, edges); ok {
		return r
	}
	if r, ok := strongestSharedFacet(anchor, others); ok {
		return r
	}
	return Relation{Kind: "none", Label: "—", Strength: 0}
}

func strongestEdge(anchor Entity, others []Entity, edges []Edge) (Relation, bool) {
	otherIDs := make(map[string]bool, len(others))
	for _, o := range others {
		otherIDs[o.ID] = true
	}
	best := Edge{}
	found := false
	for _, e := range edges {
		var peer string
		switch {
		case e.Src == anchor.ID && otherIDs[e.Dst]:
			peer = e.Dst
		case e.Dst == anchor.ID && otherIDs[e.Src]:
			peer = e.Src
		default:
			continue
		}
		if !found || e.Weight > best.Weight {
			best, found = e, true
			best.Dst = peer // normalize: Dst is the peer regardless of edge direction
		}
	}
	if !found {
		return Relation{}, false
	}
	// the brush set: every pane-B id joined to the anchor by the winning edge type (not just the
	// strongest). Positional-only — callers highlight matching rows, never print these ids.
	peers := make([]string, 0)
	for _, e := range edges {
		if e.Type != best.Type {
			continue
		}
		switch {
		case e.Src == anchor.ID && otherIDs[e.Dst]:
			peers = append(peers, e.Dst)
		case e.Dst == anchor.ID && otherIDs[e.Src]:
			peers = append(peers, e.Src)
		}
	}
	return Relation{
		Kind: "edge", Type: best.Type, Peer: best.Dst, Peers: peers,
		Label: best.Type + " " + best.Dst, Strength: best.Weight,
	}, true
}

func strongestSharedFacet(anchor Entity, others []Entity) (Relation, bool) {
	type fv struct{ facet, value string }
	counts := map[fv]int{}
	for k, v := range anchor.Facets {
		if v == "" {
			continue
		}
		for _, o := range others {
			if o.Facets[k] == v {
				counts[fv{k, v}]++
			}
		}
	}
	if len(counts) == 0 {
		return Relation{}, false
	}
	keys := make([]fv, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	// deterministic: most-shared first, then facet name, then value
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		if keys[i].facet != keys[j].facet {
			return keys[i].facet < keys[j].facet
		}
		return keys[i].value < keys[j].value
	})
	best := keys[0]
	n := counts[best]
	if n == 0 {
		return Relation{}, false
	}
	// the brush set: every pane-B id sharing the winning facet=value. Positional-only.
	peers := make([]string, 0, n)
	for _, o := range others {
		if o.Facets[best.facet] == best.value {
			peers = append(peers, o.ID)
		}
	}
	return Relation{
		Kind: "facet", Facet: best.facet, Value: best.value, Count: n, Peers: peers,
		Label:    fmt.Sprintf("shares %s=%s (%d)", best.facet, best.value, n),
		Strength: float64(n) / float64(len(others)),
	}, true
}
