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
	"strconv"

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
	// ALLOWLIST the exec gate: exec ONLY a recognized current/prev tier with a non-empty sha. Anything
	// else — breakglass, an empty sha, or an unrecognized tier string — execs nothing. This localizes the
	// "an unverified generation must never boot" invariant INSIDE swap.go rather than trusting that every
	// caller feeds fresh Resolve() output (a denylist would exec a {SHA:"x", Tier:"garbage"} resolution).
	if res.SHA == "" || (res.Tier != generation.TierCurrent && res.Tier != generation.TierPrev) {
		reason := res.Reason
		if reason == "" {
			reason = "unrecognized resolution tier " + strconv.Quote(res.Tier) + " — refusing to exec"
		}
		return ExecPlan{
			ShouldExec:       false,
			Tier:             generation.TierBreakglass,
			BreakglassReason: reason,
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

// APIExecPlan is the resolved hand-over for the reins-api.service (design pack A1.3: the service
// ExecStart resolves through the SAME generation `current` pointer the cockpit uses). ShouldExec is
// false for a breakglass/unverified resolution — systemd's StartLimit then stops the loop and the meta
// facet renders API: FAILED, never a silent wrong-generation serve.
type APIExecPlan struct {
	ShouldExec       bool
	Argv             []string // [python, <apiDir>/reins_serve.py]
	APIDir           string   // cwd for the exec (mirrors the old WorkingDirectory)
	ServingSHA       string   // exported as REINS_SERVING_SHA so /read/meta reports the generation (pointer-fact)
	BreakglassReason string
}

// ResolveAPIExecPlan turns a store Resolution into the API's exec plan: serve the resolved generation's
// api/reins_serve.py under the given python interpreter. Same allowlist gate as ResolveExecPlan — an
// unrecognized/empty resolution execs nothing (breakglass), so a bad generation can never be served.
func ResolveAPIExecPlan(store *generation.Store, res generation.Resolution, python string) APIExecPlan {
	if res.SHA == "" || (res.Tier != generation.TierCurrent && res.Tier != generation.TierPrev) {
		reason := res.Reason
		if reason == "" {
			reason = "unrecognized resolution tier " + strconv.Quote(res.Tier) + " — refusing to serve"
		}
		return APIExecPlan{ShouldExec: false, BreakglassReason: reason}
	}
	apiDir := filepath.Join(store.GenerationDir(res.SHA), "api")
	return APIExecPlan{
		ShouldExec: true,
		Argv:       []string{python, filepath.Join(apiDir, "reins_serve.py")},
		APIDir:     apiDir,
		ServingSHA: res.SHA,
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
	ActionRelaunchCurrent       = "relaunch-current"        // normal: relaunch current with --resume
	ActionQuarantineAndRollback = "quarantine-and-rollback" // bad new generation: quarantine + relaunch prev
	ActionBreakglass            = "breakglass"              // nothing safe to relaunch — manual recovery
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
	// a probation failure needs a real current to attribute the failure to (an empty current falls
	// through to the generic no-current breakglass below).
	probationFailure := s.ChildExitCode != 0 && !s.ConfirmSeen && s.WithinProbation && s.CurrentSHA != ""
	if probationFailure {
		if s.PrevSHA == "" {
			// no prev to roll back to. QUARANTINE the failed current anyway, so a naive systemd restart's
			// Resolve() falls straight to breakglass instead of re-selecting the bad current in a loop.
			return SupervisorAction{
				Kind:          ActionBreakglass,
				QuarantineSHA: s.CurrentSHA,
				Reason:        "current failed probation and there is no prev — quarantined to stop a re-select loop; manual recovery",
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
