package media

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hapax-systems/reins/internal/imgpreview"
)

func writePNG(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: uint8(40 * x), G: uint8(50 * y), B: 128, A: 255})
		}
	}
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestContainerPaneRendersKittyPlaceholderBlock(t *testing.T) {
	c := Container{Source: "ignored.png", ID: 0x010203, Fidelity: imgpreview.Tier3TruePixel}
	out := c.Pane().Render(4, 3)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("height should produce 3 lines, got %d: %q", len(lines), out)
	}
	for i, line := range lines {
		if got := strings.Count(line, string(imgpreview.KittyPlaceholderRune())); got != 4 {
			t.Fatalf("line %d should contain 4 placeholders, got %d: %q", i, got, line)
		}
	}
	if strings.Contains(out, "▀") || strings.Contains(out, "PNG") {
		t.Fatalf("tier3 pane render must be placeholder-only text, not pixels/metadata: %q", out)
	}
}

func TestContainerPaneRendersOneFallbackTier(t *testing.T) {
	dir := t.TempDir()
	p := writePNG(t, dir, "img.png")

	braille := Container{Source: p, ID: 1, Fidelity: imgpreview.Tier2Braille}.Pane().Render(4, 2)
	if !hasBraille(braille) || strings.Contains(braille, string(imgpreview.KittyPlaceholderRune())) {
		t.Fatalf("tier2 should render braille only: %q", braille)
	}
	half := Container{Source: p, ID: 1, Fidelity: imgpreview.Tier1HalfBlock}.Pane().Render(4, 2)
	if !strings.Contains(half, "▀") || hasBraille(half) || strings.Contains(half, string(imgpreview.KittyPlaceholderRune())) {
		t.Fatalf("tier1 should render half-block only: %q", half)
	}
	meta := Container{Source: p, ID: 1, Fidelity: imgpreview.Tier0Metadata}.Pane().Render(4, 2)
	if !strings.Contains(meta, "PNG") || strings.Contains(meta, "▀") || hasBraille(meta) {
		t.Fatalf("tier0 should render pixel-free metadata only: %q", meta)
	}
}

func TestClampFidelityAIRHardCeiling(t *testing.T) {
	if got := ClampFidelity(imgpreview.Tier3TruePixel, true); got != imgpreview.Tier1HalfBlock {
		t.Fatalf("AIR must clamp tier3 to tier1, got %s", got)
	}
	if got := ClampFidelity(imgpreview.Tier2Braille, true); got != imgpreview.Tier1HalfBlock {
		t.Fatalf("AIR must clamp tier2 to tier1, got %s", got)
	}
	if got := ClampFidelity(imgpreview.Tier3TruePixel, false); got != imgpreview.Tier3TruePixel {
		t.Fatalf("off-air should preserve tier3, got %s", got)
	}
}

func TestTransmitOnceUsesTeaCmdAndLiveSet(t *testing.T) {
	dir := t.TempDir()
	p := writePNG(t, dir, "img.png")
	live := NewLiveMediaIDs()
	c := Container{Source: p, ID: 42, Fidelity: imgpreview.Tier3TruePixel, Rect: Rect{Cols: 5, Rows: 3}}
	cmd := c.TransmitOnce(live)
	if cmd == nil {
		t.Fatal("first tier3 container should produce a transmit command")
	}
	if !live.Has(42) {
		t.Fatal("transmit-once should add id to live set immediately")
	}
	msg, ok := cmd().(TerminalGraphicsMsg)
	if !ok {
		t.Fatalf("cmd should return TerminalGraphicsMsg, got %T", cmd())
	}
	if msg.Err != nil || msg.Op != "transmit" || !strings.Contains(msg.Payload, "\x1b_Ga=T") || !strings.Contains(msg.Payload, "i=42") || !strings.Contains(msg.Payload, "U=1") {
		t.Fatalf("bad transmit message: %+v", msg)
	}
	if again := c.TransmitOnce(live); again != nil {
		t.Fatal("second transmit for the same live id must be a no-op")
	}
	low := Container{Source: p, ID: 43, Fidelity: imgpreview.Tier1HalfBlock}
	if cmd := low.TransmitOnce(live); cmd != nil {
		t.Fatal("non-tier3 containers should not transmit pixel data")
	}
}

func TestDeleteTeardownDiffsLiveMediaIDs(t *testing.T) {
	previous := NewLiveMediaIDs(9, 3, 5)
	next := NewLiveMediaIDs(5)
	cmd := DeleteTeardownCmd(previous, next)
	if cmd == nil {
		t.Fatal("deleted ids should produce a teardown delete command")
	}
	msg, ok := cmd().(TerminalGraphicsMsg)
	if !ok {
		t.Fatalf("cmd should return TerminalGraphicsMsg, got %T", cmd())
	}
	if msg.Op != "delete" || len(msg.IDs) != 2 || msg.IDs[0] != 3 || msg.IDs[1] != 9 {
		t.Fatalf("deleted ids should be sorted [3 9], got %+v", msg)
	}
	if strings.Count(msg.Payload, "a=d") != 2 || !strings.Contains(msg.Payload, "i=3") || !strings.Contains(msg.Payload, "i=9") || strings.Contains(msg.Payload, "i=5") {
		t.Fatalf("delete payload should target removed ids only: %q", msg.Payload)
	}
	if none := DeleteTeardownCmd(next, next.Clone()); none != nil {
		t.Fatal("unchanged live set should not emit deletes")
	}
}

func hasBraille(s string) bool {
	for _, r := range s {
		if r >= 0x2800 && r <= 0x28FF {
			return true
		}
	}
	return false
}
