package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// U1 A3.1d — a reachable non-reins port renders PORT: FOREIGN SERVER on the title bar; a
// reachable reins port and an unreachable (honest-dark) port do NOT.
func TestForeignServerIsARenderedState(t *testing.T) {
	base := Model{Width: 160, Height: 40, Title: "reins"}

	foreign := base
	foreign = mustFold(foreign, MetaMsg{Reachable: true, Foreign: true})
	if !foreign.PortForeign {
		t.Fatal("reachable non-reins port must set PortForeign")
	}
	if !strings.Contains(ansi.Strip(foreign.viewTitle(160)), "PORT: FOREIGN SERVER") {
		t.Fatalf("foreign port not rendered on the title bar:\n%s", ansi.Strip(foreign.viewTitle(160)))
	}

	ok := base
	ok = mustFold(ok, MetaMsg{Reachable: true, Foreign: false, App: "reins", ServingSHA: "abc123"})
	if ok.PortForeign {
		t.Fatal("a reins port must NOT be foreign")
	}
	if ok.ServingSHA != "abc123" {
		t.Fatalf("serving sha not folded: %q", ok.ServingSHA)
	}
	if strings.Contains(ansi.Strip(ok.viewTitle(160)), "FOREIGN") {
		t.Fatal("reins port must not render the foreign state")
	}

	dark := base
	dark = mustFold(dark, MetaMsg{Reachable: false, Foreign: false})
	if dark.PortForeign {
		t.Fatal("an unreachable port is honest-dark, NOT foreign (only reachable non-reins is foreign)")
	}
}

func mustFold(m Model, msg MetaMsg) Model {
	nm, _ := m.Update(msg)
	return nm.(Model)
}
