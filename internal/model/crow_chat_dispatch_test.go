package model

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The Yard Crow chat is the single dispatch point (convergence keystone): a "/"-directive runs the command
// grammar. A wired governed DIRECTION verb (/focus) stages the WITNESSED apply-seam POST.
func TestCrowChatSlashDispatchesWiredVerb(t *testing.T) {
	m := New("REINS")
	m.Mode = ModeCoordChat
	m.WiredVerbs = map[string]bool{"focus": true}
	m.CoordChatInput = "/focus cc-task-x"
	nm, _ := m.updateCoordChat(tea.KeyMsg{Type: tea.KeyEnter})
	out := nm.(Model)
	if out.PendingCommand == nil || out.PendingCommand.Verb != "focus" || out.PendingCommand.Target != "cc-task-x" {
		t.Fatalf("/focus must dispatch focus through the apply seam, got %+v", out.PendingCommand)
	}
	if out.CoordChatInput != "" {
		t.Fatalf("dispatch must clear the chat input, got %q", out.CoordChatInput)
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
