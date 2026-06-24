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
	ModeHint    = 3 // hint-teleport: labels on visible rows, type one to jump (navigate by looking)
	ModeFilter  = 4 // the filter input is focused — narrows the selectable rows by id substring
)

// hintAlphabet: home-row-first labels for hint teleport (one key per visible row; choose by sight).
const hintAlphabet = "asdfghjklqwertyuiopzxcvbnm"

// the selection lattice rank (coarse→fine). The cursor lives at one rank; [↵]/[⌫] descend/ascend.
const (
	RankRow   = 2 // L2 — a registry row (the default; m.Focus is the row index)
	RankField = 3 // L3 — a cell within the focused row
)

// Selection is the cursor-of-attention: ONE selection, many verbs act on it. The row index stays in
// m.Focus (the established L2 index); Selection adds the rank + which field (at L3) + the type.
type Selection struct {
	Rank    int    // RankRow | RankField
	Field   string // when Rank==RankField: the cell (task_id/stage/owner/prior_stage/predicted_stage/criticality/authority_case)
	Type    string // "task" (cross-cutting types — counts/nodes/… — arrive in S9)
	Members []int  // granularity g2: a CLASS of sibling rows (indices into visibleTasks)
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
	Filter       string // active :tasks filter (id substring); narrows the selectable set
	CritFilter   string // active criticality-class filter (ok|warn|major|crit) — a selected count
	CompIdx      int    // fish-style completion: the highlighted candidate in the navigable list
}

// critFromHint: the count labels in hint mode (cross-cutting selectables) → the criticality class.
var critFromHint = map[rune]string{'O': "ok", 'W': "warn", 'M': "major", 'C': "crit"}

// blockedIndices: indices into m.Tasks of the blocked items (predicted hold OR major/crit) — the
// Act strip's contents, also a cross-cutting selectable (jump to a blocker from the exception line).
func (m Model) blockedIndices() []int {
	var out []int
	for i, t := range m.Tasks {
		if t.PredictedStage == "hold" || t.Criticality == "crit" || t.Criticality == "major" {
			out = append(out, i)
		}
	}
	return out
}

// completions: the navigable candidate list for the command line. Dynamic on the active SELECTION —
// when a field/row is selected, a `paste <value>` candidate is offered first so the operator can
// inject the selection (the seed of the {{sel}} template language; see the handoff forward-look).
// completions returns the highlighted-level candidate LABELS — the stable string seam the older
// tests pin. The real engine (sub-menus, Detail column, dynamic-on-selection) lives in complete.go;
// this is its flat projection.
func (m Model) completions() []string {
	cs := m.completionTree()
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Label
	}
	return out
}

// visibleTasks: the selectable row set — m.Tasks narrowed by the active Filter (id substring) AND
// CritFilter (a selected criticality count). The cursor, rail, door, and yank all operate on THIS.
func (m Model) visibleTasks() []grammar.Task {
	q := strings.ToLower(strings.TrimSpace(m.Filter))
	if q == "" && m.CritFilter == "" {
		return m.Tasks
	}
	out := make([]grammar.Task, 0, len(m.Tasks))
	for _, t := range m.Tasks {
		c := t.Criticality
		if c == "" {
			c = "ok"
		}
		if m.CritFilter != "" && c != m.CritFilter {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(t.TaskID), q) {
			continue
		}
		out = append(out, t)
	}
	return out
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

// focusMax is the highest valid registry focus index (0 when empty) — over the VISIBLE (filtered) set.
func (m Model) focusMax() int {
	if n := len(m.visibleTasks()); n > 0 {
		return n - 1
	}
	return 0
}

// FocusedTask returns the task under the registry cursor (within the visible/filtered set).
func (m Model) FocusedTask() (grammar.Task, bool) {
	vt := m.visibleTasks()
	if m.Focus < 0 || m.Focus >= len(vt) {
		return grammar.Task{}, false
	}
	return vt[m.Focus], true
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
type verbDef struct {
	name, gloss string
	args        []Candidate // sub-menu: this verb's argument candidates (nil = a leaf verb, no args)
}

var verbs = []verbDef{
	{"events", "live coord event stream", nil},
	{"tasks", "the task registry", nil},
	{"dynamics", "the system-dynamics map", []Candidate{
		{Label: "overview", Detail: "the whole map at a glance"},
		{Label: "domain", Detail: "the domain layer"},
		{Label: "artifact", Detail: "the artifact layer"},
		{Label: "runtime", Detail: "the runtime layer"},
		{Label: "evidence", Detail: "the evidence layer"},
		{Label: "all", Detail: "every layer, unscaled"},
	}},
	{"legend", "decode the grammar — every glyph/color/cell", nil},
	{"help", "the help page", nil},
	{"air", "the on-air PII lens", []Candidate{
		{Label: "on", Detail: "redact non-allowlisted cells (broadcast-safe)"},
		{Label: "off", Detail: "show everything (LOCAL only)"},
	}},
	{"quit", "leave", nil},
}

// lookupVerb finds a verb by exact name.
func lookupVerb(name string) (verbDef, bool) {
	for _, v := range verbs {
		if v.name == name {
			return v, true
		}
	}
	return verbDef{}, false
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
		if m.Mode == ModeHint {
			return m.updateHint(v)
		}
		if m.Mode == ModeFilter {
			return m.updateFilter(v)
		}
		if m.Sel.Rank == RankField { // a field within the row is selected — h/l steer, [y] yanks it
			return m.updateField(v)
		}
		switch v.String() {
		case "/": // filter — narrow the selectable rows by id substring (incremental)
			if m.Page == PageTasks {
				m.Mode, m.Focus = ModeFilter, 0
			}
			return m, nil
		case "f": // hint teleport — labels bloom on visible rows; type one to jump (by looking)
			if m.Page == PageTasks && len(m.Tasks) > 0 {
				m.Mode = ModeHint
			}
			return m, nil
		case "tab": // descend into the row's fields (navigate by looking; [Tab] again / Esc ascends)
			if _, ok := m.FocusedTask(); ok {
				m.Sel.Rank, m.Sel.Field = RankField, selFields[0]
			}
			return m, nil
		case "V": // class-select — every visible row sharing the focused task's criticality (granularity g2)
			if t, ok := m.FocusedTask(); ok {
				vt := m.visibleTasks()
				var mem []int
				for i, x := range vt {
					if x.Criticality == t.Criticality {
						mem = append(mem, i)
					}
				}
				m.Sel.Members = mem
				m.Status = fmt.Sprintf("selected %d '%s' tasks · [y] yank all · [Esc] clear", len(mem), t.Criticality)
			}
			return m, nil
		case "esc": // collapse the selection to the bare row cursor + clear class/count filters
			m.Sel.Members, m.CritFilter = nil, ""
			return m, nil
		case "y": // yank — class-yank if a class is selected, else grab one field
			if len(m.Sel.Members) > 0 {
				vt := m.visibleTasks()
				vals := make([]string, 0, len(m.Sel.Members))
				for _, idx := range m.Sel.Members {
					if idx >= 0 && idx < len(vt) {
						vals = append(vals, vt[idx].TaskID)
					}
				}
				m.Ring = pushRing(m.Ring, RingEntry{Value: strings.Join(vals, "\n"), Field: "class", Page: "tasks"})
				m.Sel.Members = nil
				m.Status = fmt.Sprintf("yanked %d task ids → kill-ring", len(vals))
				return m, nil
			}
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
			m.Mode, m.Input, m.Status, m.CompIdx = ModeCommand, "", "", 0
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

// selFields: the L3 field order the field cursor walks (left→right, the lifecycle sentence).
var selFields = []string{"task_id", "stage", "owner", "prior_stage", "predicted_stage", "criticality", "authority_case"}

func (m Model) fieldIdx() int {
	for i, f := range selFields {
		if f == m.Sel.Field {
			return i
		}
	}
	return 0
}

func fieldValue(t grammar.Task, field string) string {
	switch field {
	case "task_id":
		return t.TaskID
	case "stage":
		return t.Stage
	case "owner":
		return t.Owner
	case "prior_stage":
		return t.PriorStage
	case "predicted_stage":
		return t.PredictedStage
	case "criticality":
		return t.Criticality
	case "authority_case":
		return t.AuthorityCase
	}
	return ""
}

// taskWindow: the visible row window (offset, count) — the SAME math taskBody renders with, so a
// hint label maps to the right absolute row index.
func (m Model) taskWindow() (off, visible int) {
	h := m.Height
	if h < 12 {
		h = 40 // matches View's default frame
	}
	visible = h - 9 // midH(h-7) - context - header
	if visible < 1 {
		visible = 1
	}
	return m.scrollOffset(visible), visible
}

// updateFilter: incremental id-substring filter on :tasks. Enter keeps the filter active (input
// closes), Esc clears it. The cursor re-homes to 0 on each change (the visible set shifts).
func (m Model) updateFilter(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch v.Type {
	case tea.KeyEnter:
		m.Mode = ModeNormal // filter stays active; input closes
	case tea.KeyEsc:
		m.Mode, m.Filter, m.Focus = ModeNormal, "", 0 // clear the filter
	case tea.KeyBackspace:
		if n := len(m.Filter); n > 0 {
			m.Filter = m.Filter[:n-1]
		}
		m.Focus = 0
	case tea.KeySpace:
		m.Filter += " "
	case tea.KeyRunes:
		m.Filter += string(v.Runes)
		m.Focus = 0
	}
	return m, nil
}

// updateHint: type a row's label to teleport the cursor there; Esc cancels. No other state change.
func (m Model) updateHint(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	if v.String() == "esc" {
		m.Mode = ModeNormal
		return m, nil
	}
	if v.Type == tea.KeyRunes && len(v.Runes) == 1 {
		r := v.Runes[0]
		if cf, ok := critFromHint[r]; ok { // a COUNT label → filter the list to that criticality class
			m.CritFilter, m.Focus, m.Mode = cf, 0, ModeNormal
			m.Status = "filtered to '" + cf + "' tasks · [Esc] clear"
			return m, nil
		}
		if r >= '1' && r <= '9' { // an ACT-strip item → jump the cursor to that blocker
			bi := m.blockedIndices()
			if d := int(r - '1'); d < len(bi) {
				id := m.Tasks[bi[d]].TaskID
				m.Filter, m.CritFilter = "", "" // clear filters so the blocker is reachable
				for j, t := range m.visibleTasks() {
					if t.TaskID == id {
						m.Focus = j
						break
					}
				}
				m.Mode = ModeNormal
				m.Status = "jumped to blocker " + id
				return m, nil
			}
		}
		off, visible := m.taskWindow()
		if i := strings.IndexRune(hintAlphabet, r); i >= 0 && i < visible && off+i < len(m.visibleTasks()) {
			m.Focus = off + i
		}
		m.Mode = ModeNormal
	}
	return m, nil
}

// updateField: keys at L3 (a field selected within the row). h/l steer across fields, j/k still move
// rows (staying at field rank), [y] yanks the selected field, [Tab]/[Esc] ascend to the row.
func (m Model) updateField(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch v.String() {
	case "esc", "tab":
		m.Sel.Rank, m.Sel.Field = RankRow, ""
	case "l", "right":
		m.Sel.Field = selFields[clamp(m.fieldIdx()+1, 0, len(selFields)-1)]
	case "h", "left":
		m.Sel.Field = selFields[clamp(m.fieldIdx()-1, 0, len(selFields)-1)]
	case "j", "down":
		m.Focus = clamp(m.Focus+1, 0, m.focusMax())
	case "k", "up":
		m.Focus = clamp(m.Focus-1, 0, m.focusMax())
	case "y": // verb on the current selection — yank THE selected field, no extra pick (S7 preview)
		t, ok := m.FocusedTask()
		if !ok {
			return m, nil
		}
		f := m.Sel.Field
		if m.AIR && t.AIR[f] != "ok" {
			m.Status = "yank: " + f + " is redacted on-air — un-yankable"
			return m, nil
		}
		m.Ring = pushRing(m.Ring, RingEntry{Value: fieldValue(t, f), Field: f, Page: "tasks"})
		m.Input, m.Mode, m.Sel.Rank = fieldValue(t, f), ModeCommand, RankRow
		m.Status = fmt.Sprintf("yanked %s → command line  (ring %d)", f, len(m.Ring))
	case "q", "ctrl+c":
		return m, tea.Quit
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
	case tea.KeyEnter: // run the highlighted candidate (fish-style: Tab navigates, Enter accepts)
		m = m.acceptCompletion()
		if m.Quitting {
			return m, tea.Quit
		}
		return m, nil
	case tea.KeyEsc:
		m.Mode, m.Input, m.CompIdx = ModeNormal, "", 0
		return m, nil
	case tea.KeyBackspace:
		if n := len(m.Input); n > 0 {
			m.Input = m.Input[:n-1]
		}
		m.CompIdx = 0 // input changed → re-rank candidates from the top
		return m, nil
	case tea.KeyTab, tea.KeyDown: // NAVIGATE the completion list (revealed explicitly below the prompt)
		if c := m.completions(); len(c) > 0 {
			m.CompIdx = (m.CompIdx + 1) % len(c)
		}
		return m, nil
	case tea.KeyShiftTab, tea.KeyUp:
		if c := m.completions(); len(c) > 0 {
			m.CompIdx = (m.CompIdx - 1 + len(c)) % len(c)
		}
		return m, nil
	case tea.KeyRight: // fish-style accept INTO the line (descend a sub-menu OR fill), never run
		return m.fillCompletion(), nil
	case tea.KeySpace:
		m.Input, m.CompIdx = m.Input+" ", 0
		return m, nil
	case tea.KeyRunes:
		m.Input, m.CompIdx = m.Input+string(v.Runes), 0
		return m, nil
	}
	return m, nil
}

// View lives in view.go (the four-zone composition).
