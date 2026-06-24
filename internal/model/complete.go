package model

import "strings"

// Candidate is one option in the fish-style navigable completion menu.
//
//	Label  — what's shown in the strip.
//	Value  — the text Exec'd / inserted when this LEAF is accepted.
//	Detail — the dim right-column gloss (fish's description of the highlighted item).
//	Sub    — when non-empty, this is a SUB-MENU node: accepting it DESCENDS (reveals Sub) instead
//	         of running. Descent is modelled by the INPUT string itself (append "label ") so the
//	         candidate tree re-derives — no separate path state, mirroring how a shell line works.
//
// The same Candidate/engine serves every input surface (the [:] command line today; the [/] filter
// and the future session pane next) — one Completer, reused, never bespoke per surface.
type Candidate struct {
	Label  string
	Value  string
	Detail string
	Sub    []Candidate
}

const pastePrefix = "paste " // a dynamic-on-selection candidate that injects, never runs

// completionTree builds the candidate list for the CURRENT input + active selection. This is the one
// Completer; surfaces dispatch into it by mode. Dynamic on selection: when a field is selected, a
// `paste <field>` candidate LEADS (the {{sel}} injection seed).
func (m Model) completionTree() []Candidate {
	switch m.Mode {
	case ModeFilter:
		return m.filterCandidates()
	default:
		return m.commandCandidates()
	}
}

// commandCandidates: the [:] command line. Two levels emerge from the input string —
//   - completing the VERB (≤1 token, no trailing space): prefix-matched verbs, each carrying its
//     gloss as Detail; a verb WITH args gets Sub set (so accept descends).
//   - completing an ARG (verb + trailing space, or a partial 2nd token): that verb's arg sub-menu,
//     prefix-matched; each arg's Value is the full "verb arg" line (so accept RUNS it).
func (m Model) commandCandidates() []Candidate {
	var out []Candidate
	// dynamic-on-selection: inject the selected field's live value (leads the list).
	if t, ok := m.FocusedTask(); ok && m.Sel.Rank == RankField && m.Sel.Field != "" {
		v := fieldValue(t, m.Sel.Field)
		out = append(out, Candidate{Label: pastePrefix + m.Sel.Field, Value: v, Detail: "inject ‹" + clip(v, 32) + "›"})
	}

	fields := strings.Fields(m.Input)
	trailingSpace := strings.HasSuffix(m.Input, " ")

	// ARG level: a verb is fully typed and we're now completing its argument.
	if len(fields) >= 1 && (trailingSpace || len(fields) >= 2) {
		if vd, ok := lookupVerb(fields[0]); ok && len(vd.args) > 0 {
			argPrefix := ""
			if !trailingSpace && len(fields) >= 2 {
				argPrefix = fields[len(fields)-1]
			}
			for _, a := range vd.args {
				if strings.HasPrefix(a.Label, argPrefix) {
					a.Value = vd.name + " " + a.Label // accept RUNS the full command
					out = append(out, a)
				}
			}
			return out
		}
	}

	// VERB level: prefix-match the verb table.
	prefix := ""
	if len(fields) == 1 && !trailingSpace {
		prefix = fields[0]
	}
	for _, v := range verbs {
		if !strings.HasPrefix(v.name, prefix) {
			continue
		}
		c := Candidate{Label: v.name, Detail: v.gloss}
		if len(v.args) > 0 {
			c.Sub = v.args // signals "descend on accept"
		} else {
			c.Value = v.name // leaf: accept RUNS it
		}
		out = append(out, c)
	}
	return out
}

// filterCandidates: the [/] filter surface. Same engine — completing the filter offers the live task
// ids (prefix-matched, the substring the filter narrows on) plus the criticality classes as a quick
// sub-set. Accepting a candidate fills the filter; the list re-narrows live.
func (m Model) filterCandidates() []Candidate {
	q := strings.ToLower(strings.TrimSpace(m.Filter))
	var out []Candidate
	seen := map[string]bool{}
	for _, t := range m.Tasks {
		if len(out) >= 8 {
			break
		}
		if seen[t.TaskID] {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(t.TaskID), q) {
			continue
		}
		seen[t.TaskID] = true
		c := t.Criticality
		if c == "" {
			c = "ok"
		}
		out = append(out, Candidate{Label: t.TaskID, Value: t.TaskID, Detail: c})
	}
	return out
}

// curCandidate returns the highlighted candidate at the current level (and whether one exists).
func (m Model) curCandidate() (Candidate, bool) {
	cs := m.completionTree()
	if len(cs) == 0 {
		return Candidate{}, false
	}
	return cs[m.CompIdx%len(cs)], true
}

// acceptCompletion — [Enter]: a leaf RUNS (Exec); a Sub node DESCENDS; a paste candidate INJECTS.
// With no candidates, Enter runs whatever is typed.
func (m Model) acceptCompletion() Model {
	c, ok := m.curCandidate()
	if !ok {
		return m.Exec(m.Input)
	}
	switch {
	case strings.HasPrefix(c.Label, pastePrefix):
		m.Input, m.CompIdx = c.Value, 0
		return m
	case len(c.Sub) > 0:
		m.Input, m.CompIdx = c.Label+" ", 0 // descend: re-derive candidates as the sub-menu
		return m
	default:
		return m.Exec(c.Value)
	}
}

// fillCompletion — [→]: accept INTO the line (descend or fill the input), never run. Fish's accept
// semantics: the line reflects the chosen completion; you still press Enter to execute.
func (m Model) fillCompletion() Model {
	c, ok := m.curCandidate()
	if !ok {
		return m
	}
	switch {
	case strings.HasPrefix(c.Label, pastePrefix):
		m.Input = c.Value
	case len(c.Sub) > 0:
		m.Input = c.Label + " " // descend
	default:
		m.Input = c.Value
	}
	m.CompIdx = 0
	return m
}

// clip truncates s to n runes with an ellipsis (for the paste Detail gloss).
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
