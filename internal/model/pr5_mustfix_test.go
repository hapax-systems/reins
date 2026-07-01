package model

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// Dossier F1 — the coordinator-lens breadcrumb (Z3 rank) must AIR-redact stage exactly like the
// row cells do. A raw t.Stage in the crumb leaked a denied stage on air.
func TestCoordinatorLensBreadcrumbRedactsStageOnAir(t *testing.T) {
	const stageS = "LEAKSTAGE"
	m := Model{Width: 120, Height: 40, AIR: true}
	m.Tasks = []grammar.Task{{
		TaskID:      "t1",
		Stage:       stageS,
		Criticality: "ok",
		AIR:         map[string]string{"task_id": "ok", "stage": "deny", "owner": "deny", "criticality": "ok"},
	}}
	out := ansi.Strip(m.coordinatorLensPane(120, 30))
	if strings.Contains(out, stageS) {
		t.Fatalf("on-air coordinator-lens breadcrumb leaks denied stage:\n%s", out)
	}
	off := Model{Width: 120, Height: 40, AIR: false}
	off.Tasks = m.Tasks
	if !strings.Contains(ansi.Strip(off.coordinatorLensPane(120, 30)), stageS) {
		t.Fatalf("off-air breadcrumb should still show the stage (redaction only bites on air)")
	}
}

// Dossier F5 — the hint-teleport window (taskWindow) must match the row window the REAL render
// path draws: bodyFor -> composePage -> layout.Render, which consumes ONE relation-header row
// whenever the tasks page's emergent relation is non-empty (the normal fleet case — tasks share
// owner facets). The prior fix (and its direct-call test) bypassed layout.Render and validated a
// height the renderer never passes — a false pass over the live off-by-one. This test drives the
// real path and asserts the first rendered task row is exactly vt[off].
func TestHintWindowMatchesRealRenderPath(t *testing.T) {
	rowRe := regexp.MustCompile(`task\d{2}`)
	for _, h := range []int{11, 12, 16, 20, 40} {
		m := Model{Width: 160, Height: h, Mode: ModeHint, Page: PageTasks}
		for i := 0; i < 30; i++ {
			m.Tasks = append(m.Tasks, grammar.Task{
				TaskID: fmt.Sprintf("task%02d", i), Owner: "sharedowner", Stage: "S6",
			})
		}
		m.Focus = 25 // deep in the list — the window must scroll
		if m.tasksEmergentRelation() == "" {
			t.Fatal("test setup: emergent relation empty — the connector-header case is not exercised")
		}
		off, _ := m.taskWindow()
		frame := ansi.Strip(m.bodyFor(159, frameMidH(h)))
		var first string
		for _, ln := range strings.Split(frame, "\n") {
			// a task ROW carries its id left of the stage column (hint label + status glyphs
			// push it to ~col 13); the context line mentions ids mid-line ("focus: taskNN",
			// ~col 40+) and must not match.
			if loc := rowRe.FindStringIndex(ln); loc != nil && loc[0] < 20 {
				first = ln
				break
			}
		}
		if first == "" {
			t.Fatalf("h=%d: no task row rendered through the real path:\n%s", h, frame)
		}
		if !strings.Contains(first, fmt.Sprintf("task%02d", off)) {
			t.Fatalf("h=%d: first REAL-rendered row is not vt[off=%d] — hint labels mistarget:\nrow: %q", h, off, first)
		}
	}
}

// taskPaneBodyH mirrors layout.render's header consumption: minus one when the relation header
// renders, untouched when the frame is too short for a header or the page is dark.
func TestTaskPaneBodyHMirrorsConnectorHeader(t *testing.T) {
	m := Model{Width: 160, Height: 20, Page: PageTasks}
	for i := 0; i < 3; i++ {
		m.Tasks = append(m.Tasks, grammar.Task{TaskID: fmt.Sprintf("task%02d", i), Owner: "sharedowner"})
	}
	if got, want := m.taskPaneBodyH(), frameMidH(20)-1; got != want {
		t.Fatalf("relation non-empty: taskPaneBodyH=%d want %d (header row not subtracted)", got, want)
	}
	dark := m
	dark.TasksDark = true
	if got, want := dark.taskPaneBodyH(), frameMidH(20); got != want {
		t.Fatalf("dark page renders the legacy single pane: taskPaneBodyH=%d want %d", got, want)
	}
	lone := Model{Width: 160, Height: 20, Page: PageTasks, Tasks: []grammar.Task{{TaskID: "solo"}}}
	if got, want := lone.taskPaneBodyH(), frameMidH(20); got != want {
		t.Fatalf("no emergent relation: taskPaneBodyH=%d want %d", got, want)
	}
}

// Dossier F6 — turnListBody must RESERVE every optional header line (breakdown inbox, session
// position, attention pointer) before sizing the row window, or the pane over-renders and the
// focused turn clips off exactly when the breakdown appears.
func TestTurnListBodyReservesOptionalLinesNoClip(t *testing.T) {
	m := Model{Width: 100, Height: 40}
	// a red-readiness session forces the breakdown inbox on (off air, nothing denied)
	m.Sessions = []grammar.Session{{Role: "lane-red", Readiness: "red"}}
	for i := 0; i < 40; i++ {
		m.TurnLadder = append(m.TurnLadder, grammar.Turn{
			TS: fmt.Sprintf("2026-07-01T00:00:%02dZ", i), Role: "lane", Kind: "assistant",
			Prov: "model", Summary: fmt.Sprintf("turnsum%02d", i),
		})
	}
	m.TurnFocus = len(m.TurnLadder) - 1 // bottom-scrolled — the clip victim in the bug
	h := 14
	out := ansi.Strip(m.turnListBody(100, h))
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > h {
		t.Fatalf("turnListBody over-renders: %d lines for h=%d (optional lines not reserved)", len(lines), h)
	}
	if !strings.Contains(out, "turnsum39") {
		t.Fatalf("focused (last) turn clipped out of the pane at h=%d:\n%s", h, out)
	}
}
