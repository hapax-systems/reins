package grammar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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

func TestRenderSessionRowLocalAndAIR(t *testing.T) {
	s := Session{
		Role: "cx-p0", Session: "hapax-codex-cx-p0", Platform: "codex", State: "active",
		Readiness: "claim", Blocker: "none", Attention: 0.92,
		Alive: true, ClaimedTask: "PRIVATE-TASK", OutputAgeS: 42, RelayAgeS: 3600,
		AIR: map[string]string{
			"role": "ok", "platform": "ok", "state": "ok", "alive": "ok", "idle": "ok", "stalled": "ok",
			"readiness": "ok", "blocker": "ok", "attention": "ok",
			"session": "deny", "claimed_task": "deny", "output_age_s": "ok", "relay_age_s": "ok",
		},
	}
	local := RenderSessionRow(s, false)
	if !strings.Contains(local, "cx-p0") || !strings.Contains(local, "claim") || !strings.Contains(local, "PRIVATE-TASK") {
		t.Fatalf("local session row missing fields: %q", local)
	}
	air := RenderSessionRow(s, true)
	for _, leak := range []string{"hapax-codex-cx-p0", "PRIVATE-TASK"} {
		if strings.Contains(air, leak) {
			t.Fatalf("AIR session row leaked denied %q: %q", leak, air)
		}
	}
	if !strings.Contains(air, "cx-p0") || !strings.Contains(air, "▒▒▒") {
		t.Fatalf("AIR session row should keep structural role and redact denied fields: %q", air)
	}
}

func TestRenderSessionRowRedactsHealthGlyphWhenDerivedFieldsDenied(t *testing.T) {
	s := Session{
		Role: "cx", Platform: "codex", State: "active", Readiness: "live", Alive: true,
		AIR: map[string]string{"role": "ok", "platform": "ok", "state": "deny", "readiness": "deny", "attention": "deny", "alive": "deny", "idle": "deny", "stalled": "deny"},
	}
	air := RenderSessionRow(s, true)
	if strings.Contains(air, "●") || strings.Contains(air, "active") {
		t.Fatalf("AIR session row leaked denied health state through glyph/text: %q", air)
	}
	if !strings.Contains(air, "▒") {
		t.Fatalf("AIR session row should keep health structure with redaction: %q", air)
	}
}

func TestRenderSessionDoorAIRRedactsOperationalHandles(t *testing.T) {
	s := Session{
		Role: "cx-p0", Session: "SECRET-TMUX", Platform: "codex", State: "active", Alive: true,
		Readiness: "claim", Blocker: "none", Attention: 0.91,
		ClaimedTask: "SECRET-TASK", OutputAgeS: 12, RelayAgeS: 34,
		RouteID: "codex.headless.full", RouteMode: "headless", RouteProfile: "full",
		RouteBindingState: "policy_only", RouteEvidenceRef: "SECRET-ROUTE-REF",
		AIR: map[string]string{
			"role": "ok", "platform": "ok", "state": "ok", "alive": "ok", "idle": "ok", "stalled": "ok",
			"readiness": "ok", "blocker": "ok", "attention": "ok", "route_id": "ok", "mode": "ok", "profile": "ok",
			"route_binding_state": "ok",
			"session":             "deny", "claimed_task": "deny", "output_age_s": "ok", "relay_age_s": "ok",
		},
	}
	local := RenderSessionDoor(s, SessionDetail{}, false, false, "", false, 100, 34)
	for _, want := range []string{"DOOR /session", "SECRET-TMUX", "SECRET-TASK", "ROUTE BINDING", "SECRET-ROUTE-REF", "RESUME CONTRACT"} {
		if !strings.Contains(local, want) {
			t.Fatalf("local session door missing %q:\n%s", want, local)
		}
	}
	air := RenderSessionDoor(s, SessionDetail{}, false, false, "", true, 100, 34)
	for _, leak := range []string{"SECRET-TMUX", "SECRET-TASK", "SECRET-ROUTE-REF"} {
		if strings.Contains(air, leak) {
			t.Fatalf("AIR session door leaked denied %q:\n%s", leak, air)
		}
	}
	if !strings.Contains(air, "cx-p0") || !strings.Contains(air, "policy_only") || !strings.Contains(air, "▒▒▒") || !strings.Contains(air, "No command dispatch") {
		t.Fatalf("AIR session door should keep structure/contract and redact handles:\n%s", air)
	}
}

func TestRenderSessionDoorDetailAIRRedactsRefs(t *testing.T) {
	s := Session{Role: "cx-p0", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.9,
		AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "alive": "ok", "idle": "ok", "stalled": "ok"}}
	d := SessionDetail{
		Role:         "cx-p0",
		Task:         SessionTaskDetail{TaskID: "SECRET-TASK", Status: "claimed", AuthorityCase: "SECRET-CASE", ParentSpec: "SECRET-SPEC"},
		EvidenceRefs: []EvidenceRef{{Kind: "transcript_candidate", Path: "/secret/transcript.jsonl", Size: 12}},
		AIR:          map[string]string{"status": "ok", "task_id": "deny", "authority_case": "deny", "parent_spec": "deny", "path": "deny"},
	}
	air := RenderSessionDoor(s, d, true, false, "", true, 120, 40)
	for _, leak := range []string{"SECRET-TASK", "SECRET-CASE", "SECRET-SPEC", "/secret/transcript.jsonl"} {
		if strings.Contains(air, leak) {
			t.Fatalf("AIR session detail leaked denied %q:\n%s", leak, air)
		}
	}
	if !strings.Contains(air, "EVIDENCE REFS") || !strings.Contains(air, "claimed") || !strings.Contains(air, "▒▒▒") {
		t.Fatalf("AIR session detail should keep structure/status and redact refs:\n%s", air)
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
	// drift guard (class-closure): every EVENT-kind glyph a :events row can show must also be
	// decodable in the legend — a cold/on-air viewer must not meet an un-situated mark (⇡/⚑/↟/⚙/✶).
	for kind, g := range glyphs {
		if !strings.Contains(leg, g) {
			t.Fatalf("legend missing event glyph %q for kind %q", g, kind)
		}
	}
	if generic := Glyph("__never_a_real_kind__"); !strings.Contains(leg, generic) {
		t.Fatalf("legend missing the generic/fallback event glyph %q", generic)
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

func sampleTrace() Trace {
	return Trace{
		TS: "2026-06-26T12:00:00Z", TraceID: "trace-1", Model: "claude-opus-4",
		PromptTok: 100, CompletionTok: 50, TotalTok: 150, Cost: 0.012345, LatencyMs: 2500,
		AIR: map[string]string{
			"ts": "ok", "trace_id": "ok", "model": "ok", "latency_ms": "ok",
			"prompt_tok": "ok", "completion_tok": "ok", "total_tok": "ok", "cost": "ok",
		},
	}
}

func TestRenderTraceRowLocal(t *testing.T) {
	got := RenderTraceRow(sampleTrace(), false)
	for _, want := range []string{"12:00:00", "trace-1", "claude-opus-4", "100/50/150", "$0.012345"} {
		if !strings.Contains(got, want) {
			t.Fatalf("local trace row missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "2500ms") {
		t.Fatalf("local trace row should carry the latency value:\n%s", got)
	}
}

func TestRenderTraceRowAIRRedactsDenied(t *testing.T) {
	tr := sampleTrace()
	tr.AIR = map[string]string{"ts": "ok", "trace_id": "deny", "model": "deny", "latency_ms": "ok", "total_tok": "ok", "cost": "ok"}
	got := RenderTraceRow(tr, true)
	for _, leak := range []string{"trace-1", "claude-opus-4"} {
		if strings.Contains(got, leak) {
			t.Fatalf("AIR trace row leaked denied %q:\n%s", leak, got)
		}
	}
	if !strings.Contains(got, "▒") {
		t.Fatalf("AIR trace row must show the redaction glyph for denied fields:\n%s", got)
	}
	if !strings.Contains(got, "$0.012345") { // allowlisted operational metadata survives AIR
		t.Fatalf("AIR trace row should keep allowlisted cost:\n%s", got)
	}
}

func TestTraceGlyphsAreMonochromeSafe(t *testing.T) {
	// latency is a magnitude -> carried by SHAPE (bar height), never hue. Distinct magnitudes
	// must produce distinct bars; both bars stay on the shared-ground token so they touch none
	// of the three meaning-channels (criticality-hue / freshness-brightness / ownership-family).
	lo, hi := latencyHistogram(50), latencyHistogram(5000)
	if lo == hi {
		t.Fatalf("latency bar must differ by magnitude: lo=%q hi=%q", lo, hi)
	}
	// channel-safety + Peirce redundant-label: strip ALL color and the precise labels still read.
	strip := ansi.Strip(RenderTraceRow(sampleTrace(), false))
	for _, want := range []string{"2500ms", "$0.012345"} {
		if !strings.Contains(strip, want) {
			t.Fatalf("monochrome trace row lost a label that must read without color: %q\n%s", want, strip)
		}
	}
}

// --- session-pane turn grammar (§9 step-1 read-projection scaffold) ---

func sampleTurn() Turn {
	return Turn{
		TS: "2026-06-26T12:00:00Z", Role: "cc-reins", Kind: "assistant",
		Summary: "proposed edit to grammar.go", Magnitude: 0.6,
		Model: "claude-opus-4", Route: "claude.code.full", Gate: "pass",
		// SKELETON airs (allowlisted); the BODY (summary) is omitted => default-deny on air.
		AIR: map[string]string{"ts": "ok", "kind": "ok", "role": "ok", "model": "ok", "route": "ok", "gate": "ok", "magnitude": "ok"},
	}
}

func TestRenderTurnRowLocal(t *testing.T) {
	got := RenderTurnRow(sampleTurn(), false)
	for _, want := range []string{turnGlyph["assistant"], "cc-reins", "claude-opus-4", "proposed edit"} {
		if !strings.Contains(got, want) {
			t.Fatalf("turn row missing %q:\n%q", want, got)
		}
	}
}

func TestRenderTurnRowAIRRedactsBodyKeepsSkeleton(t *testing.T) {
	got := RenderTurnRow(sampleTurn(), true)
	// the body default-denies...
	if strings.Contains(got, "proposed edit") {
		t.Fatalf("AIR must redact the turn body:\n%q", got)
	}
	if !strings.Contains(got, "▒") {
		t.Fatalf("AIR turn row must show the redaction glyph for the denied body:\n%q", got)
	}
	// ...while the air-safe skeleton survives (the livestream-safe projection of a turn).
	for _, want := range []string{turnGlyph["assistant"], "cc-reins", "claude-opus-4"} {
		if !strings.Contains(got, want) {
			t.Fatalf("AIR turn row dropped air-safe skeleton %q:\n%q", want, got)
		}
	}
}

func TestRenderTurnRowOperatorFreeTextNeverAirs(t *testing.T) {
	tk := sampleTurn()
	tk.Kind, tk.Summary = "user", "my private prompt text"
	// operator free-text: body has no "summary":"ok" => deny-by-default on air, ALWAYS.
	got := RenderTurnRow(tk, true)
	if strings.Contains(got, "private prompt") {
		t.Fatalf("operator free-text must never air:\n%q", got)
	}
}

func TestLegendCoversTurnGlyphs(t *testing.T) {
	leg := RenderLegend()
	for kind, g := range turnGlyph {
		if !strings.Contains(leg, g) || turnKindGloss[kind] == "" {
			t.Fatalf("legend missing turn glyph %q for kind %q (or its gloss)", g, kind)
		}
	}
}

func TestTurnGlyphsAreSingleRune(t *testing.T) {
	// width-determinism guard: a multi-rune glyph (ZWJ/combining/emoji-presentation) desyncs the
	// cell grid on-air and under tmux — a correctness hazard, not aesthetics.
	for kind, g := range turnGlyph {
		if r := []rune(g); len(r) != 1 {
			t.Fatalf("turn glyph for %q must be a single rune: %q (%d runes)", kind, g, len(r))
		}
	}
}

func sampleTurnBlocks() []TurnBlock {
	ok := func() map[string]string { return map[string]string{"kind": "ok", "meta": "ok"} }
	return []TurnBlock{
		{Kind: "reasoning", Summary: "widen the timeout, stub the clock", Magnitude: 0.4, AIR: map[string]string{"kind": "ok"}},
		{Kind: "tool_call", Summary: "go test ./... -run Trace", Magnitude: 0.5, Meta: "Bash", AIR: ok()},
		{Kind: "tool_result", Summary: "ok internal/grammar 0.004s", Magnitude: 0.6, Meta: "exit 0 · 42 lines", AIR: ok()},
		{Kind: "diff", Summary: "grammar_test.go hunk", Magnitude: 0.3, Meta: "+6 -2", AIR: ok()},
	}
}

func TestRenderTurnDetailLocal(t *testing.T) {
	got := RenderTurnDetail(sampleTurn(), sampleTurnBlocks(), false)
	for _, want := range []string{turnGlyph["tool_call"], "Bash", "go test", "exit 0", "+6 -2", turnGlyph["diff"]} {
		if !strings.Contains(got, want) {
			t.Fatalf("turn detail missing %q:\n%s", want, got)
		}
	}
}

func TestRenderTurnDetailAIRRedactsBodiesKeepsMeta(t *testing.T) {
	got := RenderTurnDetail(sampleTurn(), sampleTurnBlocks(), true)
	if strings.Contains(got, "go test ./... -run Trace") {
		t.Fatalf("AIR must redact block bodies:\n%s", got)
	}
	if !strings.Contains(got, "▒") {
		t.Fatalf("AIR detail must show the redaction glyph:\n%s", got)
	}
	for _, want := range []string{"Bash", "exit 0", turnGlyph["tool_call"]} { // meta + glyph are air-safe skeleton
		if !strings.Contains(got, want) {
			t.Fatalf("AIR detail dropped air-safe meta %q:\n%s", want, got)
		}
	}
}

func TestRenderTurnDetailIsTree(t *testing.T) {
	got := RenderTurnDetail(sampleTurn(), sampleTurnBlocks(), false)
	if !strings.Contains(got, "├─") || !strings.Contains(got, "└─") {
		t.Fatalf("turn detail must render an ASCII tree (├─ … └─):\n%s", got)
	}
}

func TestRenderTraceHeaderAligns(t *testing.T) {
	got := RenderTraceHeader()
	for _, want := range []string{"TIME", "LATENCY", "TRACE", "MODEL", "TOKENS", "COST"} {
		if !strings.Contains(got, want) {
			t.Fatalf("trace header missing column %q:\n%s", want, got)
		}
	}
}
