package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func withReadAPI(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	old := newReadHTTPClient
	newReadHTTPClient = func() *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			return rec.Result(), nil
		})}
	}
	t.Cleanup(func() { newReadHTTPClient = old })
	return "http://reins.test"
}

func TestFetchIntakeTreatsHTTP404AsDark(t *testing.T) {
	apiURL := withReadAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"Not Found"}`))
	})

	_, dark, err := FetchIntake(apiURL)
	if err == nil {
		t.Fatal("FetchIntake should return an error for a missing endpoint")
	}
	if !dark {
		t.Fatal("FetchIntake should darken the read source on HTTP 404")
	}
	if !strings.Contains(err.Error(), "/read/intake returned 404") {
		t.Fatalf("FetchIntake error should name the missing endpoint and status, got %q", err.Error())
	}
}

func TestFetchRouteDecodesMeasuredVsAbsent(t *testing.T) {
	// A candidate's dispatch_reqvec is measured ONLY when a COMPLETE 8-dim object decodes; a partial
	// object, the "absent" string, or a missing key must yield ReqvecMeasured=false (render says ABSENT,
	// never fabricated zeros). This pins the decode-side ABSENT honesty the U5 review flagged.
	apiURL := withReadAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/route/posture":
			_, _ = w.Write([]byte(`{"dark":false,"decision":"NO SPINE DECISION ON FILE"}`))
		case "/route/candidates":
			_, _ = w.Write([]byte(`{"dark":false,"decision":"NO SPINE DECISION ON FILE","task_reqvec":"absent","candidates":[
				{"routing_class":"complete","in_keyspace":true,"measured_events":2,"dispatch_reqvec":{"quality_floor":5,"information_scope":1,"context_length":1,"mutation_risk":3,"verification_demand":3,"ambiguity_novelty":3,"composition_coupling":4,"governance_sensitivity":1}},
				{"routing_class":"partial","in_keyspace":true,"measured_events":1,"dispatch_reqvec":{"quality_floor":5}},
				{"routing_class":"absent_str","in_keyspace":true,"measured_events":1,"dispatch_reqvec":"absent"}
			]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	_, cands, dark, err := FetchRoute(apiURL)
	if err != nil || dark {
		t.Fatalf("FetchRoute unexpected err=%v dark=%v", err, dark)
	}
	by := map[string]bool{}
	for _, c := range cands {
		by[c.RoutingClass] = c.ReqvecMeasured
	}
	if !by["complete"] {
		t.Fatal("a complete 8-dim vector must decode as measured")
	}
	if by["partial"] {
		t.Fatal("a partial vector must NOT be measured (would fabricate zeros) — must render ABSENT")
	}
	if by["absent_str"] {
		t.Fatal("the \"absent\" string must NOT be measured")
	}
}

func TestFetchDomainsReadsSourceBackedSummary(t *testing.T) {
	apiURL := withReadAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/read/domains" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"dark": false,
			"domains": {
				"sources": [{"id":"pack","status":"observed","count":1}],
				"rows": [{"domain_id":"rdlc","lifecycle":"RDLC","state":"candidate","air":{"domain_id":"ok"}}],
				"relations": [],
				"totals": {"sources":1,"rows":1,"relations":0},
				"authority": "CASE-DOMAIN",
				"generated_at": "2026-06-25T10:00:00Z",
				"package_hash": "sha256:test",
				"default_lens": "lifecycle",
				"lifecycle_sources": [{"id":"lifecycle-registry","status":"observed","count":1}],
				"lifecycles": [{"lifecycle_id":"ldlc","state":"dark_specified","maturity":"declared-not-modeled","air":{"lifecycle_id":"ok","state":"ok","maturity":"ok"}}],
				"lifecycle_totals": {"sources":1,"rows":1,"missing_sources":0},
				"lifecycle_authority": "support_non_authoritative"
			}
		}`))
	})

	domains, dark, err := FetchDomains(apiURL)
	if err != nil {
		t.Fatal(err)
	}
	if dark {
		t.Fatal("domains should not be dark")
	}
	if domains.Authority != "CASE-DOMAIN" || len(domains.Rows) != 1 || domains.Rows[0].DomainID != "rdlc" || len(domains.Lifecycles) != 1 || domains.Lifecycles[0].LifecycleID != "ldlc" {
		t.Fatalf("bad domain summary: %+v", domains)
	}
}

func TestFetchDynamicsReadsThesis(t *testing.T) {
	apiURL := withReadAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/read/dynamics" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"dark": false,
			"map_id": "system-dynamics-map-v1",
			"thesis": "source-neutral semantic graph backbone",
			"layers": [{"id":"backbone","label":"Backbone"}],
			"nodes": [{"id":"n1","label":"Node","layer":"backbone","status":"asserted","summary":"Node summary","context":"Node context","docs":"Doc Label","hardening_notes":"Validate it","aliases":"node alias","tags":"tag-a","source_refs":"docs:1 refs","source_ref_labels":["node.md#doc"],"air":{"id":"ok","label":"ok","layer":"ok","status":"ok","summary":"ok","context":"ok","docs":"ok","hardening_notes":"ok","aliases":"ok","tags":"ok","source_refs":"ok","source_ref_labels":"ok"}}],
			"edges": [{"id":"e1","source":"n1","target":"n2","relation":"feeds","status":"observed","layer":"runtime","res":"4","confidence":"0.95","summary":"Edge summary","docs":"Edge Doc","source_refs":"docs:1 refs","source_ref_labels":["edge.md#doc"],"air":{"id":"ok","source":"ok","target":"ok","relation":"ok","status":"ok","layer":"ok","res":"ok","confidence":"ok","summary":"ok","docs":"ok","source_refs":"ok","source_ref_labels":"ok"}}],
			"package": {
				"authority_case":"CASE-DYN",
				"totals":{"sources":1},
				"workbench_contract": {
					"status":"observed",
					"defaults":{"inquiry_mode":"release-gates","audience_mode":"operator","explanation_path":"release-readiness"},
					"inquiry_modes":[{"id":"release-gates","label":"What gates release?","lens":"operating-slice","prompt":"Follow gates","answer_shape":["ordered gate path"],"focus_node_ids":["n1"],"focus_edge_ids":["e1"],"air":{"id":"ok","label":"ok","lens":"ok","prompt":"ok","answer_shape":"ok","focus_node_ids":"ok","focus_edge_ids":"ok"}}],
					"audience_modes":[{"id":"operator","label":"Operator","emphasis":"diagnostic next action","air":{"id":"ok","label":"ok","emphasis":"ok"}}],
					"explanation_paths":[{"id":"release-readiness","label":"Release readiness path","summary":"Teach release readiness","must_include":["what this does not prove"],"scene_count":1,"scenes":[{"title":"State what this does not prove","lens":"evidence-risk","selection_group":"nodes","selection_id":"view-manifest","caveat":"Not live truth","air":{"title":"ok","lens":"ok","selection_group":"ok","selection_id":"ok","caveat":"ok"}}],"air":{"id":"ok","label":"ok","summary":"ok","must_include":"ok","scene_count":"ok","scenes":"ok"}}],
					"follow_on_tranches":["bitemporal snapshot registry"]
				}
			}
		}`))
	})

	g, dark, err := FetchDynamics(apiURL)
	if err != nil {
		t.Fatal(err)
	}
	if dark {
		t.Fatal("dynamics should not be dark")
	}
	if g.MapID != "system-dynamics-map-v1" || g.Thesis != "source-neutral semantic graph backbone" || len(g.Nodes) != 1 || len(g.Edges) != 1 {
		t.Fatalf("bad dynamics graph: %+v", g)
	}
	if g.Nodes[0].Summary != "Node summary" || g.Nodes[0].Docs != "Doc Label" || g.Nodes[0].HardeningNotes != "Validate it" {
		t.Fatalf("dynamics node explanation metadata should decode: %+v", g.Nodes[0])
	}
	if g.Nodes[0].SourceRefs != "docs:1 refs" || len(g.Nodes[0].SourceRefLabels) != 1 || g.Nodes[0].SourceRefLabels[0] != "node.md#doc" {
		t.Fatalf("dynamics node source refs should decode: %+v", g.Nodes[0])
	}
	if g.Edges[0].ID != "e1" || g.Edges[0].Confidence != "0.95" || g.Edges[0].Summary != "Edge summary" || g.Edges[0].Docs != "Edge Doc" {
		t.Fatalf("dynamics edge explanation metadata should decode: %+v", g.Edges[0])
	}
	if g.Edges[0].SourceRefs != "docs:1 refs" || len(g.Edges[0].SourceRefLabels) != 1 || g.Edges[0].SourceRefLabels[0] != "edge.md#doc" {
		t.Fatalf("dynamics edge source refs should decode: %+v", g.Edges[0])
	}
	if g.Package.Workbench.Defaults.InquiryMode != "release-gates" || len(g.Package.Workbench.InquiryModes) != 1 || g.Package.Workbench.InquiryModes[0].Label != "What gates release?" {
		t.Fatalf("dynamics workbench contract should decode: %+v", g.Package.Workbench)
	}
	if len(g.Package.Workbench.ExplanationPaths) != 1 || g.Package.Workbench.ExplanationPaths[0].Scenes[0].Title != "State what this does not prove" {
		t.Fatalf("dynamics explanation scenes should decode: %+v", g.Package.Workbench.ExplanationPaths)
	}
}

func TestFetchEpistemicsReadsSourceBackedRows(t *testing.T) {
	apiURL := withReadAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/read/epistemics" || r.URL.Query().Get("scope") != "dynamics" {
			t.Fatalf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"dark": false,
			"error": "",
			"epistemics": {
				"schema_version": "epistemics.read.v1",
				"scope": "dynamics",
				"authority_case": "CASE-DYN",
				"generated_at": "2026-06-25T12:00:00Z",
				"package_hash": "sha256:test",
				"sources": [{"id":"seed","status":"observed","count":2,"privacy":"metadata-only","raw_access":false,"air":{"id":"ok","status":"ok","count":"ok","privacy":"ok","raw_access":"ok"}}],
				"rows": [{
					"row_id":"map-edge:e1",
					"family":"dynamics",
					"subject_kind":"map-edge",
					"subject_ref":"e1",
					"subject":"e1",
					"status":"observed",
					"posture":"source-backed",
					"authority_case":"CASE-DYN",
					"evidence_count":1,
					"evidence":"source_refs:1",
					"source":"seed",
					"source_refs":"seed:1 refs",
					"source_ref_labels":["edge.md#doc"],
					"freshness":"2026-06-25T12:00:00Z",
					"privacy":"metadata-only",
					"raw_access":false,
					"map_kind":"edge",
					"map_id":"e1",
					"map_source":"n1",
					"map_target":"n2",
					"map_relation":"feeds",
					"air":{"row_id":"ok","family":"ok","subject_kind":"ok","subject_ref":"ok","status":"ok","posture":"ok","authority_case":"ok","evidence_count":"ok","evidence":"ok","source":"ok","source_refs":"ok","source_ref_labels":"ok","freshness":"ok","privacy":"ok","raw_access":"ok","map_kind":"ok","map_id":"ok","map_source":"ok","map_target":"ok","map_relation":"ok"}
				}],
				"totals": {"sources":1,"rows":1,"map_edges":1}
			}
		}`))
	})

	ep, dark, err := FetchEpistemics(apiURL)
	if err != nil {
		t.Fatal(err)
	}
	if dark {
		t.Fatal("epistemics should not be dark")
	}
	if ep.SchemaVersion != "epistemics.read.v1" || ep.Scope != "dynamics" || len(ep.Sources) != 1 || len(ep.Rows) != 1 {
		t.Fatalf("bad epistemics summary: %+v", ep)
	}
	row := ep.Rows[0]
	if row.RowID != "map-edge:e1" || row.MapSource != "n1" || row.MapTarget != "n2" || row.MapRelation != "feeds" || len(row.SourceRefLabels) != 1 {
		t.Fatalf("epistemics row should decode map identity and refs: %+v", row)
	}
}

func TestFetchTracesReadsRowsAndDarkFlag(t *testing.T) {
	apiURL := withReadAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/read/traces" {
			t.Fatalf("FetchTraces should GET /read/traces, got %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"dark": false, "error": "",
			"traces": [{
				"ts": "2026-06-26T12:00:00Z", "trace_id": "trace-1", "model": "claude-opus-4",
				"prompt_tok": 100, "completion_tok": 50, "total_tok": 150,
				"cost": 0.012345, "latency_ms": 2500,
				"air": {"trace_id": "ok", "model": "ok"}
			}]
		}`))
	})
	tr, dark, err := FetchTraces(apiURL)
	if err != nil {
		t.Fatal(err)
	}
	if dark {
		t.Fatal("traces should not be dark")
	}
	if len(tr) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(tr))
	}
	row := tr[0]
	if row.TraceID != "trace-1" || row.Model != "claude-opus-4" {
		t.Fatalf("trace identity fields did not decode: %+v", row)
	}
	if row.PromptTok != 100 || row.CompletionTok != 50 || row.TotalTok != 150 {
		t.Fatalf("token counts did not decode: %+v", row)
	}
	if row.Cost != 0.012345 || row.LatencyMs != 2500 {
		t.Fatalf("cost/latency did not decode: %+v", row)
	}
}

func TestFetchTracesUnreachableFoldsDark(t *testing.T) {
	tr, dark, err := FetchTraces("http://127.0.0.1:0")
	if len(tr) != 0 || !dark || err == nil {
		t.Fatalf("unreachable api must fold honest-dark (nil traces, dark=true, err): len=%d dark=%v err=%v", len(tr), dark, err)
	}
}

func TestFetchTurnsReadsTypedReceipts(t *testing.T) {
	apiURL := withReadAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/read/session/cc-reins/turns" {
			t.Fatalf("FetchTurns should GET session turns, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "80" || r.URL.Query().Get("before") != "2026-06-26T18:40:07Z" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"dark": false,
			"error": "",
			"oldest_ts": "2026-06-26T18:40:05Z",
			"turns": [{
				"ts": "2026-06-26T18:40:05Z",
				"role": "cc-reins",
				"kind": "assistant",
				"prov": "model",
				"summary": "fixture assistant response body",
				"magnitude": 0.3,
				"model": "fugu",
				"route": "codex.exec",
				"gate": "pass",
				"air": {"ts":"ok","role":"ok","kind":"ok","summary":"deny","magnitude":"ok","model":"ok","route":"ok","gate":"ok"}
			}]
		}`))
	})
	t.Setenv("REINS_API_URL", apiURL)

	turns, err := FetchTurns("cc-reins", "2026-06-26T18:40:07Z")
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	turn := turns[0]
	if turn.TS != "2026-06-26T18:40:05Z" || turn.Role != "cc-reins" || turn.Kind != "assistant" || turn.Prov != "model" {
		t.Fatalf("turn identity fields did not decode: %+v", turn)
	}
	if turn.Summary != "fixture assistant response body" || turn.Magnitude != 0.3 || turn.Model != "fugu" || turn.Route != "codex.exec" || turn.Gate != "pass" {
		t.Fatalf("turn payload fields did not decode: %+v", turn)
	}
	if turn.AIR["summary"] != "deny" || turn.AIR["route"] != "ok" {
		t.Fatalf("turn AIR map did not decode: %+v", turn.AIR)
	}
}

func TestFetchTurnsReturnsErrorOnDark(t *testing.T) {
	apiURL := withReadAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/read/session/missing/turns" {
			t.Fatalf("FetchTurns should GET session turns, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dark": true, "error": "no turn replay fixture for session role: missing", "turns": []}`))
	})
	t.Setenv("REINS_API_URL", apiURL)

	turns, err := FetchTurns("missing", "")
	if err == nil {
		t.Fatal("FetchTurns should surface a dark turns endpoint as an error")
	}
	if len(turns) != 0 {
		t.Fatalf("dark turns response should not fabricate rows: %+v", turns)
	}
	if !strings.Contains(err.Error(), "no turn replay fixture") {
		t.Fatalf("FetchTurns error should include dark reason, got %q", err.Error())
	}
}
