package layout

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// block is a test leaf that fills its whole (w,h) budget with one character.
func block(ch string, minW int) *Pane {
	return &Pane{MinW: minW, Render: func(w, h int) string {
		line := strings.Repeat(ch, w)
		lines := make([]string, h)
		for i := range lines {
			lines[i] = line
		}
		return strings.Join(lines, "\n")
	}}
}

func lines(s string) []string { return strings.Split(s, "\n") }

func TestRenderLeafIsExactlySized(t *testing.T) {
	out := Render(Leaf(block("A", 1)), 10, 3)
	ls := lines(out)
	if len(ls) != 3 {
		t.Fatalf("leaf must fill height: want 3 lines, got %d", len(ls))
	}
	for i, l := range ls {
		if ansi.StringWidth(l) != 10 {
			t.Fatalf("leaf line %d width = %d, want 10", i, ansi.StringWidth(l))
		}
	}
}

func TestSplitJoinsWithConnectorAtExactWidths(t *testing.T) {
	s := Split(Leaf(block("A", 1)), Leaf(block("B", 1)), 0.5, Connector{Glyph: "│"})
	out := Render(s, 21, 2) // inner=20, primary=10, connector=1, secondary=10
	for _, l := range lines(out) {
		if ansi.StringWidth(l) != 21 {
			t.Fatalf("split line width = %d, want 21:\n%s", ansi.StringWidth(l), out)
		}
	}
	row := lines(out)[0]
	if !strings.Contains(row, "A") || !strings.Contains(row, "B") || !strings.Contains(row, "│") {
		t.Fatalf("split row missing panes/connector: %q", row)
	}
	if strings.Count(row, "A") != 10 || strings.Count(row, "B") != 10 {
		t.Fatalf("widths wrong — A=%d B=%d, want 10/10: %q", strings.Count(row, "A"), strings.Count(row, "B"), row)
	}
}

func TestNarrowAlwaysStaysSplit(t *testing.T) {
	// Operator ruling (2026-06-27): split-as-default and ONLY split — never collapse to a single
	// pane, so layout contexts stay few and expectations hold. Even when narrow, BOTH panes render
	// (their content self-degrades); the split structure never disappears.
	s := Split(Leaf(block("A", 8)), Leaf(block("B", 8)), 0.5, Connector{Glyph: "│"})
	for _, w := range []int{21, 11, 7} {
		row := lines(Render(s, w, 1))[0]
		if !strings.Contains(row, "A") || !strings.Contains(row, "B") || !strings.Contains(row, "│") {
			t.Fatalf("only-split: both panes + connector must always render at w=%d: %q", w, row)
		}
		if ansi.StringWidth(row) != w {
			t.Fatalf("split must fill the width exactly at w=%d, got %d", w, ansi.StringWidth(row))
		}
	}
}

func TestRelationHeaderOwnsTopRow(t *testing.T) {
	s := Split(Leaf(block("A", 1)), Leaf(block("B", 1)), 0.5, Connector{Glyph: "│", Relation: "drives"})
	out := Render(s, 20, 3)
	ls := lines(out)
	if len(ls) != 3 {
		t.Fatalf("want 3 lines (header + 2 body), got %d", len(ls))
	}
	if !strings.Contains(ls[0], "drives") {
		t.Fatalf("relation header must occupy the top row: %q", ls[0])
	}
	if strings.Contains(ls[1], "drives") {
		t.Fatalf("the body rows must not repeat the header")
	}
}

// The connector header states the typed-join VERDICT honestly: a real-join split appends its asserted
// join key (⋈), and a DOOR appends "no join" — the honesty floor: a Door must never imply a join the
// data does not carry. Verdict/JoinKey unset preserves the bare-relation header (back-compat).
func TestRelationHeaderIncludesCouplingWhenSet(t *testing.T) {
	w := 48
	with := relationHeader(Connector{Relation: "ambient context", Coupling: "Door"}, w)
	if !strings.Contains(with, "ambient context · Door") {
		t.Fatalf("coupling word must be included inside the centered label: %q", with)
	}
	if got := ansi.StringWidth(with); got != w {
		t.Fatalf("header with coupling width = %d, want %d: %q", got, w, with)
	}

	without := relationHeader(Connector{Relation: "ambient context"}, w)
	if strings.Contains(without, "Door") || strings.Contains(without, " · ") {
		t.Fatalf("empty coupling must be omitted from the label: %q", without)
	}
	if got := ansi.StringWidth(without); got != w {
		t.Fatalf("header without coupling width = %d, want %d: %q", got, w, without)
	}
}

func TestRelationHeaderStatesJoinVerdict(t *testing.T) {
	joined := Split(Leaf(block("A", 1)), Leaf(block("B", 1)), 0.5,
		Connector{Glyph: "│", Relation: "focused event → neighborhood", Verdict: "Standing", JoinKey: "selection -> detail"})
	jh := lines(Render(joined, 80, 3))[0]
	if !strings.Contains(jh, "focused event → neighborhood") {
		t.Fatalf("the emergent relation must still head the connector: %q", jh)
	}
	if !strings.Contains(jh, "⋈") || !strings.Contains(jh, "selection -> detail") {
		t.Fatalf("a real-join split must assert its join key: %q", jh)
	}

	door := Split(Leaf(block("A", 1)), Leaf(block("B", 1)), 0.5,
		Connector{Glyph: "│", Relation: "ambient context", Verdict: "Door", JoinKey: ""})
	dh := lines(Render(door, 80, 3))[0]
	if strings.Contains(dh, "⋈") {
		t.Fatalf("a DOOR must NOT assert a join key (honesty floor): %q", dh)
	}
	if !strings.Contains(dh, "no join") {
		t.Fatalf("a DOOR must declare it carries no join: %q", dh)
	}
}

func TestNestedSplitMakesThreeColumns(t *testing.T) {
	inner := Split(Leaf(block("B", 1)), Leaf(block("C", 1)), 0.5, Connector{Glyph: "│"})
	s := Split(Leaf(block("A", 1)), inner, 0.34, Connector{Glyph: "│"})
	row := lines(Render(s, 31, 1))[0]
	for _, c := range []string{"A", "B", "C"} {
		if !strings.Contains(row, c) {
			t.Fatalf("nested split must yield 3 columns; missing %s: %q", c, row)
		}
	}
	if ansi.StringWidth(row) != 31 {
		t.Fatalf("nested split width = %d, want 31", ansi.StringWidth(row))
	}
}

func TestMalformedSpecsStayExactWidthNoPanic(t *testing.T) {
	// GLM-via-CC review (2026-06-27): a zero-value Spec, a nil child, or a multi-cell connector glyph
	// must not panic and must keep every line exactly w wide.
	zero := Render(&Spec{}, 10, 2) // neither Leaf nor Split
	for _, l := range lines(zero) {
		if ansi.StringWidth(l) != 10 {
			t.Fatalf("zero-value Spec line width = %d, want 10", ansi.StringWidth(l))
		}
	}
	nilChild := Split(Leaf(block("A", 1)), &Spec{}, 0.5, Connector{Glyph: "│"})
	for _, l := range lines(Render(nilChild, 21, 2)) {
		if ansi.StringWidth(l) != 21 {
			t.Fatalf("nil/empty child must not shrink the row: width = %d, want 21", ansi.StringWidth(l))
		}
	}
	multiCell := Split(Leaf(block("A", 1)), Leaf(block("B", 1)), 0.5, Connector{Glyph: "XX"})
	for _, l := range lines(Render(multiCell, 21, 2)) {
		if ansi.StringWidth(l) != 21 {
			t.Fatalf("a multi-cell connector glyph must be normalized to 1: width = %d, want 21", ansi.StringWidth(l))
		}
	}
}

func TestRenderIsPureAndHandlesEmpty(t *testing.T) {
	s := Split(Leaf(block("A", 1)), Leaf(block("B", 1)), 0.5, Connector{Glyph: "│"})
	if Render(s, 20, 2) != Render(s, 20, 2) {
		t.Fatal("Render must be pure (same input -> same output)")
	}
	if Render(nil, 10, 10) != "" || Render(Leaf(block("A", 1)), 0, 5) != "" || Render(Leaf(block("A", 1)), 5, 0) != "" {
		t.Fatal("nil spec or non-positive dims must render empty (no panic)")
	}
}
