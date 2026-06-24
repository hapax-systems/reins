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
	if LaneToken("alpha") != "org" || LaneToken("cc-seg") != "blu" || LaneToken("gov") != "fch" || LaneToken("mystery") != "mut" {
		t.Fatal("lane mapping")
	}
	if ProvToken("asserted") != "eme" || ProvToken("simulated") != "blu" {
		t.Fatal("provenance mapping")
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
