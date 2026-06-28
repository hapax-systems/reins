package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

func TestConsentFacetsHasFourFacets(t *testing.T) {
	m := New("REINS")
	facets := m.consentFacets()
	want := []string{"frame", "authorship", "gating", "stakeholders"}
	if len(facets) != 4 {
		t.Fatalf("A6 has four consent facets; got %d", len(facets))
	}
	for i, f := range facets {
		if f.Key != want[i] {
			t.Fatalf("facet %d should be %q; got %q", i, want[i], f.Key)
		}
		if f.Name == "" || f.Summary == "" || len(f.Lines) == 0 {
			t.Fatalf("facet %q is incomplete: %+v", f.Key, f)
		}
	}
}

func TestConsentFacetsAuthorshipCountsProvenance(t *testing.T) {
	m := New("REINS")
	m.TurnLadder = []grammar.Turn{
		{Prov: "operator"}, {Prov: "model"}, {Prov: "model"}, {Prov: "untrusted"}, {Prov: "structured"},
	}
	auth := m.consentFacets()[1]
	if !strings.Contains(auth.Summary, "●1") || !strings.Contains(auth.Summary, "◐2") ||
		!strings.Contains(auth.Summary, "◌1") || !strings.Contains(auth.Summary, "○1") {
		t.Fatalf("authorship must count the Prov distribution (●1 ◐2 ◌1 ○1); got %q", auth.Summary)
	}
}

// HONEST accounting: a non-canonical / empty provenance must be ACCOUNTED (an unattributed bucket),
// never silently dropped — and the "no turns" message gates on an EMPTY ladder, not the classified
// sum (a full ladder of non-canonical prov must not falsely report empty). The live FetchTurns feeds
// Prov verbatim with no enum normalization, so this is reachable.
func TestConsentFacetsAccountsForNonCanonicalProvenance(t *testing.T) {
	m := New("REINS")
	m.TurnLadder = []grammar.Turn{{Prov: "operator"}, {Prov: "weird"}, {Prov: "xyz"}}
	auth := m.consentFacets()[1]
	if strings.Contains(auth.Summary, "no turns") {
		t.Fatalf("a non-empty ladder must NOT report 'no turns'; got %q", auth.Summary)
	}
	if !strings.Contains(auth.Summary, "?2") {
		t.Fatalf("the 2 non-canonical turns must be accounted as unattributed (?2); got %q", auth.Summary)
	}
}

func TestConsentFrameReflectsAIRToggle(t *testing.T) {
	m := New("REINS")
	if !strings.Contains(m.consentFacets()[0].Summary, "present-at-hand") {
		t.Fatalf("off-air frame should be present-at-hand; got %q", m.consentFacets()[0].Summary)
	}
	m.AIR = true
	if !strings.Contains(m.consentFacets()[0].Summary, "on-air") {
		t.Fatalf("on-air frame should be on-air; got %q", m.consentFacets()[0].Summary)
	}
}

// A6 is the lens ON consent — it must itself be air-safe: the facets carry policy / counts / glyphs
// only, never a PII value. The rendered detail must not contain a principal name even though A6 folds
// the identity-roster COUNT (it shows the count, never the names).
func TestConsentFacetsAreAirSafe(t *testing.T) {
	m := New("REINS")
	m = m.FoldSessions([]grammar.Session{{Role: "cc-secret-principal"}}, false)
	for _, f := range m.consentFacets() {
		row := ansi.Strip(grammar.RenderConsentFacetRow(f, 120))
		detail := ansi.Strip(grammar.RenderConsentFacetDetail(f, 120))
		if strings.Contains(row+detail, "cc-secret-principal") {
			t.Fatalf("A6 facet %q leaked a principal NAME (must show counts, not names):\n%s\n%s", f.Key, row, detail)
		}
	}
	// the stakeholders facet shows the COUNT (1)
	stake := grammar.RenderConsentFacetDetail(m.consentFacets()[3], 120)
	if !strings.Contains(ansi.Strip(stake), "1 distinct principal") {
		t.Fatalf("stakeholders facet must show the principal COUNT:\n%s", ansi.Strip(stake))
	}
}
