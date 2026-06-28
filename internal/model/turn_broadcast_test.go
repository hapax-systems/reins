package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// E4.6 — a STREAMING (in-flight) turn renders the two-frame broadcast in the detail pane: off air the
// present-at-hand partial; on air ONLY the abstract shape '▸ generating… [N tok]' and never a character
// of the partial (the AIR egress floor for live generation). A non-streaming turn renders normally.
func TestTurnDetailStreamingShowsTwoFrameBroadcast(t *testing.T) {
	m := New("R").FoldTurns([]grammar.Turn{
		{TS: "t0", Role: "cx-a", Kind: "assistant", Summary: "partial secret answer so far",
			Streaming: true, Tokens: 142, AIR: map[string]string{"ts": "ok", "role": "ok", "kind": "ok"}},
	}, false)
	m.Page = PageSessionTurns
	m.TurnFocus = 0

	off := ansi.Strip(m.turnDetailBody(80))
	if !strings.Contains(off, "partial secret answer") {
		t.Fatalf("off air a streaming turn shows the present-at-hand partial:\n%s", off)
	}

	m.AIR = true
	on := ansi.Strip(m.turnDetailBody(80))
	if strings.Contains(on, "partial secret answer") || strings.Contains(on, "secret") {
		t.Fatalf("on air a streaming turn must NOT disclose the partial text:\n%s", on)
	}
	if !strings.Contains(on, "generating") || !strings.Contains(on, "142 tok") {
		t.Fatalf("on air a streaming turn shows ONLY the shape ▸ generating… [N tok]:\n%s", on)
	}
}

// A completed (non-streaming) turn keeps the normal detail render — the broadcast frame is reserved for
// the in-flight case, so we never falsely label a settled turn "generating".
func TestTurnDetailNonStreamingIsNormal(t *testing.T) {
	m := New("R").FoldTurns([]grammar.Turn{
		{TS: "t0", Role: "cx-a", Kind: "assistant", Summary: "the finished answer",
			AIR: map[string]string{"ts": "ok", "role": "ok", "kind": "ok", "summary": "ok"}},
	}, false)
	m.Page = PageSessionTurns
	m.TurnFocus = 0
	out := ansi.Strip(m.turnDetailBody(80))
	if strings.Contains(out, "generating") {
		t.Fatalf("a settled turn must not be labeled generating:\n%s", out)
	}
}
