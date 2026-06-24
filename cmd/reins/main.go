package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/api"
	"github.com/hapax-systems/reins/internal/config"
	"github.com/hapax-systems/reins/internal/model"
)

// fetchOnce: one fetch -> an EventsMsg. Unreachable/dark folds honestly (dark=true), never panics.
func fetchOnce(url string) tea.Msg {
	evs, dark, _ := api.FetchEvents(url)
	return model.EventsMsg{Events: evs, Dark: dark}
}

func tickCmd(url string) tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return fetchOnce(url) })
}

type root struct {
	m   model.Model
	url string
}

func (r root) Init() tea.Cmd {
	return tea.Batch(func() tea.Msg { return fetchOnce(r.url) }, tickCmd(r.url))
}

func (r root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := r.m.Update(msg)
	r.m = nm.(model.Model)
	if _, ok := msg.(model.EventsMsg); ok {
		return r, tickCmd(r.url) // re-arm the poll/re-fold loop (the hot-reload kernel)
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
	// --probe: headless acceptance — fetch -> fold -> print one rendered frame -> exit.
	// Optional second arg --air renders through the AIR (PII-safe) lens.
	if len(os.Args) > 1 && os.Args[1] == "--probe" {
		evs, dark, _ := api.FetchEvents(cfg.APIURL)
		m := model.New("REINS").Fold(evs, dark)
		if len(os.Args) > 2 && os.Args[2] == "--air" {
			m.AIR = true
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
