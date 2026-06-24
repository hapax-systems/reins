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
