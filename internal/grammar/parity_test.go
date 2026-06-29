package grammar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// E4.7 chat-UX-bar: every native-session interactive verb (session-pane design §6) maps to an explicit
// reins projection — a reins verb, a passthrough, an honest N/A (excised by the direction-only doctrine),
// or a flagged GAP. No native verb may be silently uncovered (a hidden lossless hole = the floor leaking).
func TestChatParityManifestCoversNativeVerbSetWithNoSilentGap(t *testing.T) {
	rows := ChatParityManifest()
	want := []string{"send-prompt", "/compact", "tool-approval", "interrupt", "paste-images", "scroll-history", "model-switch", "MCP-tool-list"}
	have := map[string]bool{}
	for _, r := range rows {
		if r.Kind != ParityGap && strings.TrimSpace(r.Reins) == "" {
			t.Fatalf("native verb %q is marked covered but names no reins projection (silent gap)", r.Native)
		}
		have[r.Native] = true
	}
	for _, n := range want {
		if !have[n] {
			t.Fatalf("the parity manifest omits the native verb %q", n)
		}
	}
}

// Honesty: a lossless hole is surfaced as a GAP, never hidden behind a green claim. The terminal cannot
// inline-render pasted images (a genuine native capability) — that MUST be flagged, not papered over.
func TestParityGapsAreExplicitNotHidden(t *testing.T) {
	rows := ChatParityManifest()
	gaps := ParityGaps(rows)
	found := false
	for _, g := range gaps {
		if g.Kind != ParityGap {
			t.Fatalf("ParityGaps returned a non-gap row: %+v", g)
		}
		if strings.Contains(g.Native, "image") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the known terminal image-render gap must be flagged, not hidden: %v", gaps)
	}
	out := ansi.Strip(RenderChatParity(rows, 80))
	if !strings.Contains(out, "GAP") {
		t.Fatalf("the parity render must surface GAPs (chat-UX-bar honesty):\n%s", out)
	}
	// the direction-only doctrine must read as an explicit N/A, not a gap (model-switch is excised, not missing)
	if !strings.Contains(out, "N/A") {
		t.Fatalf("excised-by-doctrine verbs must render N/A (not GAP):\n%s", out)
	}
}
