package model

import (
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

func selModel() Model {
	m := New("REINS").FoldTasks([]grammar.Task{
		{TaskID: "task-9", Stage: "S5", Owner: "alpha", Criticality: "crit",
			AIR: map[string]string{"task_id": "ok", "stage": "ok", "owner": "deny", "criticality": "ok"}},
	}, false)
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	m.Focus = 0
	return m
}

func TestTemplateResolvesSelection(t *testing.T) {
	m := selModel()
	m.Sel.Rank, m.Sel.Field = RankField, "stage"
	cases := map[string]string{
		"{{sel}}":       "S5",     // selected field's value
		"{{sel.id}}":    "task-9", // focused row id
		"{{focus}}":     "task-9",
		"{{sel.field}}": "stage", // the field NAME
		"{{sel.crit}}":  "crit",
		"{{sel.owner}}": "alpha", // arbitrary field (LOCAL — not redacted)
		"a {{sel.id}} b": "a task-9 b",
		"{{nope}}":      "{{nope}}", // unknown token stays literal
	}
	for in, want := range cases {
		if got := m.resolveTemplate(in); got != want {
			t.Errorf("resolveTemplate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTemplateRedactsOnAir(t *testing.T) {
	m := selModel()
	m.AIR = true // ON AIR — a denied field must NOT leak its value
	if got := m.resolveTemplate("{{sel.owner}}"); got != "▒▒▒" {
		t.Fatalf("on-air, a denied field must redact, got %q", got)
	}
	if got := m.resolveTemplate("{{sel.stage}}"); got != "S5" { // allowlisted: still resolves
		t.Fatalf("on-air, an allowlisted field should resolve, got %q", got)
	}
}

func TestNoteVerbExpandsTemplate(t *testing.T) {
	m := selModel()
	out := m.Exec("note owner is {{sel.owner}}")
	if out.Status != "note ▸ owner is alpha" {
		t.Fatalf("note should expand the ref, got %q", out.Status)
	}
}

func TestTemplateCandidatesWhenOpen(t *testing.T) {
	m := selModel()
	m.Mode = ModeCommand
	m.Input = "note {{sel."
	cands := m.completionTree()
	if len(cands) == 0 {
		t.Fatal("an open {{sel. should offer template refs")
	}
	for _, c := range cands {
		if c.Label[:2] != "{{" {
			t.Fatalf("expected template candidates, got %q", c.Label)
		}
	}
	// accepting fills the open fragment, stays in command mode (composing)
	m.CompIdx = 0
	m = m.acceptCompletion()
	if m.Mode != ModeCommand {
		t.Fatal("accepting a template ref should keep composing")
	}
	if got := m.Input; got[:5] != "note " || got[len(got)-2:] != "}}" {
		t.Fatalf("fill should replace the open fragment with a full ref, got %q", got)
	}
}
