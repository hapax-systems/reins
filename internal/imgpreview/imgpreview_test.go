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

func TestDetectTerminalCapabilityTierFromSyntheticEnv(t *testing.T) {
	kitty := DetectTerminal(envFrom(map[string]string{"KITTY_WINDOW_ID": "1", "COLORTERM": "truecolor"}))
	if kitty.Protocol != ProtoKitty || kitty.CapabilityTier != Tier3TruePixel || kitty.Authoritative {
		t.Fatalf("kitty env pre-guess should be non-authoritative tier3, got %+v", kitty)
	}
	truecolor := DetectTerminal(envFrom(map[string]string{"COLORTERM": "truecolor"}))
	if truecolor.Protocol != ProtoHalfBlock || truecolor.CapabilityTier != Tier2Braille {
		t.Fatalf("truecolor env should support braille tier with half-block legacy protocol, got %+v", truecolor)
	}
	color := DetectTerminal(envFrom(map[string]string{"TERM": "xterm-256color"}))
	if color.Protocol != ProtoHalfBlock || color.CapabilityTier != Tier1HalfBlock {
		t.Fatalf("256color env should support half-block floor, got %+v", color)
	}
}

func TestDetectTerminalPromotesAuthoritativeKittyResponse(t *testing.T) {
	resp := "prefix" + "\x1b_Gi=77;OK\x1b\\" + "\x1b[?62;4c"
	got := DetectTerminalFromResponse(envFrom(map[string]string{"TERM": "xterm-256color"}), 77, resp)
	if !got.Authoritative || got.Protocol != ProtoKitty || got.CapabilityTier != Tier3TruePixel {
		t.Fatalf("KGP OK before DA must promote to authoritative tier3, got %+v", got)
	}
	late := "\x1b[?62;4c" + "\x1b_Gi=77;OK\x1b\\"
	got = DetectTerminalFromResponse(envFrom(map[string]string{"TERM": "xterm-256color"}), 77, late)
	if got.Authoritative || got.CapabilityTier != Tier1HalfBlock {
		t.Fatalf("DA before KGP OK must keep fallback tier, got %+v", got)
	}
	insideTmux := DetectTerminalFromResponse(envFrom(map[string]string{"TMUX": "/tmp/t", "COLORTERM": "truecolor"}), 77, resp)
	if insideTmux.Protocol == ProtoKitty || insideTmux.CapabilityTier >= Tier3TruePixel {
		t.Fatalf("tmux passthrough is stubbed in this increment; should not claim tier3, got %+v", insideTmux)
	}
}

func TestEmitKittyPlaceholderDimensionsAndEncoding(t *testing.T) {
	out := EmitKittyPlaceholder(0x123456, 3, 2)
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("rows should become lines, got %d in %q", len(lines), out)
	}
	for i, line := range lines {
		if got := strings.Count(line, string(KittyPlaceholderRune())); got != 3 {
			t.Fatalf("line %d should contain 3 placeholders, got %d: %q", i, got, line)
		}
	}
	if !strings.Contains(out, "\x1b[38;2;18;52;86m") {
		t.Fatalf("image id low 24 bits must be encoded as foreground color: %q", out)
	}
	if strings.Contains(out, "▀") {
		t.Fatalf("placeholder render string must not contain half-block pixel glyphs: %q", out)
	}
	if EmitKittyPlaceholder(1, 0, 2) != "" || EmitKittyPlaceholder(0, 2, 2) != "" {
		t.Fatal("zero id or non-positive dimensions should render empty")
	}
}

func TestKittyGraphicsQueryShape(t *testing.T) {
	q := KittyGraphicsQuery(99)
	if !strings.Contains(q, "\x1b_Gi=99,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\") || !strings.HasSuffix(q, "\x1b[c") {
		t.Fatalf("query must be a 1x1 KGP probe followed by CSI c: %q", q)
	}
}
