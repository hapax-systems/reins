package model

import (
	"fmt"
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

// The imagery-glyph design forbids forking the disposition vocabulary: its `horseless` row binds the
// existing ○ StateAbsent rather than minting, "because minting would fork the disposition vocabulary
// (affordance.go)", and names "starved producer renders ▒ per dark != absent".
//
// grammar's silence tokens are that bind. This pins them EQUAL to what affordanceStateBadge renders,
// so the two cannot drift into two vocabularies for one meaning. If someone re-marks a state, this
// fails instead of letting the grammar quietly disagree with the badge.
func TestSilenceTokensMatchTheAffordanceBadge(t *testing.T) {
	for _, c := range []struct {
		state grammar.AffordanceState
		token string
	}{
		{grammar.StateDark, grammar.SilenceDark},
		{grammar.StateAbsent, grammar.SilenceAbsent},
	} {
		glyph, _, word := affordanceStateBadge(c.state)
		want := fmt.Sprintf("%s %s", glyph, word)
		if c.token != want {
			t.Errorf("state %q: grammar renders %q but the badge renders %q — the disposition "+
				"vocabulary has forked", c.state, c.token, want)
		}
	}
}

// The two silence tokens must stay distinguishable from each other in FORM, before colour — both
// carry the "mut" token, so colour cannot be the discriminator. This is dark != absent, mechanically.
func TestDarkAndAbsentDifferWithoutColour(t *testing.T) {
	dg, dtok, _ := affordanceStateBadge(grammar.StateDark)
	ag, atok, _ := affordanceStateBadge(grammar.StateAbsent)
	if dtok != atok {
		t.Skip("tokens differ; colour would carry it, but this test is about form")
	}
	if dg == ag {
		t.Fatalf("dark and absent share colour token %q AND glyph %q — indistinguishable in grayscale", dtok, dg)
	}
}
