package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	evs, dark, err := api.FetchEvents(url)
	msg := model.EventsMsg{Events: evs, Dark: dark}
	if err != nil {
		msg.Error = err.Error()
	}
	return msg
}

// fetchTasksOnce: one registry fetch -> a TasksMsg.
func fetchTasksOnce(url string) tea.Msg {
	ts, dark, err := api.FetchTasks(url)
	msg := model.TasksMsg{Tasks: ts, Dark: dark}
	if err != nil {
		msg.Error = err.Error()
	}
	return msg
}

// fetchDynamicsOnce: one system-dynamics-map fetch -> a DynamicsMsg.
func fetchDynamicsOnce(url string) tea.Msg {
	g, dark, err := api.FetchDynamics(url)
	msg := model.DynamicsMsg{Graph: g, Dark: dark}
	if err != nil {
		msg.Error = err.Error()
	}
	return msg
}

// fetchEpistemicsOnce: one evidence/provenance fetch -> an EpistemicsMsg.
func fetchEpistemicsOnce(url string) tea.Msg {
	ep, dark, err := api.FetchEpistemics(url)
	msg := model.EpistemicsMsg{Epistemics: ep, Dark: dark}
	if err != nil {
		msg.Error = err.Error()
	}
	return msg
}

// fetchSessionsOnce: one live lane roster fetch -> a SessionsMsg.
func fetchSessionsOnce(url string) tea.Msg {
	ss, dark, err := api.FetchSessions(url)
	msg := model.SessionsMsg{Sessions: ss, Dark: dark}
	if err != nil {
		msg.Error = err.Error()
	}
	return msg
}

func fetchSessionDetailOnce(url, role string) tea.Msg {
	d, dark, err := api.FetchSessionDetail(url, role)
	msg := model.SessionDetailMsg{Detail: d, Dark: dark}
	if err != nil {
		msg.Error = err.Error()
	}
	return msg
}

func fetchIntakeOnce(url string) tea.Msg {
	in, dark, err := api.FetchIntake(url)
	msg := model.IntakeMsg{Intake: in, Dark: dark}
	if err != nil {
		msg.Error = err.Error()
	}
	return msg
}

func fetchCapabilitiesOnce(url string) tea.Msg {
	caps, dark, err := api.FetchCapabilities(url)
	msg := model.CapabilitiesMsg{Capabilities: caps, Dark: dark}
	if err != nil {
		msg.Error = err.Error()
	}
	return msg
}

func fetchGatesOnce(url string) tea.Msg {
	gates, dark, err := api.FetchGates(url)
	msg := model.GatesMsg{Gates: gates, Dark: dark}
	if err != nil {
		msg.Error = err.Error()
	}
	return msg
}

func fetchDomainsOnce(url string) tea.Msg {
	domains, dark, err := api.FetchDomains(url)
	msg := model.DomainsMsg{Domains: domains, Dark: dark}
	if err != nil {
		msg.Error = err.Error()
	}
	return msg
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
func epistemicsTick(url string) tea.Cmd {
	return tea.Tick(15*time.Second, func(time.Time) tea.Msg { return fetchEpistemicsOnce(url) })
}
func sessionsTick(url string) tea.Cmd {
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg { return fetchSessionsOnce(url) })
}
func intakeTick(url string) tea.Cmd {
	return tea.Tick(8*time.Second, func(time.Time) tea.Msg { return fetchIntakeOnce(url) })
}
func capabilitiesTick(url string) tea.Cmd {
	return tea.Tick(12*time.Second, func(time.Time) tea.Msg { return fetchCapabilitiesOnce(url) })
}
func gatesTick(url string) tea.Cmd {
	return tea.Tick(10*time.Second, func(time.Time) tea.Msg { return fetchGatesOnce(url) })
}
func domainsTick(url string) tea.Cmd {
	return tea.Tick(20*time.Second, func(time.Time) tea.Msg { return fetchDomainsOnce(url) })
}
func beatTick() tea.Cmd {
	return tea.Tick(650*time.Millisecond, func(time.Time) tea.Msg { return model.BeatMsg{} })
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
		func() tea.Msg { return fetchEpistemicsOnce(r.url) },
		func() tea.Msg { return fetchSessionsOnce(r.url) },
		func() tea.Msg { return fetchIntakeOnce(r.url) },
		func() tea.Msg { return fetchCapabilitiesOnce(r.url) },
		func() tea.Msg { return fetchGatesOnce(r.url) },
		func() tea.Msg { return fetchDomainsOnce(r.url) },
		eventsTick(r.url), tasksTick(r.url), dynamicsTick(r.url), epistemicsTick(r.url), sessionsTick(r.url), intakeTick(r.url), capabilitiesTick(r.url), gatesTick(r.url), domainsTick(r.url), beatTick(),
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
	case model.EpistemicsMsg:
		return r, epistemicsTick(r.url) // re-arm evidence/provenance polling
	case model.SessionsMsg:
		return r, sessionsTick(r.url) // re-arm the live lane roster poll/re-fold loop
	case model.IntakeMsg:
		return r, intakeTick(r.url) // re-arm intake snapshot polling
	case model.CapabilitiesMsg:
		return r, capabilitiesTick(r.url) // re-arm capability-routing polling
	case model.GatesMsg:
		return r, gatesTick(r.url) // re-arm readiness/gate polling
	case model.DomainsMsg:
		return r, domainsTick(r.url) // re-arm domain-pack polling
	case model.BeatMsg:
		return r, beatTick() // visual-only liveness frame; no source/readiness mutation
	case model.SessionDetailRequest:
		return r, func() tea.Msg { return fetchSessionDetailOnce(r.url, msg.(model.SessionDetailRequest).Role) }
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

func parseProbeSize(arg string) (int, int, bool) {
	spec := strings.TrimPrefix(arg, "size:")
	wText, hText, ok := strings.Cut(spec, "x")
	if !ok {
		return 0, 0, false
	}
	w, wErr := strconv.Atoi(wText)
	h, hErr := strconv.Atoi(hText)
	if wErr != nil || hErr != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

func probePageToken(arg string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "events":
		return model.PageEvents, true
	case "tasks":
		return model.PageTasks, true
	case "sessions":
		return model.PageSessions, true
	case "yard":
		return model.PageYard, true
	case "readiness", "ready", "gates", "gate":
		return model.PageReadiness, true
	case "capabilities", "caps", "cap":
		return model.PageCaps, true
	case "intake", "obs", "observations":
		return model.PageIntake, true
	case "dynamics", "dyn":
		return model.PageDynamics, true
	case "epistemics", "epi", "epistemic":
		return model.PageEpistemics, true
	case "help":
		return model.PageHelp, true
	case "commands", "cmds":
		return model.PageCommands, true
	case "windows", "wins":
		return model.PageWindows, true
	case "surfaces", "surf":
		return model.PageSurfaces, true
	case "domains", "domain", "terrain":
		return model.PageDomains, true
	case "lifecycles", "life", "lifecycle", "ndlc", "n-dlc":
		return model.PageLifecycles, true
	case "intent":
		return model.PageIntent, true
	case "legend":
		return model.PageLegend, true
	}
	return 0, false
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func updateProbeModel(m model.Model, msg tea.Msg) model.Model {
	nm, _ := m.Update(msg)
	if next, ok := nm.(model.Model); ok {
		return next
	}
	return m
}

func main() {
	cfg, err := config.Load(configPath())
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(2)
	}
	grammar.SetPalette(cfg.Palette) // color grammar follows the working mode
	// --probe: headless acceptance — fetch read surfaces, fold, print one frame, exit.
	// Args: --probe [page|cmd:<verb>|split|size:WxH|--air]  (page defaults to events)
	if len(os.Args) > 1 && os.Args[1] == "--probe" {
		evs, ed, evErr := api.FetchEvents(cfg.APIURL)
		ts, td, taskErr := api.FetchTasks(cfg.APIURL)
		dg, dd, dynErr := api.FetchDynamics(cfg.APIURL)
		ep, epd, epiErr := api.FetchEpistemics(cfg.APIURL)
		ss, sd, sessErr := api.FetchSessions(cfg.APIURL)
		in, id, intakeErr := api.FetchIntake(cfg.APIURL)
		caps, cd, capsErr := api.FetchCapabilities(cfg.APIURL)
		gates, gd, gatesErr := api.FetchGates(cfg.APIURL)
		doms, domd, domErr := api.FetchDomains(cfg.APIURL)
		m := model.New("REINS")
		m = updateProbeModel(m, model.EventsMsg{Events: evs, Dark: ed, Error: errorText(evErr)})
		m = updateProbeModel(m, model.TasksMsg{Tasks: ts, Dark: td, Error: errorText(taskErr)})
		m = updateProbeModel(m, model.DynamicsMsg{Graph: dg, Dark: dd, Error: errorText(dynErr)})
		m = updateProbeModel(m, model.EpistemicsMsg{Epistemics: ep, Dark: epd, Error: errorText(epiErr)})
		m = updateProbeModel(m, model.SessionsMsg{Sessions: ss, Dark: sd, Error: errorText(sessErr)})
		m = updateProbeModel(m, model.IntakeMsg{Intake: in, Dark: id, Error: errorText(intakeErr)})
		m = updateProbeModel(m, model.CapabilitiesMsg{Capabilities: caps, Dark: cd, Error: errorText(capsErr)})
		m = updateProbeModel(m, model.GatesMsg{Gates: gates, Dark: gd, Error: errorText(gatesErr)})
		m = updateProbeModel(m, model.DomainsMsg{Domains: doms, Dark: domd, Error: errorText(domErr)})
		for _, a := range os.Args[2:] {
			if page, ok := probePageToken(a); ok {
				m.Page = page
				continue
			}
			switch {
			case a == "session-door":
				m.Page, m.SessionDoorOpen = model.PageSessions, true
				if s, ok := m.FocusedSession(); ok {
					d, dark, err := api.FetchSessionDetail(cfg.APIURL, s.Role)
					m = m.FoldSessionDetail(d, dark)
					if err != nil {
						m.SessionDetailError = err.Error()
					}
				}
			case a == "door":
				m.Page, m.DoorOpen = model.PageTasks, true
			case a == "lastlog":
				m.LastlogDoorOpen = true
			case a == "yank":
				m.Page, m.Mode = model.PageTasks, model.ModeYank
			case a == "eyank": // events page in yank-pick mode (field letters on the focused event)
				m.Page, m.Mode = model.PageEvents, model.ModeYank
			case a == "field":
				m.Page, m.Sel.Rank, m.Sel.Field = model.PageTasks, model.RankField, "stage"
			case a == "hint":
				m.Page, m.Mode = model.PageTasks, model.ModeHint
			case strings.HasPrefix(a, "complete:"): // show the fish-style completion floor for <input>
				m.Page, m.Mode, m.Input = model.PageTasks, model.ModeCommand, strings.TrimPrefix(a, "complete:")
			case strings.HasPrefix(a, "filter:"):
				m.Page, m.Mode, m.Filter = model.PageTasks, model.ModeFilter, strings.TrimPrefix(a, "filter:")
			case a == "--air":
				m.AIR = true
			case a == "split":
				m.SplitContext = true
			case strings.HasPrefix(a, "size:"):
				if w, h, ok := parseProbeSize(a); ok {
					m.Width, m.Height = w, h
				}
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
