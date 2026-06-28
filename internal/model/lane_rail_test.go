package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// The fleet lane-rail (E4.5 — one-coordinating-session) shows EVERY session lane as an ambient pulse,
// severity-ranks them (blocked > awaiting > done > streaming > idle), numbers them, and marks the
// focused lane (TurnRole) with ▌. AIR-safe: it airs only allowlisted role/state; a denied
// readiness/blocker/stalled DEGRADES that lane's rank rather than disclosing the denied value through
// the ordering (the derived-channel discipline).
func TestTurnLaneRailIsFleetRankedFocusMarkedAndAirSafe(t *testing.T) {
	m := New("REINS").FoldSessions([]grammar.Session{
		{Role: "cx-idle", Platform: "codex", State: "idle", Readiness: "green", Idle: true,
			AIR: map[string]string{"role": "ok", "state": "ok", "readiness": "ok", "idle": "ok"}},
		{Role: "cx-blocked", Platform: "claude", State: "stalled", Readiness: "red", Blocker: "stale_relay", Stalled: true,
			AIR: map[string]string{"role": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "stalled": "ok"}},
		{Role: "cx-go", Platform: "glm", State: "streaming", Readiness: "green",
			AIR: map[string]string{"role": "ok", "state": "ok", "readiness": "ok"}},
	}, false)
	m.Width, m.Page, m.TurnRole = 200, PageSessionTurns, "cx-go"

	rail := ansi.Strip(m.turnLaneRail(200))
	for _, role := range []string{"cx-idle", "cx-blocked", "cx-go"} {
		if !strings.Contains(rail, role) {
			t.Fatalf("fleet rail must show every lane, missing %q:\n%s", role, rail)
		}
	}
	if strings.Index(rail, "cx-blocked") > strings.Index(rail, "cx-idle") {
		t.Fatalf("blocked lane must rank before idle:\n%s", rail)
	}
	if !strings.Contains(rail, "cx-blocked blocked") {
		t.Fatalf("off air, the red/blocked/stalled lane must label as blocked:\n%s", rail)
	}
	if !strings.Contains(rail, "▌") {
		t.Fatalf("the focused lane (cx-go) must carry the ▌ mark:\n%s", rail)
	}

	// On air with readiness/blocker/stalled DENIED, the rail must NOT label the lane blocked — the
	// ordering must not become a side-channel disclosing the denied red/blocker/stalled.
	mAir := m
	mAir.AIR = true
	denied := make([]grammar.Session, len(m.Sessions))
	copy(denied, m.Sessions)
	for i := range denied {
		denied[i].AIR = map[string]string{"role": "ok", "state": "ok"}
	}
	mAir.Sessions = denied
	railAir := ansi.Strip(mAir.turnLaneRail(200))
	if strings.Contains(railAir, "cx-blocked blocked") {
		t.Fatalf("on air with readiness/blocker/stalled denied, the rail must not label a lane blocked:\n%s", railAir)
	}
}

// The C4 session-position readout (06-28 canon) shows the focused lane's n-DLC position — its claimed
// task + that task's SDLC stage — folded from the present read model. AIR-safe: the claimed_task id
// DENIES on air (not allowlisted = identity-ish), the stage AIRS (allowlisted structural).
func TestTurnSessionPositionShowsClaimedTaskAndStageAirSafe(t *testing.T) {
	m := New("REINS").
		FoldTasks([]grammar.Task{{TaskID: "task-x", Stage: "S7_RELEASE", PredictedStage: "hold",
			AIR: map[string]string{"task_id": "ok", "stage": "ok", "predicted_stage": "ok"}}}, false).
		FoldSessions([]grammar.Session{{Role: "cx-lead", State: "streaming", ClaimedTask: "task-x",
			AIR: map[string]string{"role": "ok", "state": "ok"}}}, false)
	m.TurnRole = "cx-lead"

	off := ansi.Strip(m.turnSessionPosition(200))
	for _, want := range []string{"cx-lead", "task-x", "S7_RELEASE"} {
		if !strings.Contains(off, want) {
			t.Fatalf("off-air C4 position must show %q:\n%s", want, off)
		}
	}
	m.AIR = true
	on := ansi.Strip(m.turnSessionPosition(200))
	if strings.Contains(on, "task-x") {
		t.Fatalf("on air the claimed_task id must deny (not allowlisted):\n%s", on)
	}
	if !strings.Contains(on, "S7_RELEASE") {
		t.Fatalf("on air the allowlisted stage must still air:\n%s", on)
	}
}

// The breakdown-inbox (E4.5) AUTO-SURFACES only at breakdown — the blocked/awaiting lanes that need the
// operator — and RECEDES to empty in steady state (the equipment-at-breakdown principle). A healthy
// lane never appears.
func TestTurnBreakdownInboxAutoSurfacesAndRecedes(t *testing.T) {
	calm := New("REINS").FoldSessions([]grammar.Session{
		{Role: "cx-a", State: "streaming", Readiness: "green", AIR: map[string]string{"role": "ok", "state": "ok", "readiness": "ok"}},
	}, false)
	if got := calm.turnBreakdownInbox(200); got != "" {
		t.Fatalf("steady state must recede (empty), got:\n%s", got)
	}
	m := New("REINS").FoldSessions([]grammar.Session{
		{Role: "cx-block", State: "stalled", Readiness: "red", Blocker: "stale_relay", Stalled: true,
			AIR: map[string]string{"role": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "stalled": "ok"}},
		{Role: "cx-wait", State: "awaiting", Readiness: "amber", AIR: map[string]string{"role": "ok", "state": "ok", "readiness": "ok"}},
		{Role: "cx-ok", State: "streaming", Readiness: "green", AIR: map[string]string{"role": "ok", "state": "ok", "readiness": "ok"}},
	}, false)
	box := ansi.Strip(m.turnBreakdownInbox(200))
	for _, want := range []string{"BREAKDOWN", "cx-block (blocked)", "cx-wait (awaiting)"} {
		if !strings.Contains(box, want) {
			t.Fatalf("breakdown-inbox must surface %q:\n%s", want, box)
		}
	}
	if strings.Contains(box, "cx-ok") {
		t.Fatalf("a healthy lane must not appear in the breakdown-inbox:\n%s", box)
	}
}

// cycleLane ([←/→] on :turns) moves the focused lane (TurnRole) through the fleet — the lane-rail
// navigation (E4.5). It wraps, and marks TurnsDark so the periodic tick refetches the new lane.
func TestCycleLaneMovesFocusedLaneThroughFleet(t *testing.T) {
	m := New("REINS").FoldSessions([]grammar.Session{
		{Role: "cx-a", AIR: map[string]string{"role": "ok"}},
		{Role: "cx-b", AIR: map[string]string{"role": "ok"}},
		{Role: "cx-c", AIR: map[string]string{"role": "ok"}},
	}, false)
	m.TurnRole = "cx-a"
	if m = m.cycleLane(1); m.TurnRole != "cx-b" {
		t.Fatalf("right should advance to cx-b, got %q", m.TurnRole)
	}
	if !m.TurnsDark {
		t.Fatalf("cycling a lane must mark the feed dark for refetch")
	}
	if m = m.cycleLane(1); m.TurnRole != "cx-c" {
		t.Fatalf("right should advance to cx-c, got %q", m.TurnRole)
	}
	if m = m.cycleLane(1); m.TurnRole != "cx-a" {
		t.Fatalf("right should wrap to cx-a, got %q", m.TurnRole)
	}
	if m = m.cycleLane(-1); m.TurnRole != "cx-c" {
		t.Fatalf("left should wrap back to cx-c, got %q", m.TurnRole)
	}
}

// turnImpingeAffordances (E4.4 preview) shows the governed act-on-this-turn decisions, contextualized
// by the turn KIND, always governed + NOT WIRED (never-mint; egress gated). AIR-safe: structural verbs,
// no turn body.
func TestTurnImpingeAffordancesAreContextualAndGovernedNotWired(t *testing.T) {
	m := New("REINS")
	cases := map[string][]string{
		"approval":  {"accept", "deny", "edit"},
		"tool_call": {"approve", "edit", "deny"},
		"assistant": {"respond", "ignore"},
		"user":      {"respond", "ignore"},
	}
	for kind, want := range cases {
		out := ansi.Strip(m.turnImpingeAffordances(grammar.Turn{Kind: kind}))
		if !strings.Contains(out, "IMPINGE") || !strings.Contains(out, "NOT wired") {
			t.Fatalf("kind %q must show the governed NOT-wired impinge preview:\n%s", kind, out)
		}
		for _, v := range want {
			if !strings.Contains(out, "["+v+"]") {
				t.Fatalf("kind %q must offer [%s]:\n%s", kind, v, out)
			}
		}
	}
}
