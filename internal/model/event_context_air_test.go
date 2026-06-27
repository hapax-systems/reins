package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// eventContextPane's kinds-breakdown aggregates kinds across events; it must consult each event's
// per-field AIR so a denied kind is redacted in the breakdown, not leaked. (Latent bug surfaced by
// the always-split migration, which renders this pane at widths where it never rendered before.)
func TestEventContextPaneKindsBreakdownIsAirSafe(t *testing.T) {
	ev := grammar.Event{
		Kind: "secret.kind", Subject: "visible-subject",
		AIR: map[string]string{"kind": "deny", "subject": "ok"},
	}
	m := New("R").Fold([]grammar.Event{ev}, false)
	m.AIR = true
	out := ansi.Strip(m.eventContextPane(80))
	if strings.Contains(out, "secret.kind") {
		t.Fatalf("the kinds breakdown must not leak a denied kind on air:\n%s", out)
	}
}
