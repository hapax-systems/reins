package model

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

func TestViewFillsExactFrame(t *testing.T) {
	m := New("REINS").Fold(evs(), false)
	m.Width, m.Height = 120, 40
	lines := strings.Split(m.View(), "\n")
	if len(lines) != 40 {
		t.Fatalf("120x40 must render exactly 40 lines, got %d", len(lines))
	}
	for i, ln := range lines {
		if ansi.StringWidth(ln) > 120 {
			t.Fatalf("line %d exceeds 120 cols: %d", i, ansi.StringWidth(ln))
		}
	}
}

func TestFocusNavigationAndRail(t *testing.T) {
	tasks := []grammar.Task{
		{TaskID: "alpha-1", Stage: "S5_DESIGN", PriorStage: "S4_PLAN", PredictedStage: "S6", Owner: "cc-a", Criticality: "warn",
			AIR: map[string]string{"task_id": "ok", "stage": "ok", "prior_stage": "ok", "predicted_stage": "ok", "owner": "ok", "criticality": "ok", "freshness": "ok"}},
		{TaskID: "beta-2", Stage: "S7_RELEASE", PredictedStage: "hold", Owner: "cc-b", Criticality: "crit",
			AIR: map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok", "owner": "ok", "criticality": "ok", "freshness": "ok"}},
	}
	m := New("REINS").FoldTasks(tasks, false)
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	// j moves focus down; k clamps at 0; G to bottom
	m = step(m, "j")
	if m.Focus != 1 {
		t.Fatalf("j should move focus to 1, got %d", m.Focus)
	}
	m = step(step(m, "k"), "k") // clamp at 0
	if m.Focus != 0 {
		t.Fatalf("k should clamp focus at 0, got %d", m.Focus)
	}
	m = step(m, "G")
	if m.Focus != 1 {
		t.Fatalf("G should jump to last (1), got %d", m.Focus)
	}
	// the rail unfolds the focused task's dims (beta-2 at focus 1: hold, crit, cc-b)
	v := m.View()
	for _, want := range []string{"beta-2", "hold", "crit", "cc-b"} {
		if !strings.Contains(v, want) {
			t.Fatalf("rail should unfold focused task dim %q:\n%s", want, v)
		}
	}
}

func TestWhoisDoorOpensAndCloses(t *testing.T) {
	tasks := []grammar.Task{{TaskID: "door-1", Stage: "S7_RELEASE", PredictedStage: "hold", Criticality: "warn",
		AIR: map[string]string{"task_id": "ok", "stage": "ok"}}}
	m := New("REINS").FoldTasks(tasks, false)
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	m = step(m, "enter")
	if !m.DoorOpen {
		t.Fatal("[enter] should open the /whois door")
	}
	if !strings.Contains(m.View(), "door-1") {
		t.Fatalf("door should render the task id:\n%s", m.View())
	}
	// a verb-dock key is a governed STUB — closes + reports, never mutates
	m = step(m, "a")
	if m.DoorOpen || !strings.Contains(m.Status, "governed COMMAND surface") {
		t.Fatalf("arm must close + report the governed route: open=%v status=%q", m.DoorOpen, m.Status)
	}
	// reopen + Esc closes cleanly
	m = step(step(m, "enter"), "esc")
	if m.DoorOpen {
		t.Fatal("[esc] should close the door")
	}
}

func TestYankGrabsFieldToRingAndCommandLine(t *testing.T) {
	tasks := []grammar.Task{{TaskID: "alpha-1", Stage: "S5", Owner: "cc-a",
		AIR: map[string]string{"task_id": "ok", "owner": "deny"}}}
	m := New("REINS").FoldTasks(tasks, false)
	m.Page = PageTasks
	// 'y' enters yank mode; 'i' grabs task_id -> ring + command line
	m = step(m, "y")
	if m.Mode != ModeYank {
		t.Fatalf("y should enter ModeYank, got %d", m.Mode)
	}
	m = step(m, "i")
	if m.Mode != ModeCommand || m.Input != "alpha-1" {
		t.Fatalf("grab should pre-seed the command line with the id: mode=%d input=%q", m.Mode, m.Input)
	}
	if len(m.Ring) != 1 || m.Ring[0].Value != "alpha-1" {
		t.Fatalf("grab should push to the ring: %+v", m.Ring)
	}
	// AIR-gated: owner is denied -> un-yankable, yields no cleartext
	m2 := New("REINS").FoldTasks(tasks, false)
	m2.Page = PageTasks
	m2.AIR = true
	m2 = step(step(m2, "y"), "o")
	if strings.Contains(m2.Input, "cc-a") || len(m2.Ring) != 0 {
		t.Fatalf("AIR-denied field must be un-yankable (no cleartext, no ring): input=%q ring=%v", m2.Input, m2.Ring)
	}
}

func TestFieldCursorDescendSteerYank(t *testing.T) {
	tasks := []grammar.Task{{TaskID: "alpha-1", Stage: "S5", Owner: "cc-a",
		AIR: map[string]string{"task_id": "ok", "stage": "ok", "owner": "ok"}}}
	m := New("REINS").FoldTasks(tasks, false)
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	m = step(m, "tab") // descend into fields
	if m.Sel.Rank != RankField || m.Sel.Field != selFields[0] {
		t.Fatalf("tab should descend to field rank at the first field: %+v", m.Sel)
	}
	m = step(m, "l") // steer to the next field (stage)
	if m.Sel.Field != "stage" {
		t.Fatalf("l should steer to 'stage', got %q", m.Sel.Field)
	}
	m = step(m, "y") // yank THE selected field directly (verb on selection)
	if m.Input != "S5" || len(m.Ring) != 1 || m.Sel.Rank != RankRow {
		t.Fatalf("y should yank the selected field + ascend: input=%q ring=%d rank=%d", m.Input, len(m.Ring), m.Sel.Rank)
	}
}

func TestWhichKeyMenu(t *testing.T) {
	if mv := matchVerbs("d"); len(mv) != 1 || mv[0].name != "dynamics" {
		t.Fatalf("'d' should match only dynamics, got %v", mv)
	}
	if len(matchVerbs("")) != len(verbs) {
		t.Fatal("empty input should match all verbs")
	}
	m := New("REINS")
	m.Width, m.Height = 120, 40
	m.Mode = ModeCommand
	m.Input = "ta" // prefix of tasks
	if !strings.Contains(m.View(), "tasks") {
		t.Fatalf("which-key should surface 'tasks' for input 'ta':\n%s", m.View())
	}
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func step(m Model, k string) Model {
	nm, _ := m.Update(key(k))
	return nm.(Model)
}

func TestViewNarrowDegradesNoPanic(t *testing.T) {
	m := New("REINS").Fold(evs(), false)
	m.Width, m.Height = 80, 24 // rail collapses
	lines := strings.Split(m.View(), "\n")
	if len(lines) != 24 {
		t.Fatalf("80x24 must render exactly 24 lines, got %d", len(lines))
	}
}

func evs() []grammar.Event {
	return []grammar.Event{
		{TS: "14:22", Kind: "pr.merged", Subject: "4284", Summary: "merged", Score: 0.7,
			AIR: map[string]string{"subject": "ok", "summary": "deny"}},
	}
}

func TestFoldIsPureAndIdempotent(t *testing.T) {
	m := New("REINS").Fold(evs(), false)
	again := New("REINS").Fold(evs(), false)
	if m.View() != again.View() {
		t.Fatal("fold must be pure: same events -> same view (the hot-reload property)")
	}
}

func TestViewRendersEventsAndStatusBar(t *testing.T) {
	v := New("REINS").Fold(evs(), false).View()
	if !strings.Contains(v, "REINS") || !strings.Contains(v, "4284") || !strings.Contains(v, "merged") {
		t.Fatalf("view missing vital frame or events: %q", v)
	}
}

func TestAIRLensRedactsInView(t *testing.T) {
	m := New("REINS").Fold(evs(), false)
	m.AIR = true
	if strings.Contains(m.View(), "merged") {
		t.Fatalf("AIR view leaked a denied field: %q", m.View())
	}
}

func TestDarkStateIsHonest(t *testing.T) {
	v := New("REINS").Fold(nil, true).View()
	if !strings.Contains(v, "dark") {
		t.Fatalf("dark fold must render an explicit dark state: %q", v)
	}
}

func TestExecSwitchesPageAndAIR(t *testing.T) {
	m := New("REINS")
	if m.Exec("tasks").Page != PageTasks {
		t.Fatal("exec :tasks must switch to the registry page")
	}
	if m.Exec("events").Page != PageEvents {
		t.Fatal("exec :events must switch to the events page")
	}
	if !m.Exec("air on").AIR {
		t.Fatal("exec :air on must enable the AIR lens")
	}
	on := m
	on.AIR = true
	if on.Exec("air off").AIR {
		t.Fatal("exec :air off must disable the AIR lens")
	}
	if !m.Exec("air").AIR {
		t.Fatal("bare :air must toggle (false -> true)")
	}
}

func TestExecUnknownIsInertButReported(t *testing.T) {
	m := New("REINS")
	out := m.Exec("frobnicate xyz")
	if out.Page != m.Page || !strings.Contains(out.Status, "unknown") {
		t.Fatalf("unknown verb must not mutate state + must report: page=%d status=%q", out.Page, out.Status)
	}
}

func TestExecHelpOpensHelpPage(t *testing.T) {
	m := New("REINS").Exec("help")
	if m.Page != PageHelp {
		t.Fatal("exec :help must open the help page")
	}
	v := m.View()
	if !strings.Contains(v, ":help") || !strings.Contains(v, ":dynamics") || !strings.Contains(v, "[a] AIR") {
		t.Fatalf("help page must list pages + keys: %q", v)
	}
}

func TestExecQuitFlags(t *testing.T) {
	if !New("REINS").Exec("quit").Quitting {
		t.Fatal("exec :quit must set the Quitting flag (Update turns it into tea.Quit)")
	}
}

func TestCommandModeViewEchoesBuffer(t *testing.T) {
	m := New("REINS")
	m.Mode = ModeCommand
	m.Input = "air on"
	if !strings.Contains(m.View(), ": air on") {
		t.Fatalf("command mode must echo the command buffer: %q", m.View())
	}
}

func TestDynamicsPageRendersViaExec(t *testing.T) {
	g := grammar.Graph{
		Layers: []grammar.Layer{{ID: "L", Label: "Backbone"}},
		Nodes: []grammar.Node{{ID: "rdf-owl-kg", Label: "KG", Layer: "L", Status: "asserted",
			AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}}},
	}
	m := New("REINS").FoldDynamics(g, false).Exec("dynamics")
	if m.Page != PageDynamics {
		t.Fatal("exec :dynamics must switch to the dynamics page")
	}
	v := m.View()
	if !strings.Contains(v, ":dynamics") || !strings.Contains(v, "BACKBONE") || !strings.Contains(v, "rdf-owl-kg") {
		t.Fatalf("dynamics page should render the map bands + nodes: %q", v)
	}
}

func TestExecDynamicsScaleFiltersResolution(t *testing.T) {
	g := grammar.Graph{
		Layers: []grammar.Layer{{ID: "L", Label: "Backbone"}},
		Nodes: []grammar.Node{
			{ID: "hi-res", Label: "deep", Layer: "L", Status: "asserted", Res: "5",
				AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
			{ID: "overview-node", Label: "top", Layer: "L", Status: "asserted", Res: "1",
				AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
		},
	}
	m := New("REINS").FoldDynamics(g, false).Exec("dynamics overview")
	if m.Page != PageDynamics || m.DynScale != 1 {
		t.Fatalf("exec :dynamics overview must set page+scale=1: page=%d scale=%d", m.Page, m.DynScale)
	}
	v := m.View()
	if strings.Contains(v, "hi-res") {
		t.Fatalf("overview scale must hide res-5 nodes: %q", v)
	}
	if !strings.Contains(v, "overview-node") {
		t.Fatalf("overview scale must keep res-1 nodes: %q", v)
	}
}

func TestTasksPageRenders(t *testing.T) {
	m := New("REINS").FoldTasks([]grammar.Task{
		{TaskID: "x-1", Stage: "S6", AIR: map[string]string{"task_id": "ok", "stage": "ok", "no_go": "ok"}},
	}, false)
	m.Page = PageTasks
	v := m.View()
	if !strings.Contains(v, "task registry") || !strings.Contains(v, "x-1") || !strings.Contains(v, "S6") || !strings.Contains(v, "TASK") {
		t.Fatalf("tasks page should render the registry context + header + rows: %q", v)
	}
}
