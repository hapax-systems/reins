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

func TestCompactTS(t *testing.T) {
	cases := map[string]string{
		"2026-06-24T01:53:07Z":        "01:53:07",
		"2026-06-24T01:53:07.123456Z": "01:53:07",
		"2026-06-24T01:53:07+00:00":   "01:53:07",
		"14:22":                       "14:22   ", // no 'T' -> padded passthrough
	}
	for in, want := range cases {
		if got := compactTS(in); got != want {
			t.Fatalf("compactTS(%q)=%q want %q", in, got, want)
		}
	}
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

func sampleTask() Task {
	return Task{TaskID: "event-spine-coord-event-log-20260623", Stage: "S6", NoGo: "",
		AIR: map[string]string{"task_id": "ok", "stage": "ok", "no_go": "ok"}}
}

func TestRenderTaskRowLocal(t *testing.T) {
	got := RenderTaskRow(sampleTask(), false)
	if !strings.Contains(got, Glyph("task.closed")) || !strings.Contains(got, "event-spine") || !strings.Contains(got, "S6") {
		t.Fatalf("task row missing fields: %q", got)
	}
}

func TestRenderTaskRowStructuredSilence(t *testing.T) {
	got := RenderTaskRow(sampleTask(), false) // empty no_go -> dots, not blank jitter
	if !strings.Contains(got, "····") {
		t.Fatalf("empty cell must be structured-silence dots: %q", got)
	}
}

func TestRenderTaskRowAIRRedacts(t *testing.T) {
	tk := sampleTask()
	tk.AIR = map[string]string{"task_id": "ok", "stage": "deny", "no_go": "ok"}
	got := RenderTaskRow(tk, true)
	if strings.Contains(got, "S6") {
		t.Fatalf("AIR must redact the denied stage: %q", got)
	}
	if !strings.Contains(got, "event-spine") {
		t.Fatalf("AIR must keep the allowlisted task_id: %q", got)
	}
}
