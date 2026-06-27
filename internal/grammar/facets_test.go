package grammar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderFacetLegendDecodesVocabularyInOrder(t *testing.T) {
	reg := FacetRegistry{
		Facets: map[string]FacetDef{
			"identity": {Gloss: "what it is", Channel: "text + selection-shape"},
			"posture":  {Gloss: "what state it's in", Channel: "criticality-hue"},
			"measure":  {Gloss: "how much", Channel: "eighth-block bar"},
		},
		CitationOrder: []string{"identity", "posture", "measure"},
	}
	out := ansi.Strip(RenderFacetLegend(reg))
	for _, want := range []string{"FACETS", "identity", "what it is", "criticality-hue", "eighth-block bar"} {
		if !strings.Contains(out, want) {
			t.Fatalf("facet legend missing %q:\n%s", want, out)
		}
	}
	// citation order is honored
	if strings.Index(out, "identity") > strings.Index(out, "posture") {
		t.Fatal("facets must render in citation order")
	}
}

func TestRenderFacetLegendFallsBackToSortedKeys(t *testing.T) {
	reg := FacetRegistry{Facets: map[string]FacetDef{"zeta": {Gloss: "z"}, "alpha": {Gloss: "a"}}}
	out := ansi.Strip(RenderFacetLegend(reg))
	if strings.Index(out, "alpha") > strings.Index(out, "zeta") {
		t.Fatal("with no citation order, facets sort by key")
	}
}
