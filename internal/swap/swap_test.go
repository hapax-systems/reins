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

// A prev-tier resolution execs the prev generation's binary (rollback boot) with its own Handoff.
func TestResolveExecPlanPrevTier(t *testing.T) {
	store := generation.NewStore("/store")
	plan := ResolveExecPlan(store, generation.Resolution{SHA: "gen1", Tier: generation.TierPrev, Reason: "current quarantined -> prev"}, "/post.json", "nonce-p")
	if !plan.ShouldExec || !strings.Contains(plan.Argv[0], "gen1") || plan.Tier != generation.TierPrev {
		t.Fatalf("prev-tier should exec gen1: %+v", plan)
	}
	if plan.Handoff.TargetSHA != "gen1" || plan.Handoff.PosturePath != "/post.json" || plan.Handoff.Nonce != "nonce-p" {
		t.Fatalf("prev-tier Handoff must be pinned independently: %+v", plan.Handoff)
	}
}

// The exec gate is an ALLOWLIST: an unrecognized tier with a non-empty sha must NOT exec (defense in
// depth — safety cannot depend on Resolve() only ever emitting known tiers).
func TestResolveExecPlanUnknownTierNeverExecs(t *testing.T) {
	store := generation.NewStore("/store")
	plan := ResolveExecPlan(store, generation.Resolution{SHA: "x", Tier: "garbage", Reason: ""}, "/p", "n")
	if plan.ShouldExec {
		t.Fatal("an unrecognized tier MUST NOT exec (allowlist gate; unverified never boots)")
	}
	if plan.BreakglassReason == "" {
		t.Fatal("must surface a reason for refusing an unrecognized tier")
	}
}

// The API resolver execs the resolved generation's api under the given python + reports its serving sha.
func TestResolveAPIExecPlanVerified(t *testing.T) {
	store := generation.NewStore("/store")
	plan := ResolveAPIExecPlan(store, generation.Resolution{SHA: "genX", Tier: generation.TierCurrent}, "/venv/bin/python")
	if !plan.ShouldExec {
		t.Fatal("a verified resolution must serve")
	}
	if len(plan.Argv) != 2 || plan.Argv[0] != "/venv/bin/python" || !strings.HasSuffix(plan.Argv[1], "/store/generations/genX/api/reins_serve.py") {
		t.Fatalf("api argv should be [python <genX>/api/reins_serve.py], got %v", plan.Argv)
	}
	if !strings.HasSuffix(plan.APIDir, "/store/generations/genX/api") || plan.ServingSHA != "genX" {
		t.Fatalf("apiDir/servingSHA wrong: %+v", plan)
	}
}

// A breakglass/unknown-tier resolution must NOT serve (allowlist gate; systemd StartLimit -> API: FAILED).
func TestResolveAPIExecPlanBreakglassNeverServes(t *testing.T) {
	store := generation.NewStore("/store")
	for _, res := range []generation.Resolution{
		{SHA: "", Tier: generation.TierBreakglass, Reason: "no verified generation"},
		{SHA: "x", Tier: "garbage"},
	} {
		plan := ResolveAPIExecPlan(store, res, "/venv/bin/python")
		if plan.ShouldExec {
			t.Fatalf("must not serve for %+v", res)
		}
		if plan.BreakglassReason == "" {
			t.Fatal("must surface a reason")
		}
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

// A probation failure with NO prev is breakglass — AND quarantines the bad current so a naive restart
// doesn't re-select it in a loop.
func TestSupervisorProbationFailureNoPrevIsBreakglassAndQuarantines(t *testing.T) {
	a := DecideSupervisorAction(SupervisorState{
		ChildExitCode: 1, ConfirmSeen: false, WithinProbation: true, CurrentSHA: "new", PrevSHA: "",
	})
	if a.Kind != ActionBreakglass {
		t.Fatalf("probation failure with no prev must be breakglass, got %s", a.Kind)
	}
	if a.QuarantineSHA != "new" {
		t.Fatalf("the bad current must be quarantined even on no-prev breakglass (stop the re-select loop), got %q", a.QuarantineSHA)
	}
}

// The !ConfirmSeen guard MUST be isolated: confirm arrived, then the process died STILL WITHIN probation.
// This is a healthy generation that later crashed -> relaunch current, NEVER roll back. (A mutation that
// drops the confirm guard would wrongly roll back here — this is the load-bearing safety property.)
func TestSupervisorConfirmedInProbationDeathRelaunchesCurrent(t *testing.T) {
	a := DecideSupervisorAction(SupervisorState{
		ChildExitCode: 1, ConfirmSeen: true, WithinProbation: true, CurrentSHA: "new", PrevSHA: "old",
	})
	if a.Kind != ActionRelaunchCurrent || a.RelaunchSHA != "new" {
		t.Fatalf("a confirmed generation dying inside probation must relaunch current, not roll back: %+v", a)
	}
}

// The ExitCode guard MUST be isolated: a CLEAN exit inside probation before confirm is not a failure ->
// relaunch current, never roll back.
func TestSupervisorCleanExitInProbationRelaunchesCurrent(t *testing.T) {
	a := DecideSupervisorAction(SupervisorState{
		ChildExitCode: 0, ConfirmSeen: false, WithinProbation: true, CurrentSHA: "new", PrevSHA: "old",
	})
	if a.Kind != ActionRelaunchCurrent {
		t.Fatalf("a clean exit within probation must relaunch current, not roll back: %+v", a)
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
