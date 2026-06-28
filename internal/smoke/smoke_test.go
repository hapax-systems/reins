package smoke

import (
	"strings"
	"testing"
)

// The cockpit must navigate to EVERY page without panicking and render a non-empty frame — the
// automated half of "smoke-test navigation without a human". A panic on any page is a hard finding.
func TestNavigateEveryPageNoPanic(t *testing.T) {
	m := SeedModel(170, 46)
	steps := make([]string, 0, len(PageCommands))
	for _, name := range PageCommands {
		steps = append(steps, ":"+name)
	}
	frames := Drive(m, steps)
	for _, f := range frames {
		if f.Panic != "" {
			t.Fatalf("navigating %q PANICKED: %s", f.Step, f.Panic)
		}
		if strings.TrimSpace(f.Plain()) == "" {
			t.Fatalf("navigating %q rendered an EMPTY frame", f.Step)
		}
	}
}

// Interaction smoke: the common in-page gestures (move, inspect-door, verb-menu, yank, filter,
// command) must not panic and must return cleanly. Exercises the door/menu/mode machinery.
func TestInPageGesturesNoPanic(t *testing.T) {
	m := SeedModel(170, 46)
	steps := []string{
		":tasks", "j", "j", "k", // move the task cursor
		"enter", "esc", // open + close the /whois door
		"v", "a", "esc", // verb menu → arm preview → dismiss
		"y", "esc", // yank mode → dismiss
		"/", "esc", // filter mode → dismiss
		":coordinator", "j", "j", // coordinator lens nav (brushing)
		":events", "j", "v", "esc", // events nav
		":sessions", "j", "enter", "esc", // session door
		":help", ":legend", // reference pages
	}
	frames := Drive(m, steps)
	for _, f := range frames {
		if f.Panic != "" {
			t.Fatalf("gesture %q PANICKED: %s", f.Step, f.Panic)
		}
	}
	// the final frame (the legend) must render
	last := frames[len(frames)-1]
	if strings.TrimSpace(last.Plain()) == "" {
		t.Fatalf("the final navigation frame was empty")
	}
}

// Deeper coverage: on EVERY page, the standard in-page gestures (move to both ends, descend a row,
// open + close a door/detail, tab into fields) must not panic — a fresh seed per page so one page's
// state never masks another's crash. This is the per-page nav backstop for new panes (A1/A6/…).
func TestEveryPageGesturesNoPanic(t *testing.T) {
	const gestures = "j j k g G enter esc tab esc l h"
	for _, name := range PageCommands {
		m := SeedModel(170, 46) // fresh per page — isolate each page's gesture crash
		frames := Drive(m, []string{":" + name, gestures})
		for _, f := range frames {
			if f.Panic != "" {
				t.Fatalf("page %q step %q PANICKED: %s", name, f.Step, f.Panic)
			}
		}
	}
}

// On-air navigation must never panic and must show the redaction token somewhere (the cockpit is
// default-deny on a livestream) — a coarse but real AIR-safety smoke across the nav surface.
func TestNavigateOnAirRedactsAndNoPanic(t *testing.T) {
	m := SeedModel(170, 46)
	m.AIR = true
	steps := []string{":tasks", ":sessions", ":events", ":traces", ":coordinator"}
	frames := Drive(m, steps)
	sawRedaction := false
	for _, f := range frames {
		if f.Panic != "" {
			t.Fatalf("on-air nav %q PANICKED: %s", f.Step, f.Panic)
		}
		if strings.Contains(f.Plain(), "▒▒▒") {
			sawRedaction = true
		}
	}
	if !sawRedaction {
		t.Fatalf("on-air navigation rendered no redaction token across %d frames — AIR may be inert", len(frames))
	}
}
