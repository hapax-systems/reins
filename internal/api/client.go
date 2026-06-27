package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/hapax-systems/reins/internal/grammar"
)

type readResp struct {
	Dark   bool            `json:"dark"`
	Error  string          `json:"error"`
	Events []grammar.Event `json:"events"`
}

func checkOK(resp *http.Response, endpoint string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("reins: READ api %s returned %s", endpoint, resp.Status)
}

// FetchEvents GETs the READ endpoint. Returns (events, dark, err).
func FetchEvents(url string) ([]grammar.Event, bool, error) {
	c := &http.Client{Timeout: 3 * time.Second}
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
	c := &http.Client{Timeout: 3 * time.Second}
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
	c := &http.Client{Timeout: 3 * time.Second}
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
	c := &http.Client{Timeout: 3 * time.Second}
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
	c := &http.Client{Timeout: 3 * time.Second}
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
	c := &http.Client{Timeout: 3 * time.Second}
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
	c := &http.Client{Timeout: 3 * time.Second}
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
	c := &http.Client{Timeout: 3 * time.Second}
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
	c := &http.Client{Timeout: 3 * time.Second}
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

type intakeResp struct {
	Dark   bool                  `json:"dark"`
	Error  string                `json:"error"`
	Intake grammar.IntakeSummary `json:"intake"`
}

// FetchIntake GETs the bounded intake-observation projection. Returns (summary, dark, err).
func FetchIntake(apiURL string) (grammar.IntakeSummary, bool, error) {
	c := &http.Client{Timeout: 3 * time.Second}
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
	c := &http.Client{Timeout: 3 * time.Second}
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

type gatesResp struct {
	Dark  bool                `json:"dark"`
	Error string              `json:"error"`
	Gates grammar.GateSummary `json:"gates"`
}

// FetchGates GETs the source-backed readiness/gate projection.
func FetchGates(apiURL string) (grammar.GateSummary, bool, error) {
	c := &http.Client{Timeout: 3 * time.Second}
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
	c := &http.Client{Timeout: 3 * time.Second}
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
