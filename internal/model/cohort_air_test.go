package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// The criticality HUE is a derived channel: even when the criticality VALUE redacts, a severity tint
// on the (allowlisted) stage/state cell discloses crit/major. taskWorkDomainPane + taskRail both pass
// criticality through airSeverityToken, which demotes the hue to "mut" when criticality is denied on
// air. Tested at the helper (color is disabled in the test env, so a rendered-hue test cannot run).
func TestAirSeverityTokenDemotesDeniedCriticality(t *testing.T) {
	if got := airSeverityToken("crit", map[string]string{"criticality": "deny"}, true); got != "mut" {
		t.Fatalf("on air a denied criticality must demote the hue to mut, got %q", got)
	}
	if got := airSeverityToken("crit", map[string]string{"criticality": "ok"}, true); got != grammar.SeverityToken("crit") {
		t.Fatalf("an allowed criticality airs its severity hue, got %q", got)
	}
	if got := airSeverityToken("crit", map[string]string{"criticality": "deny"}, false); got != grammar.SeverityToken("crit") {
		t.Fatalf("off air the hue is unchanged, got %q", got)
	}
}

// eventContextPane's failures/successes counters classify the raw kind; a denied kind must not be
// counted into the on-air "N recent" tally.
func TestEventContextPaneFailureCountIsAirSafe(t *testing.T) {
	focus := grammar.Event{Kind: "deploy.start", Subject: "svc", AIR: map[string]string{"kind": "ok", "subject": "ok"}}
	deniedFail := grammar.Event{Kind: "deploy.fail", Subject: "svc", AIR: map[string]string{"kind": "deny", "subject": "ok"}}
	m := New("R").Fold([]grammar.Event{focus, deniedFail}, false)

	m.AIR = false
	if !strings.Contains(ansi.Strip(m.eventContextPane(80)), "1 recent") {
		t.Fatalf("off air the fail event counts (failures: 1 recent)")
	}
	m.AIR = true
	if strings.Contains(ansi.Strip(m.eventContextPane(80)), "1 recent") {
		t.Fatalf("on air a denied-kind fail must not be counted (no '1 recent')")
	}
}

// renderSelectedIntakeLane's "N failures" tally over the session's related events must not count a
// denied-kind fail on air.
func TestIntakeLaneFailureCountIsAirSafe(t *testing.T) {
	sess := grammar.Session{Role: "cx-p0", AIR: map[string]string{"role": "ok"}}
	failEv := grammar.Event{Actor: "cx-p0", Kind: "deploy.fail", AIR: map[string]string{"kind": "deny", "actor": "ok"}}
	m := New("R").FoldSessions([]grammar.Session{sess}, false).Fold([]grammar.Event{failEv}, false)

	m.AIR = false
	if !strings.Contains(ansi.Strip(m.renderSelectedIntakeLane(80)), "1 failures") {
		t.Fatalf("off air the related fail event counts (1 failures)")
	}
	m.AIR = true
	if strings.Contains(ansi.Strip(m.renderSelectedIntakeLane(80)), "1 failures") {
		t.Fatalf("on air a denied-kind fail must not be counted")
	}
}
