package grammar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestTurnRenderersDemoteDeniedAirChannels(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	t.Run("turn row role hue", func(t *testing.T) {
		tk := sampleTurn()
		tk.Role = "cc-secret"
		tk.AIR["role"] = "deny"

		cell := pad("▒▒▒", 10)
		got := RenderTurnRow(tk, true)
		if want := C("mut", cell); !strings.Contains(got, want) {
			t.Fatalf("denied turn role cell was not rendered with muted hue; want %q in:\n%q", want, got)
		}
		if bad := C(LaneToken(tk.Role), cell); strings.Contains(got, bad) {
			t.Fatalf("denied turn role leaked original lane hue via %q in:\n%q", bad, got)
		}
	})

	t.Run("turn detail block magnitude shape", func(t *testing.T) {
		tk := sampleTurn()
		blk := TurnBlock{
			Kind: "tool_call", Summary: "private tool payload", Magnitude: 0.99, Meta: "Bash",
			AIR: map[string]string{"kind": "ok", "meta": "ok", "magnitude": "deny"},
		}
		got := RenderTurnDetail(tk, []TurnBlock{blk}, true)
		if !strings.Contains(got, C("mut", "▒")) {
			t.Fatalf("denied block magnitude must render the redacted bar shape:\n%q", got)
		}
		if strings.Contains(ansi.Strip(got), ScoreBar(blk.Magnitude)) {
			t.Fatalf("denied block magnitude leaked the original ScoreBar shape:\n%q", got)
		}
	})
}
