package model

import (
	"fmt"
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

func TestViewTinyTerminalDoesNotDefaultToProbeFrame(t *testing.T) {
	m := New("REINS").Fold(evs(), false)
	m.Width, m.Height = 39, 11
	lines := strings.Split(m.View(), "\n")
	if len(lines) != 11 {
		t.Fatalf("39x11 must render exactly 11 lines, got %d", len(lines))
	}
	for i, ln := range lines {
		if got := ansi.StringWidth(ln); got > 39 {
			t.Fatalf("tiny terminal line %d exceeds 39 cols: %d %q", i, got, ln)
		}
	}
}

func TestViewVeryShortFramesFitAndPreserveCommandAffordance(t *testing.T) {
	for _, h := range []int{1, 2, 3, 4, 5, 6, 7} {
		m := New("REINS").Fold(evs(), false)
		m.Width, m.Height = 46, h
		frame := ansi.Strip(m.View())
		lines := strings.Split(frame, "\n")
		if len(lines) != h {
			t.Fatalf("46x%d must render exactly %d lines, got %d:\n%s", h, h, len(lines), frame)
		}
		for i, ln := range lines {
			if got := ansi.StringWidth(ln); got > m.Width {
				t.Fatalf("46x%d line %d exceeds frame width: %d %q", h, i, got, ln)
			}
		}
		if !strings.Contains(frame, ":") {
			t.Fatalf("46x%d compact frame should keep command access visible:\n%s", h, frame)
		}
	}
}

func TestViewMissingSizeUsesProbeDefaultFrame(t *testing.T) {
	m := New("REINS").Fold(evs(), false)
	lines := strings.Split(m.View(), "\n")
	if len(lines) != 40 {
		t.Fatalf("missing size should use 120x40 probe default, got %d lines", len(lines))
	}
	for i, ln := range lines {
		if got := ansi.StringWidth(ln); got > 120 {
			t.Fatalf("probe default line %d exceeds 120 cols: %d %q", i, got, ln)
		}
	}
}

func TestFitBlockWithSlackProviderReceivesExactRemainingRows(t *testing.T) {
	seen := 0
	lines := fitBlockWithSlackFn("alpha\nbeta", 24, 7, func(maxRows int) []string {
		seen = maxRows
		return []string{"slack-1", "slack-2", "slack-3", "slack-4", "slack-5", "extra"}
	})
	if seen != 5 {
		t.Fatalf("slack provider should receive exact remaining rows, got %d", seen)
	}
	if len(lines) != 7 {
		t.Fatalf("fit block should return exact height, got %d", len(lines))
	}
	joined := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"alpha", "beta", "slack-1", "slack-5"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("fit block missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "extra") {
		t.Fatalf("fit block should not consume more slack than requested:\n%s", joined)
	}
}

func TestWideEventsUseContextConstraintSpace(t *testing.T) {
	m := New("REINS").Fold([]grammar.Event{
		{TS: "10:00", Kind: "coord_dispatch.launch_started", Subject: "task-a", Actor: "cx-a", Summary: "", Score: 0.21,
			AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "summary": "ok"}},
		{TS: "10:05", Kind: "coord_dispatch.launch_failed", Subject: "task-a", Actor: "cx-a", Summary: "blocked", Score: 0.82,
			AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "summary": "ok"}},
	}, false)
	m.Width, m.Height = 220, 32
	m.EFocus = 1

	v := ansi.Strip(m.View())
	for _, want := range []string{"EVENT CONTEXT", "constraints", "legal next", ":intent open-trace", "same subj"} {
		if !strings.Contains(v, want) {
			t.Fatalf("wide events view should use negative space for %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "Z2▸:events") {
		t.Fatalf("wide events should use the integrated context pane, not the narrow side rail:\n%s", v)
	}
}

func TestWideTasksUseWorkDomainContext(t *testing.T) {
	m := New("REINS").FoldTasks([]grammar.Task{{
		TaskID: "release-blocked-task", Stage: "S7_RELEASE", PriorStage: "S6_IMPL", PredictedStage: "hold",
		Owner: "cx-p0", Criticality: "crit", Freshness: 0.12, AuthorityCase: "CASE-1",
		NoGo: "docs_mutation_authorized,implementation_authorized", RelCount: 0,
		AIR: map[string]string{
			"task_id": "ok", "stage": "ok", "prior_stage": "ok", "predicted_stage": "ok",
			"owner": "ok", "criticality": "ok", "freshness": "ok", "authority_case": "ok", "no_go": "ok", "rel_count": "ok",
		},
	}}, false)
	m.Width, m.Height, m.Page = 220, 32, PageTasks

	v := ansi.Strip(m.View())
	for _, want := range []string{"WORK DOMAIN", "release blocked", "authority", "relationships", ":intent show-route", "governed COMMAND route required"} {
		if !strings.Contains(v, want) {
			t.Fatalf("wide tasks view should use negative space for %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "Z2▸:tasks") {
		t.Fatalf("wide tasks should use the integrated work-domain pane, not the narrow side rail:\n%s", v)
	}
}

func TestWideSessionsUseLaneReadinessContext(t *testing.T) {
	m := New("REINS").FoldSessions([]grammar.Session{
		{Role: "cx-ready", Session: "tmux-ready", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.82, ClaimedTask: "task-a", RouteID: "codex.headless.full", RouteMode: "headless", RouteProfile: "full", RouteBindingState: "policy_only", RouteEvidenceRef: "route-decisions.jsonl:rd-test", OutputAgeS: 10, RelayAgeS: 20,
			AIR: map[string]string{"role": "ok", "session": "deny", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "deny", "route_id": "ok", "mode": "ok", "profile": "ok", "route_binding_state": "ok", "route_evidence_ref": "ok", "output_age_s": "ok", "relay_age_s": "ok"}},
		{Role: "cx-stale", Platform: "claude", State: "active", Readiness: "stale", Blocker: "stale_relay", Attention: 0.55, RelayAgeS: 4000,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "deny", "output_age_s": "ok", "relay_age_s": "ok"}},
	}, false)
	m.Width, m.Height, m.Page, m.AIR = 220, 32, PageSessions, true

	v := ansi.Strip(m.View())
	for _, want := range []string{"LANE READINESS", "claim-ready", "route binding", "codex.headless.full", "policy-only", "headless/full", "route-decisions.jsonl:rd-test", "fleet context", ":intent resume", "governed COMMAND route required", "no transcript"} {
		if !strings.Contains(v, want) {
			t.Fatalf("wide sessions view should use negative space for %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "tmux-ready") || strings.Contains(v, "task-a") {
		t.Fatalf("wide sessions panel must honor AIR for tmux/task refs:\n%s", v)
	}
	if strings.Contains(v, "Z2▸:sessions") {
		t.Fatalf("wide sessions should use the integrated lane-readiness pane, not the narrow side rail:\n%s", v)
	}
}

func TestWideContextTallFrameUsesSlackPayloadBeforeBlankGround(t *testing.T) {
	m := New("REINS").
		FoldTasks([]grammar.Task{{
			TaskID: "release-blocked-task", Stage: "S7_RELEASE", PredictedStage: "hold", Owner: "cx-ready", Criticality: "crit",
			AIR: map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok", "owner": "ok", "criticality": "ok"},
		}}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-ready", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.82, ClaimedTask: "release-blocked-task",
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "ok"},
		}}, false)
	m.Width, m.Height, m.Page = 220, 52, PageTasks

	v := ansi.Strip(m.View())
	for _, want := range []string{"CONTEXT SLACK", "sources", "task mix", "release/hold"} {
		if !strings.Contains(v, want) {
			t.Fatalf("tall wide context should fill spare rows with %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "lane        cx-ready") {
		t.Fatalf("non-split task slack must not imply unrelated selected session lane context:\n%s", v)
	}
}

func TestReferencePageTallFrameUsesScreenSlack(t *testing.T) {
	m := New("REINS").
		FoldTasks([]grammar.Task{{TaskID: "task-a", Criticality: "warn", AIR: map[string]string{"task_id": "ok", "criticality": "ok"}}}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-ready", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.82, Alive: true,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "alive": "ok", "idle": "ok", "stalled": "ok"},
		}}, false)
	m.Width, m.Height, m.Page = 220, 80, PageLegend

	v := ansi.Strip(m.View())
	for _, want := range []string{"LEGEND", "SCREEN SLACK", "sources", "layout", "cursor", "contract"} {
		if !strings.Contains(v, want) {
			t.Fatalf("tall reference page should fill spare rows with %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "lane        cx-ready") {
		t.Fatalf("non-split reference slack must not imply unrelated selected session lane context:\n%s", v)
	}
}

func TestReferenceMainSlackIsRoleSpecificAndContextOwnsTrust(t *testing.T) {
	m := New("REINS").
		FoldTasks([]grammar.Task{{TaskID: "task-a", Criticality: "warn", AIR: map[string]string{"task_id": "ok", "criticality": "ok"}}}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-ready", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.82, Alive: true,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "alive": "ok", "idle": "ok", "stalled": "ok"},
		}}, false)
	m.Width, m.Height, m.Page = 220, 80, PageCommands

	main := ansi.Strip(strings.Join(m.slackRowsForPage(110, 24, slackSlotReferenceMain), "\n"))
	for _, bad := range []string{"trust      ", "sources    "} {
		if strings.Contains(main, bad) {
			t.Fatalf("reference-main slack should not duplicate context %q rows:\n%s", bad, main)
		}
	}
	for _, want := range []string{"screen", "command grammar", "cycle", "moves", "pairing"} {
		if !strings.Contains(main, want) {
			t.Fatalf("reference-main slack missing %q:\n%s", want, main)
		}
	}

	ctx := ansi.Strip(strings.Join(m.slackRowsForPage(110, 24, slackSlotWideContext), "\n"))
	for _, want := range []string{"trust", "sources", "command classes"} {
		if !strings.Contains(ctx, want) {
			t.Fatalf("wide-context slack should retain %q:\n%s", want, ctx)
		}
	}
}

func TestContextRailPreservesSemanticSlackLabels(t *testing.T) {
	m := New("REINS")
	m.Width, m.Height, m.Page = 220, 80, PageSurfaces
	v := ansi.Strip(m.View())
	for _, want := range []string{"CONTEXT SLACK", "surface classes", "selected surface", "mode:4", "open=[:]"} {
		if !strings.Contains(v, want) {
			t.Fatalf("wide context rail should preserve %q:\n%s", want, v)
		}
	}
	for _, bad := range []string{"surface clas…", "selected sur…"} {
		if strings.Contains(v, bad) {
			t.Fatalf("wide context rail should not clip semantic label %q:\n%s", bad, v)
		}
	}
}

func TestRegistrySlackRowsArePageSpecific(t *testing.T) {
	for _, tt := range []struct {
		name string
		page int
		want []string
	}{
		{name: "commands", page: PageCommands, want: []string{"read:18", "arg verbs", "mutation surface"}},
		{name: "windows", page: PageWindows, want: []string{"registered:19", "linked:8", "source-only:11"}},
		{name: "surfaces", page: PageSurfaces, want: []string{"mode:4", "projection:12", "open=[:]"}},
		{name: "intent", page: PageIntent, want: []string{"resume", "9 targets", "preview only", "{{sel.*}}"}},
		{name: "help", page: PageHelp, want: []string{"split marks", "contract", "no buried screens"}},
		{name: "lifecycles", page: PageLifecycles, want: []string{"rows:0", "fallback:", "support-only"}},
		{name: "yard", page: PageYard, want: []string{"S7:1", "holds:1+0 hidden"}},
		{name: "readiness", page: PageReadiness, want: []string{"gates", "lane gates"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := New("REINS").
				FoldTasks([]grammar.Task{{TaskID: "task-a", Stage: "S7_RELEASE", PredictedStage: "hold", Criticality: "crit", AIR: map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok", "criticality": "ok"}}}, false).
				FoldSessions([]grammar.Session{{Role: "cx-ready", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.82, Alive: true,
					AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "alive": "ok", "idle": "ok", "stalled": "ok"}}}, false)
			m.Page = tt.page
			rows := ansi.Strip(strings.Join(m.slackRowsForPage(110, 24, slackSlotReferenceMain), "\n"))
			for _, want := range tt.want {
				if !strings.Contains(rows, want) {
					t.Fatalf("%s slack missing %q:\n%s", tt.name, want, rows)
				}
			}
		})
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

func TestSessionsPageRendersAndNavigatesIndependently(t *testing.T) {
	ss := []grammar.Session{
		{Role: "cx-p0", Session: "tmux-p0", Platform: "codex", State: "active", Readiness: "live", Blocker: "no_claim", Attention: 0.74, Alive: true,
			AIR: map[string]string{"role": "ok", "session": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "alive": "ok", "idle": "ok", "stalled": "ok", "claimed_task": "deny", "output_age_s": "ok", "relay_age_s": "ok"}},
		{Role: "cx-fugu-1", Session: "tmux-fugu", Platform: "codex", State: "idle", Readiness: "claim", Blocker: "none", Attention: 0.88, Alive: true, Idle: true, ClaimedTask: "PRIVATE-TASK",
			AIR: map[string]string{"role": "ok", "session": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "alive": "ok", "idle": "ok", "stalled": "ok", "claimed_task": "deny", "output_age_s": "ok", "relay_age_s": "ok"}},
	}
	m := New("REINS").FoldTasks([]grammar.Task{{TaskID: "task-1", AIR: map[string]string{"task_id": "ok"}}}, false).FoldSessions(ss, false)
	m.Width, m.Height = 120, 40
	m.Page, m.Focus = PageSessions, 0
	m = step(m, "j")
	if m.SFocus != 1 || m.Focus != 0 {
		t.Fatalf("session j should move SFocus only: SFocus=%d Focus=%d", m.SFocus, m.Focus)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{":sessions", "cx-fugu-1", "selected lane", "no transcript"} {
		if !strings.Contains(v, want) {
			t.Fatalf("sessions page missing %q:\n%s", want, v)
		}
	}
}

func TestFocusGutterBreathesWithoutMovingSelection(t *testing.T) {
	events := []grammar.Event{
		{TS: "10:00", Kind: "coord_dispatch.launch_started", Subject: "event-a", Actor: "cx-a", Score: 0.21, AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "summary": "ok", "score": "ok"}},
		{TS: "10:01", Kind: "coord_dispatch.launch_failed", Subject: "event-b", Actor: "cx-b", Score: 0.88, AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "summary": "ok", "score": "ok"}},
	}
	tasks := []grammar.Task{
		{TaskID: "task-a", Criticality: "ok", Stage: "S5", AIR: map[string]string{"task_id": "ok", "criticality": "ok", "stage": "ok"}},
		{TaskID: "task-b", Criticality: "crit", Stage: "S7", AIR: map[string]string{"task_id": "ok", "criticality": "ok", "stage": "ok"}},
	}
	sessions := []grammar.Session{
		{Role: "cx-a", Platform: "codex", State: "active", Readiness: "live", Blocker: "no_claim", Attention: 0.74, Alive: true,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "alive": "ok", "idle": "ok", "stalled": "ok", "claimed_task": "ok", "output_age_s": "ok", "relay_age_s": "ok"}},
		{Role: "cx-b", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88, Alive: true,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "alive": "ok", "idle": "ok", "stalled": "ok", "claimed_task": "ok", "output_age_s": "ok", "relay_age_s": "ok"}},
	}
	base := New("REINS").Fold(events, false).FoldTasks(tasks, false).FoldSessions(sessions, false)
	base.Width, base.Height = 120, 26

	for _, tc := range []struct {
		name   string
		page   int
		focus0 func(*Model)
		focus1 func(Model) int
		row    string
	}{
		{"events", PageEvents, func(m *Model) { m.EFocus = 1 }, func(m Model) int { return m.EFocus }, ansi.Strip(grammar.RenderEventRow(events[1], false))},
		{"tasks", PageTasks, func(m *Model) { m.Focus = 1 }, func(m Model) int { return m.Focus }, ansi.Strip(grammar.RenderTaskRow(tasks[1], false))},
		{"sessions", PageSessions, func(m *Model) { m.SFocus = 1 }, func(m Model) int { return m.SFocus }, ansi.Strip(grammar.RenderSessionRow(sessions[1], false))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m0 := base
			m0.Page = tc.page
			tc.focus0(&m0)
			before := tc.focus1(m0)
			v0 := ansi.Strip(m0.View())
			rowPrefix := ansi.Truncate(tc.row, 44, "")
			if !strings.Contains(v0, "▶"+rowPrefix) {
				t.Fatalf("%s selected row should use the first focus frame:\n%s", tc.name, v0)
			}
			m1 := m0
			m1.Beat = 1
			v1 := ansi.Strip(m1.View())
			if got := tc.focus1(m1); got != before {
				t.Fatalf("%s beat should not move focus: before=%d after=%d", tc.name, before, got)
			}
			if !strings.Contains(v1, "▸"+rowPrefix) {
				t.Fatalf("%s selected row should use the second focus frame without changing row identity:\n%s", tc.name, v1)
			}
		})
	}
}

func TestSessionListLivePulseUsesNonselectedGutter(t *testing.T) {
	ss := []grammar.Session{
		{Role: "cx-live", Platform: "codex", State: "active", Readiness: "live", Blocker: "no_claim", Attention: 0.74, Alive: true,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "alive": "ok", "idle": "ok", "stalled": "ok", "claimed_task": "ok", "output_age_s": "ok", "relay_age_s": "ok"}},
		{Role: "cx-focus", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88, Alive: true,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "alive": "ok", "idle": "ok", "stalled": "ok", "claimed_task": "ok", "output_age_s": "ok", "relay_age_s": "ok"}},
	}
	m := New("REINS").FoldSessions(ss, false)
	m.Width, m.Height, m.Page, m.SFocus = 120, 18, PageSessions, 1

	v0 := ansi.Strip(m.View())
	liveRow := ansi.Strip(grammar.RenderSessionRow(ss[0], false))
	focusRow := ansi.Strip(grammar.RenderSessionRow(ss[1], false))
	if !strings.Contains(v0, "· "+ansi.Truncate(liveRow, 44, "")) || !strings.Contains(v0, "▶"+ansi.Truncate(focusRow, 44, "")) {
		t.Fatalf("sessions page should put live pulse in nonselected gutter and keep focus cursor on selected row:\n%s", v0)
	}
	m.Beat = 2
	v2 := ansi.Strip(m.View())
	if !strings.Contains(v2, "• "+ansi.Truncate(liveRow, 44, "")) || !strings.Contains(v2, "▶"+ansi.Truncate(focusRow, 44, "")) {
		t.Fatalf("live gutter should animate with Beat without moving selected row:\n%s", v2)
	}
}

func TestTitleBarRendersChannelHotlistWithActivity(t *testing.T) {
	m := New("REINS").
		Fold(evFixture(), false).
		FoldTasks([]grammar.Task{
			{TaskID: "calm", Criticality: "ok", AIR: map[string]string{"task_id": "ok"}},
			{TaskID: "blocked", Criticality: "crit", AIR: map[string]string{"task_id": "ok"}},
		}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-hot", Readiness: "claim", Blocker: "none", Attention: 0.88,
			AIR: map[string]string{"role": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"},
		}}, false).
		FoldDynamics(grammar.Graph{Nodes: []grammar.Node{{ID: "n1", Res: "1"}}}, false)
	m.Width, m.Height, m.Page = 120, 40, PageSessions

	title := ansi.Strip(m.viewTitle(300))
	for _, want := range []string{"REINS", "win", "1.events:", "2.tasks:2!1", "‹3.sess:1!1›", "Y.yard:2!1", "R.ready:1!0", "I.obs", "C.caps:18c!12", "4.dyn:1@all", "E.epi", "5.help:ref", "6.cmds:22", "7.wins:19", "8.intent:9", "9.surf:35", "0.doms:17", "L.life:6", "?.legend:ref"} {
		if !strings.Contains(title, want) {
			t.Fatalf("title hotlist missing %q:\n%s", want, title)
		}
	}
	if ansi.StringWidth(m.viewTitle(70)) > 70 {
		t.Fatalf("title hotlist must truncate to width, got %d", ansi.StringWidth(m.viewTitle(70)))
	}
	compact := ansi.Strip(m.viewTitle(70))
	if !strings.Contains(compact, "‹3.sess:1!1›") || !strings.Contains(compact, "›") {
		t.Fatalf("compact title should preserve active tab with hidden-count marker:\n%s", compact)
	}
}

func TestWindowRegistrySeparatesEngineAndInstanceLifecycleWindows(t *testing.T) {
	ss, ok := windowForPage(PageSessions)
	if !ok || ss.Scope != "instance" || ss.Lifecycle != "sdlc" || ss.Kind != "fleet" {
		t.Fatalf("sessions window should be an instance SDLC fleet window, got %+v", ss)
	}
	help, ok := windowForPage(PageHelp)
	if !ok || help.Scope != "engine" || help.Lifecycle != "engine" {
		t.Fatalf("help window should be engine-scoped, got %+v", help)
	}

	out := New("REINS").Exec("wins")
	if out.Page != PageWindows {
		t.Fatalf(":wins should open the windows page, got page %d", out.Page)
	}
	if !strings.Contains(out.Status, "windows:") || !strings.Contains(out.Status, "engine 8") || !strings.Contains(out.Status, "instance 11") || !strings.Contains(out.Status, "intake") || !strings.Contains(out.Status, "routing") || !strings.Contains(out.Status, "sdlc") {
		t.Fatalf("window registry summary should expose scope and lifecycle split, got %q", out.Status)
	}
}

func TestSplitPairRegistryCoversRegisteredWindows(t *testing.T) {
	seen := map[int]bool{}
	for _, pair := range registeredSplitPairs() {
		if _, ok := windowForPage(pair.Page); !ok {
			t.Fatalf("split pair must target a registered window: %+v", pair)
		}
		if pair.Source == "" || pair.Target == "" || pair.Join == "" || pair.Mode == "" || pair.Contract == "" {
			t.Fatalf("split pair must declare source, target, join, mode, and contract: %+v", pair)
		}
		if pair.SourceCursor == "" || pair.TargetReactivity == "" || pair.TargetCursor == "" {
			t.Fatalf("split pair must declare cursor/reactivity traits: %+v", pair)
		}
		if !pair.SourceOwns("detail") || !pair.SourceOwns("resume") || !pair.SourceOwns("yank") {
			t.Fatalf("session-source split pair must declare source-owned verbs: %+v", pair)
		}
		seen[pair.Page] = true
	}
	for _, wnd := range registeredWindows() {
		if !seen[wnd.Page] {
			t.Fatalf("registered window %q must declare split-pair semantics", wnd.ID)
		}
	}
}

func TestSplitPairRegistryDeclaresRelationshipOperators(t *testing.T) {
	for page, want := range map[int]string{
		PageEvents:    "actor/task",
		PageTasks:     "claimed_task",
		PageSessions:  "role",
		PageYard:      "lane/task",
		PageReadiness: "role/claimed_task",
		PageIntake:    "actor/claimed_task",
		PageCaps:      "role/platform",
	} {
		pair, ok := splitPairForPage(page)
		if !ok || !pair.Reactive() || pair.Join != want {
			t.Fatalf("page %s should be a linked split pair by %q, got %+v", pageLabel(page), want, pair)
		}
	}
	for page, want := range map[int]string{
		PageDynamics:   "system topology",
		PageEpistemics: "evidence/provenance",
		PageIntent:     "explicit target",
		PageHelp:       "operator orientation",
		PageLegend:     "glyph decoding",
		PageCommands:   "command grammar",
		PageWindows:    "window topology",
		PageSurfaces:   "affordance registry",
		PageDomains:    "domain lens",
		PageLifecycles: "tenant lifecycle",
	} {
		pair, ok := splitPairForPage(page)
		if !ok || pair.Reactive() || pair.Join != want {
			t.Fatalf("page %s should be a source-only/reference split pair by %q, got %+v", pageLabel(page), want, pair)
		}
		if pair.Join == "reference" || strings.Contains(pair.Contract, "lane source held") {
			t.Fatalf("source-only split pair should declare a concrete relationship, got %+v", pair)
		}
	}
}

func TestSplitPairRegistryDeclaresControlTraits(t *testing.T) {
	for _, page := range []int{PageEvents, PageTasks, PageSessions} {
		pair, ok := splitPairForPage(page)
		if !ok {
			t.Fatalf("missing split pair for page %s", pageLabel(page))
		}
		if pair.SourceCursor != splitSourceSessionRow || pair.TargetReactivity != splitTargetLinked || pair.TargetCursor != splitTargetNone || pair.TargetScrollable {
			t.Fatalf("compact linked context should be source-linked without a scroll cursor, got %+v", pair)
		}
	}
	for _, page := range []int{PageYard, PageReadiness, PageCaps} {
		pair, ok := splitPairForPage(page)
		if !ok {
			t.Fatalf("missing split pair for page %s", pageLabel(page))
		}
		if pair.SourceCursor != splitSourceSessionRow || pair.TargetReactivity != splitTargetLinked || pair.TargetCursor != splitTargetScroll || !pair.TargetScrollable {
			t.Fatalf("reference-backed linked context should expose target scroll, got %+v", pair)
		}
	}
	intake, ok := splitPairForPage(PageIntake)
	if !ok || intake.TargetCursor != splitTargetIntake || !intake.TargetScrollable {
		t.Fatalf("intake split should declare its independent bucket cursor, got %+v", intake)
	}
	dynamics, ok := splitPairForPage(PageDynamics)
	if !ok || dynamics.SourceCursor != splitSourceAnchor || dynamics.TargetReactivity != splitTargetIndependent || dynamics.TargetCursor != splitTargetMapElement || !dynamics.TargetScrollable {
		t.Fatalf("dynamics split should hold source and expose map target cursor, got %+v", dynamics)
	}
	epistemics, ok := splitPairForPage(PageEpistemics)
	if !ok || epistemics.SourceCursor != splitSourceAnchor || epistemics.TargetReactivity != splitTargetIndependent || epistemics.TargetCursor != splitTargetEpistemic || !epistemics.TargetScrollable {
		t.Fatalf("epistemics split should hold source and expose evidence target cursor, got %+v", epistemics)
	}
	for _, page := range []int{PageIntent, PageHelp, PageLegend, PageCommands, PageWindows, PageSurfaces, PageDomains, PageLifecycles} {
		pair, ok := splitPairForPage(page)
		if !ok {
			t.Fatalf("missing split pair for page %s", pageLabel(page))
		}
		if pair.SourceCursor != splitSourceAnchor || pair.TargetReactivity != splitTargetIndependent || pair.TargetCursor != splitTargetScroll || !pair.TargetScrollable {
			t.Fatalf("source-only reference context should hold source and scroll target, got %+v", pair)
		}
	}
}

func TestSplitPairRegistryDeclaresPaneProfiles(t *testing.T) {
	cases := map[PaneProfile][]int{
		PaneLinkedCompact:      {PageEvents, PageTasks, PageSessions},
		PaneLinkedScrollable:   {PageYard, PageReadiness, PageCaps},
		PaneLinkedTargetCursor: {PageIntake},
		PaneAnchoredTarget:     {PageDynamics, PageEpistemics},
		PaneAnchoredReference:  {PageHelp, PageLegend, PageCommands, PageWindows, PageIntent, PageSurfaces, PageDomains, PageLifecycles},
	}
	for want, pages := range cases {
		for _, page := range pages {
			pair, ok := splitPairForPage(page)
			if !ok {
				t.Fatalf("missing split pair for page %s", pageLabel(page))
			}
			if got := pair.PaneProfile(); got != want {
				t.Fatalf("page %s profile=%s, want %s from %+v", pageLabel(page), got, want, pair)
			}
			if pair.PaneProfileLabel() == "" || strings.Contains(pair.PaneProfileLabel(), " ") {
				t.Fatalf("page %s should expose compact profile label, got %q", pageLabel(page), pair.PaneProfileLabel())
			}
			if pair.Relationship() == PaneRelationshipLinked && !pair.Reactive() {
				t.Fatalf("linked relationship must be reactive, got %+v", pair)
			}
			if pair.Relationship() == PaneRelationshipAnchored && pair.Reactive() {
				t.Fatalf("anchored relationship must be non-reactive, got %+v", pair)
			}
		}
	}
	for _, pair := range registeredSplitPairs() {
		switch pair.PaneProfile() {
		case PaneLinkedCompact:
			if !pair.Reactive() || pair.TargetCursor != splitTargetNone || pair.TargetScrollable {
				t.Fatalf("linked-compact invariant failed: %+v", pair)
			}
		case PaneLinkedScrollable:
			if !pair.Reactive() || pair.TargetCursor != splitTargetScroll || !pair.TargetScrollable {
				t.Fatalf("linked-scrollable invariant failed: %+v", pair)
			}
		case PaneLinkedTargetCursor:
			if !pair.Reactive() || !pair.TargetUsesNP() || !pair.TargetScrollable {
				t.Fatalf("linked-target invariant failed: %+v", pair)
			}
		case PaneAnchoredTarget:
			if pair.Reactive() || !pair.TargetUsesNP() || !pair.TargetScrollable {
				t.Fatalf("anchored-target invariant failed: %+v", pair)
			}
		case PaneAnchoredReference:
			if pair.Reactive() || pair.TargetUsesNP() || !pair.TargetScrollable {
				t.Fatalf("anchored-reference invariant failed: %+v", pair)
			}
		default:
			t.Fatalf("unknown pane profile for %+v", pair)
		}
	}
}

func TestSplitControlMatrixMatchesRegistry(t *testing.T) {
	base := New("REINS").
		FoldSessions([]grammar.Session{
			{Role: "cx-one", Platform: "codex", State: "active", Readiness: "claim", Attention: 0.88, AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "attention": "ok"}},
			{Role: "cx-two", Platform: "codex", State: "active", Readiness: "claim", Attention: 0.77, AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "attention": "ok"}},
		}, false).
		FoldIntake(grammar.IntakeSummary{Rows: []grammar.IntakeRow{
			{Source: "obsidian", Kind: "request", Status: "attention", Severity: "warn", Count: 1, AIR: map[string]string{"source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok"}},
			{Source: "security", Kind: "alert", Status: "attention", Severity: "crit", Count: 1, AIR: map[string]string{"source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok"}},
		}}, false).
		FoldDynamics(grammar.Graph{Nodes: []grammar.Node{
			{ID: "node-a", Label: "A", Status: "asserted", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
			{ID: "node-b", Label: "B", Status: "observed", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
		}}, false).
		FoldEpistemics(grammar.EpistemicsSummary{Rows: []grammar.EpistemicReadRow{
			{RowID: "map-node:node-a", Family: "map-node", SubjectKind: "map-node", SubjectRef: "node-a", Status: "observed", AIR: map[string]string{"row_id": "ok", "family": "ok", "subject_kind": "ok", "subject_ref": "ok", "status": "ok"}},
			{RowID: "map-node:node-b", Family: "map-node", SubjectKind: "map-node", SubjectRef: "node-b", Status: "observed", AIR: map[string]string{"row_id": "ok", "family": "ok", "subject_kind": "ok", "subject_ref": "ok", "status": "ok"}},
		}}, false)
	base.Width, base.Height, base.SplitContext = 220, 42, true

	for _, pair := range registeredSplitPairs() {
		t.Run(pageLabel(pair.Page), func(t *testing.T) {
			m := base
			m.Page = pair.Page
			if !m.splitContextActive() {
				t.Fatalf("split should be active for %s", pageLabel(pair.Page))
			}
			nav := ansi.Strip(m.splitNavHint(pair, false))
			header := ansi.Strip(m.splitRelationHeaderLabel(pair, 120))
			floor := ansi.Strip(m.floorActions(180))
			for _, surface := range []struct {
				name string
				text string
			}{{"nav", nav}, {"header", header}, {"footer", floor}} {
				if !strings.Contains(surface.text, "[j/k]") {
					t.Fatalf("%s split controls should expose source j/k for %s:\n%s", surface.name, pageLabel(pair.Page), surface.text)
				}
				hasNP := strings.Contains(surface.text, "[n/p]")
				if pair.TargetUsesNP() && !hasNP {
					t.Fatalf("%s split controls should expose declared n/p target for %s:\n%s", surface.name, pageLabel(pair.Page), surface.text)
				}
				if !pair.TargetUsesNP() && hasNP {
					t.Fatalf("%s split controls should not expose n/p for passive/no-target %s:\n%s", surface.name, pageLabel(pair.Page), surface.text)
				}
			}

			afterJ := step(m, "j")
			if afterJ.SFocus != 1 || afterJ.IFocus != m.IFocus || afterJ.DynFocus != m.DynFocus || afterJ.EpiFocus != m.EpiFocus {
				t.Fatalf("split j should move only source lane for %s, got S/I/D/E %d/%d/%d/%d", pageLabel(pair.Page), afterJ.SFocus, afterJ.IFocus, afterJ.DynFocus, afterJ.EpiFocus)
			}

			afterN := step(m, "n")
			switch pair.TargetCursor {
			case splitTargetIntake:
				if afterN.SFocus != m.SFocus || afterN.IFocus != 1 || afterN.DynFocus != m.DynFocus || afterN.EpiFocus != m.EpiFocus {
					t.Fatalf("split n should move only intake target for %s, got S/I/D/E %d/%d/%d/%d", pageLabel(pair.Page), afterN.SFocus, afterN.IFocus, afterN.DynFocus, afterN.EpiFocus)
				}
			case splitTargetMapElement:
				if afterN.SFocus != m.SFocus || afterN.IFocus != m.IFocus || afterN.DynFocus != 1 || afterN.EpiFocus != m.EpiFocus {
					t.Fatalf("split n should move only map target for %s, got S/I/D/E %d/%d/%d/%d", pageLabel(pair.Page), afterN.SFocus, afterN.IFocus, afterN.DynFocus, afterN.EpiFocus)
				}
			case splitTargetEpistemic:
				if afterN.SFocus != m.SFocus || afterN.IFocus != m.IFocus || afterN.DynFocus != m.DynFocus || afterN.EpiFocus != 1 {
					t.Fatalf("split n should move only epistemic target for %s, got S/I/D/E %d/%d/%d/%d", pageLabel(pair.Page), afterN.SFocus, afterN.IFocus, afterN.DynFocus, afterN.EpiFocus)
				}
			default:
				if afterN.SFocus != m.SFocus || afterN.IFocus != m.IFocus || afterN.DynFocus != m.DynFocus || afterN.EpiFocus != m.EpiFocus {
					t.Fatalf("split n should be inert for %s target=%s, got S/I/D/E %d/%d/%d/%d", pageLabel(pair.Page), pair.TargetCursor, afterN.SFocus, afterN.IFocus, afterN.DynFocus, afterN.EpiFocus)
				}
			}
		})
	}
}

// The Yard Coordinator pane: the Miller-column lens (left) drives the coordinator context (right).
// Moving the lens cursor ([j/k] → m.Focus) brushes the coordinator's selection block to that task.
func TestCoordinatorLensDrivesContext(t *testing.T) {
	ok := map[string]string{"task_id": "ok", "stage": "ok", "owner": "ok", "criticality": "ok"}
	m := New("REINS").FoldTasks([]grammar.Task{
		{TaskID: "alpha-task", Stage: "S6", Owner: "cc", Criticality: "ok", AIR: ok},
		{TaskID: "beta-task", Stage: "S7", Owner: "gov", Criticality: "crit", AIR: ok},
	}, false)
	m.Width, m.Height, m.Page = 200, 30, PageCoordinator
	v := ansi.Strip(m.View())
	for _, want := range []string{"LENS", "COORDINATOR", "selection lattice", "alpha-task"} {
		if !strings.Contains(v, want) {
			t.Fatalf("coordinator pane missing %q:\n%s", want, v)
		}
	}
	// the lens cursor starts at row 0 (alpha) — the selection block shows it driving the right pane.
	if !strings.Contains(v, "▶ selection  alpha-task") {
		t.Fatalf("coordinator selection block must show the focused task:\n%s", v)
	}
	// move the lens cursor to row 1 -> the coordinator selection must follow to beta-task (the brush).
	v2 := ansi.Strip(m.focusTo(1).View())
	if !strings.Contains(v2, "▶ selection  beta-task") {
		t.Fatalf("moving the lens cursor must drive the coordinator selection to beta-task:\n%s", v2)
	}
}

func TestRegisteredSplitPairsRenderCoherentTwoPaneFrames(t *testing.T) {
	base := New("REINS").
		Fold([]grammar.Event{{
			TS: "2026-06-25T12:00:00Z", Kind: "coord_dispatch.launch_started", Subject: "task-a", Actor: "cx-anchor", Score: 0.21,
			AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "score": "ok"},
		}}, false).
		FoldTasks([]grammar.Task{{
			TaskID: "task-a", Owner: "cx-anchor", Stage: "S7_RELEASE", PredictedStage: "hold", Criticality: "warn",
			AIR: map[string]string{"task_id": "ok", "owner": "ok", "stage": "ok", "predicted_stage": "ok", "criticality": "ok"},
		}}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-anchor", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88, ClaimedTask: "task-a",
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "ok"},
		}}, false).
		FoldDynamics(grammar.Graph{
			Nodes: []grammar.Node{{ID: "authority", Label: "Authority", Kind: "governance", Layer: "governance", Status: "asserted", AIR: map[string]string{"id": "ok", "label": "ok", "kind": "ok", "layer": "ok", "status": "ok"}}},
			Edges: []grammar.Edge{{Source: "authority", Target: "authority", Relation: "self", AIR: map[string]string{"source": "ok", "target": "ok", "relation": "ok"}}},
		}, false).
		FoldIntake(grammar.IntakeSummary{Rows: []grammar.IntakeRow{{
			Source: "request", Kind: "request_attention", Status: "attention", Severity: "warn", Count: 1,
			AIR: map[string]string{"source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok"},
		}}}, false).
		FoldCapabilities(grammar.CapabilitySummary{Rows: []grammar.CapabilityRow{{
			CapabilityID: "route_envelope", Status: "observed", Authority: "metadata-only", EvidenceCount: 1,
			AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "evidence_count": "ok"},
		}}}, false).
		FoldGates(grammar.GateSummary{Rows: []grammar.GateRow{{
			GateID: "authority", State: "observed", Domain: "release", Evidence: "metadata", Severity: "warn",
			AIR: map[string]string{"gate_id": "ok", "state": "ok", "domain": "ok", "evidence": "ok", "severity": "ok"},
		}}}, false).
		FoldDomains(grammar.DomainSummary{Lifecycles: []grammar.LifecycleRow{{
			LifecycleID: "sdlc", State: "source_backed", Posture: "source-backed", Plant: "trainyard", Scope: "tenant", AuthorityCeiling: "governed_route_required",
			AIR: map[string]string{"lifecycle_id": "ok", "state": "ok", "posture": "ok", "plant": "ok", "scope": "ok", "authority_ceiling": "ok"},
		}}}, false)
	base.Width, base.Height, base.SplitContext = 220, 38, true

	for _, pair := range registeredSplitPairs() {
		t.Run(pageLabel(pair.Page), func(t *testing.T) {
			m := base
			m.Page = pair.Page
			if pair.Page == PageCoordinator {
				// the Yard Coordinator self-composes (HConcat: lens │ coordinator), NOT the session-
				// frozen split — assert its own two-pane markers + the lens-drives-context contract.
				v := ansi.Strip(m.View())
				for _, want := range []string{"LENS", "COORDINATOR", "selection", "lattice"} {
					if !strings.Contains(v, want) {
						t.Fatalf("coordinator frame missing %q:\n%s", want, v)
					}
				}
				return
			}
			if !m.splitContextActive() {
				t.Fatalf("%s should activate split at %dx%d", pageLabel(pair.Page), m.Width, m.Height)
			}
			v := ansi.Strip(m.View())
			for _, want := range []string{"split sessions", "cx-anchor", pair.Target, pair.Join} {
				if !strings.Contains(v, want) {
					t.Fatalf("split %s frame missing relationship marker %q:\n%s", pageLabel(pair.Page), want, v)
				}
			}
			if pair.Reactive() {
				if !strings.Contains(v, "source focus drives context") && !strings.Contains(v, "source drives ctx") {
					t.Fatalf("linked split %s should explain source-driven context:\n%s", pageLabel(pair.Page), v)
				}
			} else if pair.TargetCursor == splitTargetMapElement {
				if !strings.Contains(v, "[n/p] map") || !strings.Contains(v, "SELECTED MAP ELEMENT") {
					t.Fatalf("dynamics split should expose its declared map target cursor:\n%s", v)
				}
			} else if pair.TargetCursor == splitTargetEpistemic {
				if !strings.Contains(v, "[n/p] evidence") || !strings.Contains(v, "SELECTED EVIDENCE PATH") {
					t.Fatalf("epistemics split should expose its declared evidence target cursor:\n%s", v)
				}
			} else if !strings.Contains(v, "context is independent reference") && !strings.Contains(v, "SPLIT RELATION") {
				t.Fatalf("reference split %s should explain independent context:\n%s", pageLabel(pair.Page), v)
			}
			floor := ansi.Strip(m.viewFloor(m.Width))
			if !strings.Contains(floor, "[←/→]ctx") {
				t.Fatalf("split %s footer should expose context cycling, not generic window cycling:\n%s", pageLabel(pair.Page), floor)
			}
			if strings.Contains(floor, "[←/→]win") {
				t.Fatalf("split %s footer should not show competing arrow semantics:\n%s", pageLabel(pair.Page), floor)
			}
			for i, line := range strings.Split(m.View(), "\n") {
				if got := ansi.StringWidth(line); got > m.Width {
					t.Fatalf("split %s line %d exceeds frame width %d: %d %q", pageLabel(pair.Page), i, m.Width, got, line)
				}
			}
		})
	}
}

func TestSplitTargetCursorNavigationUsesDeclaredCursor(t *testing.T) {
	m := New("REINS").
		FoldIntake(grammar.IntakeSummary{
			Rows: []grammar.IntakeRow{
				{Source: "planning", Kind: "coverage", Status: "bucket", Severity: "warn", Count: 1},
				{Source: "request", Kind: "open", Status: "attention", Severity: "major", Count: 2},
			},
		}, false).
		FoldDynamics(grammar.Graph{
			Nodes: []grammar.Node{
				{ID: "node-a", Label: "A", Status: "asserted", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
				{ID: "node-b", Label: "B", Status: "observed", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
			},
		}, false)
	m.Page = PageEvents

	next, ok := m.updateSplitTargetCursor(SplitPairDef{TargetCursor: splitTargetIntake}, "n")
	if !ok || next.IFocus != 1 || !strings.Contains(next.Status, "intake bucket 2/2") {
		t.Fatalf("intake target cursor should move from declared split pair, ok=%v iFocus=%d status=%q", ok, next.IFocus, next.Status)
	}
	next, ok = m.updateSplitTargetCursor(SplitPairDef{TargetCursor: splitTargetMapElement}, "n")
	if !ok || next.DynFocus != 1 || !strings.Contains(next.Status, "dynamics node 2/") {
		t.Fatalf("map target cursor should move from declared split pair, ok=%v dynFocus=%d status=%q", ok, next.DynFocus, next.Status)
	}
	next, ok = m.updateSplitTargetCursor(SplitPairDef{TargetCursor: splitTargetEpistemic}, "n")
	if !ok || next.EpiFocus != 1 || !strings.Contains(next.Status, "epistemic row 2/") {
		t.Fatalf("epistemic target cursor should move from declared split pair, ok=%v epiFocus=%d status=%q", ok, next.EpiFocus, next.Status)
	}
	if next, ok = m.updateSplitTargetCursor(SplitPairDef{TargetCursor: splitTargetScroll}, "n"); ok || next.IFocus != m.IFocus || next.DynFocus != m.DynFocus || next.EpiFocus != m.EpiFocus {
		t.Fatalf("scroll-only split target should not claim n/p target cursor, ok=%v i/d/e=%d/%d/%d", ok, next.IFocus, next.DynFocus, next.EpiFocus)
	}
}

func TestExecHotkeyAndCycleOpenRegisteredPages(t *testing.T) {
	if New("REINS").Exec("sessions").Page != PageSessions {
		t.Fatal("exec :sessions must switch to the sessions page")
	}
	m := New("REINS")
	m = step(m, "3")
	if m.Page != PageSessions {
		t.Fatalf("[3] must open sessions, got page %d", m.Page)
	}
	m = step(m, "Y")
	if m.Page != PageYard {
		t.Fatalf("[Y] must open yard, got page %d", m.Page)
	}
	m = step(m, "R")
	if m.Page != PageReadiness {
		t.Fatalf("[R] must open readiness, got page %d", m.Page)
	}
	m = step(m, "I")
	if m.Page != PageIntake {
		t.Fatalf("[I] must open intake observations, got page %d", m.Page)
	}
	m = step(m, "C")
	if m.Page != PageCaps {
		t.Fatalf("[C] must open capabilities, got page %d", m.Page)
	}
	m = step(m, "4")
	if m.Page != PageDynamics {
		t.Fatalf("[4] must open dynamics after capabilities insertion, got page %d", m.Page)
	}
	m = step(m, "E")
	if m.Page != PageEpistemics {
		t.Fatalf("[E] must open epistemics, got page %d", m.Page)
	}
	m = step(m, "5")
	if m.Page != PageHelp {
		t.Fatalf("[5] must open help after sessions insertion, got page %d", m.Page)
	}
	m = step(m, "6")
	if m.Page != PageCommands {
		t.Fatalf("[6] must open commands, got page %d", m.Page)
	}
	m = step(m, "7")
	if m.Page != PageWindows {
		t.Fatalf("[7] must open windows, got page %d", m.Page)
	}
	m = step(m, "8")
	if m.Page != PageIntent {
		t.Fatalf("[8] must open intent review, got page %d", m.Page)
	}
	m = step(m, "9")
	if m.Page != PageSurfaces {
		t.Fatalf("[9] must open surfaces, got page %d", m.Page)
	}
	m = step(m, "0")
	if m.Page != PageDomains {
		t.Fatalf("[0] must open domains, got page %d", m.Page)
	}
	m = step(m, "L")
	if m.Page != PageLifecycles {
		t.Fatalf("[L] must open lifecycle registry, got page %d", m.Page)
	}
	m = step(m, "]")
	if m.Page != PageTraces {
		t.Fatalf("] must cycle to traces after lifecycles, got page %d", m.Page)
	}
	m = step(m, "]")
	if m.Page != PageLegend {
		t.Fatalf("] must cycle to legend after traces, got page %d", m.Page)
	}
	m = step(m, "]")
	if m.Page != PageEvents {
		t.Fatalf("] must wrap from legend to events, got page %d", m.Page)
	}
	m = step(m, "[")
	if m.Page != PageLegend {
		t.Fatalf("[ must wrap from events to legend, got page %d", m.Page)
	}
	m = step(m, "[")
	if m.Page != PageTraces {
		t.Fatalf("[ must cycle from legend to traces, got page %d", m.Page)
	}
	m = step(m, "[")
	if m.Page != PageLifecycles {
		t.Fatalf("[ must cycle from traces to lifecycles, got page %d", m.Page)
	}
	m = step(m, "[")
	if m.Page != PageDomains {
		t.Fatalf("[ must cycle from lifecycles to domains, got page %d", m.Page)
	}
	m = step(m, "[")
	if m.Page != PageSurfaces {
		t.Fatalf("[ must cycle from domains to surfaces, got page %d", m.Page)
	}
	m = step(m, "[")
	if m.Page != PageIntent {
		t.Fatalf("[ must cycle from surfaces to intent, got page %d", m.Page)
	}
	m = step(m, "[")
	if m.Page != PageWindows {
		t.Fatalf("[ must cycle from intent to windows, got page %d", m.Page)
	}
}

func TestRegisteredWindowIDsAreExecutableCommands(t *testing.T) {
	for _, wnd := range registeredWindows() {
		got := New("REINS").Exec(wnd.ID)
		if got.Page != wnd.Page {
			t.Fatalf("registered window id %q should be executable and open page %s, got %s status=%q", wnd.ID, pageLabel(wnd.Page), pageLabel(got.Page), got.Status)
		}
	}
}

func TestSessionsResumeIntentIsStubOnly(t *testing.T) {
	m := New("REINS").FoldSessions([]grammar.Session{{
		Role: "cx-p0", Session: "tmux-p0", Platform: "codex", State: "active", Readiness: "live", Blocker: "no_claim", Attention: 0.74, Alive: true,
		AIR: map[string]string{"role": "ok", "session": "deny", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "alive": "ok", "idle": "ok", "stalled": "ok"},
	}}, false)
	m.Width, m.Height = 120, 40
	m.Page, m.AIR = PageSessions, true
	m = step(m, "enter")
	if !m.SessionDoorOpen {
		t.Fatal("sessions enter should open the full-screen lane card")
	}
	if frame := ansi.Strip(m.View()); strings.Contains(frame, "tmux-p0") || !strings.Contains(frame, "DOOR /session") {
		t.Fatalf("session door should be AIR-safe and visible:\n%s", frame)
	}
	m = step(m, "r")
	if !strings.Contains(m.Status, "resume-intent: would emit session.resume") || !strings.Contains(m.Status, "cx-p0") || strings.Contains(m.Status, "tmux-p0") {
		t.Fatalf("sessions r should report an AIR-safe governed stub, got %q", m.Status)
	}
	m = step(m, "r")
	if !strings.Contains(m.Status, "no transcript/PTY/stdin bridge") {
		t.Fatalf("sessions r should be a non-dispatch resume-intent stub, got %q", m.Status)
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

func TestWhoisDoorIllegalVerbDoesNotFire(t *testing.T) {
	tasks := []grammar.Task{{TaskID: "door-2", Stage: "S3_PLAN", PredictedStage: "next", Criticality: "warn",
		AIR: map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok"}}}
	m := New("REINS").FoldTasks(tasks, false)
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	m = step(m, "enter")
	if !m.DoorOpen {
		t.Fatal("setup should open the /whois door")
	}
	m = step(m, "a")
	if !m.DoorOpen || !strings.Contains(m.Status, "unavailable") || strings.Contains(m.Status, "would emit") {
		t.Fatalf("illegal door verb must stay in the door and not fire a stub: open=%v status=%q", m.DoorOpen, m.Status)
	}
}

func TestShortDoorsIndicateHiddenRowsBeforeDock(t *testing.T) {
	check := func(name string, m Model, w int) {
		t.Helper()
		frame := ansi.Strip(m.View())
		for _, want := range []string{"door rows hidden; taller frame", "VERB DOCK"} {
			if !strings.Contains(frame, want) {
				t.Fatalf("%s short door missing %q:\n%s", name, want, frame)
			}
		}
		for i, line := range strings.Split(frame, "\n") {
			if got := ansi.StringWidth(line); got > w {
				t.Fatalf("%s short door line %d exceeds frame width %d: %d %q", name, i, w, got, line)
			}
		}
	}

	taskDoor := New("REINS").FoldTasks([]grammar.Task{{
		TaskID: "short-door-task", Stage: "S7_RELEASE", PredictedStage: "hold", Criticality: "warn", Owner: "cx",
		NoGo: "release_authorized,review_authorized,receipt_authorized",
		AIR:  map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok", "criticality": "ok", "owner": "ok", "no_go": "ok"},
	}}, false)
	taskDoor.Width, taskDoor.Height, taskDoor.Page = 100, 12, PageTasks
	taskDoor = step(taskDoor, "enter")
	check("whois", taskDoor, taskDoor.Width)

	sessionDoor := New("REINS").FoldSessions([]grammar.Session{{
		Role: "cx-short", Session: "tmux-short", Platform: "codex", State: "active", Readiness: "claim", Blocker: "stale_relay", Attention: 0.88, OutputAgeS: 120, RelayAgeS: 240,
		AIR: map[string]string{"role": "ok", "session": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "output_age_s": "ok", "relay_age_s": "ok"},
	}}, false)
	sessionDoor.Width, sessionDoor.Height, sessionDoor.Page = 100, 14, PageSessions
	sessionDoor = step(sessionDoor, "enter")
	check("session", sessionDoor, sessionDoor.Width)

	intakeDoor := New("REINS").FoldIntake(grammar.IntakeSummary{
		Sources: []grammar.IntakeSource{{
			ID: "request_state", Path: "/tmp/request-state.json", Exists: true, Status: "observed", Count: 4, AgeBucket: "<5m", Privacy: "metadata-only",
			AIR: map[string]string{"id": "ok", "path": "ok", "exists": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "privacy": "ok"},
		}},
		Rows: []grammar.IntakeRow{{
			Source: "request_state", Kind: "request_attention", Status: "attention", Severity: "warn", Count: 4, Coverage: "requests", Blocker: "workflow_attention",
			AIR: map[string]string{"source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok", "coverage": "ok", "blocker": "ok", "age_bucket": "ok"},
		}},
		Totals: map[string]int{"request_attention": 4},
	}, false)
	intakeDoor.Width, intakeDoor.Height, intakeDoor.Page = 100, 16, PageIntake
	intakeDoor = step(intakeDoor, "enter")
	check("intake", intakeDoor, intakeDoor.Width)
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

func TestYankModeNavigatesRowsAndFieldGranularity(t *testing.T) {
	ss := []grammar.Session{
		{Role: "first", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.8,
			AIR: map[string]string{"role": "ok", "platform": "ok", "readiness": "ok"}},
		{Role: "second", Platform: "claude", State: "active", Readiness: "stale", Blocker: "stale_relay", Attention: 0.5,
			AIR: map[string]string{"role": "ok", "platform": "ok", "readiness": "ok"}},
	}
	m := New("REINS").FoldSessions(ss, false)
	m.Width, m.Height, m.Page = 120, 40, PageSessions
	m = step(m, "y")
	if m.Mode != ModeYank || m.SFocus != 0 || m.Sel.Field != "role" {
		t.Fatalf("yank should enter field granularity on the focused session: mode=%d focus=%d field=%q", m.Mode, m.SFocus, m.Sel.Field)
	}
	m = step(m, "j")
	if m.Mode != ModeYank || m.SFocus != 1 || m.Sel.Field != "role" {
		t.Fatalf("j should move rows while staying in yank mode and preserving field: mode=%d focus=%d field=%q", m.Mode, m.SFocus, m.Sel.Field)
	}
	m = step(m, "tab")
	if m.Mode != ModeYank || m.Sel.Field != "platform" {
		t.Fatalf("tab should move field granularity, got mode=%d field=%q", m.Mode, m.Sel.Field)
	}
	if frame := ansi.Strip(m.View()); !strings.Contains(frame, "yank field") || !strings.Contains(frame, "[j/k] rows") {
		t.Fatalf("yank mode footer should describe navigable yank, not task field-rank:\n%s", frame)
	}
	m = step(m, "enter")
	if m.Mode != ModeCommand || m.Input != "claude" || len(m.Ring) != 1 || m.Ring[0].Field != "platform" {
		t.Fatalf("enter should yank the selected row+field: mode=%d input=%q ring=%+v", m.Mode, m.Input, m.Ring)
	}
}

func TestYankDeniedFieldStaysNavigable(t *testing.T) {
	ss := []grammar.Session{{
		Role: "cx", Session: "SECRET-TMUX", Platform: "codex", State: "active", Readiness: "claim",
		AIR: map[string]string{"role": "ok", "session": "deny", "readiness": "ok"},
	}}
	m := New("REINS").FoldSessions(ss, false)
	m.Width, m.Height, m.Page, m.AIR = 120, 40, PageSessions, true
	m = step(step(m, "y"), "s")
	if m.Mode != ModeYank || len(m.Ring) != 0 || strings.Contains(m.Input, "SECRET-TMUX") || !strings.Contains(m.Status, "redacted") {
		t.Fatalf("denied field should be unavailable without collapsing yank mode: mode=%d input=%q status=%q ring=%+v", m.Mode, m.Input, m.Status, m.Ring)
	}
	m = step(m, "tab")
	if m.Mode != ModeYank || m.Sel.Field == "" {
		t.Fatalf("operator should still be able to move field granularity after a denied yank: mode=%d field=%q", m.Mode, m.Sel.Field)
	}
}

func TestEventYankBlocksDeniedMetadataOnAir(t *testing.T) {
	ev := grammar.Event{
		TS: "SECRET-TS", Kind: "secret.kind", Subject: "visible",
		AIR: map[string]string{"ts": "deny", "kind": "deny", "subject": "ok"},
	}
	m := New("REINS").Fold([]grammar.Event{ev}, false)
	m.Width, m.Height = 120, 40
	m.Page = PageEvents
	m.AIR = true
	m = step(step(m, "y"), "t")
	if len(m.Ring) != 0 || strings.Contains(m.Input, "SECRET-TS") || !strings.Contains(m.Status, "redacted") {
		t.Fatalf("denied event metadata must be un-yankable on AIR: input=%q status=%q ring=%+v", m.Input, m.Status, m.Ring)
	}
}

func TestClassYankBlocksDeniedTaskIDOnAir(t *testing.T) {
	tasks := []grammar.Task{
		{TaskID: "secret-a", Criticality: "crit", AIR: map[string]string{"task_id": "deny"}},
		{TaskID: "secret-b", Criticality: "crit", AIR: map[string]string{"task_id": "deny"}},
	}
	m := New("REINS").FoldTasks(tasks, false)
	m.Page = PageTasks
	m.AIR = true
	m = step(m, "V")
	m = step(m, "y")
	if len(m.Ring) != 0 || strings.Contains(m.Status, "secret-a") || strings.Contains(m.Status, "secret-b") {
		t.Fatalf("class yank must not leak denied ids on air: status=%q ring=%+v", m.Status, m.Ring)
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

func TestFieldCursorAllowsGlobalPageSwitch(t *testing.T) {
	m := New("REINS").Fold(evFixture(), false).FoldTasks([]grammar.Task{{TaskID: "alpha-1", AIR: map[string]string{"task_id": "ok"}}}, false)
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	m = step(m, "tab")
	if m.Sel.Rank != RankField {
		t.Fatal("setup should enter field rank")
	}
	m = step(m, "1")
	if m.Page != PageEvents || m.Sel.Rank != RankRow || m.Sel.Field != "" {
		t.Fatalf("page switch from field rank should normalize selection, page=%d sel=%+v", m.Page, m.Sel)
	}
}

func TestFloorAdvertisesPageAwareActions(t *testing.T) {
	m := New("REINS").Fold(evFixture(), false).FoldTasks([]grammar.Task{{TaskID: "t1", AIR: map[string]string{"task_id": "ok"}}}, false)
	m.Width, m.Height = 120, 40
	m.Page = PageEvents
	eventsFloor := ansi.Strip(m.viewFloor(120))
	if strings.Contains(eventsFloor, "inspect") {
		t.Fatalf("events floor must not advertise task inspect: %q", eventsFloor)
	}
	m.Page = PageHelp
	helpFloor := ansi.Strip(m.viewFloor(120))
	if strings.Contains(helpFloor, "select") || strings.Contains(helpFloor, "yank") || strings.Contains(helpFloor, "inspect") {
		t.Fatalf("reference-page floor must not advertise row verbs: %q", helpFloor)
	}
	if !strings.Contains(helpFloor, "focus :help") {
		t.Fatalf("reference-page floor should name the active page, not a stale row focus: %q", helpFloor)
	}
}

func TestNormalFloorPreservesCommandAffordanceAtNarrowWidth(t *testing.T) {
	m := New("REINS")
	m.Page = PageHelp
	floor := ansi.Strip(m.viewFloor(28))
	lines := strings.Split(floor, "\n")
	if len(lines) != 2 {
		t.Fatalf("normal floor should remain two lines, got %d:\n%s", len(lines), floor)
	}
	if !strings.Contains(lines[0], "[:]cmd") {
		t.Fatalf("first floor line should preserve command affordance before optional text:\n%s", floor)
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got > 28 {
			t.Fatalf("floor line %d exceeds narrow width: %d %q", i, got, line)
		}
	}
}

func TestCommandAndFilterFloorsViewportLongInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    Model
		tail string
	}{
		{
			name: "command",
			m: func() Model {
				m := New("REINS")
				m.Mode = ModeCommand
				m.Input = "intent show-route p0-incident-sdlc-dispatch-refusal-circuit-breaker-visible-tail"
				return m
			}(),
			tail: "visible-tail",
		},
		{
			name: "filter",
			m: func() Model {
				m := New("REINS").FoldTasks([]grammar.Task{{TaskID: "visible-tail", AIR: map[string]string{"task_id": "ok"}}}, false)
				m.Page = PageTasks
				m.Mode = ModeFilter
				m.Filter = "p0-incident-sdlc-dispatch-refusal-circuit-breaker-visible-tail"
				return m
			}(),
			tail: "visible-tail",
		},
	} {
		floor := ansi.Strip(tc.m.viewFloor(54))
		if !strings.Contains(floor, tc.tail) || !strings.Contains(floor, "█") {
			t.Fatalf("%s floor should keep input tail and cursor visible:\n%s", tc.name, floor)
		}
		for i, line := range strings.Split(floor, "\n") {
			if got := ansi.StringWidth(line); got > 54 {
				t.Fatalf("%s floor line %d exceeds viewport: %d %q", tc.name, i, got, line)
			}
		}
	}
}

func TestCommandFloorRedactsNoteFreeTextOnAIR(t *testing.T) {
	m := New("REINS")
	m.Mode = ModeCommand
	m.AIR = true
	m.Input = "note SECRET-FREE-TEXT {{sel.id}}"

	floor := ansi.Strip(m.viewFloor(80))
	if strings.Contains(floor, "SECRET-FREE-TEXT") || strings.Contains(floor, "{{sel.id}}") {
		t.Fatalf("AIR command floor leaked note free text:\n%s", floor)
	}
	if !strings.Contains(floor, "note ▒▒▒") {
		t.Fatalf("AIR command floor should preserve verb and redact note body:\n%s", floor)
	}
	for i, line := range strings.Split(floor, "\n") {
		if got := ansi.StringWidth(line); got > 80 {
			t.Fatalf("AIR command floor line %d exceeds viewport: %d %q", i, got, line)
		}
	}
}

func TestCompletionStripKeepsSelectedCandidateVisibleWhenNarrow(t *testing.T) {
	tasks := make([]grammar.Task, 0, 8)
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("very-long-filter-candidate-%02d", i)
		if i == 5 {
			id = "selected-task"
		}
		tasks = append(tasks, grammar.Task{TaskID: id, Criticality: "warn", AIR: map[string]string{"task_id": "ok", "criticality": "ok"}})
	}
	m := New("REINS").FoldTasks(tasks, false)
	m.Mode, m.CompIdx = ModeFilter, 5
	floor := ansi.Strip(m.viewFloor(48))
	if !strings.Contains(floor, "6/8") || !strings.Contains(floor, "selected-task") || !strings.Contains(floor, "‹5") || !strings.Contains(floor, "›2") {
		t.Fatalf("narrow completion strip should expose selected candidate and hidden counts:\n%s", floor)
	}
	for i, line := range strings.Split(floor, "\n") {
		if got := ansi.StringWidth(line); got > 48 {
			t.Fatalf("completion floor line %d exceeds viewport: %d %q", i, got, line)
		}
	}
}

func TestReferencePagesScrollWithRowKeys(t *testing.T) {
	m := New("REINS")
	m.Width, m.Height = 80, 12
	m.Page = PageLegend
	if max := m.referenceScrollMax(); max == 0 {
		t.Fatal("legend should be scrollable in a short frame")
	}
	before := m.View()
	m = step(m, "j")
	if m.RefScroll != 1 || !strings.Contains(m.Status, "scroll") {
		t.Fatalf("j should scroll a reference page: offset=%d status=%q", m.RefScroll, m.Status)
	}
	if after := m.View(); after == before {
		t.Fatal("scrolling a reference page should change the rendered frame")
	}
	m = step(m, "G")
	if m.RefScroll != m.referenceScrollMax() {
		t.Fatalf("G should jump reference page to bottom: offset=%d max=%d", m.RefScroll, m.referenceScrollMax())
	}
	m = step(m, "g")
	if m.RefScroll != 0 {
		t.Fatalf("g should jump reference page to top: offset=%d", m.RefScroll)
	}
}

func TestWideReferencePagesUseContextRail(t *testing.T) {
	m := New("REINS").FoldTasks([]grammar.Task{{
		TaskID: "blocked", PredictedStage: "hold", Criticality: "crit",
		AIR: map[string]string{"task_id": "ok", "predicted_stage": "ok", "criticality": "ok"},
	}}, false)
	m.Width, m.Height, m.Page = 220, 42, PageHelp

	v := ansi.Strip(m.View())
	for _, want := range []string{"CONTEXT", "page contract", "selection context", "{{focus}}", "{{sel.*}}", "{{ring.0}}", "current row identity", "split", "legal next"} {
		if !strings.Contains(v, want) {
			t.Fatalf("wide reference context rail missing %q:\n%s", want, v)
		}
	}
	for _, line := range strings.Split(v, "\n") {
		if ansi.StringWidth(line) > m.Width {
			t.Fatalf("wide reference line exceeds frame width %d: %d %q", m.Width, ansi.StringWidth(line), line)
		}
	}
}

func TestReferenceContextRailUsesSemanticHeading(t *testing.T) {
	caps := grammar.CapabilitySummary{Rows: []grammar.CapabilityRow{
		{CapabilityID: "source_acquisition", Status: "admission-incomplete", Authority: "sub-router", CapabilityClass: "source_acquisition", SurfaceFamily: "source_acquisition", RouteCount: 2, OKCount: 1, BlockedCount: 1, EvidenceCount: 2,
			SourceRefs: "cap-pack:1 refs", SourceRefLabels: []string{"handoff#source"},
			AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "capability_class": "ok", "surface_family": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "source_refs": "ok", "source_ref_labels": "ok"}},
		{CapabilityID: "tavily_source_acquisition", Status: "admission-incomplete", Authority: "source-acquisition", CapabilityClass: "source_acquisition", SurfaceFamily: "tavily", RouteCount: 1, BlockedCount: 1, EvidenceCount: 4,
			SourceRefs: "cap-pack:2 refs", SourceRefLabels: []string{"handoff#tavily", "docs#usage"},
			AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "capability_class": "ok", "surface_family": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "source_refs": "ok", "source_ref_labels": "ok"}},
		{CapabilityID: "route_envelope", Status: "observed", Authority: "metadata-only", CapabilityClass: "core_route", SurfaceFamily: "route_governance", RouteCount: 2, OKCount: 2, EvidenceCount: 2,
			AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "capability_class": "ok", "surface_family": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "source_ref_labels": "ok"}},
		{CapabilityID: "grounding", Status: "observed", Authority: "registry evidence", CapabilityClass: "registry_score", SurfaceFamily: "platform_capability_registry", RouteCount: 2, OKCount: 2, EvidenceCount: 2,
			AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "capability_class": "ok", "surface_family": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "source_ref_labels": "ok"}},
	}}
	m := New("REINS").FoldCapabilities(caps, false)
	m.Width, m.Height, m.Page = 220, 42, PageCaps
	if got := m.referenceContextHeading(); got != "capability class" {
		t.Fatalf("selected class row should label context as capability class, got %q", got)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "capability class") || strings.Contains(v, "surface use\n capability  source_acquisition") {
		t.Fatalf("capability class rail should not be mislabeled as surface use:\n%s", v)
	}
	m.CFocus = 1
	if got := m.referenceContextHeading(); got != "surface capability" {
		t.Fatalf("selected surface row should label context as surface capability, got %q", got)
	}
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "refs        handoff#tavily · docs#usage") {
		t.Fatalf("selected capability context should expose bounded source ref labels:\n%s", v)
	}
	m.CFocus = 2
	if got := m.referenceContextHeading(); got != "route contract" {
		t.Fatalf("selected core row should label context as route contract, got %q", got)
	}
	m.CFocus = 3
	if got := m.referenceContextHeading(); got != "score dimension" {
		t.Fatalf("selected registry score row should label context as score dimension, got %q", got)
	}

	for _, tc := range []struct {
		page int
		want string
	}{
		{PageSurfaces, "surface use"},
		{PageDomains, "domain lens"},
		{PageCommands, "command grammar"},
		{PageIntake, "observation context"},
		{PageLifecycles, "lifecycle contracts"},
	} {
		m.Page = tc.page
		if got := m.referenceContextHeading(); got != tc.want {
			t.Fatalf("%s heading = %q, want %q", pageLabel(tc.page), got, tc.want)
		}
	}
}

func TestWideReferenceContextRailIndicatesOverflow(t *testing.T) {
	m := New("REINS")
	m.Width, m.Height, m.Page = 180, 16, PageSurfaces
	v := ansi.Strip(m.View())
	for _, want := range []string{"CONTEXT", "page contract", "context rows hidden; taller frame"} {
		if !strings.Contains(v, want) {
			t.Fatalf("short wide reference context rail should expose overflow indicator %q:\n%s", want, v)
		}
	}
	for i, line := range strings.Split(v, "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("short wide reference line %d exceeds frame width %d: %d %q", i, m.Width, got, line)
		}
	}
}

func TestWideSurfaceCatalogWithContextRailUsesSemanticRows(t *testing.T) {
	m := New("REINS")
	m.Width, m.Height, m.Page = 220, 58, PageSurfaces
	v := ansi.Strip(m.View())
	for _, want := range []string{
		"CONTEXT",
		"surface=split-context",
		"contract=all splits: [j/k] source, [J/K]",
		"context; Enter/y act on source",
		"Enter/y act on source",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("wide-with-context surface catalog should preserve semantic row content %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "SURFACE            NAME") {
		t.Fatalf("wide-with-context surface catalog should not use the dense table when the left pane is narrow:\n%s", v)
	}
}

func TestWideReferenceCatalogsUseCompactContextRail(t *testing.T) {
	for _, tc := range []struct {
		name        string
		page        int
		denseHeader string
		want        []string
	}{
		{name: "commands", page: PageCommands, denseHeader: "VERB             KIND/GROUP", want: []string{"verb=intent", "preflight=target +"}},
		{name: "windows", page: PageWindows, denseHeader: "KEY WINDOW", want: []string{"page shape", "first read", "cursor      ▶ linked", "window=capabilities", "split=link:scroll role/platform"}},
		{name: "surfaces", page: PageSurfaces, denseHeader: "SURFACE            NAME", want: []string{"page shape", "layout      2 layout surfaces", "shape", "layout:2 · doors:3 modes:4", "surface=split-context", "fundamentals"}},
		{name: "domains", page: PageDomains, denseHeader: "DOMAIN              TERRAIN", want: []string{"page shape", "ontology    pack missing", "SOURCE STATUS", "shape       core:", "domain=research-rdlc", "fundamentals"}},
		{name: "lifecycles", page: PageLifecycles, denseHeader: "LIFECYCLE          STATE", want: []string{"page contract", "contracts", "SOURCE STATUS", "COMPILED FALLBACK", "lifecycle contracts", "tenant-safe"}},
		{name: "legend", page: PageLegend, denseHeader: "", want: []string{"LEGEND", "page shape", "LAYOUT", "split:ctx", "split:wide", "▶", "◆", "fundamentals"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New("REINS")
			m.Width, m.Height, m.Page = 220, 58, tc.page
			v := ansi.Strip(m.View())
			for _, want := range tc.want {
				if !strings.Contains(v, want) {
					t.Fatalf("wide %s reference page missing %q:\n%s", tc.name, want, v)
				}
			}
			if tc.denseHeader != "" && strings.Contains(v, tc.denseHeader) {
				t.Fatalf("wide %s reference page should not use dense table in the split-width main pane:\n%s", tc.name, v)
			}
			if strings.Contains(v, "context rows hidden") {
				t.Fatalf("wide %s reference context rail should fit the frame:\n%s", tc.name, v)
			}
			for i, line := range strings.Split(v, "\n") {
				if got := ansi.StringWidth(line); got > m.Width {
					t.Fatalf("wide %s line %d exceeds frame width %d: %d %q", tc.name, i, m.Width, got, line)
				}
			}
		})
	}
}

func TestSurfaceAndDomainCatalogsUseSelectableRegistryRows(t *testing.T) {
	m := New("REINS")
	m.Width, m.Height, m.Page = 220, 58, PageSurfaces
	m = step(m, "j")
	if m.SurfaceFocus != 1 || !strings.Contains(m.Status, "surface 2/") {
		t.Fatalf("j should move the surface cursor, focus=%d status=%q", m.SurfaceFocus, m.Status)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"focus filter", "[j/k]surface", "selected    filter", ": surface=filter", "glyph       : mode"} {
		if !strings.Contains(v, want) {
			t.Fatalf("selectable surface view missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "[j/k]scroll") {
		t.Fatalf("surface registry should advertise row selection, not document scroll:\n%s", v)
	}

	m.Page = PageDomains
	m = step(m, "G")
	if m.DomainFocus != len(registeredDomains())-1 || !strings.Contains(m.Status, "domain ") {
		t.Fatalf("G should move the domain cursor to the last registry row, focus=%d status=%q", m.DomainFocus, m.Status)
	}
	v = ansi.Strip(m.View())
	for _, want := range []string{"focus future-n-dlc", "[j/k]domain", "selected    future-n-dlc", "windows     domains,windows", "surfaces    surfaces"} {
		if !strings.Contains(v, want) {
			t.Fatalf("selectable domain view missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "[j/k]scroll") {
		t.Fatalf("domain registry should advertise row selection, not document scroll:\n%s", v)
	}
}

func TestSourceBackedDomainCursorDrivesRailAndFloor(t *testing.T) {
	m := New("REINS").FoldDomains(grammar.DomainSummary{
		Rows: []grammar.DomainRow{
			{
				DomainID: "sdlc-trainyard", Lifecycle: "SDLC", Terrain: "delivery", Depth: "surface", Scope: "operator", State: "observed",
				AuthorityCeiling: "projection", Windows: "yard,readiness", Surfaces: "yard,caps", Parity: "trainyard", EvidenceCount: 8, Blocker: "none",
				AIR: map[string]string{"domain_id": "ok", "lifecycle": "ok", "terrain": "ok", "depth": "ok", "scope": "ok", "state": "ok", "authority_ceiling": "ok", "windows": "ok", "surfaces": "ok", "parity": "ok", "evidence_count": "ok", "blocker": "ok"},
			},
			{
				DomainID: "rdlc-labrack", Lifecycle: "RDLC", Terrain: "research", Depth: "stratum", Scope: "tenant", State: "candidate",
				AuthorityCeiling: "support", Windows: "domains,dynamics", Surfaces: "labrack,figure", Parity: "labrack", EvidenceCount: 3, Blocker: "source review",
				AIR: map[string]string{"domain_id": "ok", "lifecycle": "ok", "terrain": "ok", "depth": "ok", "scope": "ok", "state": "ok", "authority_ceiling": "ok", "windows": "ok", "surfaces": "ok", "parity": "ok", "evidence_count": "ok", "blocker": "ok"},
			},
		},
		Totals: map[string]int{"rows": 2},
	}, false)
	m.Width, m.Height, m.Page = 220, 58, PageDomains
	m = step(m, "G")
	if m.DomainFocus != 1 || !strings.Contains(m.Status, "domain 2/2") {
		t.Fatalf("G should move through source-backed domain rows, focus=%d status=%q", m.DomainFocus, m.Status)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"focus rdlc-labrack", "selected    rdlc-labrack", "state       candidate", "windows     domains,dynamics", "surfaces    labrack,figure", "domain 2/2", "[j/k]domain"} {
		if !strings.Contains(v, want) {
			t.Fatalf("source-backed domain cursor view missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "focus future-n-dlc") || strings.Contains(v, "selected    future-n-dlc") {
		t.Fatalf("source-backed domain cursor should not bind the compiled fallback row:\n%s", v)
	}
}

func TestSplitContextPinsSessionsWhileCyclingContext(t *testing.T) {
	m := New("REINS").
		FoldTasks([]grammar.Task{{TaskID: "task-a", AIR: map[string]string{"task_id": "ok"}}}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-p0", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"},
		}}, false)
	m.Width, m.Height, m.Page = 220, 44, PageCaps
	m = step(m, "|")
	if !m.SplitContext || !strings.Contains(m.Status, "split context on") {
		t.Fatalf("[|] should enable split context, split=%v status=%q", m.SplitContext, m.Status)
	}
	floor := ansi.Strip(m.viewFloor(m.Width))
	if !strings.Contains(floor, "split context on") || !strings.Contains(floor, "press [:] command line") {
		t.Fatalf("status flashes must keep the command bar affordance visible:\n%s", floor)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"split:ctx", "split sessions", "cx-p0", "SELECTED LANE FIT", "CAPABILITY MATCH", "▶ claim   cx-p0"} {
		if !strings.Contains(v, want) {
			t.Fatalf("split context missing %q:\n%s", want, v)
		}
	}

	m = step(m, "right")
	if !m.SplitContext || m.Page != PageDynamics {
		t.Fatalf("window cycling should change the context pane while preserving split, page=%d split=%v", m.Page, m.SplitContext)
	}
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "split sessions") || !strings.Contains(v, "◆ claim   cx-p0") || !strings.Contains(v, "scale all") || !strings.Contains(v, "sessions + dynamics map as system topology") || !strings.Contains(v, "[j/k] lane anchor") || !strings.Contains(v, "focus :dynamics · system topology · anchor cx-p0") || !strings.Contains(v, "[j/k]anchor") {
		t.Fatalf("split should keep sessions left and cycle context right:\n%s", v)
	}
	if strings.Contains(v, "[j/k/J/K]") {
		t.Fatalf("split source-only context should not overload lowercase source navigation:\n%s", v)
	}
	if strings.Contains(v, "▶ claim   cx-p0") {
		t.Fatalf("source-only split should render the session as an anchor, not an active row cursor:\n%s", v)
	}

	m = step(m, "y")
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "[j/k] source rows") || !strings.Contains(v, "[j/k]source-rows") || strings.Contains(v, "[j/k/J/K]ctx-scroll") {
		t.Fatalf("split yank mode should advertise source row navigation even on source-only pages:\n%s", v)
	}
}

func TestSplitContextSourceVerbsAreLabeledAndOwnedBySessionPane(t *testing.T) {
	m := New("REINS").
		FoldSessions([]grammar.Session{{
			Role: "cx-source", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"},
		}}, false).
		FoldIntake(grammar.IntakeSummary{Rows: []grammar.IntakeRow{{
			Source: "request_state", Kind: "request_attention", Status: "attention", Severity: "warn", Count: 1,
			AIR: map[string]string{"source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok"},
		}}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 220, 42, PageIntake, true

	floor := ansi.Strip(m.viewFloor(m.Width))
	for _, want := range []string{"[↵]src-detail", "[y]src-yank", "[s/S]flt"} {
		if !strings.Contains(floor, want) {
			t.Fatalf("split footer should label source-owned verbs with %q:\n%s", want, floor)
		}
	}
	if !strings.Contains(floor, "[←/→]ctx") || strings.Contains(floor, "[←/→]win") || strings.Count(floor, "[←/→]") != 1 {
		t.Fatalf("split footer should expose one arrow meaning for context cycling:\n%s", floor)
	}
	if strings.Contains(floor, "[↵]detail ") || strings.Contains(floor, "[y]ank ") {
		t.Fatalf("split footer should not imply Enter/y act on the right pane:\n%s", floor)
	}

	m = step(m, "enter")
	if !m.SessionDoorOpen || m.IntakeDoorOpen {
		t.Fatalf("Enter in split context should open source session detail, not right-pane intake detail: sessionDoor=%v intakeDoor=%v", m.SessionDoorOpen, m.IntakeDoorOpen)
	}
}

func TestSplitContextFooterCompactsActionsBeforeClipping(t *testing.T) {
	rows := make([]grammar.CapabilityRow, 0, 24)
	for i := 0; i < 24; i++ {
		rows = append(rows, grammar.CapabilityRow{
			CapabilityID:  fmt.Sprintf("capability-%02d", i),
			Status:        "preview-only",
			Authority:     "projection",
			RouteCount:    1,
			EvidenceCount: 1,
			AIR:           map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok"},
		})
	}
	m := New("REINS").
		FoldSessions([]grammar.Session{{
			Role: "cx-source", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"},
		}}, false).
		FoldCapabilities(grammar.CapabilitySummary{Rows: rows}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 170, 18, PageCaps, true
	if !m.splitContextActive() || m.referenceScrollMax() == 0 {
		t.Fatalf("test requires active split with scrollable right context, split=%v max=%d", m.splitContextActive(), m.referenceScrollMax())
	}
	floor := ansi.Strip(m.viewFloor(m.Width))
	first := strings.Split(floor, "\n")[0]
	for _, want := range []string{"[:]cmd", "[j/k]src", "[↵]src-detail", "[r]resume", "[y]src-yank", "[←/→]ctx", "[J/K]scroll"} {
		if !strings.Contains(first, want) {
			t.Fatalf("compact split footer missing %q:\n%s", want, floor)
		}
	}
	if strings.Contains(first, "[←/→]win") || strings.Count(first, "[←/→]") != 1 {
		t.Fatalf("compact split footer should expose one arrow meaning for context cycling:\n%s", floor)
	}
	if strings.Contains(first, "[J ") || strings.Contains(first, "[J│") || strings.Contains(first, "[J/K]ctx-scroll") {
		t.Fatalf("compact split footer should expose a complete short scroll cue:\n%s", floor)
	}
	if got := ansi.StringWidth(first); got > m.Width {
		t.Fatalf("footer line exceeds width %d: %d %q", m.Width, got, first)
	}
}

func TestSplitEpistemicsFooterDoesNotClipScrollToken(t *testing.T) {
	rows := make([]grammar.EpistemicReadRow, 0, 36)
	for i := 0; i < 36; i++ {
		rows = append(rows, grammar.EpistemicReadRow{
			RowID: fmt.Sprintf("map-node:node-%02d", i), Family: "dynamics", SubjectKind: "map-node", SubjectRef: fmt.Sprintf("node-%02d", i),
			Status: "observed", Posture: "source-backed", AuthorityCase: "CASE-DYN", EvidenceCount: 1, Source: "seed",
			MapKind: "node", MapID: fmt.Sprintf("node-%02d", i),
			AIR: map[string]string{"row_id": "ok", "family": "ok", "subject_kind": "ok", "subject_ref": "ok", "status": "ok", "posture": "ok", "authority_case": "ok", "evidence_count": "ok", "source": "ok", "map_kind": "ok", "map_id": "ok"},
		})
	}
	m := New("REINS").
		FoldSessions([]grammar.Session{{
			Role: "cx-source", Platform: "codex", State: "active", Readiness: "claim", Attention: 0.88,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "attention": "ok"},
		}}, false).
		FoldEpistemics(grammar.EpistemicsSummary{Rows: rows}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 180, 44, PageEpistemics, true
	if !m.splitContextActive() || m.referenceScrollMax() == 0 {
		t.Fatalf("test requires scrollable split epistemics, split=%v max=%d", m.splitContextActive(), m.referenceScrollMax())
	}
	first := strings.Split(ansi.Strip(m.viewFloor(m.Width)), "\n")[0]
	if strings.Contains(first, "[J/K]") && !strings.Contains(first, "[J/K]scroll") && !strings.Contains(first, "[J/K]ctx-scroll") {
		t.Fatalf("split footer must not clip the scroll token:\n%s", first)
	}
	if strings.Contains(first, "[J/K]s │") || strings.Contains(first, "[J/K]s│") {
		t.Fatalf("split footer exposed a clipped scroll command:\n%s", first)
	}
	if got := ansi.StringWidth(first); got > m.Width {
		t.Fatalf("footer line exceeds width %d: %d %q", m.Width, got, first)
	}
}

func TestSplitReferenceContextHasSeparateScrollKeys(t *testing.T) {
	m := New("REINS").FoldSessions([]grammar.Session{
		{Role: "cx-a", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88, AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"}},
		{Role: "cx-b", Platform: "claude", State: "active", Readiness: "stale", Blocker: "stale_relay", Attention: 0.50, AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"}},
	}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 180, 16, PageSurfaces, true
	if !m.splitContextActive() {
		t.Fatal("test requires active split context")
	}
	if max := m.referenceScrollMax(); max == 0 {
		t.Fatal("surfaces split context should overflow in a short frame")
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "ctx surface registry · affordance registry") ||
		!strings.Contains(v, "[j/k]anchor") || !strings.Contains(v, "[J/K]scroll") || strings.Contains(v, "[j/k/J/K]ctx-scroll") {
		t.Fatalf("split reference view should advertise a lane anchor and separate right-pane scroll keys:\n%s", v)
	}
	for _, line := range strings.Split(v, "\n") {
		if strings.Contains(line, "surface registry") && strings.Contains(line, "…") {
			t.Fatalf("compact split relation header should not clip into ellipsis:\n%s", v)
		}
	}

	m = step(m, "j")
	if m.SFocus != 1 || m.RefScroll != 0 {
		t.Fatalf("lowercase j should move the left source row, sFocus=%d refScroll=%d status=%q", m.SFocus, m.RefScroll, m.Status)
	}
	m = step(m, "J")
	if m.SFocus != 1 || m.RefScroll != 1 || !strings.Contains(m.Status, "context scroll") {
		t.Fatalf("uppercase J should scroll right context only, sFocus=%d refScroll=%d status=%q", m.SFocus, m.RefScroll, m.Status)
	}
	m = step(m, "K")
	if m.SFocus != 1 || m.RefScroll != 0 {
		t.Fatalf("uppercase K should scroll right context back up without moving source, sFocus=%d refScroll=%d", m.SFocus, m.RefScroll)
	}
}

func TestSplitLinkedContextMarksRightPaneOverflow(t *testing.T) {
	m := New("REINS").
		Fold([]grammar.Event{{
			TS: "10:00", Kind: "coord_dispatch.launch_failed", Subject: "overflow-task", Actor: "cx-a", Score: 0.9,
			AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "summary": "ok", "score": "ok"},
		}}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-a", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"},
		}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 170, 14, PageEvents, true
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "context rows hidden") {
		t.Fatalf("short split linked context should mark hidden right-pane rows:\n%s", v)
	}
}

func TestSplitCompactLinkedPanesIgnoreRightScrollKeys(t *testing.T) {
	m := New("REINS").
		Fold([]grammar.Event{{
			TS: "10:00", Kind: "coord_dispatch.launch_started", Subject: "task-a", Actor: "cx-a", Score: 0.21,
			AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "summary": "ok", "score": "ok"},
		}}, false).
		FoldSessions([]grammar.Session{
			{Role: "cx-a", Platform: "codex", State: "active", Readiness: "claim", Attention: 0.88, AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "attention": "ok"}},
			{Role: "cx-b", Platform: "claude", State: "active", Readiness: "stale", Attention: 0.50, AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "attention": "ok"}},
		}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 180, 20, PageEvents, true
	if rel := m.splitRelation(); rel.PaneProfile() != PaneLinkedCompact || rel.TargetScrollable {
		t.Fatalf("events split should be compact linked, got profile=%s scroll=%v", rel.PaneProfile(), rel.TargetScrollable)
	}
	beforeSource, beforeEvent, beforeScroll, beforeStatus := m.SFocus, m.EFocus, m.RefScroll, m.Status
	m = step(m, "J")
	m = step(m, "K")
	if m.SFocus != beforeSource || m.EFocus != beforeEvent || m.RefScroll != beforeScroll || m.Status != beforeStatus {
		t.Fatalf("compact split J/K should not move hidden target state, before source/event/scroll/status=%d/%d/%d/%q after=%d/%d/%d/%q",
			beforeSource, beforeEvent, beforeScroll, beforeStatus, m.SFocus, m.EFocus, m.RefScroll, m.Status)
	}
	m = step(m, "j")
	if m.SFocus != beforeSource+1 || m.EFocus != beforeEvent || m.RefScroll != beforeScroll {
		t.Fatalf("compact split j should move only source session, source/event/scroll=%d/%d/%d", m.SFocus, m.EFocus, m.RefScroll)
	}
}

func TestSplitContextHandlesAggregatedRepeatedRuneKeys(t *testing.T) {
	m := New("REINS").FoldSessions([]grammar.Session{
		{Role: "cx-a", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88, AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"}},
		{Role: "cx-b", Platform: "claude", State: "active", Readiness: "stale", Blocker: "stale_relay", Attention: 0.50, AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"}},
	}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 180, 16, PageSurfaces, true
	if max := m.referenceScrollMax(); max < 3 {
		t.Fatalf("test requires at least three context scroll rows, max=%d", max)
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("JJJ")})
	m = nm.(Model)
	if m.SFocus != 0 || m.RefScroll != 3 {
		t.Fatalf("aggregated uppercase J runes should scroll context only, sFocus=%d refScroll=%d", m.SFocus, m.RefScroll)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jj")})
	m = nm.(Model)
	if m.SFocus != 1 || m.RefScroll != 3 {
		t.Fatalf("aggregated lowercase j runes should move the source row only, sFocus=%d refScroll=%d", m.SFocus, m.RefScroll)
	}
}

func TestSplitContextQueuedIndicatorAtNarrowWidth(t *testing.T) {
	m := New("REINS").FoldSessions([]grammar.Session{{
		Role: "cx-a", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88,
		AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"},
	}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 120, 32, PageCaps, true

	v := ansi.Strip(m.View())
	if strings.Contains(v, "split:ctx") || strings.Contains(v, "split sessions") {
		t.Fatalf("narrow width should not claim split context is active:\n%s", v)
	}
	if !strings.Contains(v, "split:wide") {
		t.Fatalf("narrow width should show split is queued on width, not active:\n%s", v)
	}
}

func TestSplitContextThresholdKeepsBodyAndNavigationCoherent(t *testing.T) {
	sessions := []grammar.Session{
		{Role: "cx-a", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88, AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"}},
		{Role: "cx-b", Platform: "claude", State: "active", Readiness: "stale", Blocker: "stale_relay", Attention: 0.50, AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"}},
	}
	events := []grammar.Event{
		{TS: "10:00", Kind: "coord_dispatch.launch_started", Subject: "task-a", Actor: "cx-a", AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok"}},
		{TS: "10:01", Kind: "coord_dispatch.launch_started", Subject: "task-b", Actor: "cx-b", AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok"}},
	}
	m := New("REINS").Fold(events, false).FoldSessions(sessions, false)
	m.Width, m.Height, m.Page, m.SplitContext = splitContextMinWidth-1, 34, PageEvents, true
	if m.splitContextActive() {
		t.Fatal("split must remain inactive below the render threshold")
	}
	v := ansi.Strip(m.View())
	if strings.Contains(v, "split sessions") || !strings.Contains(v, "split:wide") {
		t.Fatalf("queued split should render as wide request without split body:\n%s", v)
	}
	m = step(m, "j")
	if m.EFocus != 1 || m.SFocus != 0 {
		t.Fatalf("queued split must keep j on visible event rows, eFocus=%d sFocus=%d", m.EFocus, m.SFocus)
	}

	m.Width = splitContextMinWidth
	if !m.splitContextActive() {
		t.Fatal("split should become active at the render threshold")
	}
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "split sessions") || !strings.Contains(v, "split:ctx") {
		t.Fatalf("active split should render the source pane and ctx chip:\n%s", v)
	}
	m = step(m, "j")
	if m.SFocus != 1 || m.EFocus != 1 {
		t.Fatalf("active split must route j to source sessions only, eFocus=%d sFocus=%d", m.EFocus, m.SFocus)
	}
}

func TestQueuedNarrowSplitDoesNotAdvertiseInactiveContextScroll(t *testing.T) {
	m := New("REINS")
	m.Page, m.Width, m.Height, m.SplitContext = PageHelp, splitContextMinWidth-1, 34, true
	rows := m.referencePageRailRows()
	var joined string
	for _, row := range rows {
		joined += row.value + "\n"
	}
	if strings.Contains(joined, "[J/K] ctx") {
		t.Fatalf("queued narrow split should not advertise inactive context scroll:\n%s", joined)
	}

	m.Width = splitContextMinWidth
	rows = m.referencePageRailRows()
	joined = ""
	for _, row := range rows {
		joined += row.value + "\n"
	}
	if !strings.Contains(joined, "[J/K] ctx") {
		t.Fatalf("active split should advertise context scroll when applicable:\n%s", joined)
	}
}

func TestSplitTaskContextWrapsLongClaimAndAuthority(t *testing.T) {
	longTask := "claimed-task-alpha-beta-gamma-delta-epsilon-zeta-eta-theta-iota-tail-marker"
	longAuthority := "AUTHORITY-CASE-alpha-beta-gamma-delta-epsilon-zeta-tail-marker"
	m := New("REINS").
		FoldTasks([]grammar.Task{{
			TaskID: longTask, Stage: "S7_RELEASE", PriorStage: "S6_IMPL", PredictedStage: "hold",
			Owner: "cx-task", Criticality: "crit", AuthorityCase: longAuthority,
			NoGo: "docs_mutation_authorized,implementation_authorized,release_cutover_tail_authorized",
			AIR: map[string]string{
				"task_id": "ok", "stage": "ok", "prior_stage": "ok", "predicted_stage": "ok",
				"owner": "ok", "criticality": "ok", "authority_case": "ok", "no_go": "ok",
			},
		}}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-source", Platform: "codex", State: "active", Readiness: "claim", ClaimedTask: longTask,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "claimed_task": "ok"},
		}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 170, 46, PageTasks, true

	v := ansi.Strip(m.View())
	for _, want := range []string{"WORK DOMAIN", "tail-marker", "AUTHORITY-CASE-alpha-beta-gamma-delta-epsilon-zeta-tail-", "marker", "release_cutover_tail"} {
		if !strings.Contains(v, want) {
			t.Fatalf("split task work-domain should preserve long value %q:\n%s", want, v)
		}
	}
	for i, line := range strings.Split(v, "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("split task line %d exceeds frame width %d: %d %q", i, m.Width, got, line)
		}
	}
}

func TestSplitSessionContextWrapsLongBlockerAndClaim(t *testing.T) {
	longBlocker := "blocked-by-route-evidence-alpha-beta-gamma-delta-epsilon-tail-marker"
	longClaim := "claimed-task-alpha-beta-gamma-delta-epsilon-zeta-eta-tail-marker"
	m := New("REINS").FoldSessions([]grammar.Session{{
		Role: "cx-session", Platform: "codex", State: "active", Readiness: "stale", Blocker: longBlocker,
		ClaimedTask: longClaim, Attention: 0.72,
		AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "claimed_task": "ok", "attention": "ok"},
	}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 170, 46, PageSessions, true

	v := ansi.Strip(m.View())
	for _, want := range []string{"LANE READINESS", "blocked-by-route-evidence-alpha-beta-gamma-delta-epsilon-", "claimed-task-alpha-beta-gamma-delta-epsilon-zeta-eta-tail-", "tail-marker"} {
		if !strings.Contains(v, want) {
			t.Fatalf("split session context should preserve long value %q:\n%s", want, v)
		}
	}
	for i, line := range strings.Split(v, "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("split session line %d exceeds frame width %d: %d %q", i, m.Width, got, line)
		}
	}
}

func TestSessionsWidePaneRendersWorkSurfaceAndDetailRefs(t *testing.T) {
	m := New("REINS").
		FoldSessions([]grammar.Session{{
			Role: "cx-session", Session: "tmux-secret", Platform: "codex", State: "active", Readiness: "claim",
			ClaimedTask: "task-1", Attention: 0.82, RouteID: "codex.headless.full", RouteBindingState: "policy_only",
			AIR: map[string]string{"role": "ok", "session": "deny", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "ok", "route_id": "ok", "route_binding_state": "ok", "mode": "ok", "profile": "ok", "route_evidence_ref": "deny", "output_age_s": "ok", "relay_age_s": "ok"},
		}}, false).
		FoldSessionDetail(grammar.SessionDetail{
			Role:      "cx-session",
			Platform:  "codex",
			State:     "active",
			Readiness: "claim",
			Task: grammar.SessionTaskDetail{
				TaskID: "task-1", Status: "claimed", AuthorityCase: "SECRET-CASE", ParentSpec: "SECRET-PARENT", MutationSurface: "source",
			},
			EvidenceRefs: []grammar.EvidenceRef{
				{Kind: "cc_task_note", Path: "/secret/task.md", Size: 12},
				{Kind: "transcript_candidate", Path: "/secret/transcript.jsonl", Size: 34},
			},
			EvidenceSummary: grammar.SessionEvidenceSummary{
				Total:                   2,
				ByKind:                  map[string]int{"cc_task_note": 1, "transcript_candidate": 1},
				TranscriptRootsObserved: 1,
				TranscriptRootsMissing:  0,
				Truncated:               false,
				Privacy:                 "metadata-only; transcript candidates raw_access=false",
				RawAccess:               false,
			},
			Resume: grammar.ResumeContext{Intent: "session.resume", Ready: false, Authority: "supervisor_or_methodology_dispatch", BlockedReasons: []string{"not_wired"}},
			AIR:    map[string]string{"task_id": "ok", "status": "ok", "mutation_surface": "ok", "authority_case": "deny", "parent_spec": "deny", "evidence_count": "ok", "path": "deny", "resume_ready": "ok"},
		}, false)
	m.Width, m.Height, m.Page = 190, 46, PageSessions

	v := ansi.Strip(m.View())
	for _, want := range []string{"session work surface", "{{sel.role}}", "{{sel.claimed_task}}", ":intent resume", "task=task-1", "status=claimed", "mutation=source", "evidence=2", "task-note:1,transcript:1", "roots observed=1 missing=0", "raw_access=false", "resume_ready=false"} {
		if !strings.Contains(v, want) {
			t.Fatalf("session work surface missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "SECRET_TRANSCRIPT") {
		t.Fatalf("session work surface must not render raw transcript content:\n%s", v)
	}

	m.AIR = true
	v = ansi.Strip(m.View())
	for _, leak := range []string{"SECRET-CASE", "SECRET-PARENT", "/secret/task.md", "/secret/transcript.jsonl", "tmux-secret"} {
		if strings.Contains(v, leak) {
			t.Fatalf("AIR session work surface leaked %q:\n%s", leak, v)
		}
	}
	if !strings.Contains(v, "case=▒▒▒") || !strings.Contains(v, "parent=▒▒▒") {
		t.Fatalf("AIR session work surface should preserve authority structure with redaction:\n%s", v)
	}
}

func TestSplitEventContextWrapsLongRelatedEventFields(t *testing.T) {
	longSubject := "event-subject-alpha-beta-gamma-delta-epsilon-zeta-eta-tail-marker"
	longSummary := "summary-alpha-beta-gamma-delta-epsilon-zeta-eta-tail-marker"
	m := New("REINS").
		Fold([]grammar.Event{
			{TS: "10:00", Kind: "coord_dispatch.launch_started", Subject: longSubject, Actor: "cx-event", Summary: longSummary, Score: 0.21,
				AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "summary": "ok", "score": "ok"}},
			{TS: "10:05", Kind: "coord_dispatch.launch_failed", Subject: longSubject, Actor: "cx-other", Summary: longSummary + "-failure", Score: 0.82,
				AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "summary": "ok", "score": "ok"}},
		}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-event", Platform: "codex", State: "active", Readiness: "claim", ClaimedTask: longSubject, Attention: 0.71,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "claimed_task": "ok", "attention": "ok"},
		}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 170, 50, PageEvents, true

	v := ansi.Strip(m.View())
	for _, want := range []string{"EVENT NEIGHBORHOOD", "event-subject-alpha-beta-gamma-delta-epsilon-", "summary-alpha-beta-gamma-delta-epsilon-", "tail-marker"} {
		if !strings.Contains(v, want) {
			t.Fatalf("split event context should preserve long value %q:\n%s", want, v)
		}
	}
	for i, line := range strings.Split(v, "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("split event line %d exceeds frame width %d: %d %q", i, m.Width, got, line)
		}
	}
}

func TestSplitContextNavigationOwnsVisibleSessionSource(t *testing.T) {
	m := New("REINS").
		Fold([]grammar.Event{
			{TS: "10:00", Kind: "coord_dispatch.launch_started", Subject: "task-a", Actor: "cx-a", Score: 0.21,
				AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "summary": "ok"}},
			{TS: "10:05", Kind: "coord_dispatch.launch_failed", Subject: "task-b", Actor: "cx-b", Score: 0.82,
				AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "summary": "ok"}},
		}, false).
		FoldSessions([]grammar.Session{
			{Role: "cx-a", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.70,
				AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "ok", "session": "ok"}},
			{Role: "cx-b", Platform: "claude", State: "active", Readiness: "stale", Blocker: "stale_relay", Attention: 0.55,
				AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "ok", "session": "ok"}},
		}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 220, 44, PageEvents, true
	m.EFocus, m.SFocus = 0, 0

	m = step(m, "j")
	if m.SFocus != 1 {
		t.Fatalf("split j should move the visible session source, got SFocus=%d", m.SFocus)
	}
	if m.EFocus != 0 {
		t.Fatalf("split j must not mutate the hidden/right event cursor, got EFocus=%d", m.EFocus)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"sessions -> events by actor/task", "source focus drives context by actor/task", "EVENT NEIGHBORHOOD", "source", "cx-b", "matched", "1 events", "1 events · 1 fail", "task-b", "focus cx-b -> events", "[j/k]source"} {
		if !strings.Contains(v, want) {
			t.Fatalf("split relation/source affordance missing %q:\n%s", want, v)
		}
	}
}

func TestSplitContextTaskPaneUsesSelectedSessionClaim(t *testing.T) {
	m := New("REINS").
		FoldTasks([]grammar.Task{
			{TaskID: "hidden-task", Stage: "S1", Owner: "other", AIR: map[string]string{"task_id": "ok", "stage": "ok", "owner": "ok"}},
			{TaskID: "linked-task", Stage: "S7", PredictedStage: "hold", Owner: "cx-link", Criticality: "crit", AIR: map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok", "owner": "ok", "criticality": "ok"}},
		}, false).
		FoldSessions([]grammar.Session{
			{Role: "cx-link", Platform: "codex", State: "active", Readiness: "claim", ClaimedTask: "linked-task", Blocker: "none", Attention: 0.75,
				AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "ok", "session": "ok"}},
		}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 220, 44, PageTasks, true
	m.Focus, m.SFocus = 0, 0

	v := ansi.Strip(m.View())
	for _, want := range []string{"sessions -> task work by claimed_task", "selected session claimed task", "source      cx-link", "task        linked-task", "release blocked"} {
		if !strings.Contains(v, want) {
			t.Fatalf("split task context should derive from selected session claim, missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "task        hidden-task") {
		t.Fatalf("split task context should not use hidden task cursor:\n%s", v)
	}
}

func TestSplitTaskPaneBlocksHiddenTaskControls(t *testing.T) {
	base := New("REINS").
		FoldTasks([]grammar.Task{
			{TaskID: "hidden-task", Stage: "S1", Owner: "other", Criticality: "warn", AIR: map[string]string{"task_id": "ok", "stage": "ok", "owner": "ok", "criticality": "ok"}},
			{TaskID: "linked-task", Stage: "S7", PredictedStage: "hold", Owner: "cx-link", Criticality: "crit", AIR: map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok", "owner": "ok", "criticality": "ok"}},
		}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-link", Platform: "codex", State: "active", Readiness: "claim", ClaimedTask: "linked-task", Blocker: "none", Attention: 0.75,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "ok"},
		}}, false)
	base.Width, base.Height, base.Page, base.SplitContext = 220, 44, PageTasks, true
	base.Focus, base.SFocus, base.Sel.Rank = 1, 0, RankRow

	for _, k := range []string{"/", "f", "tab", "V"} {
		t.Run(k, func(t *testing.T) {
			m := step(base, k)
			if m.Mode != ModeNormal {
				t.Fatalf("split task key %q should not enter hidden task mode, got mode=%d status=%q", k, m.Mode, m.Status)
			}
			if m.Focus != base.Focus || m.SFocus != base.SFocus {
				t.Fatalf("split task key %q should not move hidden/source cursors, focus=%d/%d sfocus=%d/%d", k, m.Focus, base.Focus, m.SFocus, base.SFocus)
			}
			if m.Sel.Rank != RankRow || m.Sel.Field != "" || len(m.Sel.Members) != 0 {
				t.Fatalf("split task key %q should not mutate hidden task selection: %+v", k, m.Sel)
			}
			if m.Filter != "" || m.CritFilter != "" {
				t.Fatalf("split task key %q should not mutate hidden task filters: filter=%q crit=%q", k, m.Filter, m.CritFilter)
			}
			if !strings.Contains(m.Status, "split tasks: source pane owns") {
				t.Fatalf("split task key %q should explain ownership, got status=%q", k, m.Status)
			}
		})
	}
}

func TestSplitYardUsesSelectedSessionAsTrainyardDrilldown(t *testing.T) {
	m := New("REINS").
		Fold([]grammar.Event{
			{TS: "10:00", Kind: "coord_dispatch.launch_started", Subject: "linked-task", Actor: "cx-yard", Score: 0.20,
				AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "summary": "ok"}},
			{TS: "10:05", Kind: "coord_dispatch.launch_failed", Subject: "linked-task", Actor: "cx-yard", Score: 0.90,
				AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "summary": "ok"}},
		}, false).
		FoldTasks([]grammar.Task{
			{TaskID: "linked-task", Stage: "S7_RELEASE", PredictedStage: "hold", Criticality: "crit", Owner: "cx-yard",
				AIR: map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok", "criticality": "ok", "owner": "ok"}},
		}, false).
		FoldCapabilities(grammar.CapabilitySummary{
			Rows: []grammar.CapabilityRow{
				{CapabilityID: "route_envelope", Status: "observed", Authority: "registry evidence", RouteCount: 2, OKCount: 1, BlockedCount: 1, EvidenceCount: 4,
					AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "blocker": "ok", "hkp_posture": "ok"}},
				{CapabilityID: "wip_fanout", Status: "observed", Authority: "projection", RouteCount: 1, OKCount: 1, BlockedCount: 0, EvidenceCount: 2,
					AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "blocker": "ok", "hkp_posture": "ok"}},
			},
			Routes: []grammar.CapabilityRoute{
				{RouteID: "codex.full", CapabilityID: "route_envelope", Platform: "codex", RouteState: "active", FreshnessOK: true, QuotaState: "observed", ReceiptCount: 2,
					AIR: map[string]string{"route_id": "ok", "capability_id": "ok", "platform": "ok", "route_state": "ok", "freshness_ok": "ok", "quota_state": "ok", "receipt_count": "ok"}},
				{RouteID: "codex.blocked", CapabilityID: "wip_fanout", Platform: "codex", RouteState: "blocked", FreshnessOK: false, QuotaState: "unknown", Blockers: []string{"quota"},
					AIR: map[string]string{"route_id": "ok", "capability_id": "ok", "platform": "ok", "route_state": "ok", "freshness_ok": "ok", "quota_state": "ok", "receipt_count": "ok"}},
			},
			Tools: []grammar.CapabilityTool{
				{RouteID: "codex.full", Platform: "codex", ToolID: "filesystem", Status: "observed", Available: true, AuthorityUse: "read,write", ObservedAt: "2026-06-25T00:00:00Z",
					AIR: map[string]string{"route_id": "ok", "platform": "ok", "tool_id": "ok", "status": "ok", "available": "ok", "authority_use": "ok", "observed_at": "ok"}},
				{RouteID: "codex.blocked", Platform: "codex", ToolID: "local_shell", Status: "read-missing", Available: true, AuthorityUse: "read,execute",
					AIR: map[string]string{"route_id": "ok", "platform": "ok", "tool_id": "ok", "status": "ok", "available": "ok", "authority_use": "ok", "observed_at": "ok"}},
			},
		}, false).
		FoldGates(grammar.GateSummary{Rows: []grammar.GateRow{
			{GateID: "task.no_go.release_authorized", Domain: "task", State: "blocked", Severity: "crit", AIR: map[string]string{"gate_id": "ok", "domain": "ok", "state": "ok", "severity": "ok"}},
			{GateID: "lane.blocker.stale_relay", Domain: "lane", State: "blocked", Severity: "warn", AIR: map[string]string{"gate_id": "ok", "domain": "ok", "state": "ok", "severity": "ok"}},
			{GateID: "command.dispatch", Domain: "command", State: "preview-only", Severity: "warn", AIR: map[string]string{"gate_id": "ok", "domain": "ok", "state": "ok", "severity": "ok"}},
		}}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-yard", Platform: "codex", State: "active", Readiness: "claim", ClaimedTask: "linked-task", Blocker: "none", Attention: 0.88,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "claimed_task": "ok", "blocker": "ok", "attention": "ok"},
		}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 220, 44, PageYard, true

	v := ansi.Strip(m.View())
	for _, want := range []string{
		"sessions -> yard drilldown by lane/task",
		"SELECTED TRAINYARD LANE",
		"lane        cx-yard",
		"claimed     linked-task",
		"task        task visible",
		"stage       S7_RELEASE",
		"events      2 actor/task events",
		"failures    1 recent",
		"gate        release hold visible",
		"fit         resume-preview",
		"2 codex routes · 1 fresh · 1 blocked · receipts:2",
		"route_envelope:observed · wip_fanout:observed",
		"2 candidate codex route tools",
		"filesystem:observed",
		"local_shell:read-missing",
		"exact per-session needs route_id",
		"aggregate gates task:1 lane:1 route:0 command:1 blocked:2 preview:1",
		"focus cx-yard -> yard drilldown",
		"source focus drives context by lane/task",
		"[j/k]src→ctx",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("split yard drilldown missing %q:\n%s", want, v)
		}
	}
}

func TestSplitYardPinnedContextLeavesBodyRows(t *testing.T) {
	m := New("REINS").
		FoldTasks([]grammar.Task{
			{TaskID: "linked-task", Stage: "S7_RELEASE", PredictedStage: "hold", Criticality: "crit", Owner: "cx-yard",
				AIR: map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok", "criticality": "ok", "owner": "ok"}},
		}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-yard", Platform: "codex", State: "active", Readiness: "claim", ClaimedTask: "linked-task", Blocker: "none", Attention: 0.88,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "claimed_task": "ok", "blocker": "ok", "attention": "ok"},
		}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 180, 16, PageYard, true

	v := ansi.Strip(m.View())
	for _, want := range []string{"SELECTED TRAINYARD LANE", "pinned context rows hidden", "YARD", "RAIL TOPOLOGY", "stations"} {
		if !strings.Contains(v, want) {
			t.Fatalf("short split yard should preserve pinned context and body rows, missing %q:\n%s", want, v)
		}
	}
}

func TestSourceOnlySplitRowsCarryLaneAnchorSignals(t *testing.T) {
	m := New("REINS").
		FoldSessions([]grammar.Session{
			{Role: "cx-claim", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88,
				AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"}},
			{Role: "cx-stale", Platform: "claude", State: "active", Readiness: "stale", Blocker: "stale_relay", Attention: 0.50,
				AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"}},
		}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 180, 18, PageDomains, true

	v := ansi.Strip(m.View())
	for _, want := range []string{"claim-ready · domain lens", "stale relay · domain lens"} {
		if !strings.Contains(v, want) {
			t.Fatalf("source-only split rows should show lane-specific anchor signal %q:\n%s", want, v)
		}
	}
}

func TestSourceOnlySplitPanesRenderRelationCard(t *testing.T) {
	m := New("REINS").
		FoldSessions([]grammar.Session{{
			Role: "cx-anchor", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"},
		}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 180, 24, PageDomains, true

	v := ansi.Strip(m.View())
	for _, want := range []string{"SPLIT RELATION", "sessions + domain registry as domain lens", "cx-anchor · claim-ready", "domain lens situates selected lane", "[j/k] lane anchor"} {
		if !strings.Contains(v, want) {
			t.Fatalf("source-only split relation card missing %q:\n%s", want, v)
		}
	}
}

func TestSplitSourceLivePulseTracksBeatWithoutMovingSelection(t *testing.T) {
	m := New("REINS").
		FoldSessions([]grammar.Session{{
			Role: "cx-live", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88, Alive: true,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "alive": "ok", "idle": "ok", "stalled": "ok"},
		}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 180, 24, PageDomains, true

	v0 := ansi.Strip(m.View())
	if !strings.Contains(v0, "cx-live · claim-ready · live ·") || !strings.Contains(v0, "◆ claim   cx-live") {
		t.Fatalf("live source split should show anchored selection plus live pulse:\n%s", v0)
	}
	m.Beat = 2
	v2 := ansi.Strip(m.View())
	if !strings.Contains(v2, "cx-live · claim-ready · live •") || !strings.Contains(v2, "◆ claim   cx-live") {
		t.Fatalf("live source pulse should change with Beat without moving selection:\n%s", v2)
	}
}

func TestSplitSourceLivePulseAllowsHealthyNoClaimLane(t *testing.T) {
	m := New("REINS").
		FoldSessions([]grammar.Session{{
			Role: "cx-live", Platform: "codex", State: "active", Readiness: "live", Blocker: "no_claim", Attention: 0.21, Alive: true,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "alive": "ok", "idle": "ok", "stalled": "ok"},
		}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 180, 24, PageDomains, true

	v := ansi.Strip(m.View())
	if !strings.Contains(v, "no claim · domain lens · live ·") {
		t.Fatalf("healthy no_claim lanes should still show a live pulse:\n%s", v)
	}
}

func TestSplitReferenceCatalogsDoNotExposeInactiveTargetFocus(t *testing.T) {
	base := New("REINS").FoldSessions([]grammar.Session{
		{Role: "cx-one", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"}},
		{Role: "cx-two", Platform: "claude", State: "active", Readiness: "stale", Blocker: "stale_relay", Attention: 0.55,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"}},
	}, false)
	base.Width, base.Height, base.SplitContext = 220, 52, true

	for _, tc := range []struct {
		name       string
		page       int
		focus      func(Model) int
		inactive   string
		focusGlyph string
		stale      []string
	}{
		{"commands", PageCommands, func(m Model) int { return m.CommandFocus }, "[j/k]command", "▶ tasks", []string{"selected command"}},
		{"windows", PageWindows, func(m Model) int { return m.WindowFocus }, "[j/k]window", "▶ 2 tasks", []string{"selected window"}},
		{"surfaces", PageSurfaces, func(m Model) int { return m.SurfaceFocus }, "[j/k]surface", "▶ split-context", []string{"selected surface"}},
		{"domains", PageDomains, func(m Model) int { return m.DomainFocus }, "[j/k]domain", "▶ capability-routing", []string{"selected capability-routing"}},
		{"intent", PageIntent, func(m Model) int { return m.IntentFocus }, "[j/k]intent", "▶ dispatch", []string{"intent target dispatch"}},
		{"epistemics", PageEpistemics, func(m Model) int { return m.EpiFocus }, "[j/k]evidence", "▶ dynamics", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := base
			m.Page = tc.page
			beforeSource, beforeTarget := m.SFocus, tc.focus(m)
			m = step(m, "j")
			if m.SFocus != beforeSource+1 {
				t.Fatalf("split %s j should move source session focus, before=%d after=%d", tc.name, beforeSource, m.SFocus)
			}
			if got := tc.focus(m); got != beforeTarget {
				t.Fatalf("split %s j should not move inactive target focus, before=%d after=%d", tc.name, beforeTarget, got)
			}
			v := ansi.Strip(m.View())
			if strings.Contains(v, tc.inactive) {
				t.Fatalf("split %s should not advertise inactive target j/k control %q:\n%s", tc.name, tc.inactive, v)
			}
			if strings.Contains(v, tc.focusGlyph) {
				t.Fatalf("split %s should not render inactive target focus glyph %q:\n%s", tc.name, tc.focusGlyph, v)
			}
			flat := strings.Join(strings.Fields(v), " ")
			for _, stale := range tc.stale {
				if strings.Contains(flat, stale) {
					t.Fatalf("split %s should not emit stale passive target context %q:\n%s", tc.name, stale, v)
				}
			}
			if !strings.Contains(v, "[j/k] lane anchor") && !strings.Contains(v, "[j/k]lane anchor") && !strings.Contains(v, "[j/k]anchor") {
				t.Fatalf("split %s should advertise lane-anchor navigation:\n%s", tc.name, v)
			}
		})
	}
}

func TestSplitSessionsPaneUsesSlackForTopology(t *testing.T) {
	m := New("REINS").
		Fold([]grammar.Event{{
			TS: "10:00", Kind: "coord_dispatch.launch_started", Subject: "task-a", Actor: "cx-anchor", Score: 0.21,
			AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok"},
		}}, false).
		FoldTasks([]grammar.Task{{TaskID: "task-a", AIR: map[string]string{"task_id": "ok"}}}, false).
		FoldCapabilities(grammar.CapabilitySummary{
			Routes: []grammar.CapabilityRoute{{
				RouteID: "codex.full", Platform: "codex", RouteState: "active",
				AIR: map[string]string{"route_id": "ok", "platform": "ok", "route_state": "ok"},
			}},
		}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-anchor", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88, ClaimedTask: "task-a",
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "ok"},
		}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 220, 34, PageDomains, true

	v := ansi.Strip(m.View())
	for _, want := range []string{"cx-anchor · claim-ready", "sessions + domain registry as domain lens", "events:1 · claim:task visible · cap-routes:1", "[j/k] lane anchor"} {
		if !strings.Contains(v, want) {
			t.Fatalf("split session slack topology missing %q:\n%s", want, v)
		}
	}
}

func TestSplitReadinessExplainsSelectedLaneGate(t *testing.T) {
	m := New("REINS").
		FoldTasks([]grammar.Task{
			{TaskID: "linked-task", Stage: "S7_RELEASE", PredictedStage: "hold", Criticality: "crit", Owner: "cx-ready",
				AIR: map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok", "criticality": "ok", "owner": "ok"}},
		}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-ready", Platform: "codex", State: "active", Readiness: "claim", ClaimedTask: "linked-task", Blocker: "none", Attention: 0.88,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "claimed_task": "ok", "blocker": "ok", "attention": "ok"},
		}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 220, 44, PageReadiness, true

	v := ansi.Strip(m.View())
	for _, want := range []string{
		"sessions -> gate stack by role/claimed_task",
		"SELECTED LANE GATE",
		"lane        cx-ready",
		"claimed     linked-task",
		"task gate   release hold",
		"lane gate   release hold visible",
		"route       governed COMMAND route required",
		"receipt     not wired/read-missing",
		"[j/k]src→ctx",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("split readiness gate missing %q:\n%s", want, v)
		}
	}
}

func TestLinkedSplitPinsSelectedContextWhileRightPaneScrolls(t *testing.T) {
	m := New("REINS").
		FoldTasks([]grammar.Task{
			{TaskID: "linked-task", Stage: "S7_RELEASE", PredictedStage: "hold", Criticality: "crit", Owner: "cx-ready",
				AIR: map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok", "criticality": "ok", "owner": "ok"}},
		}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-ready", Platform: "codex", State: "active", Readiness: "claim", ClaimedTask: "linked-task", Blocker: "none", Attention: 0.88,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "claimed_task": "ok", "blocker": "ok", "attention": "ok"},
		}}, false).
		FoldGates(grammar.GateSummary{Rows: []grammar.GateRow{
			{GateID: "task.no_go.release_authorized", Domain: "task", State: "blocked", Severity: "crit", Authority: "coord_projection", Evidence: "false_on=1", Missing: "release_authorized",
				AIR: map[string]string{"gate_id": "ok", "domain": "ok", "state": "ok", "severity": "ok", "authority": "ok", "evidence": "ok", "missing": "ok"}},
			{GateID: "command.dispatch", Domain: "command", State: "preview-only", Severity: "warn", Authority: "methodology dispatch", Evidence: "verbs registered", Missing: "authority_case,parent_spec,preflight,receipt",
				AIR: map[string]string{"gate_id": "ok", "domain": "ok", "state": "ok", "severity": "ok", "authority": "ok", "evidence": "ok", "missing": "ok"}},
		}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 180, 34, PageReadiness, true
	if max := m.referenceScrollMax(); max == 0 {
		t.Fatal("test requires scrollable readiness body below pinned selected context")
	}
	m.RefScroll = m.referenceScrollMax()

	v := ansi.Strip(m.View())
	for _, want := range []string{
		"SELECTED LANE GATE",
		"lane        cx-ready",
		"task gate   release hold",
		"command.dispatch",
		"GUARDRAILS",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("pinned split context missing %q after scroll:\n%s", want, v)
		}
	}
}

func TestIntakeProjectionRendersSourceBackedMetadataAndSplit(t *testing.T) {
	m := New("REINS").
		Fold([]grammar.Event{
			{TS: "10:00", Kind: "coord_dispatch.launch_failed", Subject: "linked-task", Actor: "cx-intake", AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok"}},
			{TS: "10:05", Kind: "coord_dispatch.launch_started", Subject: "other-task", Actor: "cx-intake", AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok"}},
		}, false).
		FoldTasks([]grammar.Task{{TaskID: "linked-task", AIR: map[string]string{"task_id": "ok"}}}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-intake", Platform: "codex", State: "active", Readiness: "claim", ClaimedTask: "linked-task", Attention: 0.82,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "claimed_task": "ok", "attention": "ok"},
		}, {
			Role: "cx-other", Platform: "claude", State: "active", Readiness: "stale", Blocker: "stale_relay", Attention: 0.54,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"},
		}}, false).
		FoldIntake(grammar.IntakeSummary{
			Sources: []grammar.IntakeSource{{
				ID: "request_state", Path: "/tmp/request-intake-state.json", Exists: true, Status: "observed", Count: 7, AgeBucket: "<5m", Privacy: "metadata-only",
				AIR: map[string]string{"id": "ok", "path": "ok", "exists": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "privacy": "ok"},
			}},
			Rows: []grammar.IntakeRow{
				{Source: "planning_feed", Kind: "coverage:untracked", Status: "bucket", Severity: "warn", Count: 3, Coverage: "untracked", TaskLinkState: "untracked", AgeBucket: "<5m", AIR: map[string]string{"source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok", "coverage": "ok", "task_link_state": "ok"}},
				{Source: "p0_incident_state", Kind: "incident:notification", Status: "active", Severity: "crit", Count: 2, Blocker: "p0_incident", Coverage: "coalesced", TaskLinkState: "task_link_metadata", AgeBucket: "<5m", AIR: map[string]string{"source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok", "blocker": "ok", "coverage": "ok", "task_link_state": "ok"}},
			},
			Totals: map[string]int{"request_attention": 7, "planning_attention": 3, "p0_incidents": 2},
		}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 220, 55, PageIntake, true

	v := ansi.Strip(m.View())
	for _, want := range []string{
		"sessions -> intake observations by actor/claimed_task",
		"SELECTED INTAKE NEIGHBORHOOD",
		"lane        cx-intake",
		"task link   task visible",
		"2 related",
		"1 failures",
		"ambient     12 attention units",
		"SOURCE FRESHNESS",
		"DEMAND TOTALS",
		"OBSERVATION BUCKETS",
		"request_state",
		"coverage:untracked",
		"p0_incident",
		"read-only projection",
		"[j/k]src→ctx",
		"[n/p]bucket",
		"[Enter]source detail",
		"[n/p] bucket",
		"[Enter] source detail",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("intake split projection missing %q:\n%s", want, v)
		}
	}
	for _, bad := range []string{"[Enter]detail", "[Enter] aggregate detail"} {
		if strings.Contains(v, bad) {
			t.Fatalf("split intake right pane should not advertise right-pane Enter action %q:\n%s", bad, v)
		}
	}
	for i, line := range strings.Split(v, "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("split intake line %d exceeds frame width %d: %d %q", i, m.Width, got, line)
		}
	}

	m = step(m, "n")
	if m.IFocus != 1 || m.SFocus != 0 || !strings.Contains(m.Status, "intake bucket 2/2") {
		t.Fatalf("[n] should move split intake bucket only, iFocus=%d sFocus=%d status=%q", m.IFocus, m.SFocus, m.Status)
	}
	cycled := step(m, "]")
	if cycled.Page == PageIntake || cycled.IFocus != m.IFocus || cycled.SFocus != m.SFocus {
		t.Fatalf("] should remain global window/context cycling in split intake, page=%s iFocus=%d/%d sFocus=%d/%d status=%q",
			pageLabel(cycled.Page), cycled.IFocus, m.IFocus, cycled.SFocus, m.SFocus, cycled.Status)
	}
	m = step(m, "j")
	if m.SFocus != 1 || m.IFocus != 1 {
		t.Fatalf("[j] should move split source lane without changing bucket, sFocus=%d iFocus=%d", m.SFocus, m.IFocus)
	}
}

func TestIntakeProjectionNavigatesBucketsAndSourceFilter(t *testing.T) {
	m := New("REINS").FoldIntake(grammar.IntakeSummary{
		Sources: []grammar.IntakeSource{
			{ID: "request_state", Status: "observed", Count: 5, AgeBucket: "<5m", Privacy: "metadata-only", AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "privacy": "ok", "path": "ok"}},
			{ID: "planning_feed", Status: "observed", Count: 3, AgeBucket: "<1h", Privacy: "metadata-only", AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "privacy": "ok", "path": "ok"}},
		},
		Rows: []grammar.IntakeRow{
			{ID: "planning_feed:coverage:untracked", Source: "planning_feed", Kind: "coverage:untracked", Status: "bucket", Severity: "warn", Count: 3, Coverage: "untracked", TaskLinkState: "untracked", Authority: "planning-observation", Evidence: "count:3", Missing: "task linkage", Action: "review", Detail: "coverage=untracked · task_link=untracked", SourceRefs: "planning_feed", NextEvidence: "attach task/claim linkage metadata", AIR: map[string]string{"id": "ok", "source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok", "coverage": "ok", "task_link_state": "ok", "blocker": "ok", "authority": "ok", "evidence": "ok", "missing": "ok", "action": "ok", "detail": "ok", "source_refs": "ok", "next_evidence": "ok"}},
			{Source: "request_state", Kind: "requests:open", Status: "attention", Severity: "major", Count: 5, Coverage: "metadata", TaskLinkState: "task_metadata", Blocker: "workflow_attention", AIR: map[string]string{"source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok", "coverage": "ok", "task_link_state": "ok", "blocker": "ok"}},
			{ID: "p0_incident_state:incident:p0", Source: "p0_incident_state", Kind: "incident:p0", Status: "active", Severity: "crit", Count: 1, Coverage: "coalesced", TaskLinkState: "task_link_metadata", Blocker: "p0_incident", Authority: "incident-observation", Evidence: "count:1", Missing: "triage receipt · blocker resolution", Action: "triage-critical", Detail: "coverage=coalesced · blocker=p0_incident", SourceRefs: "p0_incident_state", NextEvidence: "attach blocker disposition and governed receipt", AIR: map[string]string{"id": "ok", "source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok", "coverage": "ok", "task_link_state": "ok", "blocker": "ok", "authority": "ok", "evidence": "ok", "missing": "ok", "action": "ok", "detail": "ok", "source_refs": "ok", "next_evidence": "ok"}},
		},
		Totals: map[string]int{"request_attention": 5, "planning_attention": 3, "p0_incidents": 1},
	}, false)
	m.Width, m.Height, m.Page = 170, 48, PageIntake

	v := ansi.Strip(m.View())
	for _, want := range []string{
		"filter all sources",
		"SELECTED BUCKET",
		"cursor      1/3",
		"bucket      incident:p0",
		"source      p0_incident_state",
		"id=p0_incident_state:incident:p0",
		"proof       incident-observation",
		"count:1",
		"refs=p0_incident_state",
		"gap         triage receipt",
		"action=triage-critical",
		"next proof  attach blocker disposition",
		"[j/k]bucket",
		"[s/S]source",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("intake bucket projection missing %q:\n%s", want, v)
		}
	}

	m = step(m, "j")
	if m.IFocus != 1 || !strings.Contains(m.Status, "2/3") {
		t.Fatalf("[j] should move the intake bucket cursor to 2/3, got focus=%d status=%q", m.IFocus, m.Status)
	}
	v = ansi.Strip(m.View())
	for _, want := range []string{"cursor      2/3", "bucket      requests:open", "source      request_state"} {
		if !strings.Contains(v, want) {
			t.Fatalf("intake [j] selection missing %q:\n%s", want, v)
		}
	}

	m = step(m, "s")
	if m.IntakeSourceFilter != "request_state" || m.IFocus != 0 {
		t.Fatalf("[s] should filter to request_state and reset cursor, got filter=%q focus=%d", m.IntakeSourceFilter, m.IFocus)
	}
	v = ansi.Strip(m.View())
	for _, want := range []string{"filter request_state", "1/3 buckets", "cursor      1/1", "bucket      requests:open"} {
		if !strings.Contains(v, want) {
			t.Fatalf("intake source filter missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "coverage:untracked") || strings.Contains(v, "incident:p0") {
		t.Fatalf("source filter should hide other-source buckets:\n%s", v)
	}

	m = step(m, "s")
	if m.IntakeSourceFilter != "planning_feed" {
		t.Fatalf("second [s] should filter to planning_feed, got %q", m.IntakeSourceFilter)
	}
	if row, ok := m.FocusedIntakeRow(); !ok || row.Kind != "coverage:untracked" {
		t.Fatalf("planning_feed filter should focus its bucket, got ok=%t row=%+v", ok, row)
	}
	m = step(m, "S")
	if m.IntakeSourceFilter != "request_state" {
		t.Fatalf("[S] should cycle source filter backward, got %q", m.IntakeSourceFilter)
	}
}

func TestIntakeProjectionUsesSemanticRowsInMediumSplitWidth(t *testing.T) {
	m := New("REINS").FoldIntake(grammar.IntakeSummary{
		Rows: []grammar.IntakeRow{
			{Source: "p0_incident_ledger", Kind: "p0_incident_notification", Status: "recent", Severity: "crit", Count: 9, Coverage: "tail_1000", Blocker: "durable_eventlog", AIR: map[string]string{"source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok", "coverage": "ok", "blocker": "ok"}},
			{Source: "request_state", Kind: "request_attention", Status: "attention", Severity: "warn", Count: 4, Coverage: "workflow_attention", Blocker: "workflow_attention_tail", AIR: map[string]string{"source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok", "coverage": "ok", "blocker": "ok"}},
		},
		Totals: map[string]int{"request_attention": 4, "p0_incidents": 9},
	}, false)
	m.Width, m.Height, m.Page = 112, 42, PageIntake

	v := ansi.Strip(m.View())
	for _, want := range []string{
		"OBSERVATION BUCKETS",
		"p0_incident_notification",
		"blocker=durable_eventlog",
		"request_attention",
		"blocker=workflow_attention_tail",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("medium-width intake projection missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "SOURCE                 KIND") {
		t.Fatalf("medium-width intake projection should not use the dense table:\n%s", v)
	}
	for i, line := range strings.Split(v, "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("medium intake line %d exceeds frame width %d: %d %q", i, m.Width, got, line)
		}
	}
}

func TestWideIntakeContextRailAdvertisesBucketCursor(t *testing.T) {
	m := New("REINS").FoldIntake(grammar.IntakeSummary{
		Rows: []grammar.IntakeRow{
			{Source: "p0_incident_ledger", Kind: "p0_incident_notification", Status: "recent", Severity: "crit", Count: 9, Coverage: "tail_1000", Blocker: "durable_eventlog", AIR: map[string]string{"source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok", "coverage": "ok", "blocker": "ok"}},
			{Source: "request_state", Kind: "request_attention", Status: "attention", Severity: "warn", Count: 4, Coverage: "workflow_attention", Blocker: "workflow_attention_tail", AIR: map[string]string{"source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok", "coverage": "ok", "blocker": "ok"}},
		},
		Totals: map[string]int{"request_attention": 4, "p0_incidents": 9},
	}, false)
	m.Width, m.Height, m.Page = 220, 52, PageIntake

	v := ansi.Strip(m.View())
	for _, want := range []string{"legal next", "[j/k]", "bucket 1/2", "[s/S]", "source filter", "[Enter]", "aggregate detail"} {
		if !strings.Contains(v, want) {
			t.Fatalf("wide intake rail/footer missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "[j/k]       scroll") {
		t.Fatalf("wide intake rail must advertise bucket cursor, not generic reference scroll:\n%s", v)
	}
}

func TestIntakeCursorJumpsToEpistemicObservationRow(t *testing.T) {
	m := New("REINS").FoldIntake(grammar.IntakeSummary{
		Rows: []grammar.IntakeRow{{
			ID: "planning_feed:coverage:untracked", Source: "planning_feed", Kind: "coverage:untracked", Status: "bucket", Severity: "warn", Count: 3,
			Authority: "planning-observation", Evidence: "count:3", Missing: "task linkage", Action: "review", Detail: "coverage=untracked", SourceRefs: "planning_feed", NextEvidence: "attach task/claim linkage metadata",
			AIR: map[string]string{"id": "ok", "source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok", "authority": "ok", "evidence": "ok", "missing": "ok", "action": "ok", "detail": "ok", "source_refs": "ok", "next_evidence": "ok"},
		}},
	}, false)
	m.Width, m.Height, m.Page = 150, 48, PageIntake

	m = step(m, "E")
	row, ok := m.FocusedEpistemicRow()
	if m.Page != PageEpistemics || !ok || row.Subject != "planning_feed:coverage:untracked" {
		t.Fatalf("E from intake should land on matching epistemic observation row, page=%d ok=%v row=%+v status=%q", m.Page, ok, row, m.Status)
	}
	if row.Authority != "planning-observation" || row.Evidence != "count:3" || !strings.Contains(row.Detail, "task linkage") {
		t.Fatalf("epistemic intake row should carry observation contract, row=%+v", row)
	}
}

func TestIntakeDoorOpensAndRedactsAggregateMetadata(t *testing.T) {
	const (
		secretPath     = "/secret/intake/request-state.json"
		secretKind     = "SECRET-OBSERVATION-KIND"
		secretCoverage = "SECRET-COVERAGE"
	)
	intake := grammar.IntakeSummary{
		Sources: []grammar.IntakeSource{{
			ID: "request_state", Path: secretPath, Exists: true, Status: "observed", Count: 4, AgeBucket: "<5m", Privacy: "metadata-only",
			AIR: map[string]string{"id": "ok", "path": "deny", "exists": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "privacy": "ok"},
		}},
		Rows: []grammar.IntakeRow{{
			Source: "request_state", Kind: secretKind, Status: "attention", Severity: "warn", Count: 4, Coverage: secretCoverage, Blocker: "workflow_attention", AgeBucket: "<5m",
			AIR: map[string]string{"source": "ok", "kind": "deny", "status": "ok", "severity": "ok", "count": "ok", "coverage": "deny", "blocker": "ok", "age_bucket": "ok"},
		}},
		Totals: map[string]int{"request_attention": 4},
	}

	m := New("REINS").FoldIntake(intake, false)
	m.Width, m.Height, m.Page = 120, 36, PageIntake
	m = step(m, "enter")
	if !m.IntakeDoorOpen {
		t.Fatal("[enter] on :intake should open the aggregate detail door")
	}
	local := ansi.Strip(m.View())
	for _, want := range []string{"DOOR /intake", "SOURCE POSTURE", "TOP BUCKETS", "raw-body", "request_state", secretPath, secretKind} {
		if !strings.Contains(local, want) {
			t.Fatalf("local intake door missing %q:\n%s", want, local)
		}
	}
	m = step(m, "enter")
	if m.IntakeDoorOpen {
		t.Fatal("[enter] should close the /intake door")
	}

	m.AIR = true
	m = step(m, "enter")
	air := ansi.Strip(m.View())
	for _, leak := range []string{secretPath, secretKind, secretCoverage} {
		if strings.Contains(air, leak) {
			t.Fatalf("AIR intake door leaked denied value %q:\n%s", leak, air)
		}
	}
	if !strings.Contains(air, "▒▒▒") || !strings.Contains(air, "count 4") {
		t.Fatalf("AIR intake door should preserve structure/count while redacting denied values:\n%s", air)
	}
}

func TestSplitContextYankUsesSessionSource(t *testing.T) {
	m := New("REINS").FoldSessions([]grammar.Session{
		{Role: "cx-a", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.70,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "ok", "session": "ok", "output_age_s": "ok", "relay_age_s": "ok"}},
		{Role: "cx-b", Platform: "claude", State: "active", Readiness: "stale", Blocker: "none", Attention: 0.55,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "ok", "session": "ok", "output_age_s": "ok", "relay_age_s": "ok"}},
	}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 220, 44, PageCaps, true

	m = step(m, "y")
	if m.Mode != ModeYank || m.Sel.Field != "role" {
		t.Fatalf("split yank should open session field picker, mode=%d field=%q", m.Mode, m.Sel.Field)
	}
	m = step(m, "j")
	if m.SFocus != 1 || m.RefScroll != 0 {
		t.Fatalf("split yank j should move session source only, SFocus=%d RefScroll=%d", m.SFocus, m.RefScroll)
	}
	m = step(m, "p")
	if len(m.Ring) == 0 || m.Ring[0].Page != "sessions" || m.Ring[0].Field != "platform" || m.Ring[0].Value != "claude" {
		t.Fatalf("split yank should grab the focused session platform, ring=%+v", m.Ring)
	}
	if m.Mode != ModeCommand || m.Input != "claude" {
		t.Fatalf("split yank should inject the session field into command mode, mode=%d input=%q", m.Mode, m.Input)
	}
}

func TestSplitYankFloorDoesNotAdvertiseArrowWindowOrContext(t *testing.T) {
	m := New("REINS").FoldSessions([]grammar.Session{{
		Role: "cx-a", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.70,
		AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "ok", "session": "ok", "output_age_s": "ok", "relay_age_s": "ok"},
	}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 220, 44, PageCaps, true
	m = step(m, "y")
	if m.Mode != ModeYank {
		t.Fatalf("expected yank mode, got %d", m.Mode)
	}
	floor := ansi.Strip(m.viewFloor(m.Width))
	for _, bad := range []string{"[←/→]win", "[←/→]ctx"} {
		if strings.Contains(floor, bad) {
			t.Fatalf("split yank floor must not advertise %s while arrows move fields:\n%s", bad, floor)
		}
	}
	for _, want := range []string{"[[/]]win", "[Tab/←/→] fields"} {
		if !strings.Contains(floor, want) {
			t.Fatalf("split yank floor missing %q:\n%s", want, floor)
		}
	}
}

func TestTaskOnlyControlsReportOffTasks(t *testing.T) {
	m := New("REINS").Fold(evFixture(), false)
	m.Width, m.Height = 120, 40
	m.Page = PageEvents
	m = step(m, "/")
	if m.Mode == ModeFilter || !strings.Contains(m.Status, ":tasks only") {
		t.Fatalf("/ off :tasks should report the page restriction: mode=%d status=%q", m.Mode, m.Status)
	}
	m = step(m, "f")
	if m.Mode == ModeHint || !strings.Contains(m.Status, ":tasks only") {
		t.Fatalf("f off :tasks should report the page restriction: mode=%d status=%q", m.Mode, m.Status)
	}
	m.Page = PageHelp
	m = step(m, "y")
	if !strings.Contains(m.Status, "no selectable rows") {
		t.Fatalf("y on reference pages should report no selectable rows: %q", m.Status)
	}
	m = step(m, "enter")
	if !strings.Contains(m.Status, ":tasks only") {
		t.Fatalf("enter on reference pages should report task-only inspect: %q", m.Status)
	}
}

func TestReferencePageNavigationPreservesTaskFocus(t *testing.T) {
	m := New("REINS").FoldTasks([]grammar.Task{
		{TaskID: "t0", AIR: map[string]string{"task_id": "ok"}},
		{TaskID: "t1", AIR: map[string]string{"task_id": "ok"}},
		{TaskID: "t2", AIR: map[string]string{"task_id": "ok"}},
	}, false)
	m.Width, m.Height = 80, 12
	m.Page, m.Focus = PageTasks, 2
	m = step(m, "4")
	m = step(m, "j")
	m = step(m, "k")
	m = step(m, "2")
	if m.Focus != 2 {
		t.Fatalf("reference-page scrolling must not mutate hidden task focus, got %d", m.Focus)
	}
}

func TestTaskRefreshClampsFocusAndClearsClassSelection(t *testing.T) {
	m := New("REINS").FoldTasks([]grammar.Task{
		{TaskID: "t0", Criticality: "crit", AIR: map[string]string{"task_id": "ok"}},
		{TaskID: "t1", Criticality: "crit", AIR: map[string]string{"task_id": "ok"}},
		{TaskID: "t2", Criticality: "crit", AIR: map[string]string{"task_id": "ok"}},
	}, false)
	m.Page, m.Focus, m.Sel.Members = PageTasks, 2, []int{0, 1, 2}
	m = m.FoldTasks([]grammar.Task{{TaskID: "only", AIR: map[string]string{"task_id": "ok"}}}, false)
	if m.Focus != 0 || len(m.Sel.Members) != 0 {
		t.Fatalf("task refresh should clamp focus and clear stale class selection: focus=%d members=%v", m.Focus, m.Sel.Members)
	}
}

func TestClassSelectionClearedWhenFilterChanges(t *testing.T) {
	m := New("REINS").FoldTasks([]grammar.Task{
		{TaskID: "crit-a", Criticality: "crit", AIR: map[string]string{"task_id": "ok"}},
		{TaskID: "crit-b", Criticality: "crit", AIR: map[string]string{"task_id": "ok"}},
		{TaskID: "other", Criticality: "ok", AIR: map[string]string{"task_id": "ok"}},
	}, false)
	m.Width, m.Height, m.Page = 120, 40, PageTasks
	m = step(m, "V")
	if len(m.Sel.Members) == 0 {
		t.Fatal("setup should class-select visible crit rows")
	}
	m = step(m, "/")
	m = step(m, "o")
	m = ent(m)
	m = step(m, "y")
	if len(m.Ring) != 0 || m.Mode != ModeYank {
		t.Fatalf("filter changes should clear class selection before yank: mode=%d ring=%v members=%v", m.Mode, m.Ring, m.Sel.Members)
	}
}

func TestActJumpStatusRedactsDeniedTaskIDOnAir(t *testing.T) {
	m := New("REINS").FoldTasks([]grammar.Task{
		{TaskID: "SECRET-BLOCKER", PredictedStage: "hold", Criticality: "crit", AIR: map[string]string{"task_id": "deny"}},
	}, false)
	m.Width, m.Height, m.Page, m.AIR = 120, 40, PageTasks, true
	m = step(m, "f")
	m = step(m, "1")
	if strings.Contains(m.Status, "SECRET-BLOCKER") || !strings.Contains(m.Status, "▒▒▒") {
		t.Fatalf("Act-jump status must be AIR-safe, got %q", m.Status)
	}
}

func TestFilterNarrowsSelectableSet(t *testing.T) {
	tasks := []grammar.Task{
		{TaskID: "reform-a", AIR: map[string]string{}}, {TaskID: "hkp-b", AIR: map[string]string{}},
		{TaskID: "reform-c", AIR: map[string]string{}},
	}
	m := New("REINS").FoldTasks(tasks, false)
	m.Page = PageTasks
	m = step(m, "/")
	if m.Mode != ModeFilter {
		t.Fatal("/ should enter filter mode")
	}
	m.Filter = "reform"
	if vt := m.visibleTasks(); len(vt) != 2 || vt[0].TaskID != "reform-a" {
		t.Fatalf("filter should narrow to the 2 reform tasks: %d", len(vt))
	}
	if ft, ok := m.FocusedTask(); !ok || ft.TaskID != "reform-a" {
		t.Fatalf("focus should index the FILTERED set: %v", ft)
	}
}

func TestHintTeleport(t *testing.T) {
	tasks := []grammar.Task{
		{TaskID: "t0", AIR: map[string]string{}}, {TaskID: "t1", AIR: map[string]string{}},
		{TaskID: "t2", AIR: map[string]string{}}, {TaskID: "t3", AIR: map[string]string{}},
	}
	m := New("REINS").FoldTasks(tasks, false)
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	m = step(m, "f")
	if m.Mode != ModeHint {
		t.Fatal("f should enter hint mode")
	}
	m = step(m, "d") // hintAlphabet = "asdf…" → 'd' is index 2
	if m.Mode != ModeNormal || m.Focus != 2 {
		t.Fatalf("typing 'd' should teleport to row 2: mode=%d focus=%d", m.Mode, m.Focus)
	}
}

func TestNavigableCompletion(t *testing.T) {
	m := New("REINS").FoldTasks([]grammar.Task{{TaskID: "x", AIR: map[string]string{}}}, false)
	m.Page = PageTasks
	m = step(m, ":") // enter the command line
	if m.Mode != ModeCommand {
		t.Fatal(": should open the command line")
	}
	if comps := m.completions(); len(comps) < 2 {
		t.Fatalf("expected a navigable candidate list: %v", comps)
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab}) // navigate down the list
	m = nm.(Model)
	if m.CompIdx != 1 {
		t.Fatalf("Tab should advance the highlighted candidate to 1, got %d", m.CompIdx)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // accept the highlighted candidate
	m = nm.(Model)
	if m.Mode != ModeNormal {
		t.Fatal("Enter should accept the highlighted candidate + leave the command line")
	}
}

func TestWhichKeyMenu(t *testing.T) {
	if mv := matchVerbs("d"); len(mv) != 2 || mv[0].name != "dynamics" || mv[1].name != "domains" {
		t.Fatalf("'d' should match dynamics and domains, got %v", mv)
	}
	if mv := matchVerbs("dy"); len(mv) != 1 || mv[0].name != "dynamics" {
		t.Fatalf("'dy' should narrow to dynamics, got %v", mv)
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

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

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
		{TS: "14:22", Kind: "task.updated", Subject: "4284", Summary: "merged", Score: 0.7,
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

func TestExecCommandRegistryAndIntentStubs(t *testing.T) {
	m := New("REINS")
	if out := m.Exec("cmds"); !strings.Contains(out.Status, "commands:") || !strings.Contains(out.Status, "intent 1") {
		t.Fatalf("command registry summary should be reachable by alias, got %q", out.Status)
	}
	if out := m.Exec("cmds"); out.Page != PageCommands || !strings.Contains(out.Status, "governed 1") {
		t.Fatalf("command registry summary should expose governed-command count, got %q", out.Status)
	}

	out := m.Exec("intent dispatch")
	if out.Page != PageIntent || out.Quitting || out.Mode != ModeNormal {
		t.Fatalf("intent preview must open review page without emitting an effect: after=%+v", out)
	}
	if !strings.Contains(out.Status, "intent dispatch") || !strings.Contains(out.Status, "governed COMMAND route required") || !strings.Contains(out.Status, "no effect emitted") {
		t.Fatalf("intent preview should report governed stub semantics, got %q", out.Status)
	}
	if !strings.Contains(out.Status, "preflight target + authority") || !strings.Contains(out.Status, "receipt preview only") {
		t.Fatalf("intent preview should report preflight and receipt metadata, got %q", out.Status)
	}

	surfaces := m.Exec("surf")
	if surfaces.Page != PageSurfaces || !strings.Contains(surfaces.Status, "surfaces:") || !strings.Contains(surfaces.Status, "instance 29") {
		t.Fatalf("surface registry should be reachable by alias, got page=%d status=%q", surfaces.Page, surfaces.Status)
	}

	domains := m.Exec("domains")
	if domains.Page != PageDomains || !strings.Contains(domains.Status, "domains:") || !strings.Contains(domains.Status, "terrain") {
		t.Fatalf("domain registry should be reachable by canonical command, got page=%d status=%q", domains.Page, domains.Status)
	}
	terrain := m.Exec("terrain")
	if terrain.Page != PageDomains || !strings.Contains(terrain.Status, "domains:") {
		t.Fatalf("domain registry should keep terrain as an alias, got page=%d status=%q", terrain.Page, terrain.Status)
	}

	for _, alias := range []string{"lifecycles", "life", "lifecycle", "ndlc", "n-dlc"} {
		out := m.Exec(alias)
		if out.Page != PageLifecycles || !strings.Contains(out.Status, "lifecycles:") || !strings.Contains(out.Status, "tenant-extensible") {
			t.Fatalf("lifecycle alias %q should open lifecycle registry, got page=%d status=%q", alias, out.Page, out.Status)
		}
	}

	yard := m.Exec("yard")
	if yard.Page != PageYard || !strings.Contains(yard.Status, ":yard") {
		t.Fatalf("yard cockpit should be reachable by command, got page=%d status=%q", yard.Page, yard.Status)
	}

	ready := m.Exec("gates")
	if ready.Page != PageReadiness || !strings.Contains(ready.Status, ":readiness") {
		t.Fatalf("readiness/gates projection should be reachable by alias, got page=%d status=%q", ready.Page, ready.Status)
	}

	intake := m.Exec("obs")
	if intake.Page != PageIntake || !strings.Contains(intake.Status, ":intake") {
		t.Fatalf("intake observation projection should be reachable by alias, got page=%d status=%q", intake.Page, intake.Status)
	}

	caps := m.Exec("cap")
	if caps.Page != PageCaps || !strings.Contains(caps.Status, ":capabilities") {
		t.Fatalf("capability projection should be reachable by alias, got page=%d status=%q", caps.Page, caps.Status)
	}

	epi := m.Exec("epi")
	if epi.Page != PageEpistemics || !strings.Contains(epi.Status, ":epistemics") {
		t.Fatalf("epistemics projection should be reachable by alias, got page=%d status=%q", epi.Page, epi.Status)
	}
}

func TestIntentPreviewIncludesAIRSafeSelectedSubject(t *testing.T) {
	m := New("REINS").FoldTasks([]grammar.Task{{
		TaskID: "SECRET-TASK", AIR: map[string]string{"task_id": "deny"},
	}}, false)
	m.Page, m.AIR = PageTasks, true
	out := m.Exec("intent dispatch")
	if strings.Contains(out.Status, "SECRET-TASK") || !strings.Contains(out.Status, "selection task ▒▒▒") {
		t.Fatalf("intent preview must include AIR-safe selected task, got %q", out.Status)
	}

	m = New("REINS").FoldSessions([]grammar.Session{{
		Role: "cx-p0", AIR: map[string]string{"role": "ok"},
	}}, false)
	m.Page = PageSessions
	out = m.Exec("intent resume")
	if !strings.Contains(out.Status, "selection session cx-p0") {
		t.Fatalf("intent preview must include selected session role, got %q", out.Status)
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
	m.Width, m.Height = 160, 80
	v := ansi.Strip(m.View())
	if !strings.Contains(v, ":help") || !strings.Contains(v, ":yard") || !strings.Contains(v, ":readiness") || !strings.Contains(v, ":intake") || !strings.Contains(v, ":capabilities") || !strings.Contains(v, ":dynamics") || !strings.Contains(v, ":commands") || !strings.Contains(v, ":intent") || !strings.Contains(v, ":surfaces") || !strings.Contains(v, ":domains") || !strings.Contains(v, ":lifecycles") || !strings.Contains(v, "[a]AIR") || !strings.Contains(v, "SPLIT PAIRS") || !strings.Contains(v, "source-only") || !strings.Contains(v, "title +N = hidden windows") {
		t.Fatalf("help page must list pages + keys: %q", v)
	}
}

func TestCommandAndWindowCatalogPagesRender(t *testing.T) {
	m := New("REINS")
	m.Width, m.Height = 260, 40

	cmds := m.Exec("commands")
	if cmds.Page != PageCommands {
		t.Fatalf(":commands must open command catalog page, got %d", cmds.Page)
	}
	cmdView := ansi.Strip(cmds.View())
	for _, want := range []string{"COMMANDS", "verb=intent", "governed COMMAND route required", "preflight=target +", "switch window"} {
		if !strings.Contains(cmdView, want) {
			t.Fatalf("commands page missing %q:\n%s", want, cmdView)
		}
	}
	if strings.Contains(cmdView, "VERB             KIND/GROUP") {
		t.Fatalf("commands page should not use dense table inside the wide reference split:\n%s", cmdView)
	}
	cmds = step(cmds, "j")
	if cmds.CommandFocus != 1 || !strings.Contains(cmds.Status, "command 2/") {
		t.Fatalf(":commands j should move command focus, focus=%d status=%q", cmds.CommandFocus, cmds.Status)
	}
	cmdView = ansi.Strip(cmds.View())
	for _, want := range []string{"tasks (t)", "read/window", "selected    tasks (t)", "[j/k]command 2/22"} {
		if !strings.Contains(cmdView, want) {
			t.Fatalf("commands selected-row context missing %q:\n%s", want, cmdView)
		}
	}

	templateCmds := New("REINS").FoldSessions([]grammar.Session{{
		Role: "cx-template", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88,
		AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"},
	}}, false)
	templateCmds.Width, templateCmds.Height, templateCmds.Page, templateCmds.SplitContext = 220, 96, PageCommands, true
	templateView := ansi.Strip(templateCmds.View())
	for _, want := range []string{
		"TEMPLATE INJECTION",
		"selection source PageSessions",
		"{{focus}}       focused row identity -> cx-template",
		"{{sel.field}}   selected field name -> role",
		"{{sel.value}}   selected field value through AIR -> cx-template",
		"{{ring.0}}      unavailable for PageSessions",
	} {
		if !strings.Contains(templateView, want) {
			t.Fatalf("commands page live template browser missing %q:\n%s", want, templateView)
		}
	}

	wins := m.Exec("windows")
	if wins.Page != PageWindows {
		t.Fatalf(":windows must open window registry page, got %d", wins.Page)
	}
	wins = step(wins, "j")
	if wins.WindowFocus != 1 || !strings.Contains(wins.Status, "window 2/") {
		t.Fatalf(":windows j should move window focus, focus=%d status=%q", wins.WindowFocus, wins.Status)
	}
	winView := ansi.Strip(wins.View())
	for _, want := range []string{"WINDOWS", "sessions", "yard", "readiness", "intake", "triage", "routing", "matrix", "sdlc", "cockpit", "gate", "epistemics", "commands", "surfaces", "domains", "lifecycles", "PageYard", "PageReadiness", "PageIntake", "PageCaps", "PageEpistemics", "PageCommands", "PageIntent", "PageSurfaces", "PageDomains", "PageLifecycles", "SPLIT", "link:compact role", "link:target actor", "link:scroll role", "anchor:target system", "anchor:target evidence", "anchor:ref tenant", "source-only", "selected    tasks", "[j/k]window 2/19", "jump        [2] / :tasks"} {
		if !strings.Contains(winView, want) {
			t.Fatalf("windows page missing %q:\n%s", want, winView)
		}
	}
	if strings.Contains(winView, "KEY WINDOW") {
		t.Fatalf("windows page should not use dense table inside the wide reference split:\n%s", winView)
	}

	surfaces := m.Exec("surfaces")
	if surfaces.Page != PageSurfaces {
		t.Fatalf(":surfaces must open surface registry page, got %d", surfaces.Page)
	}
	surfaces.Height = 120
	surfView := ansi.Strip(surfaces.View())
	for _, want := range []string{"SURFACES", "/whois", "/session", "yank", "filter", "field-rank", "class-select", "dyn-scale", "intent-target", "wide-context", "split-context", "gate-stack", "trainyard-cockpit", "dynamics-map", "labrack-lens", "section-figure-lens", "tool-capability-inventory", "source-acquisition-router", "verifier-floor", "provider-gateway", "publication-egress", "audio-avsdlc", "infra-control", "signal-pips", "terrain-depth", "severity-freshness", "observation-feed", "observation-detail", "selection-template", "air-lens", "read-dark", "command line"} {
		if !strings.Contains(surfView, want) {
			t.Fatalf("surfaces page missing %q:\n%s", want, surfView)
		}
	}

	domains := m.Exec("domains")
	if domains.Page != PageDomains {
		t.Fatalf(":domains must open domain registry page, got %d", domains.Page)
	}
	domains.Height = 120
	domView := ansi.Strip(domains.View())
	for _, want := range []string{"DOMAINS", "sdlc-work", "source-acquisit", "verifier-floor", "provider-gateway", "publication-media", "infrastructure-control", "governance-readine", "research-rdlc", "future-n-dlc", "FUNDAMENTALS", "ontology", "forms", "motion", "Trainyard"} {
		if !strings.Contains(domView, want) {
			t.Fatalf("domains page missing %q:\n%s", want, domView)
		}
	}

	life := m.Exec("lifecycles")
	if life.Page != PageLifecycles {
		t.Fatalf(":lifecycles must open lifecycle registry page, got %d", life.Page)
	}
	life.Height = 120
	lifeView := ansi.Strip(life.View())
	for _, want := range []string{"LIFECYCLES", "SDLC/RDLC/LDLC/n-DLC", "SOURCE STATUS", "COMPILED FALLBACK", "EXTENSIBILITY", "tenant-safe", "LDLC", "n-DLC", "read-only projection"} {
		if !strings.Contains(lifeView, want) {
			t.Fatalf("lifecycles page missing %q:\n%s", want, lifeView)
		}
	}

	yard := m.Exec("yard")
	if yard.Page != PageYard {
		t.Fatalf(":yard must open yard cockpit page, got %d", yard.Page)
	}
	yardView := ansi.Strip(yard.View())
	for _, want := range []string{"YARD", "read-only projection", "LADDER", "ATTENTION RAIL", "FLEET MATRIX", "GATES / READINESS", "REPRESENTATION"} {
		if !strings.Contains(yardView, want) {
			t.Fatalf("yard page missing %q:\n%s", want, yardView)
		}
	}

	readiness := m.Exec("readiness")
	if readiness.Page != PageReadiness {
		t.Fatalf(":readiness must open readiness page, got %d", readiness.Page)
	}
	readyView := ansi.Strip(readiness.View())
	for _, want := range []string{"READINESS", "SOURCE FRESHNESS", "TASK GATES", "LANE READINESS", "COMMAND ROUTE", "GAPS / NEXT CONTRACT", "read-only projection"} {
		if !strings.Contains(readyView, want) {
			t.Fatalf("readiness page missing %q:\n%s", want, readyView)
		}
	}

	intake := m.Exec("intake")
	if intake.Page != PageIntake {
		t.Fatalf(":intake must open intake observation page, got %d", intake.Page)
	}
	intakeView := ansi.Strip(intake.View())
	for _, want := range []string{"INTAKE", "read-only projection", "SOURCE FRESHNESS", "DEMAND TOTALS", "OBSERVATION BUCKETS", "GAPS / NEXT CONTRACT"} {
		if !strings.Contains(intakeView, want) {
			t.Fatalf("intake page missing %q:\n%s", want, intakeView)
		}
	}

	m.Width, m.Height = 180, 80
	caps := m.Exec("capabilities")
	if caps.Page != PageCaps {
		t.Fatalf(":capabilities must open capability projection page, got %d", caps.Page)
	}
	capView := ansi.Strip(caps.View())
	for _, want := range []string{"CAPABILITIES", "FABRIC SCOPE", "CAPABILITY STATUS", "HKP SUPPORT CONTEXT", "PLATFORM EVIDENCE", "ROUTE ADMISSION", "SESSION FIT RAIL", "spend", "fugu/fugu-ultra", "read-only projection"} {
		if !strings.Contains(capView, want) {
			t.Fatalf("capabilities page missing %q:\n%s", want, capView)
		}
	}
}

func TestReferenceCatalogsUseSemanticRowsAtMediumWidth(t *testing.T) {
	for _, tc := range []struct {
		name        string
		page        int
		missingHead []string
		want        []string
	}{
		{
			name:        "commands",
			page:        PageCommands,
			missingHead: []string{"VERB             KIND/GROUP", "PREFLIGHT                    RECEIPT"},
			want: []string{
				"verb=intent",
				"auth=governed COMMAND route required",
				"preflight=target +",
				"authority + mutation surface + receipt contract",
				"receipt=preview only",
				"TEMPLATE INJECTION",
				"{{sel.field}}",
				"COMMAND RAIL",
			},
		},
		{
			name:        "windows",
			page:        PageWindows,
			missingHead: []string{"KEY WINDOW", "SPLIT           PAGE"},
			want: []string{
				"window=capabilities",
				"lifecycle=routing",
				"split=link:scroll role/platform",
				"page=PageCaps",
			},
		},
		{
			name:        "surfaces",
			page:        PageSurfaces,
			missingHead: []string{"SURFACE            NAME", "AIR             CONTRACT"},
			want: []string{
				"surface=split-context",
				"open=[|] wide terminals",
				"contract=all splits: [j/k] source,",
				"[J/K]",
				"context; Enter/y act on source",
				"Enter/y act on source",
			},
		},
		{
			name:        "domains",
			page:        PageDomains,
			missingHead: []string{"DOMAIN              TERRAIN", "SURFACES                   PARITY"},
			want: []string{
				"domain=research-rdlc",
				"windows=domains,dynamics",
				"parity=labrack,section-figure,research corpus",
				"domain=future-n-dlc",
			},
		},
		{
			name:        "lifecycles",
			page:        PageLifecycles,
			missingHead: []string{"LIFECYCLE          STATE"},
			want: []string{
				"LIFECYCLES",
				"SOURCE STATUS",
				"COMPILED FALLBACK",
				"EXTENSIBILITY",
				"tenant-safe",
				"LDLC",
				"n-DLC",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New("REINS")
			m.Width, m.Height, m.Page = 112, 160, tc.page
			v := ansi.Strip(m.View())
			for _, head := range tc.missingHead {
				if strings.Contains(v, head) {
					t.Fatalf("medium %s catalog should not use dense header %q:\n%s", tc.name, head, v)
				}
			}
			for _, want := range tc.want {
				if !strings.Contains(v, want) {
					t.Fatalf("medium %s catalog missing %q:\n%s", tc.name, want, v)
				}
			}
			for i, line := range strings.Split(v, "\n") {
				if got := ansi.StringWidth(line); got > m.Width {
					t.Fatalf("medium %s line %d exceeds frame width %d: %d %q", tc.name, i, m.Width, got, line)
				}
			}
		})
	}
}

func TestSegmentedFactRowsPreserveLongFacts(t *testing.T) {
	prefix := " " + grammar.C("pri", "row")
	facts := []string{
		"identity=release-authority-route-envelope-ledger-missing-before-ship-cutover",
		"missing=authority_case,parent_spec,preflight,receipt",
		"evidence=codex.headless.full route-policy-only with governed receipt pending",
	}
	lines := wrapSegmentedFactSegments(prefix, facts, 64)
	v := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{
		"release-authority-route-envelope-ledger-missing-",
		"before-ship-cutover",
		"missing=authority_case,parent_spec,preflight,receipt",
		"codex.headless.full",
		"governed receipt pending",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("segmented fact row lost %q:\n%s", want, v)
		}
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got > 64 {
			t.Fatalf("segmented fact row line %d exceeds width: %d %q", i, got, ansi.Strip(line))
		}
	}
}

func TestCapabilityProjectionRendersObservedLaneFit(t *testing.T) {
	m := New("REINS").
		Fold(evFixture(), false).
		FoldTasks([]grammar.Task{{TaskID: "task-a", AIR: map[string]string{"task_id": "ok"}}}, false).
		FoldSessions([]grammar.Session{
			{Role: "cx-ready", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88,
				AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"}},
			{Role: "theta", Platform: "claude", State: "active", Readiness: "stale", Blocker: "stale_relay", Attention: 0.54,
				AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"}},
		}, false)
	m.Width, m.Height, m.Page = 180, 120, PageCaps

	v := ansi.Strip(m.View())
	for _, want := range []string{"CAPABILITIES", "CAPABILITY STATUS", "route envelope", "request ha", "HKP SUPPORT CONTEXT", "codex", "claude", "resume-preview", "verify-first", "ROUTE ADMISSION", "preview", "support-only", "learning e"} {
		if !strings.Contains(v, want) {
			t.Fatalf("capabilities projection missing %q:\n%s", want, v)
		}
	}
}

func TestCapabilityRegistryScoresStayOutOfSurfaceStatus(t *testing.T) {
	if got := capabilityStatusSourceGroup(capabilityStatusRow{
		Name:   "grounding",
		Class:  "registry_score",
		Family: "platform_capability_registry",
	}); got != "score" {
		t.Fatalf("registry score capability must group as score, got %q", got)
	}
	if got := capabilityStatusSourceGroup(capabilityStatusRow{
		Name:   "tavily_source_acquisition",
		Class:  "source_acquisition",
		Family: "tavily",
	}); got != "surface" {
		t.Fatalf("concrete tool/provider capability must remain surface, got %q", got)
	}
}

func TestCapabilityProjectionUsesSourceBackedCapabilityRows(t *testing.T) {
	caps := grammar.CapabilitySummary{
		Sources: []grammar.CapabilitySource{{
			ID: "platform_registry", Status: "observed", Count: 13, AgeBucket: "<1d", Detail: "typed platform capability registry",
			AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "detail": "ok"},
		}},
		Rows: []grammar.CapabilityRow{
			{CapabilityID: "route_envelope", Status: "observed", Authority: "metadata-only", RouteCount: 13, OKCount: 11, BlockedCount: 2, EvidenceCount: 13,
				AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "blocker": "ok", "hkp_posture": "ok"}},
			{CapabilityID: "grounding", Status: "observed", Authority: "registry evidence", RouteCount: 13, OKCount: 13, BlockedCount: 0, EvidenceCount: 24,
				AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "blocker": "ok", "hkp_posture": "ok"}},
			{CapabilityID: "hkp_support_context", Status: "support-only", Authority: "authority-capped", Blocker: "HKP may inform context only; promotion requires cited-source verification", HKPPosture: "support_only",
				AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "blocker": "ok", "hkp_posture": "ok"}},
			{CapabilityID: "glm_coding_plan_tool_surface", Status: "manual-bakeoff", Authority: "subscription coding plan", CapabilityClass: "subscription_tool_surface", SurfaceFamily: "manual_coding_plan", SpendModel: "subscription", EgressClass: "tool_runtime", ReceiptRequirement: "manual bakeoff/admission receipt", RouteCount: 1, BlockedCount: 1, EvidenceCount: 4, Blocker: "S15 proves invocation, not quality promotion; workhorse remains manual-only pending bakeoff",
				AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "capability_class": "ok", "surface_family": "ok", "spend_model": "ok", "egress_class": "ok", "receipt_requirement": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "blocker": "ok", "hkp_posture": "ok"}},
			{CapabilityID: "fugu_raw_codex", Status: "raw-manual", Authority: "not dispatchable", CapabilityClass: "subscription_tool_surface", SurfaceFamily: "sakana_fugu", SpendModel: "subscription", EgressClass: "coding_cli_session", ReceiptRequirement: "governed route + admission + bakeoff receipt", BlockedCount: 1, EvidenceCount: 4, Blocker: "raw Sakana/Fugu access is actor evidence only; no governed Fugu route, admission receipt, bakeoff, or dispatch integration exists",
				AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "capability_class": "ok", "surface_family": "ok", "spend_model": "ok", "egress_class": "ok", "receipt_requirement": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "blocker": "ok", "hkp_posture": "ok"}},
		},
		Routes: []grammar.CapabilityRoute{{
			RouteID: "codex.headless.full", Platform: "codex", Mode: "headless", Profile: "full", ModelID: "gpt-test", Effort: "high", ContextMode: "large", FastMode: "disabled", Quantization: "none", CapacityPool: "priority",
			DemandVector: "implementation", Hardening: "standard", EvalPlane: "deterministic", ReviewObligation: "review-team", LearningEligibility: "independent-gate", BenchmarkCoverage: "registry", FixedOverhead: "session-bootstrap",
			RouteState: "active", AuthorityCeiling: "authoritative", FreshnessOK: true, QuotaState: "observed", ReceiptCount: 4, EvidenceCount: 4,
			AIR: map[string]string{"route_id": "ok", "platform": "ok", "mode": "ok", "profile": "ok", "model_id": "ok", "effort": "ok", "context_mode": "ok", "fast_mode": "ok", "quantization": "ok", "capacity_pool": "ok", "demand_vector": "ok", "hardening": "ok", "eval_plane": "ok", "review_obligation": "ok", "learning_eligibility": "ok", "benchmark_coverage": "ok", "fixed_overhead": "ok", "route_state": "ok", "authority_ceiling": "ok", "freshness_ok": "ok", "quota_state": "ok", "receipt_count": "ok", "evidence_count": "ok"},
		}},
		Tools: []grammar.CapabilityTool{{
			RouteID: "codex.headless.full", Platform: "codex", ToolID: "filesystem", Status: "observed", Available: true, AuthorityUse: "read,write", ObservedAt: "2026-06-25T00:00:00Z",
			AIR: map[string]string{"route_id": "ok", "platform": "ok", "tool_id": "ok", "status": "ok", "available": "ok", "authority_use": "ok", "observed_at": "ok"},
		}},
		Totals: map[string]int{"capabilities": 3, "routes": 1, "tools": 1},
	}
	m := New("REINS").FoldCapabilities(caps, false)
	m.Width, m.Height, m.Page = 160, 100, PageCaps

	v := ansi.Strip(m.View())
	for _, want := range []string{
		"CAPABILITY SOURCES",
		"platform_r",
		"typed platform capability registry",
		"CAPABILITY STATUS",
		"route_enve",
		"grounding",
		"glm_coding_plan_tool",
		"S15 proves invocation",
		"fugu_raw_codex",
		"raw-manual",
		"not dispatchable",
		"no governed Fugu route",
		"routes:13 ok:13",
		"blocked:0 evidence:24",
		"HKP SUPPORT CONTEXT",
		"support_only",
		"ROUTE EVIDENCE",
		"codex.headless.full",
		"authoritative",
		"desc",
		"model=gpt-test",
		"effort=high",
		"context=large",
		"pool=priority",
		"gov",
		"demand=implementation",
		"hardening=standard",
		"review=review-team",
		"learning=independent-gate",
		"ROUTE TOOL EVIDENCE",
		"filesystem",
		"read,write",
		"capability registry and receipts are visible",
		"exact session tool status needs route_id",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("source-backed capability projection missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "capability descriptors are not yet registered") {
		t.Fatalf("source-backed capability projection should not show the old descriptor-missing guardrail:\n%s", v)
	}
	if strings.Contains(v, "missing     none") {
		t.Fatalf("source-backed capability projection should not spend rows on no-op missing details:\n%s", v)
	}
	for i, line := range strings.Split(v, "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("source-backed capabilities line %d exceeds frame width %d: %d %q", i, m.Width, got, line)
		}
	}
}

func TestCapabilityRouteEvidenceShowsMissingAxes(t *testing.T) {
	m := New("REINS").FoldCapabilities(grammar.CapabilitySummary{
		Routes: []grammar.CapabilityRoute{{
			RouteID: "codex.headless.full", Platform: "codex", Mode: "headless", Profile: "full", ModelID: "gpt-test",
			RouteState: "active", AuthorityCeiling: "authoritative", FreshnessOK: true, QuotaState: "observed", ReceiptCount: 4, EvidenceCount: 4,
			Effort: "missing", ContextMode: "missing", FastMode: "missing", Quantization: "missing", CapacityPool: "missing",
			DemandVector: "missing", Hardening: "missing", EvalPlane: "missing", ReviewObligation: "missing", LearningEligibility: "missing", BenchmarkCoverage: "missing", FixedOverhead: "missing",
			AIR: map[string]string{"route_id": "ok", "platform": "ok", "mode": "ok", "profile": "ok", "model_id": "ok", "effort": "ok", "context_mode": "ok", "fast_mode": "ok", "quantization": "ok", "capacity_pool": "ok", "demand_vector": "ok", "hardening": "ok", "eval_plane": "ok", "review_obligation": "ok", "learning_eligibility": "ok", "benchmark_coverage": "ok", "fixed_overhead": "ok", "route_state": "ok", "authority_ceiling": "ok", "freshness_ok": "ok", "quota_state": "ok", "receipt_count": "ok", "evidence_count": "ok"},
		}},
	}, false)
	m.Width, m.Height, m.Page = 160, 60, PageCaps
	v := ansi.Strip(m.renderCapabilityProjection(160))
	for _, want := range []string{"ROUTE EVIDENCE", "desc", "effort=missing", "context=missing", "pool=missing", "gov", "demand=missing", "hardening=missing", "learning=missing"} {
		if !strings.Contains(v, want) {
			t.Fatalf("capability route evidence should expose missing axis %q:\n%s", want, v)
		}
	}
}

func TestSplitCapabilitiesShowSelectedLaneFit(t *testing.T) {
	longTask := "linked-task-alpha-beta-gamma-delta-epsilon-zeta-eta-theta-iota-kappa-lambda-mu-nu-xi-omicron-pi-rho-sigma-tau-upsilon"
	m := New("REINS").
		Fold([]grammar.Event{{
			TS: "10:00", Kind: "coord_dispatch.launch_started", Subject: longTask, Actor: "theta", Score: 0.21,
			AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "summary": "ok"},
		}}, false).
		FoldTasks([]grammar.Task{{TaskID: longTask, AIR: map[string]string{"task_id": "ok"}}}, false).
		FoldSessions([]grammar.Session{
			{Role: "cx-ready", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88,
				AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "ok"}},
			{Role: "theta", Platform: "claude", State: "active", Readiness: "stale", Blocker: "stale_relay", Attention: 0.54, ClaimedTask: longTask,
				AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "ok"}},
		}, false)
	m.Width, m.Height, m.Page, m.SplitContext, m.SFocus = 220, 44, PageCaps, true, 1

	v := ansi.Strip(m.View())
	for _, want := range []string{
		"sessions -> capability fit by role/platform",
		"SELECTED LANE FIT",
		"ROUTE BINDING",
		"TOOLS",
		"CAPABILITY MATCH",
		"NEXT LEGAL",
		"lane        theta",
		"platform    claude",
		"fit         verify-first",
		"claimed     linked-task-alpha-beta-gamma-delta-epsilon-zeta",
		"omicron-pi-rho-sigma-tau-upsilon",
		"task        task visible",
		"events      1 actor/task events",
		"missing     route_id/demand/eligibility/receipt",
		"hkp:support-only",
		"governed route required",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("split capabilities selected-lane fit missing %q:\n%s", want, v)
		}
	}
	for _, notWant := range []string{"[j/k]capability", "selected cap", "▶ route en", "FABRIC SCOPE", "ROUTE ADMISSION", "SESSION FIT RAIL"} {
		if strings.Contains(v, notWant) {
			t.Fatalf("split capabilities should not expose inactive capability cursor %q:\n%s", notWant, v)
		}
	}
	beforeSource, beforeCap := m.SFocus, m.CFocus
	m = step(m, "k")
	if m.SFocus != beforeSource-1 || m.CFocus != beforeCap {
		t.Fatalf("split capabilities k should move only source session focus, before source/cap=%d/%d after=%d/%d", beforeSource, beforeCap, m.SFocus, m.CFocus)
	}
	for i, line := range strings.Split(v, "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("split capabilities line %d exceeds frame width %d: %d %q", i, m.Width, got, line)
		}
	}

	m.Width, m.Height = 209, 55
	v = ansi.Strip(m.View())
	for i, line := range strings.Split(v, "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("209-col split capabilities line %d exceeds frame width %d: %d %q", i, m.Width, got, line)
		}
	}
}

func TestSplitCapabilitiesUseExactRouteBindingForTools(t *testing.T) {
	caps := grammar.CapabilitySummary{
		Rows: []grammar.CapabilityRow{{
			CapabilityID: "route_envelope", Status: "observed", Authority: "metadata-only", RouteCount: 2, OKCount: 2, EvidenceCount: 2,
			AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok"},
		}},
		Routes: []grammar.CapabilityRoute{
			{RouteID: "codex.headless.full", CapabilityID: "route_envelope", Platform: "codex", Mode: "headless", Profile: "full", RouteState: "active", FreshnessOK: true, QuotaState: "observed", ReceiptCount: 4,
				AIR: map[string]string{"route_id": "ok", "capability_id": "ok", "platform": "ok", "mode": "ok", "profile": "ok", "route_state": "ok", "freshness_ok": "ok", "quota_state": "ok", "receipt_count": "ok"}},
			{RouteID: "codex.interactive.full", CapabilityID: "route_envelope", Platform: "codex", Mode: "interactive", Profile: "full", RouteState: "active", FreshnessOK: true, QuotaState: "observed", ReceiptCount: 2,
				AIR: map[string]string{"route_id": "ok", "capability_id": "ok", "platform": "ok", "mode": "ok", "profile": "ok", "route_state": "ok", "freshness_ok": "ok", "quota_state": "ok", "receipt_count": "ok"}},
		},
		Tools: []grammar.CapabilityTool{
			{RouteID: "codex.headless.full", Platform: "codex", ToolID: "filesystem", Status: "observed", Available: true, AuthorityUse: "read,write",
				AIR: map[string]string{"route_id": "ok", "platform": "ok", "tool_id": "ok", "status": "ok", "available": "ok", "authority_use": "ok"}},
			{RouteID: "codex.interactive.full", Platform: "codex", ToolID: "browser", Status: "observed", Available: true, AuthorityUse: "observe",
				AIR: map[string]string{"route_id": "ok", "platform": "ok", "tool_id": "ok", "status": "ok", "available": "ok", "authority_use": "ok"}},
		},
	}
	m := New("REINS").
		FoldCapabilities(caps, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-bound", Platform: "codex", State: "active", Readiness: "claim", Attention: 0.88,
			RouteID: "codex.headless.full", RouteMode: "headless", RouteProfile: "full", RouteBindingState: "bound",
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "attention": "ok", "route_id": "ok", "mode": "ok", "profile": "ok", "route_binding_state": "ok"},
		}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 220, 42, PageCaps, true

	v := ansi.Strip(m.View())
	for _, want := range []string{
		"bound route codex.headless.full",
		"headless/full",
		"1 bound route tools",
		"filesystem:observed",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("bound route split capability view missing %q:\n%s", want, v)
		}
	}
	for _, bad := range []string{"browser:observed", "exact per-session needs route_id", "launch not session-confirmed"} {
		if strings.Contains(v, bad) {
			t.Fatalf("bound route split capability view should not contain %q:\n%s", bad, v)
		}
	}
}

func TestSplitCapabilitiesPolicyOnlyRouteRemainsUnconfirmed(t *testing.T) {
	caps := grammar.CapabilitySummary{
		Routes: []grammar.CapabilityRoute{{
			RouteID: "codex.headless.full", Platform: "codex", Mode: "headless", Profile: "full", RouteState: "active", FreshnessOK: true, QuotaState: "observed", ReceiptCount: 4,
			AIR: map[string]string{"route_id": "ok", "platform": "ok", "mode": "ok", "profile": "ok", "route_state": "ok", "freshness_ok": "ok", "quota_state": "ok", "receipt_count": "ok"},
		}},
		Tools: []grammar.CapabilityTool{{
			RouteID: "codex.headless.full", Platform: "codex", ToolID: "filesystem", Status: "observed", Available: true, AuthorityUse: "read,write",
			AIR: map[string]string{"route_id": "ok", "platform": "ok", "tool_id": "ok", "status": "ok", "available": "ok", "authority_use": "ok"},
		}},
	}
	m := New("REINS").
		FoldCapabilities(caps, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-policy", Platform: "codex", State: "active", Readiness: "claim", Attention: 0.88,
			RouteID: "codex.headless.full", RouteBindingState: "policy_only",
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "attention": "ok", "route_id": "ok", "route_binding_state": "ok"},
		}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 220, 42, PageCaps, true

	v := ansi.Strip(m.View())
	for _, want := range []string{"policy-only route codex.headless.full", "1 policy-only route tools", "launch not session-confirmed"} {
		if !strings.Contains(v, want) {
			t.Fatalf("policy-only route split capability view missing %q:\n%s", want, v)
		}
	}
}

func TestSplitCapabilitiesSeparatesSourceAndContextScroll(t *testing.T) {
	caps := grammar.CapabilitySummary{
		Rows: []grammar.CapabilityRow{{
			CapabilityID: "route_envelope", Status: "observed", Authority: "metadata-only", RouteCount: 1, OKCount: 1, EvidenceCount: 3,
			AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok"},
		}},
		Routes: []grammar.CapabilityRoute{{
			RouteID: "codex.headless.full", CapabilityID: "route_envelope", Platform: "codex", Mode: "headless", Profile: "full", RouteState: "active", FreshnessOK: true, QuotaState: "observed", ReceiptCount: 4,
			AIR: map[string]string{"route_id": "ok", "capability_id": "ok", "platform": "ok", "mode": "ok", "profile": "ok", "route_state": "ok", "freshness_ok": "ok", "quota_state": "ok", "receipt_count": "ok"},
		}},
		Tools: []grammar.CapabilityTool{{
			RouteID: "codex.headless.full", Platform: "codex", ToolID: "filesystem", Status: "observed", Available: true, AuthorityUse: "read,write",
			AIR: map[string]string{"route_id": "ok", "platform": "ok", "tool_id": "ok", "status": "ok", "available": "ok", "authority_use": "ok"},
		}},
	}
	m := New("REINS").
		FoldCapabilities(caps, false).
		FoldSessions([]grammar.Session{
			{Role: "cx-one", Platform: "codex", State: "active", Readiness: "claim", Attention: 0.88, RouteID: "codex.headless.full", RouteBindingState: "bound",
				AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "attention": "ok", "route_id": "ok", "route_binding_state": "ok"}},
			{Role: "cx-two", Platform: "claude", State: "active", Readiness: "stale", Attention: 0.50,
				AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "attention": "ok"}},
		}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 180, 16, PageCaps, true
	if rel := m.splitRelation(); rel.PaneProfile() != PaneLinkedScrollable || !rel.TargetScrollable {
		t.Fatalf("capability split should be linked-scrollable, got profile=%s scroll=%v", rel.PaneProfile(), rel.TargetScrollable)
	}
	if max := m.referenceScrollMax(); max == 0 {
		t.Fatalf("short capability split should have context overflow:\n%s", ansi.Strip(m.View()))
	}
	beforeSource, beforeCap := m.SFocus, m.CFocus
	m = step(m, "J")
	if m.SFocus != beforeSource || m.CFocus != beforeCap || m.RefScroll != 1 || !strings.Contains(m.Status, "context scroll") {
		t.Fatalf("capability split J should scroll right context only, source/cap/scroll/status=%d/%d/%d/%q", m.SFocus, m.CFocus, m.RefScroll, m.Status)
	}
	m = step(m, "j")
	if m.SFocus != beforeSource+1 || m.CFocus != beforeCap || m.RefScroll != 1 {
		t.Fatalf("capability split j should move only source session, source/cap/scroll=%d/%d/%d", m.SFocus, m.CFocus, m.RefScroll)
	}
	m = step(m, "K")
	if m.SFocus != beforeSource+1 || m.CFocus != beforeCap || m.RefScroll != 0 {
		t.Fatalf("capability split K should scroll context back without moving source/cap, source/cap/scroll=%d/%d/%d", m.SFocus, m.CFocus, m.RefScroll)
	}
}

func TestCapabilitiesUseSemanticRowsInMediumWidth(t *testing.T) {
	longBlocker := "route-authority-capability-descriptor-ledger-missing-before-dispatch-can-be-trusted"
	m := New("REINS").FoldSessions([]grammar.Session{
		{
			Role: "cx-ready", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"},
		},
		{
			Role: "cx-review", Platform: "claude", State: "active", Readiness: "stale", Blocker: longBlocker, Attention: 0.66,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"},
		},
	}, false)
	m.Width, m.Height, m.Page = 112, 140, PageCaps

	v := ansi.Strip(m.View())
	for _, want := range []string{
		"CAPABILITY STATUS",
		"route envelope",
		"authority=none",
		"evidence=intents:",
		"missing     route_id/demand/eligibility/receipt",
		"HKP SUPPORT CONTEXT",
		"promotion + cited-source verification",
		"platform=codex",
		"contract=coding CLI/",
		"session lane; route fit needs profile + task shape",
		"target=resume",
		"preflight=preview session resume",
		"through governed route",
		"lane=cx-review",
		"blocker=route-authority-capability-descriptor-ledger-missing-before-dispatch-can-be",
		"trusted",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("medium-width capabilities projection missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "CAPABILITY               STATUS") {
		t.Fatalf("medium-width capabilities projection should not use the dense capability table:\n%s", v)
	}
	for _, dense := range []string{"PLATFORM     LANES", "TARGET       SUBJECT"} {
		if strings.Contains(v, dense) {
			t.Fatalf("medium-width capabilities projection should not use dense %q table:\n%s", dense, v)
		}
	}
	for i, line := range strings.Split(v, "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("medium capabilities line %d exceeds frame width %d: %d %q", i, m.Width, got, line)
		}
	}
}

func TestYardReadinessUseSemanticRowsInMediumWidth(t *testing.T) {
	longTask := "release-authority-route-envelope-ledger-missing-before-ship-cutover-can-proceed"
	longBlocker := "route-authority-capability-descriptor-ledger-missing-before-dispatch-can-be-trusted"
	longKind := "coord_dispatch.launch_failed.route_authority_descriptor_missing_for_reins_cutover"
	m := New("REINS").
		Fold([]grammar.Event{{
			TS: "12:34:56", Kind: longKind, Subject: longTask, Actor: "cx-yard", Summary: "failed launch because route evidence was missing", Score: 0.91,
			AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "summary": "ok"},
		}}, false).
		FoldTasks([]grammar.Task{{
			TaskID: longTask, Stage: "S7_RELEASE", PredictedStage: "hold", Criticality: "crit", Owner: "cx-yard", Freshness: 0.12,
			AIR: map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok", "criticality": "ok", "owner": "ok", "freshness": "ok"},
		}}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-yard", Platform: "codex", State: "active", Readiness: "stale", Blocker: longBlocker, Attention: 0.77,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"},
		}}, false)
	m.Width, m.Height, m.Page = 112, 140, PageYard

	yard := ansi.Strip(m.View())
	for _, want := range []string{
		"ATTENTION RAIL",
		"task=release-authority-route-envelope-ledger-missing-before-ship-cutover-can-proceed",
		"crit=crit",
		"stage=S7 -> hold",
		"lane=cx-yard",
		"blocker=route-authority-capability-descriptor-",
		"ledger-missing-before-dispatch-can-be",
		"trusted",
		"event=release-authority-route-envelope-ledger-missing-before-ship-cutover-can-proceed",
		"kind=coord_dispatch.launch_failed.route_authority_descriptor_missing_for_reins_cutover",
	} {
		if !strings.Contains(yard, want) {
			t.Fatalf("medium yard projection missing %q:\n%s", want, yard)
		}
	}
	for i, line := range strings.Split(yard, "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("medium yard line %d exceeds frame width %d: %d %q", i, m.Width, got, line)
		}
	}

	m.Page = PageReadiness
	ready := ansi.Strip(m.View())
	for _, want := range []string{
		"TASK GATES",
		"release hold",
		"task=release-authority-route-envelope-ledger-missing-before-ship-cutover-can",
		"proceed",
		"LANE READINESS",
		"blockers=route-authority-capability-descriptor-ledger-missing-before-dispatch-can-be-trusted:1",
		"exact false gate names need a /read/gates contract",
	} {
		if !strings.Contains(ready, want) {
			t.Fatalf("medium readiness projection missing %q:\n%s", want, ready)
		}
	}
	for i, line := range strings.Split(ready, "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("medium readiness line %d exceeds frame width %d: %d %q", i, m.Width, got, line)
		}
	}
}

func TestReadinessProjectionUsesSourceBackedGateRows(t *testing.T) {
	gates := grammar.GateSummary{
		Sources: []grammar.GateSource{{
			ID: "task_projection", Status: "observed", Count: 132, Detail: "coord projection task stage/no_go snapshot", AgeBucket: "live",
			AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "detail": "ok", "age_bucket": "ok"},
		}},
		Rows: []grammar.GateRow{
			{GateID: "task.no_go.release_authorized", Domain: "task", Source: "task_projection", Subject: "117 tasks", State: "blocked", Severity: "crit", Authority: "coord_projection", Evidence: "false_on=117; stages=S7:117", Missing: "release_authorized", Action: "preserve gate until governed authority/preflight/receipt clears it",
				AIR: map[string]string{"gate_id": "ok", "domain": "ok", "source": "ok", "subject": "ok", "state": "ok", "severity": "ok", "authority": "ok", "evidence": "ok", "missing": "ok", "action": "ok"}},
			{GateID: "lane.blocker.stale_relay", Domain: "lane", Source: "session_state", Subject: "4 lanes", State: "blocked", Severity: "warn", Authority: "coordinator_state", Evidence: "count=4; readiness=stale:4", Missing: "stale_relay", Action: "inspect lane/session state before resume or dispatch",
				AIR: map[string]string{"gate_id": "ok", "domain": "ok", "source": "ok", "subject": "ok", "state": "ok", "severity": "ok", "authority": "ok", "evidence": "ok", "missing": "ok", "action": "ok"}},
			{GateID: "route.binding.policy_only", Domain: "route", Source: "route_binding", Subject: "1 lanes", State: "preview-only", Severity: "warn", Authority: "route_binding_ledger", Evidence: "count=1; routes=codex.headless.full:1", Missing: "launch receipt/session confirmation", Action: "inspect :sessions/:yard route evidence before resume or dispatch",
				AIR: map[string]string{"gate_id": "ok", "domain": "ok", "source": "ok", "subject": "ok", "state": "ok", "severity": "ok", "authority": "ok", "evidence": "ok", "missing": "ok", "action": "ok"}},
			{GateID: "command.dispatch", Domain: "command", Source: "command_registry", Subject: "dispatch", State: "preview-only", Severity: "warn", Authority: "methodology dispatch", Evidence: "verbs registered; mutation disabled in Reins", Missing: "authority_case,parent_spec,preflight,receipt", Action: ":intent dispatch",
				AIR: map[string]string{"gate_id": "ok", "domain": "ok", "source": "ok", "subject": "ok", "state": "ok", "severity": "ok", "authority": "ok", "evidence": "ok", "missing": "ok", "action": "ok"}},
		},
		Totals: map[string]int{"rows": 4, "blocked": 2, "preview": 2},
	}
	m := New("REINS").
		FoldGates(gates, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-route", Platform: "codex", State: "active", Readiness: "claim", RouteID: "codex.headless.full", RouteBindingState: "policy_only",
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "route_id": "ok", "route_binding_state": "ok"},
		}}, false)
	m.Width, m.Height, m.Page = 170, 80, PageReadiness

	v := ansi.Strip(m.View())
	for _, want := range []string{
		"GATE SOURCES",
		"task_projection",
		"TASK GATES",
		"task.no_go.release_authorized",
		"false_on=117",
		"missing=release_authorized",
		"LANE READINESS",
		"bound evidence:1",
		"policy-only:1",
		"lane.blocker.stale_relay",
		"ROUTE BINDING",
		"route.binding.policy_only",
		"codex.headless.full",
		"COMMAND ROUTE",
		"command.dispatch",
		"GUARDRAILS",
		"/read/gates preserves raw false no_go names",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("source-backed readiness projection missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "no /read/gates contract yet") {
		t.Fatalf("source-backed readiness should not show old missing-contract copy:\n%s", v)
	}
	for _, old := range []string{"GATE                         DOMAIN", "gate=task.no_go.release_authorized", "next=preserve gate until governed"} {
		if strings.Contains(v, old) {
			t.Fatalf("source-backed readiness should use compact gate fact rows, not old %q:\n%s", old, v)
		}
	}
}

func TestCapabilityPageCursorSelectsCapabilityContext(t *testing.T) {
	caps := grammar.CapabilitySummary{
		Rows: []grammar.CapabilityRow{
			{CapabilityID: "source_acquisition", Status: "admission-incomplete", Authority: "sub-router", CapabilityClass: "source_acquisition", SurfaceFamily: "source_acquisition", SpendModel: "mixed_api_connector", EgressClass: "source_query", ReceiptRequirement: "source + egress receipt", RouteCount: 5, OKCount: 3, BlockedCount: 2, EvidenceCount: 5, Blocker: "Tavily usage telemetry schema",
				AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "capability_class": "ok", "surface_family": "ok", "spend_model": "ok", "egress_class": "ok", "receipt_requirement": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "blocker": "ok", "hkp_posture": "ok"}},
			{CapabilityID: "tavily_source_acquisition", Status: "admission-incomplete", Authority: "source-acquisition", CapabilityClass: "source_acquisition", SurfaceFamily: "tavily", SpendModel: "api_spend_budgeted", EgressClass: "source_query", ReceiptRequirement: "usage + budget + route receipt", RouteCount: 1, OKCount: 0, BlockedCount: 1, EvidenceCount: 4, Blocker: "route on refreshed usage receipt",
				AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "capability_class": "ok", "surface_family": "ok", "spend_model": "ok", "egress_class": "ok", "receipt_requirement": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "blocker": "ok", "hkp_posture": "ok"}},
			{CapabilityID: "route_envelope", Status: "observed", Authority: "metadata-only", RouteCount: 13, OKCount: 11, BlockedCount: 2, EvidenceCount: 13,
				AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "blocker": "ok", "hkp_posture": "ok"}},
		},
	}
	m := New("REINS").FoldCapabilities(caps, false)
	m.Width, m.Height, m.Page = 160, 48, PageCaps

	if row, ok := m.FocusedCapabilityRow(); !ok || row.Name != "source_acquisition" {
		t.Fatalf("initial capability focus = %#v ok=%v", row, ok)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"CAPABILITY CLASS COCKPIT", "source_acquisition", "status=admission-incomplete", "authority=sub-router", "ready=3/5", "surfaces=0/1", "evidence=5", "mixed_"} {
		if !strings.Contains(v, want) {
			t.Fatalf("capability map missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "CAPABILITY MAP") || strings.Contains(v, "CAPABILITY CLASS ROLLUP") {
		t.Fatalf("capability class cockpit should replace duplicate map/rollup sections:\n%s", v)
	}
	if cockpit, fabric := strings.Index(v, "CAPABILITY CLASS COCKPIT"), strings.Index(v, "FABRIC SCOPE"); cockpit < 0 || fabric < 0 || cockpit > fabric {
		t.Fatalf("capability class cockpit should appear before fabric doctrine:\n%s", v)
	}
	nm, _ := m.Update(key("j"))
	m = nm.(Model)
	if row, ok := m.FocusedCapabilityRow(); !ok || row.Name != "tavily_source_acquisition" {
		t.Fatalf("next capability focus = %#v ok=%v", row, ok)
	}
	v = ansi.Strip(m.View())
	for _, want := range []string{"focus tavily_source_acquisit", "capability  tavily_source_acquisition", "authority   source-acquisition", "evidence    routes:1 ok:0", "family      tavily", "receipt     usage + budget + route receipt", "missing     route on refreshed usage receipt", "[j/k]capability"} {
		if !strings.Contains(v, want) {
			t.Fatalf("capability cursor view missing %q:\n%s", want, v)
		}
	}
}

func TestDomainsRenderSourceBackedRowsBeforeFallback(t *testing.T) {
	m := New("REINS").FoldDomains(grammar.DomainSummary{
		LifecycleSources: []grammar.DomainSource{{
			ID: "hapax-lifecycle-registry", Status: "observed", Count: 3, AgeBucket: "<5m", Authority: "support_non_authoritative", Detail: "lifecycle registry metadata",
			AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "authority": "ok", "detail": "ok"},
		}},
		Lifecycles: []grammar.LifecycleRow{{
			LifecycleID: "ldlc", Label: "Private Life Label", Owner: "operator-instance", Scope: "tenant", Plant: "life-management", Posture: "future-tenant",
			State: "dark_specified", Maturity: "declared-not-modeled", AdapterID: "adapter.ldlc.pending", AuthorityCeiling: "support_non_authoritative",
			ClaimSurface: "practical loops", MutationSurface: "none until receipts exist", DarkPolicy: "declared future lifecycle only", FreshnessPolicy: "requires source inventory",
			AIRClass: "private-life", Windows: "domains,intake", Surfaces: "hapax-exclusive-chat", Commands: "note,show-route", ReceiptContracts: "consent,privacy",
			EvidenceCount: 1, Blocker: "source model absent", NextEvidence: "LDLC parent spec", SourceRefs: "hapax-lifecycle-registry:1 refs",
			AIR: map[string]string{"lifecycle_id": "ok", "label": "deny", "owner": "ok", "scope": "ok", "plant": "ok", "posture": "ok", "state": "ok", "maturity": "ok", "adapter_id": "ok", "authority_ceiling": "ok", "claim_surface": "ok", "mutation_surface": "ok", "dark_policy": "ok", "freshness_policy": "ok", "air_class": "ok", "windows": "ok", "surfaces": "ok", "commands": "ok", "receipt_contracts": "ok", "evidence_count": "ok", "blocker": "ok", "next_evidence": "ok", "source_refs": "ok"},
		}},
		LifecycleTotals:      map[string]int{"sources": 1, "rows": 1, "missing_sources": 0},
		LifecycleAuthority:   "support_non_authoritative",
		LifecycleGeneratedAt: "2026-06-25T20:10:00Z",
		LifecyclePackageHash: "sha256:123456",
		LifecycleDefaultLens: "tenant-lifecycle",
		Sources: []grammar.DomainSource{{
			ID: "rdlc-pack", Status: "observed", Count: 2, AgeBucket: "<5m", Authority: "CASE-RDLC", Detail: "domain pack metadata",
			AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "authority": "ok", "detail": "ok"},
		}},
		Rows: []grammar.DomainRow{{
			DomainID: "rdlc-labrack", Label: "Private Label", Lifecycle: "RDLC", Terrain: "bedrock", Depth: "stratum", Scope: "tenant", State: "candidate",
			AuthorityCeiling: "support_non_authoritative", ClaimCeiling: "navigation", Windows: "domains,dynamics", Surfaces: "wide-context", Parity: "labrack", EvidenceCount: 1, SourceRefs: "rdlc-pack:1 refs",
			AIR: map[string]string{"domain_id": "ok", "label": "deny", "lifecycle": "ok", "terrain": "ok", "depth": "ok", "scope": "ok", "state": "ok", "authority_ceiling": "ok", "claim_ceiling": "ok", "windows": "ok", "surfaces": "ok", "parity": "ok", "evidence_count": "ok", "source_refs": "ok"},
		}},
		Relations: []grammar.DomainRelation{{
			Source: "rdlc-labrack", Target: "research-rdlc", Relation: "extends", AuthorityCeiling: "support_non_authoritative", SourceRefs: "rdlc-pack:1 refs",
			AIR: map[string]string{"source": "ok", "target": "ok", "relation": "ok", "authority_ceiling": "ok", "source_refs": "ok"},
		}},
		Totals:      map[string]int{"sources": 1, "rows": 1, "relations": 1},
		Authority:   "CASE-RDLC",
		GeneratedAt: "2026-06-25T10:00:00Z",
		PackageHash: "sha256:abcdef",
		DefaultLens: "lifecycle",
	}, false)
	m.Page, m.AIR = PageDomains, true
	v := ansi.Strip(m.renderDomainCatalog(220))
	for _, want := range []string{"LIFECYCLE REGISTRY", "ldlc", "dark_specified", "declared-not-modeled", "consent,privacy", "PACK SOURCES", "TOPOLOGY", "central", "RDLC→compiled:1", "SOURCE-BACKED DOMAINS", "rdlc-labrack", "RDLC", "support_non_author", "RELATIONS", "extends", "COMPILED FALLBACK"} {
		if !strings.Contains(v, want) {
			t.Fatalf("source-backed domains view missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "Private Label") || strings.Contains(v, "Private Life Label") {
		t.Fatalf("private labels should still honor AIR default deny:\n%s", v)
	}
	if strings.Index(v, "SOURCE-BACKED DOMAINS") > strings.Index(v, "COMPILED FALLBACK") {
		t.Fatalf("source-backed domains must render before compiled fallback:\n%s", v)
	}
}

func TestLifecyclesRenderSourceBackedContractsBeforeFallback(t *testing.T) {
	m := New("REINS").FoldDomains(grammar.DomainSummary{
		LifecycleSources: []grammar.DomainSource{{
			ID: "hapax-lifecycle-registry", Status: "observed", Count: 3, AgeBucket: "<5m", Authority: "support_non_authoritative", Detail: "lifecycle registry metadata",
			AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "authority": "ok", "detail": "ok"},
		}},
		Lifecycles: []grammar.LifecycleRow{{
			LifecycleID: "ldlc", Label: "Private Life Label", Owner: "operator-instance", Scope: "tenant", Plant: "life-management", Posture: "future-tenant",
			State: "dark_specified", Maturity: "declared-not-modeled", AdapterID: "adapter.ldlc.pending", AuthorityCeiling: "support_non_authoritative",
			ClaimSurface: "practical loops", MutationSurface: "none until receipts exist", DarkPolicy: "declared future lifecycle only", FreshnessPolicy: "requires source inventory",
			AIRClass: "private-life", Windows: "lifecycles,domains,intake", Surfaces: "hapax-exclusive-chat", Commands: "note,show-route", ReceiptContracts: "consent,privacy",
			EvidenceCount: 1, Blocker: "source model absent", NextEvidence: "LDLC parent spec", SourceRefs: "hapax-lifecycle-registry:1 refs",
			AIR: map[string]string{"lifecycle_id": "ok", "label": "deny", "owner": "ok", "scope": "ok", "plant": "ok", "posture": "ok", "state": "ok", "maturity": "ok", "adapter_id": "ok", "authority_ceiling": "ok", "claim_surface": "ok", "mutation_surface": "ok", "dark_policy": "ok", "freshness_policy": "ok", "air_class": "ok", "windows": "ok", "surfaces": "ok", "commands": "ok", "receipt_contracts": "ok", "evidence_count": "ok", "blocker": "ok", "next_evidence": "ok", "source_refs": "ok"},
		}},
		LifecycleTotals:      map[string]int{"sources": 1, "rows": 1, "missing_sources": 0},
		LifecycleAuthority:   "support_non_authoritative",
		LifecycleGeneratedAt: "2026-06-25T20:10:00Z",
		LifecyclePackageHash: "sha256:123456",
		LifecycleDefaultLens: "tenant-lifecycle",
	}, false)
	m.Page, m.AIR = PageLifecycles, true
	v := ansi.Strip(m.renderLifecycleCatalog(170))
	for _, want := range []string{"LIFECYCLES", "PACK SOURCES", "SOURCE-BACKED LIFECYCLES", "ldlc", "dark_specified", "future-tenant", "life-management", "support_non_author", "practical loops", "none until receipts exist", "note,show-route", "consent,privacy", "hapax-lifecycle-registry:1 refs", "COMPILED FALLBACK", "EXTENSIBILITY", "tenant-safe", "LDLC", "n-DLC"} {
		if !strings.Contains(v, want) {
			t.Fatalf("source-backed lifecycles view missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "Private Life Label") {
		t.Fatalf("private lifecycle labels should honor AIR default deny:\n%s", v)
	}
	if strings.Index(v, "SOURCE-BACKED LIFECYCLES") > strings.Index(v, "COMPILED FALLBACK") {
		t.Fatalf("source-backed lifecycle rows must render before compiled fallback:\n%s", v)
	}
	m = step(m, "j")
	if m.LifecycleFocus != 0 || !strings.Contains(m.Status, "lifecycle 1/1") {
		t.Fatalf("lifecycle cursor should clamp on the single source-backed row, focus=%d status=%q", m.LifecycleFocus, m.Status)
	}
}

func TestReadReceiptPulseChangesOnlyOnReadFolds(t *testing.T) {
	m := New("REINS")
	m.Width, m.Height = 160, 40
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "rx . e0 t0 s0 i0 c0 g0 o0 d0 p0 none") {
		t.Fatalf("initial receipt should be zeroed:\n%s", v)
	}
	before := ansi.Strip(m.readReceipt())
	nm, cmd := m.Update(tea.WindowSizeMsg{Width: 150, Height: 35})
	if cmd != nil {
		t.Fatal("window-size updates must not arm a command")
	}
	m = nm.(Model)
	if after := ansi.Strip(m.readReceipt()); after != before {
		t.Fatalf("non-read updates must not pulse the read receipt: before=%q after=%q", before, after)
	}
	flashSeq := m.FlashSeq
	nm, cmd = m.Update(EventsMsg{Events: evFixture()})
	if cmd != nil {
		t.Fatal("read folds must not arm flash clear ticks")
	}
	m = nm.(Model)
	if m.Flash != "" || m.FlashSeq != flashSeq {
		t.Fatalf("read folds must not use the action flash channel: flash=%q seq=%d/%d", m.Flash, m.FlashSeq, flashSeq)
	}
	nm, _ = m.Update(SessionsMsg{Sessions: []grammar.Session{{Role: "cx", AIR: map[string]string{"role": "ok"}}}})
	m = nm.(Model)
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "rx / e1 t0 s1 i0 c0 g0 o0 d0 p0 sessions") {
		t.Fatalf("receipt should reflect read folds, got:\n%s", v)
	}
}

func TestBeatDoesNotMoveHealthyReadSpine(t *testing.T) {
	m := New("REINS")
	m.Width, m.Height = 160, 40
	m.EventsSeq = 1
	m.TasksSeq = 1
	m.SessionsSeq = 1
	m.IntakeSeq = 1
	m.CapabilitiesSeq = 1
	m.GatesSeq = 1
	m.DomainsSeq = 1
	m.DynamicsSeq = 1
	m.EpistemicsSeq = 1
	m.LastFold = "sessions"
	beforeReceipt := ansi.Strip(m.readReceipt())
	beforeVital := ansi.Strip(m.viewVital(160))
	if !strings.Contains(beforeVital, "spine:read/") {
		t.Fatalf("healthy spine should be read-fold driven:\n%s", beforeVital)
	}
	nm, cmd := m.Update(BeatMsg{})
	if cmd != nil {
		t.Fatal("beat must not arm a command")
	}
	m = nm.(Model)
	afterReceipt := ansi.Strip(m.readReceipt())
	afterVital := ansi.Strip(m.viewVital(160))
	if afterReceipt != beforeReceipt {
		t.Fatalf("beat must not move read receipts: before=%q after=%q", beforeReceipt, afterReceipt)
	}
	if afterVital != beforeVital {
		t.Fatalf("beat must not animate the healthy read spine:\nbefore=%s\nafter=%s", beforeVital, afterVital)
	}
}

func TestBootAndDarkSpineExposeSourceHealth(t *testing.T) {
	m := New("REINS")
	m.Width, m.Height = 160, 40
	if got := ansi.Strip(m.viewSpine(false)); got != "spine:BOOT 0/9·" {
		t.Fatalf("empty read model should render boot spine, got %q", got)
	}
	nm, _ := m.Update(BeatMsg{})
	m = nm.(Model)
	if got := ansi.Strip(m.viewSpine(false)); got != "spine:BOOT 0/9∙" {
		t.Fatalf("boot spine may animate while sources are not folded, got %q", got)
	}
	m.EventsDark = true
	if got := ansi.Strip(m.viewSpine(false)); got != "spine:DARK 1/9" {
		t.Fatalf("dark source should dominate spine, got %q", got)
	}
}

func TestReadSourceChipMarksActiveFoldWithoutBeatDrift(t *testing.T) {
	m := New("REINS")
	m.EventsSeq = 1
	m.TasksSeq = 2
	m.LastFold = "events"

	beforeReceipt := ansi.Strip(m.readReceipt())
	beforeActive := ansi.Strip(m.readSourceChip("events", 3, false, m.EventsSeq))
	beforeInactive := ansi.Strip(m.readSourceChip("tasks", 2, false, m.TasksSeq))
	if beforeActive != "events:3 r1/" {
		t.Fatalf("active source chip should expose the read-fold pulse, got %q", beforeActive)
	}
	if beforeInactive != "tasks:2 r2." {
		t.Fatalf("inactive source chip should stay stable, got %q", beforeInactive)
	}

	nm, cmd := m.Update(BeatMsg{})
	if cmd != nil {
		t.Fatal("visual beat must not arm a command")
	}
	m = nm.(Model)

	if afterReceipt := ansi.Strip(m.readReceipt()); afterReceipt != beforeReceipt {
		t.Fatalf("visual beat must not move read receipts: before=%q after=%q", beforeReceipt, afterReceipt)
	}
	afterActive := ansi.Strip(m.readSourceChip("events", 3, false, m.EventsSeq))
	afterInactive := ansi.Strip(m.readSourceChip("tasks", 2, false, m.TasksSeq))
	if afterActive != beforeActive {
		t.Fatalf("beat must not advance the active read-source chip: before=%q after=%q", beforeActive, afterActive)
	}
	if afterInactive != beforeInactive {
		t.Fatalf("inactive source chip should not animate: before=%q after=%q", beforeInactive, afterInactive)
	}
}

func TestReadReceiptPulseCoversAllReadSources(t *testing.T) {
	m := New("REINS")
	cases := []struct {
		last string
		mut  func(*Model)
		want string
	}{
		{"events", func(m *Model) { m.EventsSeq = 1 }, "rx / e1 t0 s0 i0 c0 g0 o0 d0 p0 events"},
		{"tasks", func(m *Model) { m.TasksSeq = 2 }, "rx - e0 t2 s0 i0 c0 g0 o0 d0 p0 tasks"},
		{"sessions", func(m *Model) { m.SessionsSeq = 3 }, "rx \\ e0 t0 s3 i0 c0 g0 o0 d0 p0 sessions"},
		{"intake", func(m *Model) { m.IntakeSeq = 4 }, "rx | e0 t0 s0 i4 c0 g0 o0 d0 p0 intake"},
		{"capabilities", func(m *Model) { m.CapabilitiesSeq = 5 }, "rx / e0 t0 s0 i0 c5 g0 o0 d0 p0 capabilities"},
		{"gates", func(m *Model) { m.GatesSeq = 6 }, "rx - e0 t0 s0 i0 c0 g6 o0 d0 p0 gates"},
		{"domains", func(m *Model) { m.DomainsSeq = 7 }, "rx \\ e0 t0 s0 i0 c0 g0 o7 d0 p0 domains"},
		{"dynamics", func(m *Model) { m.DynamicsSeq = 8 }, "rx | e0 t0 s0 i0 c0 g0 o0 d8 p0 dynamics"},
		{"epistemics", func(m *Model) { m.EpistemicsSeq = 9 }, "rx / e0 t0 s0 i0 c0 g0 o0 d0 p9 epistemics"},
		{"", func(m *Model) {}, "rx . e0 t0 s0 i0 c0 g0 o0 d0 p0 none"},
	}
	for _, tc := range cases {
		t.Run(tc.last, func(t *testing.T) {
			mm := m
			mm.LastFold = tc.last
			tc.mut(&mm)
			if tc.last == "" {
				mm.LastFold = ""
			}
			got := ansi.Strip(mm.readReceipt())
			if got != tc.want {
				t.Fatalf("receipt mismatch:\nwant %q\n got %q", tc.want, got)
			}
		})
	}
}

func TestYardCockpitRendersLiveReadModel(t *testing.T) {
	m := New("REINS").
		Fold([]grammar.Event{
			{TS: "2026-06-24T10:00:00Z", Kind: "coord_dispatch.launch_started", Subject: "task-a", Actor: "cx-p0", AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok"}},
			{TS: "2026-06-24T10:05:00Z", Kind: "coord_dispatch.launch_failed", Subject: "task-b", Actor: "cx-p0", AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok"}},
		}, false).
		FoldTasks([]grammar.Task{
			{TaskID: "release-blocked-task", Stage: "S7_RELEASE", PredictedStage: "hold", Criticality: "crit", AIR: map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok", "criticality": "ok"}},
			{TaskID: "calm-task", Stage: "S3_PLAN", PredictedStage: "next", Criticality: "ok", AIR: map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok", "criticality": "ok"}},
		}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-p0", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88, RouteID: "codex.headless.full", RouteBindingState: "policy_only",
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "route_id": "ok", "route_binding_state": "ok"},
		}}, false).
		FoldGates(grammar.GateSummary{
			Rows: []grammar.GateRow{{
				GateID: "task.no_go.release_authorized", Domain: "task", Source: "task_projection", Subject: "117 tasks", State: "blocked", Severity: "crit", Authority: "coord_projection",
				Evidence: "false_on=117", Missing: "release_authorized", Action: "preserve gate until governed receipt clears it",
				AIR: map[string]string{"gate_id": "ok", "domain": "ok", "source": "ok", "subject": "ok", "state": "ok", "severity": "ok", "authority": "ok", "evidence": "ok", "missing": "ok", "action": "ok"},
			}},
			Totals: map[string]int{"blocked": 1},
		}, false).
		FoldDynamics(grammar.Graph{Nodes: []grammar.Node{{ID: "n1", Res: "1"}}, Edges: []grammar.Edge{{Source: "a", Target: "b"}}}, false)
	m.Width, m.Height, m.Page = 180, 50, PageYard

	v := ansi.Strip(m.View())
	for _, want := range []string{
		"YARD", "events:2", "tasks:2", "sessions:1",
		"RAIL TOPOLOGY", "stations", "gate signals", "lane lines", "operator line", "throat", "witness terminus", "dark sources",
		"task.no_go.release_authorized", "blocked", "release_authorized",
		"LADDER", "S7:1", "release-blocked-task", "cx-p0", "policy-only codex.headless.full", "coord_dispatch.launch_failed", "FLEET MATRIX", "claim:1", "codex:1", "governed COMMAND route required",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("yard cockpit missing %q:\n%s", want, v)
		}
	}
}

func TestYardRailTopologyShowsPerSourceDarkMarkers(t *testing.T) {
	m := New("REINS").
		FoldGates(grammar.GateSummary{Rows: []grammar.GateRow{{
			GateID: "command.dispatch", Domain: "command", State: "preview-only", Severity: "warn", Missing: "authority_case,parent_spec,receipt",
			AIR: map[string]string{"gate_id": "ok", "domain": "ok", "state": "ok", "severity": "ok", "missing": "ok", "action": "ok"},
		}}}, true)
	m.DomainsDark = true
	m.Width, m.Height, m.Page = 132, 50, PageYard

	v := ansi.Strip(m.View())
	for _, want := range []string{"dark sources", "events:ok", "tasks:ok", "sessions:ok", "gates:◇", "domains:◇", "dynamics:ok"} {
		if !strings.Contains(v, want) {
			t.Fatalf("yard dark marker missing %q:\n%s", want, v)
		}
	}
}

func TestYardRailTopologyHonorsAIRDeniedFields(t *testing.T) {
	m := New("REINS").
		Fold([]grammar.Event{{
			TS: "2026-06-24T10:05:00Z", Kind: "coord_dispatch.launch_failed", Subject: "SECRET-EVENT-SUBJECT", Actor: "SECRET-LANE", Summary: "failed launch",
			AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "deny", "actor": "deny", "summary": "ok"},
		}}, false).
		FoldTasks([]grammar.Task{{
			TaskID: "SECRET-TASK-ID", Stage: "S7_RELEASE", PredictedStage: "SECRET-NEXT", Criticality: "crit",
			AIR: map[string]string{"task_id": "deny", "stage": "ok", "predicted_stage": "deny", "criticality": "ok"},
		}}, false).
		FoldSessions([]grammar.Session{{
			Role: "SECRET-LANE", Platform: "codex", State: "active", Readiness: "claim", Blocker: "SECRET-BLOCKER", Attention: 0.88, RouteID: "SECRET-ROUTE",
			AIR: map[string]string{"role": "deny", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "deny", "attention": "ok", "route_id": "deny"},
		}}, false).
		FoldGates(grammar.GateSummary{Rows: []grammar.GateRow{{
			GateID: "SECRET-GATE-ID", Domain: "task", State: "blocked", Severity: "crit", Missing: "SECRET-MISSING", Action: "SECRET-ACTION", Evidence: "SECRET-EVIDENCE",
			AIR: map[string]string{"gate_id": "deny", "domain": "ok", "state": "ok", "severity": "ok", "missing": "deny", "action": "deny", "evidence": "deny"},
		}}}, false)
	m.AIR = true
	m.Width, m.Height, m.Page = 140, 80, PageYard

	v := ansi.Strip(m.View())
	for _, secret := range []string{"SECRET-TASK-ID", "SECRET-NEXT", "SECRET-LANE", "SECRET-BLOCKER", "SECRET-ROUTE", "SECRET-EVENT-SUBJECT", "SECRET-GATE-ID", "SECRET-MISSING", "SECRET-ACTION", "SECRET-EVIDENCE"} {
		if strings.Contains(v, secret) {
			t.Fatalf("yard rail topology leaked AIR-denied value %q:\n%s", secret, v)
		}
	}
	for _, want := range []string{"RAIL TOPOLOGY", "S7:1", "hidden:", "witness terminus"} {
		if !strings.Contains(v, want) {
			t.Fatalf("yard AIR projection missing safe marker %q:\n%s", want, v)
		}
	}
}

func TestDomainRegistryCoversLifecycleExtensibility(t *testing.T) {
	byID := map[string]DomainDef{}
	for _, d := range registeredDomains() {
		if d.Terrain == "" || d.Depth == "" || d.Scope == "" || d.Windows == "" || d.Surfaces == "" || d.Parity == "" {
			t.Fatalf("domain %q must declare terrain, depth, scope, windows, surfaces, and parity: %+v", d.ID, d)
		}
		byID[d.ID] = d
	}
	for _, want := range []string{
		"substrate-events", "sdlc-work", "system-dynamics", "command-intent", "capability-routing", "governance-readiness",
		"intake-observations", "research-rdlc", "tool-sessions", "governance-safety", "future-n-dlc",
	} {
		if _, ok := byID[want]; !ok {
			t.Fatalf("domain registry missing %q", want)
		}
	}
}

func TestSurfaceRegistryCoversTransientState(t *testing.T) {
	byID := map[string]SurfaceDef{}
	for _, s := range registeredSurfaces() {
		if s.Open == "" || s.Exit == "" || s.Scope == "" || s.Kind == "" || s.AIR == "" || s.Contract == "" {
			t.Fatalf("surface %q must declare activation, exit, scope, kind, AIR policy, and contract: %+v", s.ID, s)
		}
		byID[s.ID] = s
	}
	for _, want := range []string{
		"command", "filter", "hint", "field-rank", "yank", "class-select", "whois",
		"session-detail", "dyn-scale", "intent-target", "wide-context", "split-context",
		"gate-stack", "observation-feed", "observation-detail", "selection-template", "air-lens", "read-dark", "flash", "compose",
	} {
		if _, ok := byID[want]; !ok {
			t.Fatalf("surface registry missing %q", want)
		}
	}
}

func TestIntentReviewPageRendersTargetSubjectAndContract(t *testing.T) {
	m := New("REINS").FoldTasks([]grammar.Task{{
		TaskID: "target-task", AIR: map[string]string{"task_id": "ok"},
	}}, false)
	m.Width, m.Height, m.Page = 160, 40, PageTasks
	out := m.Exec("intent show-route")
	if out.Page != PageIntent || out.IntentTarget != "show-route" {
		t.Fatalf(":intent show-route must open the intent review page, got page=%d target=%q", out.Page, out.IntentTarget)
	}
	v := ansi.Strip(out.View())
	for _, want := range []string{"INTENT REVIEW", "show-route", "selection task target-task", "governed COMMAND route required", "no effect emitted", "preflight", "receipt", "cursor", "[Enter] preview selected target", "ROUTE PREVIEW LADDER", "SUBJECT BINDING", "{{focus}}"} {
		if !strings.Contains(v, want) {
			t.Fatalf("intent review page missing %q:\n%s", want, v)
		}
	}
	out = out.Exec("intent")
	nm, _ := out.Update(key("j"))
	out = nm.(Model)
	if out.IntentFocus != 1 || !strings.Contains(out.Status, "intent target 2/9") {
		t.Fatalf("j should move the intent target cursor, focus=%d status=%q", out.IntentFocus, out.Status)
	}
	nm, _ = out.Update(key("enter"))
	out = nm.(Model)
	if out.IntentTarget != "dispatch" || !strings.Contains(out.Status, "intent dispatch") || !strings.Contains(out.Status, "no effect emitted") {
		t.Fatalf("Enter should preview the selected intent target without effect, target=%q status=%q", out.IntentTarget, out.Status)
	}
}

func TestWideIntentContextRailAndFooterAdvertiseTargetCursor(t *testing.T) {
	m := New("REINS").FoldTasks([]grammar.Task{{
		TaskID: "target-task", AIR: map[string]string{"task_id": "ok"},
	}}, false)
	m.Width, m.Height, m.Page = 220, 52, PageTasks
	m = m.Exec("intent dispatch")

	v := ansi.Strip(m.View())
	floor := ansi.Strip(m.viewFloor(m.Width))
	for _, want := range []string{"legal next", "[j/k]", "intent target 2/9", "[Enter]", "preview selected target"} {
		if !strings.Contains(v, want) && !strings.Contains(floor, want) {
			t.Fatalf("wide intent rail/footer missing %q:\nview:\n%s\nfloor:\n%s", want, v, floor)
		}
	}
	if strings.Contains(v, "[j/k]       scroll") || strings.Contains(floor, "[j/k]scroll") {
		t.Fatalf("wide intent must advertise target cursor, not generic reference scroll:\nview:\n%s\nfloor:\n%s", v, floor)
	}
}

func TestSplitIntentReviewDoesNotAdvertiseRightPaneEnter(t *testing.T) {
	m := New("REINS").FoldSessions([]grammar.Session{{
		Role: "cx-source", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88,
		AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"},
	}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 220, 44, PageIntent, true

	v := ansi.Strip(m.View())
	for _, want := range []string{"sessions + intent review as explicit target", "split: source owns [j/k]/[Enter]", "use :intent <target> or [|] unsplit", "[↵]src-detail"} {
		if !strings.Contains(v, want) {
			t.Fatalf("split intent review missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "[Enter] preview selected target") {
		t.Fatalf("split intent should not advertise right-pane Enter target preview:\n%s", v)
	}
}

func TestSplitIntentSubjectTracksSourceLane(t *testing.T) {
	m := New("REINS").FoldSessions([]grammar.Session{
		{Role: "cx-one", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"}},
		{Role: "cx-two", Platform: "claude", State: "active", Readiness: "stale", Blocker: "stale_relay", Attention: 0.55,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"}},
	}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 220, 44, PageIntent, true
	m.IntentTarget, m.IntentSubject = "dispatch", "task stale-task"

	v := ansi.Strip(m.View())
	if !strings.Contains(v, "subject     session cx-one") {
		t.Fatalf("split intent should bind to visible source lane, not stale captured subject:\n%s", v)
	}
	m = step(m, "j")
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "subject     session cx-two") {
		t.Fatalf("split intent subject should track source lane movement:\n%s", v)
	}
	if strings.Contains(v, "task stale-task") {
		t.Fatalf("split intent should not keep stale non-source subject:\n%s", v)
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

func TestDynamicsPageRendersReaderGuideAndEpistemicPath(t *testing.T) {
	g := grammar.Graph{
		MapID:  "dyn-test",
		Thesis: "Faithful rendering of system dynamics requires a source-neutral semantic graph backbone.",
		Layers: []grammar.Layer{{ID: "L", Label: "Backbone"}},
		Nodes: []grammar.Node{{
			ID: "secret-node", Label: "SECRET-REFERENCE", Layer: "L", Status: "observed",
			AIR: map[string]string{"id": "deny", "label": "deny", "layer": "ok", "status": "ok"},
		}},
	}
	m := New("REINS").FoldDynamics(g, false).Exec("dynamics")
	m.Width, m.Height, m.AIR = 120, 56, true
	v := ansi.Strip(m.View())
	for _, want := range []string{
		"Faithful rendering of system dynamics requires",
		"MAP ORIENTATION",
		"source-neutral semantic graph backbone",
		"inputs or projections",
		"overview first",
		"system dynamics asks",
		"map element -> source doc -> claim -> observation -> validation -> lens",
		"scale path",
		"▶all",
		"scale why",
		"full topology",
		"raw backends",
		"SDLC/RDLC/LDLC",
		"does not prove causality",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("dynamics reader guide missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "SECRET-REFERENCE") || strings.Contains(v, "secret-node") {
		t.Fatalf("dynamics reader guide/page must not leak denied node values under AIR:\n%s", v)
	}
	for i, line := range strings.Split(m.View(), "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("dynamics reader guide line %d exceeds width %d: %d %q", i, m.Width, got, line)
		}
	}
}

func TestDynamicsPageRendersPackageEvidenceAndAIR(t *testing.T) {
	g := grammar.Graph{
		Layers: []grammar.Layer{{ID: "L", Label: "Backbone"}},
		Nodes: []grammar.Node{
			{ID: "rdf-owl-kg", Label: "KG", Layer: "L", Status: "asserted",
				Summary:         "Canonical semantic backbone for system identity.",
				Context:         "Identity, state, evidence, and authority remain separable.",
				Docs:            "W3C RDF 1.2 Concepts, W3C OWL 2 Overview",
				HardeningNotes:  "Use named graphs for asserted and observed claims.",
				Tags:            "semantic-core, identity",
				SourceRefs:      "docs:1 refs",
				SourceRefLabels: []string{"rdf.md#concepts"},
				AIR:             map[string]string{"id": "ok", "label": "ok", "status": "ok", "summary": "deny", "context": "deny", "docs": "deny", "hardening_notes": "deny", "tags": "deny", "source_refs": "ok", "source_ref_labels": "ok"}},
			{ID: "governance", Label: "Gov", Layer: "L", Status: "observed",
				AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
		},
		Edges: []grammar.Edge{{ID: "kg-to-governance", Source: "rdf-owl-kg", Target: "governance", Relation: "governs", Status: "asserted", Layer: "semantic-backbone", Res: "2", Confidence: "0.95", Summary: "Governance attaches to the semantic backbone.", Docs: "PROV-O",
			AIR: map[string]string{"id": "ok", "source": "ok", "target": "ok", "relation": "ok", "status": "ok", "layer": "ok", "res": "ok", "confidence": "ok", "summary": "deny", "docs": "deny"}}},
		Package: grammar.DynamicsPackage{
			Authority:   "CASE-DYN",
			GeneratedAt: "2026-06-18T00:00:00Z",
			PackageHash: "abcdef1234567890",
			DefaultLens: "topology",
			Totals: map[string]int{
				"sources": 9, "artifacts": 1, "nodes": 2, "edges": 1, "claims": 1,
				"observations": 1, "relations": 1, "lenses": 1, "validation": 1,
			},
			Sources: []grammar.DynamicsSource{{
				ID: "claims", Status: "observed", Count: 1, AgeBucket: "<1d", Path: "SECRET_FILE.json", Detail: "claim fragments",
				AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "path": "deny", "detail": "ok"},
			}},
			Validation: []grammar.DynamicsRow{{
				Kind: "validation", ID: "package_gate", Source: "package", Status: "declared", Count: 1, Detail: "command_ref=scripts/system-dynamics-map-gate",
				AIR: map[string]string{"kind": "ok", "id": "ok", "source": "ok", "status": "ok", "count": "ok", "detail": "ok"},
			}},
			Lenses: []grammar.DynamicsRow{{
				Kind: "lens", ID: "topology", Source: "lenses", Status: "lossless", Count: 2, Detail: "label=Topology; mode=topology; layout=cose; edges=1; reversible=true",
				AIR: map[string]string{"kind": "ok", "id": "ok", "source": "ok", "status": "ok", "count": "ok", "detail": "ok"},
			}},
			Claims: []grammar.DynamicsRow{{
				Kind: "claim", ID: "node:asserted", Source: "claims", Status: "architecture_contract", Count: 1, Detail: "freshness=timeless",
				AIR: map[string]string{"kind": "ok", "id": "ok", "source": "ok", "status": "ok", "count": "ok", "detail": "ok"},
			}},
			Observations: []grammar.DynamicsRow{{
				Kind: "observation", ID: "stale_fixture:fixture", Source: "observations", Status: "stale", Count: 1, Detail: "state=stale_fixture; source_type=fixture",
				AIR: map[string]string{"kind": "ok", "id": "ok", "source": "ok", "status": "ok", "count": "ok", "detail": "ok"},
			}},
			Relations: []grammar.DynamicsRow{{
				Kind: "relation", ID: "governance:asserted", Source: "relations", Status: "declared", Count: 1, Detail: "edge_count=1",
				AIR: map[string]string{"kind": "ok", "id": "ok", "source": "ok", "status": "ok", "count": "ok", "detail": "ok"},
			}},
			Workbench: grammar.DynamicsWorkbench{
				Status: "observed",
				Defaults: grammar.DynamicsWorkbenchDefaults{
					InquiryMode: "release-gates", AudienceMode: "operator", ExplanationPath: "release-readiness",
					AIR: map[string]string{"inquiry_mode": "ok", "audience_mode": "ok", "explanation_path": "ok"},
				},
				InquiryModes: []grammar.DynamicsWorkbenchInquiry{{
					ID: "release-gates", Label: "What gates release?", Lens: "operating-slice", Prompt: "SECRET_WORKBENCH_PROMPT",
					AnswerShape: []string{"ordered gate path", "scope caveat"}, FocusNodeIDs: []string{"rdf-owl-kg", "missing-gate"}, FocusEdgeIDs: []string{"kg-to-governance"},
					AIR: map[string]string{"id": "ok", "label": "ok", "lens": "ok", "prompt": "deny", "answer_shape": "ok", "focus_node_ids": "ok", "focus_edge_ids": "ok"},
				}},
				AudienceModes: []grammar.DynamicsWorkbenchAudience{{
					ID: "operator", Label: "Operator", Emphasis: "diagnostic next action",
					AIR: map[string]string{"id": "ok", "label": "ok", "emphasis": "ok"},
				}},
				ExplanationPaths: []grammar.DynamicsWorkbenchExplanation{{
					ID: "release-readiness", Label: "Release readiness path", Summary: "Teach release readiness", MustInclude: []string{"source-neutral identity", "what this does not prove"}, SceneCount: 1,
					Scenes: []grammar.DynamicsWorkbenchScene{{
						Title: "State what this does not prove", Lens: "evidence-risk", SelectionGroup: "nodes", SelectionID: "view-manifest", Takeaway: "SECRET_WORKBENCH_TAKEAWAY", Caveat: "SECRET_WORKBENCH_CAVEAT",
						AIR: map[string]string{"title": "ok", "lens": "ok", "selection_group": "ok", "selection_id": "ok", "takeaway": "deny", "caveat": "deny"},
					}},
					AIR: map[string]string{"id": "ok", "label": "ok", "summary": "ok", "must_include": "ok", "scene_count": "ok"},
				}},
				FollowOnTranches: []string{"bitemporal snapshot registry"},
			},
		},
	}
	m := New("REINS").FoldDynamics(g, false).Exec("dynamics")
	m.Width, m.Height = 180, 140
	v := ansi.Strip(m.View())
	for _, want := range []string{"MAP ORIENTATION", "epistemic", "WORKBENCH", "What gates release?", "Operator", "Release readiness", "INQUIRY READOUT", "active lens", "first gate", "State what this does not prove", "bitemporal snapshot registry", "INSPECTION PATHS", "lens", "inquiry", "reference", "SELECTED MAP ELEMENT", "EPISTEMIC BRIDGE", "exact ref", "map-node", "rdf-owl-kg", "summary", "Canonical semantic backbone", "why", "Identity, state, evidence", "refs", "W3C RDF", "source refs", "rdf.md#concepts", "hardening", "Use named graphs", "tags", "semantic-core", "DYNAMICS PACKAGE", "GRAPH SUMMARY", "visible", "scale path", "▶all", "scale why", "full topology", "central", "flow", "edge status", "SOURCES", "VALIDATION", "LENSES", "CLAIM PARTITIONS", "OBSERVATION STATE", "RELATION VOCABULARY", "GRAPH RAIL"} {
		if !strings.Contains(v, want) {
			t.Fatalf("dynamics package view missing %q:\n%s", want, v)
		}
	}
	if !strings.Contains(v, "\n SELECTED MAP ELEMENT") {
		t.Fatalf("selected map element must begin on its own row:\n%s", v)
	}
	selectedAt := strings.Index(v, "SELECTED MAP ELEMENT")
	railAt := strings.Index(v, "GRAPH RAIL")
	if selectedAt < 0 || railAt < 0 || selectedAt > railAt {
		t.Fatalf("dynamics package view should make selected element precede rail, selected=%d rail=%d:\n%s", selectedAt, railAt, v)
	}
	m.AIR = true
	v = ansi.Strip(m.View())
	if strings.Contains(v, "SECRET_FILE.json") {
		t.Fatalf("AIR must redact source filenames in dynamics package view:\n%s", v)
	}
	for _, leak := range []string{"Canonical semantic backbone", "Identity, state, evidence", "W3C RDF", "Use named graphs", "semantic-core", "SECRET_WORKBENCH_PROMPT", "SECRET_WORKBENCH_TAKEAWAY", "SECRET_WORKBENCH_CAVEAT"} {
		if strings.Contains(v, leak) {
			t.Fatalf("AIR must redact source-derived dynamics explanation field %q:\n%s", leak, v)
		}
	}
}

func TestDynamicsFirstViewportShowsSelectedBridgeBeforeRail(t *testing.T) {
	g := grammar.Graph{
		Thesis: "Faithful rendering of system dynamics requires a source-neutral semantic graph backbone with provenance, temporal state overlays, validation contracts, and reproducible view manifests.",
		Layers: []grammar.Layer{{ID: "semantic-backbone", Label: "Semantic Backbone"}},
		Nodes: []grammar.Node{{
			ID: "rdf-owl-kg", Label: "RDF / OWL Knowledge Graph", Kind: "backbone", Layer: "semantic-backbone", Status: "asserted", Res: "1",
			Summary:        "Canonical semantic backbone for system identity, topology, claims, and cross-source relationships.",
			Context:        "Identity, state, evidence, and authority remain separable.",
			Docs:           "W3C RDF 1.2 Concepts, W3C OWL 2 Overview",
			HardeningNotes: "Use named graphs for asserted, inferred, observed, simulated, and rendered claims.",
			Tags:           "semantic-core, identity",
			AIR: map[string]string{
				"id": "ok", "label": "ok", "kind": "ok", "layer": "ok", "status": "ok", "res": "ok",
				"summary": "ok", "context": "ok", "docs": "ok", "hardening_notes": "ok", "tags": "ok",
			},
		}},
	}
	m := New("REINS").FoldDynamics(g, false).Exec("dynamics")
	m.Width, m.Height, m.AIR = 150, 60, true
	v := ansi.Strip(m.View())
	for _, want := range []string{"SELECTED MAP ELEMENT", "EPISTEMIC BRIDGE", "exact ref", "map-node", "rdf-owl-kg", "GRAPH RAIL"} {
		if !strings.Contains(v, want) {
			t.Fatalf("150x60 dynamics first viewport missing %q:\n%s", want, v)
		}
	}
	bridgeAt := strings.Index(v, "EPISTEMIC BRIDGE")
	railAt := strings.Index(v, "GRAPH RAIL")
	if bridgeAt < 0 || railAt < 0 || bridgeAt > railAt {
		t.Fatalf("dynamics bridge should precede graph rail at 150x60, bridge=%d rail=%d:\n%s", bridgeAt, railAt, v)
	}
	for i, line := range strings.Split(m.View(), "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("150x60 dynamics line %d exceeds width %d: %d %q", i, m.Width, got, line)
		}
	}
}

func TestDynamicsShortWorkbenchViewportStillShowsInspectionAndRail(t *testing.T) {
	g := grammar.Graph{
		Thesis: "Faithful rendering of system dynamics requires a source-neutral semantic graph backbone with provenance, temporal state overlays, validation contracts, and reproducible view manifests.",
		Layers: []grammar.Layer{{ID: "semantic-backbone", Label: "Semantic Backbone"}},
		Nodes: []grammar.Node{{
			ID: "rdf-owl-kg", Label: "RDF / OWL Knowledge Graph", Kind: "backbone", Layer: "semantic-backbone", Status: "asserted", Res: "1",
			Summary: "Canonical semantic backbone for system identity, topology, claims, and cross-source relationships.",
			AIR:     map[string]string{"id": "ok", "label": "ok", "kind": "ok", "layer": "ok", "status": "ok", "res": "ok", "summary": "ok"},
		}},
		Package: grammar.DynamicsPackage{
			Workbench: grammar.DynamicsWorkbench{
				Status: "observed",
				Defaults: grammar.DynamicsWorkbenchDefaults{
					InquiryMode:     "release-gates",
					AudienceMode:    "operator",
					ExplanationPath: "release-readiness",
				},
				InquiryModes: []grammar.DynamicsWorkbenchInquiry{{
					ID: "release-gates", Label: "What gates release?", Lens: "operating-slice", Prompt: "Follow the current intake-to-release path and identify the first non-ready gate.",
					AIR: map[string]string{"id": "ok", "label": "ok", "lens": "ok", "prompt": "ok", "answer_shape": "ok", "focus_node_ids": "ok", "focus_edge_ids": "ok"},
				}},
				AudienceModes: []grammar.DynamicsWorkbenchAudience{{ID: "operator", Label: "Operator", Emphasis: "diagnostic next action", AIR: map[string]string{"id": "ok", "label": "ok", "emphasis": "ok"}}},
				ExplanationPaths: []grammar.DynamicsWorkbenchExplanation{{
					ID: "release-readiness", Label: "Release readiness path", MustInclude: []string{"source-neutral identity", "what this does not prove"}, SceneCount: 1,
					AIR: map[string]string{"id": "ok", "label": "ok", "must_include": "ok", "scene_count": "ok"},
				}},
			},
		},
	}
	m := New("REINS").FoldDynamics(g, false).Exec("dynamics")
	m.Width, m.Height = 120, 40
	v := ansi.Strip(m.View())
	for _, want := range []string{"SELECTED MAP ELEMENT", "bridge", "rdf-owl-kg", "GRAPH RAIL"} {
		if !strings.Contains(v, want) {
			t.Fatalf("120x40 dynamics workbench viewport missing %q:\n%s", want, v)
		}
	}
	selectedAt := strings.Index(v, "SELECTED MAP ELEMENT")
	railAt := strings.Index(v, "GRAPH RAIL")
	if selectedAt < 0 || railAt < 0 || selectedAt > railAt {
		t.Fatalf("120x40 dynamics should put selected inspection before graph rail, selected=%d rail=%d:\n%s", selectedAt, railAt, v)
	}
	for i, line := range strings.Split(m.View(), "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("120x40 dynamics line %d exceeds width %d: %d %q", i, m.Width, got, line)
		}
	}
}

func TestEpistemicsPageRendersDerivedEvidenceAndAIR(t *testing.T) {
	g := grammar.Graph{
		Package: grammar.DynamicsPackage{
			Authority: "CASE-DYN",
			Sources: []grammar.DynamicsSource{{
				ID: "claims", Status: "observed", Count: 2, AgeBucket: "<1d", Path: "SECRET_DYN_SOURCE.json", Privacy: "metadata-only",
				AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "path": "deny", "privacy": "ok", "raw_access": "ok"},
			}},
			Claims: []grammar.DynamicsRow{{
				Kind: "claim", ID: "node:authority", Source: "claims", Status: "asserted", Count: 1, Detail: "source-backed",
				AIR: map[string]string{"kind": "ok", "id": "ok", "source": "ok", "status": "ok", "count": "ok", "detail": "ok"},
			}},
		},
	}
	m := New("REINS").
		FoldDynamics(g, false).
		FoldIntake(grammar.IntakeSummary{Sources: []grammar.IntakeSource{{
			ID: "obsidian", Status: "observed", Count: 3, AgeBucket: "<1h", Privacy: "metadata-only", AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "privacy": "ok"},
		}}}, false).
		FoldDomains(grammar.DomainSummary{Lifecycles: []grammar.LifecycleRow{{
			LifecycleID: "ldlc", Label: "Life Development Lifecycle", State: "candidate", AuthorityCeiling: "local_read", EvidenceCount: 1,
			AIR: map[string]string{"lifecycle_id": "ok", "label": "ok", "state": "ok", "authority_ceiling": "ok", "evidence_count": "ok"},
		}}}, false).
		FoldCapabilities(grammar.CapabilitySummary{
			Sources: []grammar.CapabilitySource{{
				ID: "hkp_bundle:sdlc", Path: "SECRET_HKP_ROOT/sdlc/_hkp/manifest.yaml", Status: "support-only", Count: 7, AgeBucket: "<1h", Detail: "cache-only HKP bundle; concepts=2 edges=3 generated=2026-06-25T18:35:16Z", Privacy: "metadata-only", RawAccess: false,
				AIR: map[string]string{"id": "ok", "path": "deny", "status": "ok", "count": "ok", "age_bucket": "ok", "detail": "ok", "privacy": "ok", "raw_access": "ok"},
			}},
			Rows: []grammar.CapabilityRow{{
				CapabilityID: "fugu-raw", Status: "support-only", Authority: "not-dispatchable", EvidenceCount: 1, Blocker: "route missing",
				AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "evidence_count": "ok", "blocker": "ok"},
			}},
		}, false).
		Exec("epistemics")
	m.Width, m.Height, m.AIR = 150, 60, true

	v := ansi.Strip(m.View())
	for _, want := range []string{
		"EPISTEMICS",
		"derived from existing read folds",
		"POSTURE SUMMARY",
		"families",
		"authority",
		"status hue = observed/support/gap/neutral",
		"evidence count is metadata",
		"SELECTED EVIDENCE PATH",
		"POSTURE ROWS",
		"metadata-only",
		"metadata-only raw=false",
		"ldlc",
		"hkp_bundle:sdlc",
		"cache-only HKP",
		"fugu-raw",
		"not-dispatchable",
		"no raw transcript",
		"vault note body",
		"source body",
		"dispatch authority",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("epistemics page missing %q:\n%s", want, v)
		}
	}
	for _, leak := range []string{"SECRET_DYN_SOURCE.json", "SECRET_HKP_ROOT"} {
		if strings.Contains(v, leak) {
			t.Fatalf("epistemics page must not leak denied source paths under AIR:\n%s", v)
		}
	}

	m = step(m, "j")
	if m.EpiFocus != 1 || !strings.Contains(m.Status, "epistemic row 2/") {
		t.Fatalf("epistemics j should move evidence focus, focus=%d status=%q", m.EpiFocus, m.Status)
	}
}

func TestEpistemicsPreferSourceBackedMapRowsAndExactMapJoin(t *testing.T) {
	g := grammar.Graph{
		Nodes: []grammar.Node{
			{ID: "node-alpha", Label: "Alpha", Status: "asserted", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
			{ID: "node-beta", Label: "Beta", Status: "observed", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
		},
		Edges: []grammar.Edge{{
			Source: "node-alpha", Target: "node-beta", Relation: "depends_on", Status: "observed",
			AIR: map[string]string{"source": "ok", "target": "ok", "relation": "ok", "status": "ok"},
		}},
	}
	ep := grammar.EpistemicsSummary{
		SchemaVersion: "epistemics.read.v1",
		Scope:         "dynamics",
		AuthorityCase: "CASE-DYN",
		Sources: []grammar.EpistemicSource{{
			ID: "seed", Status: "observed", Count: 2, Privacy: "metadata-only", RawAccess: false,
			AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "privacy": "ok", "raw_access": "ok"},
		}},
		Rows: []grammar.EpistemicReadRow{{
			RowID: "map-edge:edge-alpha-beta", Family: "dynamics", SubjectKind: "map-edge", SubjectRef: "edge-alpha-beta",
			Status: "observed", Posture: "source-backed", AuthorityCase: "CASE-DYN", EvidenceCount: 3, Evidence: "source_refs:3",
			Source: "seed", SourceRefs: "seed:3 refs", SourceRefLabels: []string{"edge.md#alpha-beta", "claims.md#edge-alpha-beta", "observations.jsonl#latest"}, Freshness: "2026-06-25T12:00:00Z",
			Privacy: "metadata-only", RawAccess: false, MapKind: "edge", MapID: "edge-alpha-beta", MapSource: "node-alpha", MapTarget: "node-beta", MapRelation: "depends_on",
			AIR: map[string]string{
				"row_id": "ok", "family": "ok", "subject_kind": "ok", "subject_ref": "ok", "status": "ok", "posture": "ok", "authority_case": "ok",
				"evidence_count": "ok", "evidence": "ok", "source": "ok", "source_refs": "ok", "source_ref_labels": "ok", "freshness": "ok",
				"privacy": "ok", "raw_access": "ok", "map_kind": "ok", "map_id": "ok", "map_source": "ok", "map_target": "ok", "map_relation": "ok",
			},
		}},
	}
	m := New("REINS").FoldDynamics(g, false).FoldEpistemics(ep, false).Exec("dynamics")
	m.Width, m.Height = 150, 60
	m = step(step(m, "j"), "j")
	if m.DynFocus != 2 {
		t.Fatalf("setup should focus edge row, got %d", m.DynFocus)
	}
	dynView := ansi.Strip(m.View())
	for _, want := range []string{"EPISTEMIC BRIDGE", "refs", "seed:3 refs", "edge.md#alpha-beta", "claims.md#edge-alpha-beta", "observations.jsonl#latest"} {
		if !strings.Contains(dynView, want) {
			t.Fatalf("dynamics bridge should expose bounded epistemic refs %q:\n%s", want, dynView)
		}
	}
	m = step(m, "enter")
	row, ok := m.FocusedEpistemicRow()
	if m.Page != PageEpistemics || !ok || row.Family != "map-edge" || row.Subject != "edge-alpha-beta" || row.MapSource != "node-alpha" || row.MapTarget != "node-beta" || row.MapRelation != "depends_on" {
		t.Fatalf("dynamics edge should land on source-backed map identity row, page=%d ok=%v row=%+v status=%q", m.Page, ok, row, m.Status)
	}
	if row.SourceRefs != "seed:3 refs" || len(row.SourceRefLabels) != 3 || row.SourceRefLabels[2] != "observations.jsonl#latest" {
		t.Fatalf("source-backed epistemic row should keep bounded source refs, row=%+v", row)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"SELECTED EVIDENCE PATH", "edge-alpha-beta", "source refs", "seed:3 refs", "claims.md#edge-alpha-beta", "POSTURE ROWS", "source-backed"} {
		if !strings.Contains(v, want) {
			t.Fatalf("source-backed epistemics view missing %q:\n%s", want, v)
		}
	}
	localMapRows := 0
	for _, er := range m.epistemicRows() {
		if er.Family == "map-edge" {
			localMapRows++
		}
	}
	if localMapRows != 1 {
		t.Fatalf("source-backed map rows should suppress duplicate local map-edge derivation, got %d rows", localMapRows)
	}
}

func TestEpistemicSourceRefLabelsHonorAIR(t *testing.T) {
	ep := grammar.EpistemicsSummary{
		SchemaVersion: "epistemics.read.v1",
		Scope:         "dynamics",
		Rows: []grammar.EpistemicReadRow{{
			RowID: "map-node:node-alpha", Family: "dynamics", SubjectKind: "map-node", SubjectRef: "node-alpha",
			Status: "observed", Posture: "source-backed", AuthorityCase: "CASE-DYN", EvidenceCount: 2, Evidence: "source_refs:2",
			Source: "seed", SourceRefs: "seed:2 refs", SourceRefLabels: []string{"SECRET_LABEL_SHOULD_NOT_RENDER"}, Freshness: "2026-06-25T12:00:00Z",
			Privacy: "metadata-only", RawAccess: false, MapKind: "node", MapID: "node-alpha",
			AIR: map[string]string{
				"row_id": "ok", "family": "ok", "subject_kind": "ok", "subject_ref": "ok", "status": "ok", "posture": "ok", "authority_case": "ok",
				"evidence_count": "ok", "evidence": "ok", "source": "ok", "source_refs": "ok", "source_ref_labels": "deny", "freshness": "ok",
				"privacy": "ok", "raw_access": "ok", "map_kind": "ok", "map_id": "ok",
			},
		}},
	}
	m := New("REINS").FoldEpistemics(ep, false).Exec("epistemics")
	m.Width, m.Height, m.AIR = 140, 48, true
	v := ansi.Strip(m.View())
	for _, want := range []string{"SELECTED EVIDENCE PATH", "source refs", "seed:2 refs", "metadata-only"} {
		if !strings.Contains(v, want) {
			t.Fatalf("AIR view should keep bounded source count %q:\n%s", want, v)
		}
	}
	for _, leak := range []string{"SECRET_LABEL_SHOULD_NOT_RENDER", "ref labels"} {
		if strings.Contains(v, leak) {
			t.Fatalf("AIR view must suppress denied ref labels %q:\n%s", leak, v)
		}
	}
}

func TestEpistemicsPreferSourceBackedPackageRows(t *testing.T) {
	g := grammar.Graph{
		Package: grammar.DynamicsPackage{
			Claims: []grammar.DynamicsRow{{
				Kind: "claim", ID: "node:asserted", Source: "claims", Status: "architecture_contract", Count: 1, Detail: "local fallback",
				AIR: map[string]string{"kind": "ok", "id": "ok", "source": "ok", "status": "ok", "count": "ok", "detail": "ok"},
			}},
		},
	}
	ep := grammar.EpistemicsSummary{
		SchemaVersion: "epistemics.read.v1",
		Scope:         "dynamics",
		Rows: []grammar.EpistemicReadRow{{
			RowID: "claim:node:asserted", Family: "claim", SubjectKind: "package-row", SubjectRef: "node:asserted",
			Status: "architecture_contract", Posture: "source-backed", AuthorityCase: "CASE-DYN", EvidenceCount: 1, Evidence: "count:1",
			Source: "claims", SourceRefs: "claims:1 records", Freshness: "2026-06-25T12:00:00Z", Privacy: "metadata-only", RawAccess: false,
			Missing: "", Action: "none", Detail: "freshness=timeless", MapKind: "package-row",
			AIR: map[string]string{
				"row_id": "ok", "family": "ok", "subject_kind": "ok", "subject_ref": "ok", "status": "ok", "posture": "ok", "authority_case": "ok",
				"evidence_count": "ok", "evidence": "ok", "source": "ok", "source_refs": "ok", "freshness": "ok", "privacy": "ok", "raw_access": "ok",
				"missing": "ok", "action": "ok", "detail": "ok", "map_kind": "ok",
			},
		}},
	}
	m := New("REINS").FoldDynamics(g, false).FoldEpistemics(ep, false).Exec("epistemics")
	rows := m.epistemicRows()
	claimRows := 0
	for _, row := range rows {
		if row.Family != "claim" {
			continue
		}
		claimRows++
		if row.Subject != "node:asserted" || row.SourceRefs != "claims:1 records" || strings.Contains(row.Detail, "local fallback") {
			t.Fatalf("claim row should use source-backed package row, got %+v", row)
		}
	}
	if claimRows != 1 {
		t.Fatalf("source-backed package rows should suppress duplicate local claim rows, got %d rows in %+v", claimRows, rows)
	}
}

func TestGenericSlackRowsIncludeTrustStripAcrossPages(t *testing.T) {
	m := New("REINS").
		Fold([]grammar.Event{{
			TS: "2026-06-25T12:00:00Z", Kind: "coord_dispatch.launch_started", Subject: "task-1", Actor: "cx", Score: 0.7,
			AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "score": "ok"},
		}}, false).
		FoldTasks([]grammar.Task{{
			TaskID: "task-1", AuthorityCase: "CASE-1", Freshness: 0.9,
			AIR: map[string]string{"task_id": "ok", "authority_case": "ok", "freshness": "ok"},
		}}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.8, RelayAgeS: 120, OutputAgeS: 60,
			AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"},
		}}, false).
		FoldGates(grammar.GateSummary{Sources: []grammar.GateSource{{
			ID: "release-gate", Status: "observed", Count: 1, AgeBucket: "<1h", Privacy: "metadata-only",
			AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "privacy": "ok"},
		}}}, false).
		FoldIntake(grammar.IntakeSummary{
			Sources: []grammar.IntakeSource{{
				ID: "obsidian", Status: "observed", Count: 2, AgeBucket: "<1h", Privacy: "metadata-only",
				AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "privacy": "ok"},
			}},
			Rows: []grammar.IntakeRow{{
				Source: "obsidian", Kind: "observation", Status: "open", Severity: "warn", Count: 2, AgeBucket: "<1h",
				AIR: map[string]string{"source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok", "age_bucket": "ok"},
			}},
		}, false).
		FoldCapabilities(grammar.CapabilitySummary{
			Sources: []grammar.CapabilitySource{{
				ID: "hkp_bundle:sdlc", Status: "support-only", Count: 7, AgeBucket: "<1h", Privacy: "metadata-only",
				AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "privacy": "ok"},
			}},
			Rows: []grammar.CapabilityRow{{
				CapabilityID: "hkp_support_context", Status: "support-only", Authority: "authority-capped", EvidenceCount: 7,
				AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "evidence_count": "ok"},
			}},
		}, false).
		FoldDynamics(grammar.Graph{Package: grammar.DynamicsPackage{
			Authority: "CASE-DYN",
			Sources: []grammar.DynamicsSource{{
				ID: "package", Status: "observed", Count: 1, AgeBucket: "<1h", Privacy: "metadata-only",
				AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "privacy": "ok"},
			}},
		}}, false).
		FoldDomains(grammar.DomainSummary{
			Sources: []grammar.DomainSource{{
				ID: "domain-pack", Status: "observed", Count: 1, AgeBucket: "<1h", Authority: "local_read", Privacy: "metadata-only",
				AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "authority": "ok", "privacy": "ok"},
			}},
			Rows: []grammar.DomainRow{{
				DomainID: "sdlc", AuthorityCeiling: "local_read", EvidenceCount: 1,
				AIR: map[string]string{"domain_id": "ok", "authority_ceiling": "ok", "evidence_count": "ok"},
			}},
		}, false)

	pages := []int{
		PageEvents, PageTasks, PageSessions, PageYard, PageReadiness, PageIntake,
		PageCaps, PageDynamics, PageDomains, PageEpistemics, PageCommands,
		PageWindows, PageSurfaces, PageHelp, PageLegend, PageIntent, PageLifecycles,
	}
	for _, page := range pages {
		m.Page = page
		rows := m.genericSlackRows(120)
		joined := ansi.Strip(strings.Join(rows, "\n"))
		for _, want := range []string{"trust", "auth=", "fresh=", "support=", "recent="} {
			if !strings.Contains(joined, want) {
				t.Fatalf("%s generic slack missing %q:\n%s", pageLabel(page), want, joined)
			}
		}
		for i, row := range rows {
			if got := ansi.StringWidth(row); got > 120 {
				t.Fatalf("%s generic slack row %d exceeds width: %d %q", pageLabel(page), i, got, ansi.Strip(row))
			}
		}
	}
}

func TestDynamicsPageUsesBeatForRailFlowWithoutMovingLabels(t *testing.T) {
	g := grammar.Graph{
		Layers: []grammar.Layer{{ID: "sense", Label: "Sense"}, {ID: "act", Label: "Act"}},
		Nodes: []grammar.Node{
			{ID: "src", Label: "Source", Layer: "sense", Status: "observed", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
			{ID: "dst", Label: "Target", Layer: "act", Status: "asserted", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
		},
		Edges: []grammar.Edge{{Source: "src", Target: "dst", Relation: "feeds", Status: "observed"}},
	}
	m := New("REINS").FoldDynamics(g, false).Exec("dynamics")
	m.Width, m.Height = 160, 42
	m.EventsSeq, m.TasksSeq, m.SessionsSeq, m.IntakeSeq = 1, 1, 1, 1
	m.CapabilitiesSeq, m.GatesSeq, m.DomainsSeq, m.DynamicsSeq = 1, 1, 1, 1
	m.LastFold = "dynamics"

	v0 := ansi.Strip(m.View())
	m.Beat = 1
	v1 := ansi.Strip(m.View())
	if v0 == v1 {
		t.Fatalf("dynamics page should advance the rail flow frame with Beat:\n%s", v0)
	}
	for _, frame := range []string{v0, v1} {
		for _, want := range []string{"SENSE", "src Source", "dst Target", "→", "•"} {
			if !strings.Contains(frame, want) {
				t.Fatalf("dynamics rail frame should preserve topology and labels %q:\n%s", want, frame)
			}
		}
		for i, line := range strings.Split(frame, "\n") {
			if got := ansi.StringWidth(line); got > m.Width {
				t.Fatalf("dynamics frame line %d exceeds width %d: %d %q", i, m.Width, got, line)
			}
		}
	}
}

func TestDynamicsGraphSummaryRespectsAIRTopologyFields(t *testing.T) {
	g := grammar.Graph{
		Layers: []grammar.Layer{
			{ID: "classified_layer", Label: "Secret Layer"},
			{ID: "public_layer", Label: "Public Layer"},
		},
		Nodes: []grammar.Node{
			{ID: "alpha", Label: "Alpha", Layer: "classified_layer", Status: "observed",
				AIR: map[string]string{"id": "ok", "label": "ok", "layer": "deny", "status": "ok"}},
			{ID: "beta", Label: "Beta", Layer: "public_layer", Status: "asserted",
				AIR: map[string]string{"id": "ok", "label": "ok", "layer": "ok", "status": "ok"}},
			{ID: "hidden-endpoint", Label: "Hidden", Layer: "classified_layer", Status: "observed",
				AIR: map[string]string{"id": "ok", "label": "ok", "layer": "deny", "status": "ok"}},
		},
		Edges: []grammar.Edge{
			{Source: "alpha", Target: "beta", Relation: "feeds", Status: "observed",
				AIR: map[string]string{"source": "ok", "target": "ok", "relation": "ok", "status": "ok"}},
			{Source: "hidden-endpoint", Target: "beta", Relation: "hidden", Status: "asserted",
				AIR: map[string]string{"source": "deny", "target": "ok", "relation": "deny", "status": "ok"}},
		},
	}
	m := New("REINS").FoldDynamics(g, true)
	m.AIR = true

	v := ansi.Strip(m.renderDynamicsGraphSummary(140, 0))
	for _, leak := range []string{"classified_layer", "Secret Layer", "hidden-endpoint degree"} {
		if strings.Contains(v, leak) {
			t.Fatalf("AIR graph summary leaked %q:\n%s", leak, v)
		}
	}
	for _, want := range []string{"alpha degree=1", "layer=▒▒▒", "hidden relation:1"} {
		if !strings.Contains(v, want) {
			t.Fatalf("AIR graph summary missing %q:\n%s", want, v)
		}
	}
}

func TestDynamicsCursorJumpsToEpistemicNodeClaim(t *testing.T) {
	g := grammar.Graph{
		Layers: []grammar.Layer{{ID: "L", Label: "Layer"}},
		Nodes: []grammar.Node{{
			ID: "policy", Label: "Policy", Layer: "L", Status: "asserted", Res: "1",
			AIR: map[string]string{"id": "ok", "label": "ok", "layer": "ok", "status": "ok", "res": "ok"},
		}},
		Package: grammar.DynamicsPackage{
			Claims: []grammar.DynamicsRow{{
				Kind: "claim", ID: "node:asserted", Source: "claims", Status: "architecture_contract", Count: 1, Detail: "freshness=timeless",
				AIR: map[string]string{"kind": "ok", "id": "ok", "source": "ok", "status": "ok", "count": "ok", "detail": "ok"},
			}},
		},
	}
	m := New("REINS").FoldDynamics(g, false).Exec("dynamics")
	m.Width, m.Height = 150, 60

	v := ansi.Strip(m.View())
	for _, want := range []string{"SELECTED MAP ELEMENT", "policy", "[E]/[Enter] epistemics"} {
		if !strings.Contains(v, want) {
			t.Fatalf("dynamics selected element card missing %q:\n%s", want, v)
		}
	}
	m = step(m, "E")
	if m.Page != PageEpistemics {
		t.Fatalf("E on dynamics focus should open epistemics, got page %d status=%q", m.Page, m.Status)
	}
	row, ok := m.FocusedEpistemicRow()
	if !ok || row.Family != "map-node" || row.Subject != "policy" {
		t.Fatalf("dynamics node focus should land on exact map node row, got ok=%v row=%+v status=%q", ok, row, m.Status)
	}
}

func TestDynamicsCursorJumpsToEpistemicRelationRow(t *testing.T) {
	g := grammar.Graph{
		Layers: []grammar.Layer{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
		Nodes: []grammar.Node{
			{ID: "src", Label: "Source", Layer: "a", Status: "observed", Res: "1", AIR: map[string]string{"id": "ok", "label": "ok", "layer": "ok", "status": "ok", "res": "ok"}},
			{ID: "dst", Label: "Dest", Layer: "b", Status: "observed", Res: "1", AIR: map[string]string{"id": "ok", "label": "ok", "layer": "ok", "status": "ok", "res": "ok"}},
		},
		Edges: []grammar.Edge{{
			Source: "src", Target: "dst", Relation: "governance", Status: "asserted",
			AIR: map[string]string{"source": "ok", "target": "ok", "relation": "ok", "status": "ok"},
		}},
		Package: grammar.DynamicsPackage{
			Relations: []grammar.DynamicsRow{{
				Kind: "relation", ID: "governance:asserted", Source: "relations", Status: "declared", Count: 1, Detail: "edge_count=1",
				AIR: map[string]string{"kind": "ok", "id": "ok", "source": "ok", "status": "ok", "count": "ok", "detail": "ok"},
			}},
		},
	}
	m := New("REINS").FoldDynamics(g, false).Exec("dynamics")
	m.Width, m.Height = 150, 60
	m = step(step(m, "j"), "j")
	if m.DynFocus != 2 || !strings.Contains(m.Status, "dynamics edge 3/3") {
		t.Fatalf("dynamics j should move focus through nodes to the edge, focus=%d status=%q", m.DynFocus, m.Status)
	}
	m = step(m, "enter")
	row, ok := m.FocusedEpistemicRow()
	if m.Page != PageEpistemics || !ok || row.Family != "map-edge" || row.Subject != "src->dst:governance" {
		t.Fatalf("dynamics edge focus should land on exact map edge row, page=%d ok=%v row=%+v status=%q", m.Page, ok, row, m.Status)
	}
}

func TestDynamicsSelectedElementRespectsAIR(t *testing.T) {
	g := grammar.Graph{
		Layers: []grammar.Layer{{ID: "secret-layer", Label: "Secret Layer"}},
		Nodes: []grammar.Node{{
			ID: "secret-node", Label: "Secret Label", Layer: "secret-layer", Status: "asserted", Res: "1",
			AIR: map[string]string{"id": "deny", "label": "deny", "layer": "deny", "status": "ok", "res": "ok"},
		}},
	}
	m := New("REINS").FoldDynamics(g, false).Exec("dynamics")
	m.Width, m.Height, m.AIR = 130, 44, true

	v := ansi.Strip(m.View())
	for _, leak := range []string{"secret-node", "Secret Label", "secret-layer"} {
		if strings.Contains(v, leak) {
			t.Fatalf("dynamics selected element leaked AIR-denied %q:\n%s", leak, v)
		}
	}
	for _, want := range []string{"SELECTED MAP ELEMENT", "▒▒▒", "status      asserted"} {
		if !strings.Contains(v, want) {
			t.Fatalf("AIR-safe selected element missing %q:\n%s", want, v)
		}
	}
	for _, want := range []string{"EPISTEMIC BRIDGE", "no direct row currently matches"} {
		if !strings.Contains(v, want) {
			t.Fatalf("AIR-safe selected element should expose bridge/gap without leaking %q:\n%s", want, v)
		}
	}
}

func TestDynamicsCompactHeightStillShowsBridgeSummary(t *testing.T) {
	g := grammar.Graph{
		Layers: []grammar.Layer{{ID: "L", Label: "Layer"}},
		Nodes: []grammar.Node{{
			ID: "policy", Label: "Policy", Layer: "L", Status: "asserted", Res: "1",
			AIR: map[string]string{"id": "ok", "label": "ok", "layer": "ok", "status": "ok", "res": "ok"},
		}},
		Package: grammar.DynamicsPackage{
			Claims: []grammar.DynamicsRow{{
				Kind: "claim", ID: "node:asserted", Source: "claims", Status: "architecture_contract", Count: 1, Detail: "freshness=timeless",
				AIR: map[string]string{"kind": "ok", "id": "ok", "source": "ok", "status": "ok", "count": "ok", "detail": "ok"},
			}},
		},
	}
	m := New("REINS").FoldDynamics(g, false).Exec("dynamics")
	m.Width, m.Height = 120, 38
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "bridge") || !strings.Contains(v, "map-node") || !strings.Contains(v, "policy") || !strings.Contains(v, "exact map reference") {
		t.Fatalf("short dynamics view should keep compact bridge evidence visible:\n%s", v)
	}
	if strings.Contains(v, "EPISTEMIC BRIDGE") {
		t.Fatalf("short dynamics view should use compact bridge, not full bridge block:\n%s", v)
	}
	selectedAt := strings.Index(v, "SELECTED MAP ELEMENT")
	railAt := strings.Index(v, "GRAPH RAIL")
	if selectedAt < 0 || railAt < 0 || selectedAt > railAt {
		t.Fatalf("short dynamics view should put selected target before graph rail, selected=%d rail=%d:\n%s", selectedAt, railAt, v)
	}
	for i, line := range strings.Split(m.View(), "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("compact bridge line %d exceeds width %d: %d %q", i, m.Width, got, line)
		}
	}
}

func TestDynamicsSelectedEdgeRendersExplanationMetadata(t *testing.T) {
	g := grammar.Graph{
		Layers: []grammar.Layer{{ID: "L", Label: "Layer"}},
		Nodes: []grammar.Node{
			{ID: "src", Label: "Source", Layer: "L", Status: "asserted", Res: "1", AIR: map[string]string{"id": "ok", "label": "ok", "layer": "ok", "status": "ok", "res": "ok"}},
			{ID: "dst", Label: "Target", Layer: "L", Status: "observed", Res: "1", AIR: map[string]string{"id": "ok", "label": "ok", "layer": "ok", "status": "ok", "res": "ok"}},
		},
		Edges: []grammar.Edge{{
			ID: "src-to-dst", Source: "src", Target: "dst", Relation: "feeds", Status: "observed", Layer: "runtime", Res: "4",
			Confidence: "0.72", Summary: "Source feeds target during runtime.", Docs: "Trace Context", SourceRefs: "docs:1 refs", SourceRefLabels: []string{"trace.md#runtime"},
			AIR: map[string]string{"id": "ok", "source": "ok", "target": "ok", "relation": "ok", "status": "ok", "layer": "ok", "res": "ok", "confidence": "ok", "summary": "deny", "docs": "deny", "source_refs": "ok", "source_ref_labels": "deny"},
		}},
	}
	m := New("REINS").FoldDynamics(g, false).Exec("dynamics")
	m.Width, m.Height = 130, 56
	m = m.dynamicsFocusTo(2)
	v := ansi.Strip(m.View())
	for _, want := range []string{"SELECTED MAP ELEMENT", "edge", "src-to-dst", "relation", "feeds", "confidence", "0.72", "summary", "Source feeds target", "refs", "Trace Context", "source refs", "trace.md#runtime", "NEIGHBORHOOD", "path", "src Source -> feeds -> dst Target", "source node", "target node"} {
		if !strings.Contains(v, want) {
			t.Fatalf("selected edge should render explanation metadata %q:\n%s", want, v)
		}
	}
	m.AIR = true
	v = ansi.Strip(m.View())
	for _, leak := range []string{"Source feeds target", "Trace Context", "trace.md#runtime"} {
		if strings.Contains(v, leak) {
			t.Fatalf("AIR must redact source-derived edge explanation field %q:\n%s", leak, v)
		}
	}
}

func TestDynamicsSelectedNodeShowsNeighborhoodAndFocusedRail(t *testing.T) {
	g := grammar.Graph{
		Layers: []grammar.Layer{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
		Nodes: []grammar.Node{
			{ID: "src", Label: "Source", Layer: "a", Status: "observed", Res: "1", AIR: map[string]string{"id": "ok", "label": "ok", "layer": "ok", "status": "ok", "res": "ok"}},
			{ID: "dst", Label: "Dest", Layer: "b", Status: "asserted", Res: "1", AIR: map[string]string{"id": "ok", "label": "ok", "layer": "ok", "status": "ok", "res": "ok"}},
		},
		Edges: []grammar.Edge{{
			Source: "src", Target: "dst", Relation: "feeds", Status: "observed", Res: "1",
			AIR: map[string]string{"source": "ok", "target": "ok", "relation": "ok", "status": "ok", "res": "ok"},
		}},
	}
	m := New("REINS").FoldDynamics(g, false).Exec("dynamics")
	m.Width, m.Height = 150, 60
	v := ansi.Strip(m.View())
	for _, want := range []string{"GRAPH RAIL", "▶src", "SELECTED MAP ELEMENT", "NEIGHBORHOOD", "degree", "in:0 · out:1", "feeds -> dst Dest"} {
		if !strings.Contains(v, want) {
			t.Fatalf("selected node should expose focused rail and local topology %q:\n%s", want, v)
		}
	}
}

func TestSplitDynamicsKeepsLaneAnchorAndMovesMapTarget(t *testing.T) {
	m := New("REINS").
		FoldSessions([]grammar.Session{
			{Role: "cx-one", Platform: "codex", State: "active", Readiness: "claim", Attention: 0.88, AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "attention": "ok"}},
			{Role: "cx-two", Platform: "codex", State: "active", Readiness: "claim", Attention: 0.77, AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "attention": "ok"}},
		}, false).
		FoldDynamics(grammar.Graph{
			Layers: []grammar.Layer{{ID: "L", Label: "Layer"}},
			Nodes: []grammar.Node{
				{ID: "node-a", Label: "A", Layer: "L", Status: "asserted", Res: "1", AIR: map[string]string{"id": "ok", "label": "ok", "layer": "ok", "status": "ok", "res": "ok"}},
				{ID: "node-b", Label: "B", Layer: "L", Status: "observed", Res: "1", AIR: map[string]string{"id": "ok", "label": "ok", "layer": "ok", "status": "ok", "res": "ok"}},
			},
			Package: grammar.DynamicsPackage{
				Claims: []grammar.DynamicsRow{{
					Kind: "claim", ID: "node:observed", Source: "claims", Status: "observed", Count: 1, Detail: "observed support",
					AIR: map[string]string{"kind": "ok", "id": "ok", "source": "ok", "status": "ok", "count": "ok", "detail": "ok"},
				}},
			},
		}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 180, 44, PageDynamics, true
	beforeDyn := m.DynFocus
	m = step(m, "j")
	if m.SFocus != 1 || m.DynFocus != beforeDyn {
		t.Fatalf("split dynamics j should move lane anchor only, SFocus=%d DynFocus=%d", m.SFocus, m.DynFocus)
	}
	m = step(m, "n")
	if m.SFocus != 1 || m.DynFocus != beforeDyn+1 || !strings.Contains(m.Status, "dynamics node 2/") {
		t.Fatalf("split dynamics n should move right-pane map target only, SFocus=%d DynFocus=%d status=%q", m.SFocus, m.DynFocus, m.Status)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"lane anchor active", "[n/p] map", "map focus", "[j/k]anchor", "node-b", "REFERENCE PATHS", "epistemics", "exact map row", "[E] opens evidence path", "EPISTEMIC BRIDGE", "map-node", "exact ref"} {
		if !strings.Contains(v, want) {
			t.Fatalf("split dynamics should advertise lane anchor plus map target control %q:\n%s", want, v)
		}
	}
	for i, line := range strings.Split(v, "\n") {
		if got := ansi.StringWidth(line); got > m.Width {
			t.Fatalf("split dynamics line %d exceeds frame width %d: %d %q", i, m.Width, got, line)
		}
	}
	if strings.Contains(v, "[j/k] focus") {
		t.Fatalf("split dynamics should suppress target focus controls:\n%s", v)
	}
	if strings.Contains(v, "[E]/[Enter]") {
		t.Fatalf("split dynamics should not advertise Enter for target evidence navigation:\n%s", v)
	}
}

func TestSplitEpistemicsKeepsLaneAnchorAndMovesEvidenceTarget(t *testing.T) {
	m := New("REINS").
		FoldSessions([]grammar.Session{
			{Role: "cx-one", Platform: "codex", State: "active", Readiness: "claim", Attention: 0.88, AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "attention": "ok"}},
			{Role: "cx-two", Platform: "codex", State: "active", Readiness: "claim", Attention: 0.77, AIR: map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "attention": "ok"}},
		}, false).
		FoldDynamics(grammar.Graph{
			Package: grammar.DynamicsPackage{
				Sources: []grammar.DynamicsSource{{
					ID: "claims", Status: "observed", Count: 2, AgeBucket: "<1h", Privacy: "metadata-only",
					AIR: map[string]string{"id": "ok", "status": "ok", "count": "ok", "age_bucket": "ok", "privacy": "ok", "raw_access": "ok"},
				}},
				Claims: []grammar.DynamicsRow{{
					Kind: "claim", ID: "node:authority", Source: "claims", Status: "asserted", Count: 1, Detail: "source-backed",
					AIR: map[string]string{"kind": "ok", "id": "ok", "source": "ok", "status": "ok", "count": "ok", "detail": "ok"},
				}},
			},
		}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 180, 44, PageEpistemics, true
	beforeEpi := m.EpiFocus
	m = step(m, "j")
	if m.SFocus != 1 || m.EpiFocus != beforeEpi {
		t.Fatalf("split epistemics j should move lane anchor only, SFocus=%d EpiFocus=%d", m.SFocus, m.EpiFocus)
	}
	m = step(m, "n")
	if m.SFocus != 1 || m.EpiFocus != beforeEpi+1 || !strings.Contains(m.Status, "epistemic row 2/") {
		t.Fatalf("split epistemics n should move right-pane evidence target only, SFocus=%d EpiFocus=%d status=%q", m.SFocus, m.EpiFocus, m.Status)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"SELECTED EVIDENCE PATH", "[n/p] evidence", "node:authority"} {
		if !strings.Contains(v, want) {
			t.Fatalf("split epistemics should pin evidence target and advertise controls %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "[j/k] moves the row") {
		t.Fatalf("split epistemics should not claim j/k moves the evidence row:\n%s", v)
	}
}

func TestDynamicsScaleCyclesOnPage(t *testing.T) {
	g := grammar.Graph{
		Layers: []grammar.Layer{{ID: "L", Label: "Backbone"}},
		Nodes: []grammar.Node{
			{ID: "overview-node", Label: "overview", Layer: "L", Status: "asserted", Res: "1", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
			{ID: "evidence-node", Label: "evidence", Layer: "L", Status: "asserted", Res: "5", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
		},
	}
	m := New("REINS").FoldDynamics(g, false)
	m.Width, m.Height, m.Page = 160, 40, PageDynamics
	m = step(m, ".")
	if m.Page != PageDynamics || m.DynScale != 1 || !strings.Contains(m.Status, "@overview") {
		t.Fatalf(". should cycle dynamics scale to overview in place: page=%d scale=%d status=%q", m.Page, m.DynScale, m.Status)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "scale overview") || strings.Contains(v, "evidence-node") {
		t.Fatalf("overview scale should be visible and filter higher-res nodes:\n%s", v)
	}
	m = step(m, ",")
	if m.DynScale != 0 || !strings.Contains(m.Status, "@all") {
		t.Fatalf(", should cycle dynamics scale back to all: scale=%d status=%q", m.DynScale, m.Status)
	}
}

func TestSplitDynamicsAllScaleRendersDomainFitInNarrowContext(t *testing.T) {
	g := grammar.Graph{
		Layers: []grammar.Layer{{ID: "L", Label: "Backbone"}},
		Nodes: []grammar.Node{
			{ID: "overview-node", Label: "overview", Layer: "L", Status: "asserted", Res: "1", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
			{ID: "deep-node", Label: "deep", Layer: "L", Status: "asserted", Res: "5", AIR: map[string]string{"id": "ok", "label": "ok", "status": "ok"}},
		},
	}
	m := New("REINS").FoldDynamics(g, false)
	m.Width, m.Height, m.Page, m.SplitContext, m.DynScale = 180, 40, PageDynamics, true, 0
	v := ansi.Strip(m.referenceContent(100))
	if !strings.Contains(v, "scale all→domain fit") {
		t.Fatalf("narrow split dynamics should label the fit scale:\n%s", v)
	}
	if strings.Contains(v, "deep-node") {
		t.Fatalf("narrow split all-scale fit should omit deep nodes from first graph rail:\n%s", v)
	}
	slack := strings.Join(m.dynamicsSlackRows(100), "\n")
	if !strings.Contains(slack, "nodes:1") || !strings.Contains(slack, "scale:all→domain fit") {
		t.Fatalf("dynamics slack should use the same fitted scale as the narrow split graph:\n%s", slack)
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

func TestFoldTracesClampsTFocus(t *testing.T) {
	three := []grammar.Trace{{TraceID: "a"}, {TraceID: "b"}, {TraceID: "c"}}
	// empty fold zeroes the cursor
	m := New("REINS").FoldTraces(nil, false)
	if m.TFocus != 0 {
		t.Fatalf("empty fold should zero TFocus, got %d", m.TFocus)
	}
	// an out-of-range cursor clamps to the last row
	m.TFocus = 99
	m = m.FoldTraces(three, false)
	if m.TFocus != 2 {
		t.Fatalf("TFocus should clamp to last row (2), got %d", m.TFocus)
	}
	// a negative cursor clamps up to 0
	m.TFocus = -1
	m = m.FoldTraces(three, false)
	if m.TFocus != 0 {
		t.Fatalf("TFocus should clamp up to 0, got %d", m.TFocus)
	}
	// pure: folding the same data twice from the same cursor yields the same TFocus
	m.TFocus = 1
	if a, b := m.FoldTraces(three, false).TFocus, m.FoldTraces(three, false).TFocus; a != b {
		t.Fatalf("FoldTraces must be pure in the cursor: %d vs %d", a, b)
	}
}

func TestFoldTracesAdvancesSeqAndLastFold(t *testing.T) {
	m := New("REINS")
	before := m.TracesSeq
	nm, _ := m.Update(TracesMsg{Traces: []grammar.Trace{{TraceID: "t1"}}, Dark: false})
	m = nm.(Model)
	if m.TracesSeq != before+1 {
		t.Fatalf("TracesMsg should advance TracesSeq %d -> %d, got %d", before, before+1, m.TracesSeq)
	}
	if m.LastFold != "traces" {
		t.Fatalf("TracesMsg should set LastFold=traces, got %q", m.LastFold)
	}
	if len(m.Traces) != 1 || m.Traces[0].TraceID != "t1" {
		t.Fatalf("TracesMsg should fold the rows: %+v", m.Traces)
	}
}

func TestTracesPageRendersRowValuesAndContext(t *testing.T) {
	m := New("REINS").FoldTraces([]grammar.Trace{
		{TS: "2026-06-26T12:00:00Z", TraceID: "trace-9", Model: "claude-opus-4",
			PromptTok: 100, CompletionTok: 50, TotalTok: 150, Cost: 0.012345, LatencyMs: 2500,
			AIR: map[string]string{"ts": "ok", "trace_id": "ok", "model": "ok", "latency_ms": "ok", "cost": "ok", "total_tok": "ok"}},
	}, false)
	m.Page = PageTraces
	m.Width, m.Height = 120, 40
	v := m.View()
	for _, want := range []string{"trace-9", "claude-opus-4", "2500ms", "$0.012345"} {
		if !strings.Contains(v, want) {
			t.Fatalf("traces page should render row values (%s):\n%s", want, v)
		}
	}
}

func TestTracesWindowDefRegistered(t *testing.T) {
	var def WindowDef
	found := false
	keys := map[string]bool{}
	for _, w := range windowRegistry {
		if w.Key != "" {
			if keys[w.Key] {
				t.Fatalf("duplicate window Key %q in registry", w.Key)
			}
			keys[w.Key] = true
		}
		if w.Page == PageTraces {
			def = w
			found = true
		}
	}
	if !found {
		t.Fatal("PageTraces must have a registered WindowDef (else it is unreachable in the tab cycle)")
	}
	if def.ID != "traces" || def.Key == "" || def.Scope == "" || def.Lifecycle == "" || def.Kind == "" {
		t.Fatalf("traces WindowDef must populate all fields: %+v", def)
	}
}

func TestTracesSplitPairIsReferenceNotLinked(t *testing.T) {
	pair, ok := splitPairForPage(PageTraces)
	if !ok {
		t.Fatal("PageTraces must declare a SplitPairDef (traces contextualizes LLM spend, not a lane-derived stream)")
	}
	if pair.Mode == splitModeLinked {
		t.Fatalf("traces split must be reference, not linked — traces are not lane-derived: %+v", pair)
	}
}
