package model

import (
	"fmt"
	"strings"

	"github.com/hapax-systems/reins/internal/grammar"
	"github.com/hapax-systems/reins/internal/layout"
	"github.com/hapax-systems/reins/internal/relate"
)

// A local cursor over existing typed fields, not a new ledger identity/schema.
type commandReference struct{ field, value string }

func (m Model) focusedWitness() (grammar.Command, bool) {
	if m.CommandsDark || m.commandLedgerFocus < 0 || m.commandLedgerFocus >= len(m.Commands) {
		return grammar.Command{}, false
	}
	return m.Commands[m.commandLedgerFocus], true
}

func commandProjectionKey(c grammar.Command) [4]string {
	return [4]string{c.Verb, c.Target, c.TaskID, c.SessionRole}
}

// The projection has no command_id. Retain a unique projected tuple across a
// fold, but never pretend duplicate tuples identify the same ledger event.
func (m Model) commandFoldFocus(next []grammar.Command, dark bool) int {
	if dark || len(next) == 0 {
		return -1
	}
	if len(m.Commands) == 0 {
		return 0
	}
	c, ok := m.focusedWitness()
	if !ok {
		return -1
	}
	key, oldCount, newCount, index := commandProjectionKey(c), 0, 0, -1
	for _, row := range m.Commands {
		if commandProjectionKey(row) == key {
			oldCount++
		}
	}
	for i, row := range next {
		if commandProjectionKey(row) == key {
			newCount++
			index = i
		}
	}
	if oldCount != 1 || newCount != 1 {
		return -1
	}
	return index
}

func (m Model) commandCarries(c grammar.Command, ref commandReference) bool {
	if m.CommandsDark || ref.value == "" || (m.AIR && c.AIR[ref.field] != "ok") {
		return false
	}
	if ref.field == "task_id" {
		return c.TaskID == ref.value
	}
	return ref.field == "session_role" && c.SessionRole == ref.value
}

// Require an unambiguous destination and AIR permission on BOTH ends. A target
// string, owner, prefix or unrelated selected session is never a reference.
func (m Model) commandReferenceValid(ref commandReference) bool {
	n := 0
	switch ref.field {
	case "task_id":
		if m.TasksDark {
			return false
		}
		for _, t := range m.Tasks {
			if t.TaskID == ref.value {
				n++
				if m.AIR && t.AIR["task_id"] != "ok" {
					return false
				}
			}
		}
	case "session_role":
		if m.SessionsDark {
			return false
		}
		for _, s := range m.Sessions {
			if s.Role == ref.value {
				n++
				if m.AIR && s.AIR["role"] != "ok" {
					return false
				}
			}
		}
	}
	return ref.value != "" && n == 1
}

func (m Model) commandReferences() []commandReference {
	var refs []commandReference
	seen := map[commandReference]bool{}
	for _, c := range m.Commands {
		for _, ref := range []commandReference{{"task_id", c.TaskID}, {"session_role", c.SessionRole}} {
			if !seen[ref] && m.commandCarries(c, ref) && m.commandReferenceValid(ref) {
				refs = append(refs, ref)
				seen[ref] = true
			}
		}
	}
	return refs
}

func (m Model) selectedCommandReference() commandReference {
	refs := m.commandReferences()
	if m.commandRefActive {
		for _, ref := range refs {
			if ref == m.commandRefFocus {
				return ref
			}
		}
		return commandReference{}
	}
	c, ok := m.focusedWitness()
	if ok {
		for _, ref := range refs {
			if m.commandCarries(c, ref) {
				return ref
			}
		}
	}
	return commandReference{}
}

func (m Model) commandConnector() layout.Connector {
	c, ok := m.focusedWitness()
	ref := m.selectedCommandReference()
	r := relate.Relation{Kind: "none"}
	if ok && m.commandCarries(c, ref) && m.commandReferenceValid(ref) {
		r = relate.Derive(relate.Entity{Facets: map[string]string{ref.field: ref.value}},
			[]relate.Entity{{Facets: map[string]string{ref.field: ref.value}}}, nil)
	}
	real := r.Kind == "facet"
	dst := ref.field
	if dst == "session_role" {
		dst = "role"
	}
	j := relate.Join{SrcKey: ref.field, DstKey: dst, Card: relate.CardOneToMany}
	v := relate.Gate(j, real, true, false, real)
	label := "unlinked command references"
	if real {
		label = m.airRelationLabel(r)
	}
	return layout.Connector{Relation: label, Verdict: v.String(), JoinKey: j.JoinKeyFor(v)}
}

func (m Model) composeCommands(w int) *layout.Spec {
	if m.commandCatalog {
		primary := layout.Leaf(&layout.Pane{Render: func(w, h int) string { return m.referenceSlice(w, h) }})
		detail := layout.Leaf(&layout.Pane{Render: func(w, h int) string { return m.commandCatalogDetail(w) }})
		if m.commandCatalogVerb != "" {
			return layout.Split(layout.Leaf(&layout.Pane{Render: m.commandLedgerBody}), detail, 0.62, layout.Connector{})
		}
		left, _, _ := m.referenceLayoutWidths(w)
		return layout.Split(primary, detail, float64(left)/float64(max(2, w-1)), layout.Connector{})
	}
	return layout.Split(
		layout.Leaf(&layout.Pane{Render: m.commandLedgerBody}),
		layout.Leaf(&layout.Pane{Render: m.commandReferenceBody}),
		0.62, m.commandConnector())
}

func (m Model) commandLedgerBody(w, h int) string {
	var b strings.Builder
	enf, integrity := firstNonEmpty(m.CommandsEnforcement, "absent"), firstNonEmpty(m.CommandsIntegrity, "unknown")
	tok := "fch"
	if integrity == "verified" {
		tok = "pri"
	}
	fmt.Fprintf(&b, " %s — enforcement %s · integrity %s\n", grammar.C("brt", "WITNESSED LEDGER"), enf, grammar.C(tok, integrity))
	b.WriteString(" j/k row · Tab refs · Enter catalog ref · c browse catalog\n")
	if m.CommandsDark {
		b.WriteString(" · dark — command ledger unreachable\n")
	} else if len(m.Commands) == 0 {
		b.WriteString(" · no commands witnessed yet\n")
	} else {
		visible := max(1, h-4)
		start := clamp(m.commandLedgerFocus-visible+1, 0, max(0, len(m.Commands)-visible))
		end := min(len(m.Commands), start+visible)
		fmt.Fprintf(&b, " rows %d-%d/%d · %d earlier / %d later hidden\n", start+1, end, len(m.Commands), start, len(m.Commands)-end)
		ref := m.selectedCommandReference()
		for i := start; i < end; i++ {
			mark := " "
			if m.commandCarries(m.Commands[i], ref) {
				mark = "├"
			}
			if i == m.commandLedgerFocus {
				mark = "▶"
			}
			b.WriteString(mark + " " + clipRunes(m.renderWitnessRow(m.Commands[i]), max(1, w-2)) + "\n")
		}
	}
	return b.String()
}

func (m Model) commandReferenceBody(w, h int) string {
	var b strings.Builder
	b.WriteString(" TYPED REFERENCES · Tab switches cursor\n")
	refs, selected := m.commandReferences(), m.selectedCommandReference()
	if len(refs) == 0 {
		return b.String() + " no available task/session join; catalog: c\n"
	}
	index := 0
	for i, ref := range refs {
		if ref == selected {
			index = i
		}
	}
	visible := max(1, h-3)
	start := clamp(index-visible+1, 0, max(0, len(refs)-visible))
	end := min(len(refs), start+visible)
	fmt.Fprintf(&b, " %d earlier / %d later references hidden\n", start, len(refs)-end)
	for _, ref := range refs[start:end] {
		mark := " "
		if ref == selected {
			mark = "▶"
		}
		// Names may be shown only after commandReferenceValid + commandCarries
		// have checked both AIR maps. Existing datom renderers own row contents.
		if ref.field == "task_id" {
			for _, t := range m.Tasks {
				if t.TaskID == ref.value {
					b.WriteString(mark + " " + grammar.RenderTaskRow(t, m.AIR) + "\n")
				}
			}
		} else {
			for _, s := range m.Sessions {
				if s.Role == ref.value {
					b.WriteString(mark + " " + grammar.RenderSessionRow(s, m.AIR) + "\n")
				}
			}
		}
	}
	return b.String()
}

// Ledger verbs refer to governed contracts, NEVER the same-spelled UI window.
func commandCatalogKnown(verb string) bool {
	if _, ok := governedVerbSpecs[verb]; ok {
		return true
	}
	for _, arg := range lookupIntentArgs() {
		if arg.Label == verb {
			return true
		}
	}
	return false
}

func (m Model) commandCatalogDetail(w int) string {
	if m.commandCatalogVerb == "" {
		return m.referenceContextPane(w)
	}
	c, ok := m.focusedWitness()
	if !ok || c.Verb != m.commandCatalogVerb || (m.AIR && c.AIR["verb"] != "ok") || !commandCatalogKnown(c.Verb) {
		return " catalog reference unavailable · Esc returns to ledger"
	}
	if _, ok := governedVerbSpecs[c.Verb]; ok {
		return " GOVERNED CATALOG REF\n" + m.governedVerbPreviewFor(c.Verb, c.Target)
	}
	v, _ := lookupVerb("intent")
	return " INTENT CATALOG REF · " + c.Verb + "\n" + v.authority + "\n" + v.preflight + "\n" + v.receipt + " · no effect emitted"
}

func (m Model) updateCommands(key string) (Model, bool) {
	if m.commandCatalog {
		if key == "esc" {
			m.commandCatalog = false
			m.commandCatalogVerb = ""
			return m, true
		}
		return m, false // existing catalog cursor/navigation remains available
	}
	switch key {
	case "enter":
		c, ok := m.focusedWitness()
		if !ok || (m.AIR && c.AIR["verb"] != "ok") || !commandCatalogKnown(c.Verb) {
			m.Status = "catalog reference unavailable; use c to browse"
			return m, true
		}
		m.commandCatalog, m.commandCatalogVerb = true, c.Verb
		m.Status = "catalog reference · inspection only · Esc returns"
		return m, true
	case "c":
		m.commandCatalog, m.commandCatalogVerb = true, ""
		return m, true
	case "tab":
		ref := m.selectedCommandReference()
		m.commandRefActive = !m.commandRefActive
		m.commandRefFocus = ref
		return m, true
	case "esc":
		m.commandRefActive = false
		return m, true
	case "j", "down", "k", "up", "g", "G":
		count, index := len(m.Commands), m.commandLedgerFocus
		refs := m.commandReferences()
		if m.commandRefActive {
			count, index = len(refs), -1
			for i, ref := range refs {
				if ref == m.commandRefFocus {
					index = i
				}
			}
		}
		if count == 0 || m.CommandsDark {
			return m, true
		}
		switch key {
		case "j", "down":
			index++
		case "k", "up":
			index--
		case "g":
			index = 0
		case "G":
			index = count - 1
		}
		index = clamp(index, 0, count-1)
		if m.commandRefActive {
			m.commandRefFocus = refs[index]
			for i, c := range m.Commands {
				if m.commandCarries(c, refs[index]) {
					m.commandLedgerFocus = i
					break
				}
			}
		} else {
			m.commandLedgerFocus = index
		}
		m.Status = "witnessed ledger · read-only selection"
		return m, true
	}
	return m, false
}
