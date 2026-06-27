package imgpreview

import (
	"image"
	"os"
)

// AIR image-fidelity doctrine (operator ruling, 2026-06-27): on a livestream the image preview is
// shown as the ORIGINAL coarse BLOCK-PIXEL (half-block) rendering — deliberately low-fidelity. Fine
// detail (text, faces, fine PII) is destroyed below legibility BY CONSTRUCTION via a hard resolution
// ceiling, while the gist (rough colors + shapes) survives. This is confidentiality-by-resolution:
// the same fidelity ladder that is an aesthetic choice off-air (true-pixel → braille → half-block)
// becomes a SAFETY clamp on-air. The off-air frame may sharpen freely; the on-air frame may not.
const (
	// AIRMaxCols / AIRMaxRows cap the on-air half-block resolution regardless of pane size — a wide
	// pane cannot sharpen the on-air image. 40×22 cells ≈ a 40×44 px downsample: any text line lands
	// well under a legible glyph height, a face is unrecognizable. Lower the ceiling to harden further.
	AIRMaxCols = 40
	AIRMaxRows = 22
)

// airShapeOnly is the name-free, pixel-free fallback when the image cannot be decoded on air. It must
// NEVER embed the path — Metadata() does, which would leak the very filename the caller redacted.
const airShapeOnly = "image · shape unavailable (pixels and filename withheld on air)"

// RenderFileAIR renders an image file for the ON-AIR frame: a half-block grid clamped to the AIR
// coarseness ceiling (never sharper than AIRMaxCols×AIRMaxRows, never sharper than the pane). It is
// egress-safe by construction: the resolution clamp destroys fine detail, and every failure path
// folds to airShapeOnly — never to a filename-bearing Metadata line.
func RenderFileAIR(path string, maxCols, maxRows int) string {
	cols, rows := clampAIR(maxCols, maxRows)
	f, err := os.Open(path)
	if err != nil {
		return airShapeOnly
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return airShapeOnly
	}
	fc, fr := FitCells(img.Bounds().Dx(), img.Bounds().Dy(), cols, rows)
	out := RenderHalfBlock(img, fc, fr)
	if out == "" {
		return airShapeOnly
	}
	return out
}

// clampAIR bounds the requested cell budget by the AIR ceiling (and a sane floor) so the on-air image
// is always coarse, whatever the pane size.
func clampAIR(maxCols, maxRows int) (int, int) {
	cols, rows := maxCols, maxRows
	if cols > AIRMaxCols {
		cols = AIRMaxCols
	}
	if rows > AIRMaxRows {
		rows = AIRMaxRows
	}
	if cols < 2 {
		cols = 2
	}
	if rows < 1 {
		rows = 1
	}
	return cols, rows
}
