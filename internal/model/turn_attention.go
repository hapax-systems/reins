package model

import (
	"fmt"

	"github.com/hapax-systems/reins/internal/grammar"
)

// turnAttention scores how much a turn NEEDS THE OPERATOR (E4.9 synergy 2 — scored attention across the
// ladder; "the one turn needing the operator surfaces"). Inputs are SKELETON-only (gate/kind air by
// construction), so it is AIR-safe: a per-turn DENIED gate/kind degrades that signal to 0 — no false
// alarm, no derived-channel leak. Returns the score and a short, airable reason.
func (m Model) turnAttention(t grammar.Turn) (int, string) {
	gateOK := !m.AIR || t.AIR["gate"] == "ok"
	kindOK := !m.AIR || t.AIR["kind"] == "ok"
	score, why := 0, ""
	if gateOK && t.Gate == "deny" {
		score, why = score+3, "gate DENY"
	}
	if kindOK {
		switch t.Kind {
		case "approval":
			score += 3
			if why == "" {
				why = "approval awaiting"
			}
		case "interrupt":
			score += 2
			if why == "" {
				why = "interrupted"
			}
		case "refusal":
			score += 2
			if why == "" {
				why = "refusal"
			}
		}
	}
	if gateOK && score == 0 && t.Gate == "pending" {
		score, why = 1, "gate pending"
	}
	return score, why
}

// turnTopAttention finds the highest-attention turn in the ladder (the one the operator most needs). The
// first such turn wins ties (chronological priority). idx == -1 when nothing needs attention.
func (m Model) turnTopAttention() (idx, score int, why string) {
	idx = -1
	for i := range m.TurnLadder {
		if s, w := m.turnAttention(m.TurnLadder[i]); s > score {
			idx, score, why = i, s, w
		}
	}
	return idx, score, why
}

// turnAttentionPointer surfaces the top-attention turn even when it is scrolled OUT of the visible window
// [start, start+visible): the "one turn that needs you" never hides below the fold. Returns "" when
// nothing needs attention (the equipment recedes — conditions appear only when abnormal).
func (m Model) turnAttentionPointer(w, start, visible int) string {
	idx, score, why := m.turnTopAttention()
	if score == 0 || idx < 0 {
		return ""
	}
	loc := "in view"
	switch {
	case idx < start:
		loc = fmt.Sprintf("↑ %d above", start-idx)
	case idx >= start+visible:
		loc = fmt.Sprintf("↓ %d below", idx-(start+visible)+1)
	}
	head := grammar.C("yel", " ‼ ATTENTION") + grammar.C("mut", " — turn needs you: ") +
		grammar.C("brt", why) + grammar.C("mut", " · "+loc)
	return fitWidth(head, w)
}
