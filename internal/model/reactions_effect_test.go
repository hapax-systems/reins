package model

import (
	"strings"
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

// E8.2: a fired /on reaction applies LOCAL effects (flash/log) but keeps NETWORK effects (ntfy) gated —
// reins never emits off-box from an automation (the egress-always-gate rule).
func TestReactionLocalEffectAppliesNetworkGated(t *testing.T) {
	for _, eff := range []string{"flash", "log", "echo", "bell"} {
		if !reactionEffectIsLocal(eff) {
			t.Fatalf("%q must be a local effect", eff)
		}
	}
	for _, eff := range []string{"ntfy", "webhook", "post", "email", "slack", "http"} {
		if reactionEffectIsLocal(eff) {
			t.Fatalf("%q is egress and must stay gated", eff)
		}
	}

	// integration: a matching event SURFACES the fire (preview-only, never-mint) — local effects read as
	// plain preview, network effects name the egress gate.
	flash := Model{Reactions: grammar.ReactionSet{{EventKind: "fail", Effect: "flash", Raw: "/on fail * {flash}"}}}
	flash = flash.fireReactionsForNewEvents([]grammar.Event{{Kind: "build.fail", Summary: "boom", TS: "t0"}}, map[string]struct{}{})
	if !strings.Contains(flash.Status, "would emit flash; NOT wired") || strings.Contains(flash.Status, "egress gated") {
		t.Fatalf("a local flash is preview-only (no egress note): %q", flash.Status)
	}

	gated := Model{Reactions: grammar.ReactionSet{{EventKind: "fail", Effect: "ntfy", Raw: "/on fail * {ntfy}"}}}
	gated = gated.fireReactionsForNewEvents([]grammar.Event{{Kind: "build.fail", Summary: "boom", TS: "t0"}}, map[string]struct{}{})
	if !strings.Contains(gated.Status, "NOT wired — network egress gated") {
		t.Fatalf("a network ntfy effect must name the egress gate: %q", gated.Status)
	}
}
