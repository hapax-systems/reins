package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// The API's stale-producer row, as /read/sessions serves it: the producer's last tick is not now, so
// every liveness field is null and state/readiness say unknown. Nulls decode to false/0 in Go; the
// cockpit must render them as unknown, never as a measured false or a zero age.
const staleProducerRow = `{"role":"lane-01","session":"sess-01","platform":"codex","state":"unknown",
"alive":null,"idle":null,"stalled":null,"claimed_task":"task-01","output_age_s":null,"relay_age_s":null,
"readiness":"unknown","blocker":"stale_producer","attention":0.26,
"air":{"role":"ok","state":"ok","alive":"ok","idle":"ok","stalled":"ok","readiness":"ok","blocker":"ok",
"attention":"ok","output_age_s":"ok","relay_age_s":"ok","platform":"ok"}}`

func decodeStaleProducerRow(t *testing.T, role string) grammar.Session {
	t.Helper()
	var s grammar.Session
	if err := json.Unmarshal([]byte(staleProducerRow), &s); err != nil {
		t.Fatalf("a stale-producer row with null liveness must decode: %v", err)
	}
	s.Role = role
	return s
}

func TestStaleProducerRowLivenessValuesReadUnknown(t *testing.T) {
	s := decodeStaleProducerRow(t, "lane-01")
	for _, f := range []string{"alive", "idle", "stalled", "output_age_s", "relay_age_s"} {
		if got := sessionFieldValue(s, f); got != "unknown" {
			t.Fatalf("sessionFieldValue(%s) = %q for a stale-producer row, want unknown", f, got)
		}
	}
	m := New("R").FoldSessions([]grammar.Session{s}, false)
	m.SFocus = 0
	for _, key := range []string{"o", "l"} {
		if _, got, _ := m.yankSessionField(key); got != "unknown" {
			t.Fatalf("yank %s = %q for a stale-producer row, want unknown", key, got)
		}
	}
	pick := ansi.Strip(sessionPickRow(s, false, ""))
	if strings.Contains(pick, "0.0") {
		t.Fatalf("pick row renders a zero age for an unknown one: %q", pick)
	}
}

func TestStaleProducerRowPanesDoNotShowZeroAges(t *testing.T) {
	s := decodeStaleProducerRow(t, "lane-01")
	m := New("R").FoldSessions([]grammar.Session{s}, false)
	m.Width = 140
	for name, out := range map[string]string{
		"constraint pane": ansi.Strip(m.sessionConstraintPane(140)),
		"session rail":    ansi.Strip(m.sessionRail(140)),
		"session door":    ansi.Strip(grammar.RenderSessionDoor(s, grammar.SessionDetail{}, false, false, "", false, 100, 40)),
		"session row":     ansi.Strip(grammar.RenderSessionRow(s, false)),
	} {
		if strings.Contains(out, "0.0s") {
			t.Fatalf("%s renders a zero age for an unknown one:\n%s", name, out)
		}
		if !strings.Contains(out, "unknown") {
			t.Fatalf("%s must say unknown for a stale-producer row:\n%s", name, out)
		}
	}
}

// Every lane of a dead producer carries blocker=stale_producer. The lanes do not need the operator;
// the producer does. The breakdown inbox must name that once, not list every lane as blocked, and the
// fleet rail must not rank an unknown lane as blocked.
func TestStaleProducerLanesAreNotEachABreakdown(t *testing.T) {
	rows := []grammar.Session{
		decodeStaleProducerRow(t, "lane-01"),
		decodeStaleProducerRow(t, "lane-02"),
		decodeStaleProducerRow(t, "lane-03"),
	}
	m := New("R").FoldSessions(rows, false)
	m.Width, m.Page = 200, PageSessionTurns

	box := ansi.Strip(m.turnBreakdownInbox(200))
	if strings.Contains(box, "(blocked)") {
		t.Fatalf("stale-producer lanes are unknown, not blocked:\n%s", box)
	}
	if !strings.Contains(box, "producer stale") || !strings.Contains(box, "3 lanes") {
		t.Fatalf("the breakdown must name the stale producer once, with the lane count:\n%s", box)
	}
	rail := ansi.Strip(m.turnLaneRail(200))
	if strings.Contains(rail, "lane-01 blocked") || strings.Contains(rail, "lane-03 blocked") {
		t.Fatalf("the fleet rail must not rank an unknown lane as blocked:\n%s", rail)
	}
	if !strings.Contains(rail, "lane-01 unknown") {
		t.Fatalf("the fleet rail must label an unknown lane unknown:\n%s", rail)
	}
}
