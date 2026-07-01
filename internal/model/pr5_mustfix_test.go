package model

import (
	"fmt"
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

// Dossier F5 — the hint-teleport window (taskWindow) must be derived through the SAME math the
// renderer uses, at every frame height: the rendered row count equals taskWindow().visible and the
// first rendered row is vt[off]. The old hand-derived copy (h<12 -> 40) mistargeted whois/yank/
// :intent on scrolled small frames.
func TestHintWindowMatchesRenderedTaskWindow(t *testing.T) {
	for _, h := range []int{8, 9, 10, 11, 12, 20, 40} {
		m := Model{Width: 120, Height: h, Mode: ModeHint}
		for i := 0; i < 30; i++ {
			m.Tasks = append(m.Tasks, grammar.Task{TaskID: fmt.Sprintf("task%02d", i)})
		}
		m.Focus = 25 // deep in the list — the window must scroll
		off, visible := m.taskWindow()
		body := ansi.Strip(m.tasksListBody(120, frameMidH(h)))
		lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
		rows := len(lines) - 2 // context line + header
		want := visible
		if rem := len(m.Tasks) - off; rem < want {
			want = rem // the window may outsize a short list — the renderer stops at the last task
		}
		if rows != want {
			t.Fatalf("h=%d: renderer drew %d rows but taskWindow window holds %d — hint labels mistarget", h, rows, want)
		}
		if !strings.Contains(lines[2], fmt.Sprintf("task%02d", off)) {
			t.Fatalf("h=%d: first rendered row is not vt[off=%d]:\n%q", h, off, lines[2])
		}
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
