package model

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/grammar"
)

const (
	PageEvents = 0
	PageTasks  = 1
)

type Model struct {
	Title      string
	Page       int
	Events     []grammar.Event
	Tasks      []grammar.Task
	EventsDark bool
	TasksDark  bool
	AIR        bool // the AIR lens
}

func New(title string) Model { return Model{Title: title} }

// Fold: the pure projection for the :events page. No hidden state; re-folding restores the view.
func (m Model) Fold(evs []grammar.Event, dark bool) Model {
	m.Events = evs
	m.EventsDark = dark
	return m
}

// FoldTasks: the pure projection for the :tasks registry page.
func (m Model) FoldTasks(ts []grammar.Task, dark bool) Model {
	m.Tasks = ts
	m.TasksDark = dark
	return m
}

func (m Model) Init() tea.Cmd { return nil }

// EventsMsg / TasksMsg carry fresh fetches into Update (sent by cmd/reins on each tick).
type EventsMsg struct {
	Events []grammar.Event
	Dark   bool
}
type TasksMsg struct {
	Tasks []grammar.Task
	Dark  bool
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case EventsMsg:
		return m.Fold(v.Events, v.Dark), nil
	case TasksMsg:
		return m.FoldTasks(v.Tasks, v.Dark), nil
	case tea.KeyMsg:
		switch v.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "a": // toggle the AIR lens
			m.AIR = !m.AIR
			return m, nil
		case "1": // :events page (re-target the same frame, never a tab)
			m.Page = PageEvents
			return m, nil
		case "2": // :tasks registry page
			m.Page = PageTasks
			return m, nil
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	mode := "LOCAL"
	if m.AIR {
		mode = "AIR ▮"
	}
	pageName, n, dark := "events", len(m.Events), m.EventsDark
	if m.Page == PageTasks {
		pageName, n, dark = "tasks", len(m.Tasks), m.TasksDark
	}
	// vital frame: status bar (trouble-appears: dark shows here)
	status := fmt.Sprintf("%s  %s  :%s  n:%d", m.Title, mode, pageName, n)
	if dark {
		status += "  ‼ dark"
	}
	b.WriteString(status + "\n")
	b.WriteString(strings.Repeat("─", 64) + "\n")
	if dark {
		b.WriteString("  (spine dark — no fabricated data)\n")
	}
	// the active page (re-targeted body, never a tab swap)
	if m.Page == PageTasks {
		b.WriteString(grammar.RenderTaskHeader() + "\n")
		for _, t := range m.Tasks {
			b.WriteString(grammar.RenderTaskRow(t, m.AIR) + "\n")
		}
	} else {
		for _, ev := range m.Events {
			b.WriteString(grammar.RenderEventRow(ev, m.AIR) + "\n")
		}
	}
	b.WriteString(strings.Repeat("─", 64) + "\n")
	b.WriteString("[1]events [2]tasks  [a]AIR  [q]quit")
	return b.String()
}
