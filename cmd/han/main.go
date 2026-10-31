// Command han is the System I front dispatcher: a THIN handoff to the four role
// programs, with no engine of its own. Row:
// system-i-han-front-command-dispatcher-slice-1-20261031 (System I, ratified by the
// operator 2026-10-31T06:40:51Z: Scope, Docket, Forum, Locus).
//
// The role words bind to today's program names (read at source): scope and docket
// ride the reins program, forum rides hapx, locus rides stead. No aliases, no
// renames; those are later slices.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// version is stamped at build time (-ldflags "-X main.version=..."), matching the
// cockpit's pattern; "dev" for a bare `go run`.
var version = "dev"

// identity names the estate dispatcher so a collision probe can tell it from a
// foreign han on PATH.
const identity = "HRL-HAN System I front dispatcher (hapax-systems/reins)"

// dispatch binds each System I role word to today's program name.
var dispatch = map[string]string{
	"scope":  "reins", // the frame housing surface; today it rides the reins program
	"docket": "reins", // work and governance
	"forum":  "hapx",  // conversation
	"locus":  "stead", // the spatial lab
}

const usage = `han — the HRL-HAN System I front dispatcher

usage: han <role> [args...]
  scope    the frame housing surface (today: the reins program)
  docket   work and governance (the reins program)
  forum    conversation (the hapx program)
  locus    the spatial lab (the stead program)
  --version  print the estate dispatcher identity
`

// run resolves argv (minus argv[0]) and executes the bound program, returning the
// process exit code. lookPath and runCmd are the only seams; tests inject fakes.
func run(args []string, stdout, stderr io.Writer, lookPath func(string) (string, error), runCmd func(path string, args []string) int) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "--version", "-v", "version":
		fmt.Fprintf(stdout, "han %s — %s\n", version, identity)
		return 0
	case "--help", "-h", "help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	program, ok := dispatch[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "han: unknown subcommand %q\n\n%s", args[0], usage)
		return 2
	}
	path, err := lookPath(program)
	if err != nil {
		fmt.Fprintf(stderr, "han: %s is bound to the %q program, which is not on PATH\n", args[0], program)
		return 127
	}
	return runCmd(path, args[1:])
}

// execAndWait runs the bound program with this process's stdio attached and
// returns its exit code.
func execAndWait(path string, args []string) int {
	cmd := exec.Command(path, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "han: %v\n", err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, exec.LookPath, execAndWait))
}
