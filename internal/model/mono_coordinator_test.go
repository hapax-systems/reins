package model

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// N Reins views : 1 coordinator (operator 2026-06-28): multiple instances must NOT lead to multiple Yard
// coordinators / Hapax sessions — every instance shows the SAME backend sessions. Reins is a stateless
// VIEW-CLIENT: it spawns no coordinator on launch (no exec/daemon) and MINTS no session. The
// coordinating session is a backend singleton (CapabilityIO SESSION); the coord-chat send is a gated
// STUB that emits nothing — so no number of instances can create a coordinator. (On the E4.8 live
// activation the send must ATTACH to the one backend session, never spawn per-instance.)
func TestCoordSendMintsNoSessionOrCoordinator(t *testing.T) {
	m := New("REINS")
	m.Mode = ModeSendGate
	m.CoordChatInput = "dispatch the parity fix to lane-beta"
	before := len(m.Sessions)

	nm, cmd := m.updateSendGate(tea.KeyMsg{Type: tea.KeyEnter}) // explicit confirm
	mm := nm.(Model)

	if cmd != nil {
		t.Fatal("the coord send must emit NO tea.Cmd — no provider send, no session/coordinator spawn")
	}
	if len(mm.Sessions) != before {
		t.Fatalf("the send must mint NO session/coordinator: sessions %d → %d", before, len(mm.Sessions))
	}
	if !strings.Contains(mm.Status, "NOT wired") {
		t.Fatalf("the send must be the gated NOT-wired stub (mints nothing):\n%s", mm.Status)
	}
}
