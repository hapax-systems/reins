package model

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

const railWidth = 36

// View composes the FOUR permanent zones to fill the whole terminal — the cure for the wasted-
// screenspace complaint (a single left column is gone). Layout, top→bottom:
//
//	Z0 title (1) · Z1 vital (2) · ─rule─ · Z2 (main │ rail) midH · ─rule─ · Z3 floor (2)
//
// Pure function of model + terminal size, so it hot-reloads. Degrades on narrow terminals
// (rail collapses) without panic.
func (m Model) View() string {
	w, h := m.Width, m.Height
	if w < 40 || h < 12 { // no WindowSizeMsg yet (e.g. --probe) -> a sane default frame
		w, h = 120, 40
	}
	railW := railWidth
	if w < 100 || m.Page == PageDynamics || m.Page == PageLegend || m.Page == PageHelp {
		railW = 0 // collapse the rail on narrow terminals + full-width reference pages
	}
	mainW := w
	if railW > 0 {
		mainW = w - railW - 1 // 1 col for the │ divider between main and rail
	}
	midH := h - 7 // title(1)+vital(2)+rule(1)+rule(1)+floor(2)
	if midH < 1 {
		midH = 1
	}

	rows := make([]string, 0, h)
	rows = append(rows, fitWidth(m.viewTitle(w), w))
	rows = append(rows, fitBlock(m.viewVital(w), w, 2)...)
	rows = append(rows, m.rule(mainW, railW, "┬"))
	main := fitBlock(m.bodyFor(mainW, midH), mainW, midH)
	if railW > 0 {
		rail := fitBlock(m.viewRail(railW), railW, midH)
		div := grammar.C("border", "│")
		for i := 0; i < midH; i++ {
			rows = append(rows, main[i]+div+rail[i])
		}
	} else {
		rows = append(rows, main...)
	}
	rows = append(rows, m.rule(mainW, railW, "┴"))
	rows = append(rows, fitBlock(m.viewFloor(w), w, 2)...)
	return strings.Join(rows, "\n")
}

func (m Model) rule(mainW, railW int, junction string) string {
	if railW == 0 {
		return grammar.C("border", strings.Repeat("─", mainW))
	}
	return grammar.C("border", strings.Repeat("─", mainW)+junction+strings.Repeat("─", railW))
}

// pageMeta: the active page's name, item count, and dark flag (one source for title/vital/body).
func (m Model) pageMeta() (string, int, bool) {
	switch m.Page {
	case PageTasks:
		return "tasks", len(m.Tasks), m.TasksDark
	case PageDynamics:
		return "dynamics", len(m.Dynamics.AtResolution(m.DynScale).Nodes), m.DynamicsDark
	case PageHelp:
		return "help", 0, false
	case PageLegend:
		return "legend", 0, false
	}
	return "events", len(m.Events), m.EventsDark
}

// Z0 — title + a TAB BAR (the views are visible + the active one obvious + the key to reach each)
// + identity. A signifier (Norman): it advertises that pages exist and how to switch.
var pageTabs = []struct {
	key, name string
	page      int
}{
	{"1", "events", PageEvents}, {"2", "tasks", PageTasks}, {"3", "dynamics", PageDynamics},
	{"4", "help", PageHelp}, {"?", "legend", PageLegend},
}

func (m Model) viewTitle(w int) string {
	tabs := ""
	for _, p := range pageTabs {
		if p.page == m.Page {
			tabs += grammar.C("brt", "‹"+p.key+" "+p.name+"›") + " "
		} else {
			tabs += grammar.C("mut", p.key+" "+p.name) + " "
		}
	}
	left := grammar.C("brt", m.Title) + "   " + strings.TrimRight(tabs, " ")
	right := grammar.C("yel", "@hapax")
	gap := w - ansi.StringWidth(left) - ansi.StringWidth(right) - 2
	if gap < 1 {
		gap = 1
	}
	return " " + left + strings.Repeat(" ", gap) + right
}

// Z1 — vital strip: row1 = mode/page + criticality-split task counts + spine; row2 = the
// EXCEPTION-ONLY Act strip (structured-silence when calm; a red hotlist of blocked items when not).
func (m Model) viewVital(w int) string {
	_, _, dark := m.pageMeta()
	mode := grammar.C("grn", "LOCAL")
	if m.AIR {
		mode = grammar.C("fch", "AIR ▮")
	}
	spine := grammar.C("grn", "spine:live")
	if dark {
		spine = grammar.C("red", "spine:DARK")
	}
	ok, warn, maj, crit, blocked := 0, 0, 0, 0, []string{}
	for _, t := range m.Tasks {
		switch t.Criticality {
		case "crit":
			crit++
		case "major":
			maj++
		case "warn":
			warn++
		default:
			ok++
		}
		if t.PredictedStage == "hold" || t.Criticality == "crit" || t.Criticality == "major" {
			blocked = append(blocked, t.TaskID)
		}
	}
	dot := grammar.C("mut", " · ")
	counts := grammar.C("grn", fmt.Sprintf("%d ok", ok)) + dot + grammar.C("yel", fmt.Sprintf("%d warn", warn)) +
		dot + grammar.C("org", fmt.Sprintf("%d major", maj)) + dot + grammar.C("red", fmt.Sprintf("%d crit", crit))
	r1 := " " + mode + grammar.C("mut", "  │  tasks ") + grammar.C("brt", fmt.Sprintf("%d", len(m.Tasks))) +
		grammar.C("mut", " = ") + counts + grammar.C("mut", "  │  events ") + grammar.C("brt", fmt.Sprintf("%d", len(m.Events))) +
		grammar.C("mut", "  │  ") + spine

	var r2 string
	if len(blocked) == 0 {
		r2 = grammar.C("mut", " "+strings.Repeat("·", 24)+"  all clear — nothing blocked")
	} else {
		head := blocked
		if len(head) > 2 {
			head = head[:2]
		}
		r2 = grammar.C("red", fmt.Sprintf(" ‼ %d release-blocked", len(blocked))) +
			grammar.C("mut", " (at S7, not release-authorized) · ") +
			grammar.C("org", strings.Join(head, "  ")) + grammar.C("mut", "  · [2] tasks to inspect")
	}
	return r1 + "\n" + r2
}

// Z2a — the main pane body: the active page, rendered by the cell grammar.
func (m Model) bodyFor(w, h int) string {
	_, _, dark := m.pageMeta()
	var b strings.Builder
	switch m.Page {
	case PageTasks:
		return m.taskBody(w, h)
	case PageDynamics:
		if dark {
			b.WriteString(darkHint())
		} else {
			b.WriteString(grammar.DynamicsHeader(m.Dynamics, w)) // thesis + inline provenance key (situate)
			b.WriteString(grammar.RenderColumnRail(m.Dynamics, m.DynScale, m.AIR, w))
		}
	case PageHelp:
		b.WriteString(grammar.RenderHelp())
	case PageLegend:
		b.WriteString(grammar.RenderLegend())
	default:
		return m.eventsBody(w, h)
	}
	return b.String()
}

// contextLine: the one-line "what am I looking at" for the active page (Norman conceptual model).
func (m Model) contextLine() string {
	switch m.Page {
	case PageTasks:
		f := "—"
		if t, ok := m.FocusedTask(); ok {
			f = t.TaskID
		}
		return grammar.C("2nd", fmt.Sprintf(" task registry · %d tasks · [j/k] inspect → rail · focus: %s", len(m.Tasks), f))
	case PageEvents:
		return grammar.C("2nd", fmt.Sprintf(" live coord events · newest at bottom · %d shown", len(m.Events)))
	}
	return ""
}

// eventsBody: context line + header + the NEWEST events (tail-windowed), so the operator sees what
// just happened rather than the oldest rows.
func (m Model) eventsBody(w, h int) string {
	if m.EventsDark {
		return m.contextLine() + "\n" + darkHint()
	}
	visible := h - 2 // context + header
	if visible < 1 {
		visible = 1
	}
	start := 0
	if len(m.Events) > visible {
		start = len(m.Events) - visible // tail = newest
	}
	var b strings.Builder
	b.WriteString(m.contextLine() + "\n")
	b.WriteString(grammar.RenderEventHeader() + "\n")
	for _, ev := range m.Events[start:] {
		b.WriteString(grammar.RenderEventRow(ev, m.AIR) + "\n")
	}
	return b.String()
}

func darkHint() string {
	return grammar.C("mut", "(spine dark — no fabricated data; is the READ API up?  `make up`)\n")
}

// taskBody windows the registry to the visible height, keeping the focused row in view, with a
// 1-col focus gutter (▌ marks the focused row).
func (m Model) taskBody(w, h int) string {
	visible := h - 2 // context line + header
	if visible < 1 {
		visible = 1
	}
	off := m.scrollOffset(visible)
	var b strings.Builder
	b.WriteString(m.contextLine() + "\n")
	b.WriteString(" " + grammar.RenderTaskHeader() + "\n")
	for i := off; i < off+visible && i < len(m.Tasks); i++ {
		mark := " "
		if i == m.Focus {
			mark = grammar.C("brt", "▌")
		}
		b.WriteString(mark + grammar.RenderTaskRow(m.Tasks[i], m.AIR) + "\n")
	}
	return b.String()
}

func (m Model) scrollOffset(visible int) int {
	n := len(m.Tasks)
	if n <= visible || m.Focus < visible {
		return 0
	}
	off := m.Focus - visible + 1 // focus sits at the last visible row
	if mx := n - visible; off > mx {
		off = mx
	}
	if off < 0 {
		off = 0
	}
	return off
}

func shortStage2(s string) string {
	if i := strings.IndexByte(s, '_'); i >= 0 {
		return s[:i]
	}
	return s
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "·····"
	}
	return s
}

// splitAuth turns the comma-list of granted authorizations into short readable labels for the rail.
func splitAuth(noGo string) []string {
	if strings.TrimSpace(noGo) == "" {
		return []string{grammar.C("mut", "(none yet)")}
	}
	var out []string
	for _, a := range strings.Split(noGo, ",") {
		out = append(out, strings.TrimSuffix(strings.TrimSpace(a), "_authorized"))
	}
	return out
}

// yankMenu: the field pick-list shown in ModeYank — keys + labels, AIR-denied fields dimmed (a
// signifier that they are un-yankable on-air).
func yankMenu(t grammar.Task, air bool) string {
	fields := []struct{ key, label, field string }{
		{"i", "id", "task_id"}, {"s", "stage", "stage"}, {"o", "owner", "owner"},
		{"w", "was", "prior_stage"}, {"n", "next", "predicted_stage"},
		{"c", "crit", "criticality"}, {"a", "auth", "authority_case"},
	}
	out := ""
	for _, f := range fields {
		tok := "yel"
		if air && t.AIR[f.field] != "ok" {
			tok = "mut" // denied on-air → dimmed, un-yankable
		}
		out += grammar.C(tok, "["+f.key+"]"+f.label) + " "
	}
	return strings.TrimRight(out, " ")
}

// whichKey: the transient verb menu shown while the command line is focused — names when several
// match, the full gloss when one does. Verbs become recognizable, not memorized.
func whichKey(input string) string {
	mv := matchVerbs(input)
	switch {
	case len(mv) == 1:
		return grammar.C("mut", "   "+mv[0].name+" — "+mv[0].gloss)
	case len(mv) > 1:
		names := make([]string, len(mv))
		for i, v := range mv {
			names[i] = v.name
		}
		return grammar.C("mut", "   ‹"+strings.Join(names, " ")+"›")
	}
	return ""
}

// Z2b — context rail: the focused registry item unfolded into its seven dimensions, plus the
// relationship web and mini :dynamics (structured-silence until their data sources land).
func (m Model) viewRail(w int) string {
	t, ok := m.FocusedTask()
	if !ok {
		return grammar.C("mut", " (no selection — [j/k] to move)")
	}
	id := t.TaskID
	if r := []rune(id); len(r) > w-3 {
		id = string(r[:w-3])
	}
	ctok := grammar.SeverityToken(t.Criticality)
	ntok := "grn"
	if t.PredictedStage == "hold" {
		ntok = "red"
	}
	line := func(label, val, tok string) string {
		return " " + grammar.C("mut", fmt.Sprintf("%-6s", label)) + grammar.C(tok, val)
	}
	rule := grammar.C("border", " "+strings.Repeat("─", w-2))
	var b strings.Builder
	b.WriteString(" " + grammar.C("brt", "◆ "+id) + "\n")
	b.WriteString(rule + "\n")
	b.WriteString(line("state", orDash(shortStage2(t.Stage)), ctok) + "\n")
	b.WriteString(line("was", orDash(shortStage2(t.PriorStage)), "mut") + "\n")
	b.WriteString(line("next", orDash(t.PredictedStage), ntok) + "\n")
	b.WriteString(line("crit", orDash(t.Criticality), ctok) + "\n")
	b.WriteString(line("who", orDash(t.Owner), grammar.LaneToken(t.Owner)) + "\n")
	b.WriteString(line("fresh", fmt.Sprintf("%.2f", t.Freshness), "pri") + "\n")
	b.WriteString(line("rel", fmt.Sprintf("●%d", t.RelCount), "blu") + "\n")
	b.WriteString(rule + "\n")
	// authorizations granted so far (situates WHY a task is or isn't release-ready)
	b.WriteString(" " + grammar.C("mut", "granted") + "\n")
	if strings.TrimSpace(t.NoGo) == "" {
		b.WriteString("  " + grammar.C("mut", "(none granted yet)") + "\n")
	} else {
		for _, g := range splitAuth(t.NoGo) {
			b.WriteString("  " + grammar.C("grn", "✓ ") + grammar.C("mut", g) + "\n")
		}
	}
	if t.AuthorityCase != "" {
		b.WriteString(" " + grammar.C("mut", "case ") + grammar.C("2nd", t.AuthorityCase) + "\n")
	}
	b.WriteString(rule + "\n")
	b.WriteString(grammar.C("2nd", " relationships") + "\n")
	b.WriteString(grammar.C("mut", " (no task-edge source yet)") + "\n")
	b.WriteString(rule + "\n")
	b.WriteString(grammar.C("2nd", " :dynamics neighborhood") + "\n")
	b.WriteString(grammar.C("mut", " (INC-6 mini-map)") + "\n")
	return b.String()
}

// Z3 — command/status floor: row1 = bezel + keys + lens; row2 = the command-as-effect line.
func (m Model) viewFloor(w int) string {
	lens := grammar.C("pri", "LOCAL")
	if m.AIR {
		lens = grammar.C("fch", "AIR ░allowlist░")
	}
	focus := grammar.C("mut", "—")
	if t, ok := m.FocusedTask(); ok {
		fid := t.TaskID
		if r := []rune(fid); len(r) > 24 {
			fid = string(r[:24])
		}
		focus = grammar.C("brt", fid)
	}
	r1 := " " + grammar.C("mut", "focus ") + focus + grammar.C("mut", " │ ") +
		grammar.C("yel", "[j/k]") + "move " + grammar.C("yel", "[y]") + "ank " +
		grammar.C("yel", "[:]") + "cmd " + grammar.C("yel", "[?]") + "legend " +
		grammar.C("yel", "[a]") + "AIR " + grammar.C("yel", "[q]") + "quit │ " + lens
	var r2 string
	switch {
	case m.Mode == ModeYank:
		t, _ := m.FocusedTask()
		r2 = grammar.C("brt", " yank ") + grammar.C("mut", "field ← ") + yankMenu(t, m.AIR) +
			grammar.C("mut", " [Esc] cancel")
	case m.Mode == ModeCommand:
		r2 = grammar.C("blu", ":") + " " + m.Input + "█" + whichKey(m.Input)
	case m.Status != "":
		r2 = " " + grammar.C("mut", m.Status)
	default:
		r2 = grammar.C("blu", ":") + grammar.C("mut", " type a command — [:] to focus, [Tab] completes")
	}
	return r1 + "\n" + r2
}

// fitWidth clips/pads a (possibly ANSI-colored) line to exactly w visible columns.
func fitWidth(s string, w int) string {
	if vw := ansi.StringWidth(s); vw > w {
		return ansi.Truncate(s, w, "")
	} else {
		return s + strings.Repeat(" ", w-vw)
	}
}

// fitBlock forces content into exactly h lines, each exactly w visible columns (clip/pad).
func fitBlock(s string, w, h int) []string {
	lines := strings.Split(s, "\n")
	out := make([]string, h)
	for i := 0; i < h; i++ {
		ln := ""
		if i < len(lines) {
			ln = lines[i]
		}
		out[i] = fitWidth(ln, w)
	}
	return out
}
