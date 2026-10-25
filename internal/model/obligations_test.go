package model

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/api"
	"github.com/hapax-systems/reins/internal/grammar"
)

func obligationsJSON(t *testing.T, raw string) grammar.ObligationsReport {
	t.Helper()
	var r grammar.ObligationsReport
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatal(err)
	}
	return r
}
func obligationsView(t *testing.T, raw string, air bool) string {
	t.Helper()
	m := New("fixture").FoldObligations(obligationsJSON(t, raw), false)
	m.AIR = air
	return ansi.Strip(m.renderObligations(200))
}
func obligationsWant(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}
func TestObligationsHonestStates(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `{"dark":null,"partial_dark":null,"summary":null}`} {
		out := obligationsView(t, raw, false)
		obligationsWant(t, out, "source status UNKNOWN", "owes=UNKNOWN", "local_loci_occupied=UNKNOWN/UNKNOWN")
		if strings.Contains(out, "=0") {
			t.Fatal(out)
		}
	}
	for _, raw := range []string{`{"dark":true}`, `{"dark":true,"summary":{"total_owes":9}}`} {
		out := obligationsView(t, raw, false)
		obligationsWant(t, out, "obligations dark")
		if strings.Contains(out, "owes=") {
			t.Fatal(out)
		}
	}
	for _, raw := range []string{`{"dark":false,"partial_dark":true}`, `{"dark":false,"partial_dark":false,"source_status":{"context_bundle":"dark"}}`} {
		obligationsWant(t, obligationsView(t, raw, false), "PARTIAL", "owes=UNKNOWN")
	}
	obligationsWant(t, obligationsView(t, `{"dark":false,"partial_dark":false,"summary":{"total_owes":0,"routes_total":0},"local_substrate_loci":[]}`, false), "owes=0", "routes_occupied_in_memory=UNKNOWN/0", "no local substrate loci observed")
	for _, value := range []string{"null", "-1"} {
		obligationsWant(t, obligationsView(t, `{"dark":false,"summary":{"total_owes":`+value+`}}`, false), "owes=UNKNOWN")
	}
}
func TestObligationsMissingCollectionsAndLocusFields(t *testing.T) {
	for _, suffix := range []string{"", `,"local_substrate_loci":null`} {
		out := obligationsView(t, `{"dark":false`+suffix+`}`, false)
		obligationsWant(t, out, "substrate loci UNKNOWN", "lanebus UNKNOWN", "grants UNKNOWN", "custody UNKNOWN", "anomalies UNKNOWN", "fulfillment UNKNOWN")
		if strings.Contains(out, "no local substrate") {
			t.Fatal(out)
		}
	}
	for _, raw := range []string{`{}`, `{"loaded_models":null,"resident_models_count":null}`} {
		out := obligationsView(t, `{"dark":false,"local_substrate_loci":[`+raw+`]}`, false)
		obligationsWant(t, out, "UNKNOWN:UNKNOWN", "models=UNKNOWN", "res=UNKNOWN")
		if strings.Contains(out, "disk-only") || strings.Contains(out, "●") {
			t.Fatal(out)
		}
	}
	out := obligationsView(t, `{"dark":false,"local_substrate_loci":[{"loaded_models":[],"resident_models_count":0,"state":"dark"}]}`, false)
	obligationsWant(t, out, "models=(none observed)", "res=0", "dark")
}

const obligationsPopulated = `{"dark":false,"partial_dark":true,"source_status":{"context_bundle":"dark","capabilities":"live"},"summary":{"total_owes":2,"local_loci_occupied_in_memory":1,"local_loci_total":1,"routes_occupied_in_memory":1,"routes_total":2},"local_substrate_loci":[{"host":"node-fixture","port":"8000","state":"lit","loaded_models":["model-fixture"],"resident_models_count":1,"topology":"topology-fixture"}],"locus_anomalies":[{"kind":"host.dark","subject":"anomaly-fixture","attention":"warning"}],"lanebus_owes":[{"kind":"lanebus.unacked","creditor":"creditor-fixture","debtor":"debtor-fixture","subject":"subject-fixture"}]}`

func TestObligationsPopulatedAndAIR(t *testing.T) {
	off := obligationsView(t, obligationsPopulated, false)
	obligationsWant(t, off, "PARTIAL", "context_bundle=dark", "owes=2", "local_loci_occupied=1/1", "routes_occupied_in_memory=1/2", "node-fixture:8000", "debtor-fixture")
	on := obligationsView(t, obligationsPopulated, true)
	obligationsWant(t, on, "PARTIAL", "owes=2", "sealed on air")
	for _, secret := range []string{"node-fixture", "8000", "model-fixture", "topology-fixture", "anomaly-fixture", "creditor-fixture", "debtor-fixture", "subject-fixture"} {
		if strings.Contains(on, secret) {
			t.Fatalf("AIR exposed %s: %s", secret, on)
		}
	}
	obligationsWant(t, obligationsView(t, `{"dark":false,"local_substrate_loci":[{}]}`, true), "loaded=UNKNOWN", "resident=UNKNOWN")
}
func TestObligationsClientAndUpdate(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		fail       bool
	}{
		{"populated", obligationsPopulated, 200, false}, {"empty", `{}`, 200, false}, {"null", `null`, 200, false},
		{"invalid", `oops`, 200, true}, {"type", `{"summary":{"total_owes":"bad"}}`, 200, true}, {"http", `{}`, 503, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/read/obligations" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer s.Close()
			rep, dark, err := api.FetchObligations(s.URL)
			if (err != nil) != tc.fail || (tc.fail && !dark) {
				t.Fatalf("dark=%v err=%v", dark, err)
			}
			m, _ := New("fixture").Update(ObligationsMsg{Report: rep, Dark: dark})
			out := ansi.Strip(m.(Model).renderObligations(200))
			if tc.fail {
				obligationsWant(t, out, "obligations dark")
			} else if tc.name == "populated" {
				obligationsWant(t, out, "owes=2", "PARTIAL")
			} else {
				obligationsWant(t, out, "UNKNOWN")
			}
		})
	}
	_, dark, err := api.FetchObligations(":bad-url")
	if err == nil || !dark {
		t.Fatal("transport failure lost darkness")
	}
	m, _ := New("fixture").Update(ObligationsMsg{Report: obligationsJSON(t, obligationsPopulated), Error: "fixture error"})
	obligationsWant(t, ansi.Strip(m.(Model).renderObligations(200)), "obligations dark")
}
func TestObligationsNavigationAndInitialState(t *testing.T) {
	obligationsWant(t, ansi.Strip(New("fixture").renderObligations(200)), "obligations dark")
	for _, cmd := range []string{"obligations", "owes"} {
		m := New("fixture").Exec(cmd)
		m.Width = 200
		m.Height = 80
		if m.Page != PageObligations {
			t.Fatal("missing page", cmd)
		}
		obligationsWant(t, ansi.Strip(m.View()), "OBLIGATIONS", "obligations dark")
		if pageLabel(m.Page) != "PageObligations" {
			t.Fatal("missing page label")
		}
		if m.Exec("tasks").Page != PageTasks {
			t.Fatal("cannot leave for work view")
		}
	}
}

// The wire keys are emitted by api/reins_obligations.py, not display aliases.
func TestObligationsPythonSourceLabels(t *testing.T) {
	out := obligationsView(t, `{"dark":false,"partial_dark":false,"source_status":{"lanebus":"live","grants":"live","claim_custody":"live","seat_succession":"live","capabilities":"live","context_bundle":"live"}}`, false)
	obligationsWant(t, out, "claim_custody=live", "seat_succession=live", "context_bundle=live")
}

func thirteenObligations() Model {
	rep := grammar.ObligationsReport{LanebusOwes: make([]grammar.ObligationRow, 13)}
	for i := range rep.LanebusOwes {
		rep.LanebusOwes[i].Subject = fmt.Sprintf("row-%02d", i)
	}
	m := New("fixture").FoldObligations(rep, false)
	m.Page, m.Width, m.Height = PageObligations, 80, 12
	return m
}
func TestObligationsTruncation(t *testing.T) {
	out := ansi.Strip(thirteenObligations().renderObligations(120))
	if strings.Count(out, "▸") != 12 || strings.Contains(out, "row-12") {
		t.Fatal(out)
	}
	obligationsWant(t, out, "row-11", "… +1 additional obligation rows")
}
func TestObligationsScroll(t *testing.T) {
	m := thirteenObligations()
	before := m.View()
	if strings.Contains(ansi.Strip(before), "row-11") {
		t.Fatal("fixture must overflow the short viewport")
	}
	m = step(m, "j")
	if m.RefScroll != 1 || m.View() == before {
		t.Fatal("j must scroll the obligations viewport")
	}
	m = step(m, "G")
	if m.RefScroll == 0 || m.RefScroll != m.referenceScrollMax() {
		t.Fatal("G must reach the bottom")
	}
	obligationsWant(t, ansi.Strip(m.View()), "row-11", "… +1 additional obligation rows")
}
