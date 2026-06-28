package model

import (
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
