package model

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
	"github.com/hapax-systems/reins/internal/relate"
)

func scopeFixture() Model {
	m := New("R").EnableScope()
	m.Width, m.Height = 170, 46
	m = m.FoldTasks([]grammar.Task{
		{TaskID: "task-alpha", Owner: "owner-private", Stage: "S6", Freshness: 1},
		{TaskID: "task-beta", Owner: "owner-private", Stage: "S5", Freshness: 1},
		{TaskID: "task-gamma", Owner: "other-owner", Stage: "S4", Freshness: 1},
	}, false)
	return m.Fold([]grammar.Event{
		{TS: "2026-11-04T00:00:00Z", Subject: "task-alpha", Kind: "claim", Actor: "actor-private"},
		{TS: "2026-11-04T00:00:01Z", Subject: "unrelated", Kind: "review"},
		{TS: "2026-11-04T00:00:02Z", Subject: "unrelated", Kind: "claim"},
	}, false)
}

func scopeKey(m Model, key string) Model {
	k := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	if key == "tab" {
		k = tea.KeyMsg{Type: tea.KeyTab}
	}
	n, _ := m.Update(k)
	return n.(Model)
}

func TestScopeIndependentCursorsAndLensSwitch(t *testing.T) {
	m := scopeFixture()
	m = scopeKey(m, "j") // first tasks -> beta
	m = scopeKey(m, "tab")
	m = scopeKey(m, "t")                // both lenses tasks
	m = scopeKey(scopeKey(m, "j"), "j") // second -> gamma
	first, _ := m.scopeLens(0).FocusedTask()
	second, _ := m.scopeLens(1).FocusedTask()
	if first.TaskID != "task-beta" || second.TaskID != "task-gamma" {
		t.Fatalf("shared cursor: %s / %s", first.TaskID, second.TaskID)
	}
	m = scopeKey(scopeKey(m, "e"), "j")
	m = scopeKey(m, "t")
	second, _ = m.scopeLens(1).FocusedTask()
	if second.TaskID != "task-gamma" {
		t.Fatal("switching lenses lost local cursor")
	}
	m = scopeKey(m, "e")
	if ev, _ := m.scopeLens(1).FocusedEvent(); ev.Subject != "unrelated" {
		t.Fatal("event cursor was not retained")
	}
	m = scopeKey(m, "tab")
	if m.scopeFocus != 0 || m.Focus != 0 || m.EFocus != 2 {
		t.Fatal("Scope changed Docket's cursors")
	}
}

func TestScopeMultiRuneKeys(t *testing.T) {
	for _, tt := range []struct {
		keys       string
		start, end int
	}{
		{"jj", 0, 2},
		{"kk", 2, 0},
		{"jk", 2, 1}, // j clamps at the last row, then k must still move up.
	} {
		t.Run(tt.keys, func(t *testing.T) {
			m := scopeFixture()
			m.scopeFrames[0].taskCursor = tt.start
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.keys)})
			got := next.(Model)
			if got.scopeFrames[0].taskCursor != tt.end || cmd != nil || got.scopeFrames[1] != m.scopeFrames[1] {
				t.Fatalf("multi-rune %q lost input or changed the other frame: %+v", tt.keys, got.scopeFrames)
			}
		})
	}
	t.Run("tq", func(t *testing.T) {
		m := scopeKey(scopeFixture(), "tab")
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tq")})
		if next.(Model).scopeFrames[1].page != PageTasks || cmd == nil {
			t.Fatal("tq did not switch lens and quit")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatal("tq lost the terminal quit command")
		}
	})
	t.Run("paste", func(t *testing.T) {
		m := scopeKey(scopeFixture(), "tab")
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tq"), Paste: true})
		if !reflect.DeepEqual(next, m) || cmd != nil {
			t.Fatal("pasted runes became Scope commands")
		}
	})
}

func TestScopeDerivedJoinAndNoJoin(t *testing.T) {
	m := scopeFixture()
	c := m.scopeConnector()
	if c.JoinKey != "task_id -> subject" || c.Verdict != relate.VerdictStanding.String() {
		t.Fatalf("exact task/event referent not joined: %+v", c)
	}
	m = scopeKey(m, "j")
	if c = m.scopeConnector(); c.JoinKey != "" || c.Verdict != relate.VerdictDoor.String() {
		t.Fatalf("unrelated rows invented a join: %+v", c)
	}
	if !strings.Contains(ansi.Strip(m.View()), "unlinked · no join") {
		t.Fatal("no-join qualification missing")
	}
	if out := ansi.Strip(m.View()); strings.Contains(out, "[/] filter") || strings.Contains(out, "[y] yank") {
		t.Fatal("Scope advertises Docket-only controls")
	}
	m = scopeKey(scopeKey(m, "tab"), "t")
	if c = m.scopeConnector(); !strings.Contains(c.Relation, "owner") || c.JoinKey != "owner -> owner" {
		t.Fatalf("shared owner facet not derived: %+v", c)
	}
}

func TestScopeAIRGatesBothSidesAndSelection(t *testing.T) {
	for _, deniedSide := range []string{"task", "event", "both"} {
		t.Run(deniedSide, func(t *testing.T) {
			m := scopeFixture()
			m.Tasks[0].AIR = map[string]string{"task_id": "ok"}
			m.Events[0].AIR = map[string]string{"subject": "ok"}
			if deniedSide != "event" {
				m.Tasks[0].AIR["task_id"] = "deny"
			}
			if deniedSide != "task" {
				m.Events[0].AIR["subject"] = "deny"
			}
			m = scopeKey(m, "a")
			if c := m.scopeConnector(); c.JoinKey != "" {
				t.Fatalf("denied join leaked topology: %+v", c)
			}
			for _, width := range []int{170, 60} {
				m.Width = width
				for range 2 {
					out := ansi.Strip(m.View())
					for _, denied := range []string{"owner-private", "actor-private", "task-beta", "task-gamma"} {
						if strings.Contains(out, denied) {
							t.Fatalf("AIR selected-row leak %q", denied)
						}
					}
					if deniedSide == "both" && strings.Contains(out, "task-alpha") {
						t.Fatal("denied referent leaked")
					}
					m = scopeKey(m, "tab")
				}
			}
		})
	}
}

func TestScopeMissingStaleAndFailedReadsStayDark(t *testing.T) {
	for _, kind := range []string{"unobserved", "stale", "failed", "empty"} {
		t.Run(kind, func(t *testing.T) {
			m := scopeFixture()
			switch kind {
			case "unobserved":
				m.scopeReadAt[0] = time.Time{}
			case "stale":
				m.scopeNow = m.scopeNow.Add(scopeReadMaxAge + time.Second)
			case "failed":
				m = m.FoldTasks(m.Tasks, true)
				m.TasksError = "private-read-error"
			case "empty":
				m = m.FoldTasks(nil, false)
			}
			out := ansi.Strip(m.View())
			if m.scopeConnector().JoinKey != "" || !strings.Contains(out, "no join") {
				t.Fatal("unavailable selection asserted a join")
			}
			if kind != "empty" && (!strings.Contains(out, "DARK") || strings.Contains(out, "private-read-error")) {
				t.Fatal("missing read-state disclosure or unsafe error")
			}
		})
	}
}

func TestScopeDeviceFramesAndGeometry(t *testing.T) {
	for _, width := range []int{170, 100, 60, 3, 1} {
		for _, height := range []int{46, 24, 3, 1} {
			m := scopeFixture()
			m.Width, m.Height, m.Demo = width, height, true
			m = scopeKey(m, "tab")
			out := ansi.Strip(m.View())
			lines := strings.Split(out, "\n")
			if len(lines) != height {
				t.Fatalf("height %d != %d", len(lines), height)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != width {
					t.Fatalf("width %d != %d", ansi.StringWidth(line), width)
				}
			}
			if width >= 60 && height >= 24 {
				if !strings.Contains(out, "frame 2 · events · focused") || !strings.Contains(out, "DEMO fixture not live") {
					t.Fatalf("focus/fixture lost at %dx%d", width, height)
				}
				if (width >= 145) != strings.Contains(out, "frame 1 · tasks") {
					t.Fatal("wrong device pane count")
				}
			}
		}
	}
	m := scopeFixture()
	m.DeviceOverride = DeviceCompact
	if strings.Contains(ansi.Strip(m.View()), "frame 2 · events") {
		t.Fatal("explicit compact device ignored")
	}
}

func TestScopeCannotEnterCommandOrShareSelection(t *testing.T) {
	m := scopeFixture()
	m.WiredVerbs = map[string]bool{"resume": true}
	for _, key := range []string{":", "v", "c", "y", "V", "enter", "resume task-alpha"} {
		m = scopeKey(m, key)
	}
	m = m.Exec("resume task-alpha")
	if m.PendingCommand != nil || m.Mode != ModeNormal || len(m.Ring) != 0 || len(m.Sel.Members) != 0 {
		t.Fatal("Scope entered an effect/selection mode")
	}
}

func TestScopeReadFoldClampsBothLocalCursors(t *testing.T) {
	m := scopeFixture()
	m = scopeKey(scopeKey(m, "G"), "tab")
	m = scopeKey(scopeKey(scopeKey(m, "G"), "t"), "G")
	m = m.FoldTasks(m.Tasks[:1], false).Fold(m.Events[:1], false)
	for i := range 2 {
		if m.scopeFrames[i].taskCursor != 0 || m.scopeFrames[i].eventCursor != 0 {
			t.Fatal("fold left an out-of-range local selection")
		}
	}
}

func TestScopeQuitAndMouseIgnore(t *testing.T) {
	m := scopeFixture()
	for _, key := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune{'q'}}, {Type: tea.KeyCtrlC}} {
		next, cmd := m.Update(key)
		if !reflect.DeepEqual(next, m) || cmd == nil {
			t.Fatal("Scope quit changed state or lost quit command")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatal("Scope quit did not reach the terminal")
		}
	}
	for _, button := range []tea.MouseButton{tea.MouseButtonLeft, tea.MouseButtonWheelUp, tea.MouseButtonWheelDown} {
		next, cmd := m.Update(tea.MouseMsg{X: 10, Y: 5, Button: button, Action: tea.MouseActionPress})
		if cmd != nil || !reflect.DeepEqual(next, m) {
			t.Fatalf("Scope mouse changed state: %v", button)
		}
	}
}
