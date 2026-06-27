package imgpreview

import (
	"fmt"
	"image"
	_ "image/gif"  // register decoders for DecodeConfig / Decode
	_ "image/jpeg" //
	_ "image/png"  //
	"os"
	"path/filepath"
	"strings"
)

// upperHalf fills the TOP of a cell with the foreground color; the cell background paints the
// bottom — so one character row carries two image pixel rows.
const upperHalf = "▀"

// RenderHalfBlock renders img as a truecolor ANSI half-block grid: cols wide × rows tall, each
// cell packing two vertical pixels (fg = top, bg = bottom) under ▀. Pure stdlib, string-native —
// safe to return from a Bubble Tea View(); works in tmux and any truecolor terminal. Returns ""
// for a nil image or non-positive dimensions (never panics).
func RenderHalfBlock(img image.Image, cols, rows int) string {
	if img == nil || cols <= 0 || rows <= 0 {
		return ""
	}
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw <= 0 || ih <= 0 {
		return ""
	}
	// nearest-neighbor sample from the cols×(2·rows) grid into the source image
	sample := func(cx, cy int) (uint8, uint8, uint8) {
		px := b.Min.X + cx*iw/cols
		py := b.Min.Y + cy*ih/(2*rows)
		r, g, bl, _ := img.At(px, py).RGBA()
		return uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8)
	}
	var sb strings.Builder
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			tr, tg, tb := sample(x, 2*y)
			br, bg, bb := sample(x, 2*y+1)
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm%s", tr, tg, tb, br, bg, bb, upperHalf)
		}
		sb.WriteString("\x1b[0m")
		if y < rows-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// Metadata returns a one-line, pixel-free descriptor of an image file ("name · size · FMT WxH").
// This is the on-air / default-deny fallback: it never egresses pixels.
func Metadata(path string) string {
	name := filepath.Base(path)
	size := "?"
	if fi, err := os.Stat(path); err == nil {
		size = humanSize(fi.Size())
	}
	cfg, format, err := decodeConfig(path)
	if err != nil {
		return fmt.Sprintf("%s · %s · (not a decodable image)", name, size)
	}
	return fmt.Sprintf("%s · %s · %s %dx%d", name, size, strings.ToUpper(format), cfg.Width, cfg.Height)
}

func decodeConfig(path string) (image.Config, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return image.Config{}, "", err
	}
	defer f.Close()
	return image.DecodeConfig(f)
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// RenderFileBraille previews an image file as a higher-resolution braille (2×4 dots/cell) grid —
// the dot-matrix mode. It aspect-fits the image into the maxCols×maxRows budget (FitCells), so a
// larger pane yields a bigger, higher-resolution preview without stretching. Decode failure falls
// back to the pixel-free Metadata line.
func RenderFileBraille(path string, maxCols, maxRows int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return Metadata(path), err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return Metadata(path), nil
	}
	cols, rows := FitCells(img.Bounds().Dx(), img.Bounds().Dy(), maxCols, maxRows)
	return RenderBraille(img, cols, rows), nil
}

// RenderFile previews an image file within cols×rows cells for the chosen protocol. ProtoHalfBlock
// and ProtoKitty decode + render the half-block grid (kitty placeholders are a follow-up that will
// upgrade ProtoKitty in place); ProtoMetadataOnly — and any decode failure — falls back to the
// pixel-free Metadata line. A decode failure is an honest fallback, not a hard error.
func RenderFile(path string, cols, rows int, proto Protocol) (string, error) {
	if proto == ProtoMetadataOnly {
		return Metadata(path), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return Metadata(path), err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return Metadata(path), nil
	}
	return RenderHalfBlock(img, cols, rows), nil
}
