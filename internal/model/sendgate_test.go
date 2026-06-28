package model

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// The egress send-gate preview is AIR-safe by construction (it reuses RenderInjectionComposer, which
// redacts text bodies + file paths on air) — the operator's compose never leaks on the livestream.
func TestSendGatePreviewRedactsOnAir(t *testing.T) {
	m := New("REINS")
	m.Width, m.Height, m.Page, m.AIR = 200, 30, PageCoordinator, true
	m.CoordChatInput = "ship the secret patch"
	m.Basket = []string{"/srv/secret/medical-results.png"}
	m.Mode = ModeSendGate
	on := ansi.Strip(m.coordinatorChatPane(130, 26))
	for _, leak := range []string{"medical-results", "/srv/secret", "ship the secret patch"} {
		if strings.Contains(on, leak) {
			t.Fatalf("on-air send-gate preview leaked %q:\n%s", leak, on)
		}
	}
	if !strings.Contains(on, "NOT WIRED") {
		t.Fatalf("the send-gate must show the always-gated never-wired egress:\n%s", on)
	}
}

// The send is a STUB (no provider send) reachable ONLY through an explicit confirm; dump discards
// everything; esc returns to compose with the staged content intact.
func TestSendGateConfirmStubsDumpDiscardsEscReturns(t *testing.T) {
	send := func(m Model, v tea.KeyMsg) Model { nm, _ := m.Update(v); return nm.(Model) }
	base := func() Model {
		m := New("REINS")
		m.Width, m.Height, m.Page = 200, 30, PageCoordinator
		m.CoordChatInput = "ship it"
		m.Basket = []string{"/srv/x/a.png"}
		m.Mode = ModeSendGate
		return m
	}
	// confirm → stages locally + the NOT-wired stub + clears; NEVER an actual provider send
	m := send(base(), tea.KeyMsg{Type: tea.KeyEnter})
	if m.Mode != ModeNormal || len(m.CoordChatLog) != 1 || len(m.Basket) != 0 {
		t.Fatalf("confirm must stage + clear; mode=%d log=%d basket=%d", m.Mode, len(m.CoordChatLog), len(m.Basket))
	}
	if !strings.Contains(m.Status, "NOT wired") {
		t.Fatalf("confirm must be a never-wired stub; got %q", m.Status)
	}
	// dump → discard everything, emit nothing
	m = send(base(), tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if m.Mode != ModeNormal || len(m.CoordChatLog) != 0 || len(m.Basket) != 0 || m.CoordChatInput != "" {
		t.Fatalf("dump must discard the compose entirely; mode=%d log=%d basket=%d input=%q",
			m.Mode, len(m.CoordChatLog), len(m.Basket), m.CoordChatInput)
	}
	// esc → back to compose, the staged text + basket survive
	m = send(base(), tea.KeyMsg{Type: tea.KeyEsc})
	if m.Mode != ModeCoordChat || m.CoordChatInput != "ship it" || len(m.Basket) != 1 {
		t.Fatalf("esc must return to compose with content intact; mode=%d input=%q basket=%d", m.Mode, m.CoordChatInput, len(m.Basket))
	}
}
