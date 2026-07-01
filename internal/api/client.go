package api

import (
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
	ServingSHA string `json:"serving_sha"`
	APITreeSHA string `json:"api_tree_sha"`
	Router     string `json:"router"`
	// Foreign is set by FetchMeta (not the wire) when the port answers without the reins
	// identity — the cockpit renders PORT: FOREIGN SERVER.
	Foreign   bool   `json:"-"`
	Reachable bool   `json:"-"`
	Detail    string `json:"-"`
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
