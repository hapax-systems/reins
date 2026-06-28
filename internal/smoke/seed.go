package smoke

import (
	"github.com/hapax-systems/reins/internal/grammar"
	"github.com/hapax-systems/reins/internal/model"
)

// SeedModel builds a deterministic, offline cockpit model with representative rows on every list
// surface — so a navigation smoke can exercise real content (j/k/enter/v) without the live API. It
// is the fixed substrate the smoke test + the `--drive` offline mode both render.
func SeedModel(w, h int) model.Model {
	m := model.New("REINS")
	m.Width, m.Height = w, h
	m = m.Fold([]grammar.Event{
		{TS: "2026-06-28T10:00:00Z", Kind: "gate_output", Subject: "reform-fix-eventlog", Actor: "cc-alpha", Summary: "gate passed"},
		{TS: "2026-06-28T10:01:00Z", Kind: "dispatch", Subject: "open PR", Actor: "cc-alpha", Summary: "dispatched to lane-beta"},
		{TS: "2026-06-28T10:02:00Z", Kind: "review", Subject: "reform-fix-eventlog", Actor: "cc-review", Summary: "review recorded fail"},
	}, false)
	m = m.FoldTasks([]grammar.Task{
		{TaskID: "reform-fix-eventlog", Stage: "S7_RELEASE", PredictedStage: "hold", Owner: "cc-alpha", Criticality: "warn"},
		{TaskID: "reform-improve-coord", Stage: "S6_IMPLEMENT", PredictedStage: "ship", Owner: "cc-beta", Criticality: "major"},
		{TaskID: "reform-add-parity", Stage: "S5_DESIGN", PredictedStage: "advance", Owner: "cc-alpha", Criticality: "ok"},
	}, false)
	m = m.FoldSessions([]grammar.Session{
		{Role: "cc-alpha", Session: "tmux-alpha", Platform: "claude", State: "streaming", Readiness: "green", Alive: true, Attention: 0.8},
		{Role: "cc-beta", Session: "tmux-beta", Platform: "codex", State: "idle", Readiness: "amber", Idle: true, Attention: 0.3},
	}, false)
	m = m.FoldTraces([]grammar.Trace{
		{TS: "2026-06-28T10:00:00Z", TraceID: "tr-1", Model: "claude-opus-4", LatencyMs: 220, TotalTok: 1500, Cost: 0.02},
		{TS: "2026-06-28T10:01:00Z", TraceID: "tr-2", Model: "claude-opus-4", LatencyMs: 180, TotalTok: 900, Cost: 0.01},
	}, false)
	m = m.FoldEpistemics(grammar.EpistemicsSummary{Rows: []grammar.EpistemicReadRow{
		{RowID: "e1", Family: "claim", Subject: "eval is a validated acceptor", Status: "asserted", Authority: "cc-alpha"},
		{RowID: "e2", Family: "validation", Subject: "positive control AUC 1.0", Status: "fresh", Authority: "cc-alpha"},
	}}, false)
	return m
}

// PageCommands is the ordered set of command names that switch to every cockpit page — the smoke
// driver types each as ":name" to visit it. (One canonical command per page; aliases omitted.)
var PageCommands = []string{
	"coordinator", "events", "tasks", "sessions", "dynamics", "loops", "readiness",
	"capabilities", "intake", "epistemics", "traces", "intent", "dispatch", "turns",
	"help", "legend", "commands", "windows", "surfaces", "domains", "lifecycles", "yard",
}
