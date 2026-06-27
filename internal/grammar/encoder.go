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
	ChannelText           Channel = iota // identity / action / qualifier: plain text (+ selection-shape, applied by the row)
	ChannelCriticalityHue                // posture (ordinal): a state glyph colored by criticality
	ChannelFamilyHue                     // ownership / place (categorical): the owner/locus family hue
	ChannelFreshnessDim                  // time (temporal): an eighth-block brightness ladder (recent → stale)
	ChannelMagnitudeBar                  // measure (quantitative): an eighth-block fill bar — magnitude by SHAPE, not hue
	ChannelProvenancePip                 // provenance (ordinal-confidence): the ●◉◐◍◌○ confidence ladder + AIR-class
)

// String names the channel (for the legend / --probe encode). Stable identifiers, not display prose.
func (c Channel) String() string {
	switch c {
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

// defaultFacetChannel is the built-in binding used when /read/facets is unreachable (offline probes).
// It MUST agree with parsing the registry's channel prose (TestDefaultFacetChannelAgreesWithProse).
var defaultFacetChannel = map[string]Channel{
	"identity":   ChannelText,
	"posture":    ChannelCriticalityHue,
	"action":     ChannelText,
	"ownership":  ChannelFamilyHue,
	"place":      ChannelFamilyHue,
	"time":       ChannelFreshnessDim,
	"provenance": ChannelProvenancePip,
	"measure":    ChannelMagnitudeBar,
	"qualifier":  ChannelText,
}

// ChannelFromProse parses a registry `channel` string into the bound Channel. The compound proses
// ("family-hue / view-axis", "text + selection-shape", "secondary pip + AIR-class") resolve by their
// DATA channel — the "view-axis" / "selection-shape" tails are Layer-3 (composition) concerns, not a
// Layer-2 cell channel. Order = most-specific data channel first; text is the default carrier.
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
	default:
		return ChannelText
	}
}

// ChannelForFacet resolves a facet's cell channel, preferring the live registry prose (the SSOT) and
// falling back to the built-in default table when the registry has no entry for it.
func ChannelForFacet(reg FacetRegistry, facet string) Channel {
	if f, ok := reg.Facets[facet]; ok && f.Channel != "" {
		return ChannelFromProse(f.Channel)
	}
	if c, ok := defaultFacetChannel[facet]; ok {
		return c
	}
	return ChannelText
}

// CellValue is one faceted datum to encode. Text is the universal cold-read carrier (the value/word/
// owner). Magnitude (0..1) feeds the bar/dim channels. Denied marks an attribute outside the on-air
// allowlist — redacted under airOn. Width pads/truncates the text portion (0 = leave as-is).
//
// For the glyph-ordinal channels the caller passes the CLASSIFIED level in Text: criticality-hue
// expects a criticality word (crit/major/warn/ok), provenance-pip expects a confidence word
// (asserted/observed/inferred/…). Classification is a DATA act upstream, not the encoder's job.
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

// EncodeCell binds one faceted value to its cell channel and renders it. AIR is universal: a denied
// attribute under airOn redacts in place (value never airs; width/shape kept) — the same default-deny
// the row renderers apply, one source.
func EncodeCell(reg FacetRegistry, facet string, v CellValue, airOn bool) Cell {
	ch := ChannelForFacet(reg, facet)
	return Cell{Rendered: renderChannel(ch, v, airOn), Channel: ch, Facet: facet}
}

func renderChannel(ch Channel, v CellValue, airOn bool) string {
	denied := airOn && v.Denied
	switch ch {
	case ChannelCriticalityHue:
		if denied {
			return C("mut", "▒ "+padTo(redactToken, v.Width))
		}
		g := critGlyph[v.Text]
		tok := SeverityToken(v.Text)
		if g == "" { // not a known criticality level -> neutral ground, no implied health
			g, tok = "·", "mut"
		}
		return C(tok, g+" "+padTo(v.Text, v.Width))

	case ChannelFamilyHue:
		if denied {
			return C("mut", padTo(redactToken, v.Width))
		}
		return C(LaneToken(v.Text), padTo(v.Text, v.Width))

	case ChannelFreshnessDim:
		if denied {
			return C("mut", "▒ "+padTo(redactToken, v.Width))
		}
		fg, ftok := freshGlyph(v.Magnitude)
		s := fg
		if strings.TrimSpace(v.Text) != "" {
			s += " " + padTo(v.Text, v.Width)
		}
		return C(ftok, s)

	case ChannelMagnitudeBar:
		if denied {
			return C("mut", redactToken+"▒")
		}
		// magnitude rides SHAPE (the fill); mut hue — NOT criticality (the traces-legend rule).
		s := ScoreBar(v.Magnitude)
		if strings.TrimSpace(v.Text) != "" {
			s += " " + padTo(v.Text, v.Width)
		}
		return C("mut", s)

	case ChannelProvenancePip:
		if denied {
			return C("mut", "▒ "+padTo(redactToken, v.Width))
		}
		g := statusGlyphs[v.Text]
		if g == "" {
			g = "·"
		}
		return C(palette.ProvToken(v.Text), g+" "+padTo(v.Text, v.Width))

	default: // ChannelText (identity / action / qualifier)
		if denied {
			return C("mut", padTo(redactToken, v.Width))
		}
		return C("pri", padTo(v.Text, v.Width))
	}
}
