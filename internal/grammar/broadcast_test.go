package grammar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestBroadcastStreamFrameOnAirShapeOnly(t *testing.T) {
	partial := "secret draft text"
	got := BroadcastStreamFrame(partial, 17, true, 80)

	if !strings.Contains(got, "generating") {
		t.Fatalf("on-air frame missing generating shape: %q", got)
	}
	if !strings.Contains(got, "17 tok") {
		t.Fatalf("on-air frame missing token count: %q", got)
	}
	if strings.Contains(got, partial) || strings.Contains(got, "secret") || strings.Contains(got, "draft") || strings.Contains(got, "text") {
		t.Fatalf("on-air frame leaked partial text: %q", got)
	}
}

func TestBroadcastStreamFrameOffAirContainsPartial(t *testing.T) {
	partial := "live partial text"
	got := BroadcastStreamFrame(partial, 17, false, 80)

	if !strings.Contains(got, partial) {
		t.Fatalf("off-air frame missing partial text: %q", got)
	}
}

func TestBroadcastStreamFrameOffAirRespectsWidth(t *testing.T) {
	got := BroadcastStreamFrame("wide 世界 partial", 17, false, 8)

	if width := ansi.StringWidth(got); width > 8 {
		t.Fatalf("off-air frame width = %d, want <= 8: %q", width, got)
	}
}
