package grammar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The five-tuple contract IS the acceptance gate: every axis must carry a complete
// ⟨question·state·controls·provenance·blind-spot⟩ or it is not a legible axis. The state must name
// its explicit ∅ (empty), and the status must be one of the three honest badges.
func TestEveryAxisHasCompleteFiveTuple(t *testing.T) {
	axes := Axes()
	if len(axes) != 6 {
		t.Fatalf("the framework has six case-role axes; got %d", len(axes))
	}
	seen := map[string]bool{}
	for _, a := range axes {
		seen[a.ID] = true
		if a.Name == "" || a.Question == "" || a.State == "" || a.Controls == "" || a.Provenance == "" || a.BlindSpot == "" {
			t.Fatalf("axis %s has an INCOMPLETE five-tuple: %+v", a.ID, a)
		}
		if !strings.Contains(a.State, "∅") {
			t.Fatalf("axis %s state must name its explicit ∅ (empty); got %q", a.ID, a.State)
		}
		switch a.Status {
		case "built", "partial", "pending":
		default:
			t.Fatalf("axis %s has an unknown status %q (want built|partial|pending)", a.ID, a.Status)
		}
	}
	for _, id := range []string{"A1", "A2", "A3", "A4", "A5", "A6"} {
		if !seen[id] {
			t.Fatalf("axis %s missing from the framework", id)
		}
	}
}

func TestAxisRowAndDetailRender(t *testing.T) {
	a := Axes()[0] // A1 Identity
	row := ansi.Strip(RenderAxisRow(a, 100))
	if !strings.Contains(row, "A1") || !strings.Contains(row, "Identity") {
		t.Fatalf("axis row must show the id + name:\n%s", row)
	}
	detail := ansi.Strip(RenderAxisDetail(a, 90))
	for _, want := range []string{"A1", "Identity", "question", "controls", "blind-spot", "∅"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("axis detail must show %q (the five-tuple contract):\n%s", want, detail)
		}
	}
}

// The build status must be honest: exactly A2 + A4 fold live today; A1/A3/A6 are projection-pending,
// A5 is partial — a regression that flips a pending axis to "built" without a backing store is a
// false-green and must fail here.
func TestAxisBuildStatusHonest(t *testing.T) {
	want := map[string]string{"A1": "pending", "A2": "built", "A3": "pending", "A4": "built", "A5": "partial", "A6": "pending"}
	for _, a := range Axes() {
		if want[a.ID] != a.Status {
			t.Fatalf("axis %s status must be %q (honest backing state); got %q", a.ID, want[a.ID], a.Status)
		}
	}
}
