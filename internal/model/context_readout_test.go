package model

import (
	"strings"
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

func TestFoldContextGroupsBySubject(t *testing.T) {
	m := New("REINS").FoldContext([]grammar.ContextAffordance{
		{Subject: "task:cc-a", Kind: "explain_why", State: "present"},
		{Subject: "task:cc-a", Kind: "stage_injection_preview", State: "hold"},
		{Subject: "chunk:y", Kind: "refocus", State: "present"},
	}, false)
	if got := len(m.ContextAffordances["task:cc-a"]); got != 2 {
		t.Fatalf("want 2 affordances for task:cc-a, got %d", got)
	}
	if m.ContextDark {
		t.Fatal("should not be dark with content")
	}
}

func TestFoldContextDarkClears(t *testing.T) {
	m := New("REINS").FoldContext([]grammar.ContextAffordance{{Subject: "x", Kind: "refocus", State: "present"}}, false)
	m = m.FoldContext(nil, true) // the producer went dark
	if !m.ContextDark || m.ContextAffordances != nil {
		t.Fatal("dark fold must clear affordances + set dark")
	}
}

func TestContextSuffixHonestDark(t *testing.T) {
	m := New("REINS")
	m.ContextDark = true
	if s := m.contextSuffix("cc-a"); s != "" {
		t.Fatalf("dark producer must yield empty suffix (no clutter), got %q", s)
	}
}

func TestContextSuffixPrefersPresentAffordance(t *testing.T) {
	m := New("REINS").FoldContext([]grammar.ContextAffordance{
		{Subject: "task:cc-a", Kind: "stage_injection_preview", State: "hold"},
		{Subject: "task:cc-a", Kind: "explain_why", State: "present"},
	}, false)
	s := m.contextSuffix("cc-a") // matches "task:cc-a" by subject Contains
	if !strings.Contains(s, "explain_why") {
		t.Fatalf("want the PRESENT (earned) affordance surfaced, got %q", s)
	}
}

func TestContextSuffixNoSubjectMatchEmpty(t *testing.T) {
	m := New("REINS").FoldContext([]grammar.ContextAffordance{{Subject: "chunk:y", Kind: "refocus", State: "present"}}, false)
	if s := m.contextSuffix("cc-unrelated"); s != "" {
		t.Fatalf("no subject match -> empty suffix, got %q", s)
	}
}

func TestContextSuffixSealedOnAir(t *testing.T) {
	// the ctx suffix is operator_private-derived — it must NOT render on the airing frame (derived-channel seal).
	m := New("REINS").FoldContext([]grammar.ContextAffordance{
		{Subject: "task:cc-a", Kind: "yank_operator_private", State: "present"}}, false)
	m.AIR = true
	if s := m.contextSuffix("cc-a"); s != "" {
		t.Fatalf("ctx suffix must be sealed on air, got %q", s)
	}
}

func TestContextSuffixExactMatchNotSubstring(t *testing.T) {
	// deterministic boundary-aware match: taskID "cc-a" must NOT match a subject "cc-a-extended"
	m := New("REINS").FoldContext([]grammar.ContextAffordance{
		{Subject: "cc-a-extended", Kind: "refocus", State: "present"}}, false)
	if s := m.contextSuffix("cc-a"); s != "" {
		t.Fatalf("substring collision: cc-a must not match cc-a-extended, got %q", s)
	}
}
