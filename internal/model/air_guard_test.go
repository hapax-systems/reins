package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// TestNoAirDeniedValueEverLeaks is the STRUCTURAL on-air guard (makes the PII-leak class impossible,
// not just the one instance the live smoke found). It plants a unique sentinel in every PRIVATE field
// of a task + an event, marks them ALL denied, renders every on-air surface (rows · rail · /whois
// door · events), and asserts NONE of the sentinels survive in the rendered frame. A new surface that
// forgets to gate a field fails here, on-air, before it can ever air. The redaction token ▒▒▒ must be
// present (structure kept; value gone).
func TestNoAirDeniedValueEverLeaks(t *testing.T) {
	const (
		idS, ownS, caseS, nogoS = "LEAKID", "LEAKOWNER", "LEAKCASE", "LEAKNOGO"
		subjS, actorS, summS    = "LEAKSUBJECT", "LEAKACTOR", "LEAKSUMMARY"
		roleS, sessS, platS     = "LEAKROLE", "LEAKSESSION", "LEAKPLATFORM"
		rdyS, blockS, claimS    = "LEAKREADY", "LEAKBLOCK", "LEAKCLAIM"
		pathS, specS            = "LEAKPATH", "LEAKSPEC"
	)
	denyAll := func(keys ...string) map[string]string {
		m := map[string]string{}
		for _, k := range keys {
			m[k] = "deny"
		}
		return m
	}
	task := grammar.Task{
		TaskID: idS, Owner: ownS, AuthorityCase: caseS, NoGo: nogoS,
		Stage: "S7", PriorStage: "S6", PredictedStage: "hold", Criticality: "crit",
		AIR: denyAll("task_id", "owner", "authority_case", "no_go",
			"stage", "prior_stage", "predicted_stage", "criticality", "freshness", "rel_count"),
	}
	ev := grammar.Event{
		TS: "2026-06-24T10:00:00", Kind: "task.updated", Subject: subjS, Actor: actorS, Summary: summS,
		AIR: denyAll("subject", "actor", "summary"),
	}
	sess := grammar.Session{
		Role: roleS, Session: sessS, Platform: platS, State: "active", Readiness: rdyS, Blocker: blockS,
		Attention: 0.77, Alive: true, ClaimedTask: claimS,
		OutputAgeS: 9, RelayAgeS: 10,
		AIR: denyAll("role", "session", "platform", "state", "readiness", "blocker", "attention", "alive", "idle", "stalled", "claimed_task", "output_age_s", "relay_age_s"),
	}
	intake := grammar.IntakeSummary{
		Sources: []grammar.IntakeSource{{
			ID:        "request_state",
			Path:      pathS,
			Status:    "observed",
			Count:     1,
			AgeBucket: "<5m",
			Privacy:   "metadata-only",
			AIR:       denyAll("path"),
		}},
		Rows: []grammar.IntakeRow{{
			Source:        "request_state",
			ID:            "SECRET-INTAKE-ID",
			Kind:          subjS,
			Status:        "attention",
			Severity:      "warn",
			Count:         1,
			Blocker:       blockS,
			Coverage:      summS,
			TaskLinkState: claimS,
			EvidenceCount: 1,
			AgeBucket:     "<5m",
			Authority:     "SECRET-INTAKE-AUTHORITY",
			Evidence:      "SECRET-INTAKE-EVIDENCE",
			Missing:       "SECRET-INTAKE-MISSING",
			Action:        "SECRET-INTAKE-ACTION",
			Detail:        "SECRET-INTAKE-DETAIL",
			SourceRefs:    "SECRET-INTAKE-SOURCE-REFS",
			NextEvidence:  "SECRET-INTAKE-NEXT-EVIDENCE",
			AIR: denyAll(
				"id", "kind", "blocker", "coverage", "task_link_state",
				"authority", "evidence", "missing", "action", "detail", "source_refs", "next_evidence",
			),
		}},
		Totals: map[string]int{"request_attention": 1},
	}
	sentinels := []string{
		idS, ownS, caseS, nogoS, subjS, actorS, summS, roleS, sessS, platS, rdyS, blockS, claimS, pathS, specS, "active",
		"SECRET-INTAKE-ID", "SECRET-INTAKE-AUTHORITY", "SECRET-INTAKE-EVIDENCE", "SECRET-INTAKE-MISSING",
		"SECRET-INTAKE-ACTION", "SECRET-INTAKE-DETAIL", "SECRET-INTAKE-SOURCE-REFS", "SECRET-INTAKE-NEXT-EVIDENCE",
	}

	base := New("REINS").FoldTasks([]grammar.Task{task}, false).Fold([]grammar.Event{ev}, false).FoldSessions([]grammar.Session{sess}, false).FoldIntake(intake, false)
	base.SessionDetail = grammar.SessionDetail{
		Role: roleS,
		Task: grammar.SessionTaskDetail{
			TaskID: claimS, Status: "claimed", AuthorityCase: caseS, ParentSpec: specS,
		},
		EvidenceRefs: []grammar.EvidenceRef{{Kind: "transcript_candidate", Path: pathS, Size: 99}},
		AIR: map[string]string{
			"task_id": "deny", "status": "ok", "authority_case": "deny", "parent_spec": "deny", "path": "deny",
		},
	}
	base.Width, base.Height, base.AIR, base.Focus = 140, 44, true, 0

	wide := base
	wide.Width = 190

	surfaces := map[string]Model{
		"tasks+rail":  func() Model { m := base; m.Page = PageTasks; return m }(),
		"tasks-wide":  func() Model { m := wide; m.Page = PageTasks; return m }(),
		"tasks-yank":  func() Model { m := base; m.Page = PageTasks; m.Mode = ModeYank; return m }(),
		"task-field":  func() Model { m := base; m.Page = PageTasks; m.Sel.Rank, m.Sel.Field = RankField, "owner"; return m }(),
		"task-filter": func() Model { m := base; m.Page = PageTasks; m.Mode = ModeFilter; return m }(),
		"whois-door":  func() Model { m := base; m.Page = PageTasks; m.DoorOpen = true; return m }(),
		"events+rail": func() Model { m := base; m.Page = PageEvents; return m }(),
		"events-wide": func() Model { m := wide; m.Page = PageEvents; return m }(),
		"events-yank": func() Model { m := base; m.Page = PageEvents; m.Mode = ModeYank; return m }(),
		"sessions+rail": func() Model {
			m := base
			m.Page = PageSessions
			return m
		}(),
		"sessions-wide": func() Model {
			m := wide
			m.Page = PageSessions
			return m
		}(),
		"sessions-yank": func() Model {
			m := base
			m.Page, m.Mode = PageSessions, ModeYank
			return m
		}(),
		"sessions-door": func() Model {
			m := base
			m.Page, m.SessionDoorOpen = PageSessions, true
			return m
		}(),
		"yard": func() Model {
			m := wide
			m.Page = PageYard
			return m
		}(),
		"readiness": func() Model {
			m := wide
			m.Page = PageReadiness
			return m
		}(),
		"split-readiness": func() Model {
			m := wide
			m.Page, m.SplitContext = PageReadiness, true
			return m
		}(),
		"intake": func() Model {
			m := wide
			m.Page = PageIntake
			return m
		}(),
		"split-intake": func() Model {
			m := wide
			m.Page, m.SplitContext = PageIntake, true
			return m
		}(),
		"intake-door": func() Model {
			m := wide
			m.Page, m.IntakeDoorOpen = PageIntake, true
			return m
		}(),
		"capabilities": func() Model {
			m := wide
			m.Page = PageCaps
			return m
		}(),
		"split-capabilities": func() Model {
			m := wide
			m.Page, m.SplitContext = PageCaps, true
			return m
		}(),
	}
	for name, m := range surfaces {
		frame := ansi.Strip(m.View())
		for _, s := range sentinels {
			if strings.Contains(frame, s) {
				t.Errorf("[%s] ON-AIR LEAK: denied value %q rendered in the frame", name, s)
			}
		}
		if !strings.Contains(frame, "▒▒▒") {
			t.Errorf("[%s] expected the redaction token ▒▒▒ in the on-air frame (structure kept)", name)
		}
	}
}

func TestAirHidesDarkErrorReason(t *testing.T) {
	m := New("REINS")
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	m.AIR = true
	m.TasksDark = true
	m.TasksError = "SECRET_LOCAL_PATH /home/user/private/config"
	frame := ansi.Strip(m.View())
	if strings.Contains(frame, "SECRET_LOCAL_PATH") || strings.Contains(frame, "/home/user/private") {
		t.Fatalf("AIR dark frame leaked raw error text:\n%s", frame)
	}
	if !strings.Contains(frame, "reason hidden on AIR") {
		t.Fatalf("AIR dark frame should disclose that the reason exists but is hidden:\n%s", frame)
	}
}

func TestAirDeniedCriticalityDoesNotLeakThroughDerivedChannels(t *testing.T) {
	task := grammar.Task{
		TaskID: "public-task", Stage: "S7", PredictedStage: "hold", Owner: "owner-lane",
		Criticality: "crit", Freshness: 0.9, RelCount: 7,
		AIR: map[string]string{
			"task_id": "ok", "stage": "ok", "predicted_stage": "deny", "owner": "deny",
			"criticality": "deny", "freshness": "deny", "rel_count": "deny",
		},
	}
	m := New("REINS").FoldTasks([]grammar.Task{task}, false)
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	m.AIR = true
	frame := ansi.Strip(m.View())
	for _, leak := range []string{"1 crit", "→hold", "owner-lane", "●7", "█"} {
		if strings.Contains(frame, leak) {
			t.Fatalf("AIR frame leaked denied derived channel %q:\n%s", leak, frame)
		}
	}
	if !strings.Contains(frame, "risk ▒▒▒") || !strings.Contains(frame, "▒▒▒") {
		t.Fatalf("AIR frame should keep structure while hiding denied risk channels:\n%s", frame)
	}
}

func TestAirDeniedEventMetadataDoesNotLeakThroughRows(t *testing.T) {
	ev := grammar.Event{
		TS: "SECRET-TS", Kind: "secret.kind", Subject: "visible-subject", Actor: "SECRET-ACTOR", Score: 0.99,
		AIR: map[string]string{"ts": "deny", "kind": "deny", "score": "deny", "subject": "ok", "actor": "deny", "summary": "deny"},
	}
	m := New("REINS").Fold([]grammar.Event{ev}, false)
	m.Width, m.Height = 120, 40
	m.Page = PageEvents
	m.AIR = true
	frame := ansi.Strip(m.View())
	for _, leak := range []string{"SECRET-TS", "secret.kind", "SECRET-ACTOR", "████"} {
		if strings.Contains(frame, leak) {
			t.Fatalf("AIR event row leaked denied metadata %q:\n%s", leak, frame)
		}
	}
	if !strings.Contains(frame, "▒▒▒") {
		t.Fatalf("AIR event row should keep structure with redaction:\n%s", frame)
	}
}
