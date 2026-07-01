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
	// mid-reference: an open `{{…` token → offer the selection-template refs (with live previews).
	if prefix, open := openTemplatePrefix(m.Input); open {
		return m.templateCandidates(prefix)
	}

	fields := strings.Fields(m.Input)
	trailingSpace := strings.HasSuffix(m.Input, " ")

	var out []Candidate
	// dynamic-on-selection: inject the selected field's live value only when paste is what the user
	// is asking for. A typed command must never be preempted by the current selection.
	if pasteCandidateAllowed(m.Input) {
		if field, v, ok := m.selectedPasteValue(); ok {
			out = append(out, Candidate{Label: pastePrefix + field, Value: v, Detail: "inject ‹" + clip(v, 32) + "›"})
		}
	}

	// ARG level: a verb is fully typed and we're now completing its argument.
	if len(fields) >= 1 && (trailingSpace || len(fields) >= 2) {
		if vd, ok := lookupVerb(fields[0]); ok {
			if len(vd.args) > 0 {
				argPrefix := ""
				if !trailingSpace && len(fields) >= 2 {
					argPrefix = fields[len(fields)-1]
				}
				for rank := 0; rank <= maxTokenCompletionRank; rank++ {
					for _, a := range vd.args {
						if tokenCompletionRank(a.Label, argPrefix) != rank {
							continue
						}
						a.Value = vd.name + " " + a.Label // accept RUNS the full command
						out = append(out, a)
					}
				}
				return out
			}
			if vd.freeform {
				return out
			}
		}
		return out
	}

	// VERB level: prefix-match the verb table first, then fall back to fuzzy subsequences.
	prefix := ""
	if len(fields) == 1 && !trailingSpace {
		prefix = fields[0]
	}
	for rank := 0; rank <= maxVerbCompletionRank; rank++ {
		for _, v := range verbs {
			if verbCompletionRank(v, prefix) != rank {
				continue
			}
			c := Candidate{Label: v.name, Detail: v.detail()}
			if len(v.args) > 0 {
				c.Sub = v.args // signals "descend on accept"
			} else {
				c.Value = v.name // leaf: accept RUNS it
			}
			out = append(out, c)
		}
	}
	return out
}

const (
	tokenCompletionNoMatch = -1
	maxTokenCompletionRank = 1

	verbCompletionNoMatch = -1
	maxVerbCompletionRank = 3
)

func tokenCompletionRank(label, q string) int {
	switch {
	case q == "", strings.HasPrefix(label, q):
		return 0
	case fuzzySubsequence(label, q):
		return 1
	default:
		return tokenCompletionNoMatch
	}
}

func verbCompletionRank(v verbDef, q string) int {
	if q == "" {
		return 0
	}
	if strings.HasPrefix(v.name, q) {
		return 0
	}
	for _, a := range v.aliases {
		if strings.HasPrefix(a, q) {
			return 1
		}
	}
	if fuzzySubsequence(v.name, q) {
		return 2
	}
	for _, a := range v.aliases {
		if fuzzySubsequence(a, q) {
			return 3
		}
	}
	return verbCompletionNoMatch
}

func fuzzySubsequence(s, q string) bool {
	if q == "" {
		return true
	}
	qr := []rune(q)
	i := 0
	for _, r := range s {
		if r != qr[i] {
			continue
		}
		i++
		if i == len(qr) {
			return true
		}
	}
	return false
}

func pasteCandidateAllowed(input string) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return true
	}
	fields := strings.Fields(trimmed)
	if len(fields) != 1 {
		return false
	}
	return strings.HasPrefix("paste", fields[0])
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
		label := taskFieldValueForAir(t, "task_id", m.AIR)
		if m.AIR && label == "▒▒▒" {
			continue
		}
		if seen[t.TaskID] {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(t.TaskID), q) {
			continue
		}
		seen[t.TaskID] = true
		out = append(out, Candidate{Label: label, Value: t.TaskID, Detail: taskFieldValueForAir(t, "criticality", m.AIR)})
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
	if commandInputShouldRunTyped(m.Input) {
		return m.Exec(m.Input)
	}
	c, ok := m.curCandidate()
	if !ok {
		return m.Exec(m.Input)
	}
	switch {
	case strings.HasPrefix(c.Label, "{{"): // a template ref → fill the open fragment, keep composing
		m.Input, m.CompIdx = fillOpenTemplate(m.Input, c.Value), 0
		return m
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

func commandInputShouldRunTyped(input string) bool {
	if _, open := openTemplatePrefix(input); open {
		return false
	}
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return false
	}
	vd, ok := lookupVerb(fields[0])
	if !ok {
		return false
	}
	if vd.freeform && len(fields) >= 2 {
		return true
	}
	return len(fields) == 1 && !strings.HasSuffix(input, " ") && len(vd.args) == 0
}

// fillCompletion — [→]: accept INTO the line (descend or fill the input), never run. Fish's accept
// semantics: the line reflects the chosen completion; you still press Enter to execute.
func (m Model) fillCompletion() Model {
	c, ok := m.curCandidate()
	if !ok {
		return m
	}
	switch {
	case strings.HasPrefix(c.Label, "{{"):
		m.Input = fillOpenTemplate(m.Input, c.Value)
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
