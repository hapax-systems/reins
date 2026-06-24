package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/api"
	"github.com/hapax-systems/reins/internal/config"
	"github.com/hapax-systems/reins/internal/grammar"
	"github.com/hapax-systems/reins/internal/model"
)

// fetchOnce: one events fetch -> an EventsMsg. Unreachable/dark folds honestly, never panics.
func fetchOnce(url string) tea.Msg {
	evs, dark, _ := api.FetchEvents(url)
	return model.EventsMsg{Events: evs, Dark: dark}
}

// fetchTasksOnce: one registry fetch -> a TasksMsg.
func fetchTasksOnce(url string) tea.Msg {
	ts, dark, _ := api.FetchTasks(url)
	return model.TasksMsg{Tasks: ts, Dark: dark}
}

// fetchDynamicsOnce: one system-dynamics-map fetch -> a DynamicsMsg.
func fetchDynamicsOnce(url string) tea.Msg {
	g, dark, _ := api.FetchDynamics(url)
	return model.DynamicsMsg{Graph: g, Dark: dark}
}

func eventsTick(url string) tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return fetchOnce(url) })
}
func tasksTick(url string) tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return fetchTasksOnce(url) })
}
func dynamicsTick(url string) tea.Cmd { // the map is near-static -> a slow refresh suffices
	return tea.Tick(15*time.Second, func(time.Time) tea.Msg { return fetchDynamicsOnce(url) })
}

type root struct {
	m   model.Model
	url string
}

func (r root) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return fetchOnce(r.url) },
		func() tea.Msg { return fetchTasksOnce(r.url) },
		func() tea.Msg { return fetchDynamicsOnce(r.url) },
		eventsTick(r.url), tasksTick(r.url), dynamicsTick(r.url),
	)
}

func (r root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := r.m.Update(msg)
	r.m = nm.(model.Model)
	switch msg.(type) {
	case model.EventsMsg:
		return r, eventsTick(r.url) // re-arm the events poll/re-fold loop
	case model.TasksMsg:
		return r, tasksTick(r.url) // re-arm the registry poll/re-fold loop
	case model.DynamicsMsg:
		return r, dynamicsTick(r.url) // re-arm the map poll/re-fold loop
	}
	return r, cmd // propagate the model's cmd (e.g. tea.Quit on [q])
}

func (r root) View() string { return r.m.View() }

func configPath() string {
	if p := os.Getenv("REINS_CONFIG"); p != "" {
		return p
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "reins", "config.toml")
}

func main() {
	cfg, err := config.Load(configPath())
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(2)
	}
	grammar.SetPalette(cfg.Palette) // color grammar follows the working mode
	// --probe: headless acceptance — fetch both feeds, fold, print one frame, exit.
	// Args: --probe [tasks] [--air]  (page defaults to events; --air = PII-safe lens)
	if len(os.Args) > 1 && os.Args[1] == "--probe" {
		evs, ed, _ := api.FetchEvents(cfg.APIURL)
		ts, td, _ := api.FetchTasks(cfg.APIURL)
		dg, dd, _ := api.FetchDynamics(cfg.APIURL)
		m := model.New("REINS").Fold(evs, ed).FoldTasks(ts, td).FoldDynamics(dg, dd)
		for _, a := range os.Args[2:] {
			switch {
			case a == "tasks":
				m.Page = model.PageTasks
			case a == "dynamics":
				m.Page = model.PageDynamics
			case a == "help":
				m.Page = model.PageHelp
			case a == "legend":
				m.Page = model.PageLegend
			case a == "door":
				m.Page, m.DoorOpen = model.PageTasks, true
			case a == "yank":
				m.Page, m.Mode = model.PageTasks, model.ModeYank
			case a == "field":
				m.Page, m.Sel.Rank, m.Sel.Field = model.PageTasks, model.RankField, "stage"
			case a == "hint":
				m.Page, m.Mode = model.PageTasks, model.ModeHint
			case a == "--air":
				m.AIR = true
			case strings.HasPrefix(a, "cmd:"): // exercise the command-as-effect path headless
				m = m.Exec(strings.TrimPrefix(a, "cmd:"))
			}
		}
		fmt.Println(m.View())
		return
	}
	r := root{m: model.New("REINS"), url: cfg.APIURL}
	if _, err := tea.NewProgram(r, tea.WithAltScreen()).Run(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
