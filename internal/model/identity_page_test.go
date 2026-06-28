package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

func identityPageModel() Model {
	m := New("REINS")
	m.Width, m.Height, m.Page, m.SplitContext = 180, 44, PageIdentity, true
	m = m.FoldSessions([]grammar.Session{
		{Role: "cc-secret-lane"},
		{Role: "cc-beta"},
	}, false)
	m = m.Fold([]grammar.Event{
		{TS: "t1", Kind: "dispatch", Subject: "s1", Actor: "cc-secret-lane"},
		{TS: "t2", Kind: "review", Subject: "s2", Actor: "watcher"},
	}, false)
	m = m.FoldTasks([]grammar.Task{
		{TaskID: "task-a", Owner: "cc-owner"},
		{TaskID: "task-b", Owner: "cc-beta"},
	}, false)
	return m
}

func TestIdentityComposesViaAlgebraNativeBinding(t *testing.T) {
	m := identityPageModel()
	m = m.switchPage(PageIdentity)
	if !m.composesViaAlgebra() {
		t.Fatal("PageIdentity must compose via the view algebra")
	}
	if m.splitContextActive() {
		t.Fatal("PageIdentity must not activate the legacy session-frozen split")
	}
	if m.commandSelectionPage() != PageIdentity {
		t.Fatalf("templates/yank context must bind to PageIdentity, got %d", m.commandSelectionPage())
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"IDENTITY ROSTER", "principal -> identity contract", "A1", "projection-pending", "cc-secret-lane"} {
		if !strings.Contains(v, want) {
			t.Fatalf("identity page should render %q via algebra split:\n%s", want, v)
		}
	}
}

func TestIdentityJMovesIdentityFocus(t *testing.T) {
	m := identityPageModel()
	before := m.IdentityFocus
	m = step(m, "j")
	if m.IdentityFocus != before+1 {
		t.Fatalf("j must move IdentityFocus natively (%d→%d)", before, m.IdentityFocus)
	}
	if !strings.Contains(m.Status, "identity 2/") {
		t.Fatalf("j should announce the focused identity, got status %q", m.Status)
	}
}

func TestIdentityListBodyRendersRosterRows(t *testing.T) {
	m := identityPageModel()
	body := ansi.Strip(m.identityListBody(120, 14))
	for _, want := range []string{"IDENTITY ROSTER", "cc-secret-lane", "cc-beta", "cc-owner", "watcher", "mixed", "s1·e1·t0", "s0·e0·t1"} {
		if !strings.Contains(body, want) {
			t.Fatalf("identity list must render roster field %q:\n%s", want, body)
		}
	}
}

func TestIdentityListBodyRedactsNamesOnAir(t *testing.T) {
	m := identityPageModel()
	m.AIR = true
	body := ansi.Strip(m.identityListBody(120, 14))
	if strings.Contains(body, "cc-secret-lane") {
		t.Fatalf("on-air identity list must redact the principal name:\n%s", body)
	}
	if !strings.Contains(body, "▒▒▒") {
		t.Fatalf("on-air identity list must show the redaction token:\n%s", body)
	}
	if !strings.Contains(body, "mixed") || !strings.Contains(body, "s1·e1·t0") {
		t.Fatalf("on-air identity list must keep class + counts skeleton:\n%s", body)
	}
}
