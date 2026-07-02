package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// The witnessed dispatch loop closes IN the Yard Crow chat: the demand→verdict ledger tail renders as
// witness turns in coordinatorChatPane, so the operator sees the consequence where they issued it.

func chatWith(cmds []grammar.Command) Model {
	m := New("REINS")
	m.Mode = ModeCoordChat
	m.Commands = cmds
	return m
}

// (a) a witnessed governed dispatch renders in the chat with its verb + verdict.
func TestChatRendersWitnessedDispatch(t *testing.T) {
	m := chatWith([]grammar.Command{{Verb: "focus", Target: "cc-task-x", Status: "ok", Witness: "pending"}})
	out := ansi.Strip(m.coordinatorChatPane(140, 24))
	if !strings.Contains(out, "witnessed frontdoor dispatches") {
		t.Fatalf("chat must show the witness block, got:\n%s", out)
	}
	if !strings.Contains(out, "focus") || !strings.Contains(out, "✓") {
		t.Fatalf("witnessed /focus must render verb + applied glyph, got:\n%s", out)
	}
}

// (f) REQUIRED false-green case: a PREVIEW verb (resume) must read as previewed, NEVER applied.
func TestChatPreviewVerbNeverReadsApplied(t *testing.T) {
	m := chatWith([]grammar.Command{{Verb: "resume", Target: "lane-a", Status: "ok", Witness: "pending"}})
	m.VerbModes = map[string]string{"resume": "preview"}
	out := ansi.Strip(m.coordinatorChatPane(140, 24))
	if !strings.Contains(out, "previewed (no write)") {
		t.Fatalf("preview /resume must read 'previewed (no write)', got:\n%s", out)
	}
	if strings.Contains(out, "✓") {
		t.Fatalf("preview /resume must NEVER render ✓ applied (false-green), got:\n%s", out)
	}
}

// the same honesty holds on the :commands page (the latent bug the design surfaced).
func TestCommandsPagePreviewVerbNeverReadsApplied(t *testing.T) {
	m := chatWith([]grammar.Command{{Verb: "resume", Target: "lane-a", Status: "ok", Witness: "pending"}})
	m.VerbModes = map[string]string{"resume": "preview"}
	out := ansi.Strip(m.renderCommandCatalog(120))
	if strings.Contains(out, "✓") || !strings.Contains(out, "previewed (no write)") {
		t.Fatalf(":commands must not false-green a preview verb, got:\n%s", out)
	}
}

// (b) dark ledger → honest disclosure, no fabricated verdict.
func TestChatWitnessDark(t *testing.T) {
	m := chatWith(nil)
	m.CommandsDark, m.CommandsError = true, "command ledger unreachable"
	out := ansi.Strip(m.coordinatorChatPane(140, 24))
	if !strings.Contains(out, "dark") || strings.Contains(out, "✓") {
		t.Fatalf("dark ledger must disclose dark + fabricate no verdict, got:\n%s", out)
	}
}

// (c) empty ledger → structured silence (no witness block, no fabricated rows).
func TestChatWitnessEmptySilent(t *testing.T) {
	out := ansi.Strip(chatWith(nil).coordinatorChatPane(140, 24))
	if strings.Contains(out, "witnessed frontdoor dispatches") {
		t.Fatalf("an empty ledger must NOT show a witness block (structured silence), got:\n%s", out)
	}
}

// (d) AIR: a denied target is sealed in the witness row; the verb/status skeleton survives.
func TestChatWitnessAirSealsTarget(t *testing.T) {
	m := chatWith([]grammar.Command{{
		Verb: "focus", Target: "cc-task-secret", Status: "ok", Witness: "pending",
		AIR: map[string]string{"verb": "ok", "target": "deny", "status": "ok"},
	}})
	m.AIR = true
	out := ansi.Strip(m.coordinatorChatPane(140, 24))
	if strings.Contains(out, "cc-task-secret") {
		t.Fatalf("denied target must be sealed on air, got:\n%s", out)
	}
	if !strings.Contains(out, "focus") {
		t.Fatalf("verb skeleton must survive AIR, got:\n%s", out)
	}
}

// (e) handheld: the input line AND the lens {{sel}} grounding turn survive even with witness rows present.
func TestChatHandheldKeepsInputAndGrounding(t *testing.T) {
	m := chatWith([]grammar.Command{
		{Verb: "focus", Target: "a", Status: "ok"}, {Verb: "breakglass", Target: "b", Status: "ok"},
		{Verb: "stage", Target: "c", Status: "ok"}, {Verb: "focus", Target: "d", Status: "ok"},
	})
	// a focused task seeds the {{sel}} lens grounding turn
	m.Tasks = []grammar.Task{{TaskID: "cc-focus-me", Stage: "S6"}}
	m.Focus = 0
	out := ansi.Strip(m.coordinatorChatPane(120, 12)) // small handheld height
	if !strings.Contains(out, "›") {
		t.Fatalf("the input prompt must survive at handheld height, got:\n%s", out)
	}
	if !strings.Contains(out, "{{sel}}") {
		t.Fatalf("the lens grounding turn must survive (never evicted by witness rows), got:\n%s", out)
	}
}
