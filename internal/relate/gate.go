package relate

// Cardinality describes the multiplicity of a real join between two views.
type Cardinality string

const (
	CardOneToOne   Cardinality = "one-to-one"
	CardOneToMany  Cardinality = "one-to-many"
	CardManyToMany Cardinality = "many-to-many"
)

// Join is a declared connector shape the data may or may not honestly carry.
type Join struct {
	SrcKey string
	DstKey string
	Card   Cardinality
	Verb   string
}

// Verdict is the derivation gate's ruling for a split-legitimacy connector.
type Verdict int

const (
	VerdictDoor Verdict = iota
	VerdictPeek
	VerdictStanding
)

func (v Verdict) String() string {
	switch v {
	case VerdictDoor:
		return "Door"
	case VerdictPeek:
		return "Peek"
	case VerdictStanding:
		return "Standing"
	default:
		return "Verdict(unknown)"
	}
}

// Gate applies the Reins view-algebra derivation gate in the operator-defined order.
func Gate(j Join, realJoin bool, different bool, noBridge bool, emergent bool) Verdict {
	_ = j
	if !realJoin {
		return VerdictDoor
	}
	if emergent {
		return VerdictStanding
	}
	if different && noBridge {
		return VerdictStanding
	}
	return VerdictPeek
}

// JoinKeyFor returns the asserted join key for non-Door verdicts.
//
// The empty string on Door is the honesty floor: a Door connector must never assert a join the data
// does not carry.
func (j Join) JoinKeyFor(v Verdict) string {
	if v == VerdictDoor {
		return ""
	}
	return j.SrcKey + " -> " + j.DstKey
}

// Ratio returns the rendering ratio assigned to the verdict.
func (v Verdict) Ratio() float64 {
	if v == VerdictStanding {
		return 0.45
	}
	return 0.18
}
