package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hapax-systems/reins/internal/grammar"
)

type readResp struct {
	Dark   bool            `json:"dark"`
	Error  string          `json:"error"`
	Events []grammar.Event `json:"events"`
}

var newReadHTTPClient = func() *http.Client {
	return &http.Client{Timeout: 3 * time.Second}
}

func checkOK(resp *http.Response, endpoint string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("reins: READ api %s returned %s", endpoint, resp.Status)
}

// ServingMeta is the /read/meta identity handshake (U1). A port is only trusted as reins
// when it answers app=="reins"; anything else on the configured port is a FOREIGN SERVER
// (the 8799-squatter class) and must render as such, never be silently trusted.
type ServingMeta struct {
	App        string `json:"app"`
	Version    string `json:"version"` // the ONE semver — the cockpit compares its compiled version → GEN-SKEW
	ServingSHA string `json:"serving_sha"`
	APITreeSHA string `json:"api_tree_sha"`
	Router     string `json:"router"`
	// Verbs is the router's per-verb wired-state ({verb: {wired: bool}}) — the cockpit's apply seam
	// only offers SEND for a wired verb; an unwired verb renders preview-only (A3.12a: one surface).
	Verbs map[string]struct {
		Wired bool   `json:"wired"`
		Mode  string `json:"mode"` // "" | "inflection" | "preview" | "governed" — preview must NOT read as applied
	} `json:"verbs"`
	// Foreign is set by FetchMeta (not the wire) when the port answers without the reins
	// identity — the cockpit renders PORT: FOREIGN SERVER.
	Foreign   bool   `json:"-"`
	Reachable bool   `json:"-"`
	Detail    string `json:"-"`
}

// WiredVerbs flattens Verbs to a {verb: wired} map for the model.
func (m ServingMeta) WiredVerbs() map[string]bool {
	out := map[string]bool{}
	for v, s := range m.Verbs {
		out[v] = s.Wired
	}
	return out
}

// VerbModes flattens Verbs to a {verb: mode} map so the cockpit renders a PREVIEW verb honestly (a
// preview-mode ok is a no-op "would emit …", never "✓ applied + witnessed").
func (m ServingMeta) VerbModes() map[string]string {
	out := map[string]string{}
	for v, s := range m.Verbs {
		out[v] = s.Mode
	}
	return out
}

// CommandResult is the router's verdict for a POST /command/{verb} (the cockpit apply seam).
type CommandResult struct {
	Status    string `json:"status"`
	HTTP      int    `json:"http"`
	EventID   string `json:"event_id"`  // the witnessed ledger event id (demand+verdict)
	Reason    string `json:"reason"`
	FoldDelta string `json:"fold_delta"` // the transport's own honest phrasing (preview: "would emit …")
	Duplicate bool   `json:"duplicate"`
	Reachable bool   `json:"-"`
	Err       string `json:"-"`
}

// PostCommand invokes a governed verb through the witnessed rail: POST /command/{verb} with a
// verify-already-minted authority packet (reins mints nothing — the operator's loopback presence IS the
// attestation, A3.3 RULING-REINS-OPERATOR-ATTESTATION). Returns the router verdict + the witnessed
// event_id. A transport failure is disclosed (Reachable=false), never a fabricated success.
func PostCommand(apiURL, verb, target string, authorityPacket map[string]any, preflightReceipt map[string]any, idempotencyKey string) CommandResult {
	body, _ := json.Marshal(map[string]any{
		"target": target, "authority_packet": authorityPacket,
		"preflight_receipt": preflightReceipt, "idempotency_key": idempotencyKey,
	})
	c := newReadHTTPClient()
	resp, err := c.Post(apiURL+"/command/"+verb, "application/json", bytes.NewReader(body))
	if err != nil {
		return CommandResult{Reachable: false, Err: err.Error(), Status: "unreachable"}
	}
	defer resp.Body.Close()
	var r CommandResult
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return CommandResult{Reachable: true, HTTP: resp.StatusCode, Status: "undecodable", Err: err.Error()}
	}
	r.Reachable = true
	if r.HTTP == 0 {
		r.HTTP = resp.StatusCode
	}
	return r
}

// FetchMeta GETs the serving-identity handshake. A reachable port that is NOT reins comes
// back Foreign=true; an unreachable port comes back Reachable=false (honest dark, not foreign).
func FetchMeta(apiURL string) ServingMeta {
	c := newReadHTTPClient()
	resp, err := c.Get(apiURL + "/read/meta")
	if err != nil {
		return ServingMeta{Reachable: false, Detail: "unreachable"}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// a port that answers HTTP but has no /read/meta is a foreign server.
		return ServingMeta{Reachable: true, Foreign: true, Detail: resp.Status}
	}
	var m ServingMeta
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil || m.App != "reins" {
		return ServingMeta{Reachable: true, Foreign: true, Detail: "not a reins server"}
	}
	m.Reachable = true
	return m
}

// FetchEvents GETs the READ endpoint. Returns (events, dark, err).
func FetchEvents(url string) ([]grammar.Event, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(url + "/read/events?limit=80")
	if err != nil {
		return nil, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/events"); err != nil {
		return nil, true, err
	}
	var r readResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, true, err
	}
	if r.Error != "" {
		return r.Events, true, fmt.Errorf("%s", r.Error)
	}
	return r.Events, r.Dark, nil
}

// FetchEventsBefore GETs one backward page — events strictly older than `before` (ISO ts),
// the /lastlog backward-paging cursor. Returns (events, dark, err).
func FetchEventsBefore(urlStr, before string) ([]grammar.Event, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(urlStr + "/read/events?limit=80&before=" + url.QueryEscape(before))
	if err != nil {
		return nil, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/events"); err != nil {
		return nil, true, err
	}
	var r readResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, true, err
	}
	if r.Error != "" {
		return r.Events, true, fmt.Errorf("%s", r.Error)
	}
	return r.Events, r.Dark, nil
}

type tasksResp struct {
	Dark  bool           `json:"dark"`
	Error string         `json:"error"`
	Tasks []grammar.Task `json:"tasks"`
}

// FetchTasks GETs the registry projection. Returns (tasks, dark, err).
func FetchTasks(url string) ([]grammar.Task, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(url + "/read/tasks")
	if err != nil {
		return nil, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/tasks"); err != nil {
		return nil, true, err
	}
	var r tasksResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, true, err
	}
	if r.Error != "" {
		return r.Tasks, true, fmt.Errorf("%s", r.Error)
	}
	return r.Tasks, r.Dark, nil
}

type dynamicsResp struct {
	Dark  bool   `json:"dark"`
	Error string `json:"error"`
	grammar.Graph
}

// FetchDynamics GETs the system-dynamics map. Returns (graph, dark, err).
// FetchFacets fetches the facet-registry SSOT (/read/facets returns the payload directly, unwrapped).
func FetchFacets(url string) (grammar.FacetRegistry, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(url + "/read/facets")
	if err != nil {
		return grammar.FacetRegistry{}, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/facets"); err != nil {
		return grammar.FacetRegistry{}, true, err
	}
	var r grammar.FacetRegistry
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return grammar.FacetRegistry{}, true, err
	}
	return r, false, nil
}

func FetchDynamics(url string) (grammar.Graph, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(url + "/read/dynamics")
	if err != nil {
		return grammar.Graph{}, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/dynamics"); err != nil {
		return grammar.Graph{}, true, err
	}
	var r dynamicsResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return grammar.Graph{}, true, err
	}
	if r.Error != "" {
		return r.Graph, true, fmt.Errorf("%s", r.Error)
	}
	return r.Graph, r.Dark, nil
}

type epistemicsResp struct {
	Dark       bool                      `json:"dark"`
	Error      string                    `json:"error"`
	Epistemics grammar.EpistemicsSummary `json:"epistemics"`
}

// FetchEpistemics GETs the typed evidence/provenance read model. Returns (summary, dark, err).
func FetchEpistemics(apiURL string) (grammar.EpistemicsSummary, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(apiURL + "/read/epistemics?scope=dynamics")
	if err != nil {
		return grammar.EpistemicsSummary{}, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/epistemics"); err != nil {
		return grammar.EpistemicsSummary{}, true, err
	}
	var r epistemicsResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return grammar.EpistemicsSummary{}, true, err
	}
	if r.Error != "" {
		return r.Epistemics, true, fmt.Errorf("%s", r.Error)
	}
	return r.Epistemics, r.Dark, nil
}

type sessionsResp struct {
	Dark     bool              `json:"dark"`
	Error    string            `json:"error"`
	Sessions []grammar.Session `json:"sessions"`
}

// FetchSessions GETs the live lane/session roster. Returns (sessions, dark, err).
func FetchSessions(url string) ([]grammar.Session, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(url + "/read/sessions")
	if err != nil {
		return nil, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/sessions"); err != nil {
		return nil, true, err
	}
	var r sessionsResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, true, err
	}
	if r.Error != "" {
		return r.Sessions, true, fmt.Errorf("%s", r.Error)
	}
	return r.Sessions, r.Dark, nil
}

type tracesResp struct {
	Dark   bool            `json:"dark"`
	Error  string          `json:"error"`
	Traces []grammar.Trace `json:"traces"`
}

// FetchTraces GETs the LLM-observability recent-traces fold. Returns (traces, dark, err).
func FetchTraces(url string) ([]grammar.Trace, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(url + "/read/traces")
	if err != nil {
		return nil, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/traces"); err != nil {
		return nil, true, err
	}
	var r tracesResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, true, err
	}
	if r.Error != "" {
		return r.Traces, true, fmt.Errorf("%s", r.Error)
	}
	return r.Traces, r.Dark, nil
}

type sessionDetailResp struct {
	Dark   bool                  `json:"dark"`
	Error  string                `json:"error"`
	Detail grammar.SessionDetail `json:"detail"`
}

// FetchSessionDetail GETs structured resume context for one live lane. Returns (detail, dark, err).
func FetchSessionDetail(apiURL, role string) (grammar.SessionDetail, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(apiURL + "/read/session/" + url.PathEscape(role))
	if err != nil {
		return grammar.SessionDetail{}, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/session"); err != nil {
		return grammar.SessionDetail{}, true, err
	}
	var r sessionDetailResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return grammar.SessionDetail{}, true, err
	}
	if r.Error != "" {
		return r.Detail, true, fmt.Errorf("%s", r.Error)
	}
	return r.Detail, r.Dark, nil
}

type turnsResp struct {
	Dark  bool           `json:"dark"`
	Error string         `json:"error"`
	Turns []grammar.Turn `json:"turns"`
}

type turnBlocksResp struct {
	Dark   bool                `json:"dark"`
	Error  string              `json:"error"`
	Blocks []grammar.TurnBlock `json:"blocks"`
}

// FetchTurnBlocks GETs the per-turn Block stream for ONE turn (role, ts) — the focused turn only, not the
// whole ladder. Honest-empty (blocks=[]) until CapabilityIO capture-output serves real Block streams.
func FetchTurnBlocks(role, ts string) ([]grammar.TurnBlock, bool, error) {
	apiURL := readBaseURL()
	endpoint := "/read/session/" + url.PathEscape(role) + "/turns/" + url.PathEscape(ts) + "/blocks"
	c := newReadHTTPClient()
	resp, err := c.Get(apiURL + endpoint)
	if err != nil {
		return nil, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, endpoint); err != nil {
		return nil, true, err
	}
	var r turnBlocksResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, true, err
	}
	if r.Error != "" {
		return r.Blocks, true, fmt.Errorf("%s", r.Error)
	}
	return r.Blocks, r.Dark, nil
}

func readBaseURL() string {
	if v := os.Getenv("REINS_API_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://127.0.0.1:8799"
}

// FetchTurns GETs one typed turn-receipt page for a session role. A dark response is surfaced as an
// error because this convenience API intentionally returns only turns + error.
func FetchTurns(role string, before string) ([]grammar.Turn, error) {
	apiURL := readBaseURL()
	endpoint := "/read/session/" + url.PathEscape(role) + "/turns"
	reqURL := apiURL + endpoint + "?limit=80"
	if before != "" {
		reqURL += "&before=" + url.QueryEscape(before)
	}
	c := newReadHTTPClient()
	resp, err := c.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, endpoint); err != nil {
		return nil, err
	}
	var r turnsResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	if r.Error != "" {
		return r.Turns, fmt.Errorf("%s", r.Error)
	}
	if r.Dark {
		return r.Turns, fmt.Errorf("reins: READ api %s returned dark", endpoint)
	}
	return r.Turns, nil
}

type intakeResp struct {
	Dark   bool                  `json:"dark"`
	Error  string                `json:"error"`
	Intake grammar.IntakeSummary `json:"intake"`
}

// FetchIntake GETs the bounded intake-observation projection. Returns (summary, dark, err).
func FetchIntake(apiURL string) (grammar.IntakeSummary, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(apiURL + "/read/intake")
	if err != nil {
		return grammar.IntakeSummary{}, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/intake"); err != nil {
		return grammar.IntakeSummary{}, true, err
	}
	var r intakeResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return grammar.IntakeSummary{}, true, err
	}
	if r.Error != "" {
		return r.Intake, true, fmt.Errorf("%s", r.Error)
	}
	return r.Intake, r.Dark, nil
}

type capabilitiesResp struct {
	Dark         bool                      `json:"dark"`
	Error        string                    `json:"error"`
	Capabilities grammar.CapabilitySummary `json:"capabilities"`
}

// FetchCapabilities GETs the source-backed capability-routing projection.
func FetchCapabilities(apiURL string) (grammar.CapabilitySummary, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(apiURL + "/read/capabilities")
	if err != nil {
		return grammar.CapabilitySummary{}, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/capabilities"); err != nil {
		return grammar.CapabilitySummary{}, true, err
	}
	var r capabilitiesResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return grammar.CapabilitySummary{}, true, err
	}
	if r.Error != "" {
		return r.Capabilities, true, fmt.Errorf("%s", r.Error)
	}
	return r.Capabilities, r.Dark, nil
}

type ctxProjection struct {
	Affordances []struct {
		Subject     string                     `json:"subject_ref"`
		Affordances []grammar.ContextAffordance `json:"affordances"`
	} `json:"affordances"`
	FactCount int `json:"fact_count"`
}

type contextResp struct {
	Dark        bool                     `json:"dark"`
	Reason      string                   `json:"reason"`
	Projections map[string]ctxProjection `json:"projections"`
}

// FetchContext reads the tri-audience /read/context substrate and returns the OPERATOR-COCKPIT projection's
// affordances (subject → offered affordance + state), flattened. Honest-dark until the spine producer emits
// the fact bundle (then it lights up with no code change). Readout only — never an injector.
func FetchContext(apiURL string) ([]grammar.ContextAffordance, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(apiURL + "/read/context")
	if err != nil {
		return nil, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/context"); err != nil {
		return nil, true, err
	}
	var r contextResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, true, err
	}
	if r.Dark {
		return nil, true, nil // producer not emitting — honest-dark, not an error
	}
	var out []grammar.ContextAffordance
	for _, subj := range r.Projections["operator_private"].Affordances {
		for _, a := range subj.Affordances {
			a.Subject = subj.Subject
			out = append(out, a)
		}
	}
	return out, false, nil
}

type vaultResp struct {
	VaultRoot string              `json:"vault_root"`
	Dark      bool                `json:"dark"`
	Notes     []grammar.VaultNote `json:"notes"`
	Error     string              `json:"error"`
}

type observeResp struct {
	Dark       bool                       `json:"dark"`
	Error      string                     `json:"error"`
	Dimensions []grammar.ObserveDimension `json:"dimensions"`
}

type commandRow struct {
	Verb        string            `json:"verb"`
	Target      string            `json:"target"`
	Status      string            `json:"status"`
	Witness     string            `json:"witness"`
	TaskID      string            `json:"task_id"`
	SessionRole string            `json:"session_role"`
	AIR         map[string]string `json:"air"`
}

type commandsResp struct {
	Dark        bool         `json:"dark"`
	Error       string       `json:"error"`
	Commands    []commandRow `json:"commands"`
	Enforcement string       `json:"enforcement"`
}

type routeCandidateRow struct {
	RoutingClass   string          `json:"routing_class"`
	InKeyspace     bool            `json:"in_keyspace"`
	MeasuredEvents int             `json:"measured_events"`
	DispatchReqvec json.RawMessage `json:"dispatch_reqvec"` // an 8-dim int object OR the string "absent"
}

type routeCandidatesResp struct {
	Dark       bool                `json:"dark"`
	Error      string              `json:"error"`
	Decision   string              `json:"decision"`
	TaskReqvec string              `json:"task_reqvec"`
	Candidates []routeCandidateRow `json:"candidates"`
}

// reqvecCanonicalDims is the frozen 8-dim requirement_vector keyset (producer contract). A measured
// dispatch_reqvec must carry ALL eight — a partial object is treated as ABSENT, never zero-filled.
var reqvecCanonicalDims = []string{
	"quality_floor", "information_scope", "context_length", "mutation_risk",
	"verification_demand", "ambiguity_novelty", "composition_coupling", "governance_sensitivity",
}

func reqvecComplete(m map[string]int) bool {
	if len(m) != len(reqvecCanonicalDims) {
		return false
	}
	for _, d := range reqvecCanonicalDims {
		if _, ok := m[d]; !ok {
			return false
		}
	}
	return true
}

// FetchRoute reads the honest ROUTE projection (U4): /route/posture + /route/candidates. Returns
// (posture, candidates, dark, err). dark=true (unreachable/errored feed) renders the :route page
// honest-dark. The polymorphic dispatch_reqvec (an 8-dim map OR "absent") is decoded here: a complete
// int map => ReqvecMeasured, anything else (the "absent" sentinel) => not measured (never fabricated).
func FetchRoute(apiURL string) (grammar.RoutePosture, []grammar.RouteCandidate, bool, error) {
	c := newReadHTTPClient()
	var posture grammar.RoutePosture
	resp, err := c.Get(apiURL + "/route/posture")
	if err != nil {
		return posture, nil, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/route/posture"); err != nil {
		return posture, nil, true, err
	}
	if err := json.NewDecoder(resp.Body).Decode(&posture); err != nil {
		return posture, nil, true, err
	}

	cresp, err := c.Get(apiURL + "/route/candidates")
	if err != nil {
		return posture, nil, posture.Dark, nil // posture rendered; candidates dark
	}
	defer cresp.Body.Close()
	if err := checkOK(cresp, "/route/candidates"); err != nil {
		return posture, nil, posture.Dark, nil
	}
	var cr routeCandidatesResp
	if err := json.NewDecoder(cresp.Body).Decode(&cr); err != nil {
		return posture, nil, posture.Dark, nil
	}
	cands := make([]grammar.RouteCandidate, 0, len(cr.Candidates))
	for _, row := range cr.Candidates {
		gc := grammar.RouteCandidate{
			RoutingClass: row.RoutingClass, InKeyspace: row.InKeyspace,
			MeasuredEvents: row.MeasuredEvents,
		}
		// measured ONLY when a COMPLETE 8-dim vector decodes — a partial object (or the "absent"
		// string) stays unmeasured, so the render says ABSENT rather than fabricating q0 for the
		// missing dims (the zero-vector honesty failure class).
		var m map[string]int
		if len(row.DispatchReqvec) > 0 && json.Unmarshal(row.DispatchReqvec, &m) == nil && reqvecComplete(m) {
			gc.DispatchReqvec = m
			gc.ReqvecMeasured = true
		}
		cands = append(cands, gc)
	}
	return posture, cands, posture.Dark, nil
}

// FetchCommands reads the witnessed command ledger projection (/read/commands): demand+verdict datoms
// with an honest witness state + the enforcement cell. Returns (commands, enforcement, dark, err).
func FetchCommands(apiURL string) ([]grammar.Command, string, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(apiURL + "/read/commands")
	if err != nil {
		return nil, "dark", true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/commands"); err != nil {
		return nil, "dark", true, err
	}
	var r commandsResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, "dark", true, err
	}
	out := make([]grammar.Command, 0, len(r.Commands))
	for _, cr := range r.Commands {
		out = append(out, grammar.Command{
			Verb: cr.Verb, Target: cr.Target, Status: cr.Status, Witness: cr.Witness,
			TaskID: cr.TaskID, SessionRole: cr.SessionRole, AIR: cr.AIR,
		})
	}
	enf := r.Enforcement
	if enf == "" {
		enf = "absent"
	}
	if r.Error != "" {
		return out, enf, true, fmt.Errorf("%s", r.Error)
	}
	return out, enf, r.Dark, nil
}

// FetchObserve reads the whole-system awareness aggregate (/read/observe) — per-dimension live/dark, raw
// (the Go renderObserve applies AIR). dark=true renders the :observe page honest-dark.
func FetchObserve(apiURL string) ([]grammar.ObserveDimension, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(apiURL + "/read/observe")
	if err != nil {
		return nil, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/observe"); err != nil {
		return nil, true, err
	}
	var r observeResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, true, err
	}
	if r.Error != "" {
		return r.Dimensions, true, fmt.Errorf("%s", r.Error)
	}
	return r.Dimensions, r.Dark, nil
}

// FetchVault reads the Obsidian vault note METADATA (/read/vault) — titles/paths/links only, never
// bodies. dark=true (unreachable or root missing) renders the :vault page honest-dark.
func FetchVault(apiURL string) ([]grammar.VaultNote, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(apiURL + "/read/vault")
	if err != nil {
		return nil, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/vault"); err != nil {
		return nil, true, err
	}
	var r vaultResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, true, err
	}
	if r.Error != "" {
		return r.Notes, true, fmt.Errorf("%s", r.Error)
	}
	return r.Notes, r.Dark, nil
}

type gatesResp struct {
	Dark  bool                `json:"dark"`
	Error string              `json:"error"`
	Gates grammar.GateSummary `json:"gates"`
}

// FetchGates GETs the source-backed readiness/gate projection.
func FetchGates(apiURL string) (grammar.GateSummary, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(apiURL + "/read/gates")
	if err != nil {
		return grammar.GateSummary{}, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/gates"); err != nil {
		return grammar.GateSummary{}, true, err
	}
	var r gatesResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return grammar.GateSummary{}, true, err
	}
	if r.Error != "" {
		return r.Gates, true, fmt.Errorf("%s", r.Error)
	}
	return r.Gates, r.Dark, nil
}

type domainsResp struct {
	Dark    bool                  `json:"dark"`
	Error   string                `json:"error"`
	Domains grammar.DomainSummary `json:"domains"`
}

// FetchDomains GETs the optional source-backed lifecycle/domain projection.
func FetchDomains(apiURL string) (grammar.DomainSummary, bool, error) {
	c := newReadHTTPClient()
	resp, err := c.Get(apiURL + "/read/domains")
	if err != nil {
		return grammar.DomainSummary{}, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	if err := checkOK(resp, "/read/domains"); err != nil {
		return grammar.DomainSummary{}, true, err
	}
	var r domainsResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return grammar.DomainSummary{}, true, err
	}
	if r.Error != "" {
		return r.Domains, true, fmt.Errorf("%s", r.Error)
	}
	return r.Domains, r.Dark, nil
}
