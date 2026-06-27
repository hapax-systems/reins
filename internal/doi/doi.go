// Package doi is the Reins Degree-of-Interest fold (framework §2) — the ONE scaling mechanism applied
// identically in every pane: rank items by importance, then fill the finite cell budget top-down —
// high-DOI rendered FULL, mid COLLAPSED to a glyph-line, the remainder AGGREGATED to an inspectable
// "+N". This is how Reins represents MUCH MORE information coherently (the CRUX) by ALLOCATION, not
// compression — and it is the A3 bridge: the importance term IS the salience/epistemic engine output
// (interestingness × info-density × classification × severity × freshness — the operator's scored-
// screenspace formula). Those factors are produced by the council salience engines (topic_interest,
// stimmung, perceptual_field_grounding) and served per-item by the read API; this package is the
// consumer-side fold. Wiring the producers into the live read API is the (operator-gated) flip that
// turns hand-sorted lists into salience-ranked ones — see facet_registry / the A3 inventory.
package doi

import "sort"

// Salience holds the five factors of the operator's attention formula. Unset factors default to 1.0
// (neutral) via NewSalience so a partially-known salience does not annihilate the product.
type Salience struct {
	Interestingness float64 // topic_interest_engine total_score
	Density         float64 // information_density (KL-surprise / novelty)
	Classification  float64 // text-class weight (the "universal lever" gap today)
	Severity        float64 // criticality (Posture facet)
	Freshness       float64 // recency (Time facet)
}

// NewSalience returns a neutral (all-1.0) salience to be selectively overridden.
func NewSalience() Salience {
	return Salience{1, 1, 1, 1, 1}
}

// Importance = the multiplicative scored-screenspace formula. Multiplicative is deliberate: a factor
// at 0 (e.g. classified irrelevant) drives importance to 0 — the item earns no cells.
func (s Salience) Importance() float64 {
	return s.Interestingness * s.Density * s.Classification * s.Severity * s.Freshness
}

// DOI = importance − distance-from-focus (Furnas). Distance is a non-negative "how far from what the
// operator is attending to" (lattice / selection distance); nearer-to-focus items rank higher.
func DOI(importance, distanceFromFocus float64) float64 {
	return importance - distanceFromFocus
}

// Scored is one candidate row carrying its computed DOI.
type Scored struct {
	ID  string
	DOI float64
}

// Tier — how a row is rendered after the fold.
type Tier int

const (
	Full       Tier = iota // the focal rows: full cell grammar
	Collapsed              // mid rows: a one-line glyph + magnitude summary
	Aggregated             // the tail: folded into a single inspectable "+N" cell
)

// Placement is the fold result for one visible row.
type Placement struct {
	ID   string
	Tier Tier
}

// Fold allocates a finite cell `budget` over DOI-ranked items: the first `budget` items are visible
// (Full if DOI ≥ fullThreshold, else Collapsed); everything past the budget is Aggregated into a
// single "+N" tail (its count returned). Stable: ties keep input order. This is the recede/summon +
// scale-by-allocation contract — every pane folds the same way, so coherence is structural.
func Fold(items []Scored, budget int, fullThreshold float64) (placements []Placement, aggregated int) {
	ranked := make([]Scored, len(items))
	copy(ranked, items)
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].DOI > ranked[j].DOI })

	if budget < 0 {
		budget = 0
	}
	for i, it := range ranked {
		if i >= budget {
			aggregated++
			continue
		}
		tier := Collapsed
		if it.DOI >= fullThreshold {
			tier = Full
		}
		placements = append(placements, Placement{ID: it.ID, Tier: tier})
	}
	return placements, aggregated
}
