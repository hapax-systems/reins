package model

import (
	"strings"
	"testing"
)

// A WIRED governed verb stages a witnessed POST (PendingCommand) — the apply seam. main.go issues it.
func TestExecWiredGovernedVerbStagesPost(t *testing.T) {
	m := New("REINS")
	m.WiredVerbs = map[string]bool{"close": true}
	out := m.Exec("close cc-task-x")
	if out.PendingCommand == nil {
		t.Fatal("a wired governed verb must stage a PendingCommand (the apply seam)")
	}
	if out.PendingCommand.Verb != "close" || out.PendingCommand.Target != "cc-task-x" {
		t.Fatalf("PendingCommand wrong: %+v", out.PendingCommand)
	}
	if !strings.Contains(out.Status, "applying") {
		t.Fatalf("wired apply should show 'applying', got %q", out.Status)
	}
}

// An UNWIRED governed verb renders the never-mint PREVIEW and stages NOTHING (no fabricated apply,
// A3.12a: one surface, the starved VerbSpec).
func TestExecUnwiredGovernedVerbPreviewsOnly(t *testing.T) {
	m := New("REINS") // WiredVerbs empty -> nothing wired
	out := m.Exec("close cc-task-x")
	if out.PendingCommand != nil {
		t.Fatal("an unwired governed verb must NOT stage a POST (preview only)")
	}
	if strings.Contains(out.Status, "applying") {
		t.Fatalf("unwired verb must not claim to apply, got %q", out.Status)
	}
}

// The verdict fold is honest across ok / refused / unreachable — never a fabricated success.
func TestFoldCommandVerdictHonest(t *testing.T) {
	ok := Model{}.FoldCommandVerdict(CommandVerdictMsg{Verb: "resume", Status: "ok", EventID: "abcdef012345deadbeef", Reachable: true})
	if !strings.Contains(ok.Status, "✓") || !strings.Contains(ok.Status, "witnessed") || !strings.Contains(ok.Status, "abcdef012345") {
		t.Fatalf("ok verdict must show applied+witnessed+eventid: %q", ok.Status)
	}
	refused := Model{}.FoldCommandVerdict(CommandVerdictMsg{Verb: "dispatch", Status: "not-wired", Reason: "no ungated path", Reachable: true})
	if !strings.Contains(refused.Status, "✖") || strings.Contains(refused.Status, "applied +") || !strings.Contains(refused.Status, "not-wired") {
		t.Fatalf("refusal must be honest (no applied), got %q", refused.Status)
	}
	dead := Model{}.FoldCommandVerdict(CommandVerdictMsg{Verb: "close", Status: "unreachable", Reason: "conn refused", Reachable: false})
	if !strings.Contains(dead.Status, "UNREACHABLE") || !strings.Contains(dead.Status, "nothing applied") {
		t.Fatalf("unreachable must disclose nothing applied, got %q", dead.Status)
	}
	replay := Model{}.FoldCommandVerdict(CommandVerdictMsg{Verb: "close", Status: "idempotent-replay", Reachable: true})
	if !strings.Contains(replay.Status, "already applied") || !strings.Contains(replay.Status, "not re-run") {
		t.Fatalf("idempotent-replay must read as already-applied, got %q", replay.Status)
	}
}

// A governed verb with no target + no focused task is refused (no accidental empty-target POST).
func TestExecGovernedVerbNeedsTarget(t *testing.T) {
	m := New("REINS")
	m.WiredVerbs = map[string]bool{"close": true}
	out := m.Exec("close")
	if out.PendingCommand != nil {
		t.Fatal("no target -> must not stage a POST")
	}
	if !strings.Contains(out.Status, "no target") {
		t.Fatalf("expected a no-target refusal, got %q", out.Status)
	}
}
