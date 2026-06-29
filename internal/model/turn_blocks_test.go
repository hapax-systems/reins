package model

import (
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

// E4.3 detail-block consumer: the focused turn's blocks store under its FULL TurnID (ts|role|kind, so
// turns sharing ts+role with different kind never mis-assign); empty/honest-empty fabricates nothing.
func TestTurnBlocksMsgStoresUnderFullTurnIDElseHonestEmpty(t *testing.T) {
	m := New("R")
	m.TurnLadder = []grammar.Turn{{TS: "t0", Role: "cx-a", Kind: "tool_call"}}
	m.TurnFocus = 0

	role, ts, id, ok := m.FocusedTurnRef()
	if !ok || role != "cx-a" || ts != "t0" || id != "t0|cx-a|tool_call" {
		t.Fatalf("FocusedTurnRef wrong: role=%q ts=%q id=%q ok=%v", role, ts, id, ok)
	}

	nm, _ := m.Update(TurnBlocksMsg{TurnID: id, Blocks: []grammar.TurnBlock{{}}})
	if got := len(nm.(Model).TurnBlocks[id]); got != 1 {
		t.Fatalf("non-empty blocks must store under the TurnID, got %d", got)
	}

	nm2, _ := m.Update(TurnBlocksMsg{TurnID: id, Blocks: nil})
	if got := len(nm2.(Model).TurnBlocks[id]); got != 0 {
		t.Fatalf("honest-empty blocks must not fabricate an entry, got %d", got)
	}
}
