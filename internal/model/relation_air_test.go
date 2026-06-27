package model

import (
	"strings"
	"testing"

	"github.com/hapax-systems/reins/internal/relate"
)

// The emergent connector relation is DEFAULT-DENY on air: only structural facet values (stage/crit/
// score) air; every sensitive or unknown facet value — and edge peer ids — are withheld, so the
// connector can never leak PII on the livestream (the Inc 1 `kind` leak class, closed structurally).
func TestAirRelationLabelDefaultDeny(t *testing.T) {
	m := New("R")

	m.AIR = true
	if got := m.airRelationLabel(relate.Relation{Kind: "facet", Facet: "stage", Value: "S7", Count: 5, Label: "shares stage=S7 (5)"}); !strings.Contains(got, "S7") {
		t.Fatalf("a structural facet value should air: %q", got)
	}
	for _, f := range []string{"owner", "case", "kind", "actor", "subject", "role", "newunknownfacet"} {
		got := m.airRelationLabel(relate.Relation{Kind: "facet", Facet: f, Value: "SECRET", Count: 3, Label: "shares " + f + "=SECRET (3)"})
		if strings.Contains(got, "SECRET") {
			t.Fatalf("non-structural facet %q must withhold its value on air: %q", f, got)
		}
	}
	if strings.Contains(m.airRelationLabel(relate.Relation{Kind: "edge", Type: "blocks", Peer: "#4278", Label: "blocks #4278"}), "#4278") {
		t.Fatal("an edge peer id must be withheld on air")
	}

	m.AIR = false
	if got := m.airRelationLabel(relate.Relation{Kind: "facet", Facet: "owner", Value: "alpha", Count: 3, Label: "shares owner=alpha (3)"}); !strings.Contains(got, "alpha") {
		t.Fatalf("off air shows the full value: %q", got)
	}
}
