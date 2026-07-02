package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// U3b — the :commands page surfaces the WITNESSED LEDGER: live demand+verdict datoms + the enforcement
// cell. Honest states: dark discloses, empty says so, enforcement `absent` (never dark-conflated).
func TestCommandsLensRendersWitnessedLedger(t *testing.T) {
	m := Model{Width: 120, AIR: false}
	m = m.FoldCommands([]grammar.Command{
		{Verb: "claim", Target: "cc-task-x", Status: "not-wired", Witness: "pending", TaskID: "cc-task-x"},
		{Verb: "resume", Target: "lane-a", Status: "ok", Witness: "pending"},
	}, "absent", false)
	out := ansi.Strip(m.renderCommandCatalog(120))
	if !strings.Contains(out, "WITNESSED LEDGER") || !strings.Contains(out, "enforcement absent") {
		t.Fatalf("ledger header/enforcement missing:\n%s", out)
	}
	if !strings.Contains(out, "claim") || !strings.Contains(out, "not-wired") || !strings.Contains(out, "witness pending") {
		t.Fatalf("witnessed command rows not rendered:\n%s", out)
	}
	if !strings.Contains(out, "cc-task-x") {
		t.Fatalf("command datom task_id ref not rendered (brushable):\n%s", out)
	}
}

func TestCommandsLensHonestDarkAndEmpty(t *testing.T) {
	dark := Model{Width: 120}.FoldCommands(nil, "", true)
	dark.CommandsError = "command ledger unreachable"
	if !strings.Contains(ansi.Strip(dark.renderCommandCatalog(120)), "dark") {
		t.Fatal("dark ledger must disclose, not render empty-as-fine")
	}
	// default enforcement is absent, never dark, on an empty-but-reachable ledger
	empty := Model{Width: 120}.FoldCommands(nil, "", false)
	out := ansi.Strip(empty.renderCommandCatalog(120))
	if !strings.Contains(out, "enforcement absent") || !strings.Contains(out, "no commands witnessed yet") {
		t.Fatalf("empty-reachable ledger not honest:\n%s", out)
	}
}

// no-display-scalar: the command lens must not render a fabricated numeric ranking/score for a command.
func TestCommandsLensNoScalar(t *testing.T) {
	m := Model{Width: 120}.FoldCommands([]grammar.Command{
		{Verb: "dispatch", Target: "lane", Status: "ok", Witness: "pending"},
	}, "absent", false)
	row := ansi.Strip(grammar.RenderCommandRow(m.Commands[0], false))
	// the row carries verb/target/status/witness — no bare 0..1 score token.
	for _, scalar := range []string{"0.", "score", "rank "} {
		if strings.Contains(strings.ToLower(row), scalar) {
			t.Fatalf("command row leaks a display scalar (%q): %q", scalar, row)
		}
	}
}
