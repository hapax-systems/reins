package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/grammar"
)

const (
	PageEvents     = 0
	PageTasks      = 1
	PageSessions   = 2
	PageDynamics   = 3
	PageHelp       = 4
	PageLegend     = 5
	PageCommands   = 6
	PageWindows    = 7
	PageIntent     = 8
	PageSurfaces   = 9
	PageDomains    = 10
	PageYard       = 11
	PageCaps       = 12
	PageReadiness  = 13
	PageIntake     = 14
	PageEpistemics = 15
	PageLifecycles = 16
)

const (
	ModeNormal  = 0 // hotkeys + page navigation
	ModeCommand = 1 // the command line is focused (typing a verb)
	ModeYank    = 2 // a field-pick sub-state on the focused row (the copy-paste killer)
	ModeHint    = 3 // hint-teleport: labels on visible rows, type one to jump (navigate by looking)
	ModeFilter  = 4 // the filter input is focused — narrows the selectable rows by id substring
)

const splitContextMinWidth = 160

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
// AIR records the source field's current AIR decision ("ok" or "deny"). Missing means deny so
// older/unclassified ring entries cannot replay cleartext after the AIR lens turns on.
type RingEntry struct {
	Value, Field, Page, AIR string
}

// pushRing prepends e, de-duping to front, bounded at 16 (MRU).
func pushRing(r []RingEntry, e RingEntry) []RingEntry {
	out := []RingEntry{e}
	for _, x := range r {
		if x.Value != e.Value || x.Field != e.Field || x.Page != e.Page || x.AIR != e.AIR {
			out = append(out, x)
		}
	}
	if len(out) > 16 {
		out = out[:16]
	}
	return out
}

func airDecision(air map[string]string, field string) string {
	if air != nil && air[field] == "ok" {
		return "ok"
	}
	return "deny"
}

func taskFieldValueForAir(t grammar.Task, field string, airOn bool) string {
	return grammar.Redact(t.AIR, field, fieldValue(t, field), airOn)
}

func taskRingEntry(t grammar.Task, field, page string) RingEntry {
	return RingEntry{Value: fieldValue(t, field), Field: field, Page: page, AIR: airDecision(t.AIR, field)}
}

func eventRingEntry(ev grammar.Event, field, val string) RingEntry {
	return RingEntry{Value: val, Field: field, Page: "events", AIR: airDecision(ev.AIR, field)}
}

func sessionRingEntry(s grammar.Session, field, val string) RingEntry {
	return RingEntry{Value: val, Field: field, Page: "sessions", AIR: airDecision(s.AIR, field)}
}

func ringValue(e RingEntry, airOn bool) string {
	if airOn && e.AIR != "ok" {
		return "▒▒▒"
	}
	return e.Value
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

func eventFieldValueForAir(ev grammar.Event, field, val string, airOn bool) string {
	return grammar.Redact(ev.AIR, field, val, airOn)
}

// yankEventField: the :events analogue of yankField — pick-keys for the focused event's fields.
func (m Model) yankEventField(key string) (field, val string, ok bool) {
	ev, has := m.FocusedEvent()
	if !has {
		return "", "", false
	}
	switch key {
	case "t":
		return "ts", ev.TS, true
	case "k", "K":
		return "kind", ev.Kind, true
	case "s":
		return "subject", ev.Subject, true
	case "a":
		return "actor", ev.Actor, true
	case "m":
		return "summary", ev.Summary, true
	}
	return "", "", false
}

type yankFieldDef struct{ key, field string }

func yankFieldsForSelectionPage(page int) []yankFieldDef {
	switch page {
	case PageEvents:
		return []yankFieldDef{{"t", "ts"}, {"K", "kind"}, {"s", "subject"}, {"a", "actor"}, {"m", "summary"}}
	case PageSessions:
		return []yankFieldDef{
			{"r", "role"}, {"p", "platform"}, {"d", "readiness"}, {"t", "state"}, {"b", "blocker"},
			{"a", "attention"}, {"s", "session"}, {"c", "claimed_task"}, {"u", "route_id"}, {"m", "mode"},
			{"f", "profile"}, {"g", "route_binding_state"}, {"e", "route_evidence_ref"}, {"o", "output_age_s"}, {"l", "relay_age_s"},
		}
	case PageTasks:
		return []yankFieldDef{
			{"i", "task_id"}, {"s", "stage"}, {"o", "owner"}, {"w", "prior_stage"},
			{"n", "predicted_stage"}, {"c", "criticality"}, {"a", "authority_case"},
		}
	}
	return nil
}

func (m Model) commandSelectionPage() int {
	if m.splitContextActive() {
		return PageSessions
	}
	return m.Page
}

func (m Model) yankFieldsForPage() []yankFieldDef {
	return yankFieldsForSelectionPage(m.commandSelectionPage())
}

func (m Model) selectedFieldForPage(page int, fallback string) string {
	if m.Sel.Rank != RankField || m.Sel.Field == "" {
		return fallback
	}
	for _, f := range yankFieldsForSelectionPage(page) {
		if f.field == m.Sel.Field {
			return m.Sel.Field
		}
	}
	return fallback
}

func (m Model) withYankMode() Model {
	m.Mode, m.Status = ModeYank, ""
	fields := m.yankFieldsForPage()
	if len(fields) > 0 {
		keep := false
		for _, f := range fields {
			if f.field == m.Sel.Field {
				keep = true
				break
			}
		}
		m.Sel.Rank = RankField
		if !keep {
			m.Sel.Field = fields[0].field
		}
	}
	return m
}

func (m Model) yankFieldIndex() int {
	fields := m.yankFieldsForPage()
	for i, f := range fields {
		if f.field == m.Sel.Field {
			return i
		}
	}
	return 0
}

func (m Model) moveYankField(delta int) Model {
	fields := m.yankFieldsForPage()
	if len(fields) == 0 {
		return m
	}
	i := clamp(m.yankFieldIndex()+delta, 0, len(fields)-1)
	m.Sel.Rank, m.Sel.Field = RankField, fields[i].field
	return m
}

func (m Model) yankCurrentField() (Model, tea.Cmd) {
	fields := m.yankFieldsForPage()
	if len(fields) == 0 {
		return m, nil
	}
	idx := m.yankFieldIndex()
	if idx < 0 || idx >= len(fields) {
		idx = 0
	}
	return m.yankFieldByKey(fields[idx].key)
}

func sessionFieldValueForAir(s grammar.Session, field string, airOn bool) string {
	return grammar.Redact(s.AIR, field, sessionFieldValue(s, field), airOn)
}

func (m Model) yankSessionField(key string) (field, val string, ok bool) {
	s, has := m.FocusedSession()
	if !has {
		return "", "", false
	}
	switch key {
	case "r":
		return "role", s.Role, true
	case "p":
		return "platform", s.Platform, true
	case "s":
		return "session", s.Session, true
	case "t":
		return "state", s.State, true
	case "d":
		return "readiness", s.Readiness, true
	case "b":
		return "blocker", s.Blocker, true
	case "a":
		return "attention", fmt.Sprintf("%.2f", s.Attention), true
	case "c":
		return "claimed_task", s.ClaimedTask, true
	case "u":
		return "route_id", s.RouteID, true
	case "m":
		return "mode", s.RouteMode, true
	case "f":
		return "profile", s.RouteProfile, true
	case "g":
		return "route_binding_state", s.RouteBindingState, true
	case "e":
		return "route_evidence_ref", s.RouteEvidenceRef, true
	case "o":
		return "output_age_s", fmt.Sprintf("%.1f", s.OutputAgeS), true
	case "l":
		return "relay_age_s", fmt.Sprintf("%.1f", s.RelayAgeS), true
	}
	return "", "", false
}

type Model struct {
	Title               string
	Page                int
	Events              []grammar.Event
	Tasks               []grammar.Task
	Sessions            []grammar.Session
	SessionDetail       grammar.SessionDetail
	Intake              grammar.IntakeSummary
	Capabilities        grammar.CapabilitySummary
	Gates               grammar.GateSummary
	Domains             grammar.DomainSummary
	Dynamics            grammar.Graph
	Epistemics          grammar.EpistemicsSummary
	EventsDark          bool
	TasksDark           bool
	SessionsDark        bool
	SessionDetailDark   bool
	IntakeDark          bool
	CapabilitiesDark    bool
	GatesDark           bool
	DomainsDark         bool
	DynamicsDark        bool
	EpistemicsDark      bool
	EventsError         string
	TasksError          string
	SessionsError       string
	SessionDetailError  string
	IntakeError         string
	CapabilitiesError   string
	GatesError          string
	DomainsError        string
	DynamicsError       string
	EpistemicsError     string
	EventsSeq           int
	TasksSeq            int
	SessionsSeq         int
	IntakeSeq           int
	CapabilitiesSeq     int
	GatesSeq            int
	DomainsSeq          int
	DynamicsSeq         int
	EpistemicsSeq       int
	LastFold            string
	AIR                 bool // the AIR lens
	Mode                int  // ModeNormal | ModeCommand
	Input               string
	Status              string // last command result / error (one line, above the hint)
	Quitting            bool   // Exec(:quit) sets this; Update turns it into tea.Quit
	DynScale            int    // :dynamics view-scale (0=all .. 5=evidence); the resolution/zoom knob
	DynFocus            int    // selected dynamics element (node/edge/source) for epistemic inspection
	Width               int    // terminal size (from tea.WindowSizeMsg) — the zones fill this
	Height              int
	Beat                int         // low-rate liveness frame; visual only, never authority/readiness
	Focus               int         // selected row index into visibleTasks (the :tasks cursor; the rail tracks it)
	EFocus              int         // selected row index into m.Events (the :events cursor) — selection is page-aware
	SFocus              int         // selected row index into m.Sessions (the :sessions cursor)
	IFocus              int         // selected row index into visibleIntakeRows (the :intake bucket cursor)
	CFocus              int         // selected capability/status row index into the grouped :capabilities projection
	CommandFocus        int         // selected command registry row
	WindowFocus         int         // selected window registry row
	SurfaceFocus        int         // selected surface registry row
	DomainFocus         int         // selected domain registry row
	LifecycleFocus      int         // selected lifecycle contract row
	EpiFocus            int         // selected epistemic posture row
	IntentFocus         int         // selected governed target row on :intent
	RefScroll           int         // scroll offset for full-width reference pages (:dynamics/:help/:legend/:commands/:windows)
	Ring                []RingEntry // the yank kill-ring (most-recent first)
	DoorOpen            bool        // the /whois full-screen drill-in is open for the focused task
	SessionDoorOpen     bool        // the /session full-screen lane card is open for the focused session
	IntakeDoorOpen      bool        // the /intake full-screen aggregate provenance door is open
	LastlogDoorOpen     bool        // the /lastlog scrollback door is open (retained event history)
	EventScrollback     Scrollback  // per-window event-history ring (the /lastlog affordance), fed on poll
	LastlogOlder        []grammar.Event // transient backward-paged events (PgUp); cleared on close
	LastlogPaging       bool        // a /lastlog backward-page fetch is in flight
	Sel                 Selection   // the cursor-of-attention's rank/field/type (row index stays in Focus)
	Filter              string      // active :tasks filter (id substring); narrows the selectable set
	CritFilter          string      // active criticality-class filter (ok|warn|major|crit) — a selected count
	IntakeSourceFilter  string      // active :intake source filter; empty means all sources
	CompIdx             int         // fish-style completion: the highlighted candidate in the navigable list
	Flash               string      // transient effect-confirmation (Norman feedback); auto-clears via FlashClearMsg
	FlashSeq            int         // monotonic flash id — a stale tick only clears the flash it was armed for
	IntentTarget        string      // currently reviewed governed intent target, e.g. resume/dispatch/show-route
	IntentSubject       string      // AIR-safe subject captured before switching to the intent review page
	SplitContext        bool        // visible session source + declared relation/context projection
	SuppressSplitPinned bool        // render-only: split body clone omits the pinned selected-source block
}

// FlashClearMsg clears a flash after its lifetime, but only if it's still the current one (seq match).
type FlashClearMsg struct{ Seq int }

// BeatMsg advances visual liveness affordances. It must not change read receipts, authority,
// readiness, or any source-derived data; those only move when a source fold arrives.
type BeatMsg struct{}

// flash sets a transient confirmation + arms a tick to clear it. Returned cmd must propagate (it does:
// the handlers return it, and root.Update forwards the model's cmd). The seq guards against a stale
// tick wiping a newer flash.
func (m Model) flash(msg string) (Model, tea.Cmd) {
	m.FlashSeq++
	m.Flash = msg
	seq := m.FlashSeq
	return m, tea.Tick(900*time.Millisecond, func(time.Time) tea.Msg { return FlashClearMsg{Seq: seq} })
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

// FocusedEvent: the event under the :events cursor (the events analogue of FocusedTask).
func (m Model) FocusedEvent() (grammar.Event, bool) {
	if m.EFocus < 0 || m.EFocus >= len(m.Events) {
		return grammar.Event{}, false
	}
	return m.Events[m.EFocus], true
}

// FocusedSession: the live lane/session under the :sessions cursor.
func (m Model) FocusedSession() (grammar.Session, bool) {
	if m.SFocus < 0 || m.SFocus >= len(m.Sessions) {
		return grammar.Session{}, false
	}
	return m.Sessions[m.SFocus], true
}

// FocusedIntakeRow returns the aggregate observation bucket under the :intake cursor.
func (m Model) FocusedIntakeRow() (grammar.IntakeRow, bool) {
	rows := m.visibleIntakeRows()
	if m.IFocus < 0 || m.IFocus >= len(rows) {
		return grammar.IntakeRow{}, false
	}
	return rows[m.IFocus], true
}

// FocusedCapabilityRow returns the capability/status row under the :capabilities cursor.
func (m Model) FocusedCapabilityRow() (capabilityStatusRow, bool) {
	rows := m.capabilityDisplayRows()
	if m.CFocus < 0 || m.CFocus >= len(rows) {
		return capabilityStatusRow{}, false
	}
	return rows[m.CFocus], true
}

func (m Model) FocusedCommand() (verbDef, bool) {
	if m.CommandFocus < 0 || m.CommandFocus >= len(verbs) {
		return verbDef{}, false
	}
	return verbs[m.CommandFocus], true
}

func (m Model) FocusedWindow() (WindowDef, bool) {
	rows := registeredWindows()
	if m.WindowFocus < 0 || m.WindowFocus >= len(rows) {
		return WindowDef{}, false
	}
	return rows[m.WindowFocus], true
}

func (m Model) FocusedSurface() (SurfaceDef, bool) {
	rows := registeredSurfaces()
	if m.SurfaceFocus < 0 || m.SurfaceFocus >= len(rows) {
		return SurfaceDef{}, false
	}
	return rows[m.SurfaceFocus], true
}

func (m Model) FocusedDomain() (DomainDef, bool) {
	rows := registeredDomains()
	if m.DomainFocus < 0 || m.DomainFocus >= len(rows) {
		return DomainDef{}, false
	}
	return rows[m.DomainFocus], true
}

func (m Model) FocusedLifecycleFallback() (LifecycleFallbackDef, bool) {
	rows := registeredLifecycleFallbacks()
	if m.LifecycleFocus < 0 || m.LifecycleFocus >= len(rows) {
		return LifecycleFallbackDef{}, false
	}
	return rows[m.LifecycleFocus], true
}

func (m Model) FocusedDomainRow() (grammar.DomainRow, bool) {
	if len(m.Domains.Rows) == 0 || m.DomainFocus < 0 || m.DomainFocus >= len(m.Domains.Rows) {
		return grammar.DomainRow{}, false
	}
	return m.Domains.Rows[m.DomainFocus], true
}

func (m Model) FocusedLifecycleRow() (grammar.LifecycleRow, bool) {
	if len(m.Domains.Lifecycles) == 0 || m.LifecycleFocus < 0 || m.LifecycleFocus >= len(m.Domains.Lifecycles) {
		return grammar.LifecycleRow{}, false
	}
	return m.Domains.Lifecycles[m.LifecycleFocus], true
}

func (m Model) FocusedEpistemicRow() (epistemicRow, bool) {
	rows := m.epistemicRows()
	if m.EpiFocus < 0 || m.EpiFocus >= len(rows) {
		return epistemicRow{}, false
	}
	return rows[m.EpiFocus], true
}

func (m Model) FocusedDynamicsElement() (dynamicsFocusRow, bool) {
	rows := m.dynamicsFocusRows()
	if len(rows) == 0 {
		return dynamicsFocusRow{}, false
	}
	idx := clamp(m.DynFocus, 0, len(rows)-1)
	return rows[idx], true
}

func (m Model) domainRowCount() int {
	if len(m.Domains.Rows) > 0 {
		return len(m.Domains.Rows)
	}
	return len(registeredDomains())
}

func (m Model) lifecycleRowCount() int {
	if len(m.Domains.Lifecycles) > 0 {
		return len(m.Domains.Lifecycles)
	}
	return len(registeredLifecycleFallbacks())
}

func (m Model) splitContextActive() bool {
	if !m.SplitContext {
		return false
	}
	if m.Width < splitContextMinWidth {
		return false
	}
	_, _, ok := splitContextWidths(m.Width)
	return ok
}

// pageRows: how many selectable rows the CURRENT page has (selection is page-aware — :tasks rows,
// :events rows; other pages have none). The cursor + its verbs operate on THIS.
func (m Model) pageRows() int {
	switch m.Page {
	case PageTasks:
		return len(m.visibleTasks())
	case PageEvents:
		return len(m.Events)
	case PageSessions:
		return len(m.Sessions)
	case PageIntake:
		return len(m.visibleIntakeRows())
	case PageCaps:
		return len(m.capabilityDisplayRows())
	case PageDynamics:
		return len(m.dynamicsFocusRows())
	case PageCommands:
		return len(verbs)
	case PageWindows:
		return len(registeredWindows())
	case PageSurfaces:
		return len(registeredSurfaces())
	case PageDomains:
		return m.domainRowCount()
	case PageLifecycles:
		return m.lifecycleRowCount()
	case PageEpistemics:
		return len(m.epistemicRows())
	case PageIntent:
		return len(lookupIntentArgs())
	}
	return 0
}

// isReferencePage reports pages with no row cursor but scrollable full-width explanatory content.
func (m Model) isReferencePage() bool {
	return m.Page == PageDynamics || m.Page == PageHelp || m.Page == PageLegend ||
		m.Page == PageCommands || m.Page == PageWindows || m.Page == PageIntent ||
		m.Page == PageSurfaces || m.Page == PageDomains || m.Page == PageLifecycles || m.Page == PageYard ||
		m.Page == PageReadiness || m.Page == PageCaps || m.Page == PageIntake ||
		m.Page == PageEpistemics
}

// hasRows reports whether the current page has any selectable rows.
func (m Model) hasRows() bool { return m.pageRows() > 0 }

// curFocus: the focus index for the current page (Focus for :tasks, EFocus for :events).
func (m Model) curFocus() int {
	if m.Page == PageEvents {
		return m.EFocus
	}
	if m.Page == PageSessions {
		return m.SFocus
	}
	if m.Page == PageIntake {
		return m.IFocus
	}
	if m.Page == PageCaps {
		return m.CFocus
	}
	if m.Page == PageDynamics {
		return m.DynFocus
	}
	if m.Page == PageCommands {
		return m.CommandFocus
	}
	if m.Page == PageWindows {
		return m.WindowFocus
	}
	if m.Page == PageSurfaces {
		return m.SurfaceFocus
	}
	if m.Page == PageDomains {
		return m.DomainFocus
	}
	if m.Page == PageLifecycles {
		return m.LifecycleFocus
	}
	if m.Page == PageEpistemics {
		return m.EpiFocus
	}
	if m.Page == PageIntent {
		return m.IntentFocus
	}
	return m.Focus
}

// focusTo: set the current page's cursor to i (clamped to its row count). One mover for both pages,
// so j/k/g/G stay page-agnostic and can never drive a focus the page doesn't render.
func (m Model) focusTo(i int) Model {
	if m.Page != PageEvents && m.Page != PageTasks && m.Page != PageSessions && m.Page != PageIntake && m.Page != PageCaps && m.Page != PageDynamics && m.Page != PageCommands && m.Page != PageWindows && m.Page != PageSurfaces && m.Page != PageDomains && m.Page != PageLifecycles && m.Page != PageEpistemics && m.Page != PageIntent {
		return m
	}
	max := m.pageRows() - 1
	if max < 0 {
		max = 0
	}
	i = clamp(i, 0, max)
	if m.Page == PageEvents {
		m.EFocus = i
	} else if m.Page == PageSessions {
		m.SFocus = i
	} else if m.Page == PageIntake {
		m.IFocus = i
	} else if m.Page == PageCaps {
		m.CFocus = i
	} else if m.Page == PageDynamics {
		m.DynFocus = i
	} else if m.Page == PageCommands {
		m.CommandFocus = i
	} else if m.Page == PageWindows {
		m.WindowFocus = i
	} else if m.Page == PageSurfaces {
		m.SurfaceFocus = i
	} else if m.Page == PageDomains {
		m.DomainFocus = i
	} else if m.Page == PageLifecycles {
		m.LifecycleFocus = i
	} else if m.Page == PageEpistemics {
		m.EpiFocus = i
	} else if m.Page == PageIntent {
		m.IntentFocus = i
	} else {
		m.Focus = i
	}
	return m
}

func (m Model) sessionFocusTo(i int) Model {
	max := len(m.Sessions) - 1
	if max < 0 {
		max = 0
	}
	m.SFocus = clamp(i, 0, max)
	return m
}

func (m Model) intakeFocusTo(i int) Model {
	max := len(m.visibleIntakeRows()) - 1
	if max < 0 {
		max = 0
	}
	m.IFocus = clamp(i, 0, max)
	return m
}

func (m Model) capabilityFocusTo(i int) Model {
	max := len(m.capabilityDisplayRows()) - 1
	if max < 0 {
		m.CFocus = 0
		m.RefScroll = 0
		m.Status = "capability: no capability rows"
		return m
	}
	m.CFocus = clamp(i, 0, max)
	m.RefScroll = m.capabilityFocusScrollOffset(m.CFocus)
	m.Status = fmt.Sprintf("capability %d/%d", m.CFocus+1, max+1)
	return m
}

func (m Model) dynamicsFocusTo(i int) Model {
	rows := m.dynamicsFocusRows()
	max := len(rows) - 1
	if max < 0 {
		m.DynFocus = 0
		m.Status = "dynamics: no map elements"
		return m
	}
	m.DynFocus = clamp(i, 0, max)
	row := rows[m.DynFocus]
	m.Status = fmt.Sprintf("dynamics %s %d/%d: %s", row.Kind, m.DynFocus+1, max+1, firstNonEmpty(row.ID, row.Label, "·"))
	return m
}

func (m Model) commandFocusTo(i int) Model {
	max := len(verbs) - 1
	if max < 0 {
		m.CommandFocus = 0
		m.RefScroll = 0
		m.Status = "command: no registry rows"
		return m
	}
	m.CommandFocus = clamp(i, 0, max)
	m.RefScroll = maxVisible(0, m.CommandFocus-2)
	m.Status = fmt.Sprintf("command %d/%d", m.CommandFocus+1, max+1)
	return m
}

func (m Model) windowFocusTo(i int) Model {
	rows := registeredWindows()
	max := len(rows) - 1
	if max < 0 {
		m.WindowFocus = 0
		m.RefScroll = 0
		m.Status = "window: no registry rows"
		return m
	}
	m.WindowFocus = clamp(i, 0, max)
	m.RefScroll = maxVisible(0, m.WindowFocus-2)
	m.Status = fmt.Sprintf("window %d/%d", m.WindowFocus+1, max+1)
	return m
}

func (m Model) surfaceFocusTo(i int) Model {
	max := len(registeredSurfaces()) - 1
	if max < 0 {
		m.SurfaceFocus = 0
		m.RefScroll = 0
		m.Status = "surface: no registry rows"
		return m
	}
	m.SurfaceFocus = clamp(i, 0, max)
	m.RefScroll = maxVisible(0, m.SurfaceFocus-2)
	m.Status = fmt.Sprintf("surface %d/%d", m.SurfaceFocus+1, max+1)
	return m
}

func (m Model) domainFocusTo(i int) Model {
	max := m.domainRowCount() - 1
	if max < 0 {
		m.DomainFocus = 0
		m.RefScroll = 0
		m.Status = "domain: no registry rows"
		return m
	}
	m.DomainFocus = clamp(i, 0, max)
	m.RefScroll = maxVisible(0, m.DomainFocus-2)
	m.Status = fmt.Sprintf("domain %d/%d", m.DomainFocus+1, max+1)
	return m
}

func (m Model) lifecycleFocusTo(i int) Model {
	max := m.lifecycleRowCount() - 1
	if max < 0 {
		m.LifecycleFocus = 0
		m.RefScroll = 0
		m.Status = "lifecycle: no registry rows"
		return m
	}
	m.LifecycleFocus = clamp(i, 0, max)
	m.RefScroll = maxVisible(0, m.LifecycleFocus-2)
	m.Status = fmt.Sprintf("lifecycle %d/%d", m.LifecycleFocus+1, max+1)
	return m
}

func (m Model) epistemicFocusTo(i int) Model {
	max := len(m.epistemicRows()) - 1
	if max < 0 {
		m.EpiFocus = 0
		m.RefScroll = 0
		m.Status = "epistemics: no rows"
		return m
	}
	m.EpiFocus = clamp(i, 0, max)
	m.RefScroll = m.epistemicFocusScrollOffset(m.EpiFocus)
	m.Status = fmt.Sprintf("epistemic row %d/%d", m.EpiFocus+1, max+1)
	return m
}

func (m Model) intentFocusTo(i int) Model {
	args := lookupIntentArgs()
	max := len(args) - 1
	if max < 0 {
		m.IntentFocus = 0
		m.Status = "intent target: no registered targets"
		return m
	}
	m.IntentFocus = clamp(i, 0, max)
	m.Status = fmt.Sprintf("intent target %d/%d", m.IntentFocus+1, max+1)
	return m
}

func (m Model) selectedIntentArg() (Candidate, bool) {
	args := lookupIntentArgs()
	if len(args) == 0 {
		return Candidate{}, false
	}
	i := clamp(m.IntentFocus, 0, len(args)-1)
	return args[i], true
}

func intentArgIndex(label string) int {
	for i, a := range lookupIntentArgs() {
		if a.Label == label {
			return i
		}
	}
	return 0
}

func (m Model) cycleIntakeSourceFilter(delta int) Model {
	ids := m.intakeSourceIDs()
	if len(ids) == 0 {
		m.IntakeSourceFilter = ""
		m.IFocus = 0
		m.Status = "intake source: no sources"
		return m
	}
	opts := append([]string{""}, ids...)
	cur := 0
	for i, opt := range opts {
		if opt == m.IntakeSourceFilter {
			cur = i
			break
		}
	}
	next := (cur + delta + len(opts)) % len(opts)
	m.IntakeSourceFilter = opts[next]
	m.IFocus = 0
	m.RefScroll = 0
	if m.IntakeSourceFilter == "" {
		m.Status = fmt.Sprintf("intake source: all (%d buckets)", len(m.visibleIntakeRows()))
	} else {
		m.Status = fmt.Sprintf("intake source: %s (%d buckets)", m.IntakeSourceFilter, len(m.visibleIntakeRows()))
	}
	return m
}

func (m Model) yankFocusTo(i int) Model {
	if m.splitContextActive() {
		return m.sessionFocusTo(i)
	}
	return m.focusTo(i)
}

func (m Model) yankCurFocus() int {
	if m.splitContextActive() {
		return m.SFocus
	}
	return m.curFocus()
}

func (m Model) yankRows() int {
	if m.splitContextActive() {
		return len(m.Sessions)
	}
	return m.pageRows()
}

// dynScales maps the seed's view_scale names to their resolution index (1=overview … 5=evidence).
var dynScales = map[string]int{
	"overview": 1, "domain": 2, "artifact": 3, "runtime": 4, "evidence": 5, "all": 0,
}

var dynScaleOrder = []int{0, 1, 2, 3, 4, 5}
var dynScaleNames = map[int]string{0: "all", 1: "overview", 2: "domain", 3: "artifact", 4: "runtime", 5: "evidence"}

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

func dynScaleName(scale int) string {
	if s, ok := dynScaleNames[scale]; ok {
		return s
	}
	return "all"
}

func dynScaleShort(scale int) string {
	switch scale {
	case 1:
		return "ov"
	case 2:
		return "dom"
	case 3:
		return "art"
	case 4:
		return "run"
	case 5:
		return "ev"
	}
	return "all"
}

func (m Model) cycleDynamicsScale(delta int) Model {
	idx := 0
	for i, s := range dynScaleOrder {
		if s == m.DynScale {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(dynScaleOrder)) % len(dynScaleOrder)
	m.DynScale = dynScaleOrder[idx]
	if n := len(m.dynamicsFocusRows()); n > 0 && m.DynFocus >= n {
		m.DynFocus = n - 1
	}
	m.Status = ":dynamics @" + dynScaleName(m.DynScale)
	return m
}

func (m Model) epistemicIndexForDynamicsFocus() (int, bool) {
	focus, ok := m.FocusedDynamicsElement()
	if !ok {
		return 0, false
	}
	rows := m.epistemicRows()
	if len(rows) == 0 {
		return 0, false
	}
	if idx, ok := epistemicIndexForExactDynamicsRow(rows, focus); ok {
		return idx, true
	}
	keys := append([]string{}, focus.MatchKeys...)
	keys = append(keys, focus.ID, focus.Label, focus.Relation)
	if focus.Kind == "node" {
		keys = append(keys, "node:"+focus.Status)
	}
	if focus.Kind == "edge" {
		keys = append(keys, "edge:"+focus.Status)
	}
	if focus.Kind == "source" {
		keys = append(keys, focus.Source)
	}
	normKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.ToLower(strings.TrimSpace(key))
		if key != "" && key != "▒▒▒" {
			normKeys = append(normKeys, key)
		}
	}
	for i, row := range rows {
		if isMapEpistemicRow(row) {
			continue
		}
		subj := strings.ToLower(strings.TrimSpace(row.Subject))
		for _, key := range normKeys {
			if subj == key {
				return i, true
			}
		}
	}
	for i, row := range rows {
		if isMapEpistemicRow(row) {
			continue
		}
		subj := strings.ToLower(strings.TrimSpace(row.Subject))
		detail := strings.ToLower(strings.TrimSpace(row.Detail))
		for _, key := range normKeys {
			if strings.Contains(subj, key) || strings.Contains(detail, key) {
				return i, true
			}
		}
	}
	return 0, false
}

func epistemicIndexForExactDynamicsRow(rows []epistemicRow, focus dynamicsFocusRow) (int, bool) {
	var family string
	candidates := []string{focus.ID}
	switch focus.Kind {
	case "node":
		family = "map-node"
		candidates = append(candidates, focus.RawID)
	case "edge":
		family = "map-edge"
		candidates = append(candidates, focus.RawID, dynamicsEdgeSubjectForFocus(focus))
	default:
		return 0, false
	}
	keys := make([]string, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		key := strings.ToLower(strings.TrimSpace(candidate))
		if key == "" || key == "▒▒▒" || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return 0, false
	}
	for i, row := range rows {
		if row.Family != family {
			continue
		}
		if focus.Kind == "node" && strings.TrimSpace(row.MapID) != "" && strings.EqualFold(strings.TrimSpace(row.MapID), strings.TrimSpace(focus.RawID)) {
			return i, true
		}
		if focus.Kind == "edge" && strings.TrimSpace(row.MapSource) != "" && strings.TrimSpace(row.MapTarget) != "" && strings.TrimSpace(row.MapRelation) != "" &&
			strings.EqualFold(strings.TrimSpace(row.MapSource), strings.TrimSpace(focus.RawSource)) &&
			strings.EqualFold(strings.TrimSpace(row.MapTarget), strings.TrimSpace(focus.RawTarget)) &&
			strings.EqualFold(strings.TrimSpace(row.MapRelation), strings.TrimSpace(focus.RawRelation)) {
			return i, true
		}
		subj := strings.ToLower(strings.TrimSpace(row.Subject))
		mapID := strings.ToLower(strings.TrimSpace(row.MapID))
		for _, key := range keys {
			if subj == key || mapID == key {
				return i, true
			}
		}
	}
	return 0, false
}

func isMapEpistemicRow(row epistemicRow) bool {
	return row.Family == "map-node" || row.Family == "map-edge"
}

func (m Model) openEpistemicsForDynamicsFocus() Model {
	focus, hasFocus := m.FocusedDynamicsElement()
	idx, ok := m.epistemicIndexForDynamicsFocus()
	m = m.switchPage(PageEpistemics)
	if ok {
		m.EpiFocus = idx
		m.RefScroll = m.epistemicFocusScrollOffset(idx)
		m.Status = fmt.Sprintf(":epistemics ⇐ dynamics %s %s", focus.Kind, firstNonEmpty(focus.ID, focus.Label, "·"))
		return m
	}
	if hasFocus {
		m.Status = fmt.Sprintf(":epistemics · no direct row for dynamics %s %s", focus.Kind, firstNonEmpty(focus.ID, focus.Label, "·"))
	} else {
		m.Status = ":epistemics · no dynamics focus"
	}
	return m
}

func (m Model) epistemicIndexForIntakeFocus() (int, bool) {
	focus, ok := m.FocusedIntakeRow()
	if !ok {
		return 0, false
	}
	keys := []string{
		firstNonEmpty(focus.ID, focus.Kind),
		focus.Kind,
		focus.Source + ":" + focus.Kind,
	}
	normKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.ToLower(strings.TrimSpace(key))
		if key != "" && key != "▒▒▒" {
			normKeys = append(normKeys, key)
		}
	}
	for i, row := range m.epistemicRows() {
		subj := strings.ToLower(strings.TrimSpace(row.Subject))
		for _, key := range normKeys {
			if subj == key {
				return i, true
			}
		}
	}
	for i, row := range m.epistemicRows() {
		subj := strings.ToLower(strings.TrimSpace(row.Subject))
		detail := strings.ToLower(strings.TrimSpace(row.Detail))
		for _, key := range normKeys {
			if strings.Contains(subj, key) || strings.Contains(detail, key) {
				return i, true
			}
		}
	}
	return 0, false
}

func (m Model) openEpistemicsForIntakeFocus() Model {
	focus, hasFocus := m.FocusedIntakeRow()
	idx, ok := m.epistemicIndexForIntakeFocus()
	m = m.switchPage(PageEpistemics)
	if ok {
		m.EpiFocus = idx
		m.RefScroll = m.epistemicFocusScrollOffset(idx)
		m.Status = fmt.Sprintf(":epistemics ⇐ intake %s", firstNonEmpty(focus.ID, focus.Kind, "·"))
		return m
	}
	if hasFocus {
		m.Status = fmt.Sprintf(":epistemics · no direct row for intake %s", firstNonEmpty(focus.ID, focus.Kind, "·"))
	} else {
		m.Status = ":epistemics · no intake focus"
	}
	return m
}

func New(title string) Model {
	return Model{Title: title, Sel: Selection{Rank: RankRow, Type: "task"}, EventScrollback: Scrollback{Cap: 512}}
}

// Fold: the pure projection for the :events page. No hidden state; re-folding restores the view.
func (m Model) Fold(evs []grammar.Event, dark bool) Model {
	// follow the tail: if the cursor was on (or past) the newest row, keep it pinned to newest as the
	// stream grows — so the :events cursor defaults to (and tracks) the latest event, matching the
	// newest-at-bottom window. If the operator scrolled up, hold their position (clamped into range).
	follow := m.EFocus >= len(m.Events)-1
	m.Events = evs
	m.EventsDark = dark
	if follow || m.EFocus >= len(m.Events) {
		m.EFocus = len(m.Events) - 1
	}
	if m.EFocus < 0 {
		m.EFocus = 0
	}
	return m
}

// FoldTasks: the pure projection for the :tasks registry page.
func (m Model) FoldTasks(ts []grammar.Task, dark bool) Model {
	m.Tasks = ts
	m.TasksDark = dark
	m.Focus = clamp(m.Focus, 0, m.focusMax())
	m.Sel.Members = nil
	if _, ok := m.FocusedTask(); !ok {
		m.Sel.Rank, m.Sel.Field = RankRow, ""
	}
	return m
}

// FoldSessions: the pure projection for the :sessions live-lane roster page.
func (m Model) FoldSessions(ss []grammar.Session, dark bool) Model {
	m.Sessions = ss
	if len(m.Sessions) == 0 {
		m.SFocus = 0
	} else if m.SFocus >= len(m.Sessions) {
		m.SFocus = len(m.Sessions) - 1
	} else if m.SFocus < 0 {
		m.SFocus = 0
	}
	m.SessionsDark = dark
	return m
}

func (m Model) FoldSessionDetail(d grammar.SessionDetail, dark bool) Model {
	m.SessionDetail = d
	m.SessionDetailDark = dark
	return m
}

// FoldIntake: the pure projection for the :intake observation/demand page.
func (m Model) FoldIntake(in grammar.IntakeSummary, dark bool) Model {
	m.Intake = in
	m.IntakeDark = dark
	if strings.TrimSpace(m.IntakeSourceFilter) != "" {
		found := false
		for _, id := range m.intakeSourceIDs() {
			if id == m.IntakeSourceFilter {
				found = true
				break
			}
		}
		if !found {
			m.IntakeSourceFilter = ""
		}
	}
	m = m.intakeFocusTo(m.IFocus)
	return m
}

// FoldCapabilities: the pure projection for capability-routing source state.
func (m Model) FoldCapabilities(caps grammar.CapabilitySummary, dark bool) Model {
	m.Capabilities = caps
	m.CapabilitiesDark = dark
	max := len(m.capabilityDisplayRows()) - 1
	if max < 0 {
		m.CFocus = 0
	} else {
		m.CFocus = clamp(m.CFocus, 0, max)
	}
	return m
}

// FoldGates: the pure projection for source-backed readiness/gate state.
func (m Model) FoldGates(gates grammar.GateSummary, dark bool) Model {
	m.Gates = gates
	m.GatesDark = dark
	return m
}

// FoldDomains: the pure projection for optional source-backed lifecycle/domain packs.
func (m Model) FoldDomains(domains grammar.DomainSummary, dark bool) Model {
	m.Domains = domains
	m.DomainsDark = dark
	return m
}

// FoldDynamics: the pure projection for the :dynamics system-dynamics-map page.
func (m Model) FoldDynamics(g grammar.Graph, dark bool) Model {
	m.Dynamics = g
	m.DynamicsDark = dark
	return m
}

// FoldEpistemics: the pure projection for typed evidence/provenance rows.
func (m Model) FoldEpistemics(ep grammar.EpistemicsSummary, dark bool) Model {
	m.Epistemics = ep
	m.EpistemicsDark = dark
	if n := len(m.epistemicRows()); n > 0 {
		m.EpiFocus = clamp(m.EpiFocus, 0, n-1)
	} else {
		m.EpiFocus = 0
	}
	return m
}

// commandKind names the current command authority surface. Today these are local/read/intent stubs;
// later COMMAND/ROUTE/GOVERN verbs can register here without adding one-off key paths.
type commandKind string

const (
	commandRead   commandKind = "read"
	commandLens   commandKind = "lens"
	commandLocal  commandKind = "local"
	commandIntent commandKind = "intent"
)

// verbDef + verbs: the command vocabulary, surfaced as a which-key menu at the point of action
// (recognition over recall — never make the operator memorize the verbs). This is the seed of the
// unified command catalog: keyboard, command line, probes, future voice/MCP, and intent doors should
// all route through this metadata instead of bespoke command strings.
type verbDef struct {
	name, group, gloss string
	aliases            []string
	kind               commandKind
	authority          string
	preflight          string
	receipt            string
	uiDelta            string
	freeform           bool
	args               []Candidate // sub-menu: this verb's argument candidates (nil = a leaf verb, no args)
}

func (v verbDef) detail() string {
	if v.group == "" {
		return string(v.kind) + " · " + v.gloss
	}
	return string(v.kind) + "/" + v.group + " · " + v.gloss
}

func (v verbDef) matchesPrefix(prefix string) bool {
	if strings.HasPrefix(v.name, prefix) {
		return true
	}
	for _, a := range v.aliases {
		if strings.HasPrefix(a, prefix) {
			return true
		}
	}
	return false
}

var verbs = []verbDef{
	{name: "events", aliases: []string{"e"}, kind: commandRead, group: "window", gloss: "live coord event stream", authority: "local_read", receipt: "none", uiDelta: "switch window"},
	{name: "tasks", aliases: []string{"t"}, kind: commandRead, group: "window", gloss: "the task registry", authority: "local_read", receipt: "none", uiDelta: "switch window"},
	{name: "sessions", aliases: []string{"s"}, kind: commandRead, group: "window", gloss: "live agent/session roster", authority: "local_read", receipt: "none", uiDelta: "switch window"},
	{name: "yard", kind: commandRead, group: "window", gloss: "Trainyard-style SDLC cockpit over live Reins read models", authority: "local_read", receipt: "none", uiDelta: "switch window"},
	{name: "readiness", aliases: []string{"ready", "gates", "gate"}, kind: commandRead, group: "window", gloss: "gates/readiness projection across read sources, lanes, tasks, and command routes", authority: "local_read", receipt: "none", uiDelta: "switch window"},
	{name: "intake", aliases: []string{"observations", "obs", "inbox"}, kind: commandRead, group: "window", gloss: "source-backed intake observations/demand projection", authority: "local_read", receipt: "none", uiDelta: "switch window"},
	{name: "capabilities", aliases: []string{"caps", "cap"}, kind: commandRead, group: "window", gloss: "capability routing fit/admission projection", authority: "local_read", receipt: "none", uiDelta: "switch window"},
	{name: "dynamics", aliases: []string{"d"}, kind: commandRead, group: "window", gloss: "the system-dynamics map", authority: "local_read", receipt: "none", uiDelta: "switch window", args: []Candidate{
		{Label: "overview", Detail: "the whole map at a glance"},
		{Label: "domain", Detail: "the domain layer"},
		{Label: "artifact", Detail: "the artifact layer"},
		{Label: "runtime", Detail: "the runtime layer"},
		{Label: "evidence", Detail: "the evidence layer"},
		{Label: "all", Detail: "every layer, unscaled"},
	}},
	{name: "epistemics", aliases: []string{"epi", "epistemic"}, kind: commandRead, group: "window", gloss: "evidence/provenance posture over dynamics, observations, domains, capabilities, and sessions", authority: "local_read", receipt: "none", uiDelta: "switch window"},
	{name: "legend", aliases: []string{"?"}, kind: commandRead, group: "reference", gloss: "decode the grammar — every glyph/color/cell", authority: "local_read", receipt: "none", uiDelta: "switch window"},
	{name: "help", aliases: []string{"h"}, kind: commandRead, group: "reference", gloss: "the help page", authority: "local_read", receipt: "none", uiDelta: "switch window"},
	{name: "commands", aliases: []string{"cmds"}, kind: commandRead, group: "registry", gloss: "open the unified command catalog", authority: "local_read", receipt: "none", uiDelta: "switch window"},
	{name: "windows", aliases: []string{"wins"}, kind: commandRead, group: "registry", gloss: "open the lifecycle/window registry", authority: "local_read", receipt: "none", uiDelta: "switch window"},
	{name: "surfaces", aliases: []string{"surf"}, kind: commandRead, group: "registry", gloss: "open the transient surface/mode registry", authority: "local_read", receipt: "none", uiDelta: "switch window"},
	{name: "domains", aliases: []string{"domain", "terrain"}, kind: commandRead, group: "registry", gloss: "open the domain/terrain lens registry", authority: "local_read", receipt: "none", uiDelta: "switch window"},
	{name: "lifecycles", aliases: []string{"life", "lifecycle", "ndlc", "n-dlc"}, kind: commandRead, group: "registry", gloss: "open the tenant lifecycle contract registry", authority: "local_read", receipt: "none", uiDelta: "switch window"},
	{name: "note", aliases: []string{"n"}, kind: commandLocal, group: "compose", gloss: "stage a note — refs like {{sel.id}} expand (AIR-safe)", authority: "local_ephemeral", preflight: "AIR free-text display check", receipt: "status only", uiDelta: "status echo", freeform: true},
	{name: "air", kind: commandLens, group: "safety", gloss: "the on-air PII lens", authority: "local_lens", preflight: "none", receipt: "visible lens state", uiDelta: "re-render frame", args: []Candidate{
		{Label: "on", Detail: "redact non-allowlisted cells (broadcast-safe)"},
		{Label: "off", Detail: "show everything (LOCAL only)"},
	}},
	{name: "intent", kind: commandIntent, group: "govern", gloss: "open a review-before-run intent preview; no effect emitted", authority: "governed COMMAND route required", preflight: "target + authority + mutation surface + receipt contract", receipt: "preview only", uiDelta: "intent status", args: []Candidate{
		{Label: "resume", Detail: "preview session resume through governed route"},
		{Label: "dispatch", Detail: "preview methodology dispatch; no launch"},
		{Label: "claim", Detail: "preview cc-claim target and authority"},
		{Label: "close", Detail: "preview cc-close receipt requirements"},
		{Label: "approve", Detail: "preview approval target and evidence"},
		{Label: "deny", Detail: "preview denial target and evidence"},
		{Label: "handoff", Detail: "preview handoff context package"},
		{Label: "open-trace", Detail: "preview trace/span focus"},
		{Label: "show-route", Detail: "preview route evidence and veto chain"},
	}},
	{name: "quit", aliases: []string{"q"}, kind: commandLocal, group: "app", gloss: "leave", authority: "local_app", receipt: "process exit", uiDelta: "quit"},
}

// lookupVerb finds a verb by exact name.
func lookupVerb(name string) (verbDef, bool) {
	for _, v := range verbs {
		if v.name == name {
			return v, true
		}
		for _, a := range v.aliases {
			if a == name {
				return v, true
			}
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
		if v.matchesPrefix(prefix) {
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
	line = m.resolveTemplate(line) // expand {{sel}}/{{ring.N}} refs (AIR-safe) before parsing
	f := strings.Fields(strings.TrimSpace(line))
	if len(f) == 0 {
		return m
	}
	verb, args := f[0], f[1:]
	vd, ok := lookupVerb(verb)
	if !ok {
		m.Status = "unknown command: " + verb
		return m
	}
	switch vd.name {
	case "events":
		m = m.switchPage(PageEvents)
		m.Status = ":events"
	case "tasks":
		m = m.switchPage(PageTasks)
		m.Status = ":tasks"
	case "sessions":
		m = m.switchPage(PageSessions)
		m.Status = ":sessions"
	case "yard":
		m = m.switchPage(PageYard)
		m.Status = ":yard"
	case "readiness":
		m = m.switchPage(PageReadiness)
		m.Status = ":readiness"
	case "intake":
		m = m.switchPage(PageIntake)
		m.Status = ":intake"
	case "capabilities":
		m = m.switchPage(PageCaps)
		m.Status = ":capabilities"
	case "dynamics":
		m = m.switchPage(PageDynamics)
		if s := arg0(args); s != "" { // :dynamics <scale> — overview|domain|artifact|runtime|evidence|1..5|all
			m.DynScale, m.Status = scaleIndex(s), ":dynamics @"+s
		} else {
			m.Status = ":dynamics"
		}
	case "epistemics":
		m = m.switchPage(PageEpistemics)
		m.Status = ":epistemics"
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
	case "note", "n": // a free-text sink — the {{…}} refs already expanded above (the template seed)
		if len(args) == 0 {
			m.Status = "note: reference the selection, e.g. note {{sel.id}} at {{sel.stage}}"
		} else if m.AIR {
			m.Status = "note ▸ ▒▒▒ (free text hidden on AIR)"
		} else {
			m.Status = "note ▸ " + strings.Join(args, " ")
		}
	case "help":
		m = m.switchPage(PageHelp)
		m.Status = ":help"
	case "legend":
		m = m.switchPage(PageLegend)
		m.Status = ":legend"
	case "lastlog":
		m.LastlogDoorOpen = true
		m.Status = ":lastlog · event-history scrollback · Esc close"
	case "commands":
		m = m.switchPage(PageCommands)
		m.Status = ":commands · " + m.commandsSummary()
	case "windows":
		m = m.switchPage(PageWindows)
		m.Status = ":windows · " + windowsSummary()
	case "surfaces":
		m = m.switchPage(PageSurfaces)
		m.Status = ":surfaces · " + surfacesSummary()
	case "domains":
		m = m.switchPage(PageDomains)
		m.Status = ":domains · " + domainsSummary()
	case "lifecycles":
		m = m.switchPage(PageLifecycles)
		m.Status = ":lifecycles · " + lifecyclesSummary(len(m.Domains.Lifecycles))
	case "intent":
		target := arg0(args)
		subject := m.selectedIntentSubject()
		m.IntentTarget, m.IntentSubject = target, subject
		if target != "" {
			m.IntentFocus = intentArgIndex(target)
		} else {
			m.IntentFocus = 0
		}
		m = m.switchPage(PageIntent)
		m.IntentTarget, m.IntentSubject = target, subject
		m.Status = m.intentStatusFor(target, subject)
	case "quit":
		m.Quitting, m.Status = true, "bye"
	}
	return m
}

func (m Model) commandsSummary() string {
	counts := map[commandKind]int{}
	needsGoverned := 0
	for _, v := range verbs {
		counts[v.kind]++
		if v.kind == commandIntent || strings.Contains(v.authority, "governed") {
			needsGoverned++
		}
	}
	return fmt.Sprintf("commands: %d registered · read %d · lens %d · local %d · intent %d · governed %d · [Tab] browses",
		len(verbs), counts[commandRead], counts[commandLens], counts[commandLocal], counts[commandIntent], needsGoverned)
}

func (m Model) intentStatus(target string) string {
	return m.intentStatusFor(target, m.selectedIntentSubject())
}

func (m Model) intentStatusFor(target, subject string) string {
	if target == "" {
		return "intent: choose resume/dispatch/claim/close/approve/deny/handoff/open-trace/show-route"
	}
	for _, a := range lookupIntentArgs() {
		if a.Label == target {
			v, _ := lookupVerb("intent")
			return "intent " + target + ": review-before-run preview stub · " + v.authority +
				" · selection " + subject +
				" · preflight " + v.preflight + " · receipt " + v.receipt + " · no effect emitted"
		}
	}
	return "intent: unknown target " + target + " · no effect emitted"
}

func lookupIntentArgs() []Candidate {
	if v, ok := lookupVerb("intent"); ok {
		return v.args
	}
	return nil
}

func (m Model) selectedIntentSubject() string {
	switch m.commandSelectionPage() {
	case PageTasks:
		if t, ok := m.FocusedTask(); ok {
			return "task " + grammar.Redact(t.AIR, "task_id", t.TaskID, m.AIR)
		}
	case PageSessions:
		if s, ok := m.FocusedSession(); ok {
			return "session " + grammar.Redact(s.AIR, "role", s.Role, m.AIR)
		}
	case PageEvents:
		if ev, ok := m.FocusedEvent(); ok {
			ref := grammar.Redact(ev.AIR, "subject", ev.Subject, m.AIR)
			if ref == "" {
				ref = grammar.Redact(ev.AIR, "kind", ev.Kind, m.AIR)
			}
			return "event " + ref
		}
	}
	if w, ok := windowForPage(m.Page); ok {
		return "window " + w.ID
	}
	return "selection none"
}

func (m Model) switchPage(page int) Model {
	m.Page = page
	m.Mode = ModeNormal
	m.DoorOpen = false
	m.SessionDoorOpen = false
	m.IntakeDoorOpen = false
	m.LastlogDoorOpen = false
	m.RefScroll = 0
	m.Sel.Rank, m.Sel.Field, m.Sel.Members = RankRow, "", nil
	if page != PageTasks {
		m.CritFilter = ""
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
	Error  string
}

// LastlogPageRequest fires a backward-page fetch (PgUp in the /lastlog door); the root
// loop honors it like SessionDetailRequest. LastlogPageMsg carries the older events back.
type LastlogPageRequest struct {
	Before string
}

type LastlogPageMsg struct {
	Events []grammar.Event
	Dark   bool
	Error  string
}
type TasksMsg struct {
	Tasks []grammar.Task
	Dark  bool
	Error string
}
type SessionsMsg struct {
	Sessions []grammar.Session
	Dark     bool
	Error    string
}
type SessionDetailRequest struct{ Role string }
type SessionDetailMsg struct {
	Detail grammar.SessionDetail
	Dark   bool
	Error  string
}
type IntakeMsg struct {
	Intake grammar.IntakeSummary
	Dark   bool
	Error  string
}
type CapabilitiesMsg struct {
	Capabilities grammar.CapabilitySummary
	Dark         bool
	Error        string
}
type GatesMsg struct {
	Gates grammar.GateSummary
	Dark  bool
	Error string
}
type DomainsMsg struct {
	Domains grammar.DomainSummary
	Dark    bool
	Error   string
}
type DynamicsMsg struct {
	Graph grammar.Graph
	Dark  bool
	Error string
}
type EpistemicsMsg struct {
	Epistemics grammar.EpistemicsSummary
	Dark       bool
	Error      string
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case EventsMsg:
		m = m.Fold(v.Events, v.Dark)
		m.EventScrollback.Feed(v.Events)
		m.EventsError = v.Error
		m.EventsSeq++
		m.LastFold = "events"
		return m, nil
	case LastlogPageMsg:
		// a PgUp backward-page landed: prepend the older events (newest-at-bottom order)
		m.LastlogPaging = false
		if v.Error == "" {
			m.LastlogOlder = append(v.Events, m.LastlogOlder...)
		}
		return m, nil
	case TasksMsg:
		m = m.FoldTasks(v.Tasks, v.Dark)
		m.TasksError = v.Error
		m.TasksSeq++
		m.LastFold = "tasks"
		return m, nil
	case SessionsMsg:
		m = m.FoldSessions(v.Sessions, v.Dark)
		m.SessionsError = v.Error
		m.SessionsSeq++
		m.LastFold = "sessions"
		return m, nil
	case SessionDetailMsg:
		m = m.FoldSessionDetail(v.Detail, v.Dark)
		m.SessionDetailError = v.Error
		return m, nil
	case IntakeMsg:
		m = m.FoldIntake(v.Intake, v.Dark)
		m.IntakeError = v.Error
		m.IntakeSeq++
		m.LastFold = "intake"
		return m, nil
	case CapabilitiesMsg:
		m = m.FoldCapabilities(v.Capabilities, v.Dark)
		m.CapabilitiesError = v.Error
		m.CapabilitiesSeq++
		m.LastFold = "capabilities"
		return m, nil
	case GatesMsg:
		m = m.FoldGates(v.Gates, v.Dark)
		m.GatesError = v.Error
		m.GatesSeq++
		m.LastFold = "gates"
		return m, nil
	case DomainsMsg:
		m = m.FoldDomains(v.Domains, v.Dark)
		m.DomainsError = v.Error
		m.DomainsSeq++
		m.LastFold = "domains"
		return m, nil
	case DynamicsMsg:
		m = m.FoldDynamics(v.Graph, v.Dark)
		m.DynamicsError = v.Error
		m.DynamicsSeq++
		m.LastFold = "dynamics"
		return m, nil
	case EpistemicsMsg:
		m = m.FoldEpistemics(v.Epistemics, v.Dark)
		m.EpistemicsError = v.Error
		m.EpistemicsSeq++
		m.LastFold = "epistemics"
		return m, nil
	case FlashClearMsg:
		if v.Seq == m.FlashSeq { // ignore a stale tick (a newer flash already superseded it)
			m.Flash = ""
		}
		return m, nil
	case BeatMsg:
		m.Beat++
		return m, nil
	case tea.WindowSizeMsg:
		m.Width, m.Height = v.Width, v.Height // the zones lay out against this
		return m, nil
	case tea.KeyMsg:
		if v.Type == tea.KeyRunes && len(v.Runes) > 1 && !v.Paste {
			var cmd tea.Cmd
			for _, r := range v.Runes {
				var nm tea.Model
				nm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: v.Alt})
				if next, ok := nm.(Model); ok {
					m = next
				}
				if cmd != nil {
					return m, cmd
				}
			}
			return m, nil
		}
		if m.Mode == ModeCommand {
			return m.updateCommand(v)
		}
		if m.Mode == ModeYank {
			return m.updateYank(v)
		}
		if m.DoorOpen {
			return m.updateDoor(v)
		}
		if m.SessionDoorOpen {
			return m.updateSessionDoor(v)
		}
		if m.IntakeDoorOpen {
			return m.updateIntakeDoor(v)
		}
		if m.LastlogDoorOpen {
			return m.updateLastlogDoor(v)
		}
		if m.Mode == ModeHint {
			return m.updateHint(v)
		}
		if m.Mode == ModeFilter {
			return m.updateFilter(v)
		}
		if nm, cmd, ok := m.updateGlobal(v); ok {
			return nm, cmd
		}
		if m.splitContextActive() {
			if nm, cmd, ok := m.updateSplitSource(v); ok {
				return nm, cmd
			}
		}
		if m.Sel.Rank == RankField { // a field within the row is selected — h/l steer, [y] yanks it
			return m.updateField(v)
		}
		switch keyName(v) {
		case "/": // filter — narrow the selectable rows by id substring (incremental)
			if m.Page == PageTasks {
				m.Mode, m.Focus = ModeFilter, 0
				m.Sel.Members = nil
			} else {
				m.Status = "filter: :tasks only"
			}
			return m, nil
		case "f": // hint teleport — labels bloom on visible rows; type one to jump (by looking)
			switch {
			case m.Page != PageTasks:
				m.Status = "hint: :tasks only"
			case len(m.visibleTasks()) == 0:
				m.Status = "hint: no visible task rows"
			default:
				m.Mode = ModeHint
			}
			return m, nil
		case "s":
			if m.Page == PageIntake {
				return m.cycleIntakeSourceFilter(1), nil
			}
		case "S":
			if m.Page == PageIntake {
				return m.cycleIntakeSourceFilter(-1), nil
			}
		case "J":
			if m.isReferencePage() {
				m = m.scrollReference(1)
				m.Status = strings.Replace(m.Status, "scroll", "page scroll", 1)
				return m, nil
			}
		case "K":
			if m.isReferencePage() {
				m = m.scrollReference(-1)
				m.Status = strings.Replace(m.Status, "scroll", "page scroll", 1)
				return m, nil
			}
		case "tab": // descend into the row's fields (navigate by looking; [Tab] again / Esc ascends)
			if m.Page == PageTasks {
				if _, ok := m.FocusedTask(); ok {
					m.Sel.Rank, m.Sel.Field = RankField, selFields[0]
				}
			} else {
				m.Status = "field cursor: :tasks only"
			}
			return m, nil
		case "V": // class-select — every visible row sharing the focused task's criticality (granularity g2)
			if m.Page != PageTasks {
				m.Status = "class-select: :tasks only"
				return m, nil
			}
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
		case "y": // yank — page-aware: a task class/field on :tasks, an event field on :events
			if m.Page == PageEvents {
				if _, ok := m.FocusedEvent(); ok {
					m = m.withYankMode() // the focused EVENT row becomes a navigable field-picker
				} else {
					m.Status = "yank: no event rows"
				}
				return m, nil
			}
			if m.Page == PageSessions {
				if _, ok := m.FocusedSession(); ok {
					m = m.withYankMode() // the focused SESSION row becomes a navigable field-picker
				} else {
					m.Status = "yank: no session rows"
				}
				return m, nil
			}
			if m.Page != PageTasks {
				m.Status = "yank: no selectable rows on this page"
				return m, nil
			}
			if len(m.Sel.Members) > 0 {
				vt := m.visibleTasks()
				vals := make([]string, 0, len(m.Sel.Members))
				classAIR := "ok"
				for _, idx := range m.Sel.Members {
					if idx >= 0 && idx < len(vt) {
						vals = append(vals, vt[idx].TaskID)
						if airDecision(vt[idx].AIR, "task_id") != "ok" {
							classAIR = "deny"
						}
					}
				}
				if m.AIR && classAIR != "ok" {
					m.Status = "yank: class contains task_id redacted on-air — un-yankable"
					return m, nil
				}
				m.Ring = pushRing(m.Ring, RingEntry{Value: strings.Join(vals, "\n"), Field: "task_id", Page: "tasks.class", AIR: classAIR})
				m.Sel.Members = nil
				m.Status = fmt.Sprintf("yanked %d task ids → kill-ring", len(vals))
				return m.flash(fmt.Sprintf("✓ class-yanked %d ids → ring %d", len(vals), len(m.Ring)))
			}
			if _, ok := m.FocusedTask(); ok {
				m = m.withYankMode()
			}
			return m, nil
		case "enter": // /whois — drill into the focused task (full-screen door; :tasks only)
			if m.Page == PageTasks {
				if _, ok := m.FocusedTask(); ok {
					m.DoorOpen = true
				} else {
					m.Status = "inspect: no task rows"
				}
			} else if m.Page == PageSessions {
				if s, ok := m.FocusedSession(); ok {
					m.SessionDoorOpen = true
					m.SessionDetail = grammar.SessionDetail{}
					m.SessionDetailDark, m.SessionDetailError = false, ""
					return m, func() tea.Msg { return SessionDetailRequest{Role: s.Role} }
				} else {
					m.Status = "inspect: no session rows"
				}
			} else if m.Page == PageIntake {
				m.IntakeDoorOpen = true
			} else if m.Page == PageIntent {
				if target, ok := m.selectedIntentArg(); ok {
					subject := m.selectedIntentSubject()
					m.IntentTarget, m.IntentSubject = target.Label, subject
					m.Status = m.intentStatusFor(target.Label, subject)
				} else {
					m.Status = "intent target: no registered targets"
				}
			} else if m.Page == PageDynamics {
				m = m.openEpistemicsForDynamicsFocus()
			} else {
				m.Status = "inspect: :tasks only"
			}
			return m, nil
		case "r":
			if m.Page == PageSessions {
				if s, ok := m.FocusedSession(); ok {
					m.Status = m.sessionResumeStatus(s)
				} else {
					m.Status = "resume: no session rows"
				}
				return m, nil
			}
		case "j", "down": // move the current page's focus cursor (tasks→rail tracks it; events→row)
			if m.Page == PageIntent {
				return m.intentFocusTo(m.IntentFocus + 1), nil
			}
			if m.Page == PageCaps {
				return m.capabilityFocusTo(m.CFocus + 1), nil
			}
			if m.Page == PageDynamics {
				return m.dynamicsFocusTo(m.DynFocus + 1), nil
			}
			if m.Page == PageCommands {
				return m.commandFocusTo(m.CommandFocus + 1), nil
			}
			if m.Page == PageWindows {
				return m.windowFocusTo(m.WindowFocus + 1), nil
			}
			if m.Page == PageSurfaces {
				return m.surfaceFocusTo(m.SurfaceFocus + 1), nil
			}
			if m.Page == PageDomains {
				return m.domainFocusTo(m.DomainFocus + 1), nil
			}
			if m.Page == PageLifecycles {
				return m.lifecycleFocusTo(m.LifecycleFocus + 1), nil
			}
			if m.Page == PageEpistemics {
				return m.epistemicFocusTo(m.EpiFocus + 1), nil
			}
			if m.Page == PageIntake {
				m = m.intakeFocusTo(m.IFocus + 1)
				if n := len(m.visibleIntakeRows()); n > 0 {
					m.Status = fmt.Sprintf("intake bucket %d/%d", m.IFocus+1, n)
				} else {
					m.Status = "intake bucket: no buckets in filter"
				}
				return m, nil
			}
			if m.isReferencePage() {
				return m.scrollReference(1), nil
			}
			return m.focusTo(m.curFocus() + 1), nil
		case "k", "up":
			if m.Page == PageIntent {
				return m.intentFocusTo(m.IntentFocus - 1), nil
			}
			if m.Page == PageCaps {
				return m.capabilityFocusTo(m.CFocus - 1), nil
			}
			if m.Page == PageDynamics {
				return m.dynamicsFocusTo(m.DynFocus - 1), nil
			}
			if m.Page == PageCommands {
				return m.commandFocusTo(m.CommandFocus - 1), nil
			}
			if m.Page == PageWindows {
				return m.windowFocusTo(m.WindowFocus - 1), nil
			}
			if m.Page == PageSurfaces {
				return m.surfaceFocusTo(m.SurfaceFocus - 1), nil
			}
			if m.Page == PageDomains {
				return m.domainFocusTo(m.DomainFocus - 1), nil
			}
			if m.Page == PageLifecycles {
				return m.lifecycleFocusTo(m.LifecycleFocus - 1), nil
			}
			if m.Page == PageEpistemics {
				return m.epistemicFocusTo(m.EpiFocus - 1), nil
			}
			if m.Page == PageIntake {
				m = m.intakeFocusTo(m.IFocus - 1)
				if n := len(m.visibleIntakeRows()); n > 0 {
					m.Status = fmt.Sprintf("intake bucket %d/%d", m.IFocus+1, n)
				} else {
					m.Status = "intake bucket: no buckets in filter"
				}
				return m, nil
			}
			if m.isReferencePage() {
				return m.scrollReference(-1), nil
			}
			return m.focusTo(m.curFocus() - 1), nil
		case "g": // top
			if m.Page == PageIntent {
				return m.intentFocusTo(0), nil
			}
			if m.Page == PageCaps {
				return m.capabilityFocusTo(0), nil
			}
			if m.Page == PageDynamics {
				return m.dynamicsFocusTo(0), nil
			}
			if m.Page == PageCommands {
				return m.commandFocusTo(0), nil
			}
			if m.Page == PageWindows {
				return m.windowFocusTo(0), nil
			}
			if m.Page == PageSurfaces {
				return m.surfaceFocusTo(0), nil
			}
			if m.Page == PageDomains {
				return m.domainFocusTo(0), nil
			}
			if m.Page == PageLifecycles {
				return m.lifecycleFocusTo(0), nil
			}
			if m.Page == PageEpistemics {
				return m.epistemicFocusTo(0), nil
			}
			if m.Page == PageIntake {
				m = m.intakeFocusTo(0)
				m.RefScroll = 0
				m.Status = "intake bucket: top"
				return m, nil
			}
			if m.isReferencePage() {
				m.RefScroll = 0
				m.Status = "scroll: top"
				return m, nil
			}
			return m.focusTo(0), nil
		case "G": // bottom
			if m.Page == PageIntent {
				return m.intentFocusTo(len(lookupIntentArgs()) - 1), nil
			}
			if m.Page == PageCaps {
				return m.capabilityFocusTo(len(m.capabilityDisplayRows()) - 1), nil
			}
			if m.Page == PageDynamics {
				return m.dynamicsFocusTo(len(m.dynamicsFocusRows()) - 1), nil
			}
			if m.Page == PageCommands {
				return m.commandFocusTo(len(verbs) - 1), nil
			}
			if m.Page == PageWindows {
				return m.windowFocusTo(len(registeredWindows()) - 1), nil
			}
			if m.Page == PageSurfaces {
				return m.surfaceFocusTo(len(registeredSurfaces()) - 1), nil
			}
			if m.Page == PageDomains {
				return m.domainFocusTo(m.domainRowCount() - 1), nil
			}
			if m.Page == PageLifecycles {
				return m.lifecycleFocusTo(m.lifecycleRowCount() - 1), nil
			}
			if m.Page == PageEpistemics {
				return m.epistemicFocusTo(len(m.epistemicRows()) - 1), nil
			}
			if m.Page == PageIntake {
				m = m.intakeFocusTo(len(m.visibleIntakeRows()) - 1)
				m.Status = "intake bucket: bottom"
				return m, nil
			}
			if m.isReferencePage() {
				m.RefScroll = m.referenceScrollMax()
				m.Status = "scroll: bottom"
				return m, nil
			}
			return m.focusTo(m.pageRows() - 1), nil
		}
	}
	return m, nil
}

func keyName(v tea.KeyMsg) string {
	if v.Type == tea.KeyRunes && len(v.Runes) == 1 {
		return string(v.Runes[0])
	}
	return v.String()
}

func (m Model) updateSplitTargetCursor(rel SplitPairDef, key string) (Model, bool) {
	delta := 0
	switch key {
	case "n":
		delta = 1
	case "p":
		delta = -1
	default:
		return m, false
	}

	switch rel.TargetCursor {
	case splitTargetIntake:
		m = m.intakeFocusTo(m.IFocus + delta)
		if n := len(m.visibleIntakeRows()); n > 0 {
			m.Status = fmt.Sprintf("intake bucket %d/%d", m.IFocus+1, n)
		} else {
			m.Status = "intake bucket: no buckets in filter"
		}
		return m, true
	case splitTargetMapElement:
		return m.dynamicsFocusTo(m.DynFocus + delta), true
	case splitTargetEpistemic:
		return m.epistemicFocusTo(m.EpiFocus + delta), true
	default:
		return m, false
	}
}

func (m Model) updateSplitSource(v tea.KeyMsg) (Model, tea.Cmd, bool) {
	rel := m.splitRelation()
	key := keyName(v)
	if m, ok := m.updateSplitTargetCursor(rel, key); ok {
		return m, nil, true
	}
	if m.Page == PageTasks && splitTaskHiddenControl(key) {
		m.Status = "split tasks: source pane owns [j/k]/[Enter]/[y]; use [|] unsplit or :tasks for task filters"
		return m, nil, true
	}
	switch key {
	case "j", "down":
		m = m.sessionFocusTo(m.SFocus + 1)
		m.Status = m.splitSourceStatus()
		return m, nil, true
	case "k", "up":
		m = m.sessionFocusTo(m.SFocus - 1)
		m.Status = m.splitSourceStatus()
		return m, nil, true
	case "g":
		m = m.sessionFocusTo(0)
		m.Status = m.splitSourceStatus()
		return m, nil, true
	case "G":
		m = m.sessionFocusTo(len(m.Sessions) - 1)
		m.Status = m.splitSourceStatus()
		return m, nil, true
	case "J":
		if rel.TargetScrollable {
			m = m.scrollReference(1)
			m.Status = strings.Replace(m.Status, "scroll", "context scroll", 1)
			return m, nil, true
		}
	case "K":
		if rel.TargetScrollable {
			m = m.scrollReference(-1)
			m.Status = strings.Replace(m.Status, "scroll", "context scroll", 1)
			return m, nil, true
		}
	case "y":
		if !rel.SourceOwns("yank") {
			return m, nil, false
		}
		if _, ok := m.FocusedSession(); ok {
			return m.withYankMode(), nil, true
		}
		m.Status = "yank: no session rows"
		return m, nil, true
	case "enter":
		if !rel.SourceOwns("detail") {
			return m, nil, false
		}
		if s, ok := m.FocusedSession(); ok {
			m.SessionDoorOpen = true
			m.SessionDetail = grammar.SessionDetail{}
			m.SessionDetailDark, m.SessionDetailError = false, ""
			return m, func() tea.Msg { return SessionDetailRequest{Role: s.Role} }, true
		}
		m.Status = "inspect: no session rows"
		return m, nil, true
	case "r":
		if !rel.SourceOwns("resume") {
			return m, nil, false
		}
		if s, ok := m.FocusedSession(); ok {
			m.Status = m.sessionResumeStatus(s)
		} else {
			m.Status = "resume: no session rows"
		}
		return m, nil, true
	}
	return m, nil, false
}

func splitTaskHiddenControl(key string) bool {
	switch key {
	case "/", "f", "tab", "V":
		return true
	}
	return false
}

func (m Model) splitSourceStatus() string {
	rel := m.splitRelation()
	role := "none"
	if s, ok := m.FocusedSession(); ok {
		role = sessionFieldValueForAir(s, "role", m.AIR)
		if strings.TrimSpace(role) == "" {
			role = "·"
		}
	}
	if rel.Reactive() {
		return fmt.Sprintf("split source %s -> %s", role, rel.Target)
	}
	return fmt.Sprintf("split lane anchor %s + %s", role, rel.Target)
}

func (m Model) updateGlobal(v tea.KeyMsg) (Model, tea.Cmd, bool) {
	key := keyName(v)
	switch key {
	case ":": // enter the command line (the command-as-effect surface)
		m.Mode, m.Input, m.Status, m.CompIdx = ModeCommand, "", "", 0
		return m, nil, true
	case "q", "ctrl+c":
		return m, tea.Quit, true
	case "a": // toggle the AIR lens
		m.AIR = !m.AIR
		state := "AIR off"
		if m.AIR {
			state = "AIR on"
		}
		m.Status = state
		nm, cmd := m.flash(state)
		return nm, cmd, true
	case "|":
		m.SplitContext = !m.SplitContext
		state := "split context off"
		if m.SplitContext {
			if m.splitContextActive() {
				state = "split context on"
			} else {
				state = fmt.Sprintf("split context queued: need %d columns", splitContextMinWidth)
			}
		}
		m.Status = state
		nm, cmd := m.flash(state)
		return nm, cmd, true
	case ",":
		if m.Page == PageDynamics {
			return m.cycleDynamicsScale(-1), nil, true
		}
	case ".":
		if m.Page == PageDynamics {
			return m.cycleDynamicsScale(1), nil, true
		}
	case "[":
		return m.cycleWindow(-1), nil, true
	case "]":
		return m.cycleWindow(1), nil, true
	case "left":
		if m.Sel.Rank == RankField {
			return m, nil, false
		}
		return m.cycleWindow(-1), nil, true
	case "right":
		if m.Sel.Rank == RankField {
			return m, nil, false
		}
		return m.cycleWindow(1), nil, true
	default:
		if w, ok := windowForKey(key); ok {
			if w.Page == PageEpistemics && m.Page == PageDynamics {
				return m.openEpistemicsForDynamicsFocus(), nil, true
			}
			if w.Page == PageEpistemics && m.Page == PageIntake {
				return m.openEpistemicsForIntakeFocus(), nil, true
			}
			if w.Page == PageIntent {
				m.IntentSubject = m.selectedIntentSubject()
			}
			m = m.switchPage(w.Page)
			m.Status = ":" + w.ID
			return m, nil, true
		}
	}
	return m, nil, false
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

func sessionFieldValue(s grammar.Session, field string) string {
	switch field {
	case "role":
		return s.Role
	case "session":
		return s.Session
	case "platform":
		return s.Platform
	case "state":
		return s.State
	case "readiness":
		return s.Readiness
	case "blocker":
		return s.Blocker
	case "attention":
		return fmt.Sprintf("%.2f", s.Attention)
	case "alive":
		return fmt.Sprintf("%t", s.Alive)
	case "idle":
		return fmt.Sprintf("%t", s.Idle)
	case "stalled":
		return fmt.Sprintf("%t", s.Stalled)
	case "claimed_task":
		return s.ClaimedTask
	case "route_id":
		return s.RouteID
	case "mode":
		return s.RouteMode
	case "profile":
		return s.RouteProfile
	case "route_binding_state":
		return s.RouteBindingState
	case "route_evidence_ref":
		return s.RouteEvidenceRef
	case "output_age_s":
		return fmt.Sprintf("%.1f", s.OutputAgeS)
	case "relay_age_s":
		return fmt.Sprintf("%.1f", s.RelayAgeS)
	}
	return ""
}

func (m Model) sessionResumeStatus(s grammar.Session) string {
	ref := sessionFieldValueForAir(s, "session", m.AIR)
	if strings.TrimSpace(ref) == "" || ref == "▒▒▒" {
		ref = sessionFieldValueForAir(s, "role", m.AIR)
	}
	return "resume-intent: would emit session.resume(" + ref + ") via the governed COMMAND surface — NOT wired (no transcript/PTY/stdin bridge)"
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
		m.Mode, m.Filter, m.Focus, m.CompIdx = ModeNormal, "", 0, 0 // clear the filter
		m.Sel.Members = nil
	case tea.KeyTab, tea.KeyDown: // navigate the candidate ids (same engine as the command line)
		if c := m.completions(); len(c) > 0 {
			m.CompIdx = (m.CompIdx + 1) % len(c)
		}
	case tea.KeyShiftTab, tea.KeyUp:
		if c := m.completions(); len(c) > 0 {
			m.CompIdx = (m.CompIdx - 1 + len(c)) % len(c)
		}
	case tea.KeyRight: // fill the filter with the highlighted id (fish-style accept-into-line)
		if c, ok := m.curCandidate(); ok {
			m.Filter, m.Focus, m.CompIdx = c.Value, 0, 0
			m.Sel.Members = nil
		}
	case tea.KeyBackspace:
		if n := len(m.Filter); n > 0 {
			m.Filter = m.Filter[:n-1]
		}
		m.Focus, m.CompIdx = 0, 0
		m.Sel.Members = nil
	case tea.KeySpace:
		m.Filter, m.CompIdx = m.Filter+" ", 0
		m.Sel.Members = nil
	case tea.KeyRunes:
		m.Filter, m.Focus, m.CompIdx = m.Filter+string(v.Runes), 0, 0
		m.Sel.Members = nil
	}
	return m, nil
}

// updateHint: type a row's label to teleport the cursor there; Esc cancels. No other state change.
func (m Model) updateHint(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	if keyName(v) == "esc" {
		m.Mode = ModeNormal
		return m, nil
	}
	if v.Type == tea.KeyRunes && len(v.Runes) == 1 {
		r := v.Runes[0]
		if cf, ok := critFromHint[r]; ok { // a COUNT label → filter the list to that criticality class
			m.CritFilter, m.Focus, m.Mode = cf, 0, ModeNormal
			m.Sel.Members = nil
			m.Status = "filtered to '" + cf + "' tasks · [Esc] clear"
			return m, nil
		}
		if r >= '1' && r <= '9' { // an ACT-strip item → jump the cursor to that blocker
			bi := m.blockedIndices()
			if d := int(r - '1'); d < len(bi) {
				id := m.Tasks[bi[d]].TaskID
				airID := taskFieldValueForAir(m.Tasks[bi[d]], "task_id", m.AIR)
				m.Filter, m.CritFilter = "", "" // clear filters so the blocker is reachable
				m.Sel.Members = nil
				for j, t := range m.visibleTasks() {
					if t.TaskID == id {
						m.Focus = j
						break
					}
				}
				m.Mode = ModeNormal
				m.Status = "jumped to blocker " + airID
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
	switch keyName(v) {
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
		m.Ring = pushRing(m.Ring, taskRingEntry(t, f, "tasks"))
		m.Input, m.Mode, m.Sel.Rank = taskFieldValueForAir(t, f, m.AIR), ModeCommand, RankRow
		m.Status = fmt.Sprintf("yanked %s → command line  (ring %d)", f, len(m.Ring))
		return m.flash(fmt.Sprintf("✓ yanked %s → ring %d", f, len(m.Ring)))
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
	switch keyName(v) {
	case "esc", "enter", "q":
		// just close
	case "a":
		if !m.doorVerbLegal("a") {
			m.DoorOpen, m.Status = true, "arm unavailable from this task state"
			return m, nil
		}
		m.Status = "arm: would emit sdlc.authorization_flip(release_authorized=true) via the governed COMMAND surface — NOT wired (cockpit never mints authority)"
	case "r":
		if !m.doorVerbLegal("r") {
			m.DoorOpen, m.Status = true, "rework unavailable from this task state"
			return m, nil
		}
		m.Status = "rework: would emit sdlc.stage_transition(→rework) via the governed COMMAND surface — NOT wired"
	case "f":
		if !m.doorVerbLegal("f") {
			m.DoorOpen, m.Status = true, "refute unavailable from this task state"
			return m, nil
		}
		m.Status = "refute: would record review.fail via the governed COMMAND surface — NOT wired"
	case "c":
		if !m.doorVerbLegal("c") {
			m.DoorOpen, m.Status = true, "close unavailable from this task state"
			return m, nil
		}
		m.Status = "close: would emit task.closed via the governed COMMAND surface — NOT wired"
	default:
		if nm, cmd, handled := m.updateGlobal(v); handled {
			return nm, cmd // pass-through: window cycle/jump (switchPage closes the door)
		}
		m.DoorOpen = true // non-global key — stay open (inert; nav keys no longer swallowed)
	}
	return m, nil
}

func (m Model) updateSessionDoor(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.SessionDoorOpen = false
	switch keyName(v) {
	case "esc", "enter", "q":
		// just close
	case "r":
		if s, ok := m.FocusedSession(); ok {
			m.Status = m.sessionResumeStatus(s)
		} else {
			m.Status = "resume: no session rows"
		}
	default:
		if nm, cmd, handled := m.updateGlobal(v); handled {
			return nm, cmd // pass-through: window cycle/jump (switchPage closes the door)
		}
		m.SessionDoorOpen = true // non-global key — stay open (inert; nav no longer swallowed)
	}
	return m, nil
}

func (m Model) updateIntakeDoor(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.IntakeDoorOpen = false
	switch keyName(v) {
	case "esc", "enter", "q":
		// just close
	default:
		if nm, cmd, handled := m.updateGlobal(v); handled {
			return nm, cmd // pass-through: window cycle/jump (switchPage closes the door)
		}
		m.IntakeDoorOpen = true // non-global key — stay open (inert; nav no longer swallowed)
	}
	return m, nil
}

// updateLastlogDoor: keys while the /lastlog scrollback door is open. Esc/Enter/q close;
// PgUp pages backward (FetchEventsBefore via LastlogPageRequest); PgDn returns to the live
// retained window. Unknown keys are inert (no modal swallow — the door stays open, the key
// does nothing), so the page-jump modal-trap that affects the other doors does NOT apply here.
func (m Model) updateLastlogDoor(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyName(v) {
	case "esc", "enter", "q":
		m.LastlogDoorOpen = false
		m.LastlogOlder = nil // close drops the backward-paged view (ring stays)
		m.LastlogPaging = false
		return m, nil
	case "pgup":
		if m.LastlogPaging {
			return m, nil // a page fetch is already in flight
		}
		before := m.EventScrollback.OldestTS()
		if len(m.LastlogOlder) > 0 {
			before = m.LastlogOlder[0].TS // page further back from the oldest shown
		}
		if before == "" {
			return m, nil // nothing to page back from
		}
		m.LastlogPaging = true
		cursor := before
		return m, func() tea.Msg { return LastlogPageRequest{Before: cursor} }
	case "pgdn":
		m.LastlogOlder = nil // return to the live retained window
		return m, nil
	default:
		if nm, cmd, handled := m.updateGlobal(v); handled {
			return nm, cmd // pass-through: window cycle/jump (switchPage closes the door)
		}
		return m, nil // non-global key — inert (door stays open; nav keys pass through above)
	}
}

func (m Model) doorVerbLegal(key string) bool {
	t, ok := m.FocusedTask()
	if !ok {
		return false
	}
	stage := doorStageIndex(t.Stage)
	pred := strings.ToLower(strings.TrimSpace(t.PredictedStage))
	switch key {
	case "a":
		return stage >= 7 && pred == "hold"
	case "r":
		return pred == "hold"
	case "f":
		return stage >= 5
	case "c":
		return pred == "ship"
	}
	return false
}

func doorStageIndex(stage string) int {
	s := strings.TrimSpace(shortStage2(stage))
	if len(s) < 2 || s[0] != 'S' {
		return -1
	}
	i := 1
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 1 {
		return -1
	}
	n, err := strconv.Atoi(s[1:i])
	if err != nil {
		return -1
	}
	return n
}

func (m Model) yankFieldByKey(key string) (Model, tea.Cmd) {
	for _, f := range m.yankFieldsForPage() {
		if f.key == key {
			m.Sel.Rank, m.Sel.Field = RankField, f.field
			break
		}
	}
	page := m.Page
	if m.splitContextActive() {
		page = PageSessions
	}
	if page == PageEvents { // events have their own field set (the pick row is page-aware)
		field, val, ok := m.yankEventField(key)
		if !ok {
			return m, nil
		}
		ev, _ := m.FocusedEvent()
		if m.AIR && ev.AIR[field] != "ok" {
			m.Status = "yank: " + field + " is redacted on-air — un-yankable"
			return m, nil
		}
		m.Ring = pushRing(m.Ring, eventRingEntry(ev, field, val))
		m.Input, m.Mode, m.Sel.Rank, m.Sel.Field = ringValue(m.Ring[0], m.AIR), ModeCommand, RankRow, ""
		m.Status = fmt.Sprintf("yanked %s → command line  (ring %d)", field, len(m.Ring))
		return m.flash(fmt.Sprintf("✓ yanked %s → ring %d", field, len(m.Ring)))
	}
	if page == PageSessions {
		field, val, ok := m.yankSessionField(key)
		if !ok {
			return m, nil
		}
		s, _ := m.FocusedSession()
		if m.AIR && s.AIR[field] != "ok" {
			m.Status = "yank: " + field + " is redacted on-air — un-yankable"
			return m, nil
		}
		m.Ring = pushRing(m.Ring, sessionRingEntry(s, field, val))
		m.Input, m.Mode, m.Sel.Rank, m.Sel.Field = ringValue(m.Ring[0], m.AIR), ModeCommand, RankRow, ""
		m.Status = fmt.Sprintf("yanked %s → command line  (ring %d)", field, len(m.Ring))
		return m.flash(fmt.Sprintf("✓ yanked %s → ring %d", field, len(m.Ring)))
	}
	field, _, ok := m.yankField(key)
	if !ok {
		return m, nil
	}
	t, _ := m.FocusedTask()
	if m.AIR && t.AIR[field] != "ok" {
		m.Status = "yank: " + field + " is redacted on-air — un-yankable"
		return m, nil
	}
	m.Ring = pushRing(m.Ring, taskRingEntry(t, field, "tasks"))
	m.Input, m.Mode, m.Sel.Rank, m.Sel.Field = taskFieldValueForAir(t, field, m.AIR), ModeCommand, RankRow, ""
	m.Status = fmt.Sprintf("yanked %s → command line  (ring %d)", field, len(m.Ring))
	return m.flash(fmt.Sprintf("✓ yanked %s → ring %d", field, len(m.Ring)))
}

// updateYank is a navigable selection mode, not a modal freeze-frame. j/k/g/G move the row cursor
// within the visible source (the session pane in split context); Tab/arrows move the field
// granularity; Enter/y grabs the selected field; direct field letters still work for speed.
// AIR-denied fields are unavailable but keep the operator in yank mode so another visible field can
// be selected.
func (m Model) updateYank(v tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyName(v) {
	case "esc":
		m.Mode, m.Sel.Rank, m.Sel.Field = ModeNormal, RankRow, ""
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	case ":", "1", "2", "3", "4", "5", "6", "7", "8", "9", "0", "Y", "R", "I", "C", "E", "?", "[", "]", "|":
		if nm, cmd, ok := m.updateGlobal(v); ok {
			return nm, cmd
		}
	case "j", "down":
		return m.yankFocusTo(m.yankCurFocus() + 1).withYankMode(), nil
	case "k", "up":
		return m.yankFocusTo(m.yankCurFocus() - 1).withYankMode(), nil
	case "g":
		return m.yankFocusTo(0).withYankMode(), nil
	case "G":
		return m.yankFocusTo(m.yankRows() - 1).withYankMode(), nil
	case "tab", "right":
		return m.moveYankField(1), nil
	case "shift+tab", "left":
		return m.moveYankField(-1), nil
	case "enter", "y":
		return m.yankCurrentField()
	}
	return m.yankFieldByKey(keyName(v))
}

// updateCommand: key handling while the command line is focused. Enter accepts the current
// completion context (run, descend, fill, or inject), Esc cancels, Backspace edits, Space/runes
// append; quit folds straight to tea.Quit.
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
