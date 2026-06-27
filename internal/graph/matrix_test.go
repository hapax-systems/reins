package graph

import (
	"strings"
	"testing"
)

func TestAdjacencyMatrixMarksAndSymmetry(t *testing.T) {
	g := New()
	g.Add(Relation{Src: "A", Dst: "B", Type: "feeds"})     // + (A->B)
	g.Add(Relation{Src: "B", Dst: "A", Type: "blocks"})    // - (B->A)  => A<->B feedback
	g.Add(Relation{Src: "B", Dst: "C", Type: "co-occurs"}) // · (unsigned)

	lines := g.AdjacencyMatrix()
	joined := strings.Join(lines, "\n")

	// legend present
	for _, want := range []string{" 0 A", " 1 B", " 2 C"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("legend missing %q:\n%s", want, joined)
		}
	}

	// row lines: "  %2d " gutter (5 chars) then cells; cell j at index 5+j.
	rows := map[int]string{}
	for _, ln := range lines {
		if len(ln) > 5 && (ln[:4] == "   0" || ln[:4] == "   1" || ln[:4] == "   2") {
			// parse the index
			var i int
			switch strings.TrimSpace(ln[:4]) {
			case "0":
				i = 0
			case "1":
				i = 1
			case "2":
				i = 2
			}
			rows[i] = ln
		}
	}
	cell := func(row int, col int) rune { return []rune(rows[row])[5+col] }

	if cell(0, 0) != '╲' {
		t.Fatalf("diagonal A,A must be ╲, got %q", string(cell(0, 0)))
	}
	if cell(0, 1) != '+' {
		t.Fatalf("A->B must be '+', got %q", string(cell(0, 1)))
	}
	if cell(1, 0) != '-' {
		t.Fatalf("B->A must be '-', got %q", string(cell(1, 0)))
	}
	if cell(1, 2) != '·' {
		t.Fatalf("B->C unsigned must be '·', got %q", string(cell(1, 2)))
	}
	if cell(0, 2) != ' ' {
		t.Fatalf("A->C (no edge) must be blank, got %q", string(cell(0, 2)))
	}
	// feedback = off-diagonal symmetry: both (0,1) and (1,0) marked
	m01, m10 := cell(0, 1) != ' ', cell(1, 0) != ' '
	if !(m01 && m10) {
		t.Fatal("A<->B feedback must show as off-diagonal symmetry (both cells marked)")
	}
}
