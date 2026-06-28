package grammar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderCommandEnvelopeShowsGovernedShapeNeverMint(t *testing.T) {
	e := CommandEnvelope{
		Verb:      "arm",
		Target:    "reform-fix-eventlog",
		Payload:   "sdlc.authorization_flip(release_authorized=true)",
		Authority: "operator + governed COMMAND route",
		Preflight: "target + authority-packet + release gate",
		Receipt:   "armed receipt → spine",
		UIDelta:   "task arms; gate re-evaluates",
		Wired:     false,
	}
	out := ansi.Strip(RenderCommandEnvelope(e, false))
	for _, want := range []string{"arm", "reform-fix-eventlog", "sdlc.authorization_flip", "auth ", "preflight ", "receipt ", "NOT wired"} {
		if !strings.Contains(out, want) {
			t.Fatalf("governed envelope must show %q:\n%s", want, out)
		}
	}
}

func TestRenderCommandEnvelopeRedactsTargetOnAir(t *testing.T) {
	e := CommandEnvelope{Verb: "close", Target: "secret-task-id", Payload: "task.closed", Authority: "governed COMMAND route"}
	on := ansi.Strip(RenderCommandEnvelope(e, true))
	if strings.Contains(on, "secret-task-id") {
		t.Fatalf("the target is sensitive — must redact on air:\n%s", on)
	}
	if !strings.Contains(on, "▒▒▒") {
		t.Fatalf("redaction token expected for the target on air:\n%s", on)
	}
	// the governed SHAPE survives — the stream can see WHAT without WHICH
	if !strings.Contains(on, "close") || !strings.Contains(on, "task.closed") {
		t.Fatalf("the verb + payload (governed shape) must survive on air:\n%s", on)
	}
}
