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
	PageLegend   = 4
)

const (
	ModeNormal  = 0 // hotkeys + page navigation
	ModeCommand = 1 // the command line is focused (typing a verb)
	ModeYank    = 2 // a field-pick sub-state on the focused row (the copy-paste killer)
)

// the selection lattice rank (coarse→fine). The cursor lives at one rank; [↵]/[⌫] descend/ascend.
const (
	RankRow   = 2 // L2 — a registry row (the default; m.Focus is the row index)
	RankField = 3 // L3 — a cell within the focused row
)

// Selection is the cursor-of-attention: ONE selection, many verbs act on it. The row index stays in
// m.Focus (the established L2 index); Selection adds the rank + which field (at L3) + the type.
type Selection struct {
	Rank  int    // RankRow | RankField
	Field string // when Rank==RankField: the cell (task_id/stage/owner/prior_stage/predicted_stage/criticality/authority_case)
	Type  string // "task" (cross-cutting types — counts/nodes/… — arrive in S9)
}

// RingEntry: one grabbed object, provenance-tagged (the emacs-style kill-ring, in memory).
type RingEntry struct {
	Value, Field, Page string
}

// pushRing prepends e, de-duping to front, bounded at 16 (MRU).
func pushRing(r []RingEntry, e RingEntry) []RingEntry {
	out := []RingEntry{e}
	for _, x := range r {
		if x.Value != e.Value {
			out = append(out, x)
		}
	}
	if len(out) > 16 {
		out = out[:16]
	}
	return out
}

// yankField: key -> (field-name, value) for the FOCUSED task. Reads the SOURCE STRUCT, never the
// screen — this is what makes Reins's yank categorically better than a screen-scraper.
func (m Model) yankField(key string) (field, val string, ok bool) {
	t, has := m.FocusedTask()
	if !has {
		return "", "", false
	}
	switch key {
	case "i":
		return "task_id", t.TaskID, true
	case "s":
		return "stage", t.Stage, true
	case "o":
		return "owner", t.Owner, true
	case "w":
		return "prior_stage", t.PriorStage, true
	case "n":
		return "predicted_stage", t.PredictedStage, true
	case "c":
		return "criticality", t.Criticality, true
	case "a":
		return "authority_case", t.AuthorityCase, true
	}
	return "", "", false
}

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
	Focus        int    // selected row index into m.Tasks (the registry cursor; the rail tracks it)
	Ring         []RingEntry // the yank kill-ring (most-recent first)
	DoorOpen     bool   // the /whois full-screen drill-in is open for the focused task
	Sel          Selection // the cursor-of-attention's rank/field/type (row index stays in Focus)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// focusMax is the highest valid registry focus index (0 when empty).
func (m Model) focusMax() int {
	if len(m.Tasks) == 0 {
		return 0
	}
	return len(m.Tasks) - 1
}

// FocusedTask returns the task under the registry cursor, ok=false if none.
func (m Model) FocusedTask() (grammar.Task, bool) {
	if m.Focus < 0 || m.Focus >= len(m.Tasks) {
		return grammar.Task{}, false
	}
	return m.Tasks[m.Focus], true
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

func New(title string) Model {
	return Model{Title: title, Sel: Selection{Rank: RankRow, Type: "task"}}
}

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

// verbDef + verbs: the command vocabulary, surfaced as a which-key menu at the point of action
// (recognition over recall — never make the operator memorize the verbs).
type verbDef struct{ name, gloss string }

var verbs = []verbDef{
	{"events", "live coord event stream"},
	{"tasks", "the task registry"},
	{"dynamics", "the system-dynamics map  (+ overview|domain|…|evidence)"},
	{"legend", "decode the grammar — every glyph/color/cell"},
	{"help", "the help page"},
	{"air", "the on-air PII lens  (on|off)"},
	{"quit", "leave"},
}

// matchVerbs returns the verbs whose name starts with the first token of the input.
func matchVerbs(input string) []verbDef {
	prefix := ""
	if f := strings.Fields(input); len(f) > 0 {
		prefix = f[0]
	}
	var out []verbDef
	for _, v := range verbs {
		if strings.HasPrefix(v.name, prefix) {
			out = append(out, v)
		}
	}
	return out
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
	case "help", "h":
		m.Page, m.Status = PageHelp, ":help"
	case "legend", "?":
		m.Page, m.Status = PageLegend, ":legend"
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
		if m.Mode == ModeYank {
			return m.updateYank(v)
		}
		if m.DoorOpen {
			return m.updateDoor(v)
		}
		switch v.String() {
		case "y": // yank — grab a field off the focused row (the copy-paste killer)
			if _, ok := m.FocusedTask(); ok {
				m.Mode, m.Status = ModeYank, ""
			}
			return m, nil
		case "enter": // /whois — drill into the focused task (full-screen door)
			if _, ok := m.FocusedTask(); ok {
				m.DoorOpen = true
			}
			return m, nil
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
		case "?": // :legend — decode the grammar (always situate)
			m.Page = PageLegend
			return m, nil
		case "j", "down": // move the registry focus cursor (the rail tracks it)
			m.Focus = clamp(m.Focus+1, 0, m.focusMax())
			return m, nil
		case "k", "up":
			m.Focus = clamp(m.Focus-1, 0, m.focusMax())
			return m, nil
		case "g": // top
			m.Focus = 0
			return m, nil
		case "G": // bottom
			m.Focus = m.focusMax()
			return m, nil
		}
	}
	return m, nil
}

// updateDoor: keys while the /whois door is open. [Esc]/[Enter] close (clean return). The verb-dock
// keys are GOVERNED STUBS — they report what they WOULD emit through the governed COMMAND surface but
// never mutate the live system (the cockpit never mints authority; real routing is a follow-up).
func (m Model) updateDoor(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.DoorOpen = false
	switch v.String() {
	case "esc", "enter", "q":
		// just close
	case "a":
		m.Status = "arm: would emit sdlc.authorization_flip(release_authorized=true) via the governed COMMAND surface — NOT wired (cockpit never mints authority)"
	case "r":
		m.Status = "rework: would emit sdlc.stage_transition(→rework) via the governed COMMAND surface — NOT wired"
	case "f":
		m.Status = "refute: would record review.fail via the governed COMMAND surface — NOT wired"
	case "c":
		m.Status = "close: would emit task.closed via the governed COMMAND surface — NOT wired"
	default:
		m.DoorOpen = true // unknown key — stay in the door
	}
	return m, nil
}

// updateYank: a field key grabs that field off the focused row INTO the ring and pre-seeds the
// command line (the cheapest highest-value paste target — Reins IS the operator's control plane).
// AIR-gated: a field redacted on-air is un-yankable (yields the redaction token, never cleartext).
func (m Model) updateYank(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	if v.Type == tea.KeyEsc {
		m.Mode = ModeNormal
		return m, nil
	}
	field, val, ok := m.yankField(v.String())
	if !ok {
		return m, nil // unknown pick key — stay in the menu
	}
	t, _ := m.FocusedTask()
	if m.AIR && t.AIR[field] != "ok" {
		m.Mode, m.Status = ModeNormal, "yank: "+field+" is redacted on-air — un-yankable"
		return m, nil
	}
	m.Ring = pushRing(m.Ring, RingEntry{Value: val, Field: field, Page: "tasks"})
	m.Input, m.Mode = val, ModeCommand // grabbed → straight into the command line
	m.Status = fmt.Sprintf("yanked %s → command line  (ring %d)", field, len(m.Ring))
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
	case tea.KeyTab: // complete to the single matching verb (recognition over recall)
		if mv := matchVerbs(m.Input); len(mv) == 1 {
			m.Input = mv[0].name + " "
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
