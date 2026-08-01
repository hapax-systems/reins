package grammar

// AffordanceState is the ratified disposition vocabulary — the ONE closed set that says what a
// producer has told us about a subject. It is deliberately four values, not a bool and not a
// free-form string, because the two kinds of silence are different facts:
//
//	present — the producer emitted a fact and it is affirmative
//	hold    — the producer emitted a fact, qualified or withheld pending something
//	absent  — the producer LOOKED and classified it out: a positive "no", a measured negative
//	dark    — the producer emitted nothing at all: starved, not "no"
//
// dark != absent is the whole point (F4 / Gate-13). Collapsing them loses the difference between
// "we checked and there is none" and "we never heard", which is exactly the difference an operator
// needs in order to know whether to trust a blank cell.
//
// This type is the single source. encoder.go's SilenceDark/SilenceAbsent tokens and
// model.affordanceStateBadge both render FROM it, and TestSilenceTokensMatchTheAffordanceBadge pins
// them equal so the vocabulary cannot fork into two spellings of one meaning.
type AffordanceState string

const (
	// StatePresent: the producer emitted an affirmative fact for this subject.
	StatePresent AffordanceState = "present"
	// StateHold: emitted, but qualified or withheld pending something else.
	StateHold AffordanceState = "hold"
	// StateAbsent: the producer LOOKED and classified it out — a measured negative.
	StateAbsent AffordanceState = "absent"
	// StateDark: the producer emitted nothing — starved. NOT a "no".
	StateDark AffordanceState = "dark"
)
