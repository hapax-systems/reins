package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/layout"
)

// The SELECTED/cursor row (focusBar) is the most-visible row — it must DISCLOSE horizontal truncation
// with the overflow marker, never silently clip, at the narrow widths the deck / narrow-split hit.
// (Review followup: the general fitters were fixed but focusBar was left silently lossy.)
func TestFocusBarDisclosesOverflow(t *testing.T) {
	over := ansi.Strip(focusBar("this row is much wider than the pane", 12))
	if ansi.StringWidth(over) != 12 {
		t.Fatalf("focused row width = %d, want 12 (width-determinism)", ansi.StringWidth(over))
	}
	if !strings.HasSuffix(strings.TrimRight(over, " "), layout.OverflowMark) {
		t.Fatalf("focused (selected) row is silently clipped — no overflow marker: %q", over)
	}

	// a short focused row is padded, not marked.
	short := ansi.Strip(focusBar("ok", 12))
	if strings.Contains(short, layout.OverflowMark) || ansi.StringWidth(short) != 12 {
		t.Fatalf("short focused row wrongly marked or mis-padded: %q (w=%d)", short, ansi.StringWidth(short))
	}
}
