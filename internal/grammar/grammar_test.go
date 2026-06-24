package grammar

import (
	"strings"
	"testing"
)

func sample() Event {
	return Event{TS: "14:22", Kind: "pr.merged", Subject: "4284", Actor: "alpha",
		Summary: "PR#4284 merged to main", Score: 0.7,
		AIR: map[string]string{"subject": "ok", "actor": "deny", "summary": "deny"}}
}

func TestRenderEventRowLocal(t *testing.T) {
	got := RenderEventRow(sample(), false)
	if !strings.Contains(got, Glyph("pr.merged")) || !strings.Contains(got, "4284") || !strings.Contains(got, "merged to main") {
		t.Fatalf("local row missing fields: %q", got)
	}
}

func TestRenderEventRowAIRRedactsDenied(t *testing.T) {
	got := RenderEventRow(sample(), true)
	if strings.Contains(got, "merged to main") {
		t.Fatalf("AIR row leaked a denied field: %q", got)
	}
	if !strings.Contains(got, "4284") || !strings.Contains(got, "▒") {
		t.Fatalf("AIR row should keep allowlisted subject + show redaction glyph: %q", got)
	}
}

func TestGlyphIsStableAndMonochromeSafe(t *testing.T) {
	if Glyph("pr.merged") == Glyph("review.fail") {
		t.Fatal("distinct kinds must have distinct glyphs (the glyph carries the kind)")
	}
}
