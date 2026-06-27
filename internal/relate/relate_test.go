package relate

import (
	"strings"
	"testing"
)

func TestEdgeWinsOverFacet(t *testing.T) {
	// When the info-processing systems have already derived a graph edge between the anchor and a
	// pane-B entity, THAT is the emergent relation (consume it), ahead of any shared facet.
	anchor := Entity{ID: "#4278", Facets: map[string]string{"owner": "alpha", "stage": "S5"}}
	others := []Entity{
		{ID: "#4270", Facets: map[string]string{"owner": "alpha", "stage": "S5"}}, // shares facets
	}
	edges := []Edge{{Src: "#4278", Dst: "#4270", Type: "blocks", Weight: 0.9}}
	r := Derive(anchor, others, edges)
	if !strings.HasPrefix(r.Label, "blocks") || !strings.Contains(r.Label, "#4270") {
		t.Fatalf("a derived edge must win: got %q", r.Label)
	}
	if r.Strength != 0.9 {
		t.Fatalf("edge strength carries through: got %v", r.Strength)
	}
}

func TestSharedFacetWhenNoEdge(t *testing.T) {
	anchor := Entity{ID: "#a", Facets: map[string]string{"owner": "alpha", "stage": "S5"}}
	others := []Entity{
		{ID: "#b", Facets: map[string]string{"owner": "alpha", "stage": "S7"}},
		{ID: "#c", Facets: map[string]string{"owner": "alpha", "stage": "S2"}},
		{ID: "#d", Facets: map[string]string{"owner": "codex", "stage": "S2"}},
	}
	r := Derive(anchor, others, nil)
	if !strings.Contains(r.Label, "owner=alpha") {
		t.Fatalf("the most-shared facet is the emergent relation: got %q", r.Label)
	}
	if !strings.Contains(r.Label, "2") { // 2 of 3 others share owner=alpha
		t.Fatalf("shared-facet count should surface: got %q", r.Label)
	}
}

func TestNoRelationIsHonest(t *testing.T) {
	anchor := Entity{ID: "#a", Facets: map[string]string{"owner": "alpha"}}
	others := []Entity{{ID: "#b", Facets: map[string]string{"owner": "codex"}}}
	r := Derive(anchor, others, nil)
	if r.Label != "—" || r.Strength != 0 {
		t.Fatalf("no edge + no shared facet => an honest empty relation, got %q / %v", r.Label, r.Strength)
	}
}

func TestDeriveIsPureAndSafe(t *testing.T) {
	anchor := Entity{ID: "#a", Facets: map[string]string{"owner": "alpha"}}
	if Derive(anchor, nil, nil).Label != "—" {
		t.Fatal("no others => no relation")
	}
	// empty-value facets never count as shared
	others := []Entity{{ID: "#b", Facets: map[string]string{"owner": ""}}}
	anchorEmpty := Entity{ID: "#a", Facets: map[string]string{"owner": ""}}
	if Derive(anchorEmpty, others, nil).Label != "—" {
		t.Fatal("empty facet values must not register as a shared relation")
	}
}
