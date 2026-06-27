package imgpreview

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDetectProtocol(t *testing.T) {
	// kitty OUTSIDE tmux -> kitty placeholders are claimable.
	if got := DetectProtocol(envFrom(map[string]string{"KITTY_WINDOW_ID": "1", "COLORTERM": "truecolor"})); got != ProtoKitty {
		t.Fatalf("kitty (no tmux) -> ProtoKitty, got %v", got)
	}
	// kitty INSIDE tmux -> the universal half-block floor: passthrough cannot be confirmed from env,
	// so claiming kitty would risk a broken blit. Conservative is correct.
	if got := DetectProtocol(envFrom(map[string]string{"KITTY_WINDOW_ID": "1", "TMUX": "/tmp/s", "COLORTERM": "truecolor"})); got != ProtoHalfBlock {
		t.Fatalf("kitty in tmux falls to the half-block floor, got %v", got)
	}
	// Konsole (KDE host) + truecolor -> floor.
	if got := DetectProtocol(envFrom(map[string]string{"KONSOLE_VERSION": "230804", "COLORTERM": "truecolor"})); got != ProtoHalfBlock {
		t.Fatalf("konsole+truecolor -> ProtoHalfBlock, got %v", got)
	}
	// No color signal -> metadata only (never paint pixels we can't render).
	if got := DetectProtocol(envFrom(map[string]string{"TERM": "dumb"})); got != ProtoMetadataOnly {
		t.Fatalf("no color -> ProtoMetadataOnly, got %v", got)
	}
}

func TestTmuxWrapDoublesEsc(t *testing.T) {
	out := TmuxWrap("\x1b_Gabc\x1b\\")
	if !strings.HasPrefix(out, "\x1bPtmux;") || !strings.HasSuffix(out, "\x1b\\") {
		t.Fatalf("tmux passthrough envelope missing: %q", out)
	}
	if !strings.Contains(out, "\x1b\x1b") {
		t.Fatal("ESC must be doubled inside the tmux passthrough envelope")
	}
}

func TestRenderHalfBlock(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	out := RenderHalfBlock(img, 4, 2) // 4 cols x 2 rows -> 2 lines
	if out == "" {
		t.Fatal("expected a non-empty render")
	}
	if lines := strings.Split(out, "\n"); len(lines) != 2 {
		t.Fatalf("2 rows -> 2 lines, got %d", len(lines))
	}
	if !strings.Contains(out, "▀") {
		t.Fatal("half-block glyph ▀ expected")
	}
	if !strings.Contains(out, "\x1b[38;2;255;0;0m") {
		t.Fatal("truecolor red foreground expected")
	}
	if RenderHalfBlock(img, 0, 2) != "" || RenderHalfBlock(nil, 4, 2) != "" {
		t.Fatal("non-positive dims or nil image -> empty (no panic)")
	}
}

func TestRenderFileAndMetadata(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "img.png")
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	f, _ := os.Create(p)
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	out, err := RenderFile(p, 8, 4, ProtoHalfBlock)
	if err != nil || !strings.Contains(out, "▀") {
		t.Fatalf("png half-block render failed: %v out=%q", err, out)
	}
	// Metadata-only protocol egresses NO pixels — just a descriptor with dims/format.
	meta, _ := RenderFile(p, 8, 4, ProtoMetadataOnly)
	if !strings.Contains(meta, "8x8") || !strings.Contains(strings.ToUpper(meta), "PNG") {
		t.Fatalf("metadata must carry dims+format: %q", meta)
	}
	if strings.Contains(meta, "▀") {
		t.Fatalf("metadata-only must NOT egress pixels: %q", meta)
	}
	// A non-image file degrades honestly, never panics.
	txt := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txt, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m := Metadata(txt); !strings.Contains(m, "not a decodable image") {
		t.Fatalf("non-image metadata should say so: %q", m)
	}
}
