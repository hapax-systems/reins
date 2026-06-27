package grammar

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func runeW(s string) int { return len([]rune(ansi.Strip(s))) }

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
	"variant":    "text / view-axis",
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
	"variant":    ChannelText,
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

// identity/action/variant -> plain text in the primary ink, padded to width, value preserved.
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

// RenderFacetRow composes a row from faceted cells via the encoder — the generalization of the
// per-kind RenderXxxRow strips to ANY faceted entity. Cells render in the given order, every value
// carried, grayscale-safe (no live-pane behavior change; purely additive).
func TestRenderFacetRowComposesCellsInOrder(t *testing.T) {
	row := ansi.Strip(RenderFacetRow(FacetRegistry{}, []FacetCell{
		{Facet: "identity", Value: CellValue{Text: "task-4284", Width: 12}},
		{Facet: "posture", Value: CellValue{Text: "warn", Width: 6}},
		{Facet: "ownership", Value: CellValue{Text: "alpha", Width: 8}},
	}, false))
	iIdx, pIdx, oIdx := strings.Index(row, "task-4284"), strings.Index(row, "warn"), strings.Index(row, "alpha")
	if iIdx < 0 || pIdx < 0 || oIdx < 0 {
		t.Fatalf("row dropped a cell value: %q", row)
	}
	if !(iIdx < pIdx && pIdx < oIdx) {
		t.Fatalf("cells must render in the given order: %q", row)
	}
}

// AIR is per-cell in a composed row: a denied cell redacts while its neighbors still render on air.
func TestRenderFacetRowAIRRedactsPerCell(t *testing.T) {
	row := ansi.Strip(RenderFacetRow(FacetRegistry{}, []FacetCell{
		{Facet: "ownership", Value: CellValue{Text: "alpha", Width: 8}},
		{Facet: "identity", Value: CellValue{Text: "SECRET-PATH", Denied: true, Width: 12}},
	}, true))
	if strings.Contains(row, "SECRET-PATH") {
		t.Fatalf("a denied cell leaked in a composed on-air row: %q", row)
	}
	if !strings.Contains(row, "alpha") {
		t.Fatalf("an allowed cell must still air alongside a denied one: %q", row)
	}
	if !strings.Contains(row, "▒") {
		t.Fatalf("the denied cell must show the redaction token: %q", row)
	}
}

// An empty cell list yields an empty row (no spurious gutters).
func TestRenderFacetRowEmpty(t *testing.T) {
	if got := RenderFacetRow(FacetRegistry{}, nil, false); got != "" {
		t.Fatalf("empty row must be empty, got %q", got)
	}
}

// Every glyph-bearing channel holds a STABLE column width across visible / empty-label / denied
// states when Width>0 — the freeze-frame "grid never jitters" property (structured-silence). A denied
// or empty cell that collapses to a narrower width misaligns every column after it on air.
func TestEncodeCellWidthStableAcrossStates(t *testing.T) {
	const w = 8
	for _, facet := range []string{"measure", "time", "posture", "provenance"} {
		var vis, empty string
		switch facet {
		case "measure", "time":
			vis = EncodeCell(FacetRegistry{}, facet, CellValue{Magnitude: 0.9, Text: "0.90", Width: w}, false).Rendered
			empty = EncodeCell(FacetRegistry{}, facet, CellValue{Magnitude: 0.9, Width: w}, false).Rendered
		default:
			vis = EncodeCell(FacetRegistry{}, facet, CellValue{Text: "crit", Width: w}, false).Rendered
			empty = EncodeCell(FacetRegistry{}, facet, CellValue{Width: w}, false).Rendered
		}
		denied := EncodeCell(FacetRegistry{}, facet, CellValue{Magnitude: 0.9, Text: "0.90", Denied: true, Width: w}, true).Rendered
		if runeW(vis) != runeW(denied) {
			t.Errorf("facet %q: visible width %d != denied width %d (on-air grid jitter)", facet, runeW(vis), runeW(denied))
		}
		if runeW(vis) != runeW(empty) {
			t.Errorf("facet %q: visible width %d != empty-label width %d (grid jitter)", facet, runeW(vis), runeW(empty))
		}
	}
}

// An empty text/family cell at Width>0 renders structured-silence dots (the grid reads "nothing here"
// rather than a jarring blank — the dotsOr convention, made a property of the encoder).
func TestEncodeCellTextEmptyIsStructuredSilence(t *testing.T) {
	for _, facet := range []string{"identity", "action", "variant", "ownership", "place"} {
		out := ansi.Strip(EncodeCell(FacetRegistry{}, facet, CellValue{Text: "", Width: 6}, false).Rendered)
		if !strings.Contains(out, "······") {
			t.Errorf("facet %q empty cell must be structured-silence dots, got %q", facet, out)
		}
	}
}

// An unrecognized channel prose (a future re-wording of the registry) must resolve to ChannelUnknown
// — an explicit sentinel — never silently to text. The three real text facets still resolve to text.
func TestChannelFromProseUnknownIsExplicit(t *testing.T) {
	if ChannelFromProse("magnitude fill") != ChannelUnknown {
		t.Fatalf("unrecognized prose must be ChannelUnknown, not a silent text downgrade")
	}
	for _, p := range []string{"text + selection-shape", "text / view-axis"} {
		if ChannelFromProse(p) != ChannelText {
			t.Fatalf("text prose must resolve to ChannelText: %q", p)
		}
	}
}

// When the LIVE registry prose is unrecognized, ChannelForFacet falls back to the NAME-keyed default
// (stable across channel re-wording) — so the live path never silently drops a meaning channel on air.
func TestChannelForFacetFallsBackOnUnknownProse(t *testing.T) {
	reg := FacetRegistry{Facets: map[string]FacetDef{"measure": {Channel: "magnitude fill"}}}
	if got := ChannelForFacet(reg, "measure"); got != ChannelMagnitudeBar {
		t.Fatalf("unrecognized prose must fall back to the name default (magnitude-bar), got %v", got)
	}
}

// TestChannelBindingMatchesPythonRegistry is the REAL cross-language SSOT pin: it reads the actual
// api/facet_registry.py (the Python SSOT — pure stdlib, so plain python3 imports it) and asserts every
// served facet's channel prose parses to the SAME Channel the Go default table holds, and the facet
// key sets match. This closes the drift the Go-vs-Go guards cannot see (an operator re-wording the
// registry). It SKIPS (not fails) when no python can import the registry, so a python-less build still
// passes — but it runs wherever python3 exists.
func TestChannelBindingMatchesPythonRegistry(t *testing.T) {
	py := findRegistryPython()
	if py == "" {
		t.Skip("no python able to import api/facet_registry; cross-language SSOT guard skipped")
	}
	out, err := exec.Command(py, "-c",
		"import sys,os,json; sys.path.insert(0, os.path.abspath('../../api')); "+
			"import facet_registry as fr; print(json.dumps(fr.facets_payload()))").Output()
	if err != nil {
		t.Skipf("could not dump facet_registry payload: %v", err)
	}
	var payload struct {
		Facets map[string]struct {
			Channel string `json:"channel"`
		} `json:"facets"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("registry payload not JSON: %v", err)
	}
	if len(payload.Facets) == 0 {
		t.Fatal("registry served no facets")
	}
	for f := range payload.Facets {
		if _, ok := defaultFacetChannel[f]; !ok {
			t.Errorf("registry facet %q is absent from the Go default table", f)
		}
	}
	for f := range defaultFacetChannel {
		if _, ok := payload.Facets[f]; !ok {
			t.Errorf("Go default table facet %q is absent from the registry", f)
		}
	}
	for f, fd := range payload.Facets {
		want, ok := defaultFacetChannel[f]
		if !ok {
			continue
		}
		if got := ChannelFromProse(fd.Channel); got != want {
			t.Errorf("facet %q: registry prose %q parses to %v, Go default is %v (SSOT DRIFT)", f, fd.Channel, got, want)
		}
	}
}

// findRegistryPython returns a python interpreter that can import api/facet_registry, or "" if none.
func findRegistryPython() string {
	for _, c := range []string{os.ExpandEnv("python3"), "python3", "python"} {
		p, err := exec.LookPath(c)
		if err != nil {
			if _, statErr := os.Stat(c); statErr != nil {
				continue
			}
			p = c
		}
		if exec.Command(p, "-c",
			"import sys,os; sys.path.insert(0, os.path.abspath('../../api')); import facet_registry").Run() == nil {
			return p
		}
	}
	return ""
}
