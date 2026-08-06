package model

import "github.com/hapax-systems/reins/internal/grammar"

// affordanceStateBadge renders a disposition as (glyph, colourToken, word).
//
// RECONSTRUCTED, not recovered. internal/model/affordance.go has never been committed to any ref in
// this repository, yet f4/0004's silence_binding_test.go and tier0/0002 both reference it — the patch
// that created it was lost. This restores the MINIMUM the staged patches need to build.
//
// What is PINNED by evidence, not chosen:
//   - dark  -> "▒" + "dark"   and absent -> "○" + "absent", because encoder.go declares
//     SilenceDark = "▒ dark" and SilenceAbsent = "○ absent" and states outright that they are
//     "the existing AffordanceState disposition vocabulary reused verbatim (affordance.go
//     StateDark/StateAbsent, rendered by model.affordanceStateBadge as ▒ dark / ○ absent)".
//     TestSilenceTokensMatchTheAffordanceBadge fails if this drifts.
//   - dark and absent must remain distinguishable WITHOUT colour. They share the "mut" token, so
//     the glyphs carry the difference. TestDarkAndAbsentDifferWithoutColour pins this.
//
// What is CHOSEN and not pinned by any test — review before landing:
//   - the glyphs ● / ◐ for present / hold (from the stated ●present ◐hold ○absent ▒dark set)
//   - the colour tokens "pri" for present and "yel" for hold. Both are existing tokens from this
//     grammar's set (mut/brt/pri/yel/2nd); nothing new is minted. But the specific assignment is a
//     reconstruction, and the original may have differed.
//
// The returned word is always the state's own string, so the vocabulary cannot fork by typo.
func affordanceStateBadge(s grammar.AffordanceState) (glyph, token, word string) {
	switch s {
	case grammar.StatePresent:
		return "●", "pri", string(grammar.StatePresent)
	case grammar.StateHold:
		return "◐", "yel", string(grammar.StateHold)
	case grammar.StateAbsent:
		return "○", "mut", string(grammar.StateAbsent)
	case grammar.StateDark:
		return "▒", "mut", string(grammar.StateDark)
	}
	// Fail closed: an unknown disposition renders as dark ("we never heard"), never as present.
	// Claiming presence on an unrecognised state would be the one unsafe direction.
	return "▒", "mut", string(grammar.StateDark)
}
