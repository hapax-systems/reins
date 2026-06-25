package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hapax-systems/reins/internal/grammar"
)

type readResp struct {
	Dark   bool            `json:"dark"`
	Error  string          `json:"error"`
	Events []grammar.Event `json:"events"`
}

// FetchEvents GETs the READ endpoint. Returns (events, dark, err).
func FetchEvents(url string) ([]grammar.Event, bool, error) {
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(url + "/read/events?limit=80")
	if err != nil {
		return nil, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	var r readResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, true, err
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
	var r tasksResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, true, err
	}
	return r.Tasks, r.Dark, nil
}

type dynamicsResp struct {
	Dark  bool   `json:"dark"`
	Error string `json:"error"`
	grammar.Graph
}

// FetchDynamics GETs the system-dynamics map. Returns (graph, dark, err).
func FetchDynamics(url string) (grammar.Graph, bool, error) {
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(url + "/read/dynamics")
	if err != nil {
		return grammar.Graph{}, true, fmt.Errorf("reins: READ api unreachable: %w", err)
	}
	defer resp.Body.Close()
	var r dynamicsResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return grammar.Graph{}, true, err
	}
	return r.Graph, r.Dark, nil
}
