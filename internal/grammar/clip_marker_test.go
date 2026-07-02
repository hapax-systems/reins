package grammar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// grammar.clipRunes must be ansi-aware AND disclose truncation — several call sites (RenderAxisRow,
// RenderIdentityRow, RenderConsentFacetRow) pass already-COLORIZED rows. Raw-rune clipping miscounts
// ansi escape bytes and silently drops content; the fix truncates by visible width with an overflow mark.
func TestClipRunesAnsiAwareWithMarker(t *testing.T) {
	// a colorized string wider than the budget: visible width must end at w AND disclose the drop.
	colored := C("pri", "abcdefghij") + C("2nd", "klmnopqrst") // 20 visible cells, ~dozens of raw runes
	out := clipRunes(colored, 8)
	if ansi.StringWidth(out) > 8 {
		t.Fatalf("clipRunes must bound VISIBLE width to 8 (ansi-aware), got %d: %q", ansi.StringWidth(out), out)
	}
	if !strings.Contains(out, OverflowMark) {
		t.Fatalf("clipRunes must disclose truncation with the overflow marker, got %q", out)
	}
	// a string within budget is returned unchanged (no spurious marker).
	if got := clipRunes(C("mut", "abc"), 40); strings.Contains(got, OverflowMark) {
		t.Fatalf("within-budget clip must not add a marker, got %q", got)
	}
}

// RenderAxisRow (a colorized-row clip site) must stay within visible width and disclose when it
// overflows — no silent horizontal loss on this visible row.
func TestColorizedRowClipSiteDiscloses(t *testing.T) {
	axis := RenderAxisRow(Axis{ID: "A1", Name: strings.Repeat("verylongname", 6), Question: strings.Repeat("q", 80), Status: "live"}, 40)
	if ansi.StringWidth(axis) > 40 {
		t.Fatalf("RenderAxisRow exceeds width 40: %d", ansi.StringWidth(axis))
	}
	if !strings.Contains(axis, OverflowMark) {
		t.Fatalf("an overflowing axis row must disclose: %q", axis)
	}
}

// commandStatusGlyph: the SUCCESS-class idempotent-replay (http 200) must read ✓, never the ✖ failure
// glyph (the sweep's u3b finding).
func TestCommandStatusGlyphIdempotentReplayIsSuccess(t *testing.T) {
	if commandStatusGlyph("idempotent-replay") != "✓" {
		t.Fatalf("idempotent-replay is success-class; want ✓, got %q", commandStatusGlyph("idempotent-replay"))
	}
	if commandStatusGlyph("ok") != "✓" || commandStatusGlyph("not-wired") != "⊘" || commandStatusGlyph("stage-rejected") != "✖" {
		t.Fatal("glyph mapping regressed for ok/not-wired/stage-rejected")
	}
}
