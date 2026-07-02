package swap

import (
	"strings"
	"testing"

	"github.com/hapax-systems/reins/internal/generation"
)

// A verified resolution -> exec the resolved generation's binary with --resume + a consume-once Handoff.
func TestResolveExecPlanVerified(t *testing.T) {
	store := generation.NewStore("/store")
	plan := ResolveExecPlan(store, generation.Resolution{SHA: "gen2", Tier: generation.TierCurrent, Reason: "current verified"}, "/posture.json", "nonce-1")
	if !plan.ShouldExec {
		t.Fatal("a verified resolution must exec")
	}
	if len(plan.Argv) != 2 || !strings.HasSuffix(plan.Argv[0], "/store/generations/gen2/reins") || plan.Argv[1] != "--resume" {
		t.Fatalf("argv should be <gen2 binary> --resume, got %v", plan.Argv)
	}
	if plan.Handoff.TargetSHA != "gen2" || plan.Handoff.PosturePath != "/posture.json" || plan.Handoff.Nonce != "nonce-1" {
		t.Fatalf("handoff wrong: %+v", plan.Handoff)
	}
}

// A breakglass resolution -> exec NOTHING (never boot an unverified generation) + surface the reason.
func TestResolveExecPlanBreakglassNeverExecs(t *testing.T) {
	store := generation.NewStore("/store")
	plan := ResolveExecPlan(store, generation.Resolution{SHA: "", Tier: generation.TierBreakglass, Reason: "no verified generation"}, "/p", "n")
	if plan.ShouldExec {
		t.Fatal("breakglass MUST NOT exec — an unverified generation must never boot")
	}
	if plan.BreakglassReason == "" {
		t.Fatal("breakglass must surface an honest manual-recovery reason")
	}
}

// A prev-tier resolution execs the prev generation's binary (rollback boot).
func TestResolveExecPlanPrevTier(t *testing.T) {
	store := generation.NewStore("/store")
	plan := ResolveExecPlan(store, generation.Resolution{SHA: "gen1", Tier: generation.TierPrev, Reason: "current quarantined -> prev"}, "/p", "n")
	if !plan.ShouldExec || !strings.Contains(plan.Argv[0], "gen1") || plan.Tier != generation.TierPrev {
		t.Fatalf("prev-tier should exec gen1: %+v", plan)
	}
}

// The ONLY rollback trigger is a probation failure: nonzero exit, before confirm, within probation.
func TestSupervisorProbationFailureRollsBack(t *testing.T) {
	a := DecideSupervisorAction(SupervisorState{
		ChildExitCode: 1, ConfirmSeen: false, WithinProbation: true, CurrentSHA: "new", PrevSHA: "old",
	})
	if a.Kind != ActionQuarantineAndRollback {
		t.Fatalf("pre-confirm in-probation failure must quarantine+rollback, got %s", a.Kind)
	}
	if a.QuarantineSHA != "new" || a.RelaunchSHA != "old" || !a.WithResume {
		t.Fatalf("should quarantine new + relaunch old --resume: %+v", a)
	}
}

// A probation failure with NO prev is breakglass (nothing safe to roll back to).
func TestSupervisorProbationFailureNoPrevIsBreakglass(t *testing.T) {
	a := DecideSupervisorAction(SupervisorState{
		ChildExitCode: 1, ConfirmSeen: false, WithinProbation: true, CurrentSHA: "new", PrevSHA: "",
	})
	if a.Kind != ActionBreakglass {
		t.Fatalf("probation failure with no prev must be breakglass, got %s", a.Kind)
	}
}

// A confirmed child that later dies -> relaunch current, NOT a rollback (not a swap failure).
func TestSupervisorConfirmedThenDiesRelaunchesCurrent(t *testing.T) {
	a := DecideSupervisorAction(SupervisorState{
		ChildExitCode: 1, ConfirmSeen: true, WithinProbation: false, CurrentSHA: "new", PrevSHA: "old",
	})
	if a.Kind != ActionRelaunchCurrent || a.RelaunchSHA != "new" {
		t.Fatalf("a post-confirm death must relaunch current (not roll back), got %+v", a)
	}
}

// A nonzero exit AFTER the probation window (even without confirm) is not attributed to the swap ->
// relaunch current, not rollback (a long-lived generation that eventually crashed).
func TestSupervisorPostProbationFailureRelaunchesCurrent(t *testing.T) {
	a := DecideSupervisorAction(SupervisorState{
		ChildExitCode: 1, ConfirmSeen: false, WithinProbation: false, CurrentSHA: "new", PrevSHA: "old",
	})
	if a.Kind != ActionRelaunchCurrent {
		t.Fatalf("a post-probation failure must relaunch current (not roll back), got %s", a.Kind)
	}
}

// A clean exit relaunches current with --resume.
func TestSupervisorCleanExitRelaunchesCurrent(t *testing.T) {
	a := DecideSupervisorAction(SupervisorState{
		ChildExitCode: 0, ConfirmSeen: true, WithinProbation: false, CurrentSHA: "cur", PrevSHA: "old",
	})
	if a.Kind != ActionRelaunchCurrent || a.RelaunchSHA != "cur" || !a.WithResume {
		t.Fatalf("clean exit should relaunch current --resume: %+v", a)
	}
}
