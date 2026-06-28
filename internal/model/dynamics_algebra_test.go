package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// Inc 3 TRANSFORM — PageDynamics migrates onto the view-algebra as a SELF-ANCHORED page (own DynFocus),
// ABOLISHING the legacy session-frozen reference split (sessions │ dynamics map). The primary IS the
// navigable map document (header + scale + graph rail + orientation, minus the full inline element); the
// secondary is the focused element's full detail (renderDynamicsSelectedElement, reused). j moves
// DynFocus NATIVELY; the connector is the honest "selected map element ← navigate the map" elucidation.

func dynamicsAlgebraModel() Model {
	m := New("REINS").FoldDynamics(grammar.Graph{
		Layers: []grammar.Layer{{ID: "L", Label: "Layer"}},
		Nodes: []grammar.Node{
			{ID: "node-alpha", Label: "Alpha", Layer: "L", Kind: "governance", Status: "asserted", Res: "1",
				AIR: map[string]string{"id": "ok", "label": "ok", "layer": "ok", "kind": "ok", "status": "ok", "res": "ok"}},
			{ID: "node-beta", Label: "Beta", Layer: "L", Kind: "governance", Status: "observed", Res: "1",
				AIR: map[string]string{"id": "ok", "label": "ok", "layer": "ok", "kind": "ok", "status": "ok", "res": "ok"}},
		},
	}, false)
	m.Width, m.Height, m.Page = 200, 48, PageDynamics
	m.DynFocus = 0
	return m
}

func TestDynamicsComposesViaAlgebraNativeBinding(t *testing.T) {
	m := dynamicsAlgebraModel()
	if !m.composesViaAlgebra() {
		t.Fatal("PageDynamics must compose via the view-algebra (only-split)")
	}
	if m.splitContextActive() {
		t.Fatal("a migrated page must NOT be session-frozen (splitContextActive==false)")
	}
	if m.commandSelectionPage() != PageDynamics {
		t.Fatalf("templates/yank must bind to PageDynamics, got page %d", m.commandSelectionPage())
	}
}

func TestDynamicsJMovesElementNatively(t *testing.T) {
	m := dynamicsAlgebraModel()
	before := m.DynFocus
	m = step(m, "j")
	if m.DynFocus != before+1 {
		t.Fatalf("j must move the dynamics element natively (DynFocus %d→%d)", before, m.DynFocus)
	}
}

func TestDynamicsFocusTemplateBindsToElement(t *testing.T) {
	m := dynamicsAlgebraModel()
	got := m.resolveTemplate("element {{focus}}")
	if !strings.Contains(got, "node-alpha") {
		t.Fatalf("{{focus}} must resolve to the focused element id, got %q", got)
	}
	if strings.Contains(got, "{{focus}}") {
		t.Fatalf("{{focus}} must not render literally, got %q", got)
	}
	m = step(m, "j")
	if got := m.resolveTemplate("{{focus}}"); !strings.Contains(got, "node-beta") {
		t.Fatalf("{{focus}} must track the moved element, got %q", got)
	}
}

func TestDynamicsFocusTemplateIsAirSafe(t *testing.T) {
	m := New("REINS").FoldDynamics(grammar.Graph{
		Layers: []grammar.Layer{{ID: "L", Label: "L"}},
		Nodes: []grammar.Node{
			{ID: "SECRET-NODE", Label: "x", Layer: "L", Kind: "governance", Status: "asserted", Res: "1",
				AIR: map[string]string{"id": "deny", "label": "ok", "layer": "ok", "kind": "ok", "status": "ok", "res": "ok"}},
		},
	}, false)
	m.Width, m.Height, m.Page = 200, 48, PageDynamics
	m.AIR = true
	if got := m.resolveTemplate("{{focus}}"); strings.Contains(got, "SECRET-NODE") {
		t.Fatalf("on air {{focus}} must not leak a denied element id, got %q", got)
	}
}

func TestDynamicsViewIsAlgebraSplitWithElementSecondary(t *testing.T) {
	m := dynamicsAlgebraModel()
	v := ansi.Strip(m.View())
	if strings.Contains(v, "split sessions") || strings.Contains(v, "[j/k]source") {
		t.Fatalf("migrated dynamics must NOT render the legacy session-frozen split:\n%s", v)
	}
	flat := flattenSplitColumns(v)
	if !strings.Contains(flat, "SELECTED MAP ELEMENT") {
		t.Fatalf("the secondary must show the focused element detail:\n%s", v)
	}
	if !strings.Contains(flat, "node-alpha") {
		t.Fatalf("the focused element must render:\n%s", v)
	}
}

func TestDynamicsPrimaryPaneScrollsTallDocument(t *testing.T) {
	rowAIR := func() map[string]string {
		return map[string]string{"kind": "ok", "id": "ok", "source": "ok", "status": "ok", "severity": "ok", "count": "ok", "detail": "ok"}
	}
	sourceAIR := func() map[string]string {
		return map[string]string{"id": "ok", "status": "ok", "count": "ok", "detail": "ok", "age_bucket": "ok", "path": "ok", "privacy": "ok", "raw_access": "ok"}
	}
	rows := func(prefix string, n int) []grammar.DynamicsRow {
		out := make([]grammar.DynamicsRow, 0, n)
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("%s-%02d", prefix, i)
			if prefix == "relation" && i == n-1 {
				id = "relation-bottom-sentinel"
			}
			out = append(out, grammar.DynamicsRow{
				Kind: prefix, ID: id, Source: "test-package", Status: "observed", Count: i + 1,
				Detail: fmt.Sprintf("detail row %02d for %s; intentionally long enough to wrap in the narrow primary pane", i, prefix),
				AIR:    rowAIR(),
			})
		}
		return out
	}
	m := New("REINS").FoldDynamics(grammar.Graph{
		MapID:  "scroll-test",
		Thesis: "scrollable dynamics document",
		Layers: []grammar.Layer{{ID: "L", Label: "Layer"}},
		Nodes: []grammar.Node{
			{ID: "node-alpha", Label: "Alpha", Layer: "L", Kind: "governance", Status: "asserted", Res: "1",
				AIR: map[string]string{"id": "ok", "label": "ok", "layer": "ok", "kind": "ok", "status": "ok", "res": "ok"}},
			{ID: "node-beta", Label: "Beta", Layer: "L", Kind: "runtime", Status: "observed", Res: "1",
				AIR: map[string]string{"id": "ok", "label": "ok", "layer": "ok", "kind": "ok", "status": "ok", "res": "ok"}},
		},
		Package: grammar.DynamicsPackage{
			Sources: []grammar.DynamicsSource{{
				ID: "package-source", Status: "observed", Count: 9, Detail: "source package", AgeBucket: "<1h", Path: "package.json", Privacy: "metadata-only",
				AIR: sourceAIR(),
			}},
			Validation:   rows("validation", 4),
			Lenses:       rows("lens", 4),
			Claims:       rows("claim", 4),
			Observations: rows("observation", 4),
			Relations:    rows("relation", 6),
		},
	}, false)
	m.Width, m.Height, m.Page = 120, 36, PageDynamics

	primaryW, primaryH := dynamicsAlgebraPrimaryPaneDims(m.Width, m.Height)
	if got := len(strings.Split(m.dynamicsMapBody(primaryW, primaryH), "\n")); got > primaryH {
		t.Fatalf("dynamics primary pane must stay within its height budget: got %d lines, want <= %d", got, primaryH)
	}
	maxScroll := m.referenceScrollMax()
	if maxScroll == 0 {
		t.Fatal("test requires a scrollable dynamics primary pane")
	}
	top := ansi.Strip(m.View())
	if !strings.Contains(top, "↓") {
		t.Fatalf("top dynamics window should disclose hidden lower content:\n%s", top)
	}
	if strings.Contains(top, "relation-bottom-sentinel") {
		t.Fatalf("bottom package detail should not be visible before scrolling:\n%s", top)
	}

	beforeFocus := m.DynFocus
	m = step(m, "J")
	if m.RefScroll != 1 || m.DynFocus != beforeFocus {
		t.Fatalf("uppercase J should scroll the dynamics primary pane without moving map focus, scroll=%d focus=%d", m.RefScroll, m.DynFocus)
	}
	if after := ansi.Strip(m.View()); after == top {
		t.Fatal("scrolling the dynamics primary pane should change the rendered window")
	}
	for m.RefScroll < maxScroll {
		m = step(m, "J")
	}
	bottom := ansi.Strip(m.View())
	if !strings.Contains(bottom, "↑") {
		t.Fatalf("bottom dynamics window should disclose hidden upper content:\n%s", bottom)
	}
	if !strings.Contains(bottom, "RELATION VOCABULARY") || !strings.Contains(bottom, "relation-bottom-sentinel") {
		t.Fatalf("scrolling should make the bottom dynamics package sections reachable:\n%s", bottom)
	}
	m = step(m, "K")
	if m.RefScroll != maxScroll-1 || m.DynFocus != beforeFocus {
		t.Fatalf("uppercase K should scroll back without moving map focus, scroll=%d focus=%d", m.RefScroll, m.DynFocus)
	}
}
