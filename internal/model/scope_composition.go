package model

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/grammar"
	"github.com/hapax-systems/reins/internal/layout"
	"github.com/hapax-systems/reins/internal/relate"
)

// A frame owns only its lens and row cursors. Read data is shared; selection is
// never shared or persisted. Switching lenses retains both local positions.
type scopeFrame struct {
	page, taskCursor, eventCursor int
}

// Scope's reads poll every 2s/3s. This is a local receipt-age ceiling, not a
// claim about source freshness: a stopped poll cannot leave a live-looking join.
const scopeReadMaxAge = 15 * time.Second

func (m Model) EnableScope() Model {
	m.Scope = true
	m.scopeFrames = [2]scopeFrame{{page: PageTasks}, {page: PageEvents}}
	m.scopeFocus = 0
	m.scopeNow = time.Now()
	m.Mode, m.Input, m.Status = ModeNormal, "", ""
	m.PendingCommand = nil
	m.Sel = Selection{Rank: RankRow}
	return m
}

// scopeLens is a render-only value: no frame can mutate another's selection.
func (m Model) scopeLens(i int) Model {
	f := m.scopeFrames[i]
	m.Scope = false
	m.Page, m.Focus, m.EFocus = f.page, f.taskCursor, f.eventCursor
	m.Filter, m.CritFilter = "", ""
	m.Mode, m.Sel = ModeNormal, Selection{Rank: RankRow}
	return m
}

func (m Model) scopeReadState(page int) string {
	idx, dark := 0, m.TasksDark
	if page == PageEvents {
		idx, dark = 1, m.EventsDark
	}
	if dark {
		return "DARK: read unavailable"
	}
	if m.Demo {
		return "" // explicitly labelled fixture, never a live-read witness
	}
	at := m.scopeReadAt[idx]
	if at.IsZero() {
		return "DARK: read unobserved"
	}
	if m.scopeNow.Sub(at) > scopeReadMaxAge {
		return "DARK: read stale"
	}
	return ""
}

func (m Model) updateScope(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.scopeFrames[m.scopeFocus]
	switch keyName(v) {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab", "shift+tab":
		m.scopeFocus = 1 - m.scopeFocus
	case "t":
		f.page = PageTasks
	case "e":
		f.page = PageEvents
	case "a":
		m.AIR = !m.AIR
	case "j", "down", "k", "up", "g", "home", "G", "end":
		if m.scopeReadState(f.page) != "" {
			return m, nil
		}
		cursor, count := &f.taskCursor, len(m.Tasks)
		if f.page == PageEvents {
			cursor, count = &f.eventCursor, len(m.Events)
		}
		switch keyName(v) {
		case "j", "down":
			*cursor++
		case "k", "up":
			*cursor--
		case "g", "home":
			*cursor = 0
		case "G", "end":
			*cursor = count - 1
		}
		*cursor = clamp(*cursor, 0, max(0, count-1))
	}
	// All other keys are inert: Scope has no command, chat, yank or verb mode.
	return m, nil
}

func (m Model) scopeEntity(i int) (relate.Entity, bool) {
	lens := m.scopeLens(i)
	if m.scopeReadState(lens.Page) != "" {
		return relate.Entity{}, false
	}
	if lens.Page == PageTasks {
		t, ok := lens.FocusedTask()
		e := taskEntity(t, m.AIR)
		// Existing event-subject/task-reference equality, also used by the
		// session event neighborhood. No substring matching or reference parser.
		if t.TaskID != "" && (!m.AIR || t.AIR["task_id"] == "ok") {
			e.Facets["subject"] = t.TaskID
		}
		return e, ok
	}
	e, ok := lens.FocusedEvent()
	return eventEntity(e, m.AIR), ok
}

func (m Model) scopeConnector() layout.Connector {
	a, aok := m.scopeEntity(0)
	b, bok := m.scopeEntity(1)
	r := relate.Relation{Kind: "none"}
	if aok && bok {
		r = relate.Derive(a, []relate.Entity{b}, nil)
	}
	real := r.Kind == "facet"
	j := relate.Join{SrcKey: r.Facet, DstKey: r.Facet, Card: relate.CardOneToOne}
	if r.Facet == "subject" {
		if m.scopeFrames[0].page == PageTasks {
			j.SrcKey = "task_id"
		}
		if m.scopeFrames[1].page == PageTasks {
			j.DstKey = "task_id"
		}
	}
	v := relate.Gate(j, real, m.scopeFrames[0].page != m.scopeFrames[1].page, false, real)
	label := "unlinked"
	if real {
		label = m.airRelationLabel(r)
	}
	return layout.Connector{Glyph: grammar.C("border", "│"), Relation: label,
		Verdict: v.String(), JoinKey: j.JoinKeyFor(v)}
}

func (m Model) scopePane(i int) *layout.Spec {
	return layout.Leaf(&layout.Pane{MinW: 72, Render: func(w, h int) string {
		lens := m.scopeLens(i)
		name := "tasks"
		if lens.Page == PageEvents {
			name = "events"
		}
		label := fmt.Sprintf("frame %d · %s", i+1, name)
		if i == m.scopeFocus {
			label = "▸ " + label + " · focused"
		}
		rows := []string{fitWidth(grammar.C("brt", label), w)}
		body := ""
		if state := m.scopeReadState(lens.Page); state != "" {
			body = grammar.C("yel", state) // errors can contain private paths
		} else if lens.Page == PageTasks {
			body = lens.tasksListBody(w, h-1)
		} else if len(lens.Events) == 0 {
			body = grammar.C("mut", "no event rows")
		} else {
			body = lens.eventsListBody(w, h-1)
		}
		if m.scopeReadState(lens.Page) == "" && body != "" {
			// Reuse the row lens, but not Docket's filter/yank instructions:
			// those controls are deliberately absent from this read-only mode.
			count, cursor := len(m.Tasks), lens.Focus
			if lens.Page == PageEvents {
				count, cursor = len(m.Events), lens.EFocus
			}
			if count > 0 {
				_, rest, _ := strings.Cut(body, "\n")
				body = grammar.C("mut", fmt.Sprintf("%d rows · cursor %d/%d", count, cursor+1, count)) + "\n" + rest
			}
		}
		rows = append(rows, fitBlock(body, w, max(0, h-1))...)
		return strings.Join(rows, "\n")
	}})
}

func (m Model) scopeView(w, h int) string {
	c := m.scopeConnector()
	air := "LOCAL"
	if m.AIR {
		air = "AIR"
	}
	title := "SCOPE · " + air
	if m.Demo {
		title += " · DEMO fixture not live"
	}
	status := c.Relation + " · no join"
	if c.JoinKey != "" {
		status = c.Relation + " · ⋈ " + c.JoinKey
	}
	var spec *layout.Spec
	if m.paneCap() < 2 || w < 145 {
		// Two frames remain in the model; one is visible under the existing
		// device ceiling. Keep the selected frame, not a fixed primary.
		spec = m.scopePane(m.scopeFocus)
		title += fmt.Sprintf(" · frame %d/2", m.scopeFocus+1)
	} else {
		spec = layout.Split(m.scopePane(0), m.scopePane(1), 0.5, c)
	}
	rows := []string{fitWidth(grammar.C("brt", title), w)}
	if h > 3 {
		rows = append(rows, strings.Split(layout.Render(spec, w, h-3), "\n")...)
	}
	rows = append(rows, fitWidth(grammar.C("mut", status), w),
		fitWidth(grammar.C("mut", "Tab focus · t/e lens · j/k row · a AIR · q quit"), w))
	return strings.Join(fitBlock(strings.Join(rows, "\n"), w, h), "\n")
}
