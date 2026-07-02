package grammar

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// OverflowMark is the single-cell right-edge glyph that discloses a horizontally TRUNCATED row (matches
// layout.OverflowMark — kept in sync; grammar cannot import layout without a cycle). A glyph, not a
// color, carries the signal.
const OverflowMark = "›"

const columnRailLaneW = 28
const columnRailMaxFlowMarks = 6

type columnRailFlowPath struct {
	coords [][2]int
	color  string
}

type ColumnRailFocus struct {
	Kind, ID, Source, Target, Relation string
}

// RenderColumnRail lays the :dynamics graph out as layer lanes (left to right in seed order),
// stacked nodes, and ASCII rails. It is deterministic by construction: it preserves the seed order
// of Layers, Nodes, and Edges and never sorts.
func RenderColumnRail(g Graph, maxRes int, airOn bool, w int) string {
	return renderColumnRail(g, maxRes, airOn, w, -1, ColumnRailFocus{})
}

// RenderColumnRailFrame preserves the deterministic topology but lets a single flow mark move along
// each rendered edge. Phase affects only rail glyphs; nodes, labels, ordering, and AIR redaction stay
// stable.
func RenderColumnRailFrame(g Graph, maxRes int, airOn bool, w int, phase int) string {
	return renderColumnRail(g, maxRes, airOn, w, phase, ColumnRailFocus{})
}

func RenderColumnRailFrameFocused(g Graph, maxRes int, airOn bool, w int, phase int, focus ColumnRailFocus) string {
	return renderColumnRail(g, maxRes, airOn, w, phase, focus)
}

func renderColumnRail(g Graph, maxRes int, airOn bool, w int, phase int, focus ColumnRailFocus) string {
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
	rightRailX := func(lane int) int { return xOf(lane) + columnRailLaneW - 2 }
	targetRailX := func(srcLane, targetLane int) int {
		if srcLane < targetLane {
			x := xOf(targetLane) - 1
			if x >= 0 {
				return x
			}
		}
		if srcLane > targetLane {
			return rightRailX(targetLane)
		}
		return rightRailX(targetLane)
	}
	edgeRailRow := func(sr, tr int) int {
		if sr == tr {
			if sr+1 < H {
				return sr + 1
			}
			if sr > 0 {
				return sr - 1
			}
			return sr
		}
		r := minI(sr, tr) + 1
		if r >= H {
			return maxI(0, H-1)
		}
		return r
	}

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
		if columnRailFocusesNode(focus, n) {
			put(r, x+1, '▶', "brt")
		}

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
	flowPaths := make([]columnRailFlowPath, 0, len(g.Edges))
	var selectedEdgePath [][2]int
	for _, e := range g.Edges {
		sr, ok1 := rowOf[e.Source]
		tr, ok2 := rowOf[e.Target]
		src, ok3 := colOf[e.Source]
		tc, ok4 := colOf[e.Target]
		if !ok1 || !ok2 || !ok3 || !ok4 {
			continue
		}
		ecol := columnRailEdgeToken(e.Status)
		sx := rightRailX(src)
		path := make([][2]int, 0, 8)
		if src == tc {
			a, b := minI(sr, tr), maxI(sr, tr)
			for rr := a + 1; rr < b; rr++ {
				mergeColumnRail(grid, cgrid, rr, sx, '│', ecol)
				path = append(path, [2]int{rr, sx})
			}
			put(tr, sx, columnRailEdgeHead(src, tc, sr, tr), ecol)
			if columnRailFocusesEdge(focus, e) {
				selectedEdgePath = append([][2]int{}, path...)
				selectedEdgePath = append(selectedEdgePath, [2]int{tr, sx})
			}
			flowPaths = appendColumnRailFlowPath(flowPaths, path, ecol)
			continue
		}

		dropX := targetRailX(src, tc)
		railR := edgeRailRow(sr, tr)
		ra, rb := minI(sr, railR), maxI(sr, railR)
		for rr := ra + 1; rr < rb; rr++ {
			mergeColumnRail(grid, cgrid, rr, sx, '│', ecol)
			path = append(path, [2]int{rr, sx})
		}
		mergeColumnRail(grid, cgrid, sr, sx, '├', ecol)
		mergeColumnRail(grid, cgrid, railR, sx, '┴', ecol)
		path = append(path, [2]int{railR, sx})

		a, b := minI(sx, dropX), maxI(sx, dropX)
		for cc := a + 1; cc < b; cc++ {
			mergeColumnRail(grid, cgrid, railR, cc, '─', ecol)
			path = append(path, [2]int{railR, cc})
		}

		mergeColumnRail(grid, cgrid, railR, dropX, '┬', ecol)
		path = append(path, [2]int{railR, dropX})
		ra, rb = minI(railR, tr), maxI(railR, tr)
		for rr := ra + 1; rr < rb; rr++ {
			mergeColumnRail(grid, cgrid, rr, dropX, '│', ecol)
			path = append(path, [2]int{rr, dropX})
		}
		put(tr, dropX, columnRailEdgeHead(src, tc, sr, tr), ecol)
		if columnRailFocusesEdge(focus, e) {
			selectedEdgePath = append([][2]int{}, path...)
			selectedEdgePath = append(selectedEdgePath, [2]int{tr, dropX})
		}
		flowPaths = appendColumnRailFlowPath(flowPaths, path, ecol)
	}
	putColumnRailFlowFrames(grid, cgrid, flowPaths, phase)
	putColumnRailSelectedEdge(grid, cgrid, selectedEdgePath)

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

func columnRailFocusesNode(focus ColumnRailFocus, n Node) bool {
	return focus.Kind == "node" && strings.TrimSpace(focus.ID) != "" && focus.ID == n.ID
}

func columnRailFocusesEdge(focus ColumnRailFocus, e Edge) bool {
	if focus.Kind != "edge" {
		return false
	}
	if strings.TrimSpace(focus.ID) != "" && focus.ID == e.ID {
		return true
	}
	if strings.TrimSpace(focus.Source) == "" || strings.TrimSpace(focus.Target) == "" {
		return false
	}
	if focus.Source != e.Source || focus.Target != e.Target {
		return false
	}
	return strings.TrimSpace(focus.Relation) == "" || focus.Relation == e.Relation
}

func putColumnRailSelectedEdge(g [][]rune, cg [][]string, path [][2]int) {
	if len(path) == 0 {
		return
	}
	rc := path[len(path)/2]
	r, c := rc[0], rc[1]
	if r < 0 || r >= len(g) || c < 0 || c >= len(g[r]) {
		return
	}
	g[r][c] = '◆'
	cg[r][c] = "brt"
}

func appendColumnRailFlowPath(paths []columnRailFlowPath, coords [][2]int, color string) []columnRailFlowPath {
	if len(coords) == 0 {
		return paths
	}
	return append(paths, columnRailFlowPath{coords: coords, color: color})
}

func putColumnRailFlowFrames(g [][]rune, cg [][]string, paths []columnRailFlowPath, phase int) {
	if phase < 0 || len(paths) == 0 {
		return
	}
	marks := len(paths)
	if marks > columnRailMaxFlowMarks {
		marks = columnRailMaxFlowMarks
	}
	for i := 0; i < marks; i++ {
		pathIdx := i
		if len(paths) > marks {
			pathIdx = (phase + i*(len(paths)/marks)) % len(paths)
		}
		path := paths[pathIdx]
		if len(path.coords) == 0 {
			continue
		}
		coordIdx := (phase + i) % len(path.coords)
		if coordIdx < 0 {
			coordIdx += len(path.coords)
		}
		rc := path.coords[coordIdx]
		r, c := rc[0], rc[1]
		if r < 0 || r >= len(g) || c < 0 || c >= len(g[r]) {
			continue
		}
		g[r][c] = '•'
		cg[r][c] = path.color
	}
}

func columnRailEdgeHead(srcLane, targetLane, sourceRow, targetRow int) rune {
	switch {
	case srcLane < targetLane:
		return '→'
	case srcLane > targetLane:
		return '←'
	case targetRow < sourceRow:
		return '↑'
	default:
		return '↓'
	}
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
		b.WriteString(pad(strings.ToUpper(layer.Label), columnRailLaneW-1))
		b.WriteRune(' ')
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

// clipRunes clips a (possibly COLORIZED) string to w VISIBLE columns, disclosing truncation with the
// overflow marker. It is ansi-aware: it measures visible width (not raw rune count, which counts escape
// bytes) and truncates without cutting mid-escape-sequence — several call sites (RenderAxisRow,
// RenderIdentityRow, RenderConsentFacetRow) pass already-colorized rows. A dropped tail is NEVER silent.
func clipRunes(s string, w int) string {
	if w <= 0 {
		return s
	}
	if ansi.StringWidth(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, OverflowMark)
}
