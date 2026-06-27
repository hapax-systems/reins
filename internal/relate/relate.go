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

// Relation is the emergent pane-to-pane relationship: a short label for the connector header + a strength.
type Relation struct {
	Label    string
	Strength float64
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
	return Relation{Label: "—", Strength: 0}
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
	return Relation{Label: best.Type + " " + best.Dst, Strength: best.Weight}, true
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
	return Relation{
		Label:    fmt.Sprintf("shares %s=%s (%d)", best.facet, best.value, n),
		Strength: float64(n) / float64(len(others)),
	}, true
}
