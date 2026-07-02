// Command reins-run is the hot-plug shell (U6b-deploy) around the proven decision core in internal/swap.
// It has three modes:
//
//	reins-run --api          resolve the generation `current` pointer and exec that generation's
//	                         api/reins_serve.py (the reins-api.service ExecStart; A1.3 pointer-resolved).
//	reins-run stage ...      stage a generation into the store (deploy tooling): copy a built binary + the
//	                         api/*.py tree + write a byte-binding manifest; optionally flip `current`.
//	reins-run supervise ...  the cockpit crash-BACKSTOP: run the resolved cockpit as a child, observe its
//	                         probation, and quarantine+rollback or relaunch per DecideSupervisorAction.
//
// All decisions come from internal/swap (pure, tested); this binary is the thin effectful adapter.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hapax-systems/reins/internal/generation"
	"github.com/hapax-systems/reins/internal/swap"
)

func storeRoot() string {
	if v := strings.TrimSpace(os.Getenv("REINS_GENERATION_ROOT")); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "reins")
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "reins-run: "+format+"\n", a...)
	os.Exit(1)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		die("usage: reins-run --api | stage <flags> | supervise <flags>")
	}
	store := generation.NewStore(storeRoot())
	switch args[0] {
	case "--api":
		runAPI(store)
	case "stage":
		runStage(store, args[1:])
	case "supervise":
		runSupervise(store, args[1:])
	default:
		die("unknown mode %q (want --api | stage | supervise)", args[0])
	}
}

// runAPI resolves the current generation and execs its api under $REINS_API_PYTHON. A breakglass
// resolution exits nonzero so systemd's StartLimit stops the loop (rendered API: FAILED, not a silent
// wrong-generation serve).
func runAPI(store *generation.Store) {
	python := strings.TrimSpace(os.Getenv("REINS_API_PYTHON"))
	if python == "" {
		die("REINS_API_PYTHON unset — need the interpreter path for the generation's api")
	}
	plan := swap.ResolveAPIExecPlan(store, store.Resolve(), python)
	if !plan.ShouldExec {
		die("BREAKGLASS — refusing to serve: %s", plan.BreakglassReason)
	}
	if err := os.Chdir(plan.APIDir); err != nil {
		die("chdir %s: %v", plan.APIDir, err)
	}
	// pointer-fact serving identity: the generation reports its own sha via /read/meta (A1.3).
	os.Setenv("REINS_SERVING_SHA", plan.ServingSHA)
	os.Setenv("REINS_GENERATION_ROOT", store.Root())
	if err := syscall.Exec(plan.Argv[0], plan.Argv, os.Environ()); err != nil {
		die("exec %v: %v", plan.Argv, err)
	}
}

// runStage stages a generation: reins-run stage --binary <path> --api-dir <dir> --sha <sha>
// [--prev <sha>] [--created <iso>] [--set-current]. Copies the binary + the api/*.py tree + writes the
// byte-binding manifest via the proven generation.Store.Stage. --set-current flips the pointer (deploy).
func runStage(store *generation.Store, args []string) {
	f := parseFlags(args)
	binaryPath, apiDir, sha := f["binary"], f["api-dir"], f["sha"]
	if binaryPath == "" || apiDir == "" || sha == "" {
		die("stage: --binary, --api-dir and --sha are required")
	}
	binary, err := os.ReadFile(binaryPath)
	if err != nil {
		die("stage: read binary %s: %v", binaryPath, err)
	}
	tree, err := readAPITreeForStage(apiDir)
	if err != nil {
		die("stage: read api tree %s: %v", apiDir, err)
	}
	created := f["created"]
	if created == "" {
		created = time.Now().UTC().Format(time.RFC3339)
	}
	m, err := store.Stage(sha, binary, tree, created, f["prev"])
	if err != nil {
		die("stage: %v", err)
	}
	if err := store.Verify(sha); err != nil {
		die("stage: staged generation FAILS verify (refusing): %v", err)
	}
	fmt.Printf("staged %s (binary_sha256=%s… api_tree_sha256=%s…)\n", sha, m.BinarySHA256[:12], m.APITreeSHA256[:12])
	if _, ok := f["set-current"]; ok {
		// A promotion must ride the WITNESSED rail (design pack A1.5): witness the stage through the live
		// stage verb (the SSOT — no cross-language ledger-format duplication) BEFORE flipping current.
		// If the API is unreachable (bootstrap / mid-swap), this is the modeled breakglass-manual path —
		// it must be DISCLOSED loudly, never a silent out-of-rail promotion.
		witnessStage(sha)
		if err := store.SetCurrent(sha); err != nil {
			die("stage: set-current: %v", err)
		}
		fmt.Printf("current -> %s (prev -> %s)\n", store.Current(), store.Prev())
	}
}

// witnessStage POSTs the promotion to the live stage verb so it lands a demand+verdict on the durable
// ledger (governed rail). Non-fatal: an unreachable API means BREAKGLASS-MANUAL, which is disclosed
// loudly to stderr (the operator then knows the promotion was out-of-rail), never suppressed.
func witnessStage(sha string) {
	url := strings.TrimSpace(os.Getenv("REINS_API_URL"))
	if url == "" {
		url = "http://127.0.0.1:8799"
	}
	body, _ := json.Marshal(map[string]any{
		"target":            sha,
		"authority_packet":  map[string]any{"kind": "cli-stage"},
		"preflight_receipt": map[string]any{},
		"idempotency_key":   "clistage-" + sha + "-" + strconv.FormatInt(time.Now().UnixNano(), 36),
	})
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Post(url+"/command/stage", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "reins-run: ⚠ BREAKGLASS-MANUAL stage — API unreachable (%v); promotion is OUT-OF-RAIL (unwitnessed). Re-witness when the API is up.\n", err)
		return
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 200 {
		fmt.Fprintf(os.Stderr, "reins-run: stage WITNESSED via the governed rail (verb ok).\n")
	} else {
		// the verb refused (e.g. the store copy is not yet verifiable) — disclose; the operator decides.
		fmt.Fprintf(os.Stderr, "reins-run: ⚠ stage verb returned %d: %s — promotion NOT cleanly witnessed.\n", resp.StatusCode, strings.TrimSpace(string(rb)))
	}
}

// readAPITreeForStage reads the canonical {top-level *.py} set from a repo api/ dir (the same set
// generation.APITreeHash + reins_serve.api_tree_sha hash) — so a staged generation verifies.
func readAPITreeForStage(apiDir string) (map[string][]byte, error) {
	entries, err := os.ReadDir(apiDir)
	if err != nil {
		return nil, err
	}
	tree := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".py") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(apiDir, e.Name()))
		if err != nil {
			return nil, err
		}
		tree[e.Name()] = b
	}
	return tree, nil
}

// runSupervise is the cockpit crash-BACKSTOP: run the resolved current generation's cockpit as a child,
// observe its probation (exit code, confirm-file, probation window), and quarantine+rollback or relaunch
// per DecideSupervisorAction. It NEVER initiates a swap — swaps are the cockpit's own exec-handover.
//
//	reins-run supervise --confirm <path> [--probation <seconds>] [-- <cockpit args>]
func runSupervise(store *generation.Store, args []string) {
	f := parseFlags(args)
	confirmPath := f["confirm"]
	if confirmPath == "" {
		die("supervise: --confirm <path> is required (the probation confirm-file the cockpit writes)")
	}
	probation := 30 * time.Second
	if v := f["probation"]; v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			probation = time.Duration(secs) * time.Second
		}
	}
	for {
		res := store.Resolve()
		if res.Tier == generation.TierBreakglass || res.SHA == "" {
			die("BREAKGLASS — no verified generation to supervise: %s", res.Reason)
		}
		cockpit := filepath.Join(store.GenerationDir(res.SHA), "reins")
		_ = os.Remove(confirmPath) // clear a stale confirm before this run

		start := time.Now()
		cmd := exec.Command(cockpit, "--resume")
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		cmd.Env = append(os.Environ(), "REINS_GENERATION_ROOT="+store.Root(),
			"REINS_SERVING_SHA="+res.SHA, "REINS_CONFIRM_PATH="+confirmPath)
		runErr := cmd.Run()
		exitCode := 0
		if runErr != nil {
			exitCode = 1
			if ee, ok := runErr.(*exec.ExitError); ok {
				exitCode = ee.ExitCode()
			}
		}
		_, confirmErr := os.Stat(confirmPath)
		action := swap.DecideSupervisorAction(swap.SupervisorState{
			ChildExitCode:   exitCode,
			ConfirmSeen:     confirmErr == nil,
			WithinProbation: time.Since(start) < probation,
			CurrentSHA:      res.SHA,
			PrevSHA:         store.Prev(),
		})
		fmt.Fprintf(os.Stderr, "reins-run: %s — %s\n", action.Kind, action.Reason)
		switch action.Kind {
		case swap.ActionQuarantineAndRollback:
			// coupling (c): quarantine DURABLY before relaunching prev — Resolve then skips the bad
			// current and self-heals to prev (no explicit SetCurrent needed).
			if err := store.Quarantine(action.QuarantineSHA, action.Reason); err != nil {
				die("supervise: quarantine %s: %v", action.QuarantineSHA, err)
			}
			continue
		case swap.ActionBreakglass:
			if action.QuarantineSHA != "" {
				_ = store.Quarantine(action.QuarantineSHA, action.Reason)
			}
			die("BREAKGLASS — %s", action.Reason)
		case swap.ActionRelaunchCurrent:
			continue
		}
	}
}

func parseFlags(args []string) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			continue
		}
		key := strings.TrimPrefix(a, "--")
		if eq := strings.IndexByte(key, '='); eq >= 0 {
			out[key[:eq]] = key[eq+1:]
			continue
		}
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			out[key] = args[i+1]
			i++
		} else {
			out[key] = "" // bare flag (e.g. --set-current)
		}
	}
	return out
}
