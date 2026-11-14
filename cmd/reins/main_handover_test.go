package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hapax-systems/reins/internal/generation"
	"github.com/hapax-systems/reins/internal/model"
)

// Exercise the real exec boundary in an isolated process: the staged fixture only
// writes a marker, so an unsafe exec cannot replace the test runner or live cockpit.
func TestHandoverWriteFailureRefusesExec(t *testing.T) {
	if os.Getenv("REINS_TEST_HANDOVER_CHILD") == "1" {
		m := model.New("fixture")
		m.Deck = []string{"retained operator readout"}
		execHandover(m)
		os.Exit(90)
	}
	for _, failure := range []string{"posture", "handoff", "healthy"} {
		t.Run(failure, func(t *testing.T) {
			dir := t.TempDir()
			store := generation.NewStore(filepath.Join(dir, "store"))
			marker := filepath.Join(dir, "executed")
			binary := []byte("#!/bin/sh\nprintf '%s' \"$1\" > \"$REINS_TEST_EXEC_MARKER\"\n")
			if _, err := store.Stage("fixture", binary, map[string][]byte{"reins_serve.py": []byte("# fixture")}, "2026-11-14", ""); err != nil {
				t.Fatal(err)
			}
			if err := store.SetCurrent("fixture"); err != nil {
				t.Fatal(err)
			}
			posture := filepath.Join(dir, "posture.json")
			blocked := posture
			if failure == "handoff" {
				blocked = filepath.Join(store.Root(), "handoff.json")
			}
			if failure != "healthy" {
				// A directory at the final atomic-rename destination fails even as root.
				if err := os.Mkdir(blocked, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHandoverWriteFailureRefusesExec$")
			cmd.Env = append(os.Environ(), "REINS_TEST_HANDOVER_CHILD=1", "HOME="+dir,
				"REINS_GENERATION_ROOT="+store.Root(), "REINS_POSTURE_PATH="+posture,
				"REINS_TEST_EXEC_MARKER="+marker)
			output, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatal("fixture exec exceeded its deadline")
			}
			if failure != "healthy" {
				if err == nil {
					t.Fatalf("%s write failed but handover succeeded: %s", failure, output)
				}
				if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
					t.Fatalf("%s write failed but a generation was executed", failure)
				}
				if !strings.Contains(string(output), failure) || !strings.Contains(string(output), "refused") {
					t.Fatalf("failure has no named refusal: %s", output)
				}
				return
			}
			if err != nil {
				t.Fatalf("healthy handover failed: %v: %s", err, output)
			}
			if argv, err := os.ReadFile(marker); err != nil || string(argv) != "--resume" {
				t.Fatalf("healthy handover did not exec with resume: %q, %v", argv, err)
			}
			if snap, ok, err := model.ReadPosture(posture); err != nil || !ok || len(snap.Deck) != 1 || snap.Deck[0] != "retained operator readout" {
				t.Fatalf("posture was lost before exec: %+v, %v", snap, err)
			}
			if handoff, ok, err := store.ConsumeHandoff(); err != nil || !ok || handoff.TargetSHA != "fixture" || handoff.PosturePath != posture {
				t.Fatalf("handoff was not durable before exec: %+v, %v", handoff, err)
			}
		})
	}
}
