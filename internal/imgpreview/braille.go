package imgpreview

import (
	"fmt"
	"image"
	"strings"
)

// brailleBits[col][row] is the U+2800 dot bit for each of the 2×4 sub-cells of a braille glyph
// (Unicode dot numbering 1-2-3-7 down the left, 4-5-6-8 down the right).
var brailleBits = [2][4]byte{
	{0x01, 0x02, 0x04, 0x40},
	{0x08, 0x10, 0x20, 0x80},
}

func lum(r, g, b uint8) float64 { return 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b) }

// RenderBraille renders img as a braille (2×4 dots per cell) grid — ~4× the spatial resolution of
// RenderHalfBlock, dot-matrix style. Within each cell a sub-pixel's dot is set when it is brighter
// than the cell's mean luminance (so local structure/edges show), and the cell is truecolor-tinted
// by the mean color of its set dots — higher detail while keeping color. Returns "" for a nil image
// or non-positive dimensions (never panics).
func RenderBraille(img image.Image, cols, rows int) string {
	if img == nil || cols <= 0 || rows <= 0 {
		return ""
	}
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw <= 0 || ih <= 0 {
		return ""
	}
	at := func(sx, sy int) (r, g, bl uint8) {
		px := b.Min.X + sx*iw/(2*cols)
		py := b.Min.Y + sy*ih/(4*rows)
		rr, gg, bb, _ := img.At(px, py).RGBA()
		return uint8(rr >> 8), uint8(gg >> 8), uint8(bb >> 8)
	}
	var sb strings.Builder
	for cy := 0; cy < rows; cy++ {
		for cx := 0; cx < cols; cx++ {
			var cr, cg, cb [2][4]uint8
			var l [2][4]float64
			mean := 0.0
			for dx := 0; dx < 2; dx++ {
				for dy := 0; dy < 4; dy++ {
					r, g, bl := at(cx*2+dx, cy*4+dy)
					cr[dx][dy], cg[dx][dy], cb[dx][dy] = r, g, bl
					l[dx][dy] = lum(r, g, bl)
					mean += l[dx][dy]
				}
			}
			mean /= 8
			var bits byte
			sr, sg, sbl, n := 0, 0, 0, 0
			for dx := 0; dx < 2; dx++ {
				for dy := 0; dy < 4; dy++ {
					if l[dx][dy] >= mean {
						bits |= brailleBits[dx][dy]
						sr += int(cr[dx][dy])
						sg += int(cg[dx][dy])
						sbl += int(cb[dx][dy])
						n++
					}
				}
			}
			if n == 0 {
				n = 1
			}
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm%c", sr/n, sg/n, sbl/n, rune(0x2800+int(bits)))
		}
		sb.WriteString("\x1b[0m")
		if cy < rows-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
