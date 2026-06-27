package imgpreview

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func hasBraille(s string) bool {
	for _, r := range s {
		if r >= 0x2800 && r <= 0x28FF {
			return true
		}
	}
	return false
}

func TestFitCellsPreservesAspectAndMaximizes(t *testing.T) {
	// A wide image (2:1) fills the width and uses fewer rows; aspect cols:2·rows ≈ iw:ih.
	if c, r := FitCells(200, 100, 80, 40); c != 80 || r != 20 {
		t.Fatalf("wide image: want (80,20) to preserve 2:1 aspect, got (%d,%d)", c, r)
	}
	// A tall image (1:4) fills the rows and uses fewer cols.
	if c, r := FitCells(100, 400, 80, 40); r != 40 || c != 20 {
		t.Fatalf("tall image: want (20,40), got (%d,%d)", c, r)
	}
	if c, r := FitCells(0, 0, 80, 40); c != 80 || r != 40 {
		t.Fatal("degenerate dims fall back to the max budget")
	}
}

func TestRenderBrailleIsHigherResAndColored(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 16), G: uint8(y * 16), B: 128, A: 255})
		}
	}
	out := RenderBraille(img, 8, 4) // 8×4 cells = a 16×16 dot grid
	if ls := strings.Split(out, "\n"); len(ls) != 4 {
		t.Fatalf("4 cell-rows expected, got %d", len(ls))
	}
	if !hasBraille(out) {
		t.Fatal("output must contain braille glyphs (U+2800–U+28FF)")
	}
	if !strings.Contains(out, "\x1b[38;2;") {
		t.Fatal("braille cells must be truecolor-tinted")
	}
	if RenderBraille(nil, 8, 4) != "" || RenderBraille(img, 0, 4) != "" || RenderBraille(img, 8, 0) != "" {
		t.Fatal("nil image or non-positive dims must render empty (no panic)")
	}
}
