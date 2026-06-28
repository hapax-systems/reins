package model

import (
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

func TestAuditEmptyRoster(t *testing.T) {
	var m Model // zero value: nil Sessions/Events/Tasks/TurnLadder
	r := m.identityRoster()
	if r == nil {
		t.Fatal("roster nil")
	}
	t.Logf("empty roster len=%d", len(r))
	cf := m.consentFacets()
	t.Logf("facets len=%d", len(cf))
	for _, f := range cf {
		if f.Key == "stakeholders" {
			t.Logf("stakeholders summary=%q dangerLine0=%q", f.Summary, f.Lines[0])
		}
		if f.Key == "authorship" {
			t.Logf("authorship summary=%q", f.Summary)
		}
	}
	// focus motions on empty
	m = m.identityFocusTo(5)
	t.Logf("identityFocus after focusTo(5) empty = %d status=%q", m.IdentityFocus, m.Status)
	m = m.identityFocusTo(-3)
	t.Logf("identityFocus after focusTo(-3) empty = %d", m.IdentityFocus)
	m = m.relationalFocusTo(99)
	t.Logf("relationalFocus after focusTo(99) = %d", m.RelationalFocus)
	// bodies on empty must not panic
	_ = m.identityListBody(80, 24)
	_ = m.identityDetailBody(80)
	_ = m.relationalListBody(80, 24)
	_ = m.relationalDetailBody(80)
	_ = m.axisListBody(80, 24)
}

func TestAuditStaleFocus(t *testing.T) {
	var m Model
	m.Sessions = []grammar.Session{{Role: "cc-a"}, {Role: "cc-b"}}
	m.IdentityFocus = 50 // stale high
	body := m.identityListBody(80, 24)
	t.Logf("stale-focus list body:\n%s", body)
	det := m.identityDetailBody(80)
	t.Logf("stale-focus detail clamps:\n%s", det)
}

func TestAuditDuplicateAcrossSurfaces(t *testing.T) {
	var m Model
	m.Sessions = []grammar.Session{{Role: "cc-x"}, {Role: "cc-x"}}
	m.Events = []grammar.Event{{Actor: "cc-x"}, {Actor: "  "}, {Actor: ""}}
	m.Tasks = []grammar.Task{{Owner: "cc-x"}}
	r := m.identityRoster()
	for _, id := range r {
		t.Logf("name=%q class=%q s=%d e=%d t=%d", id.Name, id.Class, id.Sessions, id.Events, id.Tasks)
	}
	if len(r) != 1 {
		t.Fatalf("expected 1 deduped principal, got %d", len(r))
	}
}

func TestAuditUnknownProv(t *testing.T) {
	var m Model
	m.TurnLadder = []grammar.Turn{{Prov: "weird"}, {Prov: "operator"}}
	cf := m.consentFacets()
	for _, f := range cf {
		if f.Key == "authorship" {
			t.Logf("mixed-known/unknown prov summary=%q", f.Summary)
		}
	}
	// all-unknown
	m.TurnLadder = []grammar.Turn{{Prov: "weird"}, {Prov: "xyz"}}
	cf = m.consentFacets()
	for _, f := range cf {
		if f.Key == "authorship" {
			t.Logf("all-unknown prov (3 turns present) summary=%q", f.Summary)
		}
	}
}
