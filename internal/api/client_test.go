package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
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

func TestConsumeReadErrorUsesClosedVocabulary(t *testing.T) {
	tests := []struct {
		name string
		wire string
		want string
	}{
		{
			name: "approved",
			wire: "turn_replay_fixture_only",
			want: "turn_replay_fixture_only",
		},
		{name: "version skew", wire: "events_read_error_v2", want: "read_contract_error"},
		{
			name: "sentinel",
			wire: "SENTINEL:/private/operator/path:stack-frame",
			want: "read_contract_error",
		},
		{
			name: "oversized",
			wire: strings.Repeat("x", maxReadErrorCodeBytes+1),
			want: "read_contract_error",
		},
		{name: "malformed", wire: string([]byte{0xff}), want: "read_contract_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire := tt.wire
			err := consumeReadError(&wire)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("consumeReadError() = %v, want %q", err, tt.want)
			}
			if wire != "" {
				t.Fatalf("wire error bytes retained: %q", wire)
			}
			if strings.Contains(err.Error(), "SENTINEL") {
				t.Fatalf("returned error retained remote detail: %q", err)
			}
		})
	}
}

func TestFetchEventsRejectsUnknownRemoteErrorDetail(t *testing.T) {
	const sentinel = "SENTINEL:/private/operator/path:stack-frame"
	apiURL := withReadAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"dark":true,"error":%q,"events":[]}`, sentinel)
	})

	_, dark, err := FetchEvents(apiURL)
	if !dark || err == nil || err.Error() != "read_contract_error" {
		t.Fatalf("FetchEvents() dark=%v err=%v", dark, err)
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("FetchEvents retained remote detail: %q", err)
	}
}

func TestFetchEventsCollapsesHostileWrapperDecode(t *testing.T) {
	const sentinel = "SENTINEL:/private/operator/path:stack-frame"
	invalidUTF8 := append(
		[]byte(`{"dark":true,"error":"`),
		append([]byte{0xff}, []byte(`","events":[]}`)...)...,
	)
	tests := []struct {
		name string
		wire []byte
	}{
		{
			name: "wrong type error",
			wire: []byte(`{"dark":true,"error":{"detail":"` + sentinel + `"},"events":[]}`),
		},
		{
			name: "oversized error",
			wire: []byte(fmt.Sprintf(
				`{"dark":true,"error":%q,"events":[]}`,
				strings.Repeat("x", maxReadErrorCodeBytes+1),
			)),
		},
		{
			name: "malformed error",
			wire: []byte(`{"dark":true,"error":"` + sentinel + `\q","events":[]}`),
		},
		{name: "invalid utf8 error", wire: invalidUTF8},
		{
			name: "trailing value",
			wire: []byte(`{"dark":true,"error":"","events":[]} "` + sentinel + `"`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiURL := withReadAPI(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(tt.wire)
			})

			events, dark, err := FetchEvents(apiURL)
			if !dark || err != errReadContract {
				t.Fatalf("FetchEvents() events=%+v dark=%v err=%v", events, dark, err)
			}
			if events != nil {
				t.Fatalf("contract failure released partially decoded events: %+v", events)
			}
			if err.Error() != "read_contract_error" || strings.Contains(err.Error(), sentinel) {
				t.Fatalf("contract failure exposed producer detail: %q", err)
			}
		})
	}
}

func TestFetchMetaSanitizesRemoteStatusDetail(t *testing.T) {
	apiURL := withReadAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	meta := FetchMeta(apiURL)
	if !meta.Reachable || !meta.Foreign || meta.Detail != "foreign_http_status" {
		t.Fatalf("FetchMeta() = %+v", meta)
	}
}

func TestContextProjectionHashStreamingPreservesFrozenDigests(t *testing.T) {
	payload, err := os.ReadFile("../../api/fixtures/context-canon-gate0-carriers.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		OperatorProjection json.RawMessage `json:"operator_projection"`
	}
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	var projection struct {
		Events []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(fixture.OperatorProjection, &projection); err != nil {
		t.Fatal(err)
	}
	if len(projection.Events) == 0 {
		t.Fatal("frozen operator projection has no events")
	}

	projectionHash, err := grammar.ContextProjectionContentHash(fixture.OperatorProjection)
	if err != nil {
		t.Fatal(err)
	}
	const wantProjection = "1a7f44abeb2b0bf04d55cb845ffa60d6e67d1c8694cdd77d54b70e5a4efc9079"
	if projectionHash != wantProjection {
		t.Fatalf(
			"ContextProjectionContentHash() = %s, want %s",
			projectionHash,
			wantProjection,
		)
	}

	eventHash, err := grammar.ContextProjectionEventContentHash(projection.Events[0])
	if err != nil {
		t.Fatal(err)
	}
	const wantEvent = "dc34763eca21b0d96ed9ea8853b9dd27ce43ba6949d42742ed0d327f850bc441"
	if eventHash != wantEvent {
		t.Fatalf(
			"ContextProjectionEventContentHash() = %s, want %s",
			eventHash,
			wantEvent,
		)
	}
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

func TestPostCommandParsesVerdictAndWiredSet(t *testing.T) {
	// the apply seam: PostCommand posts to /command/{verb} and returns the router verdict + witnessed
	// event_id; FetchMeta exposes the wired-set. A refusal is disclosed, never a fabricated success.
	apiURL := withReadAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/read/meta":
			_, _ = w.Write([]byte(`{"app":"reins","serving_sha":"s","api_tree_sha":"t","router":"mounted","verbs":{"resume":{"wired":true},"dispatch":{"wired":false}}}`))
		case "/command/resume":
			_, _ = w.Write([]byte(`{"status":"ok","http":200,"event_id":"ev-witnessed-123","reason":""}`))
		case "/command/dispatch":
			w.WriteHeader(501)
			_, _ = w.Write([]byte(`{"status":"not-wired","http":501,"event_id":"ev-ref-456","reason":"no ungated path"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})

	meta := FetchMeta(apiURL)
	wired := meta.WiredVerbs()
	if !wired["resume"] || wired["dispatch"] {
		t.Fatalf("wired-set wrong: %v", wired)
	}

	ok := PostCommand(apiURL, "resume", "lane-a", map[string]any{"kind": "operator_attestation"}, map[string]any{}, "resume:lane-a:1")
	if !ok.Reachable || ok.Status != "ok" || ok.EventID != "ev-witnessed-123" {
		t.Fatalf("resume verdict wrong: %+v", ok)
	}
	ref := PostCommand(apiURL, "dispatch", "lane-b", map[string]any{}, map[string]any{}, "dispatch:lane-b:1")
	if ref.Status != "not-wired" || ref.HTTP != 501 || ref.EventID != "ev-ref-456" {
		t.Fatalf("dispatch refusal must be disclosed, got %+v", ref)
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

func TestFetchRouteCollapsesPartialWrapperDecode(t *testing.T) {
	const sentinel = "SENTINEL:/private/operator/path"
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "posture",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/route/posture" {
					t.Fatalf("posture contract failure must stop before candidates, got %s", r.URL.Path)
				}
				_, _ = w.Write([]byte(
					`{"dark":true,"error":"` + sentinel + `","decision":123}`,
				))
			},
		},
		{
			name: "candidates",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/route/posture":
					_, _ = w.Write([]byte(
						`{"dark":false,"error":"","decision":"NO SPINE DECISION ON FILE"}`,
					))
				case "/route/candidates":
					_, _ = w.Write([]byte(
						`{"dark":true,"error":"` + sentinel + `","decision":123,"candidates":[]}`,
					))
				default:
					t.Fatalf("unexpected route path %s", r.URL.Path)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiURL := withReadAPI(t, tt.handler)
			posture, candidates, dark, err := FetchRoute(apiURL)
			if !dark || err != errReadContract {
				t.Fatalf(
					"FetchRoute() posture=%+v candidates=%+v dark=%v err=%v",
					posture,
					candidates,
					dark,
					err,
				)
			}
			if !reflect.DeepEqual(posture, grammar.RoutePosture{}) || candidates != nil {
				t.Fatalf(
					"contract failure released route state: posture=%+v candidates=%+v",
					posture,
					candidates,
				)
			}
			if err.Error() != "read_contract_error" || strings.Contains(err.Error(), sentinel) {
				t.Fatalf("route contract failure exposed producer detail: %q", err)
			}
		})
	}
}

func TestDecodeReadWrapperPreservesApprovedStructuredData(t *testing.T) {
	wire := strings.NewReader(`{
		"dark": true,
		"error": "turn_replay_fixture_only",
		"turns": [{
			"ts": "2026-07-13T10:00:00Z",
			"role": "cc-reins",
			"kind": "assistant",
			"prov": "model",
			"summary": "approved structured turn"
		}]
	}`)

	decoded, err := decodeReadWrapper(
		wire,
		func(r *turnsResp) *string { return &r.Error },
	)
	if err == nil || err.Error() != "turn_replay_fixture_only" {
		t.Fatalf("decodeReadWrapper() err=%v", err)
	}
	if decoded.Error != "" {
		t.Fatalf("approved wire error was not cleared: %q", decoded.Error)
	}
	if len(decoded.Turns) != 1 || decoded.Turns[0].Summary != "approved structured turn" {
		t.Fatalf("approved structured data was not preserved: %+v", decoded.Turns)
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

func contextEnvelope(state, reasons, projection, compatibility string) []byte {
	return []byte(fmt.Sprintf(
		`{"schema":"hapax.reins-context-read.v1","state":%q,"audience":"operator_private","reason_codes":%s,"projection":%s,"compatibility":%s}`,
		state,
		reasons,
		projection,
		compatibility,
	))
}

func TestFetchContextDarkEnvelope(t *testing.T) {
	payload := contextEnvelope("dark", `["producer_absent"]`, "null", "null")
	apiURL := withReadAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/read/context" {
			t.Fatalf("FetchContext should GET /read/context, got %s", r.URL.Path)
		}
		_, _ = w.Write(payload)
	})

	readout, err := FetchContext(apiURL)
	if err != nil {
		t.Fatal(err)
	}
	if readout.State != grammar.ContextReadDark ||
		len(readout.ReasonCodes) != 1 ||
		readout.ReasonCodes[0] != "producer_absent" ||
		readout.Projection != nil ||
		readout.Compatibility != nil {
		t.Fatalf("unexpected DARK readout: %+v", readout)
	}
}

func TestDecodeContextHoldRetainsRawProjectionAndEnvelope(t *testing.T) {
	nested := `{"z": 1.2300, "a":[true, null], "sentinel":"PRIVATE-NESTED"}`
	payload := contextEnvelope(
		"hold",
		`["canonical_verifier_unavailable","producer_receipt_missing"]`,
		nested,
		"null",
	)
	original := append([]byte(nil), payload...)

	readout, err := decodeContextReadout(payload)
	if err != nil {
		t.Fatal(err)
	}
	if readout.State != grammar.ContextReadHold || readout.Projection == nil {
		t.Fatalf("expected projection HOLD, got %+v", readout)
	}
	if got := string(*readout.Projection); got != nested {
		t.Fatalf("nested bytes changed:\nwant %q\n got %q", nested, got)
	}
	if !bytes.Equal(readout.RawEnvelope, original) {
		t.Fatal("full envelope bytes changed")
	}

	payload[0] = '['
	if !bytes.Equal(readout.RawEnvelope, original) || string(*readout.Projection) != nested {
		t.Fatal("retained bytes alias the caller buffer")
	}
}

func TestFetchContextHoldRetainsRawCompatibility(t *testing.T) {
	nested := `{"compatibility_only":true,"sentinel":"COMPAT-PRIVATE"}`
	payload := contextEnvelope("hold", `["compatibility_only"]`, "null", nested)
	apiURL := withReadAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	})

	readout, err := FetchContext(apiURL)
	if err != nil {
		t.Fatal(err)
	}
	if readout.Compatibility == nil || readout.Projection != nil {
		t.Fatalf("expected compatibility-only HOLD, got %+v", readout)
	}
	if got := string(*readout.Compatibility); got != nested {
		t.Fatalf("compatibility bytes changed: %q", got)
	}
}

func TestDecodeContextPresentReturnsOnlySafeDark(t *testing.T) {
	nested := `{"projection_id":"p1","sentinel":"UNVERIFIED-PRIVATE"}`
	payload := contextEnvelope("present", `[]`, nested, "null")

	readout, err := decodeContextReadout(payload)
	if !errors.Is(err, ErrContextPresentValidationUnavailable) {
		t.Fatalf("want canonical verifier sentinel, got %v", err)
	}
	if readout.State != grammar.ContextReadDark ||
		len(readout.ReasonCodes) != 1 ||
		readout.ReasonCodes[0] != grammar.ContextReadReasonCanonUnverified ||
		readout.Projection != nil ||
		readout.Compatibility != nil ||
		len(readout.RawEnvelope) != 0 {
		t.Fatalf("unverified PRESENT escaped as a usable carrier: %+v", readout)
	}
}

func TestDecodeContextRejectsMissingAndDuplicateOuterFields(t *testing.T) {
	base := map[string]any{
		"schema":        grammar.ContextReadSchema,
		"state":         "dark",
		"audience":      grammar.ContextReadAudience,
		"reason_codes":  []string{"producer_absent"},
		"projection":    nil,
		"compatibility": nil,
	}
	valid, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}

	for key := range base {
		t.Run("missing_"+key, func(t *testing.T) {
			missing := make(map[string]any, len(base)-1)
			for candidate, value := range base {
				if candidate != key {
					missing[candidate] = value
				}
			}
			payload, marshalErr := json.Marshal(missing)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if _, decodeErr := decodeContextReadout(payload); decodeErr == nil {
				t.Fatalf("missing %q was accepted", key)
			}
		})

		t.Run("duplicate_"+key, func(t *testing.T) {
			encoded, marshalErr := json.Marshal(base[key])
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			payload := []byte(fmt.Sprintf("{%q:%s,%s", key, encoded, valid[1:]))
			if _, decodeErr := decodeContextReadout(payload); decodeErr == nil {
				t.Fatalf("duplicate %q was accepted", key)
			}
		})
	}
}

func TestDecodeContextRejectsMalformedOuterEnvelope(t *testing.T) {
	valid := contextEnvelope("dark", `["producer_absent"]`, "null", "null")
	invalidUTF8 := append(append([]byte(nil), valid...), 0xff)
	cases := map[string][]byte{
		"non_object":   []byte(`[]`),
		"unknown":      []byte(strings.TrimSuffix(string(valid), "}") + `,"extra":true}`),
		"trailing":     append(append([]byte(nil), valid...), []byte(` { }`)...),
		"invalid_utf8": invalidUTF8,
		"legacy":       []byte(`{"dark":true,"projections":{}}`),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeContextReadout(payload); err == nil {
				t.Fatalf("invalid envelope %q was accepted", payload)
			}
		})
	}
}

func TestDecodeContextRejectsInvalidValuesAndStateMatrices(t *testing.T) {
	cases := map[string][]byte{
		"wrong_schema": contextEnvelope("dark", `["x"]`, "null", "null"),
		"wrong_audience": []byte(
			`{"schema":"hapax.reins-context-read.v1","state":"dark","audience":"all","reason_codes":["x"],"projection":null,"compatibility":null}`,
		),
		"unknown_state":              contextEnvelope("pending", `["x"]`, "null", "null"),
		"reasons_null":               contextEnvelope("dark", `null`, "null", "null"),
		"reasons_scalar":             contextEnvelope("dark", `"x"`, "null", "null"),
		"reasons_blank":              contextEnvelope("dark", `[""]`, "null", "null"),
		"reasons_duplicate":          contextEnvelope("dark", `["x","x"]`, "null", "null"),
		"reasons_unsorted":           contextEnvelope("dark", `["z","a"]`, "null", "null"),
		"reasons_edge_whitespace":    contextEnvelope("dark", `[" x"]`, "null", "null"),
		"reasons_uppercase":          contextEnvelope("dark", `["NotCanonical"]`, "null", "null"),
		"reasons_newline":            contextEnvelope("dark", `["line\nbreak"]`, "null", "null"),
		"reasons_ansi":               contextEnvelope("dark", `["\u001b[31m"]`, "null", "null"),
		"reasons_control":            contextEnvelope("dark", `["x\u0000y"]`, "null", "null"),
		"reasons_lone_surrogate":     contextEnvelope("dark", `["\ud800"]`, "null", "null"),
		"projection_array":           contextEnvelope("hold", `["x"]`, `[]`, "null"),
		"compatibility_scalar":       contextEnvelope("hold", `["x"]`, "null", `"bad"`),
		"dark_with_projection":       contextEnvelope("dark", `["x"]`, `{}`, "null"),
		"dark_without_reasons":       contextEnvelope("dark", `[]`, "null", "null"),
		"hold_without_payload":       contextEnvelope("hold", `["x"]`, "null", "null"),
		"hold_with_both":             contextEnvelope("hold", `["x"]`, `{}`, `{}`),
		"hold_without_reasons":       contextEnvelope("hold", `[]`, `{}`, "null"),
		"present_without_projection": contextEnvelope("present", `[]`, "null", "null"),
		"present_with_compatibility": contextEnvelope("present", `[]`, `{}`, `{}`),
		"present_with_reasons":       contextEnvelope("present", `["x"]`, `{}`, "null"),
	}
	tooLongJSON, err := json.Marshal(
		[]string{strings.Repeat("a", grammar.ContextReadMaxReasonCodeBytes+1)},
	)
	if err != nil {
		t.Fatal(err)
	}
	cases["reason_too_long"] = contextEnvelope(
		"dark",
		string(tooLongJSON),
		"null",
		"null",
	)
	tooMany := make([]string, grammar.ContextReadMaxReasonCodes+1)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("reason_%03d", index)
	}
	tooManyJSON, err := json.Marshal(tooMany)
	if err != nil {
		t.Fatal(err)
	}
	cases["too_many_reasons"] = contextEnvelope(
		"dark",
		string(tooManyJSON),
		"null",
		"null",
	)
	// Override the schema field in the one schema-specific case.
	cases["wrong_schema"] = []byte(strings.Replace(
		string(cases["wrong_schema"]),
		grammar.ContextReadSchema,
		"hapax.reins-context-read.v0",
		1,
	))

	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeContextReadout(payload); err == nil {
				t.Fatalf("invalid context matrix was accepted: %s", payload)
			}
		})
	}
}

func TestFetchContextRejectsOversizeBody(t *testing.T) {
	apiURL := withReadAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), maxContextReadBytes+1))
	})
	if _, err := FetchContext(apiURL); err == nil {
		t.Fatal("oversize /read/context body was accepted")
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
		_, _ = w.Write([]byte(`{"dark": true, "error": "session_turn_fixture_absent", "turns": []}`))
	})
	t.Setenv("REINS_API_URL", apiURL)

	turns, err := FetchTurns("missing", "")
	if err == nil {
		t.Fatal("FetchTurns should surface a dark turns endpoint as an error")
	}
	if len(turns) != 0 {
		t.Fatalf("dark turns response should not fabricate rows: %+v", turns)
	}
	if err.Error() != "session_turn_fixture_absent" {
		t.Fatalf("FetchTurns error should preserve the approved code, got %q", err.Error())
	}
}
