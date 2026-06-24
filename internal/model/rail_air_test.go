package model

import (
	"strings"
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

// REGRESSION (live-smoke 2026-06-24): the context rail unfolded EVERY field in cleartext, bypassing
// the on-air lens — authority_case leaked on the stream. The rail must honor AIR like the body rows.
func TestRailRedactsDeniedFieldsOnAir(t *testing.T) {
	task := grammar.Task{
		TaskID: "x", Stage: "S7", Owner: "zeta", AuthorityCase: "CASE-SECRET-001", NoGo: "impl_authorized",
		AIR: map[string]string{
			"task_id": "ok", "stage": "ok", "owner": "deny",
			"authority_case": "deny", "no_go": "deny", "criticality": "ok",
			"prior_stage": "ok", "predicted_stage": "ok", "freshness": "ok", "rel_count": "ok",
		},
	}
	m := New("REINS").FoldTasks([]grammar.Task{task}, false)
	m.Width, m.Height, m.Page, m.Focus = 120, 40, PageTasks, 0

	local := m.viewRail(60)
	if !strings.Contains(local, "CASE-SECRET-001") || !strings.Contains(local, "zeta") {
		t.Fatal("LOCAL rail should show the real values")
	}

	m.AIR = true
	air := m.viewRail(60)
	for _, leak := range []string{"CASE-SECRET-001", "zeta", "impl_authorized"} {
		if strings.Contains(air, leak) {
			t.Fatalf("on-air rail leaked a denied field: %q", leak)
		}
	}
	if !strings.Contains(air, "▒▒▒") {
		t.Fatal("on-air rail should render the redaction token for denied fields")
	}
	// an allowlisted field (stage) must still resolve on-air
	if !strings.Contains(air, "S7") {
		t.Fatal("on-air rail should still show allowlisted fields")
	}
}
