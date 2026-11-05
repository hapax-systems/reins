package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIHelpAndVersionExitZeroWithoutTTY(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "reins-test")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v: %s", err, string(out))
	}
	for _, flag := range []string{"--help", "-h", "--version"} {
		cmd := exec.Command(bin, flag)
		cmd.Stdin = nil // /dev/null non-TTY
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("reins %s failed without TTY: %v\n%s", flag, err, string(out))
		}
		if !strings.Contains(string(out), "reins ") {
			t.Fatalf("unexpected output for %s: %s", flag, string(out))
		}
	}
}
