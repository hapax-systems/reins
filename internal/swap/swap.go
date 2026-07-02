// Package swap is the PURE decision core of the hot-plug live swap (U6b-swap logic, design pack §Design 1
// A1.11). It answers two questions with no side effects:
//
//   - ResolveExecPlan: given the generation store's rollback Resolution, WHICH binary should the cockpit
//     hand over to, with what argv + consume-once Handoff — or is this breakglass (exec nothing, surface
//     manual recovery)?
//   - DecideSupervisorAction: given a child's exit + probation state, what should the reins-run crash
//     BACKSTOP do — quarantine the current generation and relaunch prev, or relaunch current?
//
// This package performs NO syscall.Exec, spawns no process, touches no systemd unit, and imports nothing
// effectful — it only computes plans. The effectful wiring (the actual syscall.Exec in cmd/reins, the
// reins-run main, the reins-api.service ExecStart repoint) is the deliberate U6b-deploy step, which wraps
// these tested functions. Isolating the decision here means the risky live swap is built on a proven core.
package swap

import (
	"path/filepath"

	"github.com/hapax-systems/reins/internal/generation"
)

// ExecPlan is the resolved hand-over decision. ShouldExec is false for a breakglass resolution — the
// cockpit must NEVER exec an unverified generation; it surfaces BreakglassReason for manual recovery.
type ExecPlan struct {
	ShouldExec       bool
	Argv             []string // the resolved binary + flags (e.g. --resume) — argv[0] is the binary path
	Handoff          generation.Handoff
	Tier             string // which rollback tier was chosen (current|prev|breakglass) — for disclosure
	BreakglassReason string // set iff !ShouldExec
}

// ResolveExecPlan turns a store Resolution into a hand-over plan. A verified current/prev resolution
// execs the resolved generation's `reins` binary with --resume (so the restored posture reloads) and
// writes a consume-once Handoff naming the posture + target sha. A breakglass resolution (no verified
// generation) execs NOTHING — an unverified generation must never boot.
func ResolveExecPlan(store *generation.Store, res generation.Resolution, posturePath, nonce string) ExecPlan {
	if res.Tier == generation.TierBreakglass || res.SHA == "" {
		return ExecPlan{
			ShouldExec:       false,
			Tier:             generation.TierBreakglass,
			BreakglassReason: res.Reason,
		}
	}
	binary := filepath.Join(store.GenerationDir(res.SHA), "reins")
	return ExecPlan{
		ShouldExec: true,
		Argv:       []string{binary, "--resume"},
		Handoff:    generation.Handoff{PosturePath: posturePath, TargetSHA: res.SHA, Nonce: nonce},
		Tier:       res.Tier,
	}
}

// SupervisorState is what the reins-run crash-backstop observes about a finished child run.
type SupervisorState struct {
	ChildExitCode   int    // the cockpit child's exit code (0 = clean)
	ConfirmSeen     bool   // did the child write its probation confirm-file? (it booted far enough to be healthy)
	WithinProbation bool   // did the child exit WITHIN the probation window?
	CurrentSHA      string // the store's current generation
	PrevSHA         string // the store's prev generation (recovery target)
}

// Supervisor action kinds.
const (
	ActionRelaunchCurrent      = "relaunch-current"       // normal: relaunch current with --resume
	ActionQuarantineAndRollback = "quarantine-and-rollback" // bad new generation: quarantine + relaunch prev
	ActionBreakglass           = "breakglass"              // nothing safe to relaunch — manual recovery
)

// SupervisorAction is the crash-backstop's decision. It NEVER initiates an upgrade (the supervisor is a
// backstop only — swaps are operator/cockpit-initiated via exec-handover).
type SupervisorAction struct {
	Kind          string
	RelaunchSHA   string // the generation to relaunch (current or prev)
	QuarantineSHA string // set iff Kind == quarantine-and-rollback
	WithResume    bool   // relaunch with --resume (restore the externalized posture)
	Reason        string
}

// DecideSupervisorAction is the reins-run crash-backstop decision. The ONLY failure that triggers a
// rollback is a probation failure: the child exited NONZERO, BEFORE writing its confirm-file, WITHIN the
// probation window — i.e. the freshly-swapped generation could not even come up healthy. Then the current
// generation is quarantined (never exec it again) and prev is relaunched. Every other outcome (clean
// exit, or a failure after the confirm / after probation — a healthy generation that later died) simply
// relaunches current with --resume. If there is no prev to roll back to, it is breakglass.
func DecideSupervisorAction(s SupervisorState) SupervisorAction {
	probationFailure := s.ChildExitCode != 0 && !s.ConfirmSeen && s.WithinProbation
	if probationFailure {
		if s.PrevSHA == "" {
			return SupervisorAction{
				Kind:   ActionBreakglass,
				Reason: "current generation failed probation and there is no prev to roll back to — manual recovery",
			}
		}
		return SupervisorAction{
			Kind:          ActionQuarantineAndRollback,
			QuarantineSHA: s.CurrentSHA,
			RelaunchSHA:   s.PrevSHA,
			WithResume:    true,
			Reason:        "current generation failed probation (nonzero exit before confirm) — quarantined, rolling back to prev",
		}
	}
	if s.CurrentSHA == "" {
		return SupervisorAction{
			Kind:   ActionBreakglass,
			Reason: "no current generation to relaunch — manual recovery",
		}
	}
	reason := "clean exit — relaunching current with --resume"
	if s.ChildExitCode != 0 {
		// a healthy generation (confirmed / past probation) that later died: relaunch it, do NOT roll
		// back — the failure is not attributable to the swap.
		reason = "post-confirm/post-probation exit — relaunching current with --resume (not a swap failure)"
	}
	return SupervisorAction{
		Kind:        ActionRelaunchCurrent,
		RelaunchSHA: s.CurrentSHA,
		WithResume:  true,
		Reason:      reason,
	}
}
