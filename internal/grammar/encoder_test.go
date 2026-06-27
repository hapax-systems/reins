package grammar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// canonicalFacetProse mirrors the 9 facet `channel` strings served by /read/facets
// (api/facet_registry.py FACETS). The encoder's binding is DRIVEN by this prose (the SSOT),
// so these tests pin the parser + the default table against the registry's exact wording.
var canonicalFacetProse = map[string]string{
	"identity":   "text + selection-shape",
	"posture":    "criticality-hue",
	"action":     "text / view-axis",
	"ownership":  "ownership family-hue",
	"place":      "family-hue / view-axis",
	"time":       "freshness-dim",
	"provenance": "secondary pip + AIR-class",
	"measure":    "eighth-block bar",
	"qualifier":  "text / view-axis",
}

var canonicalFacetChannel = map[string]Channel{
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

// ChannelFromProse must resolve every one of the registry's 9 channel-prose strings to the
// intended Channel — the encoder reads its binding from the SAME prose the legend shows, so it
// cannot drift from the SSOT. (The compound proses — "family-hue / view-axis", "text + selection-
// shape", "secondary pip + AIR-class" — must resolve by the data channel, not the view-axis tail.)
func TestChannelFromProseResolvesEveryFacet(t *testing.T) {
	for facet, prose := range canonicalFacetProse {
		got := ChannelFromProse(prose)
		want := canonicalFacetChannel[facet]
		if got != want {
			t.Errorf("facet %q prose %q -> %v, want %v", facet, prose, got, want)
		}
	}
}

// The built-in default table (used when /read/facets is unreachable — offline probes) must AGREE
// with parsing the canonical prose. One source of truth, two consistent readers.
func TestDefaultFacetChannelAgreesWithProse(t *testing.T) {
	for facet, prose := range canonicalFacetProse {
		viaProse := ChannelFromProse(prose)
		viaDefault := ChannelForFacet(FacetRegistry{}, facet) // empty registry -> built-in default
		if viaProse != viaDefault {
			t.Errorf("facet %q: prose->%v but default->%v (drift)", facet, viaProse, viaDefault)
		}
	}
}

// ChannelForFacet prefers the live registry prose over the built-in default (the registry is SSOT).
func TestChannelForFacetPrefersRegistryProse(t *testing.T) {
	reg := FacetRegistry{Facets: map[string]FacetDef{
		"posture": {Channel: "eighth-block bar"}, // a (hypothetical) re-binding in the live registry
	}}
	if got := ChannelForFacet(reg, "posture"); got != ChannelMagnitudeBar {
		t.Fatalf("registry prose must win: got %v, want %v", got, ChannelMagnitudeBar)
	}
}

// identity/action/qualifier -> plain text in the primary ink, padded to width, value preserved.
func TestEncodeCellTextCarriesValue(t *testing.T) {
	c := EncodeCell(FacetRegistry{}, "identity", CellValue{Text: "task-4284", Width: 12}, false)
	if c.Channel != ChannelText {
		t.Fatalf("identity must bind to text channel, got %v", c.Channel)
	}
	if !strings.Contains(ansi.Strip(c.Rendered), "task-4284") {
		t.Fatalf("text cell dropped its value: %q", ansi.Strip(c.Rendered))
	}
}

// posture -> criticality-hue: the cell carries the criticality GLYPH (grayscale-safe) AND the word,
// so meaning survives hue removal (Gate-13 grayscale tooth). Hue is the redundant amplifier.
func TestEncodeCellCriticalityHueIsGrayscaleSafe(t *testing.T) {
	c := EncodeCell(FacetRegistry{}, "posture", CellValue{Text: "crit", Width: 6}, false)
	if c.Channel != ChannelCriticalityHue {
		t.Fatalf("posture must bind to criticality-hue, got %v", c.Channel)
	}
	plain := ansi.Strip(c.Rendered)
	if !strings.Contains(plain, critGlyph["crit"]) {
		t.Fatalf("criticality cell must carry the state glyph in grayscale: %q", plain)
	}
	if !strings.Contains(plain, "crit") {
		t.Fatalf("criticality cell must carry the word: %q", plain)
	}
}

// ownership -> family-hue: the owner family colors the cell; the owner text still reads in grayscale.
func TestEncodeCellFamilyHueCarriesOwner(t *testing.T) {
	c := EncodeCell(FacetRegistry{}, "ownership", CellValue{Text: "alpha", Width: 8}, false)
	if c.Channel != ChannelFamilyHue {
		t.Fatalf("ownership must bind to family-hue, got %v", c.Channel)
	}
	if !strings.Contains(ansi.Strip(c.Rendered), "alpha") {
		t.Fatalf("family-hue cell dropped the owner: %q", ansi.Strip(c.Rendered))
	}
}

// measure -> eighth-block bar: magnitude rides SHAPE (the fill), not a criticality hue.
func TestEncodeCellMagnitudeBarRidesShape(t *testing.T) {
	hi := EncodeCell(FacetRegistry{}, "measure", CellValue{Magnitude: 1.0, Text: "1.00", Width: 4}, false)
	lo := EncodeCell(FacetRegistry{}, "measure", CellValue{Magnitude: 0.0, Text: "0.00", Width: 4}, false)
	if hi.Channel != ChannelMagnitudeBar {
		t.Fatalf("measure must bind to magnitude-bar, got %v", hi.Channel)
	}
	hp, lp := ansi.Strip(hi.Rendered), ansi.Strip(lo.Rendered)
	if !strings.Contains(hp, "█") {
		t.Fatalf("full magnitude must fill the bar (shape): %q", hp)
	}
	if strings.Contains(lp, "█") {
		t.Fatalf("zero magnitude must NOT fill the bar: %q", lp)
	}
}

// time -> freshness-dim: recent vs stale differ by the brightness glyph (eighth-block height).
func TestEncodeCellFreshnessDimVariesWithMagnitude(t *testing.T) {
	recent := ansi.Strip(EncodeCell(FacetRegistry{}, "time", CellValue{Magnitude: 0.9, Text: "2m"}, false).Rendered)
	stale := ansi.Strip(EncodeCell(FacetRegistry{}, "time", CellValue{Magnitude: 0.05, Text: "9d"}, false).Rendered)
	if recent == stale {
		t.Fatalf("freshness must vary the glyph with recency: recent=%q stale=%q", recent, stale)
	}
}

// provenance -> the confidence pip ladder (●◉◐◍◌○); the pip survives grayscale.
func TestEncodeCellProvenancePip(t *testing.T) {
	c := EncodeCell(FacetRegistry{}, "provenance", CellValue{Text: "inferred", Width: 9}, false)
	if c.Channel != ChannelProvenancePip {
		t.Fatalf("provenance must bind to provenance-pip, got %v", c.Channel)
	}
	if !strings.Contains(ansi.Strip(c.Rendered), statusGlyphs["inferred"]) {
		t.Fatalf("provenance cell must carry the pip: %q", ansi.Strip(c.Rendered))
	}
}

// AIR is universal (like the row renderers): a denied attribute under air redacts to the fixed
// redaction token in EVERY channel, never leaking the value, while keeping the column's width/shape.
func TestEncodeCellAIRRedactsEveryChannel(t *testing.T) {
	for _, facet := range []string{"identity", "posture", "ownership", "measure", "time", "provenance"} {
		v := CellValue{Text: "SECRET", Magnitude: 0.9, Denied: true, Width: 8}
		out := ansi.Strip(EncodeCell(FacetRegistry{}, facet, v, true).Rendered)
		if strings.Contains(out, "SECRET") {
			t.Errorf("facet %q leaked a denied value on air: %q", facet, out)
		}
		if !strings.Contains(out, "▒") {
			t.Errorf("facet %q must show the redaction token when denied on air: %q", facet, out)
		}
	}
}

// A denied attribute with air OFF still renders its value (the redaction is on-air only).
func TestEncodeCellDeniedRendersOffAir(t *testing.T) {
	out := ansi.Strip(EncodeCell(FacetRegistry{}, "identity", CellValue{Text: "task-1", Denied: true, Width: 8}, false).Rendered)
	if !strings.Contains(out, "task-1") {
		t.Fatalf("denied attribute must still render off air: %q", out)
	}
}

// The Cell reports its channel + facet so a composer can reason about the (scarce) channel budget.
func TestEncodeCellReportsChannelAndFacet(t *testing.T) {
	c := EncodeCell(FacetRegistry{}, "measure", CellValue{Magnitude: 0.5}, false)
	if c.Facet != "measure" {
		t.Fatalf("cell must report its facet, got %q", c.Facet)
	}
	if c.Channel.String() != "magnitude-bar" {
		t.Fatalf("channel must name itself, got %q", c.Channel.String())
	}
}
