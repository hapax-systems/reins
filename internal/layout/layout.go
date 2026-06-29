// Package layout is the Reins view-algebra: ONE composition tree (the Spec) rendered by ONE pure
// fold (Render), dissolving the old two-codebase split (bespoke global split + bespoke coordinator
// HConcat) into a single recursive interpreter.
//
// Operator ruling (2026-06-27, the split/non-split brief landing):
//   - SPLIT AS DEFAULT, and ONLY split. There is no single-pane layout — fewer layout contexts,
//     so expectations and consistency hold. Render never collapses a split to one pane.
//   - NO split-pair registry. "Any split is an implicit pair": the act of splitting IS the pairing;
//     panes are composed dynamically, not drawn from a hand-authored table.
//   - The relationship between two panes is EMERGENT, derived by the information-processing systems
//     (relation-derivation / salience / semantic recruitment / the cell-grammar joins) — NOT an
//     authored Contract string. This package stays pure: the model computes the relation and passes
//     it in as Connector.Relation; Render only renders it.
package layout

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Pane is a leaf: content for a (w,h) budget. MinW is advisory (the smallest width at which the
// pane reads well) — under the only-split rule it does NOT cause a collapse; the pane self-degrades.
type Pane struct {
	Render func(w, h int) string
	MinW   int
}

// Spec is a composition-tree node. Exactly one of Leaf / Split is non-nil. The cockpit root is
// always a Split (only-split); a Leaf is only ever the content of a split pane.
type Spec struct {
	Leaf  *Pane
	Split *HSplit
}

// HSplit is a horizontal split: Primary │ Secondary, joined by a connector column. Ratio is the
// fraction of the inner width (after the 1-col connector) given to Primary.
type HSplit struct {
	Primary, Secondary *Spec
	Ratio              float64
	Connector          Connector
}

// Connector is the divider column + the always-on relation header. Relation is the EMERGENT
// relationship between the two panes (derived by the info-processing systems, never authored);
// empty renders no header. Verdict/JoinKey carry the typed-join gate's ruling (relate.Gate /
// JoinKeyFor, computed by the model): the header states whether the split honestly carries a join.
// A "Door" verdict with an empty JoinKey is the honesty floor — the header declares "no join" and
// must never imply a coordination the data does not carry. Both empty = bare-relation header.
type Connector struct {
	Glyph    string
	Relation string
	Verdict  string
	JoinKey  string
	Coupling string
}

// Leaf wraps a pane as a spec node.
func Leaf(p *Pane) *Spec { return &Spec{Leaf: p} }

// Split composes two specs side-by-side. ratio is Primary's share of the inner width.
func Split(primary, secondary *Spec, ratio float64, c Connector) *Spec {
	return &Spec{Split: &HSplit{Primary: primary, Secondary: secondary, Ratio: ratio, Connector: c}}
}

// Render is the pure-fold interpreter: same (spec, w, h) → same string, exactly w cols × h lines.
// Splits ALWAYS render both panes (only-split); panes self-degrade their content when narrow. The
// sole exception is the physical floor w<3 (no room for two panes + a connector), where the
// primary renders alone — unreachable on any real terminal.
func Render(s *Spec, w, h int) string {
	return strings.Join(render(s, w, h), "\n")
}

func render(s *Spec, w, h int) []string {
	if s == nil || w <= 0 || h <= 0 {
		return nil
	}
	if s.Leaf != nil {
		return fit(s.Leaf.Render(w, h), w, h)
	}
	if s.Split == nil { // a zero-value Spec — render empty, never panic
		return fit("", w, h)
	}
	sp := s.Split
	if w < 3 { // degenerate physical floor — cannot place two panes + a connector
		return render(sp.Primary, w, h)
	}
	inner := w - 1 // the connector owns one column
	pw := clamp(int(float64(inner)*clampRatio(sp.Ratio)), 1, inner-1)
	sw := inner - pw

	ch := h
	var out []string
	if sp.Connector.Relation != "" && h > 1 {
		out = append(out, relationHeader(sp.Connector, w))
		ch = h - 1
	}
	left := render(sp.Primary, pw, ch)
	right := render(sp.Secondary, sw, ch)
	glyph := sp.Connector.Glyph
	if glyph == "" {
		glyph = "│"
	}
	glyph = fitLine(glyph, 1) // normalize to exactly one display cell (the width contract assumes it)
	for i := 0; i < ch; i++ {
		// fit BOTH cells to their column: a nil/short child (e.g. a malformed Spec) can't shrink the row.
		out = append(out, fitLine(at(left, i), pw)+glyph+fitLine(at(right, i), sw))
	}
	return out
}

// relationHeader is the connector's emergent-relation row: the relation centered between ─ rules,
// suffixed with the typed-join clause so the split states its join honestly. A real-join verdict
// asserts its key (⋈ <key>); a Door declares "no join"; an unset verdict keeps the bare relation.
func relationHeader(c Connector, w int) string {
	label := " " + c.Relation + joinClause(c) + couplingClause(c) + " "
	lw := ansi.StringWidth(label)
	if lw >= w {
		return fitLine(label, w)
	}
	side := (w - lw) / 2
	return strings.Repeat("─", side) + label + strings.Repeat("─", w-lw-side)
}

// couplingClause renders the verdict-tier coupling word inside the centered relation label.
func couplingClause(c Connector) string {
	if c.Coupling == "" {
		return ""
	}
	return " · " + c.Coupling
}

// joinClause renders the gate's ruling: nothing when the verdict is unset (bare relation), "· no join"
// for a Door (the honesty floor — no coordination asserted), else "· ⋈ <key>" naming the asserted join.
func joinClause(c Connector) string {
	switch {
	case c.Verdict == "":
		return ""
	case c.JoinKey == "": // Door / any keyless verdict
		return " · no join"
	default:
		return " · ⋈ " + c.JoinKey
	}
}

// fit forces a block to exactly h lines, each exactly w columns (ANSI-width-aware).
func fit(block string, w, h int) []string {
	src := strings.Split(block, "\n")
	out := make([]string, h)
	for i := 0; i < h; i++ {
		if i < len(src) {
			out[i] = fitLine(src[i], w)
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return out
}

func fitLine(s string, w int) string {
	s = ansi.Truncate(s, w, "")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

func at(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampRatio(r float64) float64 {
	if !(r >= 0.1) { // catches NaN and undershoot in one test
		return 0.1
	}
	if r > 0.9 {
		return 0.9
	}
	return r
}
