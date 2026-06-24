package grammar

import (
	"strings"
	"testing"
)

func sample() Event {
	return Event{TS: "14:22", Kind: "pr.merged", Subject: "4284", Actor: "alpha",
		Summary: "PR#4284 merged to main", Score: 0.7,
		AIR: map[string]string{"subject": "ok", "actor": "deny", "summary": "deny"}}
}

func dynGraph() Graph {
	return Graph{
		Layers: []Layer{{ID: "semantic-backbone", Label: "Semantic Backbone"}},
		Nodes: []Node{{ID: "rdf-owl-kg", Label: "RDF/OWL KG", Layer: "semantic-backbone", Status: "asserted",
			AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}}},
		Edges: []Edge{{Source: "rdf-owl-kg", Target: "shacl", Relation: "validated_by", Status: "asserted",
			AIR: map[string]string{"target": "ok", "relation": "ok", "status": "ok"}}},
	}
}

func TestRenderDynamicsHasLayerNodeEdge(t *testing.T) {
	v := RenderDynamics(dynGraph(), false)
	for _, want := range []string{"SEMANTIC BACKBONE", "rdf-owl-kg", statusGlyph("asserted", nil, false), "shacl", "validated_by"} {
		if !strings.Contains(v, want) {
			t.Fatalf("dynamics render missing %q:\n%s", want, v)
		}
	}
}

func TestGraphAtResolutionFiltersNodesAndDanglingEdges(t *testing.T) {
	g := Graph{
		Layers: []Layer{{ID: "L", Label: "L"}},
		Nodes: []Node{
			{ID: "a", Layer: "L", Res: "1"}, // overview
			{ID: "b", Layer: "L", Res: "3"}, // artifact
			{ID: "c", Layer: "L", Res: ""},  // unknown -> always kept
		},
		Edges: []Edge{
			{Source: "a", Target: "c"}, // both kept at res<=1
			{Source: "a", Target: "b"}, // b drops at res<=1 -> edge drops
		},
	}
	at1 := g.AtResolution(1)
	if len(at1.Nodes) != 2 { // a (res1) + c (unknown)
		t.Fatalf("AtResolution(1) should keep res<=1 + unknown: got %d", len(at1.Nodes))
	}
	if len(at1.Edges) != 1 { // a->c kept; a->b dropped (b filtered)
		t.Fatalf("AtResolution must drop edges to filtered nodes: got %d", len(at1.Edges))
	}
	if len(g.AtResolution(0).Nodes) != 3 {
		t.Fatal("AtResolution(0) means all")
	}
}

func TestRenderDynamicsAIRRedactsLabelAndStatusGlyph(t *testing.T) {
	g := Graph{
		Layers: []Layer{{ID: "L", Label: "L"}},
		Nodes: []Node{{ID: "n1", Label: "Secret Label", Layer: "L", Status: "asserted",
			AIR: map[string]string{"id": "ok", "label": "deny", "status": "deny"}}},
	}
	v := RenderDynamics(g, true)
	if strings.Contains(v, "Secret Label") {
		t.Fatalf("AIR leaked a denied node label: %q", v)
	}
	if !strings.Contains(v, "▒") {
		t.Fatalf("AIR must redact the denied status glyph: %q", v)
	}
}

func TestCompactTS(t *testing.T) {
	cases := map[string]string{
		"2026-06-24T01:53:07Z":        "01:53:07",
		"2026-06-24T01:53:07.123456Z": "01:53:07",
		"2026-06-24T01:53:07+00:00":   "01:53:07",
		"14:22":                       "14:22   ", // no 'T' -> padded passthrough
	}
	for in, want := range cases {
		if got := compactTS(in); got != want {
			t.Fatalf("compactTS(%q)=%q want %q", in, got, want)
		}
	}
}

func TestRenderEventRowLocal(t *testing.T) {
	got := RenderEventRow(sample(), false)
	if !strings.Contains(got, Glyph("pr.merged")) || !strings.Contains(got, "4284") || !strings.Contains(got, "merged to main") {
		t.Fatalf("local row missing fields: %q", got)
	}
}

func TestRenderEventRowAIRRedactsDenied(t *testing.T) {
	got := RenderEventRow(sample(), true)
	if strings.Contains(got, "merged to main") {
		t.Fatalf("AIR row leaked a denied field: %q", got)
	}
	if !strings.Contains(got, "4284") || !strings.Contains(got, "▒") {
		t.Fatalf("AIR row should keep allowlisted subject + show redaction glyph: %q", got)
	}
}

func TestRenderWhoisDoor(t *testing.T) {
	tk := Task{TaskID: "door-x", Stage: "S7_RELEASE", PriorStage: "S6_IMPL", PredictedStage: "hold",
		Owner: "cc-a", Criticality: "warn", NoGo: "docs_mutation_authorized,implementation_authorized",
		AuthorityCase: "CASE-1", AIR: map[string]string{"task_id": "ok", "stage": "ok"}}
	v := RenderWhoisDoor(tk, false, 100, 30)
	for _, want := range []string{"door-x", "S7", "LADDER", "arm", "rework"} {
		if !strings.Contains(v, want) {
			t.Fatalf("door missing %q:\n%s", want, v)
		}
	}
	// AIR redacts the denied authority case but keeps the structure (the ladder, the labels)
	air := RenderWhoisDoor(tk, true, 100, 30)
	if strings.Contains(air, "CASE-1") || !strings.Contains(air, "LADDER") {
		t.Fatalf("AIR door must redact the authority value but keep structure:\n%s", air)
	}
}

func TestSelLabelKeepsTextMonochromeSafe(t *testing.T) {
	// the selection swatch must never destroy its text (a label must survive a grayscale strip).
	out := SelLabel("[i]")
	if !strings.Contains(out, "[i]") {
		t.Fatalf("SelLabel must keep its text: %q", out)
	}
}

func TestLegendCoversAllGlyphMaps(t *testing.T) {
	// drift guard: every glyph the renderers use must have a legend entry + a gloss.
	leg := RenderLegend()
	for k, g := range critGlyph {
		if !strings.Contains(leg, g) || critStateGloss[k] == "" {
			t.Fatalf("legend missing crit state %q (%s) or its gloss", k, g)
		}
	}
	for k, g := range statusGlyphs {
		if !strings.Contains(leg, g) || provGloss[k] == "" {
			t.Fatalf("legend missing provenance %q (%s) or its gloss", k, g)
		}
	}
}

func TestGlyphIsStableAndMonochromeSafe(t *testing.T) {
	if Glyph("pr.merged") == Glyph("review.fail") {
		t.Fatal("distinct kinds must have distinct glyphs (the glyph carries the kind)")
	}
}

func sampleTask() Task {
	return Task{TaskID: "event-spine-coord-event-log-20260623", Stage: "S6", NoGo: "",
		AIR: map[string]string{"task_id": "ok", "stage": "ok", "no_go": "ok"}}
}

func TestRenderTaskRowLocal(t *testing.T) {
	got := RenderTaskRow(sampleTask(), false)
	if !strings.Contains(got, critGlyph["ok"]) || !strings.Contains(got, "event-spine") || !strings.Contains(got, "S6") {
		t.Fatalf("task row missing state glyph / id / stage: %q", got)
	}
}

func TestRenderTaskRowSevenDims(t *testing.T) {
	tk := Task{TaskID: "x-1", Stage: "S5_DESIGN", PriorStage: "S4_PLAN", PredictedStage: "hold",
		Owner: "cc-seg", Freshness: 0.9, Criticality: "crit",
		AIR: map[string]string{"task_id": "ok", "stage": "ok", "prior_stage": "ok", "predicted_stage": "ok", "owner": "ok", "criticality": "ok", "freshness": "ok"}}
	got := RenderTaskRow(tk, false)
	for _, want := range []string{critGlyph["crit"], "x-1", "S5", "S4", "hold", "cc-seg", critBar("crit")} {
		if !strings.Contains(got, want) {
			t.Fatalf("7-dim row missing %q:\n%q", want, got)
		}
	}
}

func TestRenderTaskRowStructuredSilence(t *testing.T) {
	got := RenderTaskRow(sampleTask(), false) // empty no_go -> dots, not blank jitter
	if !strings.Contains(got, "····") {
		t.Fatalf("empty cell must be structured-silence dots: %q", got)
	}
}

func TestRenderTaskRowAIRRedacts(t *testing.T) {
	tk := sampleTask()
	tk.AIR = map[string]string{"task_id": "ok", "stage": "deny", "no_go": "ok"}
	got := RenderTaskRow(tk, true)
	if strings.Contains(got, "S6") {
		t.Fatalf("AIR must redact the denied stage: %q", got)
	}
	if !strings.Contains(got, "event-spine") {
		t.Fatalf("AIR must keep the allowlisted task_id: %q", got)
	}
}
