package model

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The Yard Crow chat is the single dispatch point (convergence keystone). A wired governed verb does NOT
// fire on one Enter (voice-primary safety): it STAGES into ModeCommand with a preview; a second Enter
// confirms + dispatches through the witnessed apply seam.
func TestCrowChatSlashStagesWiredVerbForConfirm(t *testing.T) {
	m := New("REINS")
	m.Mode = ModeCoordChat
	m.WiredVerbs = map[string]bool{"focus": true}
	m.CoordChatInput = "/focus cc-task-x"
	nm, _ := m.updateCoordChat(tea.KeyMsg{Type: tea.KeyEnter})
	out := nm.(Model)
	if out.PendingCommand != nil {
		t.Fatal("a wired verb must NOT dispatch on the first Enter (needs confirm)")
	}
	if out.Mode != ModeCommand || out.Input != "focus cc-task-x" {
		t.Fatalf("wired verb must stage into ModeCommand with the target, got mode=%d input=%q", out.Mode, out.Input)
	}
	if out.CoordChatInput != "" {
		t.Fatalf("staging must clear the chat input, got %q", out.CoordChatInput)
	}
	// the SECOND Enter (now in ModeCommand) confirms + dispatches
	confirmed := out.Exec(out.Input)
	if confirmed.PendingCommand == nil || confirmed.PendingCommand.Verb != "focus" || confirmed.PendingCommand.Target != "cc-task-x" {
		t.Fatalf("confirm must dispatch focus through the apply seam, got %+v", confirmed.PendingCommand)
	}
}

// App-lifecycle verbs (/q, /swap) are NOT reachable from the chat dispatch surface — a mis-transcribed
// voice "/q" must not quit the cockpit.
func TestCrowChatSlashExcludesLifecycleVerbs(t *testing.T) {
	for _, v := range []string{"/q", "/quit", "/swap"} {
		m := New("REINS")
		m.Mode = ModeCoordChat
		m.CoordChatInput = v
		nm, _ := m.updateCoordChat(tea.KeyMsg{Type: tea.KeyEnter})
		out := nm.(Model)
		if out.PendingCommand != nil || out.Mode == ModeCommand {
			t.Fatalf("%s must not dispatch or stage from the chat, got mode=%d pending=%v", v, out.Mode, out.PendingCommand)
		}
	}
}

// An UNWIRED verb from the chat previews only (never-mint) — no fabricated dispatch.
func TestCrowChatSlashUnwiredPreviewsOnly(t *testing.T) {
	m := New("REINS") // WiredVerbs empty
	m.Mode = ModeCoordChat
	m.CoordChatInput = "/focus cc-task-x"
	nm, _ := m.updateCoordChat(tea.KeyMsg{Type: tea.KeyEnter})
	if nm.(Model).PendingCommand != nil {
		t.Fatal("an unwired verb from the chat must NOT stage a POST (preview only)")
	}
}

// Plain text (no "/") stays the steer COMPOSER — opens the send-gate, never dispatches.
func TestCrowChatPlainTextComposesNotDispatches(t *testing.T) {
	m := New("REINS")
	m.Mode = ModeCoordChat
	m.WiredVerbs = map[string]bool{"focus": true}
	m.CoordChatInput = "steer toward the parity fix"
	nm, _ := m.updateCoordChat(tea.KeyMsg{Type: tea.KeyEnter})
	out := nm.(Model)
	if out.PendingCommand != nil {
		t.Fatal("plain steer text must NOT dispatch a command")
	}
	if out.Mode != ModeSendGate {
		t.Fatalf("plain non-empty compose must open the send-gate, got mode %d", out.Mode)
	}
}

// A "/"-navigation verb switches views (not a governed dispatch).
func TestCrowChatSlashNavigationSwitchesPage(t *testing.T) {
	m := New("REINS")
	m.Mode = ModeCoordChat
	m.CoordChatInput = "/tasks"
	nm, _ := m.updateCoordChat(tea.KeyMsg{Type: tea.KeyEnter})
	out := nm.(Model)
	if out.Page != PageTasks {
		t.Fatalf("/tasks from the chat must switch to the tasks page, got page %d", out.Page)
	}
	if out.PendingCommand != nil {
		t.Fatal("a navigation verb must not stage a command POST")
	}
}

// The dispatch hint honestly lists only the WIRED verbs (discoverability for the Crow chat "/" surface).
func TestDispatchHintListsWiredOnly(t *testing.T) {
	m := New("REINS")
	m.WiredVerbs = map[string]bool{"focus": true, "resume": true} // stage/close not wired
	h := m.dispatchHint()
	if !strings.Contains(h, "/focus") || !strings.Contains(h, "/resume") {
		t.Fatalf("wired verbs must appear, got %q", h)
	}
	if strings.Contains(h, "/close") || strings.Contains(h, "/stage") {
		t.Fatalf("unwired verbs must NOT be listed as dispatch-ready, got %q", h)
	}
}

func TestDispatchHintNothingWired(t *testing.T) {
	if h := New("REINS").dispatchHint(); !strings.Contains(h, "no verbs wired") {
		t.Fatalf("empty wired set must say so (honest), got %q", h)
	}
}

func TestDispatchHintExcludesNonDispatchableStage(t *testing.T) {
	// stage is wired server-side but NOT in governedVerbSpecs -> the chat cannot dispatch it, so the hint
	// must not advertise it (else the operator hits "unknown command").
	m := New("REINS")
	m.WiredVerbs = map[string]bool{"focus": true, "stage": true}
	h := m.dispatchHint()
	if strings.Contains(h, "/stage") {
		t.Fatalf("hint must not advertise the non-dispatchable /stage, got %q", h)
	}
	if !strings.Contains(h, "/focus") {
		t.Fatalf("hint must still advertise the dispatchable /focus, got %q", h)
	}
}

func TestCoordChatInputSealsGovernedTargetOnAir(t *testing.T) {
	// on air, a "/"-directive's governed TARGET is sealed (mirrors commandInputDisplay), verb kept.
	m := New("REINS")
	m.AIR = true
	m.CoordChatInput = "/focus cc-task-secret"
	got := m.coordChatInputDisplay()
	if strings.Contains(got, "cc-task-secret") {
		t.Fatalf("governed target must be sealed on air, got %q", got)
	}
	if !strings.Contains(got, "/focus") {
		t.Fatalf("verb must survive (structural), got %q", got)
	}
}
