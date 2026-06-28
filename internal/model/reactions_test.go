package model

import (
	"strings"
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

func TestOnCommandArmsListsAndFiresPreviewOnly(t *testing.T) {
	m := New("REINS").Exec(":on review.fail->flash")
	if len(m.Reactions) != 1 {
		t.Fatalf("expected one armed reaction, got %d", len(m.Reactions))
	}
	if got := m.Reactions[0]; got.EventKind != "review.fail" || got.Match != "" || got.Effect != "flash" {
		t.Fatalf("unexpected reaction: %+v", got)
	}

	listed := m.Exec("on")
	if !strings.Contains(listed.Status, "flash") || !strings.Contains(listed.Status, "review.fail") {
		t.Fatalf("bare :on should list armed reactions, status=%q", listed.Status)
	}

	m.DispatchRecords = []grammar.DispatchRecord{{Capability: "sentinel", RouteID: "route-1"}}
	m = m.Fold([]grammar.Event{{
		TS:      "2026-06-27T12:00:00Z",
		Kind:    "review.fail",
		Subject: "task-1",
		Summary: "gate blocked",
	}}, false)

	if !strings.Contains(m.Status, "armed reaction fired: flash") {
		t.Fatalf("matching event should surface fired preview notice, status=%q", m.Status)
	}
	if !strings.Contains(m.Status, "would emit flash; NOT wired") {
		t.Fatalf("fired notice must stay preview-only/non-wired, status=%q", m.Status)
	}
	if len(m.DispatchRecords) != 1 || m.DispatchRecords[0].Capability != "sentinel" {
		t.Fatalf("reaction firing must not mint/append dispatch records: %+v", m.DispatchRecords)
	}

	refold := m.Fold(m.Events, false)
	if strings.Count(refold.Status, "armed reaction fired: flash") != 1 {
		t.Fatalf("re-folding existing events must not fire again, status=%q", refold.Status)
	}
}

func TestOnCommandNonMatchingEventDoesNotFire(t *testing.T) {
	m := New("REINS").Exec("on review.fail #blocked { flash }")
	if len(m.Reactions) != 1 {
		t.Fatalf("expected one armed reaction, got %d", len(m.Reactions))
	}

	m = m.Fold([]grammar.Event{{
		TS:      "2026-06-27T12:01:00Z",
		Kind:    "pr.merged",
		Subject: "task-1",
		Summary: "gate blocked",
	}}, false)

	if strings.Contains(m.Status, "armed reaction fired") {
		t.Fatalf("non-matching event must not fire reaction, status=%q", m.Status)
	}
}
