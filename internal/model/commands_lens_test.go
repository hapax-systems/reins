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
	}, "absent", "verified", false)
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
	dark := Model{Width: 120}.FoldCommands(nil, "", "unknown", true)
	dark.CommandsError = "command ledger unreachable"
	if !strings.Contains(ansi.Strip(dark.renderCommandCatalog(120)), "dark") {
		t.Fatal("dark ledger must disclose, not render empty-as-fine")
	}
	// default enforcement is absent, never dark, on an empty-but-reachable ledger
	empty := Model{Width: 120}.FoldCommands(nil, "", "empty", false)
	out := ansi.Strip(empty.renderCommandCatalog(120))
	if !strings.Contains(out, "enforcement absent") || !strings.Contains(out, "no commands witnessed yet") {
		t.Fatalf("empty-reachable ledger not honest:\n%s", out)
	}
}

// On-air, an AIR-denied target must NOT appear in the rendered row (path-class §9 — the derived
// channel must never leak a command target). Pins the Go redaction path with on=true (H1/M1).
func TestCommandRowRedactsTargetOnAir(t *testing.T) {
	c := grammar.Command{
		Verb: "claim", Target: "/home/x/projects/secret-worktree", Status: "ok", Witness: "pending",
		AIR: map[string]string{"target": "deny", "task_id": "deny", "session_role": "deny"},
	}
	on := ansi.Strip(grammar.RenderCommandRow(c, true))
	if strings.Contains(on, "secret-worktree") {
		t.Fatalf("on-air command row leaks the denied target (path-class): %q", on)
	}
	// off air, the target is visible (redaction only bites on air).
	off := ansi.Strip(grammar.RenderCommandRow(c, false))
	if !strings.Contains(off, "secret-worktree") {
		t.Fatalf("off-air row should show the target: %q", off)
	}
}

// The witnessed ledger's INTEGRITY (the signed hash-chain verdict) renders in the header and is honest:
// "verified" reads affirmative, but a "broken:*"/"unsigned" verdict must SHOW (disclosed, never hidden)
// and must NOT read as verified — tamper-evidence you can't fake green (avsdlc-receipt-integrity).
func TestCommandsLensIntegrityBannerHonest(t *testing.T) {
	verified := Model{Width: 120}.FoldCommands([]grammar.Command{
		{Verb: "resume", Target: "lane-a", Status: "ok", Witness: "pending"},
	}, "absent", "verified", false)
	if !strings.Contains(ansi.Strip(verified.renderCommandCatalog(120)), "integrity verified") {
		t.Fatalf("verified integrity not surfaced in the ledger header")
	}
	// a broken chain must be disclosed and must NOT read verified
	broken := Model{Width: 120}.FoldCommands([]grammar.Command{
		{Verb: "resume", Target: "lane-a", Status: "ok", Witness: "pending"},
	}, "absent", "broken:hash-mismatch", false)
	bout := ansi.Strip(broken.renderCommandCatalog(120))
	if !strings.Contains(bout, "integrity broken:hash-mismatch") {
		t.Fatalf("broken integrity not disclosed:\n%s", bout)
	}
	if strings.Contains(bout, "integrity verified") {
		t.Fatalf("a broken chain must never read verified:\n%s", bout)
	}
	// a blank integrity defaults to "unknown", never verified
	if !strings.Contains(ansi.Strip(Model{Width: 120}.FoldCommands(nil, "absent", "", false).renderCommandCatalog(120)), "integrity unknown") {
		t.Fatalf("blank integrity must default to unknown, not verified")
	}
}

// no-display-scalar: the command lens must not render a fabricated numeric ranking/score for a command.
func TestCommandsLensNoScalar(t *testing.T) {
	m := Model{Width: 120}.FoldCommands([]grammar.Command{
		{Verb: "dispatch", Target: "lane", Status: "ok", Witness: "pending"},
	}, "absent", "verified", false)
	row := ansi.Strip(grammar.RenderCommandRow(m.Commands[0], false))
	// the row carries verb/target/status/witness — no bare 0..1 score token.
	for _, scalar := range []string{"0.", "score", "rank "} {
		if strings.Contains(strings.ToLower(row), scalar) {
			t.Fatalf("command row leaks a display scalar (%q): %q", scalar, row)
		}
	}
}
