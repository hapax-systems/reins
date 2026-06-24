package model

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/grammar"
)

const (
	PageEvents   = 0
	PageTasks    = 1
	PageDynamics = 2
	PageHelp     = 3
)

const (
	ModeNormal  = 0 // hotkeys + page navigation
	ModeCommand = 1 // the command line is focused (typing a verb)
)

type Model struct {
	Title        string
	Page         int
	Events       []grammar.Event
	Tasks        []grammar.Task
	Dynamics     grammar.Graph
	EventsDark   bool
	TasksDark    bool
	DynamicsDark bool
	AIR          bool // the AIR lens
	Mode         int  // ModeNormal | ModeCommand
	Input        string
	Status       string // last command result / error (one line, above the hint)
	Quitting     bool   // Exec(:quit) sets this; Update turns it into tea.Quit
	DynScale     int    // :dynamics view-scale (0=all .. 5=evidence); the resolution/zoom knob
	Width        int    // terminal size (from tea.WindowSizeMsg) — the zones fill this
	Height       int
}

// dynScales maps the seed's view_scale names to their resolution index (1=overview … 5=evidence).
var dynScales = map[string]int{
	"overview": 1, "domain": 2, "artifact": 3, "runtime": 4, "evidence": 5, "all": 0,
}

// scaleIndex resolves a scale name or a bare "1".."5" to a resolution index (0 = all/unknown).
func scaleIndex(s string) int {
	if n, ok := dynScales[s]; ok {
		return n
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 0 && n <= 5 {
		return n
	}
	return 0
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

// FoldDynamics: the pure projection for the :dynamics system-dynamics-map page.
func (m Model) FoldDynamics(g grammar.Graph, dark bool) Model {
	m.Dynamics = g
	m.DynamicsDark = dark
	return m
}

// Exec: the command-as-effect core — a typed command line folds into one pure model transition.
// Today the verbs are local read-effects (page / AIR / quit); write-verbs will later route through
// the unified-API COMMAND surface, but the grammar (every command is ONE pure fold) is fixed here.
func (m Model) Exec(line string) Model {
	m.Input = ""
	m.Mode = ModeNormal
	f := strings.Fields(strings.TrimSpace(line))
	if len(f) == 0 {
		return m
	}
	verb, args := f[0], f[1:]
	switch verb {
	case "events", "e":
		m.Page, m.Status = PageEvents, ":events"
	case "tasks", "t":
		m.Page, m.Status = PageTasks, ":tasks"
	case "dynamics", "d":
		m.Page = PageDynamics
		if s := arg0(args); s != "" { // :dynamics <scale> — overview|domain|artifact|runtime|evidence|1..5|all
			m.DynScale, m.Status = scaleIndex(s), ":dynamics @"+s
		} else {
			m.Status = ":dynamics"
		}
	case "air":
		switch arg0(args) {
		case "on":
			m.AIR = true
		case "off":
			m.AIR = false
		default:
			m.AIR = !m.AIR
		}
		m.Status = fmt.Sprintf("air %v", m.AIR)
	case "help", "h", "?":
		m.Page, m.Status = PageHelp, ":help"
	case "quit", "q":
		m.Quitting, m.Status = true, "bye"
	default:
		m.Status = "unknown command: " + verb
	}
	return m
}

func arg0(a []string) string {
	if len(a) > 0 {
		return a[0]
	}
	return ""
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
type DynamicsMsg struct {
	Graph grammar.Graph
	Dark  bool
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case EventsMsg:
		return m.Fold(v.Events, v.Dark), nil
	case TasksMsg:
		return m.FoldTasks(v.Tasks, v.Dark), nil
	case DynamicsMsg:
		return m.FoldDynamics(v.Graph, v.Dark), nil
	case tea.WindowSizeMsg:
		m.Width, m.Height = v.Width, v.Height // the zones lay out against this
		return m, nil
	case tea.KeyMsg:
		if m.Mode == ModeCommand {
			return m.updateCommand(v)
		}
		switch v.String() {
		case ":": // enter the command line (the command-as-effect surface)
			m.Mode, m.Input, m.Status = ModeCommand, "", ""
			return m, nil
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
		case "3": // :dynamics system-dynamics-map page
			m.Page = PageDynamics
			return m, nil
		case "4": // :help discoverability page
			m.Page = PageHelp
			return m, nil
		}
	}
	return m, nil
}

// updateCommand: key handling while the command line is focused. Enter executes (Exec),
// Esc cancels, Backspace edits, Space/runes append; quit folds straight to tea.Quit.
func (m Model) updateCommand(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch v.Type {
	case tea.KeyEnter:
		m = m.Exec(m.Input)
		if m.Quitting {
			return m, tea.Quit
		}
		return m, nil
	case tea.KeyEsc:
		m.Mode, m.Input = ModeNormal, ""
		return m, nil
	case tea.KeyBackspace:
		if n := len(m.Input); n > 0 {
			m.Input = m.Input[:n-1]
		}
		return m, nil
	case tea.KeySpace:
		m.Input += " "
		return m, nil
	case tea.KeyRunes:
		m.Input += string(v.Runes)
		return m, nil
	}
	return m, nil
}

// View lives in view.go (the four-zone composition).
