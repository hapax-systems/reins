package model

import (
	"regexp"
	"strconv"
	"strings"
)

// The {{sel}} inline selection-template language (handoff §9, #18 — the consolidation payoff seed).
// A command/message may REFERENCE the active selection or kill-ring; the reference expands at
// send/exec time, so cockpit data flows into the line. Hosted by the [:] command line today; the
// session pane will reuse the SAME resolver. Tokens: {{sel}} {{sel.id}} {{sel.field}} {{sel.value}}
// {{sel.crit}} {{sel.<field>}} {{focus}} {{ring.N}}.
var tmplRe = regexp.MustCompile(`\{\{([a-zA-Z0-9_.]+)\}\}`)

// resolveTemplate expands every {{key}} in s against the live model. Unknown tokens are left literal
// (typos stay visible). Called at the TOP of Exec, so any command's text resolves before it runs.
func (m Model) resolveTemplate(s string) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	return tmplRe.ReplaceAllStringFunc(s, func(tok string) string {
		if v, ok := m.templateValue(tok[2 : len(tok)-2]); ok {
			return v
		}
		return tok
	})
}

// templateValue resolves one {{key}}. AIR-SAFE: a field DENIED by the on-air lens expands to the
// redaction token, never its value — the resolver must not leak a private value onto the stream.
func (m Model) templateValue(key string) (string, bool) {
	t, has := m.FocusedTask()
	field := func(f string) (string, bool) {
		if !has {
			return "", false
		}
		if m.AIR && t.AIR[f] != "ok" {
			return "▒▒▒", true // default-deny: keep the structure, blank the value
		}
		return fieldValue(t, f), true
	}
	switch {
	case key == "sel", key == "sel.value":
		if m.Sel.Rank == RankField && m.Sel.Field != "" {
			return field(m.Sel.Field)
		}
		return field("task_id")
	case key == "sel.id", key == "focus":
		return field("task_id")
	case key == "sel.field": // a field NAME, not a value — never redacted
		if m.Sel.Field != "" {
			return m.Sel.Field, true
		}
	case key == "sel.crit":
		return field("criticality")
	case strings.HasPrefix(key, "sel."):
		return field(strings.TrimPrefix(key, "sel."))
	case strings.HasPrefix(key, "ring."):
		if i, err := strconv.Atoi(strings.TrimPrefix(key, "ring.")); err == nil && i >= 0 && i < len(m.Ring) {
			return m.Ring[i].Value, true
		}
	}
	return "", false
}

// templateKeys: the offerable references (kept small + ordered for the fish menu).
var templateKeys = []string{"sel", "sel.id", "sel.field", "sel.value", "sel.crit", "focus", "ring.0"}

// templateCandidates: when the input has an OPEN `{{…` fragment, offer the references as fish
// candidates, each with an AIR-safe LIVE PREVIEW of what it resolves to (discoverability + dynamic
// on selection). Accepting one fills the open fragment; you keep composing (never runs).
func (m Model) templateCandidates(prefix string) []Candidate {
	var out []Candidate
	for _, k := range templateKeys {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		detail := "(no selection)"
		if v, ok := m.templateValue(k); ok {
			detail = "→ " + clip(v, 28)
		}
		out = append(out, Candidate{Label: "{{" + k + "}}", Value: "{{" + k + "}}", Detail: detail})
	}
	return out
}

// openTemplatePrefix reports the key-prefix of an UNCLOSED `{{…` at the end of the input (and true),
// e.g. "note {{sel." → "sel.". Empty + false when there is no open template token.
func openTemplatePrefix(input string) (string, bool) {
	i := strings.LastIndex(input, "{{")
	if i < 0 {
		return "", false
	}
	frag := input[i+2:]
	if strings.Contains(frag, "}}") {
		return "", false // already closed
	}
	return frag, true
}

// fillOpenTemplate replaces the trailing open `{{…` fragment with the full `{{key}}` reference.
func fillOpenTemplate(input, full string) string {
	if i := strings.LastIndex(input, "{{"); i >= 0 {
		return input[:i] + full
	}
	return input + full
}
