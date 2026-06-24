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
	sentinels := []string{idS, ownS, caseS, nogoS, subjS, actorS, summS}

	base := New("REINS").FoldTasks([]grammar.Task{task}, false).Fold([]grammar.Event{ev}, false)
	base.Width, base.Height, base.AIR, base.Focus = 140, 44, true, 0

	surfaces := map[string]Model{
		"tasks+rail":  func() Model { m := base; m.Page = PageTasks; return m }(),
		"tasks-yank":  func() Model { m := base; m.Page = PageTasks; m.Mode = ModeYank; return m }(),
		"whois-door":  func() Model { m := base; m.Page = PageTasks; m.DoorOpen = true; return m }(),
		"events+rail": func() Model { m := base; m.Page = PageEvents; return m }(),
		"events-yank": func() Model { m := base; m.Page = PageEvents; m.Mode = ModeYank; return m }(),
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
