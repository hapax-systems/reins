package grammar

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Sparkline (GLM-lane-generated, verified here): edges, normalization, resample, and the signed/braille
// variants. Integrating lane code without a test would be trusting it blind.
func TestSparklineEdgesAndNormalization(t *testing.T) {
	if Sparkline(nil, 5) != "" || Sparkline([]float64{1, 2}, 0) != "" {
		t.Fatalf("empty series / non-positive width must render empty")
	}
	s := []rune(Sparkline([]float64{0, 0.5, 1}, 3))
	if len(s) != 3 || s[0] != ' ' || s[2] != '█' {
		t.Fatalf("normalized sparkline should span space..full block, got %q", string(s))
	}
	// resample: a long series compresses to the requested width.
	if got := []rune(Sparkline([]float64{1, 2, 3, 4, 5, 6, 7, 8}, 4)); len(got) != 4 {
		t.Fatalf("resample to width 4, got %d", len(got))
	}
}

func TestBrailleAndNetSparkline(t *testing.T) {
	if got := []rune(BrailleSparkline([]float64{0.2, 0.8, 0.5, 1}, 2)); len(got) != 2 {
		t.Fatalf("braille sparkline should be width cells, got %d", len(got))
	}
	// a braille cell is in the U+2800 block.
	for _, r := range []rune(BrailleSparkline([]float64{1, 0.5, 0.2, 0.9}, 2)) {
		if r < 0x2800 || r > 0x28FF {
			t.Fatalf("braille rune out of block: %U", r)
		}
	}
	// NetSparkline: zero -> dot; the magnitudes render; negatives are dimmed (mut), not dropped.
	n := ansi.Strip(NetSparkline([]float64{4, -4, 0}, 3))
	if []rune(n)[2] != '·' {
		t.Fatalf("net zero bucket must be a dot: %q", n)
	}
}
