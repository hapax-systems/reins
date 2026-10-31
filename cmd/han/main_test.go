package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

// foundLookPath resolves every program name to a fake absolute path.
func foundLookPath(name string) (string, error) { return "/usr/bin/" + name, nil }

// recordingRunCmd captures the dispatched path and forwarded args.
type recorder struct {
	path string
	args []string
	code int
}

func (r *recorder) run(path string, args []string) int {
	r.path = path
	r.args = args
	return r.code
}

func TestDispatchTable(t *testing.T) {
	want := map[string]string{
		"scope":  "/usr/bin/reins",
		"docket": "/usr/bin/reins",
		"forum":  "/usr/bin/hapx",
		"locus":  "/usr/bin/stead",
	}
	for role, path := range want {
		var out, errw bytes.Buffer
		rec := &recorder{}
		code := run([]string{role, "--demo"}, &out, &errw, foundLookPath, rec.run)
		if code != 0 {
			t.Fatalf("han %s: exit %d, want 0 (stderr %q)", role, code, errw.String())
		}
		if rec.path != path {
			t.Errorf("han %s: dispatched to %q, want %q", role, rec.path, path)
		}
		if len(rec.args) != 1 || rec.args[0] != "--demo" {
			t.Errorf("han %s: forwarded args %v, want [--demo]", role, rec.args)
		}
	}
}

func TestUnknownSubcommandRefuses(t *testing.T) {
	var out, errw bytes.Buffer
	rec := &recorder{}
	code := run([]string{"bridge"}, &out, &errw, foundLookPath, rec.run)
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if rec.path != "" {
		t.Errorf("an unknown subcommand must never execute; got %q", rec.path)
	}
	if !strings.Contains(errw.String(), `unknown subcommand "bridge"`) || !strings.Contains(errw.String(), "usage:") {
		t.Errorf("stderr missing the refusal and usage: %q", errw.String())
	}
}

func TestNoSubcommandRefuses(t *testing.T) {
	var out, errw bytes.Buffer
	code := run(nil, &out, &errw, foundLookPath, (&recorder{}).run)
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if !strings.Contains(errw.String(), "usage:") {
		t.Errorf("stderr missing usage: %q", errw.String())
	}
}

func TestVersionReportsEstateIdentity(t *testing.T) {
	var out, errw bytes.Buffer
	code := run([]string{"--version"}, &out, &errw, foundLookPath, (&recorder{}).run)
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	got := out.String()
	if !strings.Contains(got, identity) || !strings.Contains(got, "han "+version) {
		t.Errorf("version output %q missing the estate identity or version", got)
	}
	if !strings.Contains(got, "hapax-systems/reins") {
		t.Errorf("version output %q must distinguish this dispatcher from a foreign han", got)
	}
}

func TestMissingProgramIs127(t *testing.T) {
	var out, errw bytes.Buffer
	absent := func(string) (string, error) { return "", errors.New("not found") }
	code := run([]string{"locus"}, &out, &errw, absent, (&recorder{}).run)
	if code != 127 {
		t.Errorf("exit %d, want 127", code)
	}
	if !strings.Contains(errw.String(), `"stead"`) || !strings.Contains(errw.String(), "not on PATH") {
		t.Errorf("stderr missing the binding and the reason: %q", errw.String())
	}
}

func TestHelpIsClean(t *testing.T) {
	var out, errw bytes.Buffer
	code := run([]string{"--help"}, &out, &errw, foundLookPath, (&recorder{}).run)
	if code != 0 {
		t.Errorf("exit %d, want 0", code)
	}
	if !strings.Contains(out.String(), "usage:") {
		t.Errorf("stdout missing usage: %q", out.String())
	}
}

func TestExitCodePassthrough(t *testing.T) {
	var out, errw bytes.Buffer
	rec := &recorder{code: 42}
	code := run([]string{"docket"}, &out, &errw, foundLookPath, rec.run)
	if code != 42 {
		t.Errorf("exit %d, want the bound program's 42", code)
	}
}

// --- the real subprocess path (no fakes): execAndWait itself ---

func TestExecAndWaitRealChildZeroExit(t *testing.T) {
	code := execAndWait("/bin/sh", []string{"-c", "exit 0"})
	if code != 0 {
		t.Errorf("exit %d, want 0", code)
	}
}

func TestExecAndWaitRealChildNonZeroPassthrough(t *testing.T) {
	code := execAndWait("/bin/sh", []string{"-c", "exit 3"})
	if code != 3 {
		t.Errorf("exit %d, want 3 (the child's own code, via exec.ExitError)", code)
	}
}

func TestExecAndWaitLaunchFailureIsDocumented1WithPrefix(t *testing.T) {
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	code := execAndWait("/nonexistent/han-bound-program", nil)
	w.Close()
	os.Stderr = old
	captured, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if code != 1 {
		t.Errorf("exit %d, want the documented 1 for a launch failure", code)
	}
	if !strings.HasPrefix(string(captured), "han: ") {
		t.Errorf("stderr %q missing the han: prefix", captured)
	}
}
