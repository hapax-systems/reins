package grammar

import (
	"strings"
	"testing"
)

func columnRailGraph() Graph {
	return Graph{
		MapID:  "dyn-test",
		Thesis: "column rail test graph",
		Layers: []Layer{
			{ID: "sense", Label: "Sense"},
			{ID: "decide", Label: "Decide"},
			{ID: "act", Label: "Act"},
		},
		Nodes: []Node{
			{ID: "input", Label: "Input", Layer: "sense", Status: "observed", Res: "1", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
			{ID: "policy", Label: "Policy", Layer: "decide", Status: "asserted", Res: "1", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
			{ID: "motion", Label: "Motion", Layer: "act", Status: "candidate", Res: "2", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
			{ID: "feedback", Label: "Feedback", Layer: "sense", Status: "inferred", Res: "2", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
		},
		Edges: []Edge{
			{Source: "input", Target: "policy", Relation: "feeds", Status: "observed"},
			{Source: "policy", Target: "motion", Relation: "selects", Status: "asserted"},
			{Source: "input", Target: "feedback", Relation: "updates", Status: "inferred"},
		},
	}
}

func TestRenderColumnRailDeterministic(t *testing.T) {
	g := columnRailGraph()
	first := RenderColumnRail(g, 0, false, 96)
	second := RenderColumnRail(g, 0, false, 96)
	if first != second {
		t.Fatalf("RenderColumnRail must be deterministic for the same seed graph:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestRenderColumnRailHasLayerAndNodeID(t *testing.T) {
	got := RenderColumnRail(columnRailGraph(), 0, false, 96)
	for _, want := range []string{"SENSE", "policy"} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderColumnRail missing %q:\n%s", want, got)
		}
	}
}

func TestRenderColumnRailAIRRedactsDeniedLabelKeepsTopology(t *testing.T) {
	g := Graph{
		Layers: []Layer{{ID: "source", Label: "Source"}, {ID: "target", Label: "Target"}},
		Nodes: []Node{
			{ID: "src", Label: "Public Source", Layer: "source", Status: "asserted", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
			{ID: "dst", Label: "Secret Target Label", Layer: "target", Status: "observed", AIR: map[string]string{"id": "ok", "label": "deny", "status": "ok"}},
		},
		Edges: []Edge{{Source: "src", Target: "dst", Relation: "feeds", Status: "asserted"}},
	}
	got := RenderColumnRail(g, 0, true, 72)
	if strings.Contains(got, "Secret Target Label") {
		t.Fatalf("AIR leaked a denied node label:\n%s", got)
	}
	for _, want := range []string{"dst", statusGlyph("observed", map[string]string{"status": "ok"}, true), "▶"} {
		if !strings.Contains(got, want) {
			t.Fatalf("AIR render should keep node identity/glyph/topology %q:\n%s", want, got)
		}
	}
}
