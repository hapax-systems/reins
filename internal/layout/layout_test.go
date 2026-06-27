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

func TestRenderIsPureAndHandlesEmpty(t *testing.T) {
	s := Split(Leaf(block("A", 1)), Leaf(block("B", 1)), 0.5, Connector{Glyph: "│"})
	if Render(s, 20, 2) != Render(s, 20, 2) {
		t.Fatal("Render must be pure (same input -> same output)")
	}
	if Render(nil, 10, 10) != "" || Render(Leaf(block("A", 1)), 0, 5) != "" || Render(Leaf(block("A", 1)), 5, 0) != "" {
		t.Fatal("nil spec or non-positive dims must render empty (no panic)")
	}
}
