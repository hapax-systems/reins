package model

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/grammar"
)

// composeParts assembles the egress parts for the send-gate from the staged text + the basket files.
// The text is the operator's typed message; each basket path becomes an image/file REFERENCE (the
// bytes/pixels would egress only AT send, which is stubbed). airProv is nil → default-deny, so paths
// + bodies redact on air through RenderInjectionComposer's interlock.
func (m Model) composeParts() []grammar.ChatPart {
	var parts []grammar.ChatPart
	if txt := strings.TrimSpace(m.CoordChatInput); txt != "" {
		parts = append(parts, grammar.TextPart(txt))
	}
	for _, p := range m.Basket {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(p), "."))
		if isImageExt(ext) {
			parts = append(parts, grammar.ImageRef(p, "image/"+ext, 0, nil))
		} else {
			parts = append(parts, grammar.FileRef(p, "", 0, nil))
		}
	}
	return parts
}

// updateSendGate is the egress SEND-GATE (ModeSendGate): the operator reviews the AIR-safe egress
// preview (RenderInjectionComposer — text bodies + paths redact on air, secrets surfaced off-air only)
// and EXPLICITLY confirms, dumps, or returns to compose. The send is a STUB — it emits/mints nothing
// (egress is ALWAYS-gated, separate from execution trust; a real provider send requires the governed
// CapabilityIO SESSION gate). There is no Enter-sends path: the gate is the only way out of a compose.
func (m Model) updateSendGate(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyName(v) {
	case "enter", "y": // explicit confirm → stage locally + the never-wired stub (NEVER a provider send)
		n := len(m.composeParts())
		if txt := strings.TrimSpace(m.CoordChatInput); txt != "" {
			m = m.AppendOperatorText(txt)
		}
		m.Status = fmt.Sprintf("send: would emit %d part(s) via the governed CapabilityIO SESSION gate — NOT wired (no provider send)", n)
		m.Mode, m.CoordChatInput, m.Basket = ModeNormal, "", nil
	case "d": // DUMP / kill — discard the composed message + basket, emit nothing
		m.Mode, m.CoordChatInput, m.Basket = ModeNormal, "", nil
		m.Status = "send: DUMPED — composed message + basket discarded (nothing emitted)"
	case "esc": // cancel → back to composing (the staged text + basket survive)
		m.Mode = ModeCoordChat
		m.Status = "send: back to compose"
	}
	return m, nil
}
