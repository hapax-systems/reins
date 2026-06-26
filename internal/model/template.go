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
	if strings.HasPrefix(key, "ring.") {
		if i, err := strconv.Atoi(strings.TrimPrefix(key, "ring.")); err == nil && i >= 0 && i < len(m.Ring) {
			return ringValue(m.Ring[i], m.AIR), true
		}
	}

	page := m.commandSelectionPage()

	if page == PageEvents {
		ev, has := m.FocusedEvent()
		field := func(f string) (string, bool) {
			if !has {
				return "", false
			}
			if f == "id" {
				f = "subject"
			}
			var val string
			switch f {
			case "ts":
				val = ev.TS
			case "kind":
				val = ev.Kind
			case "subject":
				val = ev.Subject
			case "actor":
				val = ev.Actor
			case "summary":
				val = ev.Summary
			default:
				return "", false
			}
			return eventFieldValueForAir(ev, f, val, m.AIR), true
		}
		selectedField := m.selectedFieldForPage(page, "subject")
		switch {
		case key == "sel", key == "sel.value":
			return field(selectedField)
		case key == "sel.id", key == "focus":
			return field("subject")
		case key == "sel.field":
			return selectedField, true
		case strings.HasPrefix(key, "sel."):
			return field(strings.TrimPrefix(key, "sel."))
		}
		return "", false
	}

	if page == PageSessions {
		s, has := m.FocusedSession()
		field := func(f string) (string, bool) {
			if !has {
				return "", false
			}
			if f == "id" {
				f = "role"
			}
			switch f {
			case "role", "session", "platform", "state", "readiness", "blocker", "attention", "alive", "idle", "stalled", "claimed_task", "route_id", "mode", "profile", "route_binding_state", "route_evidence_ref", "output_age_s", "relay_age_s":
				return sessionFieldValueForAir(s, f, m.AIR), true
			default:
				return "", false
			}
		}
		selectedField := m.selectedFieldForPage(page, "role")
		switch {
		case key == "sel", key == "sel.value":
			return field(selectedField)
		case key == "sel.id", key == "focus":
			return field("role")
		case key == "sel.field":
			return selectedField, true
		case strings.HasPrefix(key, "sel."):
			return field(strings.TrimPrefix(key, "sel."))
		}
		return "", false
	}

	if page == PageCaps {
		c, has := m.FocusedCapabilityRow()
		field := func(f string) (string, bool) {
			if !has {
				return "", false
			}
			if f == "id" {
				f = "capability"
			}
			switch f {
			case "capability", "capability_id", "name":
				return c.Name, true
			case "status":
				return c.Status, true
			case "authority", "meaning", "routing_meaning", "evidence_posture":
				return c.Authority, true
			case "class", "capability_class":
				return c.Class, true
			case "family", "surface_family":
				return c.Family, true
			case "spend", "spend_model":
				return c.Spend, true
			case "egress", "egress_class":
				return c.Egress, true
			case "receipt", "receipt_requirement":
				return c.Receipt, true
			case "evidence":
				return c.Evidence, true
			case "missing", "blocker":
				return c.Missing, true
			case "source", "source_refs":
				return c.SourceRefs, true
			default:
				return "", false
			}
		}
		selectedField := m.selectedFieldForPage(page, "capability")
		switch {
		case key == "sel", key == "sel.value":
			return field(selectedField)
		case key == "sel.id", key == "focus":
			return field("capability")
		case key == "sel.field":
			return selectedField, true
		case strings.HasPrefix(key, "sel."):
			return field(strings.TrimPrefix(key, "sel."))
		}
		return "", false
	}

	if page == PageSurfaces {
		surf, has := m.FocusedSurface()
		field := func(f string) (string, bool) {
			if !has {
				return "", false
			}
			if f == "id" {
				f = "surface"
			}
			switch f {
			case "surface", "surface_id":
				return surf.ID, true
			case "name":
				return surf.Name, true
			case "open":
				return surf.Open, true
			case "exit":
				return surf.Exit, true
			case "scope":
				return surf.Scope, true
			case "kind":
				return surf.Kind, true
			case "glyph":
				return surfaceKindGlyph(surf.Kind), true
			case "air":
				return surf.AIR, true
			case "contract":
				return surf.Contract, true
			default:
				return "", false
			}
		}
		switch {
		case key == "sel", key == "sel.value", key == "sel.id", key == "focus":
			return field("surface")
		case key == "sel.field":
			return "surface", true
		case strings.HasPrefix(key, "sel."):
			return field(strings.TrimPrefix(key, "sel."))
		}
		return "", false
	}

	if page == PageDomains {
		if row, has := m.FocusedDomainRow(); has {
			field := func(f string) (string, bool) {
				if f == "id" {
					f = "domain"
				}
				switch f {
				case "domain", "domain_id":
					return domainRowFieldForAir(row, "domain_id", m.AIR), true
				case "label":
					return domainRowFieldForAir(row, "label", m.AIR), true
				case "lifecycle":
					return domainRowFieldForAir(row, "lifecycle", m.AIR), true
				case "terrain":
					return domainRowFieldForAir(row, "terrain", m.AIR), true
				case "depth":
					return domainRowFieldForAir(row, "depth", m.AIR), true
				case "scope":
					return domainRowFieldForAir(row, "scope", m.AIR), true
				case "state":
					return domainRowFieldForAir(row, "state", m.AIR), true
				case "authority", "authority_ceiling":
					return domainRowFieldForAir(row, "authority_ceiling", m.AIR), true
				case "claim", "claim_ceiling":
					return domainRowFieldForAir(row, "claim_ceiling", m.AIR), true
				case "windows":
					return domainRowFieldForAir(row, "windows", m.AIR), true
				case "surfaces":
					return domainRowFieldForAir(row, "surfaces", m.AIR), true
				case "parity":
					return domainRowFieldForAir(row, "parity", m.AIR), true
				case "evidence", "evidence_count":
					return domainRowFieldForAir(row, "evidence_count", m.AIR), true
				case "blocker", "missing":
					return domainRowFieldForAir(row, "blocker", m.AIR), true
				case "source", "source_refs":
					return domainRowFieldForAir(row, "source_refs", m.AIR), true
				default:
					return "", false
				}
			}
			switch {
			case key == "sel", key == "sel.value", key == "sel.id", key == "focus":
				return field("domain")
			case key == "sel.field":
				return "domain", true
			case strings.HasPrefix(key, "sel."):
				return field(strings.TrimPrefix(key, "sel."))
			}
			return "", false
		}

		d, has := m.FocusedDomain()
		field := func(f string) (string, bool) {
			if !has {
				return "", false
			}
			if f == "id" {
				f = "domain"
			}
			switch f {
			case "domain", "domain_id":
				return d.ID, true
			case "terrain":
				return d.Terrain, true
			case "depth":
				return d.Depth, true
			case "scope":
				return d.Scope, true
			case "windows":
				return d.Windows, true
			case "surfaces":
				return d.Surfaces, true
			case "parity":
				return d.Parity, true
			default:
				return "", false
			}
		}
		switch {
		case key == "sel", key == "sel.value", key == "sel.id", key == "focus":
			return field("domain")
		case key == "sel.field":
			return "domain", true
		case strings.HasPrefix(key, "sel."):
			return field(strings.TrimPrefix(key, "sel."))
		}
		return "", false
	}

	if page != PageTasks {
		return "", false
	}

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
	}
	return "", false
}

func (m Model) selectedPasteValue() (field, value string, ok bool) {
	if m.Sel.Rank != RankField || m.Sel.Field == "" {
		return "", "", false
	}
	field = m.Sel.Field
	switch m.commandSelectionPage() {
	case PageEvents:
		ev, has := m.FocusedEvent()
		if !has {
			return "", "", false
		}
		switch field {
		case "ts":
			value = eventFieldValueForAir(ev, field, ev.TS, m.AIR)
		case "kind":
			value = eventFieldValueForAir(ev, field, ev.Kind, m.AIR)
		case "subject":
			value = eventFieldValueForAir(ev, field, ev.Subject, m.AIR)
		case "actor":
			value = eventFieldValueForAir(ev, field, ev.Actor, m.AIR)
		case "summary":
			value = eventFieldValueForAir(ev, field, ev.Summary, m.AIR)
		default:
			return "", "", false
		}
	case PageSessions:
		s, has := m.FocusedSession()
		if !has {
			return "", "", false
		}
		value = sessionFieldValueForAir(s, field, m.AIR)
		if value == "" {
			return "", "", false
		}
	case PageTasks:
		t, has := m.FocusedTask()
		if !has {
			return "", "", false
		}
		value = taskFieldValueForAir(t, field, m.AIR)
	default:
		return "", "", false
	}
	return field, value, true
}

// templateKeys: the offerable references (kept small + ordered for the fish menu).
var templateKeys = []string{"sel", "sel.id", "sel.field", "sel.value", "sel.status", "sel.meaning", "sel.authority", "sel.family", "sel.receipt", "sel.source_refs", "sel.missing", "sel.kind", "sel.contract", "sel.crit", "focus", "ring.0"}

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
