package grammar

import (
	"strings"
)

const columnRailLaneW = 24

// RenderColumnRail lays the :dynamics graph out as layer lanes (left to right in seed order),
// stacked nodes, and ASCII rails. It is deterministic by construction: it preserves the seed order
// of Layers, Nodes, and Edges and never sorts.
func RenderColumnRail(g Graph, maxRes int, airOn bool, w int) string {
	g = g.AtResolution(maxRes)
	if len(g.Nodes) == 0 || len(g.Layers) == 0 {
		return "  (no map)"
	}

	laneOf := make(map[string]int, len(g.Layers))
	for i, layer := range g.Layers {
		laneOf[layer.ID] = i
	}

	rowOf := make(map[string]int, len(g.Nodes))
	colOf := make(map[string]int, len(g.Nodes))
	nextRow := make(map[int]int, len(g.Layers))
	laidNodes := 0
	for _, n := range g.Nodes {
		lane, ok := laneOf[n.Layer]
		if !ok {
			continue
		}
		rowOf[n.ID] = nextRow[lane]
		colOf[n.ID] = lane
		nextRow[lane] += 2 // leave a rail row between node rows
		laidNodes++
	}
	if laidNodes == 0 {
		return "  (no map)"
	}

	maxRow := 0
	for _, r := range nextRow {
		if r > maxRow {
			maxRow = r
		}
	}
	H := maxRow + 1
	Wc := len(g.Layers) * columnRailLaneW
	grid, cgrid := newColumnRailGrid(H, Wc)

	put := func(r, c int, ch rune, color string) {
		if r < 0 || r >= H || c < 0 || c >= Wc {
			return
		}
		grid[r][c] = ch
		cgrid[r][c] = color
	}
	xOf := func(lane int) int { return lane * columnRailLaneW }

	// Place nodes: provenance glyph plus AIR-aware id/label text. The label redacts under AIR, but
	// the node's position remains stable so topology does not move.
	for _, n := range g.Nodes {
		r, ok := rowOf[n.ID]
		if !ok {
			continue
		}
		x := xOf(colOf[n.ID])
		pcol := columnRailProvToken(n.Status)
		pg := firstRune(statusGlyph(n.Status, n.AIR, airOn), '·')
		put(r, x, pg, pcol)

		id := redact(n.AIR, "id", n.ID, airOn)
		label := redact(n.AIR, "label", n.Label, airOn)
		text := id
		if strings.TrimSpace(label) != "" {
			text += " " + label
		}
		for i, ch := range []rune(pad(text, columnRailLaneW-4)) {
			put(r, x+2+i, ch, "pri")
		}
	}

	// Draw edges after nodes per the spec pseudocode. Edges to nodes filtered by resolution (or to
	// nodes in unknown layers) are ignored, preserving AtResolution's topology contract.
	for _, e := range g.Edges {
		sr, ok1 := rowOf[e.Source]
		tr, ok2 := rowOf[e.Target]
		src, ok3 := colOf[e.Source]
		tc, ok4 := colOf[e.Target]
		if !ok1 || !ok2 || !ok3 || !ok4 {
			continue
		}
		ecol := columnRailEdgeToken(e.Status)
		sx := xOf(src) + columnRailLaneW - 2
		if src == tc {
			a, b := minI(sr, tr), maxI(sr, tr)
			for rr := a + 1; rr < b; rr++ {
				mergeColumnRail(grid, cgrid, rr, sx, '│', ecol)
			}
			put(tr, sx, '▶', ecol)
			continue
		}

		tx := xOf(tc)
		dropX := tx - 1 // arrowhead just left of the target node glyph for forward lane flow
		if dropX < 0 {
			dropX = tx
		}
		a, b := minI(sx, dropX), maxI(sx, dropX)
		for cc := a + 1; cc < b; cc++ {
			mergeColumnRail(grid, cgrid, sr, cc, '─', ecol)
		}
		mergeColumnRail(grid, cgrid, sr, sx, '├', ecol)

		ra, rb := minI(sr, tr), maxI(sr, tr)
		if tr != sr {
			mergeColumnRail(grid, cgrid, sr, dropX, '┬', ecol)
			for rr := ra + 1; rr < rb; rr++ {
				mergeColumnRail(grid, cgrid, rr, dropX, '│', ecol)
			}
		}
		put(tr, dropX, '▶', ecol)
	}

	var b strings.Builder
	header := columnRailHeader(g.Layers)
	b.WriteString(C("mut", clipRunes(header, w)))
	b.WriteByte('\n')
	for r := 0; r < H; r++ {
		b.WriteString(emitColumnRailRow(grid[r], cgrid[r], w))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func newColumnRailGrid(h, w int) ([][]rune, [][]string) {
	grid := make([][]rune, h)
	cgrid := make([][]string, h)
	for r := range grid {
		grid[r] = make([]rune, w)
		cgrid[r] = make([]string, w)
		for c := range grid[r] {
			grid[r][c] = ' '
		}
	}
	return grid, cgrid
}

func columnRailHeader(layers []Layer) string {
	var b strings.Builder
	for _, layer := range layers {
		b.WriteString(pad(strings.ToUpper(layer.Label), columnRailLaneW))
	}
	return b.String()
}

func columnRailProvToken(status string) string {
	switch status {
	case "asserted", "observed":
		return "eme"
	case "simulated", "candidate":
		return "blu"
	default:
		return "mut"
	}
}

func columnRailEdgeToken(status string) string {
	return columnRailProvToken(status)
}

func firstRune(s string, fallback rune) rune {
	for _, r := range s {
		return r
	}
	return fallback
}

func minI(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxI(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func mergeColumnRail(g [][]rune, cg [][]string, r, c int, ch rune, color string) {
	if r < 0 || r >= len(g) || c < 0 || c >= len(g[r]) {
		return
	}
	g[r][c] = columnRailJunction(g[r][c], ch)
	if cg[r][c] == "" {
		cg[r][c] = color
	}
}

func columnRailJunction(a, b rune) rune {
	if a == ' ' {
		return b
	}
	if b == ' ' || a == b {
		return a
	}
	if a == '▶' || b == '▶' {
		return '▶'
	}
	am, aok := columnRailMask(a)
	bm, bok := columnRailMask(b)
	if !aok || !bok {
		return b
	}
	return columnRailRune(am | bm)
}

const (
	railUp = 1 << iota
	railDown
	railLeft
	railRight
)

func columnRailMask(ch rune) (int, bool) {
	switch ch {
	case '│':
		return railUp | railDown, true
	case '─':
		return railLeft | railRight, true
	case '┌':
		return railDown | railRight, true
	case '┐':
		return railDown | railLeft, true
	case '└':
		return railUp | railRight, true
	case '┘':
		return railUp | railLeft, true
	case '├':
		return railUp | railDown | railRight, true
	case '┤':
		return railUp | railDown | railLeft, true
	case '┬':
		return railLeft | railRight | railDown, true
	case '┴':
		return railLeft | railRight | railUp, true
	case '┼':
		return railUp | railDown | railLeft | railRight, true
	}
	return 0, false
}

func columnRailRune(mask int) rune {
	switch mask {
	case railUp | railDown:
		return '│'
	case railLeft | railRight:
		return '─'
	case railDown | railRight:
		return '┌'
	case railDown | railLeft:
		return '┐'
	case railUp | railRight:
		return '└'
	case railUp | railLeft:
		return '┘'
	case railUp | railDown | railRight:
		return '├'
	case railUp | railDown | railLeft:
		return '┤'
	case railLeft | railRight | railDown:
		return '┬'
	case railLeft | railRight | railUp:
		return '┴'
	case railUp | railDown | railLeft | railRight:
		return '┼'
	}
	if mask&(railUp|railDown) != 0 {
		return '│'
	}
	return '─'
}

func emitColumnRailRow(row []rune, colors []string, w int) string {
	limit := len(row)
	if w > 0 && w < limit {
		limit = w
	}
	var b strings.Builder
	start := 0
	token := ""
	if limit > 0 {
		token = colors[0]
	}
	for i := 1; i <= limit; i++ {
		if i == limit || colors[i] != token {
			span := string(row[start:i])
			if token != "" {
				span = C(token, span)
			}
			b.WriteString(span)
			start = i
			if i < limit {
				token = colors[i]
			}
		}
	}
	return b.String()
}

func clipRunes(s string, w int) string {
	if w <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	return string(r[:w])
}
