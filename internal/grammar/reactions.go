package grammar

import (
	"errors"
	"fmt"
	"strings"
)

// Reaction is a single armed automation that fires when a live cockpit event matches.
type Reaction struct {
	EventKind string // matches an event's Kind by case-insensitive substring (empty = any kind)
	Match     string // matches an event's Summary/Subject by case-insensitive substring (empty = any)
	Effect    string // an opaque effect token, e.g. "flash" | "ntfy" | "log"
	Raw       string // the original "/on ..." text, for display/legend
}

// ParseReaction parses one "/on <eventKind> <match> { <effect> }" line into a Reaction.
func ParseReaction(s string) (Reaction, error) {
	raw := s
	t := strings.TrimSpace(s)
	if t == "" {
		return Reaction{}, errors.New("reaction: empty input")
	}
	// Optional but expected "/on" prefix.
	if strings.HasPrefix(t, "/on") {
		rest := t[len("/on"):]
		if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
			return Reaction{}, fmt.Errorf("reaction: unexpected token after /on: %q", rest)
		}
		t = strings.TrimSpace(rest)
	}
	// Locate the { ... } effect block.
	open := strings.IndexByte(t, '{')
	closing := strings.LastIndexByte(t, '}')
	if open < 0 || closing < 0 || closing < open {
		return Reaction{}, errors.New("reaction: missing { ... } effect block")
	}
	effect := strings.TrimSpace(t[open+1 : closing])
	if effect == "" {
		return Reaction{}, errors.New("reaction: empty effect in { ... }")
	}
	// The two whitespace-separated tokens before the brace are eventKind and match.
	pre := strings.TrimSpace(t[:open])
	toks := strings.Fields(pre)
	r := Reaction{Effect: effect, Raw: raw}
	if len(toks) >= 1 {
		r.EventKind = toks[0]
	}
	if len(toks) >= 2 {
		r.Match = toks[1]
	}
	// "*" is the any-kind / any-text sentinel; normalize to empty.
	if r.EventKind == "*" {
		r.EventKind = ""
	}
	if r.Match == "*" {
		r.Match = ""
	}
	return r, nil
}

// Matches reports whether the reaction fires for the given event kind and text.
func (r Reaction) Matches(eventKind, eventText string) bool {
	if r.EventKind != "" {
		if !strings.Contains(strings.ToLower(eventKind), strings.ToLower(r.EventKind)) {
			return false
		}
	}
	if r.Match != "" {
		if !strings.Contains(strings.ToLower(eventText), strings.ToLower(r.Match)) {
			return false
		}
	}
	return true
}

// ReactionSet is an ordered collection of armed reactions.
type ReactionSet []Reaction

// Fired returns every reaction in the set whose Matches predicate is true for the event.
func (rs ReactionSet) Fired(eventKind, eventText string) []Reaction {
	out := make([]Reaction, 0, len(rs))
	for _, r := range rs {
		if r.Matches(eventKind, eventText) {
			out = append(out, r)
		}
	}
	return out
}

// RenderReactionLegend renders a one-per-line human legend of the armed reactions.
func RenderReactionLegend(rs ReactionSet) string {
	if len(rs) == 0 {
		return C("dim", "no reactions armed")
	}
	var b strings.Builder
	for i, r := range rs {
		if i > 0 {
			b.WriteByte('\n')
		}
		kind := r.EventKind
		if kind == "" {
			kind = "*"
		}
		match := r.Match
		if match == "" {
			match = "*"
		}
		b.WriteString(C("mut", "⟂"))
		b.WriteByte(' ')
		b.WriteString(C("dim", "on"))
		b.WriteByte(' ')
		b.WriteString(C("kind", kind))
		b.WriteByte(' ')
		b.WriteString(C("dim", "~"))
		b.WriteString(C("subj", match))
		b.WriteByte(' ')
		b.WriteString(C("dim", "→"))
		b.WriteByte(' ')
		b.WriteString(C("eff", r.Effect))
	}
	return b.String()
}
