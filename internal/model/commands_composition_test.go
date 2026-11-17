package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
	"github.com/hapax-systems/reins/internal/layout"
)

func commandCompositionModel() Model {
	m := New("REINS")
	m.Page, m.Width, m.Height, m.AIR = PageCommands, 200, 40, false
	m.Commands = []grammar.Command{
		{Verb: "resume", Target: "lane-a", SessionRole: "lane-a", Status: "ok"},
		{Verb: "close", Target: "task-a", TaskID: "task-a", Status: "refused"},
	}
	m.Tasks = []grammar.Task{{TaskID: "task-a"}, {TaskID: "task-b"}}
	m.Sessions = []grammar.Session{{Role: "lane-a"}, {Role: "lane-b"}}
	return m
}

func TestCommandsDynamicCompositionOwnsLedger(t *testing.T) {
	for _, width := range []int{40, 80, 200} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := commandCompositionModel()
			m.Width = width
			spec := m.composePage(width, 16)
			if spec == nil || spec.Split == nil {
				t.Fatal("commands must compose a ledger-primary split at every width")
			}
			primary := ansi.Strip(layout.Render(spec.Split.Primary, width, 16))
			if !strings.Contains(primary, "WITNESSED LEDGER") || !strings.Contains(primary, "resume") {
				t.Fatalf("primary must be witnessed command rows, got %q", primary)
			}
			if isSessionAnchoredPage(PageCommands) || !m.composesViaAlgebra() {
				t.Fatal("commands must own its algebra and cursor")
			}
		})
	}
}

func TestCommandsDynamicCatalogHasNoInlineLedger(t *testing.T) {
	m := commandCompositionModel()
	s := ansi.Strip(m.renderCommandCatalog(180))
	if strings.Contains(s, "WITNESSED LEDGER") || strings.Contains(s, "lane-a") {
		t.Fatal("catalog must remain a sub-lens, not duplicate live ledger rows")
	}
}

func TestCommandsDynamicTypedBrushingBothDirections(t *testing.T) {
	m := commandCompositionModel()
	if got := m.commandConnector().JoinKey; got != "session_role -> role" {
		t.Fatal(got)
	}
	m = step(m, "j")
	if m.commandLedgerFocus != 1 || m.commandConnector().JoinKey != "task_id -> task_id" {
		t.Fatal("ledger must brush exact task")
	}
	if got := ansi.Strip(m.commandReferenceBody(120, 10)); !strings.Contains(got, "▶") || !strings.Contains(got, "task-a") {
		t.Fatal(got)
	}
	m = step(step(m, "tab"), "g")
	if m.commandLedgerFocus != 0 || m.selectedCommandReference().value != "lane-a" {
		t.Fatal("reference must brush command back")
	}
	if m.SFocus != 0 || m.Focus != 0 || m.CommandFocus != 0 {
		t.Fatal("unrelated cursors changed")
	}
	if _, ok := splitPairForPage(PageCommands); ok {
		t.Fatal("retired authored pair remains")
	}
}

func TestCommandsDynamicRefusal(t *testing.T) {
	for name, change := range map[string]func(*Model){
		"command dark":           func(m *Model) { m.CommandsDark = true },
		"task dark":              func(m *Model) { m.TasksDark = true },
		"target is not ref":      func(m *Model) { m.Commands[0].TaskID = "" },
		"near miss":              func(m *Model) { m.Commands[0].TaskID = "task-a-extra" },
		"wrong type":             func(m *Model) { m.Commands[0].TaskID = "lane-a" },
		"duplicate destination":  func(m *Model) { m.Tasks = append(m.Tasks, m.Tasks[0]) },
		"AIR command denied":     func(m *Model) { m.AIR = true; m.Tasks[0].AIR = map[string]string{"task_id": "ok"} },
		"AIR destination denied": func(m *Model) { m.AIR = true; m.Commands[0].AIR = map[string]string{"task_id": "ok"} },
	} {
		t.Run(name, func(t *testing.T) {
			m := commandCompositionModel()
			m.Commands = m.Commands[1:]
			change(&m)
			if len(m.commandReferences()) != 0 || m.commandConnector().JoinKey != "" || m.commandConnector().Verdict != "Door" {
				t.Fatal("invalid input minted a join")
			}
		})
	}
	for _, dark := range []bool{false, true} {
		m := commandCompositionModel()
		m.Commands = m.Commands[:1]
		m.SessionsDark = dark
		if !dark {
			m.Sessions = append(m.Sessions, m.Sessions[0])
		}
		if len(m.commandReferences()) != 0 {
			t.Fatal("ambiguous/dark session joined")
		}
	}
	m := commandCompositionModel()
	m.Commands = m.Commands[:1]
	m.AIR = true
	m.Commands[0].AIR = map[string]string{"session_role": "ok"}
	if len(m.commandReferences()) != 0 {
		t.Fatal("AIR-denied session joined")
	}
	m.Sessions[0].AIR = map[string]string{"role": "ok"}
	if m.commandConnector().JoinKey != "session_role -> role" {
		t.Fatal("permitted session join lost")
	}
	m.Commands = []grammar.Command{{TaskID: "task-a", AIR: map[string]string{"task_id": "ok"}}}
	m.Tasks[0].AIR = map[string]string{"task_id": "ok"}
	if m.commandConnector().JoinKey != "task_id -> task_id" {
		t.Fatal("permitted task join lost")
	}
}

func TestCommandsDynamicCatalogTypedInspectionNeverDispatches(t *testing.T) {
	for _, verb := range []string{"resume", "dispatch"} {
		t.Run(verb, func(t *testing.T) {
			m := commandCompositionModel()
			m.Commands[0].Verb = verb
			m.WiredVerbs = map[string]bool{verb: true}
			next, effect := m.Update(key("enter"))
			m = next.(Model)
			if effect != nil {
				t.Fatal("inspection emitted a command")
			}
			if !m.commandCatalog || m.PendingCommand != nil {
				t.Fatal("inspection missing or dispatched")
			}
			out := ansi.Strip(m.commandCatalogDetail(120))
			if !strings.Contains(out, "CATALOG REF") || strings.Contains(out, "switch window") {
				t.Fatal(out)
			}
			for _, width := range []int{80, 200} {
				spec := m.composePage(width, 20)
				if spec.Split == nil {
					t.Fatal("catalog lost the only-split root", width)
				}
				if !strings.Contains(ansi.Strip(layout.Render(spec.Split.Primary, width, 20)), "WITNESSED LEDGER") {
					t.Fatal("typed catalog displaced the ledger", width)
				}
				shown := ansi.Strip(layout.Render(spec, width, 20))
				if !strings.Contains(shown, "CATALOG REF") {
					t.Fatal("typed catalog hidden", width, shown)
				}
			}
			m = step(m, "esc")
			if m.commandCatalog {
				t.Fatal("Esc must restore ledger")
			}
		})
	}
	for _, verb := range []string{"disp", "sessions", "unknown", ""} {
		m := commandCompositionModel()
		m.Commands[0].Verb = verb
		m = step(m, "enter")
		if m.commandCatalog || !strings.Contains(m.Status, "use c") {
			t.Fatal("untyped/alias ref admitted", verb)
		}
	}
	m := commandCompositionModel()
	m.AIR = true
	m = step(m, "enter")
	if m.commandCatalog {
		t.Fatal("AIR denied verb must not open ref")
	}
	m.AIR = false
	m = step(m, "enter")
	m.CommandsDark = true
	if got := m.commandCatalogDetail(120); !strings.Contains(got, "unavailable") {
		t.Fatal("stale catalog reference", got)
	}
	m = commandCompositionModel()
	m.AIR = true
	m.Commands[0].AIR = map[string]string{"verb": "ok"}
	m = step(m, "enter")
	if !m.commandCatalog || strings.Contains(m.commandCatalogDetail(120), "lane-a") {
		t.Fatal("typed AIR inspection leaked target")
	}
	m.Commands[0].AIR["verb"] = "deny"
	if !strings.Contains(m.commandCatalogDetail(120), "unavailable") {
		t.Fatal("changed AIR permission ignored")
	}
}

func TestCommandsDynamicFoldIdentity(t *testing.T) {
	m := commandCompositionModel()
	m.commandLedgerFocus = 1
	next := []grammar.Command{m.Commands[1], m.Commands[0]}
	next[0].Status = "ok"
	if got := m.FoldCommands(next, "absent", "verified", false).commandLedgerFocus; got != 0 {
		t.Fatal("unique projection selection lost", got)
	}
	for name, rows := range map[string][]grammar.Command{"removed": next[1:], "duplicate": {next[0], next[0]}, "empty": nil} {
		if got := m.FoldCommands(rows, "absent", "unknown", false).commandLedgerFocus; got != -1 {
			t.Fatal(name, got)
		}
	}
	m.Commands = append(m.Commands, m.Commands[1])
	if got := m.FoldCommands(next, "absent", "unknown", false).commandLedgerFocus; got != -1 {
		t.Fatal("old ambiguity invented identity", got)
	}
	m = commandCompositionModel()
	if got := m.FoldCommands(m.Commands, "absent", "unknown", true).commandLedgerFocus; got != -1 {
		t.Fatal("dark retained live focus")
	}
}

func TestCommandsDynamicFocusMover(t *testing.T) {
	m := commandCompositionModel().focusTo(1)
	if m.commandLedgerFocus != 1 || m.CommandFocus != 0 {
		t.Fatal("moved catalog cursor from ledger", m.curFocus())
	}
	m.commandCatalog = true
	m = m.focusTo(2)
	if m.CommandFocus != 2 || m.commandLedgerFocus != 1 {
		t.Fatal("catalog cursor lost independence")
	}
}

func TestCommandsDynamicAIRAndTail(t *testing.T) {
	m := commandCompositionModel()
	m.AIR = true
	m.Commands = []grammar.Command{{Verb: "resume", Target: "secret-target", TaskID: "secret-task", SessionRole: "secret-lane"}}
	m.Tasks = []grammar.Task{{TaskID: "secret-task"}}
	m.Sessions = []grammar.Session{{Role: "secret-lane"}}
	if out := ansi.Strip(m.View()); strings.Contains(out, "secret-") {
		t.Fatal("AIR leak", out)
	}
	m = commandCompositionModel()
	m.Commands = append(m.Commands, m.Commands...)
	m.Commands = append(m.Commands, m.Commands...)
	m.commandLedgerFocus = len(m.Commands) - 1
	out := ansi.Strip(m.commandLedgerBody(120, 6))
	if strings.Contains(strings.ToLower(out), "score") || strings.Contains(out, "0.5") {
		t.Fatal("fabricated display scalar", out)
	}
	if !strings.Contains(out, "6 earlier / 0 later hidden") || !strings.Contains(out, "▶") {
		t.Fatal(out)
	}
	for _, width := range []int{3, 40, 80, 200} {
		out := layout.Render(m.composePage(width, 6), width, 6)
		if len(strings.Split(out, "\n")) != 6 {
			t.Fatal("height")
		}
		for _, line := range strings.Split(out, "\n") {
			if ansi.StringWidth(line) != width {
				t.Fatal("width", width, line)
			}
		}
	}
}
