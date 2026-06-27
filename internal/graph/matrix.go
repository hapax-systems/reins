package graph

import (
	"fmt"
	"strings"
)

// AdjacencyMatrix renders the graph as a Design-Structure-Matrix — the CELL-NATIVE layout for dense
// and CYCLIC graphs (framework §5b). A monospace grid IS a matrix: zero edge-crossings by
// construction, and feedback appears as OFF-DIAGONAL symmetry (an edge i→j AND j→i = a 2-cycle).
// This is the primary layout where Sugiyama/layered-DAG (node-link) cannot go (it breaks on cycles).
//
// Cell marks (sign-aware, grayscale/on-air safe — color is a later redundant amplifier):
//
//	+  positive link (src→dst move together)    -  negative link (oppose)
//	·  unsigned link                            ╲  the diagonal (self position)
//	(space) no edge
//
// Returns monospace lines: an index→id legend, a column-index header, then one row per source.
func (g *TypedGraph) AdjacencyMatrix() []string {
	nodes := g.Nodes()
	n := len(nodes)
	idx := make(map[string]int, n)
	for i, id := range nodes {
		idx[id] = i
	}
	// edge mark lookup: src-index, dst-index -> rune
	mark := make(map[[2]int]rune)
	for _, e := range g.Edges {
		si, sok := idx[e.Src]
		di, dok := idx[e.Dst]
		if !sok || !dok {
			continue
		}
		r := '·'
		switch e.Sign {
		case SignPos:
			r = '+'
		case SignNeg:
			r = '-'
		}
		mark[[2]int{si, di}] = r
	}

	var out []string
	out = append(out, fmt.Sprintf("ADJACENCY MATRIX (DSM) — %d nodes · %d edges  (rows=source, cols=target; off-diagonal symmetry = feedback)", n, len(g.Edges)))
	// legend: index -> id
	for i, id := range nodes {
		out = append(out, fmt.Sprintf("  %2d %s", i, id))
	}
	// column-index header (units digit; tens shown in the legend indices)
	var hdr strings.Builder
	hdr.WriteString("     ") // row-label gutter
	for i := 0; i < n; i++ {
		hdr.WriteByte(byte('0' + i%10))
	}
	out = append(out, hdr.String())
	// rows
	for i := 0; i < n; i++ {
		var row strings.Builder
		row.WriteString(fmt.Sprintf("  %2d ", i))
		for j := 0; j < n; j++ {
			if i == j {
				row.WriteRune('╲')
				continue
			}
			if r, ok := mark[[2]int{i, j}]; ok {
				row.WriteRune(r)
			} else {
				row.WriteByte(' ')
			}
		}
		out = append(out, row.String())
	}
	return out
}
