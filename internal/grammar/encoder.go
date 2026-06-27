package grammar

import (
	"strings"

	"github.com/hapax-systems/reins/internal/palette"
)

// The cell-grammar ENCODER (framework §1 Layer-2 — the representational-framework's encoding layer).
//
// The info-vis canon insists DATA ≠ ENCODING: a field's identity is separate from how it maps to a
// visual channel. Reins today fuses them — one hand-rolled RenderXxxRow per kind (a chart typology,
// not a grammar). This file is the formal "Bertin-for-monospace" binding: every faceted attribute
// binds to EXACTLY ONE cell channel, by role, ONCE, for every pane. It generalizes the RenderTaskRow
// seven-dimension lifecycle strip into a channel-typed encoder the row renderers can call.
//
// The binding is DRIVEN by the registry SSOT (api/facet_registry.py, served at /read/facets): each
// facet carries a `channel` prose string and the encoder reads its Channel from that SAME prose the
// legend shows — one source, no drift (A6: the decoder travels with the artifact). Color stays a
// REDUNDANT amplifier: every channel also carries meaning in glyph/shape/position, so a cell's full
// meaning survives grayscale + a cropped header + a freeze-frame (Gate-13).

// Channel is a cell's visual channel — the scarce monospace resource. Bertin's expressiveness +
// effectiveness rule, made explicit so the channel budget is visible (monospace has FEW high-
// cardinality channels). Each attribute role binds to one and only one of these.
type Channel int

const (
	ChannelUnknown        Channel = iota - 1 // -1: an unrecognized channel prose (SSOT drift) — never rendered; callers fall back
	ChannelText                              // identity / action / variant: plain text (+ selection-shape, applied by the row)
	ChannelCriticalityHue                    // posture (ordinal): a state glyph colored by criticality
	ChannelFamilyHue                         // ownership / place (categorical): the owner/locus family hue
	ChannelFreshnessDim                      // time (temporal): an eighth-block brightness ladder (recent → stale)
	ChannelMagnitudeBar                      // measure (quantitative): an eighth-block fill bar — magnitude by SHAPE, not hue
	ChannelProvenancePip                     // provenance (ordinal-confidence): the ●◉◐◍◌○ confidence ladder + AIR-class
)

// String names the channel (for the legend / --probe encode). Stable identifiers, not display prose.
func (c Channel) String() string {
	switch c {
	case ChannelUnknown:
		return "unknown"
	case ChannelText:
		return "text"
	case ChannelCriticalityHue:
		return "criticality-hue"
	case ChannelFamilyHue:
		return "family-hue"
	case ChannelFreshnessDim:
		return "freshness-dim"
	case ChannelMagnitudeBar:
		return "magnitude-bar"
	case ChannelProvenancePip:
		return "provenance-pip"
	}
	return "?"
}

// defaultFacetChannel is the NAME-keyed built-in binding (stable across channel re-wording), used (a)
// when /read/facets is unreachable — offline probes — and (b) as the fallback when the live registry
// prose is unrecognized. The encoder binds on the `channel` prose (the SSOT driver), NOT on `role`
// (role is the Bertin justification — and one role, e.g. categorical, legitimately maps to several
// channels, so a strict role↔channel map does not exist). Two guards pin this table to the registry:
// TestDefaultFacetChannelAgreesWithProse (parser ↔ default, Go-side) and
// TestChannelBindingMatchesPythonRegistry (the real cross-language pin against api/facet_registry.py).
var defaultFacetChannel = map[string]Channel{
	"identity":   ChannelText,
	"posture":    ChannelCriticalityHue,
	"action":     ChannelText,
	"ownership":  ChannelFamilyHue,
	"place":      ChannelFamilyHue,
	"time":       ChannelFreshnessDim,
	"provenance": ChannelProvenancePip,
	"measure":    ChannelMagnitudeBar,
	"variant":    ChannelText,
}

// ChannelFromProse parses a registry `channel` string into the bound Channel. The compound proses
// ("family-hue / view-axis", "text + selection-shape", "secondary pip + AIR-class") resolve by their
// DATA channel — the "view-axis" / "selection-shape" tails are Layer-3 (composition) concerns, not a
// Layer-2 cell channel. Order = most-specific data channel first, then the explicit text carrier.
// An UNRECOGNIZED prose returns ChannelUnknown (an explicit drift sentinel) rather than silently
// masquerading as text — so a future re-wording of the registry surfaces loudly instead of dropping a
// meaning channel on air (Gate-13). ChannelForFacet turns that sentinel into a safe name-keyed fallback.
func ChannelFromProse(prose string) Channel {
	p := strings.ToLower(prose)
	switch {
	case strings.Contains(p, "criticality-hue"):
		return ChannelCriticalityHue
	case strings.Contains(p, "family-hue"):
		return ChannelFamilyHue
	case strings.Contains(p, "block bar"), strings.Contains(p, "eighth-block"):
		return ChannelMagnitudeBar
	case strings.Contains(p, "freshness"):
		return ChannelFreshnessDim
	case strings.Contains(p, "pip"):
		return ChannelProvenancePip
	case strings.Contains(p, "text"):
		return ChannelText
	default:
		return ChannelUnknown
	}
}

// ChannelForFacet resolves a facet's cell channel, preferring the live registry prose (the SSOT). If
// the prose is unrecognized (re-worded → ChannelUnknown) or the registry has no entry, it falls back
// to the NAME-keyed default table — which is stable across channel re-wording — so the live path is
// robust to drift and never silently downgrades a meaning channel to plain text.
func ChannelForFacet(reg FacetRegistry, facet string) Channel {
	if f, ok := reg.Facets[facet]; ok && f.Channel != "" {
		if ch := ChannelFromProse(f.Channel); ch != ChannelUnknown {
			return ch
		}
	}
	if c, ok := defaultFacetChannel[facet]; ok {
		return c
	}
	return ChannelText
}

// CellValue is one faceted datum to encode. Text is the universal cold-read carrier (the value/word/
// owner). Magnitude (0..1) feeds the bar/dim channels. Denied marks an attribute outside the on-air
// allowlist — redacted under airOn. Width pads/truncates the text portion (0 = leave as-is; >0 holds
// a stable column across visible/empty/denied states so the grid never jitters on a freeze-frame).
//
// For the glyph-ordinal channels the caller passes the CLASSIFIED level in Text: criticality-hue
// expects a criticality word (crit/major/warn/ok), provenance-pip expects a confidence word
// (asserted/observed/inferred/…). Classification is a DATA act upstream, not the encoder's job.
//
// For magnitude-bar/freshness-dim, supply a numeric Text label: a true zero magnitude renders an empty
// bar, so at Width==0 with no label the cell is a blank — the label keeps a zero legible cold (Gate-13).
type CellValue struct {
	Text      string
	Magnitude float64
	Denied    bool
	Width     int
}

// Cell is one encoded cell: the rendered string plus the channel + facet it bound to, so a composer
// can reason about the (scarce) channel budget and collisions.
type Cell struct {
	Rendered string
	Channel  Channel
	Facet    string
}

const redactToken = "▒▒▒"

// padTo pads/truncates to n, but leaves the string untouched when n <= 0 (unlike the column-fixed pad).
func padTo(s string, n int) string {
	if n <= 0 {
		return s
	}
	return pad(s, n)
}

// padSilent pads to n, but an EMPTY value becomes structured-silence dots — the grid reads "nothing
// here" rather than a jarring blank (the dotsOr convention, made a property of the text/family cells).
func padSilent(s string, n int) string {
	if strings.TrimSpace(s) == "" && n > 0 {
		return strings.Repeat("·", n)
	}
	return padTo(s, n)
}

// glyphCell renders a glyph-led cell: a leading mark + a space + the value, used by every glyph-bearing
// channel (criticality / freshness / magnitude / provenance, and their redaction). When width>0 the
// value slot is ALWAYS padded — so the column holds one stable width across visible / empty-label /
// denied states (the freeze-frame "grid never jitters" rule). When width==0 the cell is free-form.
func glyphCell(token, glyph, text string, width int) string {
	if width > 0 {
		return C(token, glyph+" "+pad(text, width))
	}
	if strings.TrimSpace(text) != "" {
		return C(token, glyph+" "+text)
	}
	return C(token, glyph)
}

// EncodeCell binds one faceted value to its cell channel and renders it. AIR is universal: a denied
// attribute under airOn redacts in place (value never airs; width/shape kept) — the same default-deny
// the row renderers apply, one source.
func EncodeCell(reg FacetRegistry, facet string, v CellValue, airOn bool) Cell {
	ch := ChannelForFacet(reg, facet)
	return Cell{Rendered: renderChannel(ch, v, airOn), Channel: ch, Facet: facet}
}

// FacetCell pairs a facet with its value for row composition.
type FacetCell struct {
	Facet string
	Value CellValue
}

// RenderFacetRow composes a row from faceted cells via the encoder — the generalization of the per-
// kind RenderXxxRow strips (RenderTaskRow's 7-dim lifecycle sentence, etc.) to ANY faceted entity
// (framework §1 Layer-2: "every pane renders the same way"). Cells encode in the given order and join
// by a single space (the cell gutter); column widths come from each CellValue.Width. AIR is per-cell.
// Additive — a new render PATH, wired into no live pane (the row-renderer swap stays operator-vetted).
func RenderFacetRow(reg FacetRegistry, cells []FacetCell, airOn bool) string {
	parts := make([]string, 0, len(cells))
	for _, c := range cells {
		parts = append(parts, EncodeCell(reg, c.Facet, c.Value, airOn).Rendered)
	}
	return strings.Join(parts, " ")
}

func renderChannel(ch Channel, v CellValue, airOn bool) string {
	denied := airOn && v.Denied
	switch ch {
	case ChannelCriticalityHue:
		if denied {
			return glyphCell("mut", "▒", redactToken, v.Width)
		}
		g, tok := critGlyph[v.Text], SeverityToken(v.Text)
		if g == "" { // not a known criticality level -> neutral ground, no implied health
			g, tok = "·", "mut"
		}
		return glyphCell(tok, g, v.Text, v.Width)

	case ChannelFamilyHue:
		if denied {
			return C("mut", padTo(redactToken, v.Width))
		}
		if strings.TrimSpace(v.Text) == "" { // empty owner/place -> structured silence, not a colored blank
			return C("mut", padSilent(v.Text, v.Width))
		}
		return C(LaneToken(v.Text), padTo(v.Text, v.Width))

	case ChannelFreshnessDim:
		if denied {
			return glyphCell("mut", "▒", redactToken, v.Width)
		}
		fg, ftok := freshGlyph(v.Magnitude)
		return glyphCell(ftok, fg, v.Text, v.Width)

	case ChannelMagnitudeBar:
		if denied {
			return glyphCell("mut", "▒", redactToken, v.Width)
		}
		// magnitude rides SHAPE (the fill); mut hue — NOT criticality (the traces-legend rule).
		return glyphCell("mut", ScoreBar(v.Magnitude), v.Text, v.Width)

	case ChannelProvenancePip:
		if denied {
			return glyphCell("mut", "▒", redactToken, v.Width)
		}
		g := statusGlyphs[v.Text]
		if g == "" {
			g = "·"
		}
		return glyphCell(palette.ProvToken(v.Text), g, v.Text, v.Width)

	default: // ChannelText (identity / action / variant) + the ChannelUnknown safety fallback
		if denied {
			return C("mut", padTo(redactToken, v.Width))
		}
		if strings.TrimSpace(v.Text) == "" {
			return C("mut", padSilent(v.Text, v.Width)) // structured silence, dimmed
		}
		return C("pri", padTo(v.Text, v.Width))
	}
}
