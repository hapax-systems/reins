package grammar

import (
	"sort"
	"strings"
)

// FacetDef mirrors one entry of the facet-registry SSOT served by /read/facets (api/facet_registry.py).
type FacetDef struct {
	Gloss    string `json:"gloss"`
	Question string `json:"question"`
	Role     string `json:"role"`
	Channel  string `json:"channel"`
	Air      string `json:"air"`
}

// FacetRegistry is the in-band decoder (A6) fetched from /read/facets: the facet vocabulary + the
// on-air allowlist. The Go side consumes this rather than re-deriving the cut — one source, no drift.
type FacetRegistry struct {
	Facets        map[string]FacetDef `json:"facets"`
	CitationOrder []string            `json:"citation_order"`
	AirAllowlist  []string            `json:"air_allowlist"`
}

// RenderFacetLegend is the cold-read decoder for the facet VOCABULARY (A6: a stranger meeting the
// columns cold can decode them; it travels in-band). FACET · gloss · channel, in citation order.
func RenderFacetLegend(r FacetRegistry) string {
	order := r.CitationOrder
	if len(order) == 0 {
		for k := range r.Facets {
			order = append(order, k)
		}
		sort.Strings(order)
	}
	var b strings.Builder
	b.WriteString(C("brt", "FACETS") + C("mut", " — the columns' vocabulary (every row decomposes into these)") + "\n")
	for _, k := range order {
		f, ok := r.Facets[k]
		if !ok {
			continue
		}
		b.WriteString("   " + C("pri", pad(k, 11)) + C("mut", pad(f.Gloss, 26)) + C("mut", "· "+f.Channel) + "\n")
	}
	return b.String()
}
