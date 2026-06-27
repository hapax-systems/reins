package imgpreview

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// On air the half-block is clamped to the coarseness ceiling regardless of how large a budget the
// pane offers — a wide pane cannot sharpen the on-air image. This is the structural confidentiality
// guarantee: fine detail is destroyed by construction.
func TestRenderFileAIRClampsToCeiling(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 256, 256)) // a large square image
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	p := filepath.Join(dir, "big.png")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	out := RenderFileAIR(p, 500, 500) // ask for a huge budget
	rows := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1
	if rows > AIRMaxRows {
		t.Fatalf("on-air image must clamp to AIRMaxRows=%d, got %d rows", AIRMaxRows, rows)
	}
	for _, ln := range strings.Split(out, "\n") {
		// each half-block char is one cell; the visible cell count must not exceed the col ceiling
		cells := strings.Count(ln, "▀")
		if cells > AIRMaxCols {
			t.Fatalf("on-air image must clamp to AIRMaxCols=%d, got a %d-cell row", AIRMaxCols, cells)
		}
	}
}

// A decode failure (or missing file) on air must fold to the name-free shape-only line — NEVER to a
// filename-bearing Metadata line, which would leak the very name the caller redacted.
func TestRenderFileAIRNeverLeaksNameOnFailure(t *testing.T) {
	out := RenderFileAIR("/nonexistent/super-secret-name.png", 40, 22)
	if strings.Contains(out, "super-secret-name") {
		t.Fatalf("a failed on-air decode must not embed the filename:\n%s", out)
	}
	if !strings.Contains(out, "shape unavailable") {
		t.Fatalf("a failed on-air decode must fold to the shape-only line:\n%s", out)
	}
}
