package model

// Verdict is the split-legitimacy gate outcome (decision reins-design-ref).
// Under the operator's only-split ruling the COMPOSITION is always a split; the verdict decides what
// the SECONDARY pane contains and the ratio — so only-split and honesty co-hold:
//   - VerdictStanding: a REAL elucidate/emergent join → live brushed secondary, balanced.
//   - VerdictPeek:     a memory-bridgeable dependency → receded sliver, auto-surfaces on salience.
//   - VerdictDoor:     NO real join → the secondary is honest AMBIENT context (hotlist / scent /
//     breakdown-inbox / the focus's own DOI detail); the connector asserts NO join (never-mint).
type Verdict int

const (
	VerdictStanding Verdict = iota
	VerdictPeek
	VerdictDoor
)

// pageVerdict returns the gate verdict for a page — the decision's 19-page classification. The page
// is ALWAYS rendered as a split; this selects the secondary's materialization, never "split vs not".
func pageVerdict(page int) Verdict {
	switch page {
	case PageEvents, PageTasks, PageSessions, PageYard, PageReadiness, PageIntake,
		PageCaps, PageCoordinator, PageEpistemics, PageDynamics, PageIntent:
		return VerdictStanding
	case PageTraces:
		return VerdictPeek
	default: // help, legend, commands, windows, surfaces, domains, lifecycles
		return VerdictDoor
	}
}

// PrimaryRatio is the primary pane's share of the inner width for a verdict. STANDING is balanced;
// PEEK keeps the secondary a recede-able sliver; DOOR gives the primary the floor and the secondary
// an ambient-context margin.
func (v Verdict) PrimaryRatio() float64 {
	switch v {
	case VerdictStanding:
		return 0.5
	case VerdictPeek:
		return 0.82
	default: // VerdictDoor
		return 0.62
	}
}
