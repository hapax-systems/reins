package relate

import (
	"sort"
	"testing"
)

// The brush set: Derive must report EVERY pane-B id participating in the winning relation (so the
// caller can highlight the related rows), not only the single strongest peer.

func TestDerivePeersFacetReturnsAllSharers(t *testing.T) {
	anchor := Entity{ID: "a", Facets: map[string]string{"owner": "alpha"}}
	others := []Entity{
		{ID: "b", Facets: map[string]string{"owner": "alpha"}},
		{ID: "c", Facets: map[string]string{"owner": "beta"}},
		{ID: "d", Facets: map[string]string{"owner": "alpha"}},
	}
	r := Derive(anchor, others, nil)
	if r.Kind != "facet" || r.Count != 2 {
		t.Fatalf("expected facet relation shared by 2; got %+v", r)
	}
	got := append([]string{}, r.Peers...)
	sort.Strings(got)
	if len(got) != 2 || got[0] != "b" || got[1] != "d" {
		t.Fatalf("Peers must be exactly the facet-sharers {b,d}; got %v", got)
	}
}

func TestDerivePeersEdgeReturnsAllTypedPeers(t *testing.T) {
	anchor := Entity{ID: "a"}
	others := []Entity{{ID: "b"}, {ID: "c"}, {ID: "d"}}
	edges := []Edge{
		{Src: "a", Dst: "b", Type: "blocks", Weight: 0.9},
		{Src: "c", Dst: "a", Type: "blocks", Weight: 0.5},
		{Src: "a", Dst: "d", Type: "feeds", Weight: 0.4},
	}
	r := Derive(anchor, others, edges)
	if r.Kind != "edge" || r.Type != "blocks" {
		t.Fatalf("expected the strongest edge to be blocks; got %+v", r)
	}
	got := append([]string{}, r.Peers...)
	sort.Strings(got)
	if len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Fatalf("Peers must be all 'blocks' peers {b,c}, excluding the 'feeds' peer d; got %v", got)
	}
}

func TestDerivePeersNoneIsEmpty(t *testing.T) {
	r := Derive(Entity{ID: "a"}, []Entity{{ID: "b"}}, nil)
	if r.Kind != "none" || len(r.Peers) != 0 {
		t.Fatalf("an empty relation must carry no brush peers; got %+v", r)
	}
}
