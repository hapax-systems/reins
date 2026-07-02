package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// model.clipRunes must be ANSI-AWARE: live call sites (renderCommandCatalog) pipe already-colorized
// rows through it, and a raw []rune clip counts escape bytes as budget — garbling the row + bleeding SGR.
// This is built with RAW ansi (not lipgloss, whose test profile defaults to Ascii and HID this bug).
func TestModelClipRunesAnsiAware(t *testing.T) {
	// ~24 visible cells wrapped in dozens of escape bytes.
	colored := "\x1b[38;2;235;219;178mSTAGEVERB\x1b[0m \x1b[38;2;131;165;152mtarget-name-here\x1b[0m"
	if vw := ansi.StringWidth(colored); vw != 26 {
		t.Fatalf("fixture visible width = %d, want 26", vw)
	}
	// budget 20 — well under the visible width (26) and FAR under the raw len (~50+ with escapes).
	out := clipRunes(colored, 20)
	if w := ansi.StringWidth(out); w > 20 {
		t.Fatalf("clipRunes must bound VISIBLE width to 20, got %d: %q", w, out)
	}
	// the raw-rune bug would have consumed the budget with escape bytes -> near-zero visible content.
	// ansi-aware must retain real visible cells (well over half the budget) + the leading verb text.
	if ansi.StringWidth(out) < 12 {
		t.Fatalf("ansi-aware clip must retain visible content, got only %d cells: %q", ansi.StringWidth(out), out)
	}
	if !strings.Contains(ansi.Strip(out), "STAGE") {
		t.Fatalf("leading visible text must survive the clip (raw-rune bug ate it in escapes): %q", ansi.Strip(out))
	}
	// within-budget input is returned unchanged (no spurious ellipsis).
	if got := clipRunes("\x1b[1mabc\x1b[0m", 40); got != "\x1b[1mabc\x1b[0m" {
		t.Fatalf("within-budget colorized input must be unchanged, got %q", got)
	}
}

// End-to-end: a witnessed command row (the exact live :commands feed) clipped to a deck-width budget
// stays width-bounded and legible — the pane the remediation review flagged as garbled at color-on.
func TestCommandRowClipStaysLegible(t *testing.T) {
	row := grammar.RenderCommandRow(grammar.Command{
		Verb: "stage", Target: "cc-task-something-long-that-overflows", Status: "ok", Witness: "pending",
		TaskID: "cc-task-something-long-that-overflows", AIR: map[string]string{"verb": "ok", "status": "ok", "witness": "ok"},
	}, false)
	clipped := clipRunes(row, 40)
	if w := ansi.StringWidth(clipped); w > 40 {
		t.Fatalf("clipped command row exceeds 40 visible cells: %d", w)
	}
	if !strings.Contains(ansi.Strip(clipped), "stage") {
		t.Fatalf("the verb must survive a deck-width clip: %q", ansi.Strip(clipped))
	}
}
