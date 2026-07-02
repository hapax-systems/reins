package layout

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// fitLine must DISCLOSE horizontal truncation with the right-edge overflow marker (honest-when-starved),
// never silently clip — and the marked row stays EXACTLY w wide (width-determinism).
func TestFitLineDisclosesHorizontalOverflow(t *testing.T) {
	// a row wider than the pane: content is dropped -> the marker must appear at the right edge.
	over := fitLine("abcdefghij", 5)
	if ansi.StringWidth(over) != 5 {
		t.Fatalf("truncated row width = %d, want 5 (width-determinism)", ansi.StringWidth(over))
	}
	if !strings.HasSuffix(strings.TrimRight(over, " "), OverflowMark) {
		t.Fatalf("horizontally-clipped row has no overflow marker (silently lossy): %q", over)
	}

	// an exact-width row is NOT marked (no content dropped).
	exact := fitLine("abcde", 5)
	if strings.Contains(exact, OverflowMark) {
		t.Fatalf("exact-width row wrongly marked as overflowing: %q", exact)
	}
	if ansi.StringWidth(exact) != 5 {
		t.Fatalf("exact row width = %d, want 5", ansi.StringWidth(exact))
	}

	// a short row is padded, not marked.
	short := fitLine("ab", 5)
	if strings.Contains(short, OverflowMark) || ansi.StringWidth(short) != 5 {
		t.Fatalf("short row wrongly marked or mis-padded: %q (w=%d)", short, ansi.StringWidth(short))
	}
}

// The marker survives mono (a glyph, not a color) — the doctrine floor: color is a redundant amplifier.
func TestOverflowMarkerIsAGlyph(t *testing.T) {
	if OverflowMark == "" || ansi.StringWidth(OverflowMark) != 1 {
		t.Fatalf("overflow marker must be a single visible cell, got %q (w=%d)", OverflowMark, ansi.StringWidth(OverflowMark))
	}
}
