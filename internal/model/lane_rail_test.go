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
