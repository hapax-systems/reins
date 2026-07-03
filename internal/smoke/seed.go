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
	events := make([]grammar.Event, 0, 8)
	for i, e := range []struct{ kind, subject, actor, summary string }{
		{"gate_output", "reform-fix-eventlog", "cc-alpha", "gate passed"},
		{"dispatch", "open PR for the parity fix", "cc-alpha", "dispatched to lane-beta"},
		{"review", "reform-fix-eventlog", "cc-review", "review recorded fail"},
		{"coord_dispatch.launch_succeeded", "reform-improve-coord", "cc-beta", "lane launched"},
		{"stage_transition", "reform-add-parity", "cc-gamma", "S5 → S6"},
		{"claim", "reform-improve-policy", "cc-delta", "claimed by lane-delta"},
		{"gate_output", "reform-improve-shadow", "cc-alpha", "release-armed pending"},
		{"alert", "p0-incident-eventlog", "security", "attention required"},
	} {
		events = append(events, grammar.Event{TS: tstamp(i), Kind: e.kind, Subject: e.subject, Actor: e.actor, Summary: e.summary})
	}
	m = m.Fold(events, false)

	tasks := make([]grammar.Task, 0, 12)
	for _, t := range []struct {
		id, stage, pred, owner, crit string
	}{
		{"reform-fix-eventlog-ssot-ledger", "S7_RELEASE", "hold", "cc-alpha", "warn"},
		{"reform-improve-coord-provisioning", "S7_RELEASE", "hold", "cc-beta", "warn"},
		{"reform-add-parity-compliance", "S6_IMPLEMENT", "ship", "cc-gamma", "major"},
		{"reform-improve-policy-shadow", "S6_IMPLEMENT", "advance", "cc-delta", "ok"},
		{"reform-improve-acceptance-floor", "S5_DESIGN", "advance", "cc-alpha", "ok"},
		{"reform-fix-eventlog-coord", "S7_RELEASE", "hold", "cc-beta", "warn"},
		{"reform-improve-shadow-eval", "S6_IMPLEMENT", "ship", "cc-gamma", "major"},
		{"reform-add-cost-attribution", "S4_PLAN", "advance", "cc-delta", "ok"},
		{"reform-improve-merge-queue", "S7_RELEASE", "hold", "cc-alpha", "crit"},
		{"reform-fix-router-envelope", "S6_IMPLEMENT", "advance", "cc-epsilon", "warn"},
		{"reform-improve-witness-reseat", "S5_DESIGN", "advance", "cc-beta", "ok"},
		{"reform-add-dispatch-cost", "S3_INTAKE", "advance", "cc-gamma", "ok"},
	} {
		tasks = append(tasks, grammar.Task{TaskID: t.id, Stage: t.stage, PredictedStage: t.pred, Owner: t.owner, Criticality: t.crit, RelCount: 1})
	}
	m = m.FoldTasks(tasks, false)

	m = m.FoldSessions([]grammar.Session{
		{Role: "cc-alpha", Session: "tmux-alpha", Platform: "claude", State: "streaming", Readiness: "green", Alive: true, Attention: 0.88, ClaimedTask: "reform-fix-eventlog-ssot-ledger"},
		{Role: "cc-beta", Session: "tmux-beta", Platform: "codex", State: "idle", Readiness: "amber", Idle: true, Attention: 0.41},
		{Role: "cc-gamma", Session: "tmux-gamma", Platform: "claude", State: "streaming", Readiness: "green", Alive: true, Attention: 0.67},
		{Role: "cc-delta", Session: "tmux-delta", Platform: "glm", State: "awaiting", Readiness: "amber", Alive: true, Attention: 0.52},
		{Role: "cc-epsilon", Session: "tmux-epsilon", Platform: "codex", State: "stalled", Readiness: "red", Stalled: true, Blocker: "stale_relay", Attention: 0.20},
	}, false)

	m = m.FoldTraces([]grammar.Trace{
		{TS: tstamp(0), TraceID: "tr-1", Model: "claude-opus-4", LatencyMs: 220, TotalTok: 1500, Cost: 0.02},
		{TS: tstamp(1), TraceID: "tr-2", Model: "claude-opus-4", LatencyMs: 180, TotalTok: 900, Cost: 0.01},
		{TS: tstamp(2), TraceID: "tr-3", Model: "glm-5.2", LatencyMs: 140, TotalTok: 600},
		{TS: tstamp(3), TraceID: "tr-4", Model: "command-r-35b", LatencyMs: 240, TotalTok: 1100},
	}, false)

	m = m.FoldEpistemics(grammar.EpistemicsSummary{Rows: []grammar.EpistemicReadRow{
		{RowID: "e1", Family: "claim", Subject: "eval is a validated acceptor", Status: "asserted", Authority: "cc-alpha"},
		{RowID: "e2", Family: "validation", Subject: "positive control AUC 1.0", Status: "fresh", Authority: "cc-alpha"},
		{RowID: "e3", Family: "observation", Subject: "over-research is conditional", Status: "observed", Authority: "cc-beta"},
		{RowID: "e4", Family: "source", Subject: "the representational framework", Status: "fresh", Authority: "cc-gamma"},
	}}, false)

	// the WITNESSED LEDGER (U3/CP-A): representative demand+verdict datoms so `reins --demo` shows real
	// signed receipts on :commands, with the tamper-evidence banner reading "verified" (the signed
	// hash-chain intact). A tampered ledger would read "broken:<reason>" here — never faked green.
	commandAIR := map[string]string{"verb": "ok", "status": "ok", "witness": "ok", "task_id": "ok"}
	m = m.FoldCommands([]grammar.Command{
		{Verb: "dispatch", Target: "lane-beta", Status: "not-wired", Witness: "pending", AIR: commandAIR},
		{Verb: "resume", Target: "cc-alpha", Status: "ok", Witness: "pending", TaskID: "reform-fix-eventlog-ssot-ledger", AIR: commandAIR},
	}, "absent", "verified", false)

	// turns populate the chat-pane + the A6 authorship provenance distribution
	m.TurnLadder = []grammar.Turn{
		{Role: "operator", Kind: "user", Summary: "fix the flaky trace test", Prov: "operator"},
		{Role: "cc-reins", Kind: "reasoning", Summary: "widen the timeout, stub the clock", Prov: "model", Model: "claude-opus-4"},
		{Role: "cc-reins", Kind: "tool_call", Summary: "Bash(go test ./...)", Prov: "structured", Model: "claude-opus-4"},
		{Role: "cc-reins", Kind: "tool_result", Summary: "ok internal/grammar", Prov: "untrusted"},
		{Role: "cc-reins", Kind: "assistant", Summary: "the test is widened + green", Prov: "model", Model: "claude-opus-4"},
		{Role: "cc-reins", Kind: "reasoning", Summary: "now auditing the adjacent fold path for the same race", Prov: "model", Model: "claude-opus-4", Streaming: true, Tokens: 142}, // in-flight → two-frame broadcast (E4.6)
	}

	// Give each item the AIR map the live READ API would (the allowlisted structural fields air;
	// everything else default-denies) so the OFFLINE on-air render is representative of live, not a
	// blanket ▒▒▒. The turn SKELETON airs (role/kind/model/gate/ts); the body never gets "summary":ok.
	taskAIR := map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok", "prior_stage": "ok", "no_go": "ok", "criticality": "ok", "owner": "ok"}
	// Faithful to the live config.AIRAllowlist: the event SUBJECT (free-text title) and ACTOR (identity
	// of who acted) are NOT allowlisted → they DENY on air. The seed previously hand-set them "ok",
	// making the offline --air render optimistic (it aired denied free-text the live API would redact —
	// confirmed by the adversarial AIR sweep). Only the structural skeleton airs. (The broader fidelity
	// of trace/turn/predicted_stage/criticality is config-gap-vs-intentional — operator-bound; see the
	// reins-seed-air-fidelity-gap note.)
	eventAIR := map[string]string{"ts": "ok", "kind": "ok", "score": "ok"}
	sessionAIR := map[string]string{"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "alive": "ok", "idle": "ok", "stalled": "ok"}
	// "air structural, DENY $cost" (operator bar 2026-06-28): the trace skeleton airs; the financial
	// cost is omitted → it denies on air (matches config.AIRAllowlist; faithful offline --air preview).
	traceAIR := map[string]string{"ts": "ok", "trace_id": "ok", "model": "ok", "latency_ms": "ok", "total_tok": "ok"}
	epiAIR := map[string]string{"row_id": "ok", "family": "ok", "subject": "ok", "status": "ok", "authority": "ok"}
	turnAIR := map[string]string{"role": "ok", "kind": "ok", "model": "ok", "gate": "ok", "ts": "ok"}
	for i := range m.Tasks {
		m.Tasks[i].AIR = taskAIR
	}
	for i := range m.Events {
		m.Events[i].AIR = eventAIR
	}
	for i := range m.Sessions {
		m.Sessions[i].AIR = sessionAIR
	}
	for i := range m.Traces {
		m.Traces[i].AIR = traceAIR
	}
	for i := range m.Epistemics.Rows {
		m.Epistemics.Rows[i].AIR = epiAIR
	}
	for i := range m.TurnLadder {
		m.TurnLadder[i].AIR = turnAIR
	}
	return m
}

// tstamp is a fixed-base deterministic timestamp helper for the seed (no Date.now in fixtures).
func tstamp(i int) string {
	return "2026-06-28T10:0" + string(rune('0'+i%10)) + ":00Z"
}

// PageCommands is the ordered set of command names that switch to every cockpit page — the smoke
// driver types each as ":name" to visit it. (One canonical command per page; aliases omitted.)
var PageCommands = []string{
	"coordinator", "events", "tasks", "sessions", "dynamics", "loops", "readiness",
	"capabilities", "intake", "epistemics", "traces", "intent", "dispatch", "turns",
	"help", "legend", "commands", "windows", "surfaces", "domains", "lifecycles", "yard", "axes", "identity", "relational",
}
