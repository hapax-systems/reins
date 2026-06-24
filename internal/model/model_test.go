package model

import (
	"strings"
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

func evs() []grammar.Event {
	return []grammar.Event{
		{TS: "14:22", Kind: "pr.merged", Subject: "4284", Summary: "merged", Score: 0.7,
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
