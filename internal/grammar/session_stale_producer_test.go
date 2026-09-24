package grammar

import (
	"strings"
	"testing"
)

// A row the API marks state=unknown (its producer is stale) carries alive=false because it cannot
// assert alive — not because anyone measured the lane offline. ○ is the measured negative ("absent");
// a starved producer is ▒ (dark != absent). Rendering ○ would turn "cannot say" back into "offline".
func TestSessionGlyphForAStaleProducerRowIsDarkNotAbsent(t *testing.T) {
	s := Session{Role: "lane-01", Platform: "codex", State: "unknown", Readiness: "unknown", Blocker: "stale_producer"}
	if got := sessionGlyph(s, false); got != "▒" {
		t.Fatalf("stale-producer row glyph = %q, want ▒ (starved), never ○ (measured offline)", got)
	}
	row := RenderSessionRow(s, false)
	if strings.Contains(row, "○") || strings.Contains(row, "●") {
		t.Fatalf("stale-producer row renders a liveness verdict it does not have: %q", row)
	}
}

// Only a measured state=offline earns ○. A row that is not alive with any other state — empty,
// missing, or a value this build does not know — has no measured offline behind it.
func TestSessionGlyphNeverShowsMeasuredOfflineWithoutAnOfflineState(t *testing.T) {
	for _, state := range []string{"", "idle", "somethingnew"} {
		s := Session{Role: "lane-x", State: state}
		if got := sessionGlyph(s, false); got == "○" {
			t.Fatalf("state=%q alive=false rendered ○ (measured offline) without a measured offline", state)
		}
	}
}

func TestSessionGlyphForAMeasuredOfflineRowStaysAbsent(t *testing.T) {
	s := Session{Role: "lane-18", Platform: "codex", State: "offline", Readiness: "off", Blocker: "offline"}
	if got := sessionGlyph(s, false); got != "○" {
		t.Fatalf("measured-offline row glyph = %q, want ○", got)
	}
}
