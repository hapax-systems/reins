package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/hapax-systems/reins/internal/grammar"
	"github.com/muesli/termenv"
)

func TestAirDeniedDerivedHueCellsRenderMuted(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	cases := []struct {
		name     string
		mutate   func(*Model)
		render   func(Model) string
		badToken string
		cell     string
	}{
		{
			name:     "selected yard lane role",
			mutate:   func(m *Model) { m.Sessions[0].AIR["role"] = "deny" },
			render:   func(m Model) string { return m.renderSelectedYardLane(140) },
			badToken: grammar.LaneToken("alpha"),
			cell:     "▒▒▒",
		},
		{
			name:     "selected yard lane readiness",
			mutate:   func(m *Model) { m.Sessions[0].AIR["readiness"] = "deny" },
			render:   func(m Model) string { return m.renderSelectedYardLane(140) },
			badToken: readinessPaneToken("claim"),
			cell:     "▒▒▒",
		},
		{
			name:     "selected yard lane state",
			mutate:   func(m *Model) { m.Sessions[0].AIR["state"] = "deny" },
			render:   func(m Model) string { return m.renderSelectedYardLane(140) },
			badToken: sessionStateToken("offline"),
			cell:     "▒▒▒",
		},
		{
			name:     "selected yard lane attention",
			mutate:   func(m *Model) { m.Sessions[0].AIR["attention"] = "deny" },
			render:   func(m Model) string { return m.renderSelectedYardLane(140) },
			badToken: attentionToken(0.92),
			cell:     "▒▒▒",
		},
		{
			name:     "selected yard lane blocker",
			mutate:   func(m *Model) { m.Sessions[0].AIR["blocker"] = "deny" },
			render:   func(m Model) string { return m.renderSelectedYardLane(140) },
			badToken: blockerToken("release-blocked"),
			cell:     "▒▒▒",
		},
		{
			name:     "linked task criticality colors stage",
			mutate:   func(m *Model) { m.Tasks[0].AIR["criticality"] = "deny" },
			render:   func(m Model) string { return m.renderSelectedYardLane(140) },
			badToken: grammar.SeverityToken("crit"),
			cell:     "S7",
		},
		{
			name:     "linked task next stage",
			mutate:   func(m *Model) { m.Tasks[0].AIR["predicted_stage"] = "deny" },
			render:   func(m Model) string { return m.renderSelectedYardLane(140) },
			badToken: nextToken("hold"),
			cell:     "▒▒▒",
		},
		{
			name:     "task work owner",
			mutate:   func(m *Model) { m.Tasks[0].AIR["owner"] = "deny" },
			render:   func(m Model) string { return m.taskWorkDomainPane(140) },
			badToken: grammar.LaneToken("alpha"),
			cell:     "▒▒▒",
		},
		{
			name:     "task work criticality colors stage",
			mutate:   func(m *Model) { m.Tasks[0].AIR["criticality"] = "deny" },
			render:   func(m Model) string { return m.taskWorkDomainPane(140) },
			badToken: grammar.SeverityToken("crit"),
			cell:     "S7",
		},
		{
			name:     "task work next stage",
			mutate:   func(m *Model) { m.Tasks[0].AIR["predicted_stage"] = "deny" },
			render:   func(m Model) string { return m.taskWorkDomainPane(140) },
			badToken: nextToken("hold"),
			cell:     "▒▒▒",
		},
		{
			name:     "task work freshness",
			mutate:   func(m *Model) { m.Tasks[0].AIR["freshness"] = "deny" },
			render:   func(m Model) string { return m.taskWorkDomainPane(140) },
			badToken: freshnessToken(0.91),
			cell:     "▒▒▒",
		},
		{
			name:     "session constraint blocker",
			mutate:   func(m *Model) { m.Sessions[0].AIR["blocker"] = "deny" },
			render:   func(m Model) string { return m.sessionConstraintPane(140) },
			badToken: blockerToken("release-blocked"),
			cell:     "▒▒▒",
		},
		{
			name:     "event context actor",
			mutate:   func(m *Model) { m.Events[0].AIR["actor"] = "deny" },
			render:   func(m Model) string { return m.eventContextPane(140) },
			badToken: grammar.LaneToken("alpha"),
			cell:     "▒▒▒",
		},
		{
			name:     "intake severity",
			mutate:   func(m *Model) { m.Intake.Rows[0].AIR["severity"] = "deny" },
			render:   func(m Model) string { return m.renderSelectedIntakeBucket(140) },
			badToken: intakeSeverityToken("crit"),
			cell:     "▒▒▒",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := airHueCanaryModel()
			tc.mutate(&m)
			got := tc.render(m)
			if want := grammar.C("mut", tc.cell); !strings.Contains(got, want) {
				t.Fatalf("denied field cell was not rendered with muted hue; want cell %q in:\n%q", want, got)
			}
			if bad := grammar.C(tc.badToken, tc.cell); tc.badToken != "mut" && strings.Contains(got, bad) {
				t.Fatalf("denied field cell leaked original derived hue %q via %q in:\n%q", tc.badToken, bad, got)
			}
		})
	}
}

func airHueCanaryModel() Model {
	sessAIR := map[string]string{
		"role": "ok", "session": "ok", "platform": "ok", "state": "ok", "readiness": "ok",
		"blocker": "ok", "attention": "ok", "claimed_task": "ok", "route_id": "ok",
		"route_binding_state": "ok", "route_evidence_ref": "ok",
	}
	taskAIR := map[string]string{
		"task_id": "ok", "stage": "ok", "prior_stage": "ok", "predicted_stage": "ok",
		"owner": "ok", "criticality": "ok", "freshness": "ok", "rel_count": "ok",
		"authority_case": "ok", "no_go": "ok",
	}
	eventAIR := map[string]string{
		"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok", "summary": "ok", "score": "ok",
	}
	intakeAIR := map[string]string{
		"id": "ok", "source": "ok", "kind": "ok", "status": "ok", "severity": "ok", "count": "ok",
		"blocker": "ok", "coverage": "ok", "task_link_state": "ok", "authority": "ok", "evidence": "ok",
		"missing": "ok", "action": "ok", "detail": "ok", "source_refs": "ok", "next_evidence": "ok",
	}

	sess := grammar.Session{
		Role: "alpha", Platform: "linux", State: "offline", Readiness: "claim", Blocker: "release-blocked",
		Attention: 0.92, ClaimedTask: "TASK-1", AIR: sessAIR,
	}
	task := grammar.Task{
		TaskID: "TASK-1", Stage: "S7", PriorStage: "S6", PredictedStage: "hold", Owner: "alpha",
		Criticality: "crit", Freshness: 0.91, RelCount: 1, AIR: taskAIR,
	}
	ev := grammar.Event{
		TS: "2026-06-27T12:00:00Z", Kind: "deploy.fail", Subject: "subject", Actor: "alpha",
		Summary: "failed", Score: 0.9, AIR: eventAIR,
	}
	intake := grammar.IntakeSummary{
		Rows: []grammar.IntakeRow{{
			ID: "intake-1", Source: "source", Kind: "bucket", Status: "attention", Severity: "crit",
			Count: 1, Blocker: "release-blocked", Coverage: "metadata", TaskLinkState: "linked",
			Authority: "observation", Evidence: "evidence", Missing: "missing", Action: "action",
			Detail: "detail", SourceRefs: "source", NextEvidence: "next", AIR: intakeAIR,
		}},
	}
	m := New("REINS").FoldTasks([]grammar.Task{task}, false).FoldSessions([]grammar.Session{sess}, false).Fold([]grammar.Event{ev}, false).FoldIntake(intake, false)
	m.AIR, m.Width, m.Height = true, 160, 48
	return m
}
