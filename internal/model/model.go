package model

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/grammar"
)

type Model struct {
	Title  string
	Events []grammar.Event
	Dark   bool
	Err    string
	AIR    bool // the AIR lens
}

func New(title string) Model { return Model{Title: title} }

// Fold: the pure projection — events -> Model. No hidden state; re-folding restores the view (hot-reload).
func (m Model) Fold(evs []grammar.Event, dark bool) Model {
	m.Events = evs
	m.Dark = dark
	return m
}

func (m Model) Init() tea.Cmd { return nil }

// EventsMsg carries a fresh fetch into Update (sent by cmd/reins on each tick / file-change).
type EventsMsg struct {
	Events []grammar.Event
	Dark   bool
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case EventsMsg:
		return m.Fold(v.Events, v.Dark), nil
	case tea.KeyMsg:
		switch v.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "a": // toggle the AIR lens
			m.AIR = !m.AIR
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
	// vital frame: status bar (trouble-appears: dark shows here)
	status := fmt.Sprintf("%s  %s  events:%d", m.Title, mode, len(m.Events))
	if m.Dark {
		status += "  ‼ dark"
	}
	b.WriteString(status + "\n")
	b.WriteString(strings.Repeat("─", 64) + "\n")
	// the :events page
	if m.Dark {
		b.WriteString("  (spine dark — no fabricated data)\n")
	}
	for _, ev := range m.Events {
		b.WriteString(grammar.RenderEventRow(ev, m.AIR) + "\n")
	}
	b.WriteString(strings.Repeat("─", 64) + "\n")
	b.WriteString("[a] AIR lens  [q] quit  :events")
	return b.String()
}
