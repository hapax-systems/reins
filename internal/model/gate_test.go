package model

import "testing"

// The gate verdicts pin the decision's 19-page classification (reins-design-ref):
// real-join pages stand, traces peeks, and the reference pages become honest-ambient DOORs — but the
// page is ALWAYS a split regardless of verdict (only-split).
func TestPageVerdictsMatchTheDecision(t *testing.T) {
	standing := []int{PageEvents, PageTasks, PageSessions, PageYard, PageReadiness, PageIntake, PageCaps, PageCoordinator, PageEpistemics, PageDynamics, PageIntent,
		PageLoops, PageAxes, PageIdentity, PageRelational, PageSessionTurns, PageDispatch}
	for _, p := range standing {
		if pageVerdict(p) != VerdictStanding {
			t.Fatalf("page %d should be STANDING", p)
		}
	}
	if pageVerdict(PageTraces) != VerdictPeek {
		t.Fatal("traces is a lane-scoped PEEK (live list darks; secondary is the rollup)")
	}
	for _, p := range []int{PageHelp, PageLegend, PageCommands, PageWindows, PageSurfaces, PageDomains, PageLifecycles} {
		if pageVerdict(p) != VerdictDoor {
			t.Fatalf("page %d is a reference DOOR (no real join — secondary is ambient context)", p)
		}
	}
}

func TestVerdictRatiosOrderPrimaryShare(t *testing.T) {
	// PEEK gives the primary the most (secondary a sliver); STANDING is the most balanced.
	if !(VerdictPeek.PrimaryRatio() > VerdictDoor.PrimaryRatio() && VerdictDoor.PrimaryRatio() > VerdictStanding.PrimaryRatio()) {
		t.Fatalf("ratios should order peek > door > standing for the primary share; got %v/%v/%v",
			VerdictPeek.PrimaryRatio(), VerdictDoor.PrimaryRatio(), VerdictStanding.PrimaryRatio())
	}
}
