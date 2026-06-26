package palette

import (
	"strings"
	"testing"
)

func TestModeAndHex(t *testing.T) {
	if For("gruvbox").Hex("red") != "#fb4934" {
		t.Fatal("gruvbox red hex")
	}
	r := For("research")
	if r.Mode() != "solarized" || r.Hex("red") != "#dc322f" {
		t.Fatalf("research -> solarized: mode=%s red=%s", r.Mode(), r.Hex("red"))
	}
}

func TestSemanticTokens(t *testing.T) {
	if SeverityToken("failed") != "red" || SeverityToken("done") != "grn" || SeverityToken("review") != "yel" || SeverityToken("major") != "org" {
		t.Fatal("severity mapping")
	}
	if LaneToken("alpha") != "eme" || LaneToken("cc-seg") != "blu" || LaneToken("gov") != "fch" || LaneToken("") != "mut" {
		t.Fatal("lane mapping (must be disjoint from severity)")
	}
	if ProvToken("asserted") != "eme" || ProvToken("simulated") != "blu" {
		t.Fatal("provenance mapping")
	}
}

func TestDimSeverityTokenDimsStaleWithinHue(t *testing.T) {
	// fresh -> full hue; stale -> the _dim variant (keeps the hue, breaks the warn monoculture)
	if got := DimSeverityToken("warn", 0.9); got != "yel" {
		t.Fatalf("fresh warn: want yel, got %s", got)
	}
	if got := DimSeverityToken("warn", 0.1); got != "yel_dim" {
		t.Fatalf("stale warn: want yel_dim, got %s", got)
	}
	if got := DimSeverityToken("crit", 0.1); got != "red_dim" {
		t.Fatalf("stale crit: want red_dim, got %s", got)
	}
	// a non-hue severity ("mut") has no hue to dim -> stays mut
	if got := DimSeverityToken("unknown", 0.1); got != "mut" {
		t.Fatalf("non-hue severity: want mut, got %s", got)
	}
}

func TestDimVariantsDefinedAndDisjointFromLanes(t *testing.T) {
	for _, mode := range []string{"gruvbox", "solarized"} {
		p := For(mode)
		for _, dim := range []string{"grn_dim", "yel_dim", "org_dim", "red_dim"} {
			if p.Hex(dim) == "" {
				t.Fatalf("%s dim variant %s undefined", mode, dim)
			}
			bright := strings.TrimSuffix(dim, "_dim")
			if p.Hex(dim) == p.Hex(bright) {
				t.Fatalf("%s %s identical to its bright hue (no dimming)", mode, dim)
			}
		}
	}
	// the dim variants are severity-channel hues -> must stay disjoint from lane tokens
	sev := map[string]bool{}
	for _, s := range severityColorTokens() {
		sev[s] = true
	}
	for _, l := range laneColorTokens() {
		if sev[l] {
			t.Fatalf("lane token %q collides with the severity channel", l)
		}
	}
}

func TestLaneAndSeverityChannelsDisjoint(t *testing.T) {
	// ownership and criticality are separate perceptual channels — no shared hue (anti 1+1=3).
	sev := map[string]bool{}
	for _, s := range severityColorTokens() {
		sev[s] = true
	}
	for _, l := range laneColorTokens() {
		if sev[l] {
			t.Fatalf("lane color %q also encodes severity — channel collision", l)
		}
	}
	// and the live mapping must obey it
	for _, owner := range []string{"alpha", "gov", "cc-seg", "cx-p0", "gamma", "delta"} {
		if sev[LaneToken(owner)] {
			t.Fatalf("LaneToken(%q)=%q collides with the criticality ramp", owner, LaneToken(owner))
		}
	}
}

func TestSelectionChannelIsDisjoint(t *testing.T) {
	// the SELECTION swatch (grammar.SelLabel) must ride SHAPE/CONTRAST on ground tones (border+brt),
	// never a meaning hue — or selection competes with criticality/freshness/ownership in the scan.
	hue := map[string]bool{}
	for _, x := range append(severityColorTokens(), laneColorTokens()...) {
		hue[x] = true
	}
	for _, sel := range []string{"border", "brt"} { // the tokens SelLabel uses
		if hue[sel] {
			t.Fatalf("selection channel token %q collides with a meaning hue", sel)
		}
	}
}

func TestColorizeIsNonDestructive(t *testing.T) {
	p := For("gruvbox")
	// color must never destroy the text (monochrome-safe: the glyph survives a strip)
	if !strings.Contains(p.Colorize("red", "FAIL"), "FAIL") {
		t.Fatal("colorize must keep the text")
	}
	// unknown token passes through unchanged
	if p.Colorize("nosuchtoken", "X") != "X" {
		t.Fatal("unknown token must pass through")
	}
	if p.Colorize("red", "") != "" {
		t.Fatal("empty text passes through")
	}
}
