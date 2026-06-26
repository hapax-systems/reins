package grammar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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
	for _, want := range []string{"dst", statusGlyph("observed", map[string]string{"status": "ok"}, true), "→"} {
		if !strings.Contains(got, want) {
			t.Fatalf("AIR render should keep node identity/glyph/topology %q:\n%s", want, got)
		}
	}
}

func TestRenderColumnRailEdgesDoNotOverwriteNodeLabels(t *testing.T) {
	g := Graph{
		Layers: []Layer{
			{ID: "a", Label: "A"},
			{ID: "b", Label: "B"},
			{ID: "c", Label: "C"},
		},
		Nodes: []Node{
			{ID: "src", Label: "Source", Layer: "a", Status: "observed", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
			{ID: "mid", Label: "Middle", Layer: "b", Status: "observed", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
			{ID: "dst", Label: "Target", Layer: "c", Status: "asserted", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
		},
		Edges: []Edge{{Source: "src", Target: "dst", Relation: "skips", Status: "observed"}},
	}

	got := RenderColumnRail(g, 0, false, 96)
	for _, want := range []string{"src Source", "mid Middle", "dst Target", "→"} {
		if !strings.Contains(got, want) {
			t.Fatalf("column rail should preserve node text and flow glyph %q:\n%s", want, got)
		}
	}
}

func TestRenderColumnRailFrameAnimatesEdgesWithoutMovingTopology(t *testing.T) {
	g := columnRailGraph()
	frame0 := RenderColumnRailFrame(g, 0, false, 96, 0)
	frame1 := RenderColumnRailFrame(g, 0, false, 96, 1)
	if frame0 == frame1 {
		t.Fatalf("frame phase should move a flow mark without rebuilding topology:\n%s", frame0)
	}
	for _, frame := range []string{frame0, frame1} {
		for _, want := range []string{"SENSE", "input Input", "policy Policy", "→", "•"} {
			if !strings.Contains(frame, want) {
				t.Fatalf("animated rail frame missing stable topology/flow mark %q:\n%s", want, frame)
			}
		}
		for i, line := range strings.Split(frame, "\n") {
			if got := ansi.StringWidth(ansi.Strip(line)); got > 96 {
				t.Fatalf("animated rail line %d exceeds width: %d %q", i, got, line)
			}
		}
	}
}

func TestRenderColumnRailFrameCapsFlowMarks(t *testing.T) {
	g := Graph{
		Layers: []Layer{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
	}
	for i := 0; i < 12; i++ {
		src := "src" + string(rune('a'+i))
		dst := "dst" + string(rune('a'+i))
		g.Nodes = append(g.Nodes,
			Node{ID: src, Label: "Source", Layer: "a", Status: "observed", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
			Node{ID: dst, Label: "Target", Layer: "b", Status: "asserted", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
		)
		g.Edges = append(g.Edges, Edge{Source: src, Target: dst, Relation: "feeds", Status: "observed"})
	}

	frame := RenderColumnRailFrame(g, 0, false, 96, 0)
	marks := strings.Count(frame, "•")
	if marks == 0 || marks > columnRailMaxFlowMarks {
		t.Fatalf("animated rail should cap flow marks to 1..%d, got %d:\n%s", columnRailMaxFlowMarks, marks, frame)
	}
}

func TestRenderColumnRailFrameFocusedMarksNodeAndEdge(t *testing.T) {
	g := columnRailGraph()
	nodeFrame := ansi.Strip(RenderColumnRailFrameFocused(g, 0, false, 96, 0, ColumnRailFocus{Kind: "node", ID: "input"}))
	if !strings.Contains(nodeFrame, "▶input") {
		t.Fatalf("focused rail should mark the selected node without moving its label:\n%s", nodeFrame)
	}

	edgeFrame := ansi.Strip(RenderColumnRailFrameFocused(g, 0, false, 96, 0, ColumnRailFocus{Kind: "edge", Source: "input", Target: "policy", Relation: "feeds"}))
	if !strings.Contains(edgeFrame, "◆") {
		t.Fatalf("focused rail should mark the selected edge path:\n%s", edgeFrame)
	}
}
