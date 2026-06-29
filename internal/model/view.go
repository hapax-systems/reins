package model

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/doi"
	"github.com/hapax-systems/reins/internal/files"
	"github.com/hapax-systems/reins/internal/grammar"
	"github.com/hapax-systems/reins/internal/graph"
	"github.com/hapax-systems/reins/internal/imgpreview"
	"github.com/hapax-systems/reins/internal/layout"
	"github.com/hapax-systems/reins/internal/relate"
)

// focusBar renders text as an unmistakable full-width SELECTION bar (bright on the focus background),
// stripping the row's per-cell colors so the highlight is uniform and obvious. This is THE visible
// cursor — the operator must always see what is selected.
func focusBar(text string, w int) string {
	plain := ansi.Strip(text)
	if vw := ansi.StringWidth(plain); vw < w {
		plain += strings.Repeat(" ", w-vw)
	} else if vw > w {
		plain = ansi.Truncate(plain, w, "")
	}
	return lipgloss.NewStyle().
		Background(lipgloss.Color(grammar.Hex("focus"))).
		Foreground(lipgloss.Color(grammar.Hex("brt"))).
		Bold(true).Render(plain)
}

const railWidth = 36
const catalogDenseBreakpoint = 180

// View composes the FOUR permanent zones to fill the whole terminal — the cure for the wasted-
// screenspace complaint (a single left column is gone). Layout, top→bottom:
//
//	Z0 title (1) · Z1 vital (2) · ─rule─ · Z2 (main │ rail) midH · ─rule─ · Z3 floor (2)
//
// Pure function of model + terminal size, so it hot-reloads. Degrades on narrow terminals
// (rail collapses) without panic.
func (m Model) View() string {
	w, h := m.Width, m.Height
	if w <= 0 || h <= 0 { // no WindowSizeMsg yet (e.g. --probe) -> a sane default frame
		w, h = 120, 40
	}
	if h < 8 {
		return m.compactView(w, h)
	}
	// the /whois door is a full-screen present-at-hand drill-in (replaces the body, clean return).
	if m.DoorOpen {
		if t, ok := m.FocusedTask(); ok {
			return grammar.RenderWhoisDoor(t, m.AIR, w, h)
		}
	}
	if m.SessionDoorOpen {
		if s, ok := m.FocusedSession(); ok {
			hasDetail := m.SessionDetail.Role == s.Role
			return grammar.RenderSessionDoor(s, m.SessionDetail, hasDetail, m.SessionDetailDark, m.SessionDetailError, m.AIR, w, h)
		}
	}
	if m.IntakeDoorOpen {
		return m.renderIntakeDoor(w, h)
	}
	if m.LastlogDoorOpen {
		return m.renderLastlogDoor(w, h)
	}
	railW := railWidth
	if w < 100 || m.isReferencePage() || m.Page == PageCoordinator || ((m.Page == PageEvents || m.Page == PageTasks || m.Page == PageSessions) && w >= 160) {
		railW = 0 // reference pages, the self-composing coordinator, and wide row pages manage their own context panes
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

func (m Model) compactView(w, h int) string {
	if h <= 0 {
		return ""
	}
	floor := fitBlock(m.viewFloor(w), w, 2)
	if h == 1 {
		if m.Mode == ModeCommand || m.Mode == ModeFilter {
			return floor[0]
		}
		return floor[1]
	}
	rows := make([]string, 0, h)
	if h >= 3 {
		rows = append(rows, fitWidth(m.viewTitle(w), w))
	}
	bodyH := h - len(rows) - 2
	if bodyH > 0 {
		if bodyH <= 2 {
			rows = append(rows, fitBlock(m.viewVital(w), w, bodyH)...)
		} else {
			rows = append(rows, fitBlock(m.viewVital(w), w, 2)...)
			remaining := bodyH - 2
			if remaining > 0 {
				rows = append(rows, fitBlock(m.bodyFor(w, remaining), w, remaining)...)
			}
		}
	}
	rows = append(rows, floor...)
	if len(rows) > h {
		rows = rows[len(rows)-h:]
	}
	for len(rows) < h {
		rows = append([]string{fitWidth("", w)}, rows...)
	}
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
	case PageSessions:
		return "sessions", len(m.Sessions), m.SessionsDark
	case PageTraces:
		return "traces", len(m.Traces), m.TracesDark
	case PageSessionTurns:
		return "session-turns", len(m.TurnLadder), false
	case PageYard:
		return "yard", len(m.Tasks), m.TasksDark && m.SessionsDark && m.EventsDark
	case PageReadiness:
		if len(m.Gates.Rows) > 0 || len(m.Gates.Sources) > 0 {
			return "readiness", len(m.Gates.Rows), m.GatesDark
		}
		visibleBlocked, _ := m.yardBlockedIndices() // AIR: don't leak the count of tasks whose denied criticality/predicted_stage made them "blocked"
		return "readiness", len(visibleBlocked), m.TasksDark && m.SessionsDark
	case PageIntake:
		return "intake", len(m.Intake.Rows), m.IntakeDark
	case PageCaps:
		if len(m.Capabilities.Rows) > 0 || len(m.Capabilities.Sources) > 0 {
			return "capabilities", len(m.Capabilities.Rows), m.CapabilitiesDark
		}
		return "capabilities", len(m.Sessions), m.SessionsDark
	case PageDynamics:
		return "dynamics", len(m.Dynamics.AtResolution(m.DynScale).Nodes), m.DynamicsDark
	case PageLoops:
		return "loops", len(m.loopRows()), m.DynamicsDark
	case PageAxes:
		return "axes", len(grammar.Axes()), false
	case PageIdentity:
		return "identity", len(m.identityRoster()), false
	case PageRelational:
		return "relational", len(m.consentFacets()), false
	case PageEpistemics:
		return "epistemics", len(m.epistemicRows()), m.EpistemicsDark && m.DynamicsDark && m.IntakeDark && m.DomainsDark && m.CapabilitiesDark
	case PageHelp:
		return "help", 0, false
	case PageLegend:
		return "legend", 0, false
	case PageCommands:
		return "commands", len(verbs), false
	case PageWindows:
		return "windows", len(windowRegistry), false
	case PageIntent:
		return "intent", len(lookupIntentArgs()), false
	case PageSurfaces:
		return "surfaces", len(surfaceRegistry), false
	case PageDomains:
		if len(m.Domains.Rows) > 0 || len(m.Domains.Sources) > 0 {
			return "domains", len(m.Domains.Rows), m.DomainsDark
		}
		return "domains", len(domainRegistry), false
	case PageLifecycles:
		if len(m.Domains.Lifecycles) > 0 || len(m.Domains.LifecycleSources) > 0 {
			return "lifecycles", len(m.Domains.Lifecycles), m.DomainsDark
		}
		return "lifecycles", len(registeredLifecycleFallbacks()), false
	}
	return "events", len(m.Events), m.EventsDark
}

func (m Model) viewTitle(w int) string {
	windows := registeredWindows()
	tabs := make([]string, 0, len(windows))
	active := 0
	for i, p := range windows {
		label, tok := m.windowSignal(p.Page)
		item := p.Key + "." + p.Short
		if label != "" {
			item += ":" + label
		}
		if p.Page == m.Page {
			active = i
			tabs = append(tabs, grammar.C("brt", "‹"+item+"›"))
		} else {
			if m.windowActive(p.Page) && tok == "mut" {
				tok = "pri" // a calm window changed since visit -> brighten (AUTO-SURFACE)
			}
			tabs = append(tabs, grammar.C(tok, item))
		}
	}
	left := grammar.C("brt", m.Title) + grammar.C("mut", " │ win ")
	mid := strings.Join(tabs, grammar.C("mut", " · "))
	right := grammar.C("yel", "@hapax")
	maxMid := w - ansi.StringWidth(left) - ansi.StringWidth(right) - 3
	if maxMid < 0 {
		maxMid = 0
	}
	if ansi.StringWidth(mid) > maxMid {
		mid = compactTitleTabs(tabs, active, maxMid)
	}
	gap := w - ansi.StringWidth(left) - ansi.StringWidth(mid) - ansi.StringWidth(right) - 2
	if gap < 1 {
		gap = 1
	}
	return " " + left + mid + strings.Repeat(" ", gap) + right
}

func compactTitleTabs(tabs []string, active, maxWidth int) string {
	if len(tabs) == 0 || maxWidth <= 0 {
		return ""
	}
	if active < 0 || active >= len(tabs) {
		active = 0
	}
	sep := grammar.C("mut", " · ")
	build := func(l, r int) string {
		parts := make([]string, 0, r-l+3)
		if l > 0 {
			parts = append(parts, grammar.C("mut", fmt.Sprintf("‹+%d", l)))
		}
		parts = append(parts, tabs[l:r+1]...)
		if r < len(tabs)-1 {
			parts = append(parts, grammar.C("mut", fmt.Sprintf("+%d›", len(tabs)-r-1)))
		}
		return strings.Join(parts, sep)
	}
	l, r := active, active
	for {
		addLeft := l > 0
		addRight := r < len(tabs)-1
		if !addLeft && !addRight {
			break
		}
		tryLeftFirst := active-l <= r-active
		advanced := false
		try := func(left bool) bool {
			nl, nr := l, r
			if left {
				if !addLeft {
					return false
				}
				nl--
			} else {
				if !addRight {
					return false
				}
				nr++
			}
			if ansi.StringWidth(build(nl, nr)) <= maxWidth {
				l, r = nl, nr
				return true
			}
			return false
		}
		if tryLeftFirst {
			advanced = try(true) || try(false)
		} else {
			advanced = try(false) || try(true)
		}
		if !advanced {
			break
		}
	}
	out := build(l, r)
	if ansi.StringWidth(out) > maxWidth {
		return ansi.Truncate(out, maxWidth, "…")
	}
	return out
}

func (m Model) windowSignal(page int) (string, string) {
	switch page {
	case PageEvents:
		if m.EventsDark {
			return "DARK", "red"
		}
		if n := len(m.Events); n > 0 {
			return fmt.Sprintf("%d", n), "mut"
		}
	case PageTraces:
		if m.TracesDark {
			return "DARK", "red"
		}
		if n := len(m.Traces); n > 0 {
			return fmt.Sprintf("%d", n), "mut"
		}
	case PageSessionTurns:
		if n := len(m.TurnLadder); n > 0 {
			return fmt.Sprintf("%d", n), "mut"
		}
	case PageTasks:
		if m.TasksDark {
			return "DARK", "red"
		}
		vis, _ := m.yardBlockedIndices() // AIR-aware blocked count — never tally a denied-gated task on air
		n, blocked := len(m.Tasks), len(vis)
		if blocked > 0 {
			return fmt.Sprintf("%d!%d", n, blocked), "red"
		}
		if n > 0 {
			return fmt.Sprintf("%d", n), "mut"
		}
	case PageSessions:
		if m.SessionsDark {
			return "DARK", "red"
		}
		if n := len(m.Sessions); n > 0 {
			hotIdx, _ := m.yardHotSessionIndices() // AIR-aware: a denied attention/blocker/readiness never tallies on air
			if hot := len(hotIdx); hot > 0 {
				return fmt.Sprintf("%d!%d", n, hot), "yel"
			}
			return fmt.Sprintf("%d", n), "mut"
		}
	case PageYard:
		if m.TasksDark && m.SessionsDark && m.EventsDark {
			return "DARK", "red"
		}
		vis, _ := m.yardBlockedIndices()
		blocked := len(vis)
		if blocked > 0 {
			return fmt.Sprintf("%d!%d", len(m.Tasks), blocked), "red"
		}
		if n := len(m.Tasks); n > 0 {
			return fmt.Sprintf("%d", n), "mut"
		}
	case PageReadiness:
		if m.TasksDark && m.SessionsDark {
			return "DARK", "red"
		}
		visHolds, _ := m.yardBlockedIndices()
		holds := len(visHolds)
		hot := 0
		for _, s := range m.Sessions {
			// a denied blocker must not push a session into the readiness hot count (presence leak)
			if s.Blocker != "" && s.Blocker != "none" && (!m.AIR || s.AIR["blocker"] == "ok") {
				hot++
			}
		}
		if holds > 0 || hot > 0 {
			return fmt.Sprintf("%d!%d", holds, hot), "red"
		}
		return "ok", "grn"
	case PageIntake:
		if m.IntakeDark {
			return "DARK", "red"
		}
		total := m.intakeAttentionTotal()
		if total > 0 {
			return fmt.Sprintf("%d!%d", len(m.Intake.Rows), total), "yel"
		}
		if n := len(m.Intake.Rows); n > 0 {
			return fmt.Sprintf("%d", n), "mut"
		}
	case PageCaps:
		if m.SessionsDark {
			return "DARK", "red"
		}
		total, blocked := m.capabilityTitleCounts()
		if blocked > 0 {
			return fmt.Sprintf("%dc!%d", total, blocked), "yel"
		}
		return fmt.Sprintf("%dc", total), "mut"
	case PageDynamics:
		if m.DynamicsDark {
			return "DARK", "red"
		}
		if n := len(m.Dynamics.AtResolution(m.DynScale).Nodes); n > 0 {
			return fmt.Sprintf("%d@%s", n, dynScaleShort(m.DynScale)), "mut"
		}
	case PageLoops:
		if m.DynamicsDark {
			return "DARK", "red"
		}
		if n := len(m.loopRows()); n > 0 {
			return fmt.Sprintf("%dL", n), "mut"
		}
		return "0L", "mut"
	case PageAxes:
		return fmt.Sprintf("%dA", len(grammar.Axes())), "mut"
	case PageIdentity:
		return fmt.Sprintf("%dI", len(m.identityRoster())), "mut"
	case PageRelational:
		return fmt.Sprintf("%dA6", len(m.consentFacets())), "mut"
	case PageEpistemics:
		if m.EpistemicsDark && m.DynamicsDark && m.IntakeDark && m.DomainsDark && m.CapabilitiesDark {
			return "DARK", "red"
		}
		rows := m.epistemicRows()
		gaps := 0
		for _, row := range rows {
			if row.Token == "red" || strings.Contains(strings.ToLower(row.Status), "missing") || strings.Contains(strings.ToLower(row.Status), "dark") {
				gaps++
			}
		}
		if gaps > 0 {
			return fmt.Sprintf("%d!%d", len(rows), gaps), "yel"
		}
		if len(rows) > 0 {
			return fmt.Sprintf("%d", len(rows)), "mut"
		}
	case PageCommands:
		return fmt.Sprintf("%d", len(verbs)), "mut"
	case PageWindows:
		return fmt.Sprintf("%d", len(windowRegistry)), "mut"
	case PageIntent:
		if strings.TrimSpace(m.IntentTarget) != "" {
			return m.IntentTarget, "yel"
		}
		return fmt.Sprintf("%d", len(lookupIntentArgs())), "mut"
	case PageSurfaces:
		return fmt.Sprintf("%d", len(surfaceRegistry)), "mut"
	case PageDomains:
		return fmt.Sprintf("%d", len(domainRegistry)), "mut"
	case PageLifecycles:
		if m.DomainsDark {
			return "DARK", "red"
		}
		if n := len(m.Domains.Lifecycles); n > 0 {
			return fmt.Sprintf("%d", n), "mut"
		}
		return fmt.Sprintf("%d", len(registeredLifecycleFallbacks())), "mut"
	case PageHelp, PageLegend:
		return "ref", "mut"
	}
	return "", "mut"
}

// Z1 — vital strip: row1 = mode/page + criticality-split task counts + spine; row2 = the
// EXCEPTION-ONLY Act strip (structured-silence when calm; a red hotlist of blocked items when not).
func (m Model) viewVital(w int) string {
	_, _, dark := m.pageMeta()
	// AIR state is PER-INSTANCE (m.AIR, in memory): >1 Reins may run at once, some ON-AIR and some
	// LOCAL. The badge is the unmistakable one-glance anchor so the operator never captures the wrong
	// terminal for the broadcast — "▮ ON-AIR" (broadcast convention) vs "● LOCAL" (private/cleartext).
	mode := grammar.C("grn", "● LOCAL")
	if m.AIR {
		mode = grammar.C("fch", "▮ ON-AIR")
	}
	spine := m.viewSpine(dark)
	ok, warn, maj, crit, hiddenCrit := 0, 0, 0, 0, 0
	for _, t := range m.Tasks {
		if m.AIR && t.AIR["criticality"] != "ok" {
			hiddenCrit++
			continue
		}
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
	}
	blocked, _ := m.yardBlockedIndices() // AIR-aware: on air, exclude tasks gated solely by a denied stage/predicted_stage/criticality (off air == blockedIndices()). Feeds the count, Act-strip ids, and blockedBreakdown consistently.
	dot := grammar.C("mut", " · ")
	lbl := func(c string) string { // counts are SELECTABLE in hint mode (cross-cutting: a count = a class)
		if m.Mode == ModeHint {
			return grammar.SelLabel(c)
		}
		return ""
	}
	var counts string
	if hiddenCrit > 0 {
		counts = grammar.C("mut", fmt.Sprintf("risk ▒▒▒ (%d hidden)", hiddenCrit))
	} else {
		counts = lbl("O") + grammar.C("grn", fmt.Sprintf("%d ok", ok)) + dot + lbl("W") + grammar.C("yel", fmt.Sprintf("%d warn", warn)) +
			dot + lbl("M") + grammar.C("org", fmt.Sprintf("%d major", maj)) + dot + lbl("C") + grammar.C("red", fmt.Sprintf("%d crit", crit))
	}
	splitChip := ""
	if m.sessionSplit() {
		// Inc-5 only-split: the four session-anchored pages always compose sessions │ drilldown.
		splitChip = grammar.C("mut", "  │  ") + grammar.C("yel", "split:ctx")
	}
	r1 := " " + mode + grammar.C("mut", "  │  tasks ") + grammar.C("brt", fmt.Sprintf("%d", len(m.Tasks))) +
		grammar.C("mut", " = ") + counts + grammar.C("mut", "  │  events ") + grammar.C("brt", fmt.Sprintf("%d", len(m.Events))) +
		grammar.C("mut", "  │  sessions ") + grammar.C("brt", fmt.Sprintf("%d", len(m.Sessions))) +
		grammar.C("mut", "  │  ") + spine + splitChip + grammar.C("mut", "  │  ") + m.readReceipt()
	if m.Flash != "" { // transient effect-confirmation — always-visible, even when an effect jumped to command mode
		r1 += grammar.C("mut", "   ") + grammar.FlashLabel(" "+m.Flash+" ")
	}

	var r2 string
	if len(blocked) == 0 {
		r2 = grammar.C("mut", " "+strings.Repeat("·", 24)+"  all clear — nothing blocked")
	} else {
		head := blocked
		if len(head) > 2 {
			head = head[:2]
		}
		// each Act item is jumpable: in hint mode label it [1]/[2] (pick → cursor lands on that blocker).
		ids := make([]string, 0, len(head))
		for i, idx := range head {
			id := grammar.Redact(m.Tasks[idx].AIR, "task_id", m.Tasks[idx].TaskID, m.AIR) // Act strip honors AIR
			if m.Mode == ModeHint {
				id = grammar.SelLabel(fmt.Sprintf("%d", i+1)) + id
			}
			ids = append(ids, id)
		}
		hint := "  · [f] then 1/2 jumps to a blocker"
		if m.Mode == ModeHint {
			hint = "  · press 1/2 to jump"
		}
		hold, risk := m.blockedBreakdown(blocked)
		r2 = grammar.C("red", fmt.Sprintf(" ‼ %d task-gated", len(blocked))) +
			grammar.C("mut", fmt.Sprintf(" (%d hold · %d risk) · ", hold, risk)) +
			grammar.C("org", strings.Join(ids, "  ")) + grammar.C("mut", hint)
	}
	return r1 + "\n" + r2
}

func (m Model) blockedBreakdown(indices []int) (hold, risk int) {
	for _, idx := range indices {
		if idx < 0 || idx >= len(m.Tasks) {
			continue
		}
		t := m.Tasks[idx]
		if m.AIR && (t.AIR["predicted_stage"] != "ok" || t.AIR["criticality"] != "ok") {
			continue // on air a denied predicted_stage OR criticality must not be classified — the hold·risk split (and crit/major membership) discloses it
		}
		if strings.EqualFold(t.PredictedStage, "hold") {
			hold++
			continue
		}
		risk++
	}
	return hold, risk
}

// Z2a — the main pane body: the active page, rendered by the cell grammar.
func (m Model) bodyFor(w, h int) string {
	// The view-algebra entry: a migrated page composes as a layout.Spec (only-split). Pages not yet
	// migrated fall through to the legacy split / single-pane codepaths, which retire at Increment 5.
	if spec := m.composePage(w, h); spec != nil {
		return layout.Render(spec, w, h)
	}
	// Inc-5: the legacy session-frozen splitContextBody fallback is RETIRED. The session-anchored pages
	// (Caps/Yard/Readiness/Intake) render their split via composePage above; every other page — algebra
	// pages that returned nil (dark/empty) AND the pure reference/door pages — renders its own body.
	// Reference pages route to referenceBody (catalog │ ambient context), never a session-frozen anchor.
	return m.bodyForPage(w, h)
}

// hconcatCols is the HConcat composition primitive (framework §1 Layer-3 seed): join N fitted columns
// (each h lines tall) side-by-side with a divider between them. The view-algebra's split = HConcat; the
// Yard Coordinator composes its OWN columns this way rather than inheriting the session-frozen global
// split (dissolving the "left always frozen to sessions" anti-pattern).
func hconcatCols(div string, h int, cols ...[]string) string {
	out := make([]string, 0, h)
	for i := 0; i < h; i++ {
		var row strings.Builder
		for c, col := range cols {
			if c > 0 {
				row.WriteString(div)
			}
			if i < len(col) {
				row.WriteString(col[i])
			}
		}
		out = append(out, row.String())
	}
	return strings.Join(out, "\n")
}

// coordinatorBody composes the Yard Coordinator as up to THREE coordinated surfaces the lens selection
// drives (framework §5: the Yard-Coordinator + Hapax-chat adjacency): LENS │ COORDINATOR │ CHAT, an
// HConcat of page-owned panes (no session-source assumptions leak in). Width-responsive: 3 columns
// when wide, lens │ coordinator when medium, coordinator-only when narrow.
// composePage is the SINGLE view-algebra entry (only-split): a page → its layout.Spec, rendered by
// the one pure fold. As cohorts migrate it covers more pages; once total, bodyFor collapses to
// layout.Render(composePage(...)) and the legacy split codepaths are deleted. nil = not-yet-migrated
// (bodyFor falls through to the legacy path for that page).
func (m Model) composePage(w, h int) *layout.Spec {
	switch m.Page {
	case PageCoordinator:
		return m.specCoordinator()
	case PageEvents:
		if m.EventsDark {
			return nil // dark: fall through to eventsBody (preserves the dark-reason disclosure)
		}
		return m.specListContext(
			&layout.Pane{MinW: 72, Render: func(pw, ph int) string { return m.eventsListBody(pw, ph) }},
			&layout.Pane{MinW: 56, Render: func(pw, ph int) string { return m.eventContextPane(pw) }},
			0.65, m.eventsEmergentRelation())
	case PageTasks:
		if m.TasksDark {
			return nil // dark: fall through to taskBody (dark-reason disclosure)
		}
		return m.specListContext(
			&layout.Pane{MinW: 74, Render: func(pw, ph int) string { return m.tasksListBody(pw, ph) }},
			&layout.Pane{MinW: 56, Render: func(pw, ph int) string { return m.taskWorkDomainPane(pw) }},
			0.60, m.tasksEmergentRelation())
	case PageSessions:
		if m.SessionsDark {
			return nil // dark: fall through to sessionsBody (dark-reason disclosure)
		}
		return m.specListContext(
			&layout.Pane{MinW: 76, Render: func(pw, ph int) string { return m.sessionsListBody(pw, ph) }},
			&layout.Pane{MinW: 56, Render: func(pw, ph int) string { return m.sessionConstraintPane(pw) }},
			0.62, m.sessionsEmergentRelation())
	case PageEpistemics:
		// Inc 3 TRANSFORM — SELF-ANCHORED like tasks/events (own EpiFocus). The legacy session-frozen
		// split-pair (sessions │ evidence) is ABOLISHED: the primary IS the posture list, [j/k] moves the
		// epistemic row natively (composesViaAlgebra → not session-anchored → the updateSplitSource
		// intercept is skipped → j falls through to epistemicFocusTo), and the secondary is the focused
		// row's evidence path. The connector is the EMERGENT row-to-siblings relation, never authored.
		if len(m.epistemicRows()) == 0 {
			return nil // empty/dark → the legacy single-pane body (NO EPISTEMIC ROWS disclosure)
		}
		return m.specListContext(
			&layout.Pane{MinW: 76, Render: func(pw, ph int) string { return m.epistemicListBody(pw, ph) }},
			&layout.Pane{MinW: 56, Render: func(pw, ph int) string { return m.renderSelectedEpistemicPath(pw) }},
			0.62, m.epistemicsEmergentRelation())
	case PageTraces:
		// Inc 3 TRANSFORM — SELF-ANCHORED like events/tasks (own TFocus, ungated by the session-anchored split).
		// The legacy session-frozen reference split (sessions │ trace feed) is ABOLISHED: the primary IS
		// the trace list, [j/k] moves the trace row natively, and the secondary is the focused trace's
		// authored spend/latency detail (the narrow list clips trailing cost). Connector = shared model.
		if m.TracesDark || len(m.Traces) == 0 {
			return nil // dark/empty → tracesBody (dark hint / empty list)
		}
		return m.specListContext(
			&layout.Pane{MinW: 64, Render: func(pw, ph int) string { return m.tracesListBody(pw, ph) }},
			&layout.Pane{MinW: 40, Render: func(pw, ph int) string { return m.renderSelectedTrace(pw) }},
			0.62, m.tracesEmergentRelation())
	case PageSessionTurns:
		// E4.2 — SESSION TURN-LADDER, fixture-fed ahead of CapabilityIO. The primary is the receded
		// chat/turn ladder; the secondary is the focused turn's expanded block stream. It is self-anchored
		// like traces/events: [j/k] moves TurnFocus and bindings resolve to the turn, never a lane row.
		if len(m.TurnLadder) == 0 {
			return nil
		}
		return m.specListContext(
			&layout.Pane{MinW: 72, Render: func(pw, ph int) string { return m.turnListBody(pw, ph) }},
			&layout.Pane{MinW: 48, Render: func(pw, ph int) string { return m.turnDetailBody(pw) }},
			0.55, "focused turn → blocks")
	case PageIntent:
		// Inc 3 TRANSFORM — SELF-ANCHORED (own IntentFocus; the explicit j handler beats isReferencePage).
		// The legacy session-frozen reference split (sessions │ intent review) is ABOLISHED: the primary
		// IS the governed-route targets list, [j/k] moves the target, and the secondary is the selected
		// target's review ladder. The targets are a static catalog, so the connector is the honest
		// "selected target → governed route review" elucidation (like :dispatch), not relate.Derive.
		if len(lookupIntentArgs()) == 0 {
			return nil
		}
		return m.specListContext(
			&layout.Pane{MinW: 60, Render: func(pw, ph int) string { return m.intentTargetsBody(pw, ph) }},
			&layout.Pane{MinW: 56, Render: func(pw, ph int) string { return m.renderSelectedIntentReview(pw) }},
			0.50, "selected target → governed route review")
	case PageDynamics:
		// Inc 3 TRANSFORM — SELF-ANCHORED (own DynFocus; the explicit dynamicsFocusTo j handler beats the
		// isReferencePage scroll fallback). The legacy session-frozen reference split (sessions │ dynamics
		// map) is ABOLISHED: the primary IS the navigable map document, [j/k] moves the element, and the
		// secondary is the focused element's full detail (the full inline element is omitted from the map).
		// The secondary is genuinely the cursor's element, so the connector is an honest elucidation.
		if m.DynamicsDark || len(m.dynamicsFocusRows()) == 0 {
			return nil // dark/empty → bodyForPage (the legacy reference document / dark hint)
		}
		return m.specListContext(
			&layout.Pane{MinW: 72, Render: func(pw, ph int) string { return m.dynamicsMapBody(pw, ph) }},
			&layout.Pane{MinW: 48, Render: func(pw, ph int) string { return m.renderDynamicsSelectedElement(pw) }},
			0.62, "selected map element ← navigate the map")
	case PageLoops:
		// E7.1 — A5 Tier-1 causal-loop page promoted from --probe into a live, algebra-composed window.
		// It derives qualitative feedback structure from m.Dynamics at render time: no simulation, no
		// probe fixture unless the dynamics graph itself is empty. Empty acyclic graphs still compose and
		// disclose that there are no current loops.
		return m.specListContext(
			&layout.Pane{MinW: 70, Render: func(pw, ph int) string { return m.loopListBody(pw, ph) }},
			&layout.Pane{MinW: 50, Render: func(pw, ph int) string { return m.loopDetailBody(pw) }},
			0.50, "loop -> structure")
	case PageAxes:
		return m.specListContext(
			&layout.Pane{MinW: 64, Render: func(pw, ph int) string { return m.axisListBody(pw, ph) }},
			&layout.Pane{MinW: 52, Render: func(pw, ph int) string { return m.axisDetailBody(pw) }},
			0.50, "axis -> five-tuple contract")
	case PageIdentity:
		return m.specListContext(
			&layout.Pane{MinW: 60, Render: func(pw, ph int) string { return m.identityListBody(pw, ph) }},
			&layout.Pane{MinW: 52, Render: func(pw, ph int) string { return m.identityDetailBody(pw) }},
			0.5, "principal -> identity contract")
	case PageRelational:
		return m.specListContext(
			&layout.Pane{MinW: 58, Render: func(pw, ph int) string { return m.relationalListBody(pw, ph) }},
			&layout.Pane{MinW: 54, Render: func(pw, ph int) string { return m.relationalDetailBody(pw) }},
			0.46, "consent facet -> posture")
	case PageCaps:
		// Inc 2 — SESSION-ANCHORED drilldown. Unlike the Inc 1 self-anchored pages, caps' secondary is
		// the SELECTED SESSION's capability fit (renderSelectedCapabilityFit keys off FocusedSession), so
		// the primary IS the sessions list and the session-source nav/binding must STAY. We therefore only
		// swap the legacy splitContextBody RENDER for the algebra fold (gated on the session-anchored split, so
		// the session-anchored split/commandSelectionPage/nav are unchanged — no silent binding inversion), and
		// abolish the authored split-pair: the connector is the honest literal self-elucidation join.
		if !m.sessionSplit() {
			return nil // narrow / split-off → the legacy reference body (caps is not yet only-split)
		}
		return layout.Split(
			layout.Leaf(&layout.Pane{MinW: 76, Render: func(pw, ph int) string { return m.splitSessionsPane(pw, ph) }}),
			layout.Leaf(&layout.Pane{MinW: 56, Render: func(pw, ph int) string { return m.renderCapabilitySplitPane(pw, ph) }}),
			0.55, m.verdictConnector("selected lane → capability fit"))
	case PageYard, PageReadiness, PageIntake:
		// Inc 2 — the SESSION-ANCHORED drilldowns. Same safe recipe as caps: swap the legacy
		// splitContextBody RENDER for the algebra fold (gated on the session-anchored split → session-source
		// nav/binding unchanged). The secondary is splitContextPane (the per-page drilldown dispatcher
		// the legacy split already used, including the pinned card via splitReferenceSlice). The primary
		// is splitSessionsPane, so these projection pages need NO own row-list / custom spec — the
		// session IS the cursor (yard anchored session-per the playbook, operator-confirmed).
		if !m.sessionSplit() {
			return nil // narrow / split-off → the legacy reference body
		}
		rel := map[int]string{
			PageYard:      "selected lane → yard drilldown",
			PageReadiness: "selected lane → gate stack",
			PageIntake:    "selected lane → intake observations",
		}[m.Page]
		ratio := map[int]float64{PageYard: 0.58, PageReadiness: 0.57, PageIntake: 0.56}[m.Page]
		return layout.Split(
			layout.Leaf(&layout.Pane{MinW: 76, Render: func(pw, ph int) string { return m.splitSessionsPane(pw, ph) }}),
			layout.Leaf(&layout.Pane{MinW: 56, Render: func(pw, ph int) string { return m.splitContextPane(pw, ph) }}),
			ratio, m.verdictConnector(rel))
	case PageHelp, PageLegend, PageCommands, PageWindows, PageSurfaces, PageDomains, PageLifecycles:
		// Inc 4 — DEMOTE-TO-DOOR. These engine/reference pages have NO real session join, so when the
		// legacy split would session-anchor them (the anti-pattern), render the honest catalog │ ambient
		// door instead. Gated on the session-anchored split (off → referenceBody already does catalog│context).
		if !m.sessionSplit() {
			return nil
		}
		return m.specDoor()
	case PageDispatch:
		// STANDING: the dispatch LEDGER (primary) │ its MEASUREMENT rollup (secondary). The secondary is
		// genuinely DERIVED from the primary (utilization + blind-spots over the same records) — a real
		// elucidation, not a minted join, so the connector relation is honest.
		return layout.Split(
			layout.Leaf(&layout.Pane{MinW: 64, Render: func(pw, ph int) string { return m.dispatchLedgerPane(pw, ph) }}),
			layout.Leaf(&layout.Pane{MinW: 40, Render: func(pw, ph int) string { return m.dispatchMeasurementPane(pw) }}),
			0.62, m.verdictConnector("measurement derived from the ledger"))
	}
	return nil
}

// dispatchLedgerPane renders the cc-dispatch ledger (the primary of the :dispatch page): one compact
// ACTIVITY row per record (newest first, windowed to the height). The measurement half lives in the
// secondary. Empty → says so plainly rather than faking activity.
func (m Model) dispatchLedgerPane(w, h int) string {
	var b strings.Builder
	b.WriteString(" " + grammar.C("brt", "DISPATCH LEDGER") + grammar.C("mut", "  capability · route · slice · admission · launched · latency") + "\n")
	b.WriteString(" " + grammar.C("border", strings.Repeat("─", maxVisible(10, w-2))) + "\n")
	if len(m.DispatchRecords) == 0 {
		b.WriteString(" " + grammar.C("mut", "(no dispatches recorded — ledger empty; the cc-dispatch lane has not emitted yet)"))
		return strings.TrimRight(b.String(), "\n")
	}
	visible := maxVisible(1, h-3)
	for i, r := range m.DispatchRecords {
		if i >= visible {
			b.WriteString(" " + grammar.C("mut", fmt.Sprintf("…+%d older", len(m.DispatchRecords)-visible)) + "\n")
			break
		}
		b.WriteString("  " + grammar.RenderDispatchRowCompact(r, m.AIR) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// dispatchMeasurementPane renders the measurement readout (the secondary, derived from the same
// ledger): the measurement-completion summary (cost/quality/outcome gaps COUNTED + named, never
// faked), the latent-resource utilization split, and the standing blind-spots. Measurement-first.
// econCells projects the live dispatch ledger into (task×capability) economics cells. v̂ is UNBUILT
// (dev2's STEP-7 producer) → every live cell carries ValueStatus "absent", so the partition honestly
// reads FRONTIER UNDEFINED until the producer ships (never a fabricated rank). Cost rides the ledger's
// pointer (nil = UNMEASURED). Gate-state from !Launched — orthogonal to dominance.
func (m Model) econCells() []grammar.EconCell {
	cells := make([]grammar.EconCell, 0, len(m.DispatchRecords))
	for _, r := range m.DispatchRecords {
		cells = append(cells, grammar.EconCell{
			Capability:  r.Capability,
			Task:        r.CCTask,
			CostUSD:     r.CostUSD,
			ValueStatus: "absent", // v̂ producer unbuilt
			Conf:        "candidate",
			Held:        !r.Launched,
		})
	}
	return cells
}

func (m Model) dispatchMeasurementPane(w int) string {
	var b strings.Builder
	b.WriteString(" " + grammar.C("brt", "MEASUREMENT") + grammar.C("mut", "  derived from the ledger") + "\n")
	b.WriteString(" " + grammar.C("border", strings.Repeat("─", maxVisible(10, w-2))) + "\n")
	// the (task×capability) Pareto PARTITION (frontier/dominated/incomparable) — a partial order, never a
	// rank. AIR policy: $cost denies on air → the partition SEALS (value airs, cost does not).
	part := grammar.ClassifyEcon(m.econCells(), m.AIR, func(axis string) bool { return axis == "value" })
	b.WriteString(grammar.RenderEconPartition(part, m.AIR, false, w) + "\n\n")
	b.WriteString(grammar.RenderMeasurementSummary(grammar.SummarizeMeasurement(m.DispatchRecords)) + "\n\n")
	b.WriteString(grammar.RenderUtilization(grammar.Utilization(m.DispatchRecords, dispatchRoutableSet)) + "\n\n")
	b.WriteString(grammar.RenderDispatchBlindSpots())
	return strings.TrimRight(b.String(), "\n")
}

// dispatchRoutableSet is the canonical routable capability set the utilization rollup measures
// active-vs-latent against (the "latent resource" denominator). Mirrors the cc-dispatch resolver's
// active set + the gated follow-on routes.
var dispatchRoutableSet = []string{
	"glm-via-cc", "codex.full", "claude.fast", "claude.interactive", "agy",
	"api.provider_gateway", "fugu", "fugu-ultra", "glmcp-worker", "sakana",
}

// specCoordinator is the flagship STANDING-EMERGENT split: LENS │ (COORDINATOR │ CHAT). The lens is
// the single driving locus; coordinator+chat are transcluded from its selection. The root connector
// relation is EMERGENT (relate.Derive); the inner one is ambient (no authored join → no header).
func (m Model) specCoordinator() *layout.Spec {
	div := grammar.C("border", "│")
	lens := &layout.Pane{MinW: 28, Render: func(pw, ph int) string { return m.coordinatorLensPane(pw, ph) }}
	coord := &layout.Pane{MinW: 22, Render: func(pw, ph int) string { return m.coordinatorContextPane(pw, ph) }}
	// The files zone uses the SAME standard three-pane layout (operator: return to the previous
	// coordinator layout, before the expanded-cell real-estate reallocation) — the preview renders
	// in the middle COORD pane, the chat keeps its column.
	chat := &layout.Pane{MinW: 20, Render: func(pw, ph int) string { return m.coordinatorChatPane(pw, ph) }}
	inner := layout.Split(layout.Leaf(coord), layout.Leaf(chat), 0.5, layout.Connector{Glyph: div})
	return layout.Split(layout.Leaf(lens), inner, 0.4, m.verdictConnector(m.coordinatorEmergentRelation()))
}

// coordinatorRelation derives the lens selection's emergent relationship to the rest of the visible
// lattice ONCE (AIR-aware), so the connector label and the brush set agree. v1 consumes facet-overlap
// (the richer-edge relation-derivation PRODUCER is cross-repo, not yet built).
func (m Model) coordinatorRelation() (relate.Relation, bool) {
	focused, ok := m.FocusedTask()
	if !ok {
		return relate.Relation{}, false
	}
	others := make([]relate.Entity, 0, len(m.visibleTasks()))
	for _, t := range m.visibleTasks() {
		if t.TaskID == focused.TaskID {
			continue
		}
		others = append(others, taskEntity(t, m.AIR))
	}
	return relate.Derive(taskEntity(focused, m.AIR), others, nil), true
}

// coordinatorEmergentRelation renders the connector label AIR-aware; when the relation has
// participants it decodes the ├ brush glyph in-band ("· ├ related") so the highlighted rows read.
func (m Model) coordinatorEmergentRelation() string {
	rel, ok := m.coordinatorRelation()
	if !ok {
		return ""
	}
	label := m.airRelationLabel(rel)
	if len(rel.Peers) > 0 {
		label += " · ├ related"
	}
	return label
}

// coordinatorBrushedTasks is the brush set: the task ids participating in the focused task's emergent
// relation (relate.Derive Peers). AIR-safe — the same air-aware facets feed the derive (a relation
// over a redacted facet never forms) and the brush is positional (highlights rows, never prints ids).
func (m Model) coordinatorBrushedTasks() map[string]bool {
	rel, ok := m.coordinatorRelation()
	if !ok || len(rel.Peers) == 0 {
		return nil
	}
	set := make(map[string]bool, len(rel.Peers))
	for _, id := range rel.Peers {
		set[id] = true
	}
	return set
}

// emergentRelation derives a pane-to-pane connector relation (relate.Derive) and renders it
// AIR-aware — the one helper every migrated page's connector reuses.
func (m Model) emergentRelation(anchor relate.Entity, others []relate.Entity, edges []relate.Edge) string {
	return m.airRelationLabel(relate.Derive(anchor, others, edges))
}

// taskEntity (and eventEntity/sessionEntity) feed relate.Derive so the connector relation is EMERGENT
// (strongest shared facet), never authored. AIR-AWARE: on air a facet whose source field is DENIED is
// OMITTED entirely — so relate.Derive can never pick it, and the connector never discloses "shares
// crit (N)" over a redacted criticality (the facet-CHOICE + count is itself a derived-channel leak).
func taskEntity(t grammar.Task, airOn bool) relate.Entity {
	facets := map[string]string{}
	put := func(facet, field, val string) {
		if val == "" || (airOn && t.AIR[field] != "ok") {
			return
		}
		facets[facet] = val
	}
	put("owner", "owner", t.Owner)
	put("stage", "stage", shortStage2(t.Stage))
	put("crit", "criticality", t.Criticality)
	put("case", "authority_case", t.AuthorityCase)
	return relate.Entity{ID: t.TaskID, Facets: facets}
}

// Events have no id, so identity is the (ts,kind,subject) composite (used only to skip the anchor
// from its own others set — never rendered).
func eventEntity(e grammar.Event, airOn bool) relate.Entity {
	facets := map[string]string{}
	put := func(facet, field, val string) {
		if val == "" || (airOn && e.AIR[field] != "ok") {
			return
		}
		facets[facet] = val
	}
	put("subject", "subject", e.Subject)
	put("actor", "actor", e.Actor)
	put("kind", "kind", shortKindForPanel(e.Kind))
	return relate.Entity{ID: e.TS + "|" + e.Kind + "|" + e.Subject, Facets: facets}
}

func sessionEntity(s grammar.Session, airOn bool) relate.Entity {
	id := s.Session
	if id == "" {
		id = s.Role
	}
	facets := map[string]string{}
	put := func(facet, field, val string) {
		if val == "" || (airOn && s.AIR[field] != "ok") {
			return
		}
		facets[facet] = val
	}
	put("role", "role", s.Role)
	put("task", "claimed_task", s.ClaimedTask)
	put("readiness", "readiness", s.Readiness)
	return relate.Entity{ID: id, Facets: facets}
}

// specListContext is the shared STANDING-ELUCIDATE shape: a list primary │ the focused row's context
// secondary, joined by the EMERGENT connector relation. Every self-anchored cohort page composes
// through this — the generalization of specCoordinator's split.
func (m Model) specListContext(primary, secondary *layout.Pane, ratio float64, relation string) *layout.Spec {
	return layout.Split(layout.Leaf(primary), layout.Leaf(secondary), ratio, m.verdictConnector(relation))
}

// verdictConnector stamps the connector with the PAGE'S typed-join verdict — pageVerdict, the tested
// 19-page decision classification (gate.go) — and the matching honesty clause. Standing/Peek carry a
// real join keyed to the operator's SELECTION, so the header asserts that key; a DOOR carries NO real
// join, so the key is dropped and the header declares "no join" (the never-mint honesty floor). This
// is what lets only-split-by-default co-hold with honesty: a door page can't imply a coordination.
func (m Model) verdictConnector(relation string) layout.Connector {
	v := pageVerdict(m.Page)
	key := "selection → detail"
	if v == VerdictDoor {
		key = "" // honesty floor: a Door asserts no join
	}
	return layout.Connector{
		Glyph:    grammar.C("border", "│"),
		Relation: relation,
		Verdict:  v.String(),
		JoinKey:  key,
	}
}

// specDoor is the shared DEMOTE-TO-DOOR shape (Inc 4): an engine/reference page has NO real session
// join, so its secondary is honest AMBIENT context — the page catalog (referenceSlice) │ its
// contract/source state (referenceContextPane), a constant "ambient context" relation that asserts
// nothing it cannot back. This swaps the session-frozen splitContextBody anti-pattern (a help page
// anchored on a session) for the only-split door; referenceBody already renders this same
// catalog│context shape (no session-frozen anchor).
func (m Model) specDoor() *layout.Spec {
	return layout.Split(
		layout.Leaf(&layout.Pane{MinW: 96, Render: func(pw, ph int) string { return m.referenceSlice(pw, ph) }}),
		layout.Leaf(&layout.Pane{MinW: 56, Render: func(pw, ph int) string { return m.referenceContextPane(pw) }}),
		0.75, m.verdictConnector("ambient context"))
}

// eventsRelation anchors on the focused event vs the rest of m.Events, deriving ONCE so the connector
// label and the brush set agree. AIR-aware (facet VALUES/peers withheld on air).
func (m Model) eventsRelation() (relate.Relation, bool) {
	focused, ok := m.FocusedEvent()
	if !ok {
		return relate.Relation{}, false
	}
	anchor := eventEntity(focused, m.AIR)
	others := make([]relate.Entity, 0, len(m.Events))
	for _, e := range m.Events {
		oe := eventEntity(e, m.AIR)
		if oe.ID == anchor.ID {
			continue
		}
		others = append(others, oe)
	}
	return relate.Derive(anchor, others, nil), true
}

// brushedEvents is the brush set: the composite ids of the events participating in the focused
// event's emergent relation (relate.Peers). AIR-safe — air-aware facets feed the derive; positional.
func (m Model) brushedEvents() map[string]bool {
	rel, ok := m.eventsRelation()
	if !ok || len(rel.Peers) == 0 {
		return nil
	}
	set := make(map[string]bool, len(rel.Peers))
	for _, id := range rel.Peers {
		set[id] = true
	}
	return set
}

// eventsEmergentRelation renders the connector label AIR-aware; decodes the ├ brush glyph in-band.
func (m Model) eventsEmergentRelation() string {
	rel, ok := m.eventsRelation()
	if !ok {
		return ""
	}
	label := m.airRelationLabel(rel)
	if len(rel.Peers) > 0 {
		label += " · ├ related"
	}
	return label
}

// tasksEmergentRelation anchors on the focused task; others = the visible task list (same derivation
// as the coordinator's task-anchored relation).
func (m Model) tasksEmergentRelation() string {
	focused, ok := m.FocusedTask()
	if !ok {
		return ""
	}
	others := make([]relate.Entity, 0, len(m.visibleTasks()))
	for _, t := range m.visibleTasks() {
		if t.TaskID == focused.TaskID {
			continue
		}
		others = append(others, taskEntity(t, m.AIR))
	}
	return m.emergentRelation(taskEntity(focused, m.AIR), others, nil)
}

// epistemicEntity builds the relate facets for an epistemic posture row. The row is ALREADY
// AIR-projected (epistemicRows applied m.AIR), so a denied field is the redaction token; we OMIT any
// redacted/empty facet so relate.Derive can never derive over a denied dimension (the connector would
// otherwise disclose "shares family (N)" over a withheld field — the derived-channel leak the audit
// closes everywhere else). Subject is the row IDENTITY (and the PII-risk field) — not a shared facet.
func epistemicEntity(row epistemicRow) relate.Entity {
	facets := map[string]string{}
	put := func(facet, val string) {
		if val == "" || strings.Contains(val, "▒") { // empty or AIR-redacted → never relate over it
			return
		}
		facets[facet] = val
	}
	put("family", row.Family)
	put("status", row.Status)
	put("authority", row.Authority)
	put("privacy", row.Privacy)
	return relate.Entity{ID: row.RowID + "|" + row.Family + "|" + row.Subject, Facets: facets}
}

// epistemicsRelation anchors on the focused posture row vs the rest of the rows, deriving ONCE so the
// connector label and the brush set agree. The rows are already AIR-projected by epistemicRows.
func (m Model) epistemicsRelation() (relate.Relation, bool) {
	rows := m.epistemicRows()
	if len(rows) == 0 {
		return relate.Relation{}, false
	}
	idx := clamp(m.EpiFocus, 0, len(rows)-1)
	others := make([]relate.Entity, 0, len(rows))
	for i, r := range rows {
		if i == idx {
			continue
		}
		others = append(others, epistemicEntity(r))
	}
	return relate.Derive(epistemicEntity(rows[idx]), others, nil), true
}

// brushedEpistemics is the brush set: the composite ids of the posture rows participating in the
// focused row's emergent relation (relate.Peers). AIR-safe — rows are projected before deriving.
func (m Model) brushedEpistemics() map[string]bool {
	rel, ok := m.epistemicsRelation()
	if !ok || len(rel.Peers) == 0 {
		return nil
	}
	set := make(map[string]bool, len(rel.Peers))
	for _, id := range rel.Peers {
		set[id] = true
	}
	return set
}

// epistemicsEmergentRelation renders the connector label AIR-aware; decodes the ├ brush glyph in-band.
func (m Model) epistemicsEmergentRelation() string {
	rel, ok := m.epistemicsRelation()
	if !ok {
		return ""
	}
	label := m.airRelationLabel(rel)
	if len(rel.Peers) > 0 {
		label += " · ├ related"
	}
	return label
}

// traceEntity builds the relate facets for an LLM trace row. Cost/latency are continuous (not facets);
// the meaningful shared dimension is the MODEL (these N calls hit the same model). Denied facets are
// omitted so the connector never derives over a withheld field.
func traceEntity(tr grammar.Trace, airOn bool) relate.Entity {
	facets := map[string]string{}
	if tr.Model != "" && !(airOn && tr.AIR["model"] != "ok") {
		facets["model"] = tr.Model
	}
	return relate.Entity{ID: tr.TraceID + "|" + tr.TS, Facets: facets}
}

// tracesRelation anchors on the focused trace vs the rest of the feed, deriving ONCE so the connector
// label and the brush set agree. AIR-aware (facet VALUES/peers withheld on air).
func (m Model) tracesRelation() (relate.Relation, bool) {
	focused, ok := m.FocusedTrace()
	if !ok {
		return relate.Relation{}, false
	}
	others := make([]relate.Entity, 0, len(m.Traces))
	for i, tr := range m.Traces {
		if i == m.TFocus {
			continue
		}
		others = append(others, traceEntity(tr, m.AIR))
	}
	return relate.Derive(traceEntity(focused, m.AIR), others, nil), true
}

// brushedTraces is the brush set: the composite ids of the traces participating in the focused trace's
// emergent relation (relate.Peers). AIR-safe — air-aware facets feed the derive; positional.
func (m Model) brushedTraces() map[string]bool {
	rel, ok := m.tracesRelation()
	if !ok || len(rel.Peers) == 0 {
		return nil
	}
	set := make(map[string]bool, len(rel.Peers))
	for _, id := range rel.Peers {
		set[id] = true
	}
	return set
}

// tracesEmergentRelation renders the connector label AIR-aware; decodes the ├ brush glyph in-band.
func (m Model) tracesEmergentRelation() string {
	rel, ok := m.tracesRelation()
	if !ok {
		return ""
	}
	label := m.airRelationLabel(rel)
	if len(rel.Peers) > 0 {
		label += " · ├ related"
	}
	return label
}

// sessionsEmergentRelation anchors on the focused session; others = the rest of the fleet.
func (m Model) sessionsEmergentRelation() string {
	focused, ok := m.FocusedSession()
	if !ok {
		return ""
	}
	anchor := sessionEntity(focused, m.AIR)
	others := make([]relate.Entity, 0, len(m.Sessions))
	for _, s := range m.Sessions {
		oe := sessionEntity(s, m.AIR)
		if oe.ID == anchor.ID {
			continue
		}
		others = append(others, oe)
	}
	return m.emergentRelation(anchor, others, nil)
}

// airRelationLabel renders an emergent relation, withholding sensitive facet VALUES (owner/case)
// and edge PEER ids on air while keeping the structural shape — never a raw-value leak.
func (m Model) airRelationLabel(r relate.Relation) string {
	switch r.Kind {
	case "edge":
		if m.AIR {
			return grammar.C("2nd", r.Type+" ▒▒▒")
		}
		return grammar.C("2nd", r.Label)
	case "facet":
		if m.AIR && !airSafeFacet(r.Facet) {
			return grammar.C("2nd", fmt.Sprintf("shares %s (%d)", r.Facet, r.Count))
		}
		return grammar.C("2nd", r.Label)
	default:
		return ""
	}
}

// airSafeFacet allowlists the STRUCTURAL facets whose VALUE is safe to air (stage/score carry no
// PII). Default-deny: every other facet's value — criticality, owner, case, actor, subject, role,
// kind, or any unknown/newly-added facet — is withheld on air (a shared-criticality facet airs as
// "shares crit (N)", the structural shape, value withheld), so a sensitive facet can never leak.
func airSafeFacet(f string) bool {
	switch f {
	case "stage", "score":
		return true
	}
	return false
}

// coordinatorRelationBanner names the coordination TYPE + join, always-on (framework §5: "adjacency is
// never mysterious"). The lens selection drives the coordinator + chat; the SAME selected id echoes
// across all three panes — the brush made legible. Dual-coded (word + the ⟶ connector glyph).
func (m Model) coordinatorRelationBanner(w int) string {
	sel := "—"
	if t, ok := m.FocusedTask(); ok {
		if id := grammar.Redact(t.AIR, "task_id", t.TaskID, m.AIR); strings.TrimSpace(id) != "" {
			sel = id
		}
	}
	msg := fmt.Sprintf("▶ COORDINATION   lens selection ⟶ drives ⟶ coordinator + chat   ·   join: {{sel}} lattice-descent   ·   sel=%s", sel)
	return " " + grammar.C("2nd", clipRunes(msg, maxVisible(8, w-1)))
}

// coordinatorChatPane: the Yard Coordinator's CHAT surface (framework §5 — the Hapax-chat adjacency).
// The lens selection TRANSCLUDES into the chat as the grounding context turn ({{sel}}); the
// conversation renders turn-shaped (the session pane's grammar, integrated); an input affordance sits
// at the foot. Live agent dispatch is gated on CapabilityIO (#4296), so SEND is a governed stub — the
// pane is render + transclude + input integrated, AIR-safe (free-text turn summaries redact on air).
func (m Model) coordinatorChatPane(w, h int) string {
	var b strings.Builder
	// the chat STEERS DIRECTION only (priority · hold · scope · accept/reject) — never speed/provider/
	// fanout; those are DERIVED by the routing/admission calculation (the throughput-governed doctrine).
	b.WriteString(" " + grammar.C("brt", "CHAT") + grammar.C("mut", " · steer direction — not speed/provider") + "\n")
	b.WriteString(" " + grammar.C("border", strings.Repeat("─", maxVisible(10, w-2))) + "\n")
	marks := map[string]string{"operator": "›", "hapax": "‹", "lens": "·"}
	for _, t := range m.coordinatorChatTurns() {
		mark := marks[t.Role]
		if mark == "" {
			mark = "·"
		}
		sum := grammar.Redact(t.AIR, "summary", t.Summary, m.AIR)
		roleTok := airHue(grammar.LaneToken(t.Role), t.AIR, "role", m.AIR) // hue is a derived channel — demote on denied role
		roleText := grammar.Redact(t.AIR, "role", t.Role, m.AIR)
		line := grammar.C(roleTok, fmt.Sprintf(" %s %-8s ", mark, roleText)) + grammar.C("pri", sum)
		b.WriteString(clipRunes(line, w) + "\n")
	}
	if m.Mode == ModeSendGate {
		// the egress preview is AIR-safe by construction (RenderInjectionComposer redacts text bodies +
		// paths on air; secrets surface off-air only). The send is a stub — explicit confirm/dump only.
		for _, ln := range strings.Split(grammar.RenderInjectionComposer(m.composeParts(), m.AIR), "\n") {
			b.WriteString(clipRunes(ln, w) + "\n")
		}
		b.WriteString(clipRunes(grammar.C("yel", " [enter/y] confirm send (stub) · [d] dump/kill · [esc] back to compose"), maxVisible(8, w)))
		return strings.TrimRight(b.String(), "\n")
	}
	prompt := " › "
	if m.Mode == ModeCoordChat {
		prompt += m.CoordChatInput + "▌"
	} else {
		prompt += grammar.C("mut", "[c] steer: prioritize · hold · clarify scope · accept/reject (send gated)")
	}
	b.WriteString(clipRunes(grammar.C("pri", prompt), maxVisible(8, w)) + "\n")
	return strings.TrimRight(b.String(), "\n")
}

// coordinatorChatTurns seeds the chat from the lens selection (the {{sel}} transclusion) + the
// operator's local coordination log. Free-text summaries carry no "summary":"ok" so they redact on air.
func (m Model) coordinatorChatTurns() []grammar.Turn {
	sk := map[string]string{"ts": "ok", "kind": "ok", "role": "ok"}
	var turns []grammar.Turn
	if t, ok := m.FocusedTask(); ok {
		crit := t.Criticality
		if crit == "" {
			crit = "ok"
		}
		ctx := fmt.Sprintf("{{sel}} %s · stage %s · %s · %d ties",
			grammar.Redact(t.AIR, "task_id", t.TaskID, m.AIR), t.Stage, crit, t.RelCount)
		turns = append(turns, grammar.Turn{Role: "lens", Kind: "reasoning", Summary: ctx, AIR: sk})
	}
	hadLocal := false
	for _, msg := range m.CoordChatLog {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "operator"
		}
		messageHadPart := false
		for _, part := range msg.Parts {
			summary, air := m.coordChatPartSummary(part)
			if strings.TrimSpace(summary) == "" {
				continue
			}
			hadLocal, messageHadPart = true, true
			turns = append(turns, grammar.Turn{Role: role, Kind: coordChatPartKind(part), Summary: summary, AIR: air})
		}
		if !messageHadPart {
			continue
		}
		turns = append(turns, grammar.Turn{Role: "hapax", Kind: "refusal", Summary: "queued · live dispatch awaits the CapabilityIO session backend (Phase-1 capture-output)", AIR: sk})
	}
	if !hadLocal {
		turns = append(turns, grammar.Turn{Role: "hapax", Kind: "assistant", Summary: "you steer DIRECTION (priority/hold/scope/accept); speed, provider, and fanout are derived — not yours to set", AIR: sk})
	}
	return turns
}

func coordChatPartKind(part CoordChatPart) string {
	switch strings.ToLower(strings.TrimSpace(part.Type)) {
	case CoordChatPartFile, CoordChatPartImage:
		return "attachment"
	default:
		return "user"
	}
}

func (m Model) coordChatPartSummary(part CoordChatPart) (string, map[string]string) {
	switch strings.ToLower(strings.TrimSpace(part.Type)) {
	case CoordChatPartFile, CoordChatPartImage:
		return m.coordChatFileChip(part), map[string]string{"role": "ok", "summary": "ok"}
	default:
		return part.Text, map[string]string{"role": "ok"}
	}
}

func (m Model) coordChatFileChip(part CoordChatPart) string {
	path := strings.TrimSpace(part.FilePath)
	name := strings.TrimSpace(part.Text)
	if name == "" && path != "" {
		name = filepath.Base(path)
	}
	if name == "" {
		name = "attachment"
	}
	mime := strings.TrimSpace(part.MimeType)
	if mime == "" {
		mime = "unknown"
	}
	// The filename is sensitive PII — a name like "medical-results.png" leaks regardless of the path.
	// Deny it on air (it shares the path's confidentiality); the mime is the structural TYPE and is
	// the air-safe skeleton (the coarse tier the block-pixel preview airs), so it stays visible.
	safeName := grammar.Redact(map[string]string{}, "file_name", name, m.AIR)
	chip := fmt.Sprintf("▤ %s (%s)", safeName, mime)
	if path == "" {
		return chip
	}
	redactedPath := grammar.Redact(map[string]string{}, "file_path", path, m.AIR)
	return chip + " · path " + redactedPath
}

// coordinatorLensPane: the Yard Coordinator's LEFT pane — a Miller-column lens over the selection
// lattice (framework §5 + the selection-model spec). v1 shows L0 ZONE (the lattice's domains; the
// live subject is `tasks`) and L2 ROW (the tasks, rendered THROUGH the cell-grammar encoder), with the
// cursor; [j/k] drives m.Focus, which brushes the right coordinator. The breadcrumb situates the
// cursor in the whole. (L3 field / L4 token descent + zone-switching land next.)
func (m Model) coordinatorLensPane(w, h int) string {
	var b strings.Builder
	b.WriteString(" " + grammar.C("2nd", clipRunes("LENS · the cursor of attention over the selection lattice", maxVisible(8, w-1))) + "\n")
	if m.LensZone == "files" {
		zone := " " + grammar.C("mut", "zone  ") + grammar.C("mut", "tasks   sessions   events   gates   ") + grammar.C("pri", "▸files")
		b.WriteString(clipRunes(zone, maxVisible(8, w)) + "\n")
		b.WriteString(" " + grammar.C("border", strings.Repeat("─", maxVisible(10, w-2))) + "\n")
		if m.FilesErr != "" {
			b.WriteString(" " + grammar.C("red", "dir error: ") + grammar.C("mut", clipRunes(m.FilesErr, maxVisible(8, w-2))))
			return strings.TrimRight(b.String(), "\n")
		}
		visible := h - 5
		if visible < 1 {
			visible = 1
		}
		start := 0
		if len(m.FilesEntries) > visible {
			if m.FilesCursor >= visible {
				start = m.FilesCursor - visible + 1
			}
			if mx := len(m.FilesEntries) - visible; start > mx {
				start = mx
			}
		}
		end := start + visible
		if end > len(m.FilesEntries) {
			end = len(m.FilesEntries)
		}
		marks := m.basketMarks(m.FilesEntries)
		b.WriteString(files.RenderListMarked(m.FilesEntries[start:end], m.FilesCwd, m.FilesCursor-start, marks[start:end], m.AIR, w-1) + "\n")
		hint := "[j/k] move · [l/⏎] dir · [h] up · [space] stage · [z] tasks"
		if n := len(m.Basket); n > 0 {
			hint = fmt.Sprintf("▣ %d staged → {{basket}} · ", n) + hint
		}
		b.WriteString(" " + grammar.C("mut", clipRunes(hint, maxVisible(8, w-1))))
		return strings.TrimRight(b.String(), "\n")
	}
	zone := " " + grammar.C("mut", "zone  ") + grammar.C("pri", "▸tasks") + grammar.C("mut", "   sessions   events   gates")
	b.WriteString(clipRunes(zone, maxVisible(8, w)) + "\n")
	b.WriteString(" " + grammar.C("border", strings.Repeat("─", maxVisible(10, w-2))) + "\n")
	tasks := m.visibleTasks()
	if len(tasks) == 0 {
		b.WriteString(" " + grammar.C("2nd", "no tasks · waiting for /read/tasks") + "\n")
		return strings.TrimRight(b.String(), "\n")
	}
	b.WriteString("  " + grammar.RenderTaskHeader() + "\n")
	visible := h - 6 // header · zone · rule · task-header · rule · breadcrumb
	if visible < 1 {
		visible = 1
	}
	start := 0
	if len(tasks) > visible {
		if m.Focus >= visible {
			start = m.Focus - visible + 1
		}
		if mx := len(tasks) - visible; start > mx {
			start = mx
		}
	}
	brushed := m.coordinatorBrushedTasks()
	for i := start; i < start+visible && i < len(tasks); i++ {
		row := grammar.RenderTaskRow(tasks[i], m.AIR)
		switch {
		case i == m.Focus:
			b.WriteString(grammar.C("yel", m.focusGlyph()) + focusBar(row, w-1) + "\n")
		case brushed[tasks[i].TaskID]:
			// brushed: this row shares the focused task's strongest emergent facet (├ decoded by the
			// connector label "shares … · ├ related"). 1-wide gutter — alignment with the plain rows.
			b.WriteString(grammar.C("2nd", "├") + row + "\n")
		default:
			b.WriteString(" " + row + "\n")
		}
	}
	b.WriteString(" " + grammar.C("border", strings.Repeat("─", maxVisible(10, w-2))) + "\n")
	// the lattice breadcrumb descends one more rank (Z3 = the focused row's facets) so the part shows
	// its place in the whole — zone → row → field, the Miller-column descent made legible.
	z3 := "—"
	if t, ok := m.FocusedTask(); ok {
		crit := t.Criticality
		if crit == "" {
			crit = "ok"
		}
		z3 = fmt.Sprintf("%s·%s·%s", grammar.Redact(t.AIR, "criticality", crit, m.AIR), dashOr(t.Stage), dashOr(grammar.Redact(t.AIR, "owner", t.Owner, m.AIR)))
	}
	crumb := fmt.Sprintf("▶ path  Z0▸tasks ▸ Z2▸row %d/%d ▸ Z3▸[%s]   ·   [j/k] row · [c] chat", m.Focus+1, len(tasks), z3)
	b.WriteString(" " + grammar.C("mut", clipRunes(crumb, maxVisible(8, w-1))) + "\n")
	return strings.TrimRight(b.String(), "\n")
}

// airSeverityToken is the criticality hue, demoted to "mut" when criticality is denied on air — the
// hue is a derived channel that would otherwise disclose crit/major on an allowlisted stage/state
// cell even though the value itself is redacted (mirrors the whois-door criticality handling).
func airSeverityToken(crit string, airMap map[string]string, air bool) string {
	if air && airMap["criticality"] != "ok" {
		return "mut"
	}
	return grammar.SeverityToken(crit)
}

// airHue demotes a DERIVED hue to "mut" when its field is denied on air. The hue (a meaning-channel
// color) is itself a derived channel that discloses the redacted value — a lane/state/readiness/blocker
// /attention/owner/predicted/freshness token computed from a raw field still leaks the field on air even
// when the VALUE is redacted to ▒▒▒. This generalizes airSeverityToken to the whole hue-leak class.
func airHue(tok string, airMap map[string]string, field string, air bool) string {
	if air && airMap[field] != "ok" {
		return "mut"
	}
	return tok
}

// dashOr returns "—" for an empty value (compact structured silence in one-line summaries).
func dashOr(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// coordinatorContextPane: the Yard Coordinator's RIGHT pane — the coordinator context the lens
// selection drives. The SELECTION block transcludes the focused task (the {{sel}} of framework §5),
// then the live Yard cockpit (ladder / attention / fleet / gates). AIR-safe throughout.
func (m Model) coordinatorContextPane(w, h int) string {
	if m.LensZone == "files" {
		return m.coordinatorFilePreview(w, h)
	}
	var b strings.Builder
	b.WriteString(" " + grammar.C("brt", "COORDINATOR") + grammar.C("mut", " · the lens selection drives this context") + "\n")
	b.WriteString(m.coordinatorSelectionContext(w))
	b.WriteString(m.coordinatorThroughputLine(w) + "\n")
	b.WriteString(" " + grammar.C("border", strings.Repeat("─", maxVisible(10, w-2))) + "\n")
	b.WriteString(m.renderYardCockpit(w))
	return strings.TrimRight(b.String(), "\n")
}

// coordinatorFilePreview renders the focused filebrowser entry in the wide coordinator pane. For an
// IMAGE it shows the ACTUAL pixels (imgpreview half-block) in the operator's present-at-hand frame;
// ON AIR it is shape-only — metadata, pixels withheld — egress-safe by construction. The filename
// is sensitive and redacts on air.
func (m Model) coordinatorFilePreview(w, h int) string {
	var b strings.Builder
	e, ok := m.focusedFile()
	if !ok {
		b.WriteString(" " + grammar.C("brt", "PREVIEW") + "\n")
		b.WriteString(grammar.C("mut", "  (no file focused — [j/k] to move)"))
		return b.String()
	}
	name := grammar.Redact(nil, "label", e.Name, m.AIR) // a filename can carry identity — redact on air
	b.WriteString(" " + grammar.C("brt", "PREVIEW") + grammar.C("mut", " · ") + grammar.C("pri", name) + "\n")
	b.WriteString(" " + grammar.C("border", strings.Repeat("─", maxVisible(10, w-2))) + "\n")
	if e.IsDir {
		b.WriteString(grammar.C("mut", "  directory · [l/⏎] enter"))
		return b.String()
	}
	if isImageExt(e.Ext) {
		if m.AIR {
			// On air (operator ruling 2026-06-27): the coarse BLOCK-PIXEL rendering. RenderFileAIR
			// clamps the half-block to a hard resolution ceiling so fine detail (text, faces) is
			// destroyed below legibility while the gist survives — confidentiality-by-resolution, not
			// by withholding. It never embeds the filename: a decode failure folds to a name-free
			// shape-only line (Metadata would leak the name the header just redacted).
			cols := maxVisible(8, w-2)
			rows := h - 4
			if rows < 2 {
				rows = 2
			}
			b.WriteString(imgpreview.RenderFileAIR(filepath.Join(m.FilesCwd, e.Name), cols, rows))
			return b.String()
		}
		// Off air (the operator's present-at-hand frame): render the ACTUAL image at higher resolution
		// via braille dot-matrix (2×4 dots/cell ≈ 4× the half-block); decode failure folds to metadata.
		cols := maxVisible(8, w-2)
		rows := h - 4
		if rows < 2 {
			rows = 2
		}
		out, _ := imgpreview.RenderFileBraille(filepath.Join(m.FilesCwd, e.Name), cols, rows)
		b.WriteString(out)
		return b.String()
	}
	// non-image file: pixel-free for now (a syntax-tokenized text head is a follow-up increment).
	meta := e.Ext
	if meta == "" {
		meta = "file"
	}
	b.WriteString("  " + grammar.C("2nd", meta) + grammar.C("mut", " · non-image · metadata-only (text-head preview is a follow-up)"))
	return strings.TrimRight(b.String(), "\n")
}

// isImageExt reports whether a lowercased extension is a previewable raster image (decode failures
// for the less-common formats fall back honestly to metadata in imgpreview.RenderFile).
func isImageExt(ext string) bool {
	switch ext {
	case "png", "jpg", "jpeg", "gif", "bmp", "webp":
		return true
	}
	return false
}

// coordinatorThroughputLine: the OBJECTIVE throughput calculation (the throughput-governed doctrine) —
// what the operator READS, never sets. Speed/provider/fanout are DERIVED from this, not steered: the
// operator steers only direction (the chat). WIP · the limiting constraint · attention contention.
func (m Model) coordinatorThroughputLine(w int) string {
	wip := len(m.Tasks)
	held, hot := 0, 0
	fresh := make([]float64, 0, len(m.Tasks))
	for _, t := range m.Tasks {
		// On air, a denied criticality/predicted_stage must not be classified into the held tally,
		// and a denied freshness must not enter the sparkline — the aggregate discloses the field.
		classOK := !m.AIR || (t.AIR["criticality"] == "ok" && t.AIR["predicted_stage"] == "ok")
		if classOK && (t.Criticality == "crit" || t.Criticality == "major" || t.PredictedStage == "hold") {
			held++
		}
		if !m.AIR || t.AIR["freshness"] == "ok" {
			fresh = append(fresh, t.Freshness)
		}
	}
	for _, s := range m.Sessions {
		// on air a denied attention must not be classified into the hot tally — the aggregate
		// discloses the per-session field the session row redacts.
		if (!m.AIR || s.AIR["attention"] == "ok") && s.Attention >= 0.5 {
			hot++
		}
	}
	sort.Slice(fresh, func(i, j int) bool { return fresh[i] > fresh[j] }) // the freshness PROFILE, fresh→stale
	spark := grammar.Sparkline(fresh, 24)
	msg := fmt.Sprintf("THROUGHPUT  WIP %d · limiting %d held · %d hot · fresh %s · speed/provider = DERIVED", wip, held, hot, spark)
	return " " + grammar.C("2nd", clipRunes(msg, maxVisible(8, w-1)))
}

// coordinatorSelectionContext: the brushed selection block — the focused task's coordination state,
// transcluded reference-not-copy and AIR-redacted (the left lens selection driving the right).
func (m Model) coordinatorSelectionContext(w int) string {
	t, ok := m.FocusedTask()
	if !ok {
		return " " + grammar.C("mut", "▶ selection  (none — [j/k] moves the lens cursor)") + "\n"
	}
	d := func(s string) string {
		if strings.TrimSpace(s) == "" {
			return "—"
		}
		return s
	}
	id := grammar.Redact(t.AIR, "task_id", t.TaskID, m.AIR)
	owner := grammar.Redact(t.AIR, "owner", t.Owner, m.AIR)
	crit := t.Criticality
	if crit == "" {
		crit = "ok"
	}
	// Renders on the LIVE coordinator: stage/prior/predicted/criticality/rel_count must redact
	// per-field on air (id + owner already do).
	stage := grammar.Redact(t.AIR, "stage", t.Stage, m.AIR)
	prior := grammar.Redact(t.AIR, "prior_stage", t.PriorStage, m.AIR)
	next := grammar.Redact(t.AIR, "predicted_stage", t.PredictedStage, m.AIR)
	critV := grammar.Redact(t.AIR, "criticality", crit, m.AIR)
	rel := grammar.Redact(t.AIR, "rel_count", fmt.Sprintf("%d", t.RelCount), m.AIR)
	var b strings.Builder
	b.WriteString(" " + grammar.C("pri", "▶ selection  ") + grammar.C("brt", clipRunes(d(id), maxVisible(8, w-14))) + "\n")
	ctx := fmt.Sprintf("   stage %s · prior %s · next %s · owner %s · %s · %s ties",
		d(stage), d(prior), d(next), d(owner), critV, rel)
	b.WriteString(" " + grammar.C("mut", clipRunes(ctx, maxVisible(8, w-1))) + "\n")
	return b.String()
}

func (m Model) bodyForPage(w, h int) string {
	switch m.Page {
	case PageTasks:
		return m.taskBody(w, h)
	case PageSessions:
		return m.sessionsBody(w, h)
	case PageTraces:
		return m.tracesBody(w, h)
	case PageSessionTurns:
		return m.turnListBody(w, h)
	case PageDynamics, PageLoops, PageAxes, PageIdentity, PageRelational, PageEpistemics, PageHelp, PageLegend, PageCommands, PageWindows, PageIntent, PageSurfaces, PageDomains, PageLifecycles, PageYard, PageReadiness, PageIntake, PageCaps:
		return m.referenceBody(w, h)
	default:
		return m.eventsBody(w, h)
	}
}

func splitContextWidths(w int) (leftW, rightW int, ok bool) {
	if w < splitContextMinWidth {
		return 0, 0, false
	}
	leftW = 112
	if w < 220 {
		leftW = 96
	}
	if leftW > w-64 {
		leftW = w - 64
	}
	if leftW < 76 {
		return 0, 0, false
	}
	rightW = w - leftW - 1
	if rightW < 56 {
		return 0, 0, false
	}
	return leftW, rightW, true
}

func (m Model) splitRelation() SplitPairDef {
	if rel, ok := splitPairForPage(m.Page); ok {
		return rel
	}
	name, _, _ := m.pageMeta()
	return SplitPairDef{
		Page:             m.Page,
		Source:           "sessions",
		Target:           name,
		Join:             "reference",
		Mode:             splitModeReference,
		SourceCursor:     splitSourceAnchor,
		TargetReactivity: splitTargetIndependent,
		TargetCursor:     splitTargetScroll,
		TargetScrollable: true,
		SourceOwnedVerbs: splitSessionVerbs(),
		Contract:         "context lens explains source without taking row ownership",
	}
}

func (m Model) targetRowFocusActive() bool {
	if !m.sessionSplit() {
		return true
	}
	rel := m.splitRelation()
	return rel.TargetUsesPageJK()
}

func (m Model) passiveReferenceSplitActive() bool {
	if !m.sessionSplit() {
		return false
	}
	rel := m.splitRelation()
	return !rel.Reactive() && !rel.TargetUsesNP()
}

func (m Model) splitRelationLabel() string {
	return m.splitRelation().RelationLabel()
}

func (m Model) splitRelationHeader(w int) string {
	r := m.splitRelation()
	label := m.splitRelationHeaderLabel(r, w)
	return " " + grammar.C("2nd", clipRunes(label, maxVisible(8, w-1)))
}

func (m Model) splitRelationHeaderLabel(r SplitPairDef, w int) string {
	if w <= 124 {
		mode := r.Mode
		if mode == splitModeReference {
			mode = "ref"
		}
		parts := []string{
			"ctx " + r.RelationLabel(),
			mode,
		}
		parts = append(parts, m.splitControlCueTexts(r, splitCueTextOptions{IncludeScroll: true, Compact: true, IncludeScrollLabel: true, IncludeContext: true, Color: false, IncludeSourceVerbs: false})...)
		label := strings.Join(parts, " · ")
		if ansi.StringWidth(label) <= maxVisible(8, w-1) {
			return label
		}
		parts = []string{
			"ctx " + r.Target,
			r.Join,
		}
		parts = append(parts, m.splitControlCueTexts(r, splitCueTextOptions{IncludeScroll: true, Compact: true, IncludeScrollLabel: false, IncludeContext: true, Color: false, IncludeSourceVerbs: false})...)
		label = strings.Join(parts, " · ")
		if ansi.StringWidth(label) <= maxVisible(8, w-1) {
			return label
		}
		parts = []string{"ctx " + r.Join}
		parts = append(parts, m.splitControlCueTexts(r, splitCueTextOptions{IncludeScroll: true, Compact: true, Minimal: true, IncludeScrollLabel: false, IncludeContext: true, Color: false, IncludeSourceVerbs: false})...)
		return strings.Join(parts, " · ")
	}
	nav := m.splitNavHint(r, true)
	return "context " + r.RelationLabel() + " · " + r.Mode + " · " + nav + " · contract " + r.Contract + " · [←/→] change context"
}

func (m Model) splitNavHint(r SplitPairDef, includeScrollLabel bool) string {
	return strings.Join(m.splitControlCueTexts(r, splitCueTextOptions{IncludeScroll: includeScrollLabel, Compact: false, IncludeScrollLabel: includeScrollLabel, IncludeContext: false, Color: false, IncludeSourceVerbs: false}), " · ")
}

type splitControlCue struct {
	Key     string
	Long    string
	Short   string
	Minimal string
	Kind    string
}

type splitCueTextOptions struct {
	IncludeScroll      bool
	Compact            bool
	Minimal            bool
	Tight              bool
	IncludeScrollLabel bool
	IncludeContext     bool
	Color              bool
	IncludeSourceVerbs bool
}

func (m Model) splitControlCues(r SplitPairDef, opts splitCueTextOptions) []splitControlCue {
	if m.Mode == ModeYank {
		return []splitControlCue{
			{Key: "[j/k]", Long: "source rows", Short: "src", Minimal: "src", Kind: "yank-source"},
			{Key: "[Tab/←/→]", Long: "fields", Short: "fields", Minimal: "fld", Kind: "field"},
		}
	}
	sourceShort := "anchor"
	sourceMin := "src"
	if r.Reactive() {
		sourceShort = "src→ctx"
		sourceMin = "src"
	}
	cues := []splitControlCue{{
		Key:     "[j/k]",
		Long:    r.SourceNavLabel(),
		Short:   sourceShort,
		Minimal: sourceMin,
		Kind:    "source",
	}}
	if long, short, ok := r.TargetNPLabels(); ok {
		cues = append(cues, splitControlCue{Key: "[n/p]", Long: long, Short: short, Minimal: short, Kind: "target"})
	}
	if opts.IncludeScroll && r.TargetScrollable && m.referenceScrollMax() > 0 {
		long := "context scroll"
		if opts.IncludeScrollLabel {
			long += " " + m.referenceScrollLabel()
		}
		cues = append(cues, splitControlCue{Key: "[J/K]", Long: long, Short: "scroll", Minimal: "scr", Kind: "scroll"})
	}
	if opts.IncludeContext {
		cues = append(cues, splitControlCue{Key: "[←/→]", Long: "context", Short: "ctx", Minimal: "ctx", Kind: "context"})
	}
	if opts.IncludeSourceVerbs {
		if r.SourceOwns("detail") {
			cues = append(cues, splitControlCue{Key: "[↵]", Long: "source-detail", Short: "src-detail", Minimal: "detail", Kind: "source-verb"})
		}
		if r.SourceOwns("resume") {
			cues = append(cues, splitControlCue{Key: "[r]", Long: "resume-intent", Short: "resume", Minimal: "resume", Kind: "source-verb"})
		}
		if r.SourceOwns("yank") {
			cues = append(cues, splitControlCue{Key: "[y]", Long: "source-yank", Short: "src-yank", Minimal: "yank", Kind: "source-verb"})
		}
		if m.Page == PageIntake {
			cues = append(cues, splitControlCue{Key: "[s/S]", Long: "filter", Short: "flt", Minimal: "flt", Kind: "source-verb"})
		}
	}
	return cues
}

func (m Model) splitControlCueTexts(r SplitPairDef, opts splitCueTextOptions) []string {
	cues := m.splitControlCues(r, opts)
	out := make([]string, 0, len(cues))
	for _, cue := range cues {
		label := cue.Long
		if opts.Compact {
			label = cue.Short
		}
		if opts.Minimal {
			label = cue.Minimal
		}
		if opts.Tight && cue.Kind == "yank-source" {
			label = "source-rows"
		}
		key := cue.Key
		if opts.Color {
			key = grammar.C("yel", key)
		}
		sep := ""
		if !opts.Compact && !opts.Minimal && !opts.Tight {
			sep = " "
		}
		out = append(out, key+sep+label)
	}
	return out
}

func (m Model) splitSessionsPane(w, h int) string {
	visible := h - 2
	if visible < 1 {
		visible = 1
	}
	if len(m.Sessions) == 0 {
		return grammar.C("2nd", " split sessions · no lanes · waiting for /read/sessions\n")
	}
	start := 0
	if len(m.Sessions) > visible {
		if m.SFocus >= visible {
			start = m.SFocus - visible + 1
		}
		if mx := len(m.Sessions) - visible; start > mx {
			start = mx
		}
	}
	ctx := ":"
	if wnd, ok := windowForPage(m.Page); ok {
		ctx += wnd.ID
	} else {
		ctx += "context"
	}
	var b strings.Builder
	rel := m.splitRelation()
	arrow := "source -> "
	link := rel.Target
	nav := m.splitNavHint(rel, false)
	if !rel.Reactive() {
		arrow = "anchor + "
		link = rel.Join
	}
	head := fmt.Sprintf("split sessions · %d lanes · %s%s · context %s · %s", len(m.Sessions), arrow, link, ctx, nav)
	if ansi.StringWidth(head) > maxVisible(8, w-1) {
		cues := strings.Join(m.splitControlCueTexts(rel, splitCueTextOptions{Compact: true, IncludeScrollLabel: false, IncludeContext: false, Color: false, IncludeSourceVerbs: false}), " · ")
		head = fmt.Sprintf("split sessions · %d · %s%s · %s", len(m.Sessions), arrow, link, cues)
	}
	if ansi.StringWidth(head) > maxVisible(8, w-1) {
		cues := strings.Join(m.splitControlCueTexts(rel, splitCueTextOptions{Compact: true, Minimal: true, IncludeScrollLabel: false, IncludeContext: false, Color: false, IncludeSourceVerbs: false}), " · ")
		head = fmt.Sprintf("split · %d · %s · %s", len(m.Sessions), link, cues)
	}
	b.WriteString(" " + grammar.C("2nd", clipRunes(head, maxVisible(8, w-1))) + "\n")
	b.WriteString("  " + grammar.C("mut", fmt.Sprintf("%-7s %-14s %-7s %-6s %s", "RDY", "ROLE", "PLAT", "ATTN", "BLOCK/TASK")) + "\n")
	rendered := 0
	for i := start; i < start+visible && i < len(m.Sessions); i++ {
		rendered++
		s := m.Sessions[i]
		if i == m.SFocus && (m.Page == PageSessions || m.sessionSplit()) && m.Mode == ModeYank {
			b.WriteString(fitWidth(sessionPickRow(s, m.AIR, m.Sel.Field), w) + "\n")
			continue
		}
		mark := " "
		selected := i == m.SFocus
		if selected {
			if rel.Reactive() {
				mark = m.focusGlyph()
			} else {
				mark = "◆"
			}
		}
		ready := sessionFieldValueForAir(s, "readiness", m.AIR)
		role := sessionFieldValueForAir(s, "role", m.AIR)
		plat := sessionFieldValueForAir(s, "platform", m.AIR)
		attn := sessionFieldValueForAir(s, "attention", m.AIR)
		context := m.splitSessionContext(s)
		// gate each derived HUE on its field's AIR — the value redacts above, but the heat/lane/
		// readiness color is itself a derived channel that discloses the denied field (fugu review).
		rdyTok, laneTok, attnTok := airHue(readinessPaneToken(s.Readiness), s.AIR, "readiness", m.AIR), airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR), airHue(attentionToken(s.Attention), s.AIR, "attention", m.AIR)
		row := grammar.C("yel", mark) +
			grammar.C(rdyTok, fmt.Sprintf(" %-7s", clipRunes(ready, 7))) +
			grammar.C(laneTok, fmt.Sprintf(" %-14s", clipRunes(role, 14))) +
			grammar.C("2nd", fmt.Sprintf(" %-7s", clipRunes(plat, 7))) +
			grammar.C(attnTok, fmt.Sprintf(" %-6s", clipRunes(attn, 6))) +
			grammar.C("mut", " "+clipRunes(context, maxVisible(8, w-42)))
		if selected {
			b.WriteString(focusBar(row, w) + "\n")
		} else {
			b.WriteString(row + "\n")
		}
	}
	if slack := visible - rendered; slack > 0 {
		for _, line := range m.splitSourceTopologyRows(w, slack) {
			b.WriteString(line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) splitSourceTopologyRows(w, maxRows int) []string {
	if maxRows <= 0 {
		return nil
	}
	var b strings.Builder
	rel := m.splitRelation()
	s, ok := m.FocusedSession()
	if !ok {
		writeWrappedKV(&b, "source", "no selected lane source", "mut", w)
		writeWrappedKV(&b, "relation", rel.RelationLabel(), "org", w)
		return firstLines(b.String(), maxRows)
	}
	role := sessionFieldValueForAir(s, "role", m.AIR)
	if strings.TrimSpace(role) == "" {
		role = "·"
	}
	claim := strings.TrimSpace(s.ClaimedTask)
	taskLink := "no claim"
	if m.AIR && s.AIR["claimed_task"] != "ok" {
		taskLink = "▒▒▒" // the visible/gap state discloses the denied claimed_task relationship
	} else if claim != "" {
		if _, found := m.taskByID(claim); found {
			taskLink = "task visible"
		} else {
			taskLink = "task gap"
		}
	}
	routeStr := "▒▒▒"
	if !m.AIR || s.AIR["platform"] == "ok" {
		routeStr = fmt.Sprintf("%d", len(m.capabilityRoutesForPlatform(strings.TrimSpace(s.Platform))))
	}
	srcTok := airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR)
	writeWrappedKV(&b, "source", strings.Join(nonEmptyParts([]string{role, sessionAnchorSignal(s, m.AIR), m.sessionLivePulse(s)}), " · "), srcTok, w)
	writeWrappedKV(&b, "relation", rel.RelationLabel()+" · "+rel.Contract, "org", w)
	writeWrappedKV(&b, "links", fmt.Sprintf("events:%d · claim:%s · cap-routes:%s", len(m.sessionRelatedEvents(s)), taskLink, routeStr), "2nd", w)
	writeWrappedKV(&b, "controls", m.splitNavHint(rel, false), "mut", w)
	return firstLines(b.String(), maxRows)
}

func firstLines(s string, maxRows int) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > maxRows {
		lines = lines[:maxRows]
	}
	return lines
}

func (m Model) splitSessionContext(s grammar.Session) string {
	switch m.Page {
	case PageEvents:
		related := m.sessionRelatedEvents(s)
		if len(related) > 0 {
			failures := 0
			for _, ev := range related {
				// a denied kind must not be classified into the fail tally — the count discloses it
				if (!m.AIR || ev.AIR["kind"] == "ok") && strings.Contains(strings.ToLower(ev.Kind), "fail") {
					failures++
				}
			}
			if failures > 0 {
				return fmt.Sprintf("%d events · %d fail", len(related), failures)
			}
			return fmt.Sprintf("%d events", len(related))
		}
	case PageTasks:
		claim := strings.TrimSpace(s.ClaimedTask)
		if claim == "" {
			return "no claimed task"
		}
		if _, ok := m.taskByID(claim); ok {
			return "task visible"
		}
		return "task gap"
	case PageCaps:
		fit, _ := capabilitySessionFit(s, m.AIR)
		if fit != "unknown" && fit != "fit hidden" {
			return fit
		}
	case PageIntake:
		return m.intakeSessionContext(s)
	case PageDynamics:
		return m.sourceOnlySessionContext(s, "system topology")
	case PageLoops:
		return m.sourceOnlySessionContext(s, "feedback structure")
	case PageAxes:
		return m.sourceOnlySessionContext(s, "case-role framework")
	case PageIdentity:
		return m.sourceOnlySessionContext(s, "identity roster")
	case PageRelational:
		return m.sourceOnlySessionContext(s, "consent posture")
	case PageEpistemics:
		return m.sourceOnlySessionContext(s, "evidence/provenance")
	case PageHelp:
		return m.sourceOnlySessionContext(s, "operator orientation")
	case PageLegend:
		return m.sourceOnlySessionContext(s, "glyph/color decoding")
	case PageCommands:
		return m.sourceOnlySessionContext(s, "command grammar")
	case PageWindows:
		return m.sourceOnlySessionContext(s, "window topology")
	case PageIntent:
		return m.sourceOnlySessionContext(s, "intent review anchor")
	case PageSurfaces:
		return m.sourceOnlySessionContext(s, "affordance registry")
	case PageDomains:
		return m.sourceOnlySessionContext(s, "domain lens")
	case PageLifecycles:
		return m.sourceOnlySessionContext(s, "lifecycle registry")
	}
	return fallbackSplitSessionContext(s, m.AIR)
}

func (m Model) sourceOnlySessionContext(s grammar.Session, relation string) string {
	return strings.Join(nonEmptyParts([]string{sessionAnchorSignal(s, m.AIR), relation, m.sessionLivePulse(s)}), " · ")
}

func sessionAnchorSignal(s grammar.Session, air bool) string {
	if air && (s.AIR["readiness"] != "ok" || s.AIR["state"] != "ok" || s.AIR["blocker"] != "ok") {
		return "anchor hidden"
	}
	blocker := strings.TrimSpace(s.Blocker)
	if blocker != "" && blocker != "none" {
		return strings.ReplaceAll(blocker, "_", " ")
	}
	ready := strings.TrimSpace(s.Readiness)
	switch ready {
	case "claim":
		return "claim-ready"
	case "stale":
		return "stale"
	case "off", "offline":
		return "offline"
	}
	state := strings.TrimSpace(s.State)
	if state == "offline" {
		return "offline"
	}
	if state != "" {
		return state
	}
	return "anchor"
}

func (m Model) sessionLivePulse(s grammar.Session) string {
	if m.AIR {
		for _, f := range []string{"state", "alive", "idle", "stalled", "blocker"} {
			if s.AIR[f] != "ok" {
				return ""
			}
		}
	}
	blocker := strings.TrimSpace(s.Blocker)
	if blocker != "" && blocker != "none" && blocker != "no_claim" {
		return ""
	}
	if !s.Alive || s.Idle || s.Stalled || s.State != "active" {
		return ""
	}
	return "live " + m.livenessGlyph()
}

func (m Model) sessionLiveGutter(s grammar.Session) string {
	if strings.TrimSpace(m.sessionLivePulse(s)) == "" {
		return "  "
	}
	return grammar.C("grn", m.livenessGlyph()) + " "
}

func nonEmptyParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, part)
		}
	}
	return out
}

func labeledPart(label, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return label + "=" + value
}

func fallbackSplitSessionContext(s grammar.Session, air bool) string {
	// gate the blocker branch on AIR: a denied blocker must fall through to claimed_task/state, else the
	// branch selection (▒▒▒ vs the next field) discloses that a blocker is present (presence leak)
	if strings.TrimSpace(s.Blocker) != "" && s.Blocker != "none" && (!air || s.AIR["blocker"] == "ok") {
		return sessionFieldValueForAir(s, "blocker", air)
	}
	if strings.TrimSpace(s.ClaimedTask) != "" {
		return sessionFieldValueForAir(s, "claimed_task", air)
	}
	return sessionFieldValueForAir(s, "state", air)
}

func (m Model) splitContextPane(w, h int) string {
	header := m.splitRelationHeader(w)
	bodyH := m.splitContextBodyRows(h)
	rel := m.splitRelation()
	if rel.Reactive() && h >= 20 {
		header += "\n" + m.splitFocusContractLine(rel, w)
	}
	var body string
	switch m.Page {
	case PageEvents:
		body = m.eventContextPane(w)
	case PageTasks:
		body = m.taskWorkDomainPane(w)
	case PageSessions:
		body = m.sessionConstraintPane(w)
	case PageCaps:
		body = m.renderCapabilitySplitPane(w, bodyH)
	case PageTraces, PageDynamics, PageLoops, PageAxes, PageIdentity, PageRelational, PageEpistemics, PageHelp, PageLegend, PageCommands, PageWindows, PageIntent, PageSurfaces, PageDomains, PageLifecycles, PageYard, PageReadiness, PageIntake:
		body = m.splitReferenceSlice(w, bodyH)
	default:
		body = m.eventContextPane(w)
	}
	if strings.TrimSpace(body) == "" {
		return header
	}
	return header + "\n" + body
}

func (m Model) splitContextBodyRows(h int) int {
	bodyH := h - 1
	if bodyH < 1 {
		bodyH = 1
	}
	if m.splitRelation().Reactive() && h >= 20 {
		bodyH--
		if bodyH < 1 {
			bodyH = 1
		}
	}
	return bodyH
}

func (m Model) splitFocusContractLine(rel SplitPairDef, w int) string {
	source := "no source"
	if s, ok := m.FocusedSession(); ok {
		source = sessionFieldValueForAir(s, "role", m.AIR)
		if strings.TrimSpace(source) == "" {
			source = "·"
		}
	}
	nav := strings.Join(m.splitControlCueTexts(rel, splitCueTextOptions{Compact: false, IncludeScrollLabel: false, IncludeContext: false, Color: false, IncludeSourceVerbs: false}), " · ")
	if rel.Reactive() {
		msg := fmt.Sprintf("focus %s · source focus drives context by %s · %s", source, rel.Join, nav)
		return " " + grammar.C("mut", clipRunes(msg, maxVisible(8, w-1)))
	}
	msg := fmt.Sprintf("anchor %s · context is independent reference by %s · %s", source, rel.Join, nav)
	return " " + grammar.C("mut", clipRunes(msg, maxVisible(8, w-1)))
}

func (m Model) splitReferenceSlice(w, h int) string {
	pinned, ok := m.splitPinnedContextBlock(w)
	if !ok {
		return m.referenceSlice(w, h)
	}
	pinnedLines := strings.Split(strings.TrimRight(pinned, "\n"), "\n")
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	if len(pinnedLines)+1 >= h {
		bodyRows := splitPinnedBodyBudget(h)
		pinnedRows := h - bodyRows - 1
		if pinnedRows < 1 {
			return strings.Join(fitBlockWithOverflow(pinned, w, h, "pinned context"), "\n")
		}
		bodyModel := m
		bodyModel.SuppressSplitPinned = true
		lines := bodyModel.referenceLines(w)
		return strings.Join(fitBlockWithOverflow(pinned, w, pinnedRows, "pinned context"), "\n") + "\n" + rule + "\n" + m.referenceSliceFromLines(lines, bodyRows)
	}
	scrollH := h - len(pinnedLines) - 1
	bodyModel := m
	bodyModel.SuppressSplitPinned = true
	lines := bodyModel.referenceLines(w)
	return strings.Join(pinnedLines, "\n") + "\n" + rule + "\n" + m.referenceSliceFromLines(lines, scrollH)
}

func splitPinnedBodyBudget(h int) int {
	if h < 5 {
		return 0
	}
	body := (h * 2) / 3
	if body < 5 {
		body = 5
	}
	if body > h-2 {
		body = h - 2
	}
	return body
}

func (m Model) splitPinnedContextBlock(w int) (string, bool) {
	rel := m.splitRelation()
	switch m.Page {
	case PageDynamics:
		if block := strings.TrimRight(m.renderDynamicsSelectedElement(w), "\n"); block != "" {
			return block, true
		}
	case PageEpistemics:
		if block := strings.TrimRight(m.renderSelectedEpistemicPath(w), "\n"); block != "" {
			return block, true
		}
	}
	if !rel.Reactive() {
		return m.renderSplitRelationCard(w), true
	}
	switch m.Page {
	case PageYard:
		return m.renderSelectedYardLane(w), true
	case PageReadiness:
		return m.renderSelectedReadinessGate(w), true
	case PageIntake:
		return m.renderSelectedIntakeLane(w), true
	case PageCaps:
		return m.renderSelectedCapabilityFit(w), true
	}
	return "", false
}

func (m Model) renderSplitRelationCard(w int) string {
	rel := m.splitRelation()
	var b strings.Builder
	writeSectionHeader(&b, w, "SPLIT RELATION", "source lane stays navigable; context pane owns this lens", rel.RelationLabel())
	writeWrappedKV(&b, "pair", rel.RelationLabel(), "org", w)
	writeWrappedKV(&b, "mode", string(rel.Mode)+" · "+rel.TargetReactivity, "2nd", w)
	if s, ok := m.FocusedSession(); ok {
		role := sessionFieldValueForAir(s, "role", m.AIR)
		if strings.TrimSpace(role) == "" {
			role = "·"
		}
		writeWrappedKV(&b, "source", strings.Join(nonEmptyParts([]string{role, sessionAnchorSignal(s, m.AIR), m.sessionLivePulse(s)}), " · "), airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR), w)
	} else {
		writeWrappedKV(&b, "source", "no selected lane", "mut", w)
	}
	writeWrappedKV(&b, "contract", rel.Contract, "mut", w)
	writeWrappedKV(&b, "controls", m.splitNavHint(rel, false), "yel", w)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) referenceSlice(w, h int) string {
	lines := m.referenceLines(w)
	return m.referenceSliceFromLines(lines, h)
}

func (m Model) referenceSliceFromLines(lines []string, h int) string {
	if len(lines) == 0 {
		return ""
	}
	max := len(lines) - h
	if max < 0 {
		max = 0
	}
	off := clamp(m.RefScroll, 0, max)
	end := off + h
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[off:end], "\n")
}

func (m Model) referenceContent(w int) string {
	_, _, dark := m.pageMeta()
	var b strings.Builder
	switch m.Page {
	case PageYard:
		b.WriteString(m.renderYardCockpit(w))
	case PageReadiness:
		b.WriteString(m.renderReadinessProjection(w))
	case PageIntake:
		b.WriteString(m.renderIntakeProjection(w))
	case PageCaps:
		b.WriteString(m.renderCapabilityProjection(w))
	case PageDynamics:
		if dark {
			b.WriteString(darkHint(m.DynamicsError, m.AIR))
		} else {
			scale, scaleLabel := m.dynamicsRenderScale(w)
			b.WriteString(grammar.DynamicsHeader(m.Dynamics, w)) // thesis + inline provenance key (situate)
			b.WriteString(" " + grammar.C("mut", "scale ") + grammar.C("yel", scaleLabel) +
				grammar.C("mut", " · [,/.] cycle overview/domain/artifact/runtime/evidence/all") + "\n")
			rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
			b.WriteString(rule + "\n")
			compactFirst := m.dynamicsCompactFirstViewport()
			if compactFirst {
				if m.Height >= 45 {
					b.WriteString(m.renderDynamicsPrimer(w))
					b.WriteString(rule + "\n")
				}
				b.WriteString(m.renderDynamicsSelectedElementCompact(w))
				b.WriteString(rule + "\n")
				b.WriteString(m.renderDynamicsGraphRail(w, scale))
				b.WriteString(rule + "\n")
				if workbench := m.renderDynamicsWorkbench(w); strings.TrimSpace(workbench) != "" {
					b.WriteString(workbench)
					b.WriteString(rule + "\n")
				}
				if summary := m.renderDynamicsGraphSummary(w, m.dynamicsRenderScaleValue(w)); strings.TrimSpace(summary) != "" {
					b.WriteString(summary)
					b.WriteString(rule + "\n")
				}
			} else {
				if m.Height >= 45 {
					b.WriteString(m.renderDynamicsPrimer(w))
					b.WriteString(rule + "\n")
				}
				if workbench := m.renderDynamicsWorkbench(w); strings.TrimSpace(workbench) != "" {
					b.WriteString(workbench)
					b.WriteString(rule + "\n")
				}
				if summary := m.renderDynamicsGraphSummary(w, m.dynamicsRenderScaleValue(w)); strings.TrimSpace(summary) != "" {
					b.WriteString(summary)
					b.WriteString(rule + "\n")
				}
				selectedBeforeRail := !m.sessionSplit() && !m.SuppressSplitPinned
				if selectedBeforeRail {
					b.WriteString(m.renderDynamicsSelectedElement(w))
					b.WriteString(rule + "\n")
				}
				b.WriteString(m.renderDynamicsGraphRail(w, scale))
				if !m.SuppressSplitPinned && !selectedBeforeRail {
					b.WriteString(m.renderDynamicsSelectedElement(w))
				}
			}
			b.WriteString(m.renderDynamicsGuide(w))
			b.WriteString(m.renderDynamicsPackage(w))
			b.WriteString(m.renderDynamicsPackageDetails(w))
		}
	case PageEpistemics:
		b.WriteString(m.renderEpistemicsProjection(w))
	case PageHelp:
		b.WriteString(grammar.RenderHelp())
	case PageLegend:
		b.WriteString(grammar.RenderLegend())
	case PageCommands:
		b.WriteString(m.renderCommandCatalog(w))
	case PageWindows:
		b.WriteString(m.renderWindowCatalog(w))
	case PageIntent:
		b.WriteString(m.renderIntentReview(w))
	case PageSurfaces:
		b.WriteString(m.renderSurfaceCatalog(w))
	case PageDomains:
		b.WriteString(m.renderDomainCatalog(w))
	case PageLifecycles:
		b.WriteString(m.renderLifecycleCatalog(w))
	}
	return b.String()
}

type epistemicRow struct {
	Family, Subject, Status, Authority string
	Evidence, Source, Freshness        string
	Privacy, Detail, Token             string
	RowID, SubjectKind, SubjectRef     string
	Posture, SourceRefs                string
	MapKind, MapID                     string
	MapSource, MapTarget, MapRelation  string
	SourceRefLabels                    []string
}

func (m Model) epistemicRows() []epistemicRow {
	rows := make([]epistemicRow, 0, 64)
	add := func(row epistemicRow) {
		if strings.TrimSpace(row.Subject) == "" {
			row.Subject = "·"
		}
		if strings.TrimSpace(row.Status) == "" {
			row.Status = "unknown"
		}
		if strings.TrimSpace(row.Authority) == "" {
			row.Authority = "support"
		}
		if strings.TrimSpace(row.Evidence) == "" {
			row.Evidence = "0"
		}
		if strings.TrimSpace(row.Source) == "" {
			row.Source = row.Family
		}
		if strings.TrimSpace(row.Freshness) == "" {
			row.Freshness = "·"
		}
		if strings.TrimSpace(row.Privacy) == "" {
			row.Privacy = "metadata-only"
		}
		if strings.TrimSpace(row.Token) == "" {
			row.Token = epistemicStatusToken(row.Status, row.Detail)
		}
		rows = append(rows, row)
	}
	sourceBackedMapRows := false
	sourceBackedPackageFamilies := map[string]bool{}
	if m.EpistemicsDark {
		add(epistemicRow{Family: "epistemics", Subject: "/read/epistemics", Status: "dark", Authority: "none", Source: "read source", Privacy: "metadata unavailable", Detail: m.EpistemicsError, Token: "red"})
	}
	for _, src := range m.Epistemics.Sources {
		add(epistemicSourceToRow(src, m.AIR))
	}
	for _, row := range m.Epistemics.Rows {
		er := epistemicReadRowToViewRow(row, m.AIR)
		if isMapEpistemicRow(er) {
			sourceBackedMapRows = true
		}
		if isPackageEpistemicRow(row, er) {
			sourceBackedPackageFamilies[er.Family] = true
		}
		add(er)
	}
	if m.DynamicsDark {
		add(epistemicRow{Family: "dynamics", Subject: "/read/dynamics", Status: "dark", Authority: "none", Source: "read source", Privacy: "metadata unavailable", Detail: m.DynamicsError, Token: "red"})
	}
	for _, src := range m.Dynamics.Package.Sources {
		add(epistemicRow{
			Family: "source", Subject: dynamicsSourceFieldForAir(src, "id", m.AIR),
			Status: dynamicsSourceFieldForAir(src, "status", m.AIR), Authority: firstNonEmpty(m.Dynamics.Package.Authority, "package"),
			Evidence: dynamicsSourceFieldForAir(src, "count", m.AIR), Source: "dynamics/package",
			Freshness: dynamicsSourceFieldForAir(src, "age_bucket", m.AIR),
			Privacy:   fmt.Sprintf("%s raw=%s", dynamicsSourceFieldForAir(src, "privacy", m.AIR), dynamicsSourceFieldForAir(src, "raw_access", m.AIR)),
			Detail:    dynamicsSourceFieldForAir(src, "detail", m.AIR),
		})
	}
	if !sourceBackedMapRows {
		for _, node := range m.Dynamics.Nodes {
			add(epistemicRow{
				Family: "map-node", Subject: dynamicsNodeFieldForAir(node, "id", m.AIR),
				Status: dynamicsNodeFieldForAir(node, "status", m.AIR), Authority: "map-element",
				Evidence: dynamicsNodeEvidenceForAir(node, m.AIR), Source: dynamicsNodeFieldForAir(node, "layer", m.AIR),
				Freshness: firstNonEmpty(dynamicsNodeFieldForAir(node, "res", m.AIR), "element"),
				Privacy:   "metadata-only",
				Detail:    dynamicsNodeEpistemicDetailForAir(node, m.AIR),
				MapKind:   "node", MapID: dynamicsNodeFieldForAir(node, "id", m.AIR),
			})
		}
		for _, edge := range m.Dynamics.Edges {
			add(epistemicRow{
				Family: "map-edge", Subject: dynamicsEdgeSubjectForAir(edge, m.AIR),
				Status: dynamicsEdgeFieldForAir(edge, "status", m.AIR), Authority: "map-element",
				Evidence: dynamicsEdgeEvidenceForAir(edge, m.AIR), Source: dynamicsEdgeFieldForAir(edge, "source", m.AIR),
				Freshness: firstNonEmpty(dynamicsEdgeFieldForAir(edge, "res", m.AIR), dynamicsEdgeFieldForAir(edge, "layer", m.AIR), "element"),
				Privacy:   "metadata-only",
				Detail:    dynamicsEdgeEpistemicDetailForAir(edge, m.AIR),
				MapKind:   "edge", MapID: dynamicsEdgeSubjectForAir(edge, m.AIR), MapSource: dynamicsEdgeFieldForAir(edge, "source", m.AIR), MapTarget: dynamicsEdgeFieldForAir(edge, "target", m.AIR), MapRelation: dynamicsEdgeFieldForAir(edge, "relation", m.AIR),
			})
		}
	}
	addDynRows := func(family, authority string, dynRows []grammar.DynamicsRow) {
		if sourceBackedPackageFamilies[family] {
			return
		}
		for _, row := range dynRows {
			add(epistemicRow{
				Family: family, Subject: dynamicsRowFieldForAir(row, "id", m.AIR),
				Status: dynamicsRowFieldForAir(row, "status", m.AIR), Authority: authority,
				Evidence: dynamicsRowFieldForAir(row, "count", m.AIR), Source: dynamicsRowFieldForAir(row, "source", m.AIR),
				Freshness: dynamicsRowFieldForAir(row, "severity", m.AIR), Detail: dynamicsRowFieldForAir(row, "detail", m.AIR),
			})
		}
	}
	addDynRows("validation", "declared-check", m.Dynamics.Package.Validation)
	addDynRows("lens", "projection", m.Dynamics.Package.Lenses)
	addDynRows("claim", "authority-separated", m.Dynamics.Package.Claims)
	addDynRows("observation", "observed-support", m.Dynamics.Package.Observations)
	addDynRows("relation", "vocabulary", m.Dynamics.Package.Relations)

	if m.IntakeDark {
		add(epistemicRow{Family: "intake", Subject: "/read/intake", Status: "dark", Authority: "none", Source: "read source", Privacy: "metadata unavailable", Detail: m.IntakeError, Token: "red"})
	}
	for _, src := range m.Intake.Sources {
		add(epistemicRow{
			Family: "intake-source", Subject: intakeSourceFieldForAir(src, "id", m.AIR),
			Status: intakeSourceFieldForAir(src, "status", m.AIR), Authority: "observation-source",
			Evidence: intakeSourceFieldForAir(src, "count", m.AIR), Source: "intake",
			Freshness: intakeSourceFieldForAir(src, "age_bucket", m.AIR), Privacy: intakeSourceFieldForAir(src, "privacy", m.AIR),
			Detail: intakeSourceFieldForAir(src, "path", m.AIR),
		})
	}
	for _, row := range m.Intake.Rows {
		subject := firstNonEmpty(intakeRowFieldForAir(row, "id", m.AIR), intakeRowFieldForAir(row, "kind", m.AIR))
		source := firstNonEmpty(intakeRowFieldForAir(row, "source_refs", m.AIR), intakeRowFieldForAir(row, "source", m.AIR))
		authority := firstNonEmpty(intakeRowFieldForAir(row, "authority", m.AIR), "observation-only")
		evidence := firstNonEmpty(intakeRowFieldForAir(row, "evidence", m.AIR), intakeRowFieldForAir(row, "count", m.AIR))
		detail := strings.Join(nonEmptyParts([]string{
			firstNonEmpty(intakeRowFieldForAir(row, "detail", m.AIR), "severity="+intakeRowFieldForAir(row, "severity", m.AIR)),
			labeledPart("coverage", intakeRowFieldForAir(row, "coverage", m.AIR)),
			labeledPart("link", intakeRowFieldForAir(row, "task_link_state", m.AIR)),
			intakeRowFieldForAir(row, "blocker", m.AIR),
			labeledPart("missing", intakeRowFieldForAir(row, "missing", m.AIR)),
			labeledPart("action", intakeRowFieldForAir(row, "action", m.AIR)),
			labeledPart("next", intakeRowFieldForAir(row, "next_evidence", m.AIR)),
		}), " · ")
		add(epistemicRow{
			Family: "intake", Subject: subject,
			Status: intakeRowFieldForAir(row, "status", m.AIR), Authority: authority,
			Evidence: evidence, Source: source,
			Freshness: intakeRowFieldForAir(row, "age_bucket", m.AIR), Detail: detail,
		})
	}

	if m.DomainsDark {
		add(epistemicRow{Family: "domains", Subject: "/read/domains", Status: "dark", Authority: "none", Source: "read source", Privacy: "metadata unavailable", Detail: m.DomainsError, Token: "red"})
	}
	for _, src := range append(m.Domains.LifecycleSources, m.Domains.Sources...) {
		add(epistemicRow{
			Family: "domain-source", Subject: domainSourceFieldForAir(src, "id", m.AIR),
			Status: domainSourceFieldForAir(src, "status", m.AIR), Authority: domainSourceFieldForAir(src, "authority", m.AIR),
			Evidence: domainSourceFieldForAir(src, "count", m.AIR), Source: "domain/lifecycle pack",
			Freshness: domainSourceFieldForAir(src, "age_bucket", m.AIR), Privacy: domainSourceFieldForAir(src, "privacy", m.AIR),
			Detail: domainSourceFieldForAir(src, "detail", m.AIR),
		})
	}
	for _, row := range m.Domains.Lifecycles {
		add(epistemicRow{
			Family: "lifecycle", Subject: lifecycleRowFieldForAir(row, "lifecycle_id", m.AIR),
			Status: lifecycleRowFieldForAir(row, "state", m.AIR), Authority: lifecycleRowFieldForAir(row, "authority_ceiling", m.AIR),
			Evidence: lifecycleRowFieldForAir(row, "evidence_count", m.AIR), Source: lifecycleRowFieldForAir(row, "source_refs", m.AIR),
			Freshness: lifecycleRowFieldForAir(row, "freshness_policy", m.AIR), Privacy: lifecycleRowFieldForAir(row, "air_class", m.AIR),
			Detail: strings.Join(nonEmptyParts([]string{
				lifecycleRowFieldForAir(row, "posture", m.AIR),
				lifecycleRowFieldForAir(row, "plant", m.AIR),
				lifecycleRowFieldForAir(row, "blocker", m.AIR),
				lifecycleRowFieldForAir(row, "next_evidence", m.AIR),
			}), " · "),
		})
	}
	for _, row := range m.Domains.Rows {
		add(epistemicRow{
			Family: "domain", Subject: domainRowFieldForAir(row, "domain_id", m.AIR),
			Status: domainRowFieldForAir(row, "state", m.AIR), Authority: domainRowFieldForAir(row, "authority_ceiling", m.AIR),
			Evidence: domainRowFieldForAir(row, "evidence_count", m.AIR), Source: domainRowFieldForAir(row, "source_refs", m.AIR),
			Freshness: domainRowFieldForAir(row, "lifecycle", m.AIR), Privacy: domainRowFieldForAir(row, "scope", m.AIR),
			Detail: strings.Join(nonEmptyParts([]string{
				domainRowFieldForAir(row, "terrain", m.AIR) + "/" + domainRowFieldForAir(row, "depth", m.AIR),
				domainRowFieldForAir(row, "windows", m.AIR),
				domainRowFieldForAir(row, "blocker", m.AIR),
			}), " · "),
		})
	}

	if m.CapabilitiesDark {
		add(epistemicRow{Family: "capabilities", Subject: "/read/capabilities", Status: "dark", Authority: "none", Source: "read source", Privacy: "metadata unavailable", Detail: m.CapabilitiesError, Token: "red"})
	}
	for _, src := range m.Capabilities.Sources {
		add(epistemicRow{
			Family: "capability-source", Subject: capabilitySourceFieldForAir(src, "id", m.AIR),
			Status: capabilitySourceFieldForAir(src, "status", m.AIR), Authority: "metadata-only source",
			Evidence: capabilitySourceFieldForAir(src, "count", m.AIR), Source: "capabilities/source",
			Freshness: capabilitySourceFieldForAir(src, "age_bucket", m.AIR),
			Privacy:   fmt.Sprintf("%s raw=%s", capabilitySourceFieldForAir(src, "privacy", m.AIR), capabilitySourceFieldForAir(src, "raw_access", m.AIR)),
			Detail:    capabilitySourceFieldForAir(src, "detail", m.AIR),
		})
	}
	for _, row := range m.Capabilities.Rows {
		add(epistemicRow{
			Family: "capability", Subject: capabilityRowFieldForAir(row, "capability_id", m.AIR),
			Status: capabilityRowFieldForAir(row, "status", m.AIR), Authority: capabilityRowFieldForAir(row, "authority", m.AIR),
			Evidence: capabilityRowFieldForAir(row, "evidence_count", m.AIR), Source: capabilityRowFieldForAir(row, "source_refs", m.AIR),
			Freshness: capabilityRowFieldForAir(row, "hkp_posture", m.AIR), Privacy: capabilityRowFieldForAir(row, "egress_class", m.AIR),
			Detail: capabilityRowFieldForAir(row, "blocker", m.AIR),
		})
	}
	if m.SessionDetail.Role != "" {
		es := m.SessionDetail.EvidenceSummary
		add(epistemicRow{
			Family: "session-evidence", Subject: sessionDetailFieldForAir(m.SessionDetail, "role", m.SessionDetail.Role, m.AIR),
			Status: m.SessionDetail.Readiness, Authority: "resume-preview only",
			Evidence: fmt.Sprintf("%d", es.Total), Source: sessionEvidenceKindSummary(m.SessionDetail),
			Freshness: fmt.Sprintf("roots observed=%d missing=%d", es.TranscriptRootsObserved, es.TranscriptRootsMissing),
			Privacy:   fmt.Sprintf("%s raw=%t", es.Privacy, es.RawAccess),
			Detail:    "metadata-only session work surface; no transcript/PTTY/stdin",
		})
	}
	return rows
}

func epistemicSourceToRow(src grammar.EpistemicSource, air bool) epistemicRow {
	return epistemicRow{
		Family:    "epistemics-source",
		Subject:   epistemicSourceFieldForAir(src, "id", air),
		Status:    epistemicSourceFieldForAir(src, "status", air),
		Authority: "typed-read",
		Evidence:  epistemicSourceFieldForAir(src, "count", air),
		Source:    "epistemics",
		Freshness: epistemicSourceFieldForAir(src, "age_bucket", air),
		Privacy:   fmt.Sprintf("%s raw=%s", epistemicSourceFieldForAir(src, "privacy", air), epistemicSourceFieldForAir(src, "raw_access", air)),
		Detail:    firstNonEmpty(epistemicSourceFieldForAir(src, "detail", air), epistemicSourceFieldForAir(src, "path", air)),
	}
}

func epistemicReadRowToViewRow(row grammar.EpistemicReadRow, air bool) epistemicRow {
	family := firstNonEmpty(epistemicReadRowFamily(row, air), "evidence")
	sourceRefs := epistemicReadRowFieldForAir(row, "source_refs", air)
	source := firstNonEmpty(sourceRefs, epistemicReadRowFieldForAir(row, "source", air))
	detail := strings.Join(nonEmptyParts([]string{
		labeledPart("posture", epistemicReadRowFieldForAir(row, "posture", air)),
		labeledPart("detail", epistemicReadRowFieldForAir(row, "detail", air)),
		labeledPart("missing", epistemicReadRowFieldForAir(row, "missing", air)),
		labeledPart("action", epistemicReadRowFieldForAir(row, "action", air)),
	}), " · ")
	return epistemicRow{
		Family:          family,
		Subject:         firstNonEmpty(epistemicReadRowFieldForAir(row, "subject_ref", air), epistemicReadRowFieldForAir(row, "map_id", air), epistemicReadRowFieldForAir(row, "subject", air), epistemicReadRowFieldForAir(row, "row_id", air)),
		Status:          epistemicReadRowFieldForAir(row, "status", air),
		Authority:       firstNonEmpty(epistemicReadRowFieldForAir(row, "authority_case", air), epistemicReadRowFieldForAir(row, "authority", air)),
		Evidence:        firstNonEmpty(epistemicReadRowFieldForAir(row, "evidence", air), epistemicReadRowFieldForAir(row, "evidence_count", air)),
		Source:          source,
		Freshness:       epistemicReadRowFieldForAir(row, "freshness", air),
		Privacy:         strings.Join(nonEmptyParts([]string{epistemicReadRowFieldForAir(row, "privacy", air), "raw=" + epistemicReadRowFieldForAir(row, "raw_access", air)}), " "),
		Detail:          detail,
		RowID:           epistemicReadRowFieldForAir(row, "row_id", air),
		SubjectKind:     epistemicReadRowFieldForAir(row, "subject_kind", air),
		SubjectRef:      epistemicReadRowFieldForAir(row, "subject_ref", air),
		Posture:         epistemicReadRowFieldForAir(row, "posture", air),
		SourceRefs:      sourceRefs,
		SourceRefLabels: epistemicReadRowSourceRefLabelsForAir(row, air),
		MapKind:         epistemicReadRowFieldForAir(row, "map_kind", air),
		MapID:           epistemicReadRowFieldForAir(row, "map_id", air),
		MapSource:       epistemicReadRowFieldForAir(row, "map_source", air),
		MapTarget:       epistemicReadRowFieldForAir(row, "map_target", air),
		MapRelation:     epistemicReadRowFieldForAir(row, "map_relation", air),
	}
}

// epistemicReadRowFamily is the AIR-aware family CATEGORY. The structural map-node/map-edge category is
// only revealed when its source kind field (subject_kind/map_kind) is AIR-ok; otherwise — and for every
// non-map row — the family is the AIR-GATED family field, never the raw value. (Root fix for the fugu
// Inc 3 review: the raw default leaked a denied family value through the posture-summary tally, the
// template/paste resolvers, and the emergent connector.)
func epistemicReadRowFamily(row grammar.EpistemicReadRow, air bool) string {
	kindOK := !air || row.AIR["subject_kind"] == "ok" || row.AIR["map_kind"] == "ok"
	if kindOK {
		switch strings.ToLower(strings.TrimSpace(firstNonEmpty(row.SubjectKind, row.MapKind))) {
		case "map-node", "node":
			return "map-node"
		case "map-edge", "edge":
			return "map-edge"
		}
	}
	return strings.TrimSpace(epistemicReadRowFieldForAir(row, "family", air))
}

func isPackageEpistemicRow(row grammar.EpistemicReadRow, er epistemicRow) bool {
	switch strings.ToLower(strings.TrimSpace(firstNonEmpty(row.SubjectKind, row.MapKind, er.SubjectKind, er.MapKind))) {
	case "package-row":
		return strings.TrimSpace(er.Family) != ""
	default:
		return false
	}
}

func epistemicSourceFieldForAir(src grammar.EpistemicSource, field string, air bool) string {
	var val string
	switch field {
	case "id":
		val = src.ID
	case "status":
		val = src.Status
	case "count":
		val = fmt.Sprintf("%d", src.Count)
	case "detail":
		val = src.Detail
	case "age_bucket":
		val = src.AgeBucket
	case "path":
		val = src.Path
	case "privacy":
		val = src.Privacy
	case "raw_access":
		val = fmt.Sprintf("%t", src.RawAccess)
	}
	return grammar.Redact(src.AIR, field, val, air)
}

func epistemicReadRowFieldForAir(row grammar.EpistemicReadRow, field string, air bool) string {
	var val string
	switch field {
	case "row_id":
		val = row.RowID
	case "family":
		val = row.Family
	case "subject_kind":
		val = row.SubjectKind
	case "subject_ref":
		val = row.SubjectRef
	case "subject":
		val = row.Subject
	case "status":
		val = row.Status
	case "posture":
		val = row.Posture
	case "authority":
		val = row.Authority
	case "authority_case":
		val = row.AuthorityCase
	case "evidence_count":
		val = fmt.Sprintf("%d", row.EvidenceCount)
	case "evidence":
		val = row.Evidence
	case "source":
		val = row.Source
	case "source_refs":
		val = row.SourceRefs
	case "freshness":
		val = row.Freshness
	case "privacy":
		val = row.Privacy
	case "raw_access":
		val = fmt.Sprintf("%t", row.RawAccess)
	case "missing":
		val = row.Missing
	case "action":
		val = row.Action
	case "detail":
		val = row.Detail
	case "map_kind":
		val = row.MapKind
	case "map_id":
		val = row.MapID
	case "map_source":
		val = row.MapSource
	case "map_target":
		val = row.MapTarget
	case "map_relation":
		val = row.MapRelation
	}
	return grammar.Redact(row.AIR, field, val, air)
}

func epistemicReadRowSourceRefLabelsForAir(row grammar.EpistemicReadRow, air bool) []string {
	if air && row.AIR["source_ref_labels"] != "ok" {
		return nil
	}
	out := make([]string, 0, len(row.SourceRefLabels))
	for _, label := range row.SourceRefLabels {
		label = strings.TrimSpace(label)
		if label != "" {
			out = append(out, label)
		}
	}
	return out
}

func epistemicStatusToken(status, detail string) string {
	s := strings.ToLower(strings.TrimSpace(status + " " + detail))
	switch {
	case strings.Contains(s, "missing"), strings.Contains(s, "dark"), strings.Contains(s, "blocked"), strings.Contains(s, "failed"), strings.Contains(s, "stale"), strings.Contains(s, "forbidden"):
		return "red"
	case strings.Contains(s, "candidate"), strings.Contains(s, "preview"), strings.Contains(s, "support"), strings.Contains(s, "unknown"), strings.Contains(s, "incomplete"), strings.Contains(s, "warn"):
		return "yel"
	case strings.Contains(s, "observed"), strings.Contains(s, "fresh"), strings.Contains(s, "present"), strings.Contains(s, "declared"), strings.Contains(s, "active"), strings.Contains(s, "lossless"):
		return "grn"
	default:
		return "2nd"
	}
}

func (m Model) epistemicFocusScrollOffset(i int) int {
	if i < 0 {
		return 0
	}
	return 16 + i
}

func (m Model) renderEpistemicsProjection(w int) string {
	rows := m.epistemicRows()
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	writeSectionHeader(&b, w, "EPISTEMICS", "live contextualizer over claims, observations, validation, provenance, and authority ceilings", "metadata-only evidence")
	writeWrappedKV(&b, "contract", "source-backed when /read/epistemics is live; derived from existing read folds as fallback; no raw transcript, vault note body, source body, or dispatch authority", "yel", w)
	writeWrappedKV(&b, "sources", fmt.Sprintf("epistemics:%d r%d · dynamics:%d · intake:%d · domains:%d · lifecycles:%d · caps:%d · session-detail:%t",
		len(m.Epistemics.Rows)+len(m.Epistemics.Sources), m.EpistemicsSeq,
		len(m.Dynamics.Package.Sources)+len(m.Dynamics.Package.Claims)+len(m.Dynamics.Package.Observations)+len(m.Dynamics.Package.Validation)+len(m.Dynamics.Package.Relations),
		len(m.Intake.Sources)+len(m.Intake.Rows), len(m.Domains.Sources)+len(m.Domains.Rows), len(m.Domains.LifecycleSources)+len(m.Domains.Lifecycles), len(m.Capabilities.Rows), m.SessionDetail.Role != ""), "2nd", w)
	writeWrappedKV(&b, "why", "every operational or research claim should expose what supports it, what it cannot prove, and what next evidence would promote it", "org", w)
	writeWrappedKV(&b, "next", "cursorful dynamics nodes/edges should jump here by subject; later rows can promote to source excerpts only through AIR and authority policy", "mut", w)
	b.WriteString(rule + "\n")
	if len(rows) == 0 {
		writeSectionHeader(&b, w, "NO EPISTEMIC ROWS", "all upstream read folds are empty or dark", "read dark")
		writeWrappedKV(&b, "next evidence", "load dynamics/intake/domain/capability sources; Reins will derive posture rows without a new endpoint", "yel", w)
		return b.String()
	}
	idx := clamp(m.EpiFocus, 0, len(rows)-1)
	b.WriteString(renderEpistemicsPostureSummary(rows, idx, w))
	b.WriteString(rule + "\n")
	if !m.SuppressSplitPinned {
		b.WriteString(m.renderSelectedEpistemicPath(w))
		b.WriteString(rule + "\n")
	}
	rowControl := "[j/k] moves the row; rows are derived projections, not source excerpts"
	if m.sessionSplit() {
		rowControl = "[n/p] moves the evidence target; rows are derived projections, not source excerpts"
	}
	writeSectionHeader(&b, w, "POSTURE ROWS", rowControl, fmt.Sprintf("%d rows", len(rows)))
	for i, row := range rows {
		line := epistemicRowLine(row, i == idx, w)
		if i == idx {
			b.WriteString(focusBar(line, w) + "\n")
		} else {
			b.WriteString(fitWidth(line, w) + "\n")
		}
	}
	return b.String()
}

// epistemicListBody is the PRIMARY pane of the algebra-composed epistemics page (Inc 3 TRANSFORM): the
// posture summary + the windowed posture rows. The focused row's full evidence path is the SECONDARY
// pane (renderSelectedEpistemicPath), so this body omits the embedded pinned path. Self-anchored —
// [j/k] moves the row (EpiFocus) natively now that the page is no longer session-frozen.
func (m Model) epistemicListBody(w, h int) string {
	rows := m.epistemicRows()
	var b strings.Builder
	writeSectionHeader(&b, w, "EPISTEMICS", "live contextualizer over claims, observations, validation, provenance, and authority ceilings", "metadata-only evidence")
	// The privacy/legibility contract stays in the primary — it tells the operator WHAT this page is
	// (derived projections, metadata-only) and what it never exposes (raw transcript/source/authority).
	writeWrappedKV(&b, "contract", "source-backed when /read/epistemics is live; derived from existing read folds as fallback; no raw transcript, vault note body, source body, or dispatch authority", "yel", w)
	if len(rows) == 0 {
		writeWrappedKV(&b, "next evidence", "load dynamics/intake/domain/capability sources; Reins derives posture rows without a new endpoint", "yel", w)
		return b.String()
	}
	idx := clamp(m.EpiFocus, 0, len(rows)-1)
	b.WriteString(renderEpistemicsPostureSummary(rows, idx, w))
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	b.WriteString(rule + "\n")
	writeSectionHeader(&b, w, "POSTURE ROWS", "[j/k] moves the row; rows are derived projections, not source excerpts", fmt.Sprintf("%d rows", len(rows)))
	// window the rows so the focused row stays visible within the pane height
	used := strings.Count(b.String(), "\n")
	visible := h - used - 1
	if visible < 1 {
		visible = 1
	}
	off := 0
	if len(rows) > visible {
		off = clamp(idx-visible/2, 0, len(rows)-visible)
	}
	brushed := m.brushedEpistemics()
	for i := off; i < off+visible && i < len(rows); i++ {
		switch {
		case i == idx:
			line := epistemicRowLine(rows[i], true, w)
			b.WriteString(focusBar(line, w) + "\n")
		case brushed[epistemicEntity(rows[i]).ID]:
			// brushed: shares the focused posture row's strongest emergent facet (├ decoded by the connector)
			line := epistemicRowLineWithMark(rows[i], "├", "2nd", w)
			b.WriteString(fitWidth(line, w) + "\n")
		default:
			line := epistemicRowLine(rows[i], false, w)
			b.WriteString(fitWidth(line, w) + "\n")
		}
	}
	return b.String()
}

type epistemicPostureSummary struct {
	Observed, Support, Gaps, Neutral int
	Families, Authorities, Privacy   map[string]int
}

func summarizeEpistemicRows(rows []epistemicRow) epistemicPostureSummary {
	s := epistemicPostureSummary{
		Families:    map[string]int{},
		Authorities: map[string]int{},
		Privacy:     map[string]int{},
	}
	for _, row := range rows {
		// Skip AIR-redacted dimensions: a denied family/authority/privacy must not become a "▒▒▒:N"
		// cardinality bucket (nor — for the now-AIR-projected family — its raw value). The hue tally
		// (below) is derived from the redaction-defaulted token, so it discloses no value.
		if fam := firstNonEmpty(row.Family, "unknown"); !strings.Contains(fam, "▒") {
			s.Families[fam]++
		}
		if auth := firstNonEmpty(row.Authority, "support"); !strings.Contains(auth, "▒") {
			s.Authorities[auth]++
		}
		if !strings.Contains(row.Privacy, "▒") {
			s.Privacy[epistemicPrivacyClass(row.Privacy)]++
		}
		switch row.Token {
		case "grn":
			s.Observed++
		case "yel":
			s.Support++
		case "red":
			s.Gaps++
		default:
			s.Neutral++
		}
	}
	return s
}

func renderEpistemicsPostureSummary(rows []epistemicRow, selectedIdx, w int) string {
	summary := summarizeEpistemicRows(rows)
	selected := rows[clamp(selectedIdx, 0, len(rows)-1)]
	var b strings.Builder
	writeSectionHeader(&b, w, "POSTURE SUMMARY", "families, evidence health, authority ceilings", fmt.Sprintf("%d rows", len(rows)))
	writeWrappedKV(&b, "posture", fmt.Sprintf("observed:%d · support:%d · gaps:%d · neutral:%d", summary.Observed, summary.Support, summary.Gaps, summary.Neutral), countWarnToken(summary.Gaps), w)
	if len(summary.Families) > 0 {
		writeWrappedKV(&b, "families", compactCountMap(summary.Families, 7), "2nd", w)
	}
	if len(summary.Authorities) > 0 {
		writeWrappedKV(&b, "authority", compactCountMap(summary.Authorities, 5), "yel", w)
	}
	if len(summary.Privacy) > 0 {
		writeWrappedKV(&b, "privacy", compactCountMap(summary.Privacy, 4), "mut", w)
	}
	writeWrappedKV(&b, "selected", fmt.Sprintf("%d/%d · %s · %s", selectedIdx+1, len(rows), selected.Family, selected.Subject), selected.Token, w)
	writeWrappedKV(&b, "key", "status hue = observed/support/gap/neutral; authority is ceiling, not permission; evidence count is metadata, not source body", "org", w)
	writeWrappedKV(&b, "next proof", epistemicNextEvidence(summary), "pri", w)
	return b.String()
}

func epistemicPrivacyClass(privacy string) string {
	p := strings.ToLower(strings.TrimSpace(privacy))
	switch {
	case p == "":
		return "metadata-only"
	case strings.Contains(p, "metadata unavailable"):
		return "unavailable"
	case strings.Contains(p, "raw=true"):
		return "raw-declared"
	case strings.Contains(p, "metadata-only"), strings.Contains(p, "raw=false"):
		return "metadata-only"
	case strings.Contains(p, "redact"):
		return "redacted"
	default:
		return "scoped"
	}
}

func epistemicNextEvidence(summary epistemicPostureSummary) string {
	switch {
	case summary.Gaps > 0:
		return "resolve red rows first: dark reads, missing descriptors, stale proof, or blocked source posture"
	case summary.Support > 0:
		return "promote yellow support rows with source verification, validation receipt, or authority-specific evidence"
	case summary.Neutral > 0:
		return "classify neutral rows so posture is explicit before they influence routing or interpretation"
	default:
		return "keep observed rows fresh; refresh receipts before treating the surface as current"
	}
}

func (m Model) renderSelectedEpistemicPath(w int) string {
	rows := m.epistemicRows()
	if len(rows) == 0 {
		return ""
	}
	idx := clamp(m.EpiFocus, 0, len(rows)-1)
	selected := rows[idx]
	var b strings.Builder
	controls := "[j/k] evidence focus · [J/K] scroll"
	if m.sessionSplit() {
		controls = m.splitNavHint(m.splitRelation(), false)
	}
	writeSectionHeader(&b, w, "SELECTED EVIDENCE PATH", fmt.Sprintf("row %d/%d; evidence, not authority", idx+1, len(rows)), selected.Family)
	writeWrappedKV(&b, "controls", controls, "yel", w)
	writeWrappedKV(&b, "subject", selected.Subject, selected.Token, w)
	writeWrappedKV(&b, "family", selected.Family, "org", w)
	writeWrappedKV(&b, "status", selected.Status, selected.Token, w)
	writeWrappedKV(&b, "authority", selected.Authority, "yel", w)
	writeWrappedKV(&b, "evidence", selected.Evidence+" · source="+selected.Source, "2nd", w)
	if strings.TrimSpace(selected.SourceRefs) != "" {
		writeWrappedKV(&b, "source refs", selected.SourceRefs, "2nd", w)
	}
	if refs := dynamicsSourceRefLabelSummary(selected.SourceRefLabels); refs != "" {
		writeWrappedKV(&b, "ref labels", refs, "blu", w)
	}
	writeWrappedKV(&b, "freshness", selected.Freshness, epistemicFreshnessToken(selected.Freshness), w)
	writeWrappedKV(&b, "privacy", selected.Privacy, "mut", w)
	writeWrappedKV(&b, "detail", selected.Detail, "mut", w)
	return b.String()
}

type dynamicsFocusRow struct {
	Kind, ID, Label, Status, Source, Relation, Detail, Token string
	Summary, Context, Docs, Hardening, Tags, Confidence      string
	RawID, RawSource, RawTarget, RawRelation                 string
	SourceRefLabels                                          []string
	MatchKeys                                                []string
}

func (m Model) dynamicsColumnRailFocus() grammar.ColumnRailFocus {
	row, ok := m.FocusedDynamicsElement()
	if !ok {
		return grammar.ColumnRailFocus{}
	}
	return grammar.ColumnRailFocus{
		Kind:     row.Kind,
		ID:       row.RawID,
		Source:   row.RawSource,
		Target:   row.RawTarget,
		Relation: row.RawRelation,
	}
}

func (m Model) dynamicsFocusRows() []dynamicsFocusRow {
	w := m.Width
	if w < 40 {
		w = 120
	}
	scale := m.dynamicsRenderScaleValue(w)
	g := m.Dynamics.AtResolution(scale)
	rows := make([]dynamicsFocusRow, 0, len(g.Nodes)+len(g.Edges)+len(m.Dynamics.Package.Sources))
	add := func(row dynamicsFocusRow) {
		if strings.TrimSpace(row.Kind) == "" {
			row.Kind = "element"
		}
		if strings.TrimSpace(row.ID) == "" {
			row.ID = "·"
		}
		if strings.TrimSpace(row.Status) == "" {
			row.Status = "unknown"
		}
		if strings.TrimSpace(row.Token) == "" {
			row.Token = dynamicsStatusToken(row.Status)
		}
		rows = append(rows, row)
	}
	for _, n := range g.Nodes {
		id := dynamicsNodeFieldForAir(n, "id", m.AIR)
		label := dynamicsNodeFieldForAir(n, "label", m.AIR)
		layer := dynamicsNodeFieldForAir(n, "layer", m.AIR)
		kind := dynamicsNodeFieldForAir(n, "kind", m.AIR)
		status := dynamicsNodeFieldForAir(n, "status", m.AIR)
		res := dynamicsNodeFieldForAir(n, "res", m.AIR)
		summary := dynamicsNodeFieldForAir(n, "summary", m.AIR)
		context := dynamicsNodeFieldForAir(n, "context", m.AIR)
		docs := dynamicsNodeFieldForAir(n, "docs", m.AIR)
		sourceRefs := dynamicsNodeSourceRefLabelsForAir(n, m.AIR)
		hardening := dynamicsNodeFieldForAir(n, "hardening_notes", m.AIR)
		tags := dynamicsNodeFieldForAir(n, "tags", m.AIR)
		add(dynamicsFocusRow{
			Kind:            "node",
			ID:              id,
			RawID:           n.ID,
			Label:           label,
			Status:          status,
			Source:          layer,
			Summary:         summary,
			Context:         context,
			Docs:            docs,
			Hardening:       hardening,
			Tags:            tags,
			SourceRefLabels: sourceRefs,
			Detail: strings.Join(nonEmptyParts([]string{
				"label=" + label,
				"layer=" + layer,
				"kind=" + kind,
				"res=" + res,
				labeledPart("source_refs", dynamicsSourceRefLabelSummary(sourceRefs)),
			}), " · "),
			MatchKeys: []string{n.ID, n.Label, "node:" + n.Status, n.Kind + ":" + n.Status},
		})
	}
	for _, e := range g.Edges {
		edgeID := dynamicsEdgeFieldForAir(e, "id", m.AIR)
		source := dynamicsEdgeFieldForAir(e, "source", m.AIR)
		target := dynamicsEdgeFieldForAir(e, "target", m.AIR)
		relation := dynamicsEdgeFieldForAir(e, "relation", m.AIR)
		status := dynamicsEdgeFieldForAir(e, "status", m.AIR)
		layer := dynamicsEdgeFieldForAir(e, "layer", m.AIR)
		res := dynamicsEdgeFieldForAir(e, "res", m.AIR)
		confidence := dynamicsEdgeFieldForAir(e, "confidence", m.AIR)
		summary := dynamicsEdgeFieldForAir(e, "summary", m.AIR)
		docs := dynamicsEdgeFieldForAir(e, "docs", m.AIR)
		sourceRefs := dynamicsEdgeSourceRefLabelsForAir(e, m.AIR)
		id := firstNonEmpty(edgeID, strings.Join(nonEmptyParts([]string{source, target}), "→"))
		add(dynamicsFocusRow{
			Kind:            "edge",
			ID:              id,
			RawID:           e.ID,
			RawSource:       e.Source,
			RawTarget:       e.Target,
			RawRelation:     e.Relation,
			Label:           relation,
			Status:          status,
			Source:          source,
			Relation:        relation,
			Summary:         summary,
			Docs:            docs,
			Confidence:      confidence,
			SourceRefLabels: sourceRefs,
			Detail: strings.Join(nonEmptyParts([]string{
				"source=" + source,
				"target=" + target,
				"relation=" + relation,
				"layer=" + layer,
				"res=" + res,
				labeledPart("source_refs", dynamicsSourceRefLabelSummary(sourceRefs)),
			}), " · "),
			MatchKeys: []string{
				e.ID, e.Source, e.Target, e.Relation,
				"edge:" + e.Status,
				e.Relation + ":" + e.Status,
			},
		})
	}
	for _, src := range m.Dynamics.Package.Sources {
		id := dynamicsSourceFieldForAir(src, "id", m.AIR)
		status := dynamicsSourceFieldForAir(src, "status", m.AIR)
		count := dynamicsSourceFieldForAir(src, "count", m.AIR)
		age := dynamicsSourceFieldForAir(src, "age_bucket", m.AIR)
		detail := dynamicsSourceFieldForAir(src, "detail", m.AIR)
		add(dynamicsFocusRow{
			Kind:      "source",
			ID:        id,
			RawID:     src.ID,
			Status:    status,
			Source:    "dynamics/package",
			Detail:    strings.Join(nonEmptyParts([]string{"count=" + count, "age=" + age, detail}), " · "),
			MatchKeys: []string{src.ID, src.Detail, src.Status},
		})
	}
	return rows
}

func (m Model) dynamicsCompactFirstViewport() bool {
	return !m.sessionSplit() && !m.SuppressSplitPinned && m.Height < 65
}

func (m Model) renderDynamicsGraphRail(w, scale int) string {
	var b strings.Builder
	b.WriteString(" " + grammar.C("brt", "GRAPH RAIL") + grammar.C("mut", " — topology follows selected context; arrows show dependency/governance direction") + "\n")
	rail := grammar.RenderColumnRailFrameFocused(m.Dynamics, scale, m.AIR, w, m.Beat, m.dynamicsColumnRailFocus())
	b.WriteString(rail)
	if !strings.HasSuffix(rail, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) renderDynamicsSelectedElementCompact(w int) string {
	rows := m.dynamicsFocusRows()
	if len(rows) == 0 {
		return ""
	}
	idx := clamp(m.DynFocus, 0, len(rows)-1)
	row := rows[idx]
	var b strings.Builder
	writeSectionHeader(&b, w, "SELECTED MAP ELEMENT", fmt.Sprintf("compact row %d/%d; [j/k] focus, [E]/[Enter] epistemics", idx+1, len(rows)), row.Kind)
	writeWrappedKV(&b, "id", row.ID, row.Token, w)
	if strings.TrimSpace(row.Label) != "" {
		writeWrappedKV(&b, "label", row.Label, "pri", w)
	}
	writeWrappedKV(&b, "status", row.Status, row.Token, w)
	writeWrappedKV(&b, "source", row.Source, "2nd", w)
	if strings.TrimSpace(row.Relation) != "" {
		writeWrappedKV(&b, "relation", row.Relation, "blu", w)
	}
	if strings.TrimSpace(row.Confidence) != "" {
		writeWrappedKV(&b, "confidence", row.Confidence, dynamicsConfidenceToken(row.Confidence), w)
	}
	if strings.TrimSpace(row.Summary) != "" {
		writeWrappedKV(&b, "summary", row.Summary, "pri", w)
	}
	if strings.TrimSpace(row.Docs) != "" {
		writeWrappedKV(&b, "refs", row.Docs, "blu", w)
	}
	if refs := dynamicsSourceRefLabelSummary(row.SourceRefLabels); refs != "" {
		writeWrappedKV(&b, "source refs", refs, "blu", w)
	}
	if m.Height >= 44 {
		b.WriteString(m.renderDynamicsEpistemicBridge(row, w))
	} else {
		b.WriteString(m.renderDynamicsCompactBridge(row, w))
	}
	if neighborhood := m.renderDynamicsNeighborhood(row, w); neighborhood != "" {
		b.WriteString(neighborhood)
	}
	return b.String()
}

// dynamicsMapBody is the PRIMARY pane of the algebra-composed dynamics page (Inc 3 TRANSFORM): the
// navigable map document — header + scale + (compact-preview) + graph rail + workbench/summary/guide/
// package orientation. The FULL focused-element detail is the SECONDARY (renderDynamicsSelectedElement),
// so the full inline element is omitted here (the compactFirst path keeps only the compact preview, for
// map context). Height-parameterized (the pane height drives the primer gate), unlike the m.Height-
// coupled referenceContent it was extracted from.
func (m Model) dynamicsMapBody(w, h int) string {
	return m.dynamicsMapWindow(m.dynamicsMapDocumentLines(w, h), w, h)
}

func (m Model) dynamicsMapDocumentLines(w, h int) []string {
	s := strings.TrimRight(m.dynamicsMapDocument(w, h), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func (m Model) dynamicsMapWindow(lines []string, w, h int) string {
	if len(lines) == 0 || h <= 0 {
		return ""
	}
	maxOff := len(lines) - h
	if maxOff < 0 {
		maxOff = 0
	}
	off := clamp(m.RefScroll, 0, maxOff)
	end := off + h
	if end > len(lines) {
		end = len(lines)
	}
	window := append([]string(nil), lines[off:end]...)
	if len(window) == 0 {
		return ""
	}
	above := off
	below := len(lines) - end
	indicator := func(s string) string {
		return fitWidth(" "+grammar.C("mut", s), w)
	}
	if above > 0 && below > 0 && len(window) == 1 {
		window[0] = indicator(fmt.Sprintf("↑%d more · ↓%d more", above, below))
	} else {
		if above > 0 {
			window[0] = indicator(fmt.Sprintf("↑%d more", above))
		}
		if below > 0 {
			window[len(window)-1] = indicator(fmt.Sprintf("↓%d more", below))
		}
	}
	return strings.Join(window, "\n")
}

func (m Model) dynamicsMapDocument(w, h int) string {
	if m.DynamicsDark {
		return m.contextLine() + "\n" + darkHint(m.DynamicsError, m.AIR)
	}
	scale, scaleLabel := m.dynamicsRenderScale(w)
	var b strings.Builder
	b.WriteString(grammar.DynamicsHeader(m.Dynamics, w))
	b.WriteString(" " + grammar.C("mut", "scale ") + grammar.C("yel", scaleLabel) +
		grammar.C("mut", " · [,/.] cycle overview/domain/artifact/runtime/evidence/all") + "\n")
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	b.WriteString(rule + "\n")
	if m.dynamicsCompactFirstViewport() {
		if h >= 45 {
			b.WriteString(m.renderDynamicsPrimer(w))
			b.WriteString(rule + "\n")
		}
		b.WriteString(m.renderDynamicsSelectedElementCompact(w)) // compact preview stays in the map for context
		b.WriteString(rule + "\n")
		b.WriteString(m.renderDynamicsGraphRail(w, scale))
		b.WriteString(rule + "\n")
	} else {
		if h >= 45 {
			b.WriteString(m.renderDynamicsPrimer(w))
			b.WriteString(rule + "\n")
		}
		b.WriteString(m.renderDynamicsGraphRail(w, scale)) // full inline element omitted — it is the secondary
		b.WriteString(rule + "\n")
	}
	if workbench := m.renderDynamicsWorkbench(w); strings.TrimSpace(workbench) != "" {
		b.WriteString(workbench)
		b.WriteString(rule + "\n")
	}
	if summary := m.renderDynamicsGraphSummary(w, m.dynamicsRenderScaleValue(w)); strings.TrimSpace(summary) != "" {
		b.WriteString(summary)
		b.WriteString(rule + "\n")
	}
	b.WriteString(m.renderDynamicsGuide(w))
	b.WriteString(m.renderDynamicsPackage(w))
	b.WriteString(m.renderDynamicsPackageDetails(w))
	return b.String()
}

func (m Model) renderDynamicsSelectedElement(w int) string {
	rows := m.dynamicsFocusRows()
	if len(rows) == 0 {
		return ""
	}
	idx := clamp(m.DynFocus, 0, len(rows)-1)
	row := rows[idx]
	var b strings.Builder
	control := "[j/k] focus, [J/K] scroll, [E]/[Enter] epistemics"
	if m.sessionSplit() {
		control = "lane anchor active; [n/p] map focus, [J/K] scroll, [E] epistemics"
	}
	writeSectionHeader(&b, w, "SELECTED MAP ELEMENT", fmt.Sprintf("row %d/%d; %s", idx+1, len(rows), control), row.Kind)
	writeWrappedKV(&b, "controls", control, "yel", w)
	writeWrappedKV(&b, "id", row.ID, row.Token, w)
	if strings.TrimSpace(row.Label) != "" {
		writeWrappedKV(&b, "label", row.Label, "pri", w)
	}
	writeWrappedKV(&b, "status", row.Status, row.Token, w)
	writeWrappedKV(&b, "source", row.Source, "2nd", w)
	if strings.TrimSpace(row.Relation) != "" {
		writeWrappedKV(&b, "relation", row.Relation, "blu", w)
	}
	if strings.TrimSpace(row.Confidence) != "" {
		writeWrappedKV(&b, "confidence", row.Confidence, dynamicsConfidenceToken(row.Confidence), w)
	}
	if strings.TrimSpace(row.Summary) != "" {
		writeWrappedKV(&b, "summary", row.Summary, "pri", w)
	}
	if strings.TrimSpace(row.Context) != "" {
		writeWrappedKV(&b, "why", row.Context, "2nd", w)
	}
	if strings.TrimSpace(row.Docs) != "" {
		writeWrappedKV(&b, "refs", row.Docs, "blu", w)
	}
	if refs := dynamicsSourceRefLabelSummary(row.SourceRefLabels); refs != "" {
		writeWrappedKV(&b, "source refs", refs, "blu", w)
	}
	if strings.TrimSpace(row.Hardening) != "" {
		writeWrappedKV(&b, "hardening", row.Hardening, "yel", w)
	}
	if strings.TrimSpace(row.Tags) != "" {
		writeWrappedKV(&b, "tags", row.Tags, "mut", w)
	}
	writeWrappedKV(&b, "detail", row.Detail, "mut", w)
	if m.sessionSplit() {
		b.WriteString(m.renderDynamicsReferencePaths(row, w))
	}
	if neighborhood := m.renderDynamicsNeighborhood(row, w); neighborhood != "" {
		b.WriteString(neighborhood)
	}
	if m.sessionSplit() || m.Height >= 44 {
		b.WriteString(m.renderDynamicsEpistemicBridge(row, w))
	} else {
		b.WriteString(m.renderDynamicsCompactBridge(row, w))
	}
	return b.String()
}

type loopEdge struct {
	SourceID, TargetID string
	Source, Target     string
	Relation           string
	Sign               graph.Sign
	Delay              bool
	Prov               graph.Provenance
}

type loopRow struct {
	Nodes        []string
	DisplayNodes []string
	Edges        []loopEdge
	Kind         graph.LoopKind
	HasDelay     bool
	Fallback     bool
	NodeCount    int
	EdgeCount    int
}

func loopFixtureGraph() grammar.Graph {
	air := map[string]string{"id": "ok", "label": "ok", "kind": "ok", "layer": "ok", "status": "ok", "res": "ok"}
	edgeAir := map[string]string{"source": "ok", "target": "ok", "relation": "ok", "status": "ok", "sign": "ok", "delay": "ok", "prov": "ok"}
	return grammar.Graph{
		MapID:  "loops-fixture",
		Thesis: "offline causal-loop fixture used only when :dynamics is empty",
		Nodes: []grammar.Node{
			{ID: "attention", Label: "Operator attention", Kind: "stock", Layer: "system", Status: "asserted", Res: "1", AIR: air},
			{ID: "coordination", Label: "Coordination", Kind: "flow", Layer: "system", Status: "asserted", Res: "1", AIR: air},
			{ID: "work-clarity", Label: "Work clarity", Kind: "stock", Layer: "system", Status: "asserted", Res: "1", AIR: air},
		},
		Edges: []grammar.Edge{
			{Source: "attention", Target: "coordination", Relation: "enables", Sign: "+", Status: "inferred", AIR: edgeAir},
			{Source: "coordination", Target: "work-clarity", Relation: "feeds", Sign: "+", Status: "inferred", AIR: edgeAir},
			{Source: "work-clarity", Target: "attention", Relation: "supports", Sign: "+", Status: "inferred", AIR: edgeAir},
		},
	}
}

func (m Model) dynamicsGraphForLoops() (grammar.Graph, bool) {
	if len(m.Dynamics.Nodes) == 0 && len(m.Dynamics.Edges) == 0 {
		return loopFixtureGraph(), true
	}
	return m.Dynamics, false
}

func graphSignFromDynamics(e grammar.Edge) graph.Sign {
	switch strings.ToLower(strings.TrimSpace(e.Sign)) {
	case "+", "pos", "positive", "plus", "1":
		return graph.SignPos
	case "-", "neg", "negative", "minus", "-1":
		return graph.SignNeg
	case "?", "unknown", "indeterminate":
		return graph.SignUnknown
	}
	return graph.InferSign(e.Relation)
}

func graphProvFromDynamics(e grammar.Edge) graph.Provenance {
	switch strings.ToLower(strings.TrimSpace(firstNonEmpty(e.Prov, e.Status))) {
	case string(graph.Inferred):
		return graph.Inferred
	case string(graph.Derived):
		return graph.Derived
	case string(graph.Asserted), "observed":
		return graph.Asserted
	}
	return ""
}

func dynamicsEdgeDelay(e grammar.Edge) bool {
	if e.Delay {
		return true
	}
	needle := strings.ToLower(strings.Join([]string{e.Relation, e.Summary, e.Docs}, " "))
	return strings.Contains(needle, "delay") || strings.Contains(needle, "latency") || strings.Contains(needle, "lag")
}

func (m Model) loopRows() []loopRow {
	dg, fallback := m.dynamicsGraphForLoops()
	tg := graph.New()
	for _, e := range dg.Edges {
		if strings.TrimSpace(e.Source) == "" || strings.TrimSpace(e.Target) == "" {
			continue
		}
		tg.Add(graph.Relation{
			Src:   e.Source,
			Dst:   e.Target,
			Type:  e.Relation,
			Sign:  graphSignFromDynamics(e),
			Delay: dynamicsEdgeDelay(e),
			Prov:  graphProvFromDynamics(e),
		})
	}
	nodes := map[string]grammar.Node{}
	for _, n := range dg.Nodes {
		nodes[n.ID] = n
	}
	relations := map[[2]string]graph.Relation{}
	for _, e := range tg.Edges {
		key := [2]string{e.Src, e.Dst}
		if _, exists := relations[key]; !exists {
			relations[key] = e
		}
	}
	rawEdges := map[[2]string]grammar.Edge{}
	for _, e := range dg.Edges {
		key := [2]string{e.Source, e.Target}
		if _, exists := rawEdges[key]; !exists {
			rawEdges[key] = e
		}
	}
	displayNode := func(id string) string {
		if n, ok := nodes[id]; ok {
			return firstNonEmpty(dynamicsNodeFieldForAir(n, "id", m.AIR), "▒▒▒")
		}
		if m.AIR {
			return "▒▒▒"
		}
		return firstNonEmpty(id, "·")
	}
	out := []loopRow{}
	for _, lp := range tg.CausalLoops() {
		row := loopRow{
			Nodes:     append([]string(nil), lp.Nodes...),
			Kind:      lp.Kind,
			HasDelay:  lp.HasDelay,
			Fallback:  fallback,
			NodeCount: len(dg.Nodes),
			EdgeCount: len(dg.Edges),
		}
		for _, id := range lp.Nodes {
			row.DisplayNodes = append(row.DisplayNodes, displayNode(id))
		}
		for i := range lp.Nodes {
			src, dst := lp.Nodes[i], lp.Nodes[(i+1)%len(lp.Nodes)]
			key := [2]string{src, dst}
			rel := relations[key]
			raw := rawEdges[key]
			relation := rel.Type
			if strings.TrimSpace(raw.Relation) != "" {
				relation = dynamicsEdgeFieldForAir(raw, "relation", m.AIR)
			}
			row.Edges = append(row.Edges, loopEdge{
				SourceID: src,
				TargetID: dst,
				Source:   displayNode(src),
				Target:   displayNode(dst),
				Relation: relation,
				Sign:     rel.Sign,
				Delay:    rel.Delay,
				Prov:     rel.Prov,
			})
		}
		out = append(out, row)
	}
	return out
}

func loopKindGlyph(kind graph.LoopKind) string {
	switch kind {
	case graph.Reinforcing:
		return "⟳R"
	case graph.Balancing:
		return "⊖B"
	default:
		return "◌?"
	}
}

func loopKindToken(kind graph.LoopKind) string {
	switch kind {
	case graph.Reinforcing:
		return "grn"
	case graph.Balancing:
		return "yel"
	default:
		return "mut"
	}
}

func loopKindWord(kind graph.LoopKind) string {
	switch kind {
	case graph.Reinforcing:
		return "Reinforcing"
	case graph.Balancing:
		return "Balancing"
	default:
		return "Indeterminate"
	}
}

func signGlyph(s graph.Sign) string {
	switch s {
	case graph.SignPos:
		return "+"
	case graph.SignNeg:
		return "−"
	default:
		return "?"
	}
}

func loopDominantSign(kind graph.LoopKind) string {
	switch kind {
	case graph.Reinforcing:
		return "+"
	case graph.Balancing:
		return "−"
	default:
		return "?"
	}
}

func (m Model) loopListBody(w, h int) string {
	rows := m.loopRows()
	dg, fallback := m.dynamicsGraphForLoops()
	source := "live :dynamics"
	if fallback {
		source = "fixture fallback (:dynamics empty)"
	}
	header := []string{
		" " + grammar.C("brt", "CAUSAL LOOPS") + grammar.C("mut", fmt.Sprintf(" — A5 Tier-1 over %s · %d nodes · %d edges", source, len(dg.Nodes), len(dg.Edges))),
		" " + grammar.C("mut", "Reinforcing loops amplify change; Balancing loops counteract change. Legend: ") + grammar.C("grn", "⟳R") + grammar.C("mut", " reinforcing · ") + grammar.C("yel", "⊖B") + grammar.C("mut", " balancing · computed, no simulation."),
		" " + grammar.C("mut", "TYPE is sign parity around the directed cycle: even negative links => R; odd negative links => B. Color is redundant; shape+position carry type."),
		" " + grammar.C("border", strings.Repeat("─", maxVisible(10, w-2))),
	}
	visible := h - len(header)
	if visible < 1 {
		visible = 1
	}
	start := 0
	if len(rows) > visible {
		if m.LoopFocus >= visible {
			start = m.LoopFocus - visible + 1
		}
		if mx := len(rows) - visible; start > mx {
			start = mx
		}
	}
	var b strings.Builder
	for _, line := range header {
		b.WriteString(fitWidth(line, w) + "\n")
	}
	if len(rows) == 0 {
		b.WriteString(" " + grammar.C("mut", "no causal loops in the current graph — acyclic or single-node") + "\n")
		return strings.TrimRight(b.String(), "\n")
	}
	for i := start; i < start+visible && i < len(rows); i++ {
		row := rows[i]
		delay := ""
		if row.HasDelay {
			delay = " ‖"
		}
		path := strings.Join(row.DisplayNodes, " → ")
		line := fmt.Sprintf("%s%s  len=%-2d sign=%s  %s", loopKindGlyph(row.Kind), delay, len(row.Nodes), loopDominantSign(row.Kind), path)
		line = clipRunes(line, maxVisible(8, w-2))
		if i == m.LoopFocus {
			b.WriteString(grammar.C("yel", m.focusGlyph()) + focusBar(line, w-1) + "\n")
		} else {
			b.WriteString("  " + grammar.C(loopKindToken(row.Kind), line) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) axisListBody(w, h int) string {
	rows := grammar.Axes()
	header := []string{
		" " + grammar.C("brt", "CASE-ROLE AXES") + grammar.C("mut", " — the representational framework"),
		" " + grammar.C("mut", "Each row is a case-role pane; the detail is the five-tuple contract that makes it legible."),
		" " + grammar.RenderAxisHeader(),
		" " + grammar.C("border", strings.Repeat("─", maxVisible(10, w-2))),
	}
	visible := h - len(header)
	if visible < 1 {
		visible = 1
	}
	start := 0
	if len(rows) > visible {
		if m.AxisFocus >= visible {
			start = m.AxisFocus - visible + 1
		}
		if mx := len(rows) - visible; start > mx {
			start = mx
		}
	}
	var b strings.Builder
	for _, line := range header {
		b.WriteString(fitWidth(line, w) + "\n")
	}
	if len(rows) == 0 {
		b.WriteString(" " + grammar.C("mut", "no case-role axes") + "\n")
		return strings.TrimRight(b.String(), "\n")
	}
	for i := start; i < start+visible && i < len(rows); i++ {
		line := grammar.RenderAxisRow(rows[i], maxVisible(24, w-1))
		if i == m.AxisFocus {
			b.WriteString(grammar.C("yel", m.focusGlyph()) + focusBar(line, w-1) + "\n")
		} else {
			b.WriteString(" " + line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) axisDetailBody(w int) string {
	rows := grammar.Axes()
	if len(rows) == 0 {
		return " " + grammar.C("brt", "AXIS CONTRACT") + "\n\n " + grammar.C("mut", "no case-role axes")
	}
	idx := clamp(m.AxisFocus, 0, len(rows)-1)
	return grammar.RenderAxisDetail(rows[idx], w) + "\n\n" + grammar.RenderAxisFramework(w)
}

func (m Model) identityListBody(w, h int) string {
	roster := m.identityRoster()
	header := []string{
		" " + grammar.C("brt", "IDENTITY ROSTER") + grammar.C("mut", " — A1: who is acting (derived; projection-pending)"),
		" " + grammar.C("mut", "Each row is a principal; the name DENIES on air — class + counts are the skeleton."),
		" " + grammar.RenderIdentityHeader(),
		" " + grammar.C("border", strings.Repeat("─", maxVisible(10, w-2))),
	}
	visible := h - len(header)
	if visible < 1 {
		visible = 1
	}
	start := 0
	if len(roster) > visible {
		if m.IdentityFocus >= visible {
			start = m.IdentityFocus - visible + 1
		}
		if mx := len(roster) - visible; start > mx {
			start = mx
		}
	}
	var b strings.Builder
	for _, line := range header {
		b.WriteString(fitWidth(line, w) + "\n")
	}
	if len(roster) == 0 {
		b.WriteString(" " + grammar.C("mut", "no principals in the current role/actor/owner fold") + "\n")
		return strings.TrimRight(b.String(), "\n")
	}
	for i := start; i < start+visible && i < len(roster); i++ {
		line := grammar.RenderIdentityRow(roster[i], m.AIR, w-1)
		if i == m.IdentityFocus {
			b.WriteString(grammar.C("yel", m.focusGlyph()) + focusBar(line, w-1) + "\n")
		} else {
			b.WriteString(" " + line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) identityDetailBody(w int) string {
	roster := m.identityRoster()
	if len(roster) == 0 {
		return " " + grammar.C("brt", "IDENTITY CONTRACT") + "\n\n " + grammar.C("mut", "no principals in the current role/actor/owner fold")
	}
	return grammar.RenderIdentityDetail(roster[clamp(m.IdentityFocus, 0, len(roster)-1)], m.AIR, w)
}

func (m Model) relationalListBody(w, h int) string {
	facets := m.consentFacets()
	header := []string{
		" " + grammar.C("brt", "CONSENT POSTURE") + grammar.C("mut", " — A6: who is affected · the access-control lens (projection-pending)"),
		" " + grammar.C("mut", "The pane governs PII — it shows POLICY + counts + glyphs, never a protected value."),
		" " + grammar.RenderConsentFacetHeader(),
		" " + grammar.C("border", strings.Repeat("─", maxVisible(10, w-2))),
	}
	visible := h - len(header)
	if visible < 1 {
		visible = 1
	}
	start := 0
	if len(facets) > visible {
		if m.RelationalFocus >= visible {
			start = m.RelationalFocus - visible + 1
		}
		if mx := len(facets) - visible; start > mx {
			start = mx
		}
	}
	var b strings.Builder
	for _, line := range header {
		b.WriteString(fitWidth(line, w) + "\n")
	}
	if len(facets) == 0 {
		b.WriteString(" " + grammar.C("mut", "no consent facets in the current posture fold") + "\n")
		return strings.TrimRight(b.String(), "\n")
	}
	for i := start; i < start+visible && i < len(facets); i++ {
		line := grammar.RenderConsentFacetRow(facets[i], w-1)
		if i == m.RelationalFocus {
			b.WriteString(grammar.C("yel", m.focusGlyph()) + focusBar(line, w-1) + "\n")
		} else {
			b.WriteString(" " + line + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) relationalDetailBody(w int) string {
	facets := m.consentFacets()
	if len(facets) == 0 {
		return " " + grammar.C("brt", "CONSENT POSTURE") + "\n\n " + grammar.C("mut", "no consent facets in the current posture fold")
	}
	return grammar.RenderConsentFacetDetail(facets[clamp(m.RelationalFocus, 0, len(facets)-1)], w)
}

func (m Model) focusedLoopRow() (loopRow, bool) {
	rows := m.loopRows()
	if len(rows) == 0 {
		return loopRow{}, false
	}
	idx := clamp(m.LoopFocus, 0, len(rows)-1)
	return rows[idx], true
}

func loopArchetype(row loopRow) string {
	if row.Kind == graph.Indeterminate {
		return "sign-gap — assert missing polarity before naming the loop"
	}
	if row.Kind == graph.Balancing {
		if row.HasDelay {
			return "balancing-with-delay — oscillation risk"
		}
		if len(row.Nodes) == 2 {
			return "balancing pair — a direct counteraction loop"
		}
		return "balancing control loop — goal/gap correction structure"
	}
	allPositive := true
	for _, e := range row.Edges {
		if e.Sign != graph.SignPos {
			allPositive = false
			break
		}
	}
	if allPositive {
		return "reinforcing growth engine — virtuous or vicious depending on the variable"
	}
	if len(row.Nodes) == 2 {
		return "reinforcing dyad — even negative parity returns amplification"
	}
	return "reinforcing feedback loop — amplification by even sign parity"
}

func loopLeverage(row loopRow) []string {
	switch row.Kind {
	case graph.Reinforcing:
		return []string{
			"add damping/guardrails where amplification crosses an authority or capacity boundary",
			"watch for saturation: a stock/limit outside the loop may turn growth into collapse",
		}
	case graph.Balancing:
		if row.HasDelay {
			return []string{
				"shorten or expose the delayed link (‖); delay makes balancing loops oscillate",
				"tune the goal/gap sensor before increasing corrective force",
			}
		}
		return []string{
			"inspect the negative edge: it is the control surface that turns motion back",
			"make the intended setpoint explicit so balancing is legible, not invisible drag",
		}
	default:
		return []string{
			"assert a sign for every unknown edge before interpreting behavior",
			"keep the structure visible but do not infer dominance without polarity",
		}
	}
}

func (m Model) loopDetailBody(w int) string {
	row, ok := m.focusedLoopRow()
	if !ok {
		return " " + grammar.C("brt", "LOOP STRUCTURE") + "\n\n " + grammar.C("mut", "no causal loops in the current graph — acyclic or single-node")
	}
	var b strings.Builder
	idx := clamp(m.LoopFocus, 0, maxVisible(0, len(m.loopRows())-1))
	writeSectionHeader(&b, w, "LOOP STRUCTURE", fmt.Sprintf("focused loop %d/%d · %s · length %d · net sign %s", idx+1, maxVisible(1, len(m.loopRows())), loopKindWord(row.Kind), len(row.Nodes), loopDominantSign(row.Kind)), loopKindGlyph(row.Kind))
	writeWrappedKV(&b, "path", strings.Join(row.DisplayNodes, " → "), loopKindToken(row.Kind), w)
	if row.HasDelay {
		writeWrappedKV(&b, "delay", "‖ at least one delayed edge; loop can oscillate even without simulation", "yel", w)
	}
	writeWrappedKV(&b, "archetype", loopArchetype(row), "pri", w)
	writeSectionHeader(&b, w, "EDGE PARITY", "each link is read from :dynamics; sign parity determines loop type", "no simulation")
	if len(row.Edges) == 0 {
		writeWrappedKV(&b, "edges", "none", "mut", w)
	} else {
		for _, e := range row.Edges {
			delay := ""
			if e.Delay {
				delay = " ‖"
			}
			prov := string(e.Prov)
			if prov == "" {
				prov = string(graph.Asserted)
			}
			rel := ""
			if strings.TrimSpace(e.Relation) != "" && e.Relation != "▒▒▒" {
				rel = " · " + e.Relation
			}
			writeWrappedKV(&b, signGlyph(e.Sign)+delay, fmt.Sprintf("%s → %s%s · %s", e.Source, e.Target, rel, prov), "2nd", w)
		}
	}
	writeSectionHeader(&b, w, "LEVERAGE POINTS", "where to look first; still structural, not simulated dominance", "A5")
	for _, leverage := range loopLeverage(row) {
		writeWrappedBullet(&b, leverage, "yel", w)
	}
	writeWrappedKV(&b, "boundary", "loop TYPE/length/sign are structural and air-ok; node identities are gated through dynamics AIR", "mut", w)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderDynamicsReferencePaths(row dynamicsFocusRow, w int) string {
	var b strings.Builder
	writeSectionHeader(&b, w, "REFERENCE PATHS", "source refs, evidence row, and authority boundary", "source / evidence")
	if strings.TrimSpace(row.Docs) != "" {
		writeWrappedKV(&b, "docs", row.Docs, "blu", w)
	}
	if refs := dynamicsSourceRefLabelSummary(row.SourceRefLabels); refs != "" {
		writeWrappedKV(&b, "source refs", refs, "blu", w)
	} else {
		writeWrappedKV(&b, "source refs", "none indexed for this map cell yet", "mut", w)
	}
	if idx, ok := m.epistemicIndexForDynamicsFocus(); ok {
		rows := m.epistemicRows()
		if idx >= 0 && idx < len(rows) {
			ev := rows[idx]
			kind := "best match"
			if isMapEpistemicRow(ev) {
				kind = "exact map row"
			}
			writeWrappedKV(&b, "epistemics", fmt.Sprintf("%s %d/%d · %s · %s · %s", kind, idx+1, len(rows), ev.Family, ev.Subject, ev.Status), ev.Token, w)
			writeWrappedKV(&b, "authority", ev.Authority, "yel", w)
			writeWrappedKV(&b, "posture", strings.Join(nonEmptyParts([]string{"fresh=" + ev.Freshness, ev.Privacy, "support=" + ev.Evidence}), " · "), "2nd", w)
			if strings.TrimSpace(ev.SourceRefs) != "" {
				writeWrappedKV(&b, "evidence refs", ev.SourceRefs, "2nd", w)
			}
			if refs := dynamicsSourceRefLabelSummary(ev.SourceRefLabels); refs != "" {
				writeWrappedKV(&b, "ref labels", refs, "blu", w)
			}
			writeWrappedKV(&b, "next", "[E] opens evidence path; refs stay evidence inputs until source extraction and authority policy promote them", "pri", w)
			return b.String()
		}
	}
	writeWrappedKV(&b, "epistemics", "no direct evidence row currently matches this map cell", "org", w)
	writeWrappedKV(&b, "authority", "support only; source verification required before promotion", "yel", w)
	writeWrappedKV(&b, "next", "[E] opens the closest evidence lens; source-backed extraction should fill the missing reference path", "pri", w)
	return b.String()
}

func (m Model) renderDynamicsNeighborhood(row dynamicsFocusRow, w int) string {
	switch row.Kind {
	case "node":
		return m.renderDynamicsNodeNeighborhood(row, w)
	case "edge":
		return m.renderDynamicsEdgeNeighborhood(row, w)
	default:
		return ""
	}
}

func (m Model) renderDynamicsNodeNeighborhood(row dynamicsFocusRow, w int) string {
	if strings.TrimSpace(row.RawID) == "" {
		return ""
	}
	scale := m.dynamicsRenderScaleValue(w)
	g := m.Dynamics.AtResolution(scale)
	nodes := map[string]grammar.Node{}
	for _, n := range g.Nodes {
		nodes[n.ID] = n
	}
	type neighbor struct {
		dir, value, token string
	}
	neighbors := []neighbor{}
	in, out := 0, 0
	for _, e := range g.Edges {
		switch {
		case e.Source == row.RawID:
			out++
			target := dynamicsNodeLabelForNeighborhood(nodes[e.Target], m.AIR)
			rel := dynamicsEdgeFieldForAir(e, "relation", m.AIR)
			neighbors = append(neighbors, neighbor{"out", fmt.Sprintf("%s -> %s", firstNonEmpty(rel, "rel"), firstNonEmpty(target, dynamicsEdgeFieldForAir(e, "target", m.AIR), "·")), dynamicsStatusToken(e.Status)})
		case e.Target == row.RawID:
			in++
			source := dynamicsNodeLabelForNeighborhood(nodes[e.Source], m.AIR)
			rel := dynamicsEdgeFieldForAir(e, "relation", m.AIR)
			neighbors = append(neighbors, neighbor{"in", fmt.Sprintf("%s -> %s", firstNonEmpty(source, dynamicsEdgeFieldForAir(e, "source", m.AIR), "·"), firstNonEmpty(rel, "rel")), dynamicsStatusToken(e.Status)})
		}
	}
	if in == 0 && out == 0 {
		return dynamicsNeighborhoodSection(w, []contextRow{
			{"degree", fmt.Sprintf("in:%d · out:%d · isolated at %s scale", in, out, dynScaleName(scale)), "mut"},
		})
	}
	rows := []contextRow{{"degree", fmt.Sprintf("in:%d · out:%d · scale:%s", in, out, dynScaleName(scale)), "2nd"}}
	limit := len(neighbors)
	if limit > 4 {
		limit = 4
	}
	for i := 0; i < limit; i++ {
		n := neighbors[i]
		rows = append(rows, contextRow{n.dir, n.value, n.token})
	}
	if hidden := len(neighbors) - limit; hidden > 0 {
		rows = append(rows, contextRow{"more", fmt.Sprintf("%d hidden links at this scale", hidden), "mut"})
	}
	return dynamicsNeighborhoodSection(w, rows)
}

func (m Model) renderDynamicsEdgeNeighborhood(row dynamicsFocusRow, w int) string {
	if strings.TrimSpace(row.RawSource) == "" || strings.TrimSpace(row.RawTarget) == "" {
		return ""
	}
	scale := m.dynamicsRenderScaleValue(w)
	g := m.Dynamics.AtResolution(scale)
	nodes := map[string]grammar.Node{}
	for _, n := range g.Nodes {
		nodes[n.ID] = n
	}
	source := nodes[row.RawSource]
	target := nodes[row.RawTarget]
	sourceLabel := dynamicsNodeLabelForNeighborhood(source, m.AIR)
	targetLabel := dynamicsNodeLabelForNeighborhood(target, m.AIR)
	rows := []contextRow{
		{"path", fmt.Sprintf("%s -> %s -> %s", firstNonEmpty(sourceLabel, row.Source, "·"), firstNonEmpty(row.Relation, "rel"), firstNonEmpty(targetLabel, row.RawTarget, "·")), row.Token},
		{"source node", dynamicsNeighborhoodNodeDetail(source, m.AIR), dynamicsStatusToken(source.Status)},
		{"target node", dynamicsNeighborhoodNodeDetail(target, m.AIR), dynamicsStatusToken(target.Status)},
		{"scale", dynScaleName(scale), "2nd"},
	}
	return dynamicsNeighborhoodSection(w, rows)
}

func dynamicsNeighborhoodSection(w int, rows []contextRow) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	writeSectionHeader(&b, w, "NEIGHBORHOOD", "local topology around the selected map element", "spatial context")
	for _, row := range rows {
		writeWrappedKV(&b, row.label, row.value, row.token, w)
	}
	return b.String()
}

func dynamicsNodeLabelForNeighborhood(n grammar.Node, air bool) string {
	id := dynamicsNodeFieldForAir(n, "id", air)
	label := dynamicsNodeFieldForAir(n, "label", air)
	if strings.TrimSpace(label) != "" && label != "▒▒▒" {
		return strings.Join(nonEmptyParts([]string{id, label}), " ")
	}
	return id
}

func dynamicsNeighborhoodNodeDetail(n grammar.Node, air bool) string {
	if strings.TrimSpace(n.ID) == "" {
		return "not visible at current scale"
	}
	return strings.Join(nonEmptyParts([]string{
		dynamicsNodeLabelForNeighborhood(n, air),
		"layer=" + dynamicsNodeFieldForAir(n, "layer", air),
		"status=" + dynamicsNodeFieldForAir(n, "status", air),
	}), " · ")
}

func (m Model) renderDynamicsCompactBridge(row dynamicsFocusRow, w int) string {
	var b strings.Builder
	if idx, ok := m.epistemicIndexForDynamicsFocus(); ok {
		rows := m.epistemicRows()
		if idx >= 0 && idx < len(rows) {
			ev := rows[idx]
			path := "evidence path"
			if isMapEpistemicRow(ev) {
				path = "exact map reference"
			}
			writeWrappedKV(&b, "bridge", fmt.Sprintf("%s · %s · %s · [E] %s", ev.Family, ev.Subject, ev.Status, path), ev.Token, w)
			return b.String()
		}
	}
	writeWrappedKV(&b, "bridge", dynamicsFocusMeaning(row)+" · no direct evidence row yet", "org", w)
	return b.String()
}

func (m Model) renderDynamicsEpistemicBridge(row dynamicsFocusRow, w int) string {
	var b strings.Builder
	writeSectionHeader(&b, w, "EPISTEMIC BRIDGE", "why this map cell matters, what evidence supports it, and what remains unproven", "evidence bridge")
	openHint := "[E]/[Enter]"
	if m.sessionSplit() {
		openHint = "[E]"
	}
	writeWrappedKV(&b, "meaning", dynamicsFocusMeaning(row), "2nd", w)
	if idx, ok := m.epistemicIndexForDynamicsFocus(); ok {
		rows := m.epistemicRows()
		if idx >= 0 && idx < len(rows) {
			ev := rows[idx]
			label := "evidence"
			if isMapEpistemicRow(ev) {
				label = "exact ref"
			}
			writeWrappedKV(&b, label, fmt.Sprintf("%d/%d · %s · %s · %s", idx+1, len(rows), ev.Family, ev.Subject, ev.Status), ev.Token, w)
			writeWrappedKV(&b, "authority", ev.Authority, "yel", w)
			writeWrappedKV(&b, "support", strings.Join(nonEmptyParts([]string{ev.Evidence, "fresh=" + ev.Freshness, ev.Privacy}), " · "), "2nd", w)
			if strings.TrimSpace(ev.SourceRefs) != "" {
				writeWrappedKV(&b, "source refs", ev.SourceRefs, "2nd", w)
			}
			if refs := dynamicsSourceRefLabelSummary(ev.SourceRefLabels); refs != "" {
				writeWrappedKV(&b, "ref labels", refs, "blu", w)
			}
			writeWrappedKV(&b, "limits", ev.Detail, "mut", w)
			writeWrappedKV(&b, "next", openHint+" opens that evidence path; promote only through source verification, AIR, and authority policy", "pri", w)
			return b.String()
		}
	}
	writeWrappedKV(&b, "evidence", "no direct row currently matches this map cell", "org", w)
	writeWrappedKV(&b, "gap", "the topology is visible before its support chain is fully indexed; inspect nearby epistemic rows and package details before treating it as authority", "mut", w)
	writeWrappedKV(&b, "next", openHint+" opens the epistemics lens at the best available context; future source-backed extraction should fill this bridge", "pri", w)
	return b.String()
}

func dynamicsFocusMeaning(row dynamicsFocusRow) string {
	switch row.Kind {
	case "node":
		return "node = named system entity or lifecycle artifact; status is claim posture, centrality is likely leverage, layer is context"
	case "edge":
		return "edge = dependency, governance, or flow; source and target explain direction while relation names the behavior being asserted"
	case "source":
		return "source = provenance bundle for the map; freshness, privacy, and count set the ceiling for what Reins may claim"
	default:
		return "map cell = inspectable projection; meaning depends on its evidence row, authority ceiling, and validation posture"
	}
}

func dynamicsNodeEvidenceForAir(n grammar.Node, air bool) string {
	count := 0
	for _, field := range []string{"summary", "context", "docs", "hardening_notes", "aliases", "tags", "source_refs"} {
		if strings.TrimSpace(dynamicsNodeFieldForAir(n, field, air)) != "" {
			count++
		}
	}
	if len(dynamicsNodeSourceRefLabelsForAir(n, air)) > 0 {
		count++
	}
	if count == 0 {
		return "metadata"
	}
	return fmt.Sprintf("%d fields", count)
}

func dynamicsNodeEpistemicDetailForAir(n grammar.Node, air bool) string {
	return strings.Join(nonEmptyParts([]string{
		labeledPart("label", dynamicsNodeFieldForAir(n, "label", air)),
		labeledPart("kind", dynamicsNodeFieldForAir(n, "kind", air)),
		labeledPart("layer", dynamicsNodeFieldForAir(n, "layer", air)),
		labeledPart("summary", dynamicsNodeFieldForAir(n, "summary", air)),
		labeledPart("why", dynamicsNodeFieldForAir(n, "context", air)),
		labeledPart("refs", dynamicsNodeFieldForAir(n, "docs", air)),
		labeledPart("source refs", dynamicsSourceRefLabelSummary(dynamicsNodeSourceRefLabelsForAir(n, air))),
		labeledPart("hardening", dynamicsNodeFieldForAir(n, "hardening_notes", air)),
		labeledPart("tags", dynamicsNodeFieldForAir(n, "tags", air)),
	}), " · ")
}

func dynamicsEdgeSubject(id, source, target, relation string) string {
	if strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	path := strings.Join(nonEmptyParts([]string{strings.TrimSpace(source), strings.TrimSpace(target)}), "->")
	if strings.TrimSpace(path) != "" && strings.TrimSpace(relation) != "" {
		return path + ":" + strings.TrimSpace(relation)
	}
	return firstNonEmpty(path, strings.TrimSpace(relation))
}

func dynamicsEdgeSubjectForAir(e grammar.Edge, air bool) string {
	return dynamicsEdgeSubject(
		dynamicsEdgeFieldForAir(e, "id", air),
		dynamicsEdgeFieldForAir(e, "source", air),
		dynamicsEdgeFieldForAir(e, "target", air),
		dynamicsEdgeFieldForAir(e, "relation", air),
	)
}

func dynamicsEdgeSubjectForFocus(row dynamicsFocusRow) string {
	return dynamicsEdgeSubject(row.RawID, row.RawSource, row.RawTarget, row.RawRelation)
}

func dynamicsEdgeEvidenceForAir(e grammar.Edge, air bool) string {
	count := 0
	for _, field := range []string{"summary", "docs", "confidence", "layer", "source_refs"} {
		if strings.TrimSpace(dynamicsEdgeFieldForAir(e, field, air)) != "" {
			count++
		}
	}
	if len(dynamicsEdgeSourceRefLabelsForAir(e, air)) > 0 {
		count++
	}
	if count == 0 {
		return "metadata"
	}
	return fmt.Sprintf("%d fields", count)
}

func dynamicsEdgeEpistemicDetailForAir(e grammar.Edge, air bool) string {
	return strings.Join(nonEmptyParts([]string{
		labeledPart("source", dynamicsEdgeFieldForAir(e, "source", air)),
		labeledPart("target", dynamicsEdgeFieldForAir(e, "target", air)),
		labeledPart("relation", dynamicsEdgeFieldForAir(e, "relation", air)),
		labeledPart("layer", dynamicsEdgeFieldForAir(e, "layer", air)),
		labeledPart("confidence", dynamicsEdgeFieldForAir(e, "confidence", air)),
		labeledPart("summary", dynamicsEdgeFieldForAir(e, "summary", air)),
		labeledPart("refs", dynamicsEdgeFieldForAir(e, "docs", air)),
		labeledPart("source refs", dynamicsSourceRefLabelSummary(dynamicsEdgeSourceRefLabelsForAir(e, air))),
	}), " · ")
}

func epistemicRowLine(row epistemicRow, selected bool, w int) string {
	mark := " "
	if selected {
		mark = "▶"
	}
	return epistemicRowLineWithMark(row, mark, row.Token, w)
}

func epistemicRowLineWithMark(row epistemicRow, mark, markToken string, w int) string {
	return grammar.C(markToken, mark+" ") +
		grammar.C("org", fmt.Sprintf("%-15s", clipRunes(row.Family, 15))) +
		grammar.C(row.Token, fmt.Sprintf(" %-14s", clipRunes(row.Status, 14))) +
		grammar.C("brt", fmt.Sprintf(" %-32s", clipRunes(row.Subject, 32))) +
		grammar.C("yel", fmt.Sprintf(" %-20s", clipRunes(row.Authority, 20))) +
		grammar.C("2nd", fmt.Sprintf(" ev=%-5s", clipRunes(row.Evidence, 5))) +
		grammar.C("mut", " "+clipRunes(row.Detail, maxVisible(8, w-93)))
}

func epistemicFreshnessToken(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(v, "missing"), strings.Contains(v, "stale"), strings.Contains(v, "expired"), strings.Contains(v, "dark"):
		return "red"
	case strings.Contains(v, "fresh"), strings.Contains(v, "observed"), strings.Contains(v, "current"):
		return "grn"
	case strings.TrimSpace(v) == "", v == "·":
		return "mut"
	default:
		return "2nd"
	}
}

func (m Model) renderDynamicsGuide(w int) string {
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	b.WriteString(rule + "\n")
	writeSectionHeader(&b, w, "MAP ORIENTATION", "inspection and learning: what the map is, why it exists, and how to read it", "what / why / read")
	writeWrappedKV(&b, "what", "source-neutral semantic graph backbone for system identity, topology, temporal state, provenance, validation, and projections", "2nd", w)
	writeWrappedKV(&b, "why", "source notations, telemetry, event logs, and rendered views are inputs or projections; the map keeps identity, state, evidence, and authority separable", "org", w)
	writeWrappedKV(&b, "read", "overview first; cycle scale to domain/artifact/runtime/evidence, then inspect details; centrality is leverage, flow is dependency, status glyph is claim posture", "yel", w)
	writeWrappedKV(&b, "research", "system dynamics asks what structure drives behavior over time; visualization research asks overview, filter, details, and validated task-data-encoding fit", "blu", w)
	writeWrappedKV(&b, "epistemic", "map element -> source doc -> claim -> observation -> validation -> lens; no topology should be orphaned from a reference path", "pri", w)
	writeWrappedKV(&b, "backend", "Obsidian, plugins, manifests, and files are raw backends; Reins should be the epistemic inspection and learning surface", "grn", w)
	writeWrappedKV(&b, "n-DLC", "SDLC/RDLC/LDLC and future lifecycles are map tenants; Reins must expose lifecycle ontology without assuming every operator shares one taxonomy", "org", w)
	writeWrappedKV(&b, "boundary", "does not prove causality or fresh gate execution; it exposes where proof, telemetry, and validation are present, stale, missing, or rendered", "mut", w)
	b.WriteString(rule + "\n")
	return b.String()
}

func (m Model) renderDynamicsPrimer(w int) string {
	var b strings.Builder
	writeSectionHeader(&b, w, "MAP ORIENTATION", "compact primer before topology; expanded reference follows the selected map cell", "compact primer")
	writeWrappedKV(&b, "what", "overview first: source-neutral semantic graph backbone; source models, telemetry, observations, and views attach to shared identities", "2nd", w)
	writeWrappedKV(&b, "why", "source notations are inputs or projections, not the center; identity, state, evidence, authority, and rendered views stay separable", "org", w)
	writeWrappedKV(&b, "behavior", "system dynamics asks what structure drives behavior over time; observations show temporal state but topology alone does not prove causality", "blu", w)
	writeWrappedKV(&b, "epistemic", "map element -> source doc -> claim -> observation -> validation -> lens; [E] opens the evidence path", "pri", w)
	writeWrappedKV(&b, "rail marks", "▶ selected node · ◆ selected edge path · • moving flow · arrows show dependency/governance direction", "yel", w)
	writeWrappedKV(&b, "boundary", "Obsidian, plugins, manifests, and files are raw backends; Reins is the SDLC/RDLC/LDLC/n-DLC inspection surface, not proof authority", "mut", w)
	return b.String()
}

func (m Model) renderDynamicsWorkbench(w int) string {
	wb := m.Dynamics.Package.Workbench
	if wb.Status == "" && len(wb.InquiryModes) == 0 && len(wb.AudienceModes) == 0 && len(wb.ExplanationPaths) == 0 {
		return ""
	}
	var b strings.Builder
	writeSectionHeader(&b, w, "WORKBENCH", "question-first contract from the canonical view manifest", "question contract")
	if wb.Status != "observed" {
		writeWrappedKV(&b, "status", firstNonEmpty(wb.Status, "missing")+" · missing="+firstNonEmpty(wb.Missing, "workbench_contract"), countWarnToken(1), w)
		writeWrappedKV(&b, "effect", "dynamics can show graph topology, but inquiry/audience/explanation scenes are not source-backed", "yel", w)
		return strings.TrimRight(b.String(), "\n") + "\n"
	}
	inquiry := dynamicsWorkbenchInquiryByID(wb, wb.Defaults.InquiryMode)
	audience := dynamicsWorkbenchAudienceByID(wb, wb.Defaults.AudienceMode)
	path := dynamicsWorkbenchPathByID(wb, wb.Defaults.ExplanationPath)
	writeWrappedKV(&b, "defaults", fmt.Sprintf("inquiry=%s · audience=%s · path=%s",
		firstNonEmpty(wb.Defaults.InquiryMode, inquiry.ID),
		firstNonEmpty(wb.Defaults.AudienceMode, audience.ID),
		firstNonEmpty(wb.Defaults.ExplanationPath, path.ID)), "2nd", w)
	if inquiry.ID != "" {
		writeWrappedKV(&b, "inquiry", fmt.Sprintf("%s · lens=%s · prompt=%s",
			workbenchFieldForAir(inquiry.AIR, "label", inquiry.Label, m.AIR),
			firstNonEmpty(inquiry.Lens, "unknown"),
			workbenchFieldForAir(inquiry.AIR, "prompt", inquiry.Prompt, m.AIR)), "yel", w)
	}
	if audience.ID != "" {
		writeWrappedKV(&b, "audience", fmt.Sprintf("%s · %s",
			workbenchFieldForAir(audience.AIR, "label", audience.Label, m.AIR),
			workbenchFieldForAir(audience.AIR, "emphasis", audience.Emphasis, m.AIR)), "org", w)
	}
	if path.ID != "" {
		writeWrappedKV(&b, "path", fmt.Sprintf("%s · scenes:%d · must=%s",
			workbenchFieldForAir(path.AIR, "label", path.Label, m.AIR),
			maxVisible(path.SceneCount, len(path.Scenes)),
			workbenchListForAir(path.AIR, "must_include", path.MustInclude, m.AIR, 5)), "pri", w)
	}
	writeSectionHeader(&b, w, "INQUIRY READOUT", "default question projected onto the current graph package", "default readout")
	for _, row := range m.dynamicsWorkbenchReadoutRows(inquiry, path) {
		writeWrappedKV(&b, row.label, row.value, row.token, w)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func dynamicsWorkbenchInquiryByID(wb grammar.DynamicsWorkbench, id string) grammar.DynamicsWorkbenchInquiry {
	id = strings.TrimSpace(id)
	for _, row := range wb.InquiryModes {
		if strings.TrimSpace(row.ID) == id {
			return row
		}
	}
	if len(wb.InquiryModes) > 0 {
		return wb.InquiryModes[0]
	}
	return grammar.DynamicsWorkbenchInquiry{}
}

func dynamicsWorkbenchAudienceByID(wb grammar.DynamicsWorkbench, id string) grammar.DynamicsWorkbenchAudience {
	id = strings.TrimSpace(id)
	for _, row := range wb.AudienceModes {
		if strings.TrimSpace(row.ID) == id {
			return row
		}
	}
	if len(wb.AudienceModes) > 0 {
		return wb.AudienceModes[0]
	}
	return grammar.DynamicsWorkbenchAudience{}
}

func dynamicsWorkbenchPathByID(wb grammar.DynamicsWorkbench, id string) grammar.DynamicsWorkbenchExplanation {
	id = strings.TrimSpace(id)
	for _, row := range wb.ExplanationPaths {
		if strings.TrimSpace(row.ID) == id {
			return row
		}
	}
	if len(wb.ExplanationPaths) > 0 {
		return wb.ExplanationPaths[0]
	}
	return grammar.DynamicsWorkbenchExplanation{}
}

func workbenchFieldForAir(airMap map[string]string, field, value string, air bool) string {
	if !air {
		return value
	}
	switch field {
	case "label", "prompt", "emphasis", "summary", "title", "takeaway", "caveat":
		if airMap[field] != "ok" {
			return "▒▒▒"
		}
	}
	return value
}

func workbenchListForAir(airMap map[string]string, field string, values []string, air bool, limit int) string {
	if air {
		switch field {
		case "must_include", "answer_shape":
			if airMap[field] != "ok" {
				return "▒▒▒"
			}
		}
	}
	if limit <= 0 || limit > len(values) {
		limit = len(values)
	}
	if limit == 0 {
		return "none"
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, values[i])
	}
	if len(values) > limit {
		out = append(out, fmt.Sprintf("+%d more", len(values)-limit))
	}
	return strings.Join(out, " · ")
}

func (m Model) dynamicsWorkbenchReadoutRows(inquiry grammar.DynamicsWorkbenchInquiry, path grammar.DynamicsWorkbenchExplanation) []contextRow {
	rows := []contextRow{}
	if inquiry.ID != "" {
		foundNodes, totalNodes := m.dynamicsFocusNodeCount(inquiry.FocusNodeIDs)
		foundEdges, totalEdges := m.dynamicsFocusEdgeCount(inquiry.FocusEdgeIDs)
		rows = append(rows, contextRow{"active lens", firstNonEmpty(inquiry.Lens, "unknown") + fmt.Sprintf(" · focus nodes:%d/%d · edges:%d/%d", foundNodes, totalNodes, foundEdges, totalEdges), "yel"})
		rows = append(rows, contextRow{"answer", workbenchListForAir(inquiry.AIR, "answer_shape", inquiry.AnswerShape, m.AIR, 5), "2nd"})
	}
	recent, stale := m.dynamicsObservationFreshnessCounts()
	rows = append(rows, contextRow{"observations", fmt.Sprintf("recent:%d · stale:%d · source rows:%d", recent, stale, len(m.Dynamics.Package.Observations)), countWarnToken(stale)})
	rows = append(rows, contextRow{"first gate", m.dynamicsFirstNonReadyFocus(inquiry), "org"})
	if scene := dynamicsWorkbenchDoesNotProveScene(path); scene.Title != "" || scene.Caveat != "" {
		value := strings.Join(nonEmptyParts([]string{
			workbenchFieldForAir(scene.AIR, "title", scene.Title, m.AIR),
			workbenchFieldForAir(scene.AIR, "caveat", scene.Caveat, m.AIR),
		}), " · ")
		rows = append(rows, contextRow{"does not prove", value, "mut"})
	}
	if len(m.Dynamics.Package.Workbench.FollowOnTranches) > 0 {
		rows = append(rows, contextRow{"next tranche", strings.Join(m.Dynamics.Package.Workbench.FollowOnTranches[:1], " · "), "pri"})
	}
	return rows
}

func (m Model) dynamicsFocusNodeCount(ids []string) (int, int) {
	if len(ids) == 0 {
		return 0, 0
	}
	byID := map[string]bool{}
	for _, n := range m.Dynamics.Nodes {
		byID[n.ID] = true
	}
	found := 0
	for _, id := range ids {
		if byID[id] {
			found++
		}
	}
	return found, len(ids)
}

func (m Model) dynamicsFocusEdgeCount(ids []string) (int, int) {
	if len(ids) == 0 {
		return 0, 0
	}
	byID := map[string]bool{}
	for _, e := range m.Dynamics.Edges {
		byID[e.ID] = true
	}
	found := 0
	for _, id := range ids {
		if byID[id] {
			found++
		}
	}
	return found, len(ids)
}

func (m Model) dynamicsObservationFreshnessCounts() (int, int) {
	recent, stale := 0, 0
	for _, row := range m.Dynamics.Package.Observations {
		status := strings.ToLower(strings.Join(nonEmptyParts([]string{row.Status, row.ID, row.Detail}), " "))
		if strings.Contains(status, "stale") || strings.Contains(status, "expired") {
			stale++
		} else {
			recent++
		}
	}
	return recent, stale
}

func (m Model) dynamicsFirstNonReadyFocus(inquiry grammar.DynamicsWorkbenchInquiry) string {
	ready := func(status string) bool {
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "asserted", "observed", "rendered", "lossless", "declared":
			return true
		}
		return false
	}
	nodes := map[string]grammar.Node{}
	for _, n := range m.Dynamics.Nodes {
		nodes[n.ID] = n
	}
	for _, id := range inquiry.FocusNodeIDs {
		n, ok := nodes[id]
		if !ok {
			return "missing node " + id
		}
		status := dynamicsNodeFieldForAir(n, "status", m.AIR)
		if !ready(status) {
			return fmt.Sprintf("node %s status=%s", dynamicsNodeFieldForAir(n, "id", m.AIR), firstNonEmpty(status, "unknown"))
		}
	}
	edges := map[string]grammar.Edge{}
	for _, e := range m.Dynamics.Edges {
		edges[e.ID] = e
	}
	for _, id := range inquiry.FocusEdgeIDs {
		e, ok := edges[id]
		if !ok {
			return "missing edge " + id
		}
		status := dynamicsEdgeFieldForAir(e, "status", m.AIR)
		if !ready(status) {
			return fmt.Sprintf("edge %s status=%s", dynamicsEdgeSubjectForAir(e, m.AIR), firstNonEmpty(status, "unknown"))
		}
	}
	if len(inquiry.FocusNodeIDs) == 0 && len(inquiry.FocusEdgeIDs) == 0 {
		return "no focus path declared"
	}
	return "none visible in declared focus path; inspect live gates/events before authority"
}

func dynamicsWorkbenchDoesNotProveScene(path grammar.DynamicsWorkbenchExplanation) grammar.DynamicsWorkbenchScene {
	for _, scene := range path.Scenes {
		text := strings.ToLower(scene.Title + " " + scene.Caveat + " " + scene.Takeaway)
		if strings.Contains(text, "does not prove") || strings.Contains(text, "not prove") {
			return scene
		}
	}
	if len(path.Scenes) > 0 {
		return path.Scenes[len(path.Scenes)-1]
	}
	return grammar.DynamicsWorkbenchScene{}
}

func (m Model) renderDynamicsPackage(w int) string {
	pkg := m.Dynamics.Package
	if len(pkg.Sources) == 0 && len(pkg.Lenses) == 0 && len(pkg.Claims) == 0 && len(pkg.Observations) == 0 && len(pkg.Relations) == 0 {
		return ""
	}
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	var b strings.Builder
	b.WriteString(" " + grammar.C("brt", "DYNAMICS PACKAGE") + grammar.C("mut", " — source-backed map package; read-only evidence, not authority") + "\n")
	if pkg.GeneratedAt != "" || pkg.Authority != "" || pkg.DefaultLens != "" {
		writeWrappedKV(&b, "package", fmt.Sprintf("generated=%s · authority=%s · default_lens=%s · hash=%s",
			dynPackageText(pkg.GeneratedAt), dynPackageText(pkg.Authority), dynPackageText(pkg.DefaultLens), dynPackageText(shortHash(pkg.PackageHash))), "2nd", w)
	}
	if len(pkg.Totals) > 0 {
		writeWrappedKV(&b, "totals", fmt.Sprintf("sources=%d · artifacts=%d · nodes=%d · edges=%d · claims=%d · obs=%d · relations=%d · lenses=%d · validation=%d · missing=%d",
			pkg.Totals["sources"], pkg.Totals["artifacts"], pkg.Totals["nodes"], pkg.Totals["edges"], pkg.Totals["claims"], pkg.Totals["observations"], pkg.Totals["relations"], pkg.Totals["lenses"], pkg.Totals["validation"], pkg.Totals["missing_sources"]), dynamicsCountToken(pkg.Totals["missing_sources"]), w)
	}
	writeWrappedKV(&b, "details", "graph rail stays in the first pass; source/claim/validation details continue below the topology", "mut", w)
	b.WriteString(rule + "\n")
	writeSectionHeader(&b, w, "INSPECTION PATHS", "lens, inquiry, and epistemic references should be reachable without leaving Reins", "lens / inquiry / refs")
	writeWrappedKV(&b, "lens", "topology = whole graph · operating-slice = live observed path · evidence-risk = stale or missing proof", "2nd", w)
	writeWrappedKV(&b, "inquiry", "release gates · stuck work · changed state · stale evidence · trust chain · missing context", "yel", w)
	writeWrappedKV(&b, "reference", "sources, claims, observations, validation, relation vocabulary, lenses, and manifest scenes are first-class sections below", "pri", w)
	writeWrappedKV(&b, "next focus", "node/edge focus should jump directly to related claim, evidence, reference, and validation rows; backend paths remain evidence, not destinations", "mut", w)
	b.WriteString(rule + "\n")
	return b.String()
}

func (m Model) dynamicsRenderScaleValue(w int) int {
	scale, _ := m.dynamicsRenderScale(w)
	return scale
}

type dynamicsDegreeRow struct {
	id, layer, status string
	in, out           int
}

func (m Model) renderDynamicsGraphSummary(w, scale int) string {
	g := m.Dynamics.AtResolution(scale)
	if len(g.Nodes) == 0 {
		return ""
	}
	layerLabels := map[string]string{}
	for _, layer := range g.Layers {
		label := strings.TrimSpace(layer.Label)
		if label == "" {
			label = layer.ID
		}
		layerLabels[layer.ID] = label
	}
	nodes := map[string]*dynamicsDegreeRow{}
	for _, n := range g.Nodes {
		id := dynamicsNodeFieldForAir(n, "id", m.AIR)
		if strings.TrimSpace(id) == "" {
			id = "·"
		}
		layerID := dynamicsNodeFieldForAir(n, "layer", m.AIR)
		layer := "unknown"
		if strings.TrimSpace(layerID) != "" {
			layer = layerID
			if layerID != "▒▒▒" {
				if label := layerLabels[layerID]; strings.TrimSpace(label) != "" {
					layer = label
				}
			}
		}
		nodes[n.ID] = &dynamicsDegreeRow{id: id, layer: layer, status: dynamicsNodeFieldForAir(n, "status", m.AIR)}
	}
	flows := map[string]int{}
	edgeStatus := map[string]int{}
	for _, e := range g.Edges {
		sourceID := dynamicsEdgeFieldForAir(e, "source", m.AIR)
		targetID := dynamicsEdgeFieldForAir(e, "target", m.AIR)
		status := dynamicsEdgeFieldForAir(e, "status", m.AIR)
		if strings.TrimSpace(status) == "" {
			status = "unknown"
		}
		if sourceID == "▒▒▒" || targetID == "▒▒▒" || strings.TrimSpace(sourceID) == "" || strings.TrimSpace(targetID) == "" {
			flows["hidden relation"]++
			edgeStatus[status]++
			continue
		}
		src := nodes[sourceID]
		tgt := nodes[targetID]
		if src == nil || tgt == nil {
			continue
		}
		src.out++
		tgt.in++
		flow := firstNonEmpty(src.layer, "unknown") + "→" + firstNonEmpty(tgt.layer, "unknown")
		flows[flow]++
		edgeStatus[status]++
	}
	degrees := make([]dynamicsDegreeRow, 0, len(nodes))
	for _, row := range nodes {
		degrees = append(degrees, *row)
	}
	sort.Slice(degrees, func(i, j int) bool {
		di := degrees[i].in + degrees[i].out
		dj := degrees[j].in + degrees[j].out
		if di != dj {
			return di > dj
		}
		if degrees[i].out != degrees[j].out {
			return degrees[i].out > degrees[j].out
		}
		return degrees[i].id < degrees[j].id
	})
	var b strings.Builder
	writeSectionHeader(&b, w, "GRAPH SUMMARY", "centrality and flow before the deterministic rail", "centrality + flow")
	writeWrappedKV(&b, "visible", fmt.Sprintf("nodes=%d · edges=%d · layers=%d · scale=%s", len(g.Nodes), len(g.Edges), len(g.Layers), dynScaleName(scale)), "2nd", w)
	b.WriteString(m.renderDynamicsScaleLadder(w, scale))
	limit := len(degrees)
	if limit > 4 {
		limit = 4
	}
	wroteCentral := false
	for i := 0; i < limit; i++ {
		row := degrees[i]
		degree := row.in + row.out
		if degree == 0 {
			continue
		}
		writeWrappedKV(&b, "central", fmt.Sprintf("%s degree=%d · in=%d out=%d · layer=%s · status=%s", row.id, degree, row.in, row.out, row.layer, firstNonEmpty(row.status, "unknown")), dynamicsStatusToken(row.status), w)
		wroteCentral = true
	}
	if !wroteCentral {
		writeWrappedKV(&b, "at scale", "no visible edge flow at this scale; cycle domain/artifact/runtime/evidence/all for relation topology", "mut", w)
	}
	if len(flows) > 0 {
		writeWrappedKV(&b, "flow", compactCountMap(flows, 4), "blu", w)
	}
	if len(edgeStatus) > 0 {
		writeWrappedKV(&b, "edge status", compactCountMap(edgeStatus, 4), "2nd", w)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func (m Model) renderDynamicsScaleLadder(w, current int) string {
	if len(m.Dynamics.Nodes) == 0 {
		return ""
	}
	order := []int{1, 2, 3, 4, 5, 0}
	parts := make([]string, 0, len(order))
	for _, scale := range order {
		g := m.Dynamics.AtResolution(scale)
		name := dynScaleName(scale)
		if scale == current {
			name = "▶" + name
		}
		parts = append(parts, fmt.Sprintf("%s:%dn/%de", name, len(g.Nodes), len(g.Edges)))
	}
	var b strings.Builder
	writeWrappedKV(&b, "scale path", strings.Join(parts, " · "), "yel", w)
	full := m.Dynamics.AtResolution(0)
	visible := m.Dynamics.AtResolution(current)
	if current > 0 && (len(visible.Nodes) < len(full.Nodes) || len(visible.Edges) < len(full.Edges)) {
		hiddenNodes := len(full.Nodes) - len(visible.Nodes)
		if hiddenNodes < 0 {
			hiddenNodes = 0
		}
		hiddenEdges := len(full.Edges) - len(visible.Edges)
		if hiddenEdges < 0 {
			hiddenEdges = 0
		}
		writeWrappedKV(&b, "scale why",
			fmt.Sprintf("%s is a teaching/filter lens; %d deeper nodes and %d deeper edges are withheld here. [.] adds detail, [,] removes detail, all shows the full graph.",
				dynScaleName(current), hiddenNodes, hiddenEdges),
			"2nd", w)
	} else if current == 0 && (len(full.Nodes) > 0 || len(full.Edges) > 0) {
		writeWrappedKV(&b, "scale why", "all shows the full topology; use [,/.] to move between didactic overview, lifecycle/domain layers, artifacts, runtime, and evidence-risk slices.", "2nd", w)
	}
	return b.String()
}

func dynamicsNodeFieldForAir(n grammar.Node, field string, air bool) string {
	var val string
	switch field {
	case "id":
		val = n.ID
	case "label":
		val = n.Label
	case "kind":
		val = n.Kind
	case "layer":
		val = n.Layer
	case "status":
		val = n.Status
	case "res":
		val = n.Res
	case "summary":
		val = n.Summary
	case "context":
		val = n.Context
	case "docs":
		val = n.Docs
	case "hardening_notes":
		val = n.HardeningNotes
	case "aliases":
		val = n.Aliases
	case "tags":
		val = n.Tags
	case "source_refs":
		val = n.SourceRefs
	}
	if strings.TrimSpace(val) == "" {
		return ""
	}
	return grammar.Redact(n.AIR, field, val, air)
}

func dynamicsSourceRefLabelSummary(labels []string) string {
	return capabilityRefLabelSummary(labels)
}

func dynamicsNodeSourceRefLabelsForAir(n grammar.Node, air bool) []string {
	if air && n.AIR["source_ref_labels"] != "ok" {
		return nil
	}
	out := make([]string, 0, len(n.SourceRefLabels))
	for _, label := range n.SourceRefLabels {
		label = strings.TrimSpace(label)
		if label != "" {
			out = append(out, label)
		}
	}
	return out
}

func dynamicsEdgeFieldForAir(e grammar.Edge, field string, air bool) string {
	var val string
	switch field {
	case "id":
		val = e.ID
	case "source":
		val = e.Source
	case "target":
		val = e.Target
	case "relation":
		val = e.Relation
	case "status":
		val = e.Status
	case "layer":
		val = e.Layer
	case "res":
		val = e.Res
	case "confidence":
		val = e.Confidence
	case "summary":
		val = e.Summary
	case "docs":
		val = e.Docs
	case "source_refs":
		val = e.SourceRefs
	}
	if strings.TrimSpace(val) == "" {
		return ""
	}
	return grammar.Redact(e.AIR, field, val, air)
}

func dynamicsEdgeSourceRefLabelsForAir(e grammar.Edge, air bool) []string {
	if air && e.AIR["source_ref_labels"] != "ok" {
		return nil
	}
	out := make([]string, 0, len(e.SourceRefLabels))
	for _, label := range e.SourceRefLabels {
		label = strings.TrimSpace(label)
		if label != "" {
			out = append(out, label)
		}
	}
	return out
}

func (m Model) dynamicsRenderScale(w int) (int, string) {
	// Fit the map to a NARROW render width regardless of the (abolished, for dynamics) session-frozen
	// split: the migrated dynamics primary is itself a sub-full-width pane, so the fit must key off the
	// actual render width, not the session-anchored split.
	if m.DynScale == 0 && w < 130 {
		return 2, "all→domain fit"
	}
	return m.DynScale, dynScaleName(m.DynScale)
}

func (m Model) renderDynamicsPackageDetails(w int) string {
	pkg := m.Dynamics.Package
	if len(pkg.Sources) == 0 && len(pkg.Validation) == 0 && len(pkg.Lenses) == 0 && len(pkg.Claims) == 0 && len(pkg.Observations) == 0 && len(pkg.Relations) == 0 {
		return ""
	}
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	var b strings.Builder
	b.WriteString("\n" + rule + "\n")
	b.WriteString(" " + grammar.C("brt", "DYNAMICS PACKAGE DETAIL") + grammar.C("mut", " — source, validation, lens, claim, observation, and relation rows") + "\n")
	if len(pkg.Sources) > 0 {
		b.WriteString(rule + "\n")
		b.WriteString(" " + grammar.C("brt", "SOURCES") + grammar.C("mut", " — package files; raw graph/doc bodies are not exposed here") + "\n")
		for _, src := range pkg.Sources {
			writeWrappedKV(&b, dynamicsSourceFieldForAir(src, "id", m.AIR),
				fmt.Sprintf("status=%s · count=%s · age=%s · file=%s · detail=%s",
					dynamicsSourceFieldForAir(src, "status", m.AIR),
					dynamicsSourceFieldForAir(src, "count", m.AIR),
					dynamicsSourceFieldForAir(src, "age_bucket", m.AIR),
					dynamicsSourceFieldForAir(src, "path", m.AIR),
					dynamicsSourceFieldForAir(src, "detail", m.AIR)),
				dynamicsStatusToken(src.Status), w)
		}
	}
	if len(pkg.Validation) > 0 {
		b.WriteString(rule + "\n")
		b.WriteString(" " + grammar.C("brt", "VALIDATION") + grammar.C("mut", " — declared checks; Reins renders posture, not proof of fresh execution") + "\n")
		for _, row := range pkg.Validation {
			writeWrappedKV(&b, dynamicsRowFieldForAir(row, "id", m.AIR),
				fmt.Sprintf("status=%s · source=%s · check=%s",
					dynamicsRowFieldForAir(row, "status", m.AIR),
					dynamicsRowFieldForAir(row, "source", m.AIR),
					dynamicsRowFieldForAir(row, "detail", m.AIR)),
				dynamicsStatusToken(row.Status), w)
		}
	}
	if len(pkg.Lenses) > 0 {
		b.WriteString(rule + "\n")
		b.WriteString(" " + grammar.C("brt", "LENSES") + grammar.C("mut", " — projection scope/lossiness must stay visible") + "\n")
		for _, row := range pkg.Lenses {
			writeWrappedKV(&b, dynamicsRowFieldForAir(row, "id", m.AIR),
				fmt.Sprintf("nodes=%s · status=%s · %s",
					dynamicsRowFieldForAir(row, "count", m.AIR),
					dynamicsRowFieldForAir(row, "status", m.AIR),
					dynamicsRowFieldForAir(row, "detail", m.AIR)),
				dynamicsStatusToken(row.Status), w)
		}
	}
	if len(pkg.Claims) > 0 {
		b.WriteString(rule + "\n")
		b.WriteString(" " + grammar.C("brt", "CLAIM PARTITIONS") + grammar.C("mut", " — confidence and authority ceiling stay separate from topology") + "\n")
		for _, row := range pkg.Claims {
			writeWrappedKV(&b, dynamicsRowFieldForAir(row, "id", m.AIR),
				fmt.Sprintf("authority=%s · count=%s · %s",
					dynamicsRowFieldForAir(row, "status", m.AIR),
					dynamicsRowFieldForAir(row, "count", m.AIR),
					dynamicsRowFieldForAir(row, "detail", m.AIR)),
				dynamicsStatusToken(row.Status), w)
		}
	}
	if len(pkg.Observations) > 0 {
		b.WriteString(rule + "\n")
		b.WriteString(" " + grammar.C("brt", "OBSERVATION STATE") + grammar.C("mut", " — temporal state is evidence, not overwritten topology") + "\n")
		for _, row := range pkg.Observations {
			writeWrappedKV(&b, dynamicsRowFieldForAir(row, "id", m.AIR),
				fmt.Sprintf("freshness=%s · count=%s · %s",
					dynamicsRowFieldForAir(row, "status", m.AIR),
					dynamicsRowFieldForAir(row, "count", m.AIR),
					dynamicsRowFieldForAir(row, "detail", m.AIR)),
				dynamicsStatusToken(row.Status), w)
		}
	}
	if len(pkg.Relations) > 0 {
		b.WriteString(rule + "\n")
		b.WriteString(" " + grammar.C("brt", "RELATION VOCABULARY") + grammar.C("mut", " — relation categories, not causal proof") + "\n")
		for _, row := range pkg.Relations {
			writeWrappedKV(&b, dynamicsRowFieldForAir(row, "id", m.AIR),
				fmt.Sprintf("status=%s · relations=%s · %s",
					dynamicsRowFieldForAir(row, "status", m.AIR),
					dynamicsRowFieldForAir(row, "count", m.AIR),
					dynamicsRowFieldForAir(row, "detail", m.AIR)),
				dynamicsStatusToken(row.Status), w)
		}
	}
	return b.String()
}

func dynPackageText(s string) string {
	if strings.TrimSpace(s) == "" {
		return "·"
	}
	return s
}

func shortHash(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}

func dynamicsSourceFieldForAir(src grammar.DynamicsSource, field string, air bool) string {
	var val string
	switch field {
	case "id":
		val = src.ID
	case "status":
		val = src.Status
	case "count":
		val = fmt.Sprintf("%d", src.Count)
	case "detail":
		val = src.Detail
	case "age_bucket":
		val = src.AgeBucket
	case "path":
		val = src.Path
	case "privacy":
		val = src.Privacy
	case "raw_access":
		val = fmt.Sprintf("%t", src.RawAccess)
	}
	return grammar.Redact(src.AIR, field, val, air)
}

func dynamicsRowFieldForAir(row grammar.DynamicsRow, field string, air bool) string {
	var val string
	switch field {
	case "kind":
		val = row.Kind
	case "id":
		val = row.ID
	case "source":
		val = row.Source
	case "status":
		val = row.Status
	case "severity":
		val = row.Severity
	case "count":
		val = fmt.Sprintf("%d", row.Count)
	case "detail":
		val = row.Detail
	}
	return grammar.Redact(row.AIR, field, val, air)
}

func dynamicsStatusToken(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "missing", "dark", "stale", "lossy":
		return "red"
	case "present", "observed", "declared", "lossless", "fresh", "architecture_contract":
		return "grn"
	case "candidate", "unknown":
		return "yel"
	default:
		return "2nd"
	}
}

func dynamicsConfidenceToken(value string) string {
	v, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return "2nd"
	}
	switch {
	case v >= 0.9:
		return "grn"
	case v >= 0.6:
		return "yel"
	default:
		return "red"
	}
}

func dynamicsCountToken(missing int) string {
	if missing > 0 {
		return "red"
	}
	return "grn"
}

func (m Model) renderCommandCatalog(w int) string {
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	targetFocus := m.targetRowFocusActive()
	b.WriteString(" " + grammar.C("brt", "COMMANDS") + grammar.C("mut", " — "+m.commandsSummary()) + "\n")
	b.WriteString(" " + grammar.C("mut", "one catalog backs hotkeys, command line completion, probes, and governed intent previews") + "\n")
	if targetFocus {
		b.WriteString(" " + grammar.C("mut", "[j/k] command focus · [←/→] cycle windows · :<verb> jumps here when the verb is a screen") + "\n")
	} else {
		b.WriteString(" " + grammar.C("mut", "split reference: source lane owns [j/k]/[Enter]/[y]; use :<verb> or [|] unsplit for command focus") + "\n")
	}
	b.WriteString("\n")
	for i, v := range verbs {
		display := commandDisplayName(v)
		auth := v.authority
		if strings.TrimSpace(auth) == "" {
			auth = "none"
		}
		preflight := v.preflight
		if strings.TrimSpace(preflight) == "" {
			preflight = "none"
		}
		receipt := v.receipt
		if strings.TrimSpace(receipt) == "" {
			receipt = "none"
		}
		kg := commandKindGroup(v)
		token := commandToken(v)
		prefix := commandFactPrefix(display, targetFocus && i == m.CommandFocus, token)
		writeSegmentedFactRow(&b, prefix, []string{
			"verb=" + display,
			"kind=" + kg,
			"auth=" + auth,
			"preflight=" + preflight,
			"receipt=" + receipt,
			"ui=" + v.uiDelta,
		}, w, targetFocus && i == m.CommandFocus)
		if len(v.args) > 0 {
			args := make([]string, 0, len(v.args))
			for _, a := range v.args {
				args = append(args, a.Label)
			}
			writeSegmentedFactRow(&b, " "+grammar.C("mut", "  ▸ args"), args, w, false)
		}
	}
	b.WriteString("\n" + rule + "\n")
	b.WriteString(" " + grammar.C("brt", "COMMAND FLOW") + grammar.C("mut", " — command-as-effect stays pure until governed routes exist") + "\n")
	for _, row := range []contextRow{
		{"complete", "Tab browses verbs/args/templates", "yel"},
		{"preview", ":intent shows target/preflight/receipt", "org"},
		{"inject", "{{focus}}/{{sel.*}}/{{ring.0}}", "yel"},
		{"effect", "local read/lens/status only today", "grn"},
		{"future", "mutation via COMMAND receipt path", "2nd"},
	} {
		b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-10s", row.label)) +
			grammar.C(row.token, clipRunes(row.value, maxVisible(12, w-13))) + "\n")
	}
	b.WriteString("\n" + rule + "\n")
	b.WriteString(" " + grammar.C("brt", "TEMPLATE INJECTION") + grammar.C("mut", " — AIR-safe references bind selections at run time") + "\n")
	b.WriteString(" " + grammar.C("mut", "selection source ") + grammar.C("pri", pageLabel(m.commandSelectionPage())) + grammar.C("mut", " · values shown are exactly what command execution expands") + "\n")
	for _, row := range m.templateInjectionRows() {
		b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-16s", row.label)) +
			grammar.C(row.token, clipRunes(row.value, maxVisible(12, w-19))) + "\n")
	}
	b.WriteString("\n" + rule + "\n")
	b.WriteString(" " + grammar.C("brt", "COMMAND RAIL") + grammar.C("mut", " — preview before effect, receipt before mutation") + "\n")
	for _, row := range []contextRow{
		{"read", "window/lens/status verbs are local effects", "grn"},
		{"compose", ":note and templates stay local unless routed", "yel"},
		{"preview", ":intent resolves subject, authority, preflight, receipt", "org"},
		{"govern", "mutation waits for COMMAND route authority", "red"},
	} {
		b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-10s", row.label)) +
			grammar.C(row.token, clipRunes(row.value, maxVisible(12, w-13))) + "\n")
	}
	// E4.7 chat-UX-bar: the catalog above is reins's OWN verbs; this manifest holds reins ACCOUNTABLE to
	// the native-session verb set (CC/Codex/Agy/GLMCP §6) — each native verb maps to a reins projection,
	// an honest N/A (direction-only doctrine), or a flagged GAP. The bar is "≥ native"; the holes are owned.
	b.WriteString("\n" + rule + "\n")
	b.WriteString(grammar.RenderChatParity(grammar.ChatParityManifest(), w) + "\n")
	return strings.TrimRight(b.String(), "\n")
}

func commandFactPrefix(display string, selected bool, tok string) string {
	mark := " "
	if selected {
		mark = "▶"
	}
	return grammar.C("yel", mark+" ") + grammar.C(tok, fmt.Sprintf("%-16s", clipRunes(display, 16)))
}

func commandDisplayName(v verbDef) string {
	name := v.name
	if len(v.aliases) > 0 {
		name += " (" + strings.Join(v.aliases, ",") + ")"
	}
	return name
}

func commandKindGroup(v verbDef) string {
	kg := string(v.kind)
	if v.group != "" {
		kg += "/" + v.group
	}
	return kg
}

func commandToken(v verbDef) string {
	if strings.Contains(v.authority, "governed") || v.kind == commandIntent {
		return "org"
	}
	if v.kind == commandLocal {
		return "yel"
	}
	return "pri"
}

func strongerToken(a, b string) string {
	if tokenRank(b) > tokenRank(a) {
		return b
	}
	return a
}

func tokenRank(token string) int {
	switch token {
	case "red":
		return 7
	case "org":
		return 6
	case "yel":
		return 5
	case "pri":
		return 4
	case "grn":
		return 3
	case "2nd":
		return 2
	case "mut":
		return 1
	default:
		return 0
	}
}

func catalogArgWidth(w int) int {
	if w-12 > 20 {
		return w - 12
	}
	return 20
}

func (m Model) templateInjectionRows() []contextRow {
	keys := []string{"focus", "sel.field", "sel.value", "sel.status", "sel.meaning", "sel.family", "sel.receipt", "sel.source_refs", "sel.missing", "ring.0"}
	rows := make([]contextRow, 0, len(keys)+1)
	for _, key := range keys {
		label := "{{" + key + "}}"
		desc := templateInjectionDescription(key)
		value, ok := m.templateValue(key)
		token := "yel"
		if !ok {
			value = "unavailable for " + pageLabel(m.commandSelectionPage()) + " · " + desc
			token = "mut"
		} else {
			if strings.TrimSpace(value) == "" {
				value = "·"
			}
			value = desc + " -> " + value
		}
		rows = append(rows, contextRow{label, value, token})
	}
	rows = append(rows, contextRow{"{{sel.<field>}}", "named field from the active row when supported; rejected tokens stay literal", "yel"})
	return rows
}

func templateInjectionDescription(key string) string {
	switch key {
	case "focus":
		return "focused row identity"
	case "sel.field":
		return "selected field name"
	case "sel.value":
		return "selected field value through AIR"
	case "sel.status":
		return "status field"
	case "sel.meaning":
		return "routing meaning/posture, not authority"
	case "sel.family":
		return "capability/surface family"
	case "sel.receipt":
		return "required receipt contract"
	case "sel.source_refs":
		return "metadata-only provenance pointer"
	case "sel.missing":
		return "missing policy/receipt"
	case "ring.0":
		return "last yanked AIR-safe value"
	}
	return "selection reference"
}

func splitPairCatalogCell(page int) (string, string) {
	pair, ok := splitPairForPage(page)
	if !ok {
		return "none", "mut"
	}
	if pair.Reactive() {
		return pair.PaneProfileLabel() + " " + pair.Join, "org"
	}
	return pair.PaneProfileLabel() + " " + pair.Join, "2nd"
}

func windowFactPrefix(mark, key, id string) string {
	return grammar.C("yel", mark+" ") +
		grammar.C("brt", fmt.Sprintf("%-3s", key)) +
		grammar.C("pri", fmt.Sprintf(" %-11s", clipRunes(id, 11)))
}

func (m Model) renderWindowCatalog(w int) string {
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	targetFocus := m.targetRowFocusActive()
	writeSectionHeader(&b, w, "WINDOWS", windowsSummary(), "registered screens")
	if targetFocus {
		b.WriteString(" " + grammar.C("mut", "screens are registered objects; [j/k] window focus, split pairs are declared per window, [key] jumps, [←/→] cycles") + "\n")
	} else {
		b.WriteString(" " + grammar.C("mut", "split reference: source lane owns [j/k]/[Enter]/[y]; [key]/[←/→] still change windows") + "\n")
	}
	writeWrappedKV(&b, "shape", splitPairSummary()+" · linked pairs rebind context; source-only pairs keep right independent while source stays navigable", "org", w)
	b.WriteString("\n")
	for i, wnd := range registeredWindows() {
		mark := " "
		if targetFocus && i == m.WindowFocus {
			mark = "▶"
		}
		signal, _ := m.windowSignal(wnd.Page)
		if signal == "" {
			signal = "quiet"
		}
		pair, _ := splitPairCatalogCell(wnd.Page)
		prefix := windowFactPrefix(mark, wnd.Key, wnd.ID)
		writeSegmentedFactRow(&b, prefix, []string{
			"window=" + wnd.ID,
			"scope=" + wnd.Scope,
			"lifecycle=" + wnd.Lifecycle,
			"kind=" + wnd.Kind,
			"signal=" + signal,
			"split=" + pair,
			"page=" + pageLabel(wnd.Page),
		}, w, targetFocus && i == m.WindowFocus)
	}
	b.WriteString("\n" + rule + "\n")
	b.WriteString(" " + grammar.C("brt", "SPLIT PAIR CONTRACT") + grammar.C("mut", " — split/context is a layout surface, not command lore") + "\n")
	for _, row := range referenceSplitRows() {
		b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-10s", row.label)) +
			grammar.C(row.token, clipRunes(row.value, maxVisible(12, w-13))) + "\n")
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("brt", "REGISTRY CONTRACT") + grammar.C("mut", " — every screen must be discoverable, cycleable, and authority-labeled") + "\n")
	for _, row := range []contextRow{
		{"discover", "title hotlist + :windows + :help", "yel"},
		{"cycle", "[←/→] traverses the registered order", "yel"},
		{"scope", "engine vs instance is visible", "pri"},
		{"lifecycle", "SDLC/RDLC/n-DLC fit lives in domains", "org"},
		{"authority", "windows project; commands govern effects", "2nd"},
	} {
		b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-10s", row.label)) +
			grammar.C(row.token, clipRunes(row.value, maxVisible(12, w-13))) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderYardCockpit(w int) string {
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	b.WriteString(" " + grammar.C("brt", "YARD") + grammar.C("mut", " — Trainyard SDLC cockpit over live Reins read models") + "\n")
	b.WriteString(" " + grammar.C("yel", "read-only projection") + grammar.C("mut", " · no claim/close/dispatch · no transcript/PTY bridge · [Y]/:yard") + "\n")
	b.WriteString(" " + grammar.C("2nd", "sources") + " " +
		m.readSourceChip("events", len(m.Events), m.EventsDark, m.EventsSeq) + grammar.C("mut", " · ") +
		m.readSourceChip("tasks", len(m.Tasks), m.TasksDark, m.TasksSeq) + grammar.C("mut", " · ") +
		m.readSourceChip("sessions", len(m.Sessions), m.SessionsDark, m.SessionsSeq) + grammar.C("mut", " · ") +
		m.readSourceChip("lifecycles", len(m.Domains.Lifecycles), m.DomainsDark, m.DomainsSeq) + grammar.C("mut", " · ") +
		m.readSourceChip("domains", len(m.Domains.Rows), m.DomainsDark, m.DomainsSeq) + grammar.C("mut", " · ") +
		m.readSourceChip("dynamics", len(m.Dynamics.AtResolution(m.DynScale).Nodes), m.DynamicsDark, m.DynamicsSeq) + "\n")
	b.WriteString(rule + "\n")

	if m.sessionSplit() && !m.SuppressSplitPinned {
		b.WriteString(m.renderSelectedYardLane(w) + "\n")
		b.WriteString(rule + "\n")
	}

	b.WriteString(m.renderYardRailTopology(w) + "\n")
	b.WriteString(rule + "\n")

	// The rail topology, DRAWN: the octolinear SDLC metro-map. The textual RAIL TOPOLOGY above is
	// its legend; this is the map itself — state in shape+position (gates ►▷■✖◌, train physics,
	// blocked→siding, WITNESS terminus), color a redundant amplifier.
	b.WriteString(" " + grammar.C("brt", "TRAINYARD MAP") + grammar.C("mut", " — the rail topology drawn (octolinear; reads in grayscale)") + "\n")
	b.WriteString(grammar.RenderTrainyard(grammar.Trainyard{Tasks: m.Tasks}, w) + "\n")
	b.WriteString(rule + "\n")

	b.WriteString(" " + grammar.C("brt", "LADDER") + grammar.C("mut", " — lifecycle shape, not a task table") + "\n")
	stageCounts, hiddenStages, unstaged := m.yardStageCounts()
	parts := make([]string, 0, len(stageCounts))
	for i, n := range stageCounts {
		tok := "mut"
		if n > 0 {
			tok = "pri"
		}
		parts = append(parts, grammar.C(tok, fmt.Sprintf("S%d:%d", i, n)))
	}
	b.WriteString(" " + strings.Join(parts, grammar.C("mut", "  ")) + "\n")
	if hiddenStages > 0 || unstaged > 0 {
		b.WriteString(" " + grammar.C("2nd", "stage quality") + grammar.C("mut", fmt.Sprintf(" hidden:%d · unstaged:%d", hiddenStages, unstaged)) + "\n")
	}
	b.WriteString(rule + "\n")

	visibleBlocked, hiddenBlocked := m.yardBlockedIndices()
	hotSessions, hiddenHot := m.yardHotSessionIndices()
	failures, hiddenFailures := m.yardFailureEventIndices()
	b.WriteString(" " + grammar.C("brt", "ATTENTION RAIL") + grammar.C("mut", " — blockers, hot lanes, fresh failures") + "\n")
	if len(visibleBlocked) == 0 && hiddenBlocked == 0 {
		b.WriteString(" tasks clear · no visible release blocker in current read model\n")
	} else {
		for i, idx := range visibleBlocked {
			if i >= 5 {
				break
			}
			t := m.Tasks[idx]
			id := taskFieldValueForAir(t, "task_id", m.AIR)
			stage := taskFieldValueForAir(t, "stage", m.AIR)
			next := taskFieldValueForAir(t, "predicted_stage", m.AIR)
			crit := taskFieldValueForAir(t, "criticality", m.AIR)
			if w < 132 {
				writeWrappedKV(&b, "task hold", fmt.Sprintf("task=%s · crit=%s · stage=%s -> %s", id, crit, shortStage2(stage), next), airSeverityToken(t.Criticality, t.AIR, m.AIR), w)
			} else {
				b.WriteString(" " + grammar.C("red", "!") +
					" " + grammar.C(airSeverityToken(t.Criticality, t.AIR, m.AIR), fmt.Sprintf("%-5s", clipRunes(crit, 5))) +
					grammar.C("2nd", fmt.Sprintf(" %-5s → %-5s", clipRunes(shortStage2(stage), 5), clipRunes(next, 5))) +
					" " + grammar.C("pri", clipRunes(id, maxVisible(12, w-24))) + "\n")
			}
		}
		if hiddenBlocked > 0 {
			b.WriteString(" " + grammar.C("mut", fmt.Sprintf("▒ %d task blockers hidden by AIR policy", hiddenBlocked)) + "\n")
		}
	}
	if len(hotSessions) == 0 && hiddenHot == 0 {
		b.WriteString(" lanes calm · no visible lane above attention threshold\n")
	} else {
		for i, idx := range hotSessions {
			if i >= 4 {
				break
			}
			s := m.Sessions[idx]
			role := sessionFieldValueForAir(s, "role", m.AIR)
			ready := sessionFieldValueForAir(s, "readiness", m.AIR)
			blocker := sessionFieldValueForAir(s, "blocker", m.AIR)
			routeChip := sessionRouteChip(s, m.AIR)
			blockerTail := blocker
			if routeChip != "" {
				blockerTail = routeChip + " · " + blocker
			}
			if w < 132 {
				detail := fmt.Sprintf("lane=%s · readiness=%s · attn=%s", role, ready, sessionFieldValueForAir(s, "attention", m.AIR))
				if routeChip != "" {
					detail += " · route=" + routeChip
				}
				detail += " · blocker=" + blocker
				writeWrappedKV(&b, role, detail, airHue(readinessPaneToken(s.Readiness), s.AIR, "readiness", m.AIR), w)
			} else {
				b.WriteString(" " + grammar.C("yel", "lane") +
					" " + grammar.C(airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR), fmt.Sprintf("%-12s", clipRunes(role, 12))) +
					grammar.C(airHue(readinessPaneToken(s.Readiness), s.AIR, "readiness", m.AIR), fmt.Sprintf(" %-7s", clipRunes(ready, 7))) +
					grammar.C(airHue(attentionToken(s.Attention), s.AIR, "attention", m.AIR), fmt.Sprintf(" attn:%s", sessionFieldValueForAir(s, "attention", m.AIR))) +
					grammar.C("mut", " · "+clipRunes(blockerTail, maxVisible(8, w-41))) + "\n")
			}
		}
		if hiddenHot > 0 {
			b.WriteString(" " + grammar.C("mut", fmt.Sprintf("▒ %d hot lanes hidden by AIR policy", hiddenHot)) + "\n")
		}
	}
	if len(failures) == 0 && hiddenFailures == 0 {
		b.WriteString(" failure stream clear · no visible launch/review failures in recent events\n")
	} else {
		for i, idx := range failures {
			if i >= 3 {
				break
			}
			ev := m.Events[idx]
			ts := eventFieldValueForAir(ev, "ts", ev.TS, m.AIR)
			kind := eventFieldValueForAir(ev, "kind", ev.Kind, m.AIR)
			subj := eventFieldValueForAir(ev, "subject", ev.Subject, m.AIR)
			if w < 132 {
				writeWrappedKV(&b, "fail", fmt.Sprintf("event=%s · kind=%s · ts=%s", subj, kind, ts), "red", w)
			} else {
				b.WriteString(" " + grammar.C("red", "fail") +
					" " + grammar.C("2nd", fmt.Sprintf("%-8s", clipRunes(ts, 8))) +
					" " + grammar.C("pri", clipRunes(subj, maxVisible(10, w-34))) +
					grammar.C("mut", " · "+clipRunes(kind, 36)) + "\n")
			}
		}
		if hiddenFailures > 0 {
			b.WriteString(" " + grammar.C("mut", fmt.Sprintf("▒ %d failures hidden by AIR policy", hiddenFailures)) + "\n")
		}
	}
	b.WriteString(rule + "\n")

	b.WriteString(" " + grammar.C("brt", "FLEET MATRIX") + grammar.C("mut", " — lane readiness and platform shape") + "\n")
	fleet := m.yardFleetCounts()
	b.WriteString(" " +
		yardCount("total", len(m.Sessions), "pri") + "  " +
		yardCount("claim", fleet.claim, countToken(fleet.claim)) + "  " +
		yardCount("stale", fleet.stale, countWarnToken(fleet.stale)) + "  " +
		yardCount("off", fleet.off, countWarnToken(fleet.off)) + "  " +
		yardCount("live", fleet.live, "grn") + "  " +
		yardCount("stalled", fleet.stalled, countWarnToken(fleet.stalled)) + "  " +
		yardCount("codex", fleet.codex, "blu") + "  " +
		yardCount("claude", fleet.claude, "org") + "\n")
	if fleet.hidden > 0 {
		b.WriteString(" " + grammar.C("mut", fmt.Sprintf("▒ %d fleet attributes hidden by AIR policy", fleet.hidden)) + "\n")
	}
	b.WriteString(rule + "\n")

	darkSources := m.yardDarkSourceCount()
	b.WriteString(" " + grammar.C("brt", "GATES / READINESS") + grammar.C("mut", " — what must be true before action") + "\n")
	b.WriteString(" " + yardCount("release-holds", len(visibleBlocked), countWarnToken(len(visibleBlocked))) +
		grammar.C("mut", fmt.Sprintf(" hidden:%d", hiddenBlocked)) + "\n")
	b.WriteString(" " + yardCount("dark-sources", darkSources, countWarnToken(darkSources)) +
		grammar.C("mut", fmt.Sprintf(" · dynamics nodes:%d · edges:%d", len(m.Dynamics.AtResolution(m.DynScale).Nodes), len(m.Dynamics.AtResolution(m.DynScale).Edges))) + "\n")
	b.WriteString(" " + grammar.C("yel", "governed COMMAND route required") +
		grammar.C("mut", " · authority/preflight/receipt must be visible before mutation") + "\n")
	b.WriteString(rule + "\n")

	b.WriteString(" " + grammar.C("brt", "REPRESENTATION") + grammar.C("mut", " — form follows data shape") + "\n")
	b.WriteString(" " + grammar.C("2nd", "ladder") + grammar.C("mut", " lifecycle · ") +
		grammar.C("2nd", "rail") + grammar.C("mut", " priority · ") +
		grammar.C("2nd", "matrix") + grammar.C("mut", " fleet · ") +
		grammar.C("2nd", "stream") + grammar.C("mut", " arrivals · ") +
		grammar.C("2nd", "graph") + grammar.C("mut", " topology/liveness") + "\n")
	b.WriteString(" " + grammar.C("mut", "next parity surfaces: Trainyard attention drilldown, capability routing, RDLC/labrack, intake observations"))
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderYardRailTopology(w int) string {
	var b strings.Builder
	b.WriteString(" " + grammar.C("brt", "RAIL TOPOLOGY") + grammar.C("mut", " — stations, signals, lanes, throat, witness") + "\n")
	writeWrappedKV(&b, "stations", m.yardStationSummary(), "pri", w)
	writeWrappedKV(&b, "signals", "gate signals · "+m.yardGateSignalSummary(3), "yel", w)
	writeWrappedKV(&b, "lane lines", m.yardLaneLineSummary(), "2nd", w)
	writeWrappedKV(&b, "operator line", "read-only above; governed COMMAND route required below; no claim/close/dispatch/PTY", "org", w)
	writeWrappedKV(&b, "throat", m.yardThroatSummary(), "yel", w)
	writeWrappedKV(&b, "witness", "witness terminus · "+m.yardWitnessTerminus(), "blu", w)
	writeWrappedKV(&b, "dark", "dark sources · "+m.yardDarkSourceSummary(), countWarnToken(m.yardDarkSourceCount()), w)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) yardStationSummary() string {
	counts, hidden, unstaged := m.yardStageCounts()
	parts := make([]string, 0, len(counts)+2)
	for i, n := range counts {
		if n == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("S%d:%d", i, n))
	}
	if len(parts) == 0 {
		parts = append(parts, "empty")
	}
	if hidden > 0 {
		parts = append(parts, fmt.Sprintf("hidden:%d", hidden))
	}
	if unstaged > 0 {
		parts = append(parts, fmt.Sprintf("unstaged:%d", unstaged))
	}
	return strings.Join(parts, " · ")
}

func (m Model) yardGateSignalSummary(limit int) string {
	if len(m.Gates.Rows) == 0 {
		return "no /read/gates rows; route readiness inferred from tasks, sessions, events"
	}
	if limit <= 0 || limit > len(m.Gates.Rows) {
		limit = len(m.Gates.Rows)
	}
	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		row := m.Gates.Rows[i]
		gateID := gateRowFieldForAir(row, "gate_id", m.AIR)
		state := gateRowFieldForAir(row, "state", m.AIR)
		severity := gateRowFieldForAir(row, "severity", m.AIR)
		missing := gateRowFieldForAir(row, "missing", m.AIR)
		action := gateRowFieldForAir(row, "action", m.AIR)
		detail := strings.Join(nonEmptyParts([]string{
			gateID,
			"state=" + firstNonEmpty(state, "unknown"),
			"sev=" + firstNonEmpty(severity, "unknown"),
			"missing=" + firstNonEmpty(missing, "none"),
			"next=" + firstNonEmpty(action, "inspect"),
		}), " ")
		parts = append(parts, detail)
	}
	if len(m.Gates.Rows) > limit {
		parts = append(parts, fmt.Sprintf("+%d more", len(m.Gates.Rows)-limit))
	}
	return strings.Join(parts, " · ")
}

func (m Model) yardLaneLineSummary() string {
	fleet := m.yardFleetCounts()
	routed, routeHidden := 0, 0
	for _, s := range m.Sessions {
		if strings.TrimSpace(s.RouteID) == "" {
			continue
		}
		if m.AIR && s.AIR["route_id"] != "ok" {
			routeHidden++
			continue
		}
		routed++
	}
	parts := []string{
		fmt.Sprintf("live:%d", fleet.live),
		fmt.Sprintf("claim:%d", fleet.claim),
		fmt.Sprintf("stale:%d", fleet.stale),
		fmt.Sprintf("off:%d", fleet.off),
		fmt.Sprintf("stalled:%d", fleet.stalled),
		fmt.Sprintf("routed:%d", routed),
	}
	if fleet.hidden > 0 || routeHidden > 0 {
		parts = append(parts, fmt.Sprintf("hidden:%d", fleet.hidden+routeHidden))
	}
	return strings.Join(parts, " · ")
}

func (m Model) yardThroatSummary() string {
	blocked, preview := 0, 0
	for _, row := range m.Gates.Rows {
		state := strings.ToLower(row.State)
		switch {
		case strings.Contains(state, "block"):
			blocked++
		case strings.Contains(state, "preview"):
			preview++
		}
	}
	if n := m.Gates.Totals["blocked"]; n > blocked {
		blocked = n
	}
	if n := m.Gates.Totals["preview"]; n > preview {
		preview = n
	}
	return fmt.Sprintf("route binding + authority/preflight/receipt · gate rows:%d · blocked:%d · preview:%d", len(m.Gates.Rows), blocked, preview)
}

func (m Model) yardWitnessTerminus() string {
	for i := len(m.Events) - 1; i >= 0; i-- {
		ev := m.Events[i]
		kind := eventFieldValueForAir(ev, "kind", ev.Kind, m.AIR)
		subj := eventFieldValueForAir(ev, "subject", ev.Subject, m.AIR)
		actor := eventFieldValueForAir(ev, "actor", ev.Actor, m.AIR)
		ts := eventFieldValueForAir(ev, "ts", ev.TS, m.AIR)
		return "latest event " + strings.Join(nonEmptyParts([]string{
			"kind=" + firstNonEmpty(kind, "unknown"),
			"subject=" + firstNonEmpty(subj, "unknown"),
			"actor=" + firstNonEmpty(actor, "unknown"),
			"ts=" + firstNonEmpty(ts, "unknown"),
		}), " · ")
	}
	for _, row := range m.Gates.Rows {
		evidence := gateRowFieldForAir(row, "evidence", m.AIR)
		if strings.TrimSpace(evidence) == "" {
			continue
		}
		return "gate evidence " + gateRowFieldForAir(row, "gate_id", m.AIR) + " · " + evidence
	}
	return "◇ quiet/no witness in current read model"
}

func (m Model) yardDarkSourceSummary() string {
	return strings.Join([]string{
		yardDarkMark("events", m.EventsDark),
		yardDarkMark("tasks", m.TasksDark),
		yardDarkMark("sessions", m.SessionsDark),
		yardDarkMark("gates", m.GatesDark),
		yardDarkMark("domains", m.DomainsDark),
		yardDarkMark("dynamics", m.DynamicsDark),
	}, " ")
}

func (m Model) yardDarkSourceCount() int {
	n := 0
	for _, dark := range []bool{m.EventsDark, m.TasksDark, m.SessionsDark, m.GatesDark, m.DomainsDark, m.DynamicsDark} {
		if dark {
			n++
		}
	}
	return n
}

func yardDarkMark(label string, dark bool) string {
	if dark {
		return label + ":◇"
	}
	return label + ":ok"
}

func (m Model) renderSelectedYardLane(w int) string {
	s, ok := m.FocusedSession()
	if !ok {
		return " " + grammar.C("brt", "SELECTED TRAINYARD LANE") + grammar.C("mut", " — no selected session source")
	}
	claim := strings.TrimSpace(s.ClaimedTask)
	claimOK := !m.AIR || s.AIR["claimed_task"] == "ok"
	taskState, taskTok := "no claimed task", "mut"
	taskStage, taskNext := "", ""
	var linkedTask grammar.Task
	hasTask := false
	if !claimOK {
		taskState, taskTok = "▒▒▒", "mut" // the visible/gap state discloses the denied claimed_task
	} else if claim != "" {
		if t, found := m.taskByID(claim); found {
			linkedTask, hasTask = t, true
			taskState, taskTok = "task visible", "grn"
			taskStage = taskFieldValueForAir(t, "stage", m.AIR)
			taskNext = taskFieldValueForAir(t, "predicted_stage", m.AIR)
		} else {
			taskState, taskTok = "task gap", "red"
		}
	}
	related := m.sessionRelatedEvents(s)
	failures, successes := 0, 0
	for _, ev := range related {
		if m.AIR && ev.AIR["kind"] != "ok" {
			continue // a denied kind must not classify into the fail/succeed tally
		}
		kind := strings.ToLower(ev.Kind)
		if strings.Contains(kind, "fail") {
			failures++
		}
		if strings.Contains(kind, "succeed") {
			successes++
		}
	}
	blocker := sessionFieldValueForAir(s, "blocker", m.AIR)
	if strings.TrimSpace(blocker) == "" {
		blocker = "none"
	}
	gate, gateTok := yardLaneGate(s, hasTask, linkedTask)
	if m.AIR {
		// the gate derives from readiness + blocker + the linked task's predicted_stage; redact it if
		// any of those inputs is denied (else the gate label discloses the hidden field).
		if s.AIR["readiness"] != "ok" || (hasTask && linkedTask.AIR["predicted_stage"] != "ok") {
			gate, gateTok = "▒▒▒", "mut"
		} else if s.AIR["blocker"] != "ok" {
			gate, gateTok = "▒▒▒", "mut"
		}
	}
	var b strings.Builder
	line := func(label, value, tok string) {
		if strings.TrimSpace(value) == "" {
			value, tok = "·", "mut"
		}
		writeWrappedKV(&b, label, value, tok, w)
	}
	// hue-gate: a redacted value must not keep its derived color (the hue is itself a derived channel).
	laneTok, rdyTok, stateTok, attnTok := airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR), airHue(readinessPaneToken(s.Readiness), s.AIR, "readiness", m.AIR), airHue(sessionStateToken(s.State), s.AIR, "state", m.AIR), airHue(attentionToken(s.Attention), s.AIR, "attention", m.AIR)
	writeSectionHeader(&b, w, "SELECTED TRAINYARD LANE", "session-driven drilldown; evidence, not control", "lane drilldown")
	line("lane", sessionFieldValueForAir(s, "role", m.AIR), laneTok)
	line("platform", sessionFieldValueForAir(s, "platform", m.AIR), "2nd")
	line("readiness", sessionFieldValueForAir(s, "readiness", m.AIR), rdyTok)
	line("state", sessionFieldValueForAir(s, "state", m.AIR), stateTok)
	line("attention", sessionFieldValueForAir(s, "attention", m.AIR), attnTok)
	line("blocker", blocker, airHue(blockerToken(blocker), s.AIR, "blocker", m.AIR))
	line("claimed", sessionFieldValueForAir(s, "claimed_task", m.AIR), "pri")
	line("task", taskState, taskTok)
	if hasTask {
		line("stage", taskStage, airSeverityToken(linkedTask.Criticality, linkedTask.AIR, m.AIR))
		line("next", taskNext, airHue(nextToken(linkedTask.PredictedStage), linkedTask.AIR, "predicted_stage", m.AIR))
	}
	line("events", fmt.Sprintf("%d actor/task events", len(related)), countToken(len(related)))
	line("failures", fmt.Sprintf("%d recent", failures), severityCountToken(failures))
	line("successes", fmt.Sprintf("%d recent", successes), "grn")
	line("gate", gate, gateTok)
	fit, fitTok := capabilitySessionFit(s, m.AIR)
	line("fit", fit, fitTok)
	routeText, routeTok := m.selectedLaneRoutePosture(s)
	line("routes", routeText, routeTok)
	capText, capTok := m.selectedLaneCapabilityPosture(s)
	line("capability", capText, capTok)
	toolText, toolTok := m.selectedLaneToolPosture(s)
	line("tools", toolText, toolTok)
	gateText, gatePostureTok := m.selectedLaneGatePosture()
	line("gate rows", gateText, gatePostureTok)
	line("legal next", ":intent show-route · :intent open-trace", "yel")
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) selectedLaneRoutePosture(s grammar.Session) (string, string) {
	if m.AIR && s.AIR["platform"] != "ok" {
		return "platform hidden by AIR; route match unavailable", "mut"
	}
	if m.AIR && s.RouteID != "" && s.AIR["route_id"] != "ok" {
		return "route binding hidden by AIR", "mut"
	}
	routeID := strings.TrimSpace(s.RouteID)
	bindingState := strings.TrimSpace(s.RouteBindingState)
	if routeID != "" {
		if route, ok := m.capabilityRouteByID(routeID); ok {
			tok := capabilityRouteToken(route)
			if bindingState == "policy_only" && tok == "grn" {
				tok = "yel"
			}
			return fmt.Sprintf("%s route %s · %s/%s · %s · receipts:%d",
				routeBindingLabel(bindingState), routeID, route.Mode, route.Profile, route.RouteState, route.ReceiptCount), tok
		}
		return fmt.Sprintf("%s route %s · descriptor missing from capability registry", routeBindingLabel(bindingState), routeID), "red"
	}
	if bindingState != "" && bindingState != "no_claim" && bindingState != "unbound" {
		return fmt.Sprintf("%s; no route_id", routeBindingLabel(bindingState)), routeBindingToken(bindingState)
	}
	platform := strings.TrimSpace(s.Platform)
	if platform == "" {
		return "missing platform; descriptor required before routing", "red"
	}
	if len(m.Capabilities.Routes) == 0 {
		if len(m.Capabilities.Rows) > 0 {
			return "no route rows; capability status is aggregate only", "yel"
		}
		return "route evidence read-missing", "red"
	}
	matches := m.capabilityRoutesForPlatform(platform)
	if len(matches) == 0 {
		return fmt.Sprintf("0 %s routes; route evidence not bound to lane", platform), "red"
	}
	fresh, blocked, receipts := 0, 0, 0
	for _, route := range matches {
		receipts += route.ReceiptCount
		switch capabilityRouteToken(route) {
		case "grn":
			fresh++
		case "red":
			blocked++
		}
	}
	tok := "yel"
	if blocked > 0 || fresh == 0 {
		tok = "red"
	} else if fresh == len(matches) {
		tok = "grn"
	}
	return fmt.Sprintf("%d %s routes · %d fresh · %d blocked · receipts:%d", len(matches), platform, fresh, blocked, receipts), tok
}

func (m Model) selectedLaneCapabilityPosture(s grammar.Session) (string, string) {
	if len(m.Capabilities.Rows) == 0 {
		return "capability status read-missing; lane fit is platform-derived", "red"
	}
	if m.AIR && s.AIR["platform"] != "ok" {
		return "platform hidden by AIR; capability binding unavailable", "mut"
	}
	matches := m.capabilityRoutesForPlatform(strings.TrimSpace(s.Platform))
	if len(matches) == 0 {
		return fmt.Sprintf("%d aggregate capability rows; no lane route binding", len(m.Capabilities.Rows)), "yel"
	}
	parts := make([]string, 0, len(matches))
	worst := "grn"
	seen := map[string]bool{}
	for _, route := range matches {
		capID := strings.TrimSpace(capabilityRouteFieldForAir(route, "capability_id", m.AIR))
		if capID == "" {
			capID = "capability_id missing"
		}
		if seen[capID] {
			continue
		}
		seen[capID] = true
		status, tok := "route-only", "yel"
		if row, ok := m.capabilityRowByID(route.CapabilityID); ok {
			status = capabilityRowFieldForAir(row, "status", m.AIR)
			tok = capabilityStatusToken(row.Status)
		}
		if tok == "red" || (tok == "org" && worst != "red") || (tok == "yel" && worst == "grn") {
			worst = tok
		}
		parts = append(parts, fmt.Sprintf("%s:%s", capID, status))
	}
	if len(parts) == 0 {
		return "route rows lack capability ids; platform-only evidence", "red"
	}
	if len(parts) > 3 {
		parts = append(parts[:3], fmt.Sprintf("+%d", len(parts)-3))
	}
	return strings.Join(parts, " · "), worst
}

func (m Model) selectedLaneToolPosture(s grammar.Session) (string, string) {
	if m.AIR && s.AIR["platform"] != "ok" {
		return "platform hidden by AIR; candidate tools unavailable", "mut"
	}
	if m.AIR && s.RouteID != "" && s.AIR["route_id"] != "ok" {
		return "route binding hidden by AIR; tool binding unavailable", "mut"
	}
	platform := strings.TrimSpace(s.Platform)
	if platform == "" {
		return "missing platform; candidate tools unavailable", "red"
	}
	if len(m.Capabilities.Tools) == 0 {
		return "route tool evidence read-missing; exact per-session needs route_id", "red"
	}
	routeID := strings.TrimSpace(s.RouteID)
	bindingState := strings.TrimSpace(s.RouteBindingState)
	toolScope := fmt.Sprintf("candidate %s route", platform)
	needsSuffix := " · exact per-session needs route_id"
	matches := []grammar.CapabilityTool(nil)
	if routeID != "" {
		matches = m.capabilityToolsForRoute(routeID)
		toolScope = fmt.Sprintf("%s route", routeBindingLabel(bindingState))
		if bindingState == "bound" {
			needsSuffix = ""
		} else {
			needsSuffix = " · launch not session-confirmed"
		}
		if len(matches) == 0 {
			return fmt.Sprintf("0 %s tools for %s%s", toolScope, routeID, needsSuffix), routeBindingToken(bindingState)
		}
	} else {
		matches = m.capabilityToolsForPlatform(platform)
	}
	if len(matches) == 0 {
		return fmt.Sprintf("0 candidate %s route tools; exact per-session needs route_id", platform), "yel"
	}
	observed, missing, unavailable := 0, 0, 0
	parts := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, tool := range matches {
		switch capabilityToolToken(tool) {
		case "grn":
			observed++
		case "red":
			if strings.EqualFold(tool.Status, "unavailable") || !tool.Available {
				unavailable++
			} else {
				missing++
			}
		default:
			missing++
		}
		id := strings.TrimSpace(capabilityToolFieldForAir(tool, "tool_id", m.AIR))
		if id == "" {
			id = "tool"
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		if len(parts) < 4 {
			parts = append(parts, fmt.Sprintf("%s:%s", id, capabilityToolFieldForAir(tool, "status", m.AIR)))
		}
	}
	if len(seen) > len(parts) {
		parts = append(parts, fmt.Sprintf("+%d", len(seen)-len(parts)))
	}
	tok := "grn"
	if unavailable > 0 {
		tok = "red"
	} else if missing > 0 {
		tok = "yel"
	}
	return fmt.Sprintf("%d %s tools · observed:%d missing:%d unavailable:%d · %s%s",
		len(matches), toolScope, observed, missing, unavailable, strings.Join(parts, " · "), needsSuffix), tok
}

func (m Model) selectedLaneGatePosture() (string, string) {
	if len(m.Gates.Rows) == 0 {
		return "no source gate rows; using derived lane/task gates", "yel"
	}
	taskRows, laneRows, routeRows, commandRows, blocked, preview := 0, 0, 0, 0, 0, 0
	for _, row := range m.Gates.Rows {
		switch row.Domain {
		case "task":
			taskRows++
		case "lane":
			laneRows++
		case "route":
			routeRows++
		case "command":
			commandRows++
		}
		if strings.EqualFold(row.State, "blocked") || strings.EqualFold(row.State, "missing") || gateRowToken(row) == "red" {
			blocked++
		}
		if strings.EqualFold(row.State, "preview-only") || strings.EqualFold(row.State, "preview_only") {
			preview++
		}
	}
	tok := "yel"
	if blocked > 0 {
		tok = "red"
	}
	return fmt.Sprintf("aggregate gates task:%d lane:%d route:%d command:%d blocked:%d preview:%d", taskRows, laneRows, routeRows, commandRows, blocked, preview), tok
}

func (m Model) capabilityRoutesForPlatform(platform string) []grammar.CapabilityRoute {
	if strings.TrimSpace(platform) == "" {
		return nil
	}
	out := make([]grammar.CapabilityRoute, 0)
	for _, route := range m.Capabilities.Routes {
		if strings.EqualFold(route.Platform, platform) {
			out = append(out, route)
		}
	}
	return out
}

func (m Model) capabilityRouteByID(routeID string) (grammar.CapabilityRoute, bool) {
	if strings.TrimSpace(routeID) == "" {
		return grammar.CapabilityRoute{}, false
	}
	for _, route := range m.Capabilities.Routes {
		if route.RouteID == routeID {
			return route, true
		}
	}
	return grammar.CapabilityRoute{}, false
}

func (m Model) capabilityToolsForPlatform(platform string) []grammar.CapabilityTool {
	if strings.TrimSpace(platform) == "" {
		return nil
	}
	out := make([]grammar.CapabilityTool, 0)
	for _, tool := range m.Capabilities.Tools {
		if strings.EqualFold(tool.Platform, platform) {
			out = append(out, tool)
		}
	}
	return out
}

func (m Model) capabilityToolsForRoute(routeID string) []grammar.CapabilityTool {
	if strings.TrimSpace(routeID) == "" {
		return nil
	}
	out := make([]grammar.CapabilityTool, 0)
	for _, tool := range m.Capabilities.Tools {
		if tool.RouteID == routeID {
			out = append(out, tool)
		}
	}
	return out
}

func routeBindingLabel(state string) string {
	switch strings.TrimSpace(state) {
	case "bound":
		return "bound"
	case "policy_only":
		return "policy-only"
	case "eligible_not_launched":
		return "eligible-not-launched"
	case "mq_unbound":
		return "mq-unbound"
	case "launch_failed":
		return "launch-failed"
	case "policy_refused":
		return "policy-refused"
	case "policy_held":
		return "policy-held"
	case "platform_mismatch":
		return "platform-mismatch"
	case "source_missing":
		return "source-missing"
	case "source_unreadable":
		return "source-unreadable"
	case "no_claim":
		return "no-claim"
	case "unbound":
		return "unbound"
	case "":
		return "unbound"
	default:
		return state
	}
}

func routeBindingToken(state string) string {
	switch strings.TrimSpace(state) {
	case "bound":
		return "grn"
	case "policy_only", "eligible_not_launched", "mq_unbound", "unbound", "no_claim":
		return "yel"
	case "launch_failed", "policy_refused", "platform_mismatch", "source_missing", "source_unreadable":
		return "red"
	case "policy_held":
		return "org"
	}
	return "mut"
}

func yardLaneGate(s grammar.Session, hasTask bool, t grammar.Task) (string, string) {
	blocker := strings.TrimSpace(s.Blocker)
	if blocker != "" && blocker != "none" {
		return "blocked by " + blocker, "red"
	}
	switch s.Readiness {
	case "off", "offline":
		return "offline; no resume", "red"
	case "stale":
		return "verify relay before action", "yel"
	case "claim":
		if hasTask && strings.EqualFold(t.PredictedStage, "hold") {
			return "release hold visible", "red"
		}
		if hasTask {
			return "resume preflight visible", "yel"
		}
		return "claim without visible task", "red"
	}
	if hasTask {
		return "observe task context", "2nd"
	}
	return "observe lane only", "2nd"
}

func (m Model) renderReadinessProjection(w int) string {
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	visibleBlocked, hiddenBlocked := m.yardBlockedIndices()
	fleet := m.yardFleetCounts()
	hasGates := len(m.Gates.Rows) > 0 || len(m.Gates.Sources) > 0
	writeSectionHeader(&b, w, "READINESS", "gates/readiness projection over live read sources", "gate stack")
	b.WriteString(" " + grammar.C("yel", "read-only projection") + grammar.C("mut", " · no claim/close/dispatch · route receipts required before mutation · [R]/:readiness") + "\n")
	b.WriteString(" " + grammar.C("2nd", "sources") + " " +
		m.readSourceChip("gates", len(m.Gates.Rows), m.GatesDark, m.GatesSeq) + grammar.C("mut", " · ") +
		m.readSourceChip("tasks", len(m.Tasks), m.TasksDark, m.TasksSeq) + grammar.C("mut", " · ") +
		m.readSourceChip("sessions", len(m.Sessions), m.SessionsDark, m.SessionsSeq) + grammar.C("mut", " · ") +
		m.readSourceChip("events", len(m.Events), m.EventsDark, m.EventsSeq) + grammar.C("mut", " · ") +
		m.readSourceChip("dynamics", len(m.Dynamics.AtResolution(m.DynScale).Nodes), m.DynamicsDark, m.DynamicsSeq) + "\n")
	b.WriteString(rule + "\n")

	if m.sessionSplit() && !m.SuppressSplitPinned {
		b.WriteString(m.renderSelectedReadinessGate(w) + "\n")
		b.WriteString(rule + "\n")
	}

	if hasGates {
		writeSectionHeader(&b, w, "GATE SOURCES", "task no_go, lane blockers, event log, and command route contracts", "source-backed gates")
		if len(m.Gates.Sources) == 0 {
			b.WriteString(" " + grammar.C("red", "no gate source rows returned") + "\n")
		}
		for _, src := range m.Gates.Sources {
			writeWrappedKV(&b, gateSourceFieldForAir(src, "id", m.AIR),
				fmt.Sprintf("source=%s · status=%s · count=%s · age=%s · detail=%s",
					gateSourceFieldForAir(src, "id", m.AIR),
					gateSourceFieldForAir(src, "status", m.AIR),
					gateSourceFieldForAir(src, "count", m.AIR),
					gateSourceFieldForAir(src, "age_bucket", m.AIR),
					gateSourceFieldForAir(src, "detail", m.AIR)),
				sourceStatusToken(src.Status), w)
		}
	} else {
		writeSectionHeader(&b, w, "SOURCE FRESHNESS", "dark sources must stay visible; unknown never renders green", "dark/unknown state")
		darkSources := 0
		for _, dark := range []bool{m.TasksDark, m.SessionsDark, m.EventsDark, m.DomainsDark, m.DynamicsDark, m.EpistemicsDark} {
			if dark {
				darkSources++
			}
		}
		b.WriteString(" " + yardCount("dark-sources", darkSources, countWarnToken(darkSources)) +
			grammar.C("mut", fmt.Sprintf(" · rx e%d t%d s%d o%d d%d p%d", m.EventsSeq%10, m.TasksSeq%10, m.SessionsSeq%10, m.DomainsSeq%10, m.DynamicsSeq%10, m.EpistemicsSeq%10)) + "\n")
		b.WriteString(" " + grammar.C("mut", "current projection is derived from task/session/event rows; no /read/gates contract yet") + "\n")
	}
	b.WriteString(rule + "\n")

	writeSectionHeader(&b, w, "TASK GATES", "raw no_go names and task authority gaps", "task holds/risk")
	taskGateRows := m.gateRowsByDomain("task")
	if len(taskGateRows) > 0 {
		m.renderGateRows(&b, taskGateRows, w, 10)
	} else {
		b.WriteString(" " + yardCount("visible", len(visibleBlocked), countWarnToken(len(visibleBlocked))) +
			grammar.C("mut", fmt.Sprintf(" hidden:%d", hiddenBlocked)) + "\n")
		if len(visibleBlocked) == 0 && hiddenBlocked == 0 {
			b.WriteString(" " + grammar.C("grn", "no visible task hold/risk rows") + "\n")
		} else {
			for i, idx := range visibleBlocked {
				if i >= 8 {
					break
				}
				t := m.Tasks[idx]
				reason, tok := taskGateReason(t)
				id := taskFieldValueForAir(t, "task_id", m.AIR)
				stage := taskFieldValueForAir(t, "stage", m.AIR)
				next := taskFieldValueForAir(t, "predicted_stage", m.AIR)
				if w < 132 {
					writeWrappedKV(&b, reason, fmt.Sprintf("gate=%s · task=%s · stage=%s -> %s", reason, id, shortStage2(stage), next), tok, w)
				} else {
					b.WriteString(" " + grammar.C(tok, "!") +
						grammar.C(tok, fmt.Sprintf(" %-17s", clipRunes(reason, 17))) +
						grammar.C("2nd", fmt.Sprintf(" %-5s → %-5s", clipRunes(shortStage2(stage), 5), clipRunes(next, 5))) +
						" " + grammar.C("pri", clipRunes(id, maxVisible(10, w-34))) + "\n")
				}
			}
		}
	}
	b.WriteString(rule + "\n")

	writeSectionHeader(&b, w, "LANE READINESS", "session readiness, blockers, and platform posture", "lane blockers")
	b.WriteString(" " +
		yardCount("claim", fleet.claim, countToken(fleet.claim)) + "  " +
		yardCount("stale", fleet.stale, countWarnToken(fleet.stale)) + "  " +
		yardCount("off", fleet.off, countWarnToken(fleet.off)) + "  " +
		yardCount("live", fleet.live, "grn") + "  " +
		yardCount("stalled", fleet.stalled, countWarnToken(fleet.stalled)) + "\n")
	if routeSummary := m.sessionRouteBindingSummary(); routeSummary != "" {
		b.WriteString(" " + grammar.C("2nd", "routes ") + grammar.C("mut", routeSummary) + "\n")
	}
	laneGateRows := m.gateRowsByDomain("lane")
	if len(laneGateRows) > 0 {
		m.renderGateRows(&b, laneGateRows, w, 8)
	} else {
		for _, row := range sessionBlockerRows(m.Sessions, w, m.AIR) {
			b.WriteString(row + "\n")
		}
	}
	b.WriteString(rule + "\n")

	routeGateRows := m.gateRowsByDomain("route")
	if len(routeGateRows) > 0 {
		writeSectionHeader(&b, w, "ROUTE BINDING", "route decision/session binding evidence", "route evidence")
		m.renderGateRows(&b, routeGateRows, w, 8)
		b.WriteString(rule + "\n")
	}

	writeSectionHeader(&b, w, "COMMAND ROUTE", "authority/preflight/receipt state before action", "route receipts")
	commandGateRows := m.gateRowsByDomain("command")
	if len(commandGateRows) > 0 {
		m.renderGateRows(&b, commandGateRows, w, 8)
	} else {
		for _, row := range []contextRow{
			{"resume", "preview only; no transcript/PTY/stdin bridge", "yel"},
			{"dispatch", "governed methodology route required", "yel"},
			{"claim/close", "cc-task authority + receipt required", "yel"},
			{"route envelope", "read-missing: model/effort/context/spend/quota/floor/veto", "red"},
			{"receipt", "not wired; mutation controls remain disabled", "red"},
		} {
			writeWrappedKV(&b, row.label, row.value, row.token, w)
		}
	}
	b.WriteString(rule + "\n")

	if hasGates {
		writeSectionHeader(&b, w, "GUARDRAILS", "what the gate contract does and does not authorize", "gate boundaries")
		for _, gap := range []string{
			"/read/gates preserves raw false no_go names and lane blockers; it does not clear or mutate them",
			"command route rows are preview contracts only; dispatch/claim/close/release still require governed receipts",
			"next parity: parent_spec, mutation_surface, route authority receipts, and preflight evidence as first-class gate rows",
		} {
			writeWrappedBullet(&b, gap, "mut", w)
		}
	} else {
		writeSectionHeader(&b, w, "GAPS / NEXT CONTRACT", "what a real gates endpoint must preserve", "missing gate contract")
		for _, gap := range []string{
			"raw gate booleans are collapsed before the TUI; exact false gate names need a /read/gates contract",
			"route authority receipts are not source-backed yet",
			"parent_spec, mutation_surface, preflight, and receipt refs should become first-class gate records",
		} {
			writeWrappedBullet(&b, gap, "mut", w)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderSelectedReadinessGate(w int) string {
	s, ok := m.FocusedSession()
	if !ok {
		return " " + grammar.C("brt", "SELECTED LANE GATE") + grammar.C("mut", " — no selected session source")
	}
	claim := strings.TrimSpace(s.ClaimedTask)
	claimOK := !m.AIR || s.AIR["claimed_task"] == "ok"
	taskState, taskTok := "no claimed task", "mut"
	taskGate, gateTok := "lane only", "2nd"
	var linkedTask grammar.Task
	hasTask := false
	if !claimOK {
		taskState, taskTok = "▒▒▒", "mut" // visible/gap/absent discloses the denied claimed_task
		taskGate, gateTok = "▒▒▒", "mut"
	} else if claim != "" {
		if t, found := m.taskByID(claim); found {
			linkedTask, hasTask = t, true
			taskState, taskTok = "task visible", "grn"
			taskGate, gateTok = taskGateReason(t)
		} else {
			taskState, taskTok = "task gap", "red"
			taskGate, gateTok = "claimed task absent", "red"
		}
	}
	laneGate, laneTok := yardLaneGate(s, hasTask, linkedTask)
	if m.AIR {
		if s.AIR["readiness"] != "ok" || (hasTask && linkedTask.AIR["predicted_stage"] != "ok") {
			laneGate, laneTok = "▒▒▒", "mut"
		} else if s.AIR["blocker"] != "ok" {
			laneGate, laneTok = "▒▒▒", "mut"
		}
	}
	laneHue, rdyHue, blkHue := airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR), airHue(readinessPaneToken(s.Readiness), s.AIR, "readiness", m.AIR), airHue(blockerToken(s.Blocker), s.AIR, "blocker", m.AIR)
	var b strings.Builder
	writeSectionHeader(&b, w, "SELECTED LANE GATE", "source lane, claimed task, route receipt posture", "lane gate")
	writeWrappedKV(&b, "lane", sessionFieldValueForAir(s, "role", m.AIR), laneHue, w)
	writeWrappedKV(&b, "readiness", sessionFieldValueForAir(s, "readiness", m.AIR), rdyHue, w)
	writeWrappedKV(&b, "blocker", sessionFieldValueForAir(s, "blocker", m.AIR), blkHue, w)
	writeWrappedKV(&b, "claimed", sessionFieldValueForAir(s, "claimed_task", m.AIR), "pri", w)
	writeWrappedKV(&b, "task", taskState, taskTok, w)
	writeWrappedKV(&b, "task gate", taskGate, gateTok, w)
	if hasTask {
		nextHue := airHue(nextToken(linkedTask.PredictedStage), linkedTask.AIR, "predicted_stage", m.AIR)
		writeWrappedKV(&b, "stage", taskFieldValueForAir(linkedTask, "stage", m.AIR), airSeverityToken(linkedTask.Criticality, linkedTask.AIR, m.AIR), w)
		writeWrappedKV(&b, "next", taskFieldValueForAir(linkedTask, "predicted_stage", m.AIR), nextHue, w)
	}
	writeWrappedKV(&b, "lane gate", laneGate, laneTok, w)
	writeWrappedKV(&b, "route", "governed COMMAND route required", "yel", w)
	writeWrappedKV(&b, "receipt", "not wired/read-missing", "red", w)
	writeWrappedKV(&b, "legal next", ":intent show-route · :intent open-trace", "yel", w)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderIntakeProjection(w int) string {
	if m.IntakeDark {
		return darkHint(m.IntakeError, m.AIR)
	}
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	writeSectionHeader(&b, w, "INTAKE", "source-backed intake observations and demand projection", "intake observations")
	b.WriteString(" " + grammar.C("yel", "read-only projection") + grammar.C("mut", " · no drain/dismiss/write · no raw Obsidian/notification bodies · [I]/:intake") + "\n")
	b.WriteString(" " + grammar.C("2nd", "sources") + " " +
		m.readSourceChip("intake", len(m.Intake.Rows), m.IntakeDark, m.IntakeSeq) + grammar.C("mut", " · ") +
		m.readSourceChip("events", len(m.Events), m.EventsDark, m.EventsSeq) + grammar.C("mut", " · ") +
		m.readSourceChip("tasks", len(m.Tasks), m.TasksDark, m.TasksSeq) + grammar.C("mut", " · ") +
		m.readSourceChip("sessions", len(m.Sessions), m.SessionsDark, m.SessionsSeq) + "\n")
	bucketKeys := "[j/k]bucket"
	detailKey := "[Enter]detail"
	if m.sessionSplit() {
		bucketKeys = "[n/p]bucket"
		detailKey = "[Enter]source detail"
	}
	b.WriteString(" " + grammar.C("2nd", "filter") + " " +
		grammar.C("pri", m.intakeFilterLabel()) + grammar.C("mut", fmt.Sprintf(" · %d/%d buckets · %s · [s/S]source · %s", len(m.visibleIntakeRows()), len(m.Intake.Rows), bucketKeys, detailKey)) + "\n")
	b.WriteString(rule + "\n")

	if m.sessionSplit() && !m.SuppressSplitPinned {
		b.WriteString(m.renderSelectedIntakeLane(w) + "\n")
		b.WriteString(rule + "\n")
	}

	b.WriteString(m.renderSelectedIntakeBucket(w) + "\n")
	b.WriteString(rule + "\n")

	writeSectionHeader(&b, w, "SOURCE FRESHNESS", "durable snapshots, metadata only; raw source bodies remain outside this model", "snapshot freshness")
	if len(m.Intake.Sources) == 0 {
		b.WriteString(" " + grammar.C("mut", "no intake source rows · /read/intake returned an empty projection") + "\n")
	} else if w < 102 {
		for _, src := range m.Intake.Sources {
			writeWrappedKV(&b, "source", intakeSourceFieldForAir(src, "id", m.AIR), "pri", w)
			writeWrappedKV(&b, "status", intakeSourceFieldForAir(src, "status", m.AIR), intakeStatusToken(src.Status), w)
			writeWrappedKV(&b, "count", intakeSourceFieldForAir(src, "count", m.AIR), countWarnToken(src.Count), w)
			writeWrappedKV(&b, "age", intakeSourceFieldForAir(src, "age_bucket", m.AIR), intakeAgeToken(src.AgeBucket), w)
			writeWrappedKV(&b, "path", intakeSourceFieldForAir(src, "path", m.AIR), "mut", w)
		}
	} else {
		b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-23s %-11s %7s %-8s %-14s %s", "SOURCE", "STATUS", "COUNT", "AGE", "PRIVACY", "PATH")) + "\n")
		for _, src := range m.Intake.Sources {
			b.WriteString(" " +
				grammar.C("pri", fmt.Sprintf("%-23s", clipRunes(intakeSourceFieldForAir(src, "id", m.AIR), 23))) +
				grammar.C(intakeStatusToken(src.Status), fmt.Sprintf(" %-11s", clipRunes(intakeSourceFieldForAir(src, "status", m.AIR), 11))) +
				grammar.C(countWarnToken(src.Count), fmt.Sprintf(" %7s", clipRunes(intakeSourceFieldForAir(src, "count", m.AIR), 7))) +
				grammar.C(intakeAgeToken(src.AgeBucket), fmt.Sprintf(" %-8s", clipRunes(intakeSourceFieldForAir(src, "age_bucket", m.AIR), 8))) +
				grammar.C("2nd", fmt.Sprintf(" %-14s", clipRunes(intakeSourceFieldForAir(src, "privacy", m.AIR), 14))) +
				grammar.C("mut", " "+clipRunes(intakeSourceFieldForAir(src, "path", m.AIR), maxVisible(8, w-72))) + "\n")
		}
	}
	b.WriteString(rule + "\n")

	writeSectionHeader(&b, w, "DEMAND TOTALS", "aggregate pressure from request, P0, planning, and security intake", "aggregate pressure")
	for _, row := range m.intakeTotalRows() {
		writeWrappedKV(&b, row.label, row.value, row.token, w)
	}
	b.WriteString(rule + "\n")

	writeSectionHeader(&b, w, "OBSERVATION BUCKETS", "coverage, staleness, incident, and security aggregates", "bucketed observations")
	rows := m.visibleIntakeRows()
	if len(rows) == 0 {
		b.WriteString(" " + grammar.C("mut", "no aggregate intake rows visible in current read model") + "\n")
	} else if w < 128 {
		for i, row := range rows {
			label := intakeRowFieldForAir(row, "kind", m.AIR)
			if i == m.IFocus {
				label = "▶ " + label
			}
			parts := []string{
				"kind=" + intakeRowFieldForAir(row, "kind", m.AIR),
				intakeRowFieldForAir(row, "source", m.AIR),
				intakeRowFieldForAir(row, "status", m.AIR),
				intakeRowFieldForAir(row, "severity", m.AIR),
				"n=" + intakeRowFieldForAir(row, "count", m.AIR),
				intakeRowFieldForAir(row, "coverage", m.AIR),
			}
			if blocker := strings.TrimSpace(intakeRowFieldForAir(row, "blocker", m.AIR)); blocker != "" {
				parts = append(parts, "blocker="+blocker)
			}
			writeWrappedKV(&b,
				label,
				strings.Join(parts, " · "),
				airHue(intakeSeverityToken(row.Severity), row.AIR, "severity", m.AIR), w)
		}
	} else {
		b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-22s %-28s %-12s %-8s %7s %-18s %s", "SOURCE", "KIND", "STATUS", "SEV", "COUNT", "COVERAGE", "BLOCKER")) + "\n")
		for i, row := range rows {
			rowLine := " " +
				grammar.C("2nd", fmt.Sprintf("%-22s", clipRunes(intakeRowFieldForAir(row, "source", m.AIR), 22))) +
				grammar.C("pri", fmt.Sprintf(" %-28s", clipRunes(intakeRowFieldForAir(row, "kind", m.AIR), 28))) +
				grammar.C(intakeStatusToken(row.Status), fmt.Sprintf(" %-12s", clipRunes(intakeRowFieldForAir(row, "status", m.AIR), 12))) +
				grammar.C(airHue(intakeSeverityToken(row.Severity), row.AIR, "severity", m.AIR), fmt.Sprintf(" %-8s", clipRunes(intakeRowFieldForAir(row, "severity", m.AIR), 8))) +
				grammar.C(countWarnToken(row.Count), fmt.Sprintf(" %7s", clipRunes(intakeRowFieldForAir(row, "count", m.AIR), 7))) +
				grammar.C("2nd", fmt.Sprintf(" %-18s", clipRunes(intakeRowFieldForAir(row, "coverage", m.AIR), 18))) +
				grammar.C("mut", " "+clipRunes(intakeRowFieldForAir(row, "blocker", m.AIR), maxVisible(8, w-105)))
			if i == m.IFocus {
				b.WriteString(focusBar(rowLine, w) + "\n")
			} else {
				b.WriteString(rowLine + "\n")
			}
		}
	}
	b.WriteString(rule + "\n")

	writeSectionHeader(&b, w, "GAPS / NEXT CONTRACT", "what must be added before intake is complete", "missing intake contracts")
	for _, gap := range []string{
		"Obsidian navigation is represented by durable request/task metadata only; body/backlink/search connector is not yet wired",
		"desktop notifications are consumed via durable P0 incident snapshots; live mako/dbus bodies stay outside this read model",
		"fresh GitHub security truth requires a connector refresh; current security rows are snapshot evidence",
		"row-specific source drilldown, governed drain/dismiss receipts, and Obsidian metadata search remain future work",
	} {
		b.WriteString(" " + grammar.C("mut", "· "+clipRunes(gap, maxVisible(12, w-3))) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderSelectedIntakeLane(w int) string {
	s, ok := m.FocusedSession()
	if !ok {
		return " " + grammar.C("brt", "SELECTED INTAKE NEIGHBORHOOD") + grammar.C("mut", " — no selected session source")
	}
	claim := strings.TrimSpace(s.ClaimedTask)
	taskState, taskTok := "no claimed task", "mut"
	if claim != "" {
		if _, found := m.taskByID(claim); found {
			taskState, taskTok = "task visible", "grn"
		} else {
			taskState, taskTok = "task gap", "red"
		}
	}
	related := m.sessionRelatedEvents(s)
	failures := 0
	for _, ev := range related {
		if m.AIR && ev.AIR["kind"] != "ok" {
			continue // a denied kind must not be classified into the on-air failure tally
		}
		if strings.Contains(strings.ToLower(ev.Kind), "fail") {
			failures++
		}
	}
	var b strings.Builder
	writeSectionHeader(&b, w, "SELECTED INTAKE NEIGHBORHOOD", "lane source brushes actor events, claimed task, and ambient demand", "lane intake context")
	writeWrappedKV(&b, "lane", sessionFieldValueForAir(s, "role", m.AIR), airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR), w)
	writeWrappedKV(&b, "claimed", sessionFieldValueForAir(s, "claimed_task", m.AIR), "pri", w)
	writeWrappedKV(&b, "task link", taskState, taskTok, w)
	writeWrappedKV(&b, "actor events", fmt.Sprintf("%d related · %d failures", len(related), failures), severityCountToken(failures), w)
	writeWrappedKV(&b, "ambient", fmt.Sprintf("%d attention units across intake snapshots", m.intakeAttentionTotal()), countWarnToken(m.intakeAttentionTotal()), w)
	writeWrappedKV(&b, "sources", fmt.Sprintf("%d snapshots · %d buckets", len(m.Intake.Sources), len(m.Intake.Rows)), countToken(len(m.Intake.Sources)), w)
	writeWrappedKV(&b, "authority", "read only; no drain/dismiss/write", "yel", w)
	writeWrappedKV(&b, "legal next", ":intent show-route · :intent open-trace", "yel", w)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderSelectedIntakeBucket(w int) string {
	row, ok := m.FocusedIntakeRow()
	if !ok {
		return " " + grammar.C("brt", "SELECTED BUCKET") + grammar.C("mut", " — no bucket in current source filter")
	}
	total := len(m.visibleIntakeRows())
	var b strings.Builder
	writeSectionHeader(&b, w, "SELECTED BUCKET", "focused intake bucket; source filter controls row set", "focused intake bucket")
	writeWrappedKV(&b, "cursor", fmt.Sprintf("%d/%d", m.IFocus+1, total), "2nd", w)
	legalNext := "[Enter] aggregate detail · [s/S] source filter · :intent show-route"
	if m.sessionSplit() {
		legalNext = "[n/p] bucket · [E] evidence path · [s/S] source filter · [Enter] source detail"
	}
	if w < 118 {
		proof := strings.Join(nonEmptyParts([]string{
			firstNonEmpty(intakeRowFieldForAir(row, "authority", m.AIR), "observation-only"),
			intakeRowFieldForAir(row, "evidence", m.AIR),
			labeledPart("refs", intakeRowFieldForAir(row, "source_refs", m.AIR)),
		}), " · ")
		gap := strings.Join(nonEmptyParts([]string{
			intakeRowFieldForAir(row, "missing", m.AIR),
			labeledPart("action", intakeRowFieldForAir(row, "action", m.AIR)),
		}), " · ")
		if strings.TrimSpace(gap) == "" {
			gap = "no declared gap"
		}
		writeWrappedKV(&b, "bucket", strings.Join(nonEmptyParts([]string{
			intakeRowFieldForAir(row, "kind", m.AIR),
			intakeRowFieldForAir(row, "status", m.AIR),
			labeledPart("sev", intakeRowFieldForAir(row, "severity", m.AIR)),
			labeledPart("n", intakeRowFieldForAir(row, "count", m.AIR)),
		}), " · "), airHue(intakeSeverityToken(row.Severity), row.AIR, "severity", m.AIR), w)
		writeWrappedKV(&b, "source", strings.Join(nonEmptyParts([]string{
			intakeRowFieldForAir(row, "source", m.AIR),
			labeledPart("id", intakeRowFieldForAir(row, "id", m.AIR)),
		}), " · "), "pri", w)
		writeWrappedKV(&b, "proof", proof, "yel", w)
		writeWrappedKV(&b, "coverage", strings.Join(nonEmptyParts([]string{
			intakeRowFieldForAir(row, "coverage", m.AIR),
			labeledPart("link", intakeRowFieldForAir(row, "task_link_state", m.AIR)),
			labeledPart("blocker", intakeRowFieldForAir(row, "blocker", m.AIR)),
		}), " · "), "2nd", w)
		writeWrappedKV(&b, "gap", gap, "org", w)
		writeWrappedKV(&b, "next proof", intakeRowFieldForAir(row, "next_evidence", m.AIR), "pri", w)
		writeWrappedKV(&b, "legal next", legalNext, "yel", w)
		return strings.TrimRight(b.String(), "\n")
	}
	writeWrappedKV(&b, "id", intakeRowFieldForAir(row, "id", m.AIR), "pri", w)
	writeWrappedKV(&b, "source", intakeRowFieldForAir(row, "source", m.AIR), "pri", w)
	writeWrappedKV(&b, "kind", intakeRowFieldForAir(row, "kind", m.AIR), airHue(intakeSeverityToken(row.Severity), row.AIR, "severity", m.AIR), w)
	writeWrappedKV(&b, "status", intakeRowFieldForAir(row, "status", m.AIR), intakeStatusToken(row.Status), w)
	writeWrappedKV(&b, "severity", intakeRowFieldForAir(row, "severity", m.AIR), airHue(intakeSeverityToken(row.Severity), row.AIR, "severity", m.AIR), w)
	writeWrappedKV(&b, "count", intakeRowFieldForAir(row, "count", m.AIR), countWarnToken(row.Count), w)
	writeWrappedKV(&b, "authority", intakeRowFieldForAir(row, "authority", m.AIR), "yel", w)
	writeWrappedKV(&b, "evidence", intakeRowFieldForAir(row, "evidence", m.AIR), "2nd", w)
	writeWrappedKV(&b, "coverage", intakeRowFieldForAir(row, "coverage", m.AIR), "2nd", w)
	writeWrappedKV(&b, "task link", intakeRowFieldForAir(row, "task_link_state", m.AIR), "2nd", w)
	writeWrappedKV(&b, "blocker", intakeRowFieldForAir(row, "blocker", m.AIR), "yel", w)
	writeWrappedKV(&b, "missing", intakeRowFieldForAir(row, "missing", m.AIR), "mut", w)
	writeWrappedKV(&b, "action", intakeRowFieldForAir(row, "action", m.AIR), "org", w)
	writeWrappedKV(&b, "detail", intakeRowFieldForAir(row, "detail", m.AIR), "mut", w)
	writeWrappedKV(&b, "source refs", intakeRowFieldForAir(row, "source_refs", m.AIR), "2nd", w)
	writeWrappedKV(&b, "next proof", intakeRowFieldForAir(row, "next_evidence", m.AIR), "pri", w)
	writeWrappedKV(&b, "legal next", legalNext, "yel", w)
	return strings.TrimRight(b.String(), "\n")
}

// renderLastlogDoor: the /lastlog scrollback — retained coord-event history (newest at
// bottom), the BitchX/irssi lastlog affordance. Reuses grammar.RenderEventRow so AIR is
// single-sourced (a locally-captured private event cannot replay cleartext on-air). The
// ring is fed on every poll (EventScrollback.Feed); PgUp/PgDn backward-paging through the
// /read/events `before` cursor arrives in the next slice.
func (m Model) renderLastlogDoor(w, h int) string {
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	var lines []string
	add := func(s string) { lines = append(lines, fitWidth(s, w)) }
	add(" " + grammar.C("brt", "◆ DOOR /lastlog") + grammar.C("2nd", "  event-history scrollback"))
	add(" " + grammar.C("mut", "retained coord events, newest at bottom — Esc closes."))
	add(" " + grammar.C("border", strings.Repeat("─", maxVisible(10, w))))
	add("  " + grammar.RenderEventHeader())
	// the view = backward-paged older (PgUp, if any) on top of the retained recent window
	view := append([]grammar.Event{}, m.LastlogOlder...)
	view = append(view, m.EventScrollback.Rows...)
	if len(view) == 0 {
		add(" " + grammar.C("mut", "no retained events yet — accumulates as the event stream flows"))
	}
	headroom := h - len(lines) - 1
	if headroom < 1 {
		headroom = 1
	}
	skip := len(view) - headroom
	if skip < 0 {
		skip = 0
	}
	for _, ev := range view[skip:] {
		add("  " + grammar.RenderEventRow(ev, m.AIR))
	}
	for len(lines) < h-1 {
		lines = append(lines, fitWidth("", w))
	}
	paging := ""
	if m.LastlogPaging {
		paging = grammar.C("yel", " · paging…")
	}
	add(grammar.C("mut", "  [PgUp]older [PgDn]live [Esc]close · ") + grammar.C("2nd", fmt.Sprintf("%d shown", len(view))) + paging)
	for len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderIntakeDoor(w, h int) string {
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	line := func(label, value, tok string) string {
		if strings.TrimSpace(value) == "" {
			value, tok = "·", "mut"
		}
		return fitWidth(" "+grammar.C("mut", fmt.Sprintf("%-16s", clipRunes(label, 16)))+grammar.C(tok, value), w)
	}
	var lines []string
	add := func(s string) { lines = append(lines, fitWidth(s, w)) }
	blank := func() { lines = append(lines, "") }
	rule := func() { add(grammar.C("border", strings.Repeat("─", maxVisible(10, w)))) }

	add(" " + grammar.C("brt", "◆ DOOR /intake") + grammar.C("2nd", "  aggregate provenance"))
	add(" " + grammar.C("mut", "present-at-hand intake decode; metadata only; no raw Obsidian, notification, URL, or body text."))
	blank()

	add(" " + grammar.C("mut", "SOURCE POSTURE — durable snapshots backing this projection"))
	if len(m.Intake.Sources) == 0 {
		add(" " + grammar.C("mut", "no intake source rows visible"))
	} else {
		for _, src := range m.Intake.Sources {
			name := intakeSourceFieldForAir(src, "id", m.AIR)
			status := intakeSourceFieldForAir(src, "status", m.AIR)
			count := intakeSourceFieldForAir(src, "count", m.AIR)
			age := intakeSourceFieldForAir(src, "age_bucket", m.AIR)
			path := intakeSourceFieldForAir(src, "path", m.AIR)
			add(" " + grammar.C("pri", clipRunes(name, maxVisible(8, w-2))))
			add(line("status", status+" · count "+count+" · age "+age, intakeStatusToken(src.Status)))
			add(line("path", path, "2nd"))
		}
	}
	rule()

	add(" " + grammar.C("mut", "DEMAND TOTALS — aggregate pressure, not action authority"))
	for _, row := range m.intakeTotalRows() {
		add(line(row.label, row.value, row.token))
	}
	rule()

	add(" " + grammar.C("mut", "SELECTED BUCKET — current source filter: ") + grammar.C("pri", m.intakeFilterLabel()))
	if row, ok := m.FocusedIntakeRow(); ok {
		add(line("source", intakeRowFieldForAir(row, "source", m.AIR), "pri"))
		add(line("kind", intakeRowFieldForAir(row, "kind", m.AIR), airHue(intakeSeverityToken(row.Severity), row.AIR, "severity", m.AIR)))
		add(line("status", intakeRowFieldForAir(row, "status", m.AIR), intakeStatusToken(row.Status)))
		add(line("severity", intakeRowFieldForAir(row, "severity", m.AIR), airHue(intakeSeverityToken(row.Severity), row.AIR, "severity", m.AIR)))
		add(line("count", intakeRowFieldForAir(row, "count", m.AIR), countWarnToken(row.Count)))
		add(line("coverage", intakeRowFieldForAir(row, "coverage", m.AIR), "2nd"))
		add(line("task link", intakeRowFieldForAir(row, "task_link_state", m.AIR), "2nd"))
		add(line("blocker", intakeRowFieldForAir(row, "blocker", m.AIR), "yel"))
	} else {
		add(" " + grammar.C("mut", "no selected bucket in current filter"))
	}
	rule()

	add(" " + grammar.C("mut", "TOP BUCKETS — filtered, sorted by severity then count; values pass AIR policy"))
	rows := m.visibleIntakeRows()
	if len(rows) == 0 {
		add(" " + grammar.C("mut", "no aggregate observation buckets"))
	} else {
		limit := 8
		for i, row := range rows {
			if i >= limit {
				add(" " + grammar.C("mut", fmt.Sprintf("… %d more buckets", len(rows)-i)))
				break
			}
			add(" " +
				grammar.C(airHue(intakeSeverityToken(row.Severity), row.AIR, "severity", m.AIR), fmt.Sprintf("%-6s", clipRunes(intakeRowFieldForAir(row, "severity", m.AIR), 6))) +
				grammar.C("pri", " "+clipRunes(intakeRowFieldForAir(row, "kind", m.AIR), maxVisible(8, w-38))) +
				grammar.C("2nd", " n="+intakeRowFieldForAir(row, "count", m.AIR)))
			add(line("coverage", intakeRowFieldForAir(row, "coverage", m.AIR), "2nd"))
			add(line("blocker", intakeRowFieldForAir(row, "blocker", m.AIR), "yel"))
		}
	}
	rule()

	add(" " + grammar.C("mut", "CONSTRAINTS"))
	add(" " + grammar.C("red", "× ") + grammar.C("mut", "drain, dismiss, claim, dispatch, write, and raw body reads are unavailable here"))
	add(" " + grammar.C("grn", "✓ ") + grammar.C("mut", "safe next: correlate via :events, :tasks, :sessions, or :intent show-route"))

	dock := []string{
		fitWidth(" "+grammar.C("mut", "VERB DOCK: ")+grammar.C("mut", " unavailable: drain · dismiss · write · raw-body"), w),
		fitWidth(" "+grammar.C("mut", "[Esc]/[Enter]/[q] back · detail is aggregate-only until row/source selectors are wired"), w),
	}
	if h <= len(dock) {
		return strings.Join(dock[:h], "\n")
	}
	maxBody := h - len(dock)
	if len(lines) > maxBody {
		hidden := len(lines) - maxBody + 1
		lines = lines[:maxBody]
		lines[maxBody-1] = fitWidth(" "+grammar.C("mut", fmt.Sprintf("… %d door rows hidden; taller frame", hidden)), w)
	}
	for len(lines) < maxBody {
		blank()
	}
	lines = append(lines, dock...)
	return strings.Join(lines, "\n")
}

func (m Model) intakeSessionContext(s grammar.Session) string {
	related := len(m.sessionRelatedEvents(s))
	total := m.intakeAttentionTotal()
	if related > 0 {
		return fmt.Sprintf("%d actor events · demand %d", related, total)
	}
	return fmt.Sprintf("ambient demand %d", total)
}

func (m Model) intakeAttentionTotal() int {
	if m.Intake.Totals != nil {
		return m.Intake.Totals["planning_attention"] + m.Intake.Totals["request_attention"] +
			m.Intake.Totals["p0_incidents"] + m.Intake.Totals["security_signals"]
	}
	total := 0
	for _, row := range m.Intake.Rows {
		if row.Severity == "crit" || row.Severity == "major" || row.Severity == "warn" {
			total += row.Count
		}
	}
	return total
}

func (m Model) intakeTotalRows() []contextRow {
	t := m.Intake.Totals
	if t == nil {
		t = map[string]int{}
	}
	return []contextRow{
		{"request attention", fmt.Sprintf("%d", t["request_attention"]), countWarnToken(t["request_attention"])},
		{"planning attention", fmt.Sprintf("%d", t["planning_attention"]), countWarnToken(t["planning_attention"])},
		{"p0 incidents", fmt.Sprintf("%d", t["p0_incidents"]), countWarnToken(t["p0_incidents"])},
		{"security signals", fmt.Sprintf("%d", t["security_signals"]), countWarnToken(t["security_signals"])},
		{"sources", fmt.Sprintf("%d observed · %d buckets", len(m.Intake.Sources), len(m.Intake.Rows)), countToken(len(m.Intake.Sources))},
	}
}

func (m Model) intakeRowsSorted() []grammar.IntakeRow {
	rows := make([]grammar.IntakeRow, len(m.Intake.Rows))
	copy(rows, m.Intake.Rows)
	sort.SliceStable(rows, func(i, j int) bool {
		ri, rj := intakeSeverityRank(rows[i].Severity), intakeSeverityRank(rows[j].Severity)
		if ri != rj {
			return ri > rj
		}
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Kind < rows[j].Kind
	})
	return rows
}

func (m Model) visibleIntakeRows() []grammar.IntakeRow {
	rows := m.intakeRowsSorted()
	filter := strings.TrimSpace(m.IntakeSourceFilter)
	if filter == "" {
		return rows
	}
	out := make([]grammar.IntakeRow, 0, len(rows))
	for _, row := range rows {
		if row.Source == filter {
			out = append(out, row)
		}
	}
	return out
}

func (m Model) intakeSourceIDs() []string {
	seen := map[string]bool{}
	var ids []string
	for _, src := range m.Intake.Sources {
		id := strings.TrimSpace(src.ID)
		if id != "" && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	var rest []string
	for _, row := range m.Intake.Rows {
		id := strings.TrimSpace(row.Source)
		if id != "" && !seen[id] {
			rest = append(rest, id)
			seen[id] = true
		}
	}
	sort.Strings(rest)
	return append(ids, rest...)
}

func (m Model) intakeFilterLabel() string {
	if strings.TrimSpace(m.IntakeSourceFilter) == "" {
		return "all sources"
	}
	return m.IntakeSourceFilter
}

func intakeSourceFieldForAir(src grammar.IntakeSource, field string, air bool) string {
	var value string
	switch field {
	case "id":
		value = src.ID
	case "path":
		value = src.Path
	case "exists":
		value = fmt.Sprintf("%t", src.Exists)
	case "mtime":
		value = src.MTime
	case "age_bucket":
		value = src.AgeBucket
	case "status":
		value = src.Status
	case "count":
		value = fmt.Sprintf("%d", src.Count)
	case "privacy":
		value = src.Privacy
	case "raw_access":
		value = fmt.Sprintf("%t", src.RawAccess)
	}
	return grammar.Redact(src.AIR, field, value, air)
}

func intakeRowFieldForAir(row grammar.IntakeRow, field string, air bool) string {
	var value string
	switch field {
	case "id":
		value = row.ID
	case "source":
		value = row.Source
	case "kind":
		value = row.Kind
	case "status":
		value = row.Status
	case "severity":
		value = row.Severity
	case "count":
		value = fmt.Sprintf("%d", row.Count)
	case "blocker":
		value = row.Blocker
	case "coverage":
		value = row.Coverage
	case "task_link_state":
		value = row.TaskLinkState
	case "evidence_count":
		value = fmt.Sprintf("%d", row.EvidenceCount)
	case "age_bucket":
		value = row.AgeBucket
	case "authority":
		value = row.Authority
	case "evidence":
		value = row.Evidence
	case "missing":
		value = row.Missing
	case "action":
		value = row.Action
	case "detail":
		value = row.Detail
	case "source_refs":
		value = row.SourceRefs
	case "next_evidence":
		value = row.NextEvidence
	}
	return grammar.Redact(row.AIR, field, value, air)
}

func intakeSeverityRank(sev string) int {
	switch sev {
	case "crit":
		return 4
	case "major":
		return 3
	case "warn":
		return 2
	case "ok":
		return 1
	}
	return 0
}

func intakeSeverityToken(sev string) string {
	switch sev {
	case "crit":
		return "red"
	case "major":
		return "org"
	case "warn":
		return "yel"
	case "ok":
		return "grn"
	}
	return "mut"
}

func intakeStatusToken(status string) string {
	switch status {
	case "observed", "bucket", "snapshot", "recent":
		return "grn"
	case "attention", "stale":
		return "yel"
	case "missing", "needs repair":
		return "red"
	}
	return "mut"
}

func intakeAgeToken(age string) string {
	switch age {
	case "<5m", "<1h":
		return "grn"
	case "<6h":
		return "yel"
	case "<1d", ">1d", "missing":
		return "org"
	}
	return "mut"
}

func taskGateReason(t grammar.Task) (string, string) {
	if strings.EqualFold(t.PredictedStage, "hold") {
		return "release hold", "red"
	}
	switch t.Criticality {
	case "crit":
		return "critical risk", "red"
	case "major":
		return "major risk", "org"
	case "warn":
		return "warning", "yel"
	}
	return "clear", "grn"
}

func sessionBlockerRows(sessions []grammar.Session, w int, air bool) []string {
	counts := map[string]int{}
	for _, s := range sessions {
		blocker := strings.TrimSpace(s.Blocker)
		if blocker == "" {
			blocker = "none"
		}
		if air && s.AIR["blocker"] != "ok" {
			blocker = "hidden"
		}
		counts[blocker]++
	}
	order := []string{"hidden", "no_session", "stale_relay", "no_claim", "stalled", "offline", "none"}
	var rows []string
	seen := map[string]bool{}
	for _, blocker := range order {
		if n := counts[blocker]; n > 0 {
			if w < 132 {
				rows = append(rows, wrappedKVLines(blocker, fmt.Sprintf("blocker=%s · lanes=%d", blocker, n), blockerCountToken(blocker), w)...)
			} else {
				rows = append(rows, " "+grammar.C("2nd", fmt.Sprintf("%-12s", blocker))+grammar.C(blockerCountToken(blocker), fmt.Sprintf("%d lanes", n)))
			}
			seen[blocker] = true
		}
	}
	var extra []string
	for blocker, n := range counts {
		if !seen[blocker] {
			extra = append(extra, fmt.Sprintf("%s:%d", blocker, n))
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		if w < 132 {
			rows = append(rows, wrappedKVLines("other", "blockers="+strings.Join(extra, " · "), "yel", w)...)
		} else {
			rows = append(rows, " "+grammar.C("2nd", "other       ")+grammar.C("yel", clipRunes(strings.Join(extra, " · "), maxVisible(8, w-14))))
		}
	}
	if len(rows) == 0 {
		rows = append(rows, " "+grammar.C("mut", "no session readiness rows"))
	}
	return rows
}

func blockerCountToken(blocker string) string {
	if blocker == "none" {
		return "grn"
	}
	return "red"
}

func yardSourceChip(label string, n int, dark bool) string {
	if dark {
		return grammar.C("red", label+":DARK")
	}
	if n == 0 {
		return grammar.C("mut", label+":0")
	}
	return grammar.C("pri", fmt.Sprintf("%s:%d", label, n))
}

func (m Model) readSourceChip(label string, n int, dark bool, seq int) string {
	base := label + ":"
	tok := "pri"
	if dark {
		base += "DARK"
		tok = "red"
	} else {
		base += fmt.Sprintf("%d", n)
		if n == 0 {
			tok = "mut"
		}
	}
	return grammar.C(tok, base) + grammar.C("2nd", fmt.Sprintf(" r%d", seq%10)) + m.readSourcePulse(label, seq)
}

func (m Model) readSourcePulse(label string, seq int) string {
	if seq <= 0 || label != m.LastFold {
		return grammar.C("mut", ".")
	}
	pulse, tok := m.readFoldPulse(label)
	return grammar.C(tok, pulse)
}

func (m Model) livenessGlyph() string {
	frames := []string{"·", "∙", "•", "∙"}
	return frames[m.Beat%len(frames)]
}

func (m Model) focusGlyph() string {
	frames := []string{"▶", "▸"}
	return frames[m.Beat%len(frames)]
}

func (m Model) viewSpine(pageDark bool) string {
	total, dark, unseen := m.readSourceHealth()
	if dark > 0 || pageDark {
		if dark == 0 {
			dark = 1
		}
		return grammar.C("red", fmt.Sprintf("spine:DARK %d/%d", dark, total))
	}
	seen := total - unseen
	if unseen > 0 {
		return grammar.C("mut", fmt.Sprintf("spine:BOOT %d/%d%s", seen, total, m.livenessGlyph()))
	}
	last := strings.TrimSpace(m.LastFold)
	if last == "" {
		last = "none"
	}
	pulse, tok := m.readFoldPulse(last)
	return grammar.C("grn", "spine:read") + grammar.C(tok, pulse)
}

func (m Model) readSourceHealth() (total int, dark int, unseen int) {
	sources := []struct {
		dark bool
		seq  int
	}{
		{m.EventsDark, m.EventsSeq},
		{m.TasksDark, m.TasksSeq},
		{m.SessionsDark, m.SessionsSeq},
		{m.IntakeDark, m.IntakeSeq},
		{m.CapabilitiesDark, m.CapabilitiesSeq},
		{m.GatesDark, m.GatesSeq},
		{m.DomainsDark, m.DomainsSeq},
		{m.DynamicsDark, m.DynamicsSeq},
		{m.EpistemicsDark, m.EpistemicsSeq},
	}
	for _, source := range sources {
		total++
		if source.dark {
			dark++
		}
		if source.seq <= 0 {
			unseen++
		}
	}
	return total, dark, unseen
}

func (m Model) readReceipt() string {
	last := m.LastFold
	if strings.TrimSpace(last) == "" {
		last = "none"
	}
	pulse, pulseToken := m.readFoldPulse(last)
	return grammar.C("2nd", "rx ") + grammar.C(pulseToken, pulse) + grammar.C("2nd", fmt.Sprintf(" e%d t%d s%d i%d c%d g%d o%d d%d p%d %s",
		m.EventsSeq%10, m.TasksSeq%10, m.SessionsSeq%10, m.IntakeSeq%10, m.CapabilitiesSeq%10, m.GatesSeq%10, m.DomainsSeq%10, m.DynamicsSeq%10, m.EpistemicsSeq%10, last))
}

func (m Model) readFoldPulse(last string) (string, string) {
	seq, dark, ok := m.readFoldState(last)
	if !ok || seq <= 0 {
		return ".", "mut"
	}
	frames := []string{"|", "/", "-", "\\"}
	token := "yel"
	if dark {
		token = "red"
	}
	return frames[seq%len(frames)], token
}

func (m Model) readFoldState(last string) (int, bool, bool) {
	switch last {
	case "events":
		return m.EventsSeq, m.EventsDark, true
	case "tasks":
		return m.TasksSeq, m.TasksDark, true
	case "sessions":
		return m.SessionsSeq, m.SessionsDark, true
	case "intake":
		return m.IntakeSeq, m.IntakeDark, true
	case "capabilities":
		return m.CapabilitiesSeq, m.CapabilitiesDark, true
	case "gates":
		return m.GatesSeq, m.GatesDark, true
	case "domains":
		return m.DomainsSeq, m.DomainsDark, true
	case "dynamics":
		return m.DynamicsSeq, m.DynamicsDark, true
	case "epistemics":
		return m.EpistemicsSeq, m.EpistemicsDark, true
	}
	return 0, false, false
}

func (m Model) yardStageCounts() ([12]int, int, int) {
	var counts [12]int
	hidden, unstaged := 0, 0
	for _, t := range m.Tasks {
		if m.AIR && t.AIR["stage"] != "ok" {
			hidden++
			continue
		}
		stage := doorStageIndex(t.Stage)
		if stage >= 0 && stage < len(counts) {
			counts[stage]++
		} else {
			unstaged++
		}
	}
	return counts, hidden, unstaged
}

func (m Model) yardBlockedIndices() ([]int, int) {
	var visible []int
	hidden := 0
	for _, idx := range m.blockedIndices() {
		t := m.Tasks[idx]
		if m.AIR && (t.AIR["stage"] != "ok" || t.AIR["predicted_stage"] != "ok" || t.AIR["criticality"] != "ok") {
			hidden++
			continue
		}
		visible = append(visible, idx)
	}
	return visible, hidden
}

func (m Model) yardHotSessionIndices() ([]int, int) {
	var visible []int
	hidden := 0
	for i, s := range m.Sessions {
		hot := s.Attention >= 0.50 || (s.Blocker != "" && s.Blocker != "none") ||
			s.Readiness == "claim" || s.Readiness == "stall" || s.Readiness == "stale"
		if !hot {
			continue
		}
		if m.AIR && (s.AIR["attention"] != "ok" || s.AIR["blocker"] != "ok" || s.AIR["readiness"] != "ok") {
			hidden++
			continue
		}
		visible = append(visible, i)
	}
	return visible, hidden
}

func (m Model) yardFailureEventIndices() ([]int, int) {
	var visible []int
	hidden := 0
	for i := len(m.Events) - 1; i >= 0; i-- {
		ev := m.Events[i]
		if !strings.Contains(strings.ToLower(ev.Kind), "fail") {
			continue
		}
		if m.AIR && ev.AIR["kind"] != "ok" {
			hidden++
			continue
		}
		visible = append(visible, i)
		if len(visible) >= 3 {
			break
		}
	}
	return visible, hidden
}

type yardFleet struct {
	claim, stale, off, live, stalled, codex, claude, hidden int
}

func (m Model) yardFleetCounts() yardFleet {
	var f yardFleet
	for _, s := range m.Sessions {
		if m.AIR && (s.AIR["readiness"] != "ok" || s.AIR["state"] != "ok" || s.AIR["platform"] != "ok") {
			f.hidden++
			continue
		}
		switch s.Readiness {
		case "claim":
			f.claim++
		case "stale":
			f.stale++
		}
		switch s.State {
		case "active":
			f.live++
		}
		if s.Readiness == "off" || s.Readiness == "offline" || s.State == "offline" {
			f.off++
		}
		// readiness is already hide-gated above; gate the raw Stalled term so a denied stalled is not
		// classified into the per-class count (the count discloses the denied field)
		if (s.Stalled && (!m.AIR || s.AIR["stalled"] == "ok")) || s.Readiness == "stall" {
			f.stalled++
		}
		switch s.Platform {
		case "codex":
			f.codex++
		case "claude":
			f.claude++
		}
	}
	return f
}

func yardCount(label string, n int, tok string) string {
	return grammar.C("2nd", label+":") + grammar.C(tok, fmt.Sprintf("%d", n))
}

func writeSectionHeader(b *strings.Builder, w int, title, wideDetail, narrowDetail string) {
	detail := wideDetail
	if w < 96 && narrowDetail != "" {
		detail = narrowDetail
	}
	prefix := " " + grammar.C("brt", title)
	if strings.TrimSpace(detail) == "" {
		b.WriteString(prefix + "\n")
		return
	}
	avail := maxVisible(8, w-ansi.StringWidth(title)-4)
	b.WriteString(prefix + grammar.C("mut", " — "+clipRunes(detail, avail)) + "\n")
}

func (m Model) renderCapabilityProjection(w int) string {
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	platforms := m.capabilityPlatformRows()
	routes := lookupIntentArgs()
	caps := m.capabilityStatusRows()
	grouped := groupCapabilityStatusRows(caps)
	rollups := capabilityClassRollups(caps)
	targetFocus := m.targetRowFocusActive()
	selectedCapability := m.CFocus
	if !targetFocus {
		selectedCapability = -1
	}
	writeSectionHeader(&b, w, "CAPABILITIES", "routing fit/admission projection over live tool lanes", "routing fit over live lanes")
	b.WriteString(" " + grammar.C("yel", "read-only projection") + grammar.C("mut", " · no dispatch/claim/close/PTY · [C]/:capabilities") + "\n")
	b.WriteString(" " + grammar.C("2nd", "sources") + " " +
		m.readSourceChip("capabilities", len(m.Capabilities.Rows), m.CapabilitiesDark, m.CapabilitiesSeq) + grammar.C("mut", " · ") +
		m.readSourceChip("sessions", len(m.Sessions), m.SessionsDark, m.SessionsSeq) + grammar.C("mut", " · ") +
		yardSourceChip("commands", len(verbs), false) + grammar.C("mut", " · ") +
		yardSourceChip("intents", len(routes), false) + grammar.C("mut", " · ") +
		yardSourceChip("surfaces", len(surfaceRegistry), false) + "\n")
	if len(m.Capabilities.Rows) > 0 && len(rollups) > 0 {
		b.WriteString(" " + grammar.C("2nd", "class posture") + " " + renderCapabilityClassPosture(rollups, w) + "\n")
	}
	b.WriteString(rule + "\n")

	capDisplayIdx := 0
	if rows := grouped["class"]; len(rows) > 0 {
		renderCapabilityClassCockpit(&b, w, rows, grouped["surface"], selectedCapability, &capDisplayIdx)
		b.WriteString(rule + "\n")
	}

	if m.sessionSplit() && !m.SuppressSplitPinned {
		b.WriteString(m.renderSelectedCapabilityFit(w) + "\n")
		b.WriteString(rule + "\n")
	}

	if len(m.Capabilities.Sources) > 0 {
		writeSectionHeader(&b, w, "CAPABILITY SOURCES", "registry, receipts, quota, and authority evidence", "source-backed")
		if w < 132 {
			for _, src := range m.Capabilities.Sources {
				writeWrappedKV(&b, capabilitySourceFieldForAir(src, "id", m.AIR),
					fmt.Sprintf("status=%s · count=%s · age=%s · detail=%s",
						capabilitySourceFieldForAir(src, "status", m.AIR),
						capabilitySourceFieldForAir(src, "count", m.AIR),
						capabilitySourceFieldForAir(src, "age_bucket", m.AIR),
						capabilitySourceFieldForAir(src, "detail", m.AIR)), sourceStatusToken(src.Status), w)
			}
		} else {
			b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-26s %-12s %6s %-8s %s", "SOURCE", "STATUS", "COUNT", "AGE", "DETAIL")) + "\n")
			for _, src := range m.Capabilities.Sources {
				b.WriteString(" " +
					grammar.C("pri", fmt.Sprintf("%-26s", clipRunes(capabilitySourceFieldForAir(src, "id", m.AIR), 26))) +
					grammar.C(sourceStatusToken(src.Status), fmt.Sprintf(" %-12s", clipRunes(capabilitySourceFieldForAir(src, "status", m.AIR), 12))) +
					grammar.C(countToken(src.Count), fmt.Sprintf(" %6s", capabilitySourceFieldForAir(src, "count", m.AIR))) +
					grammar.C("2nd", fmt.Sprintf(" %-8s", clipRunes(capabilitySourceFieldForAir(src, "age_bucket", m.AIR), 8))) +
					grammar.C("mut", " "+clipRunes(capabilitySourceFieldForAir(src, "detail", m.AIR), maxVisible(10, w-60))) + "\n")
			}
		}
		b.WriteString(rule + "\n")
	}

	writeSectionHeader(&b, w, "FABRIC SCOPE", "capability choice is derived, not manually picked", "derived routing posture")
	for _, scope := range []string{
		"admission/pacing, WIP/fanout, capability selection, verifier/reviewer/watcher selection",
		"hardening allocation, eval-plane choice, learning eligibility, quota/context economics",
		"support-only/HKP context must stay below authority; route selections alone do not train calibration",
		"role names such as fugu/fugu-ultra are evidence labels only; descriptors and receipts decide fit",
	} {
		writeWrappedBullet(&b, scope, "mut", w)
	}
	b.WriteString(rule + "\n")

	writeSectionHeader(&b, w, "CAPABILITY STATUS", "every capability is explicit; platforms are evidence, not ontology", "capability gaps/status")
	for _, section := range []struct {
		key, title, detail, label string
	}{
		{"surface", "SURFACE STATUS", "concrete tools/providers/connectors; evidence, not authority", "surface inventory"},
		{"core", "ROUTE CONTRACTS", "route envelope, quota, receipts, and HKP authority ceiling", "core contracts"},
		{"score", "SCORE DIMENSIONS", "registry capability evidence dimensions", "registry dimensions"},
	} {
		rows := grouped[section.key]
		if len(rows) == 0 {
			continue
		}
		renderCapabilityStatusRows(&b, w, section.title, section.detail, section.label, rows, selectedCapability, &capDisplayIdx)
		b.WriteString(rule + "\n")
	}

	writeSectionHeader(&b, w, "HKP SUPPORT CONTEXT", "represented, but authority-capped", "authority-capped support")
	if hkp, ok := m.capabilityRowByID("hkp_support_context"); ok {
		b.WriteString(" " + grammar.C(capabilityStatusToken(hkp.Status), hkp.Status) + grammar.C("mut", " · ") +
			grammar.C("2nd", capabilityRowFieldForAir(hkp, "authority", m.AIR)) + grammar.C("mut", " · ") +
			grammar.C("pri", capabilityRowFieldForAir(hkp, "hkp_posture", m.AIR)) + "\n")
		writeWrappedKV(&b, "requires", capabilityRowFieldForAir(hkp, "blocker", m.AIR), "mut", w)
	} else {
		b.WriteString(" " + grammar.C("yel", "support-only") + grammar.C("mut", " · advisory cache/context; not source truth, not dispatch authority, not calibration evidence") + "\n")
		writeWrappedKV(&b, "requires", "source verification, promotion route, redaction policy, consumer policy, and receipt", "mut", w)
	}
	b.WriteString(rule + "\n")

	writeSectionHeader(&b, w, "PLATFORM EVIDENCE", "observed lane posture; not the capability list", "lane evidence")
	if len(platforms) == 0 {
		b.WriteString(" no visible platform rows · waiting for /read/sessions\n")
	} else if w < 132 {
		for _, row := range platforms {
			tok := capabilityPlatformToken(row)
			writeWrappedKV(&b, row.name, fmt.Sprintf("platform=%s · lanes=%d · claim=%d · live=%d · stale=%d · off=%d · hot=%d · contract=%s",
				row.name, row.total, row.claim, row.live, row.stale, row.off, row.hot, capabilityPlatformContract(row.name)), tok, w)
			if row.hidden > 0 {
				writeWrappedKV(&b, "hidden", fmt.Sprintf("%d capability attributes hidden by AIR policy", row.hidden), "mut", w)
			}
		}
	} else {
		b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-13s %5s %5s %5s %5s %5s %5s %s", "PLATFORM", "LANES", "CLAIM", "LIVE", "STALE", "OFF", "HOT", "CONTRACT")) + "\n")
		for _, row := range platforms {
			tok := capabilityPlatformToken(row)
			b.WriteString(" " +
				grammar.C(tok, fmt.Sprintf("%-13s", clipRunes(row.name, 13))) +
				grammar.C("pri", fmt.Sprintf(" %5d", row.total)) +
				grammar.C(countToken(row.claim), fmt.Sprintf(" %5d", row.claim)) +
				grammar.C("grn", fmt.Sprintf(" %5d", row.live)) +
				grammar.C(countWarnToken(row.stale), fmt.Sprintf(" %5d", row.stale)) +
				grammar.C(countWarnToken(row.off), fmt.Sprintf(" %5d", row.off)) +
				grammar.C(attentionCountToken(row.hot), fmt.Sprintf(" %5d", row.hot)) +
				grammar.C("mut", " "+clipRunes(capabilityPlatformContract(row.name), maxVisible(12, w-62))) + "\n")
			if row.hidden > 0 {
				b.WriteString(" " + grammar.C("mut", fmt.Sprintf("  ▒ %d capability attributes hidden by AIR policy", row.hidden)) + "\n")
			}
		}
	}
	b.WriteString(rule + "\n")

	if len(m.Capabilities.Routes) > 0 {
		writeSectionHeader(&b, w, "ROUTE EVIDENCE", "routes are evidence below capabilities; no launch authority", "registry route rows")
		limit := len(m.Capabilities.Routes)
		if limit > 8 {
			limit = 8
		}
		if w < 132 {
			for i := 0; i < limit; i++ {
				r := m.Capabilities.Routes[i]
				writeWrappedKV(&b, capabilityRouteFieldForAir(r, "route_id", m.AIR),
					fmt.Sprintf("route=%s · platform=%s · state=%s · authority=%s · freshness=%s · quota=%s · receipts=%s",
						capabilityRouteFieldForAir(r, "route_id", m.AIR),
						capabilityRouteFieldForAir(r, "platform", m.AIR),
						capabilityRouteFieldForAir(r, "route_state", m.AIR),
						capabilityRouteFieldForAir(r, "authority_ceiling", m.AIR),
						capabilityRouteFieldForAir(r, "freshness_ok", m.AIR),
						capabilityRouteFieldForAir(r, "quota_state", m.AIR),
						capabilityRouteFieldForAir(r, "receipt_count", m.AIR)), capabilityRouteToken(r), w)
				writeWrappedKV(&b, "descriptor", capabilityRouteDescriptorSummary(r, m.AIR), routeAxisToken(r), w)
				writeWrappedKV(&b, "governance", capabilityRouteGovernanceSummary(r, m.AIR), routeAxisToken(r), w)
			}
		} else {
			b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-28s %-10s %-11s %-9s %-8s %s", "ROUTE", "PLATFORM", "STATE", "FRESH", "QUOTA", "AUTHORITY")) + "\n")
			for i := 0; i < limit; i++ {
				r := m.Capabilities.Routes[i]
				b.WriteString(" " +
					grammar.C("pri", fmt.Sprintf("%-28s", clipRunes(capabilityRouteFieldForAir(r, "route_id", m.AIR), 28))) +
					grammar.C("2nd", fmt.Sprintf(" %-10s", clipRunes(capabilityRouteFieldForAir(r, "platform", m.AIR), 10))) +
					grammar.C(capabilityRouteToken(r), fmt.Sprintf(" %-11s", clipRunes(capabilityRouteFieldForAir(r, "route_state", m.AIR), 11))) +
					grammar.C(boolToken(r.FreshnessOK), fmt.Sprintf(" %-9s", clipRunes(capabilityRouteFieldForAir(r, "freshness_ok", m.AIR), 9))) +
					grammar.C("mut", fmt.Sprintf(" %-8s", clipRunes(capabilityRouteFieldForAir(r, "quota_state", m.AIR), 8))) +
					grammar.C("2nd", " "+clipRunes(capabilityRouteFieldForAir(r, "authority_ceiling", m.AIR), maxVisible(10, w-74))) + "\n")
				b.WriteString(" " + grammar.C("2nd", "  desc ") + grammar.C(routeAxisToken(r), clipRunes(capabilityRouteDescriptorSummary(r, m.AIR), maxVisible(10, w-8))) + "\n")
				b.WriteString(" " + grammar.C("2nd", "  gov  ") + grammar.C(routeAxisToken(r), clipRunes(capabilityRouteGovernanceSummary(r, m.AIR), maxVisible(10, w-8))) + "\n")
			}
		}
		if len(m.Capabilities.Routes) > limit {
			b.WriteString(" " + grammar.C("mut", fmt.Sprintf("… %d more route evidence rows", len(m.Capabilities.Routes)-limit)) + "\n")
		}
		b.WriteString(rule + "\n")
	}

	if len(m.Capabilities.Tools) > 0 {
		writeSectionHeader(&b, w, "ROUTE TOOL EVIDENCE", "candidate tools by route; no per-session binding yet", "tool status")
		limit := len(m.Capabilities.Tools)
		if limit > 12 {
			limit = 12
		}
		if w < 132 {
			for i := 0; i < limit; i++ {
				tool := m.Capabilities.Tools[i]
				writeWrappedKV(&b, capabilityToolFieldForAir(tool, "tool_id", m.AIR),
					fmt.Sprintf("tool=%s · route=%s · platform=%s · status=%s · available=%s · use=%s · observed=%s",
						capabilityToolFieldForAir(tool, "tool_id", m.AIR),
						capabilityToolFieldForAir(tool, "route_id", m.AIR),
						capabilityToolFieldForAir(tool, "platform", m.AIR),
						capabilityToolFieldForAir(tool, "status", m.AIR),
						capabilityToolFieldForAir(tool, "available", m.AIR),
						capabilityToolFieldForAir(tool, "authority_use", m.AIR),
						capabilityToolFieldForAir(tool, "observed_at", m.AIR)), capabilityToolToken(tool), w)
			}
		} else {
			b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-18s %-28s %-10s %-12s %-10s %s", "TOOL", "ROUTE", "PLATFORM", "STATUS", "AVAILABLE", "USE")) + "\n")
			for i := 0; i < limit; i++ {
				tool := m.Capabilities.Tools[i]
				b.WriteString(" " +
					grammar.C("pri", fmt.Sprintf("%-18s", clipRunes(capabilityToolFieldForAir(tool, "tool_id", m.AIR), 18))) +
					grammar.C("2nd", fmt.Sprintf(" %-28s", clipRunes(capabilityToolFieldForAir(tool, "route_id", m.AIR), 28))) +
					grammar.C("2nd", fmt.Sprintf(" %-10s", clipRunes(capabilityToolFieldForAir(tool, "platform", m.AIR), 10))) +
					grammar.C(capabilityToolToken(tool), fmt.Sprintf(" %-12s", clipRunes(capabilityToolFieldForAir(tool, "status", m.AIR), 12))) +
					grammar.C(boolToken(tool.Available), fmt.Sprintf(" %-10s", clipRunes(capabilityToolFieldForAir(tool, "available", m.AIR), 10))) +
					grammar.C("mut", " "+clipRunes(capabilityToolFieldForAir(tool, "authority_use", m.AIR), maxVisible(8, w-86))) + "\n")
			}
		}
		if len(m.Capabilities.Tools) > limit {
			b.WriteString(" " + grammar.C("mut", fmt.Sprintf("… %d more route tool rows", len(m.Capabilities.Tools)-limit)) + "\n")
		}
		b.WriteString(rule + "\n")
	}

	writeSectionHeader(&b, w, "ROUTE ADMISSION", "intent targets are previewable, not launchable", "preview only")
	if w < 132 {
		for _, r := range routes {
			state, tok := m.capabilityIntentState(r.Label)
			writeWrappedKV(&b, r.Label, fmt.Sprintf("target=%s · subject=%s · state=%s · preflight=%s",
				r.Label, capabilityIntentSubject(r.Label), state, r.Detail), tok, w)
		}
	} else {
		b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-12s %-14s %-16s %s", "TARGET", "SUBJECT", "STATE", "PREFLIGHT")) + "\n")
		for _, r := range routes {
			state, tok := m.capabilityIntentState(r.Label)
			b.WriteString(" " +
				grammar.C("yel", fmt.Sprintf("%-12s", clipRunes(r.Label, 12))) +
				grammar.C("2nd", fmt.Sprintf(" %-14s", clipRunes(capabilityIntentSubject(r.Label), 14))) +
				grammar.C(tok, fmt.Sprintf(" %-16s", clipRunes(state, 16))) +
				grammar.C("mut", " "+clipRunes(r.Detail, maxVisible(12, w-46))) + "\n")
		}
	}
	b.WriteString(rule + "\n")

	writeSectionHeader(&b, w, "SESSION FIT RAIL", "top lanes by attention; role/profile remains evidence, not assumption", "attention rail")
	if len(m.Sessions) == 0 {
		b.WriteString(" no visible sessions · route fit unknown\n")
	} else {
		limit := len(m.Sessions)
		if limit > 6 {
			limit = 6
		}
		for i := 0; i < limit; i++ {
			s := m.Sessions[i]
			role := sessionFieldValueForAir(s, "role", m.AIR)
			plat := sessionFieldValueForAir(s, "platform", m.AIR)
			ready := sessionFieldValueForAir(s, "readiness", m.AIR)
			blocker := sessionFieldValueForAir(s, "blocker", m.AIR)
			fit, tok := capabilitySessionFit(s, m.AIR)
			if w < 132 {
				writeWrappedKV(&b, role, fmt.Sprintf("lane=%s · platform=%s · readiness=%s · attn=%s · fit=%s · blocker=%s",
					role, plat, ready, sessionFieldValueForAir(s, "attention", m.AIR), fit, blocker), tok, w)
			} else {
				b.WriteString(" " +
					grammar.C(airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR), fmt.Sprintf("%-13s", clipRunes(role, 13))) +
					grammar.C("2nd", fmt.Sprintf(" %-8s", clipRunes(plat, 8))) +
					grammar.C(airHue(readinessPaneToken(s.Readiness), s.AIR, "readiness", m.AIR), fmt.Sprintf(" %-7s", clipRunes(ready, 7))) +
					grammar.C(airHue(attentionToken(s.Attention), s.AIR, "attention", m.AIR), fmt.Sprintf(" attn:%s", sessionFieldValueForAir(s, "attention", m.AIR))) +
					grammar.C(tok, fmt.Sprintf(" %-16s", clipRunes(fit, 16))) +
					grammar.C("mut", " · "+clipRunes(blocker, maxVisible(8, w-64))) + "\n")
			}
		}
	}
	b.WriteString(rule + "\n")

	writeSectionHeader(&b, w, "GAPS / GUARDRAILS", "what Reins must still learn before routing can be trusted", "unknowns before routing")
	for _, gap := range m.capabilityGuardrailBullets() {
		writeWrappedBullet(&b, gap, "mut", w)
	}
	b.WriteString(rule + "\n")

	writeSectionHeader(&b, w, "REPRESENTATION", "matrix = fit, rail = attention, ladder = route admission, gaps = unknowns", "fit/rail/ladder/gaps")
	b.WriteString(" " + grammar.C("mut", "next parity surfaces: capability descriptors, route veto chain, quota/context load, evaluator fit, governed launch receipts"))
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderSelectedCapabilityFit(w int) string {
	s, ok := m.FocusedSession()
	if !ok {
		return " " + grammar.C("brt", "SELECTED LANE FIT") + grammar.C("mut", " — no selected session source")
	}
	fit, fitTok := capabilitySessionFit(s, m.AIR)
	claim := strings.TrimSpace(s.ClaimedTask)
	taskState, taskTok := "no claimed task", "mut"
	if claim != "" {
		if _, found := m.taskByID(claim); found {
			taskState, taskTok = "task visible", "grn"
		} else {
			taskState, taskTok = "task gap", "red"
		}
	}
	eventCount := len(m.sessionRelatedEvents(s))
	var b strings.Builder
	line := func(label, value, tok string) {
		if strings.TrimSpace(value) == "" {
			value, tok = "·", "mut"
		}
		writeWrappedKV(&b, label, value, tok, w)
	}
	writeSectionHeader(&b, w, "SELECTED LANE FIT", "current split source; evidence, not dispatch authority", "source evidence; no dispatch")
	line("lane", sessionFieldValueForAir(s, "role", m.AIR), airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR))
	line("platform", sessionFieldValueForAir(s, "platform", m.AIR), "2nd")
	line("readiness", sessionFieldValueForAir(s, "readiness", m.AIR), airHue(readinessPaneToken(s.Readiness), s.AIR, "readiness", m.AIR))
	line("fit", fit, fitTok)
	line("claimed", sessionFieldValueForAir(s, "claimed_task", m.AIR), "pri")
	line("task", taskState, taskTok)
	line("events", fmt.Sprintf("%d actor/task events", eventCount), countToken(eventCount))
	line("authority", "preview only; governed route required", "yel")
	routeText, routeTok := m.selectedLaneRoutePosture(s)
	line("routes", routeText, routeTok)
	capText, capTok := m.selectedLaneCapabilityPosture(s)
	line("capability", capText, capTok)
	toolText, toolTok := m.selectedLaneToolPosture(s)
	line("tools", toolText, toolTok)
	posture := fmt.Sprintf("resume:%s · task:%s · trace:%d · hkp:support-only", fit, taskState, eventCount)
	writeWrappedKV(&b, "posture", posture, "mut", w)
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderCapabilitySplitPane(w, h int) string {
	return m.referenceSliceFromLines(m.capabilitySplitPaneLines(w), h)
}

func (m Model) capabilitySplitPaneLines(w int) []string {
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	if selected := strings.TrimRight(m.renderSelectedCapabilityFit(w), "\n"); selected != "" {
		b.WriteString(selected)
		b.WriteString("\n" + rule + "\n")
	}
	if s, ok := m.FocusedSession(); ok {
		m.renderCapabilitySplitRouteBinding(&b, w, s)
		b.WriteString(rule + "\n")
		m.renderCapabilitySplitTools(&b, w, s)
		b.WriteString(rule + "\n")
	}
	m.renderCapabilitySplitNextLegal(&b, w)
	b.WriteString(rule + "\n")
	m.renderCapabilitySplitMatch(&b, w)
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) renderCapabilitySplitRouteBinding(b *strings.Builder, w int, s grammar.Session) {
	writeSectionHeader(b, w, "ROUTE BINDING", "lane-linked evidence; not launch authority", "route evidence")
	routeText, routeTok := m.selectedLaneRoutePosture(s)
	writeWrappedKV(b, "route", routeText, routeTok, w)
	binding := sessionFieldValueForAir(s, "route_binding_state", m.AIR)
	if strings.TrimSpace(binding) == "" {
		binding = "unbound"
	}
	mode := strings.TrimSpace(sessionFieldValueForAir(s, "mode", m.AIR))
	profile := strings.TrimSpace(sessionFieldValueForAir(s, "profile", m.AIR))
	bindingParts := []string{routeBindingLabel(binding)}
	if mode != "" {
		bindingParts = append(bindingParts, "mode="+mode)
	}
	if profile != "" {
		bindingParts = append(bindingParts, "profile="+profile)
	}
	writeWrappedKV(b, "binding", strings.Join(bindingParts, " · "), routeBindingToken(binding), w)
	routeID := strings.TrimSpace(sessionFieldValueForAir(s, "route_id", m.AIR))
	if routeID == "" || routeID == "▒▒▒" {
		writeWrappedKV(b, "route id", "not session-bound; platform evidence remains candidate only", "yel", w)
		return
	}
	route, ok := m.capabilityRouteByID(strings.TrimSpace(s.RouteID))
	if !ok {
		writeWrappedKV(b, "route id", routeID+" · descriptor missing from capability registry", "red", w)
		return
	}
	writeWrappedKV(b, "route state", fmt.Sprintf("%s · fresh=%s · quota=%s · receipts=%s · evidence=%s",
		capabilityRouteFieldForAir(route, "route_state", m.AIR),
		capabilityRouteFieldForAir(route, "freshness_ok", m.AIR),
		capabilityRouteFieldForAir(route, "quota_state", m.AIR),
		capabilityRouteFieldForAir(route, "receipt_count", m.AIR),
		capabilityRouteFieldForAir(route, "evidence_count", m.AIR)), capabilityRouteToken(route), w)
	writeWrappedKV(b, "authority", capabilityRouteFieldForAir(route, "authority_ceiling", m.AIR), "yel", w)
	writeWrappedKV(b, "descriptor", capabilityRouteDescriptorSummary(route, m.AIR), routeAxisToken(route), w)
}

func (m Model) renderCapabilitySplitTools(b *strings.Builder, w int, s grammar.Session) {
	writeSectionHeader(b, w, "TOOLS", "candidate route tools; exact only when route-bound", "tool evidence")
	toolText, toolTok := m.selectedLaneToolPosture(s)
	writeWrappedKV(b, "posture", toolText, toolTok, w)
	var tools []grammar.CapabilityTool
	routeID := strings.TrimSpace(s.RouteID)
	if routeID != "" && !(m.AIR && s.AIR["route_id"] != "ok") {
		tools = m.capabilityToolsForRoute(routeID)
	} else {
		tools = m.capabilityToolsForPlatform(strings.TrimSpace(s.Platform))
	}
	if len(tools) == 0 {
		writeWrappedKV(b, "tools", "none visible; candidate capability still requires route receipt", "yel", w)
		return
	}
	limit := len(tools)
	if limit > 4 {
		limit = 4
	}
	for i := 0; i < limit; i++ {
		tool := tools[i]
		writeWrappedKV(b, "tool", fmt.Sprintf("%s:%s · route=%s · available=%s · use=%s",
			capabilityToolFieldForAir(tool, "tool_id", m.AIR),
			capabilityToolFieldForAir(tool, "status", m.AIR),
			capabilityToolFieldForAir(tool, "route_id", m.AIR),
			capabilityToolFieldForAir(tool, "available", m.AIR),
			capabilityToolFieldForAir(tool, "authority_use", m.AIR)), capabilityToolToken(tool), w)
	}
	if len(tools) > limit {
		writeWrappedKV(b, "more", fmt.Sprintf("%d additional route-tool evidence rows", len(tools)-limit), "mut", w)
	}
}

func (m Model) renderCapabilitySplitMatch(b *strings.Builder, w int) {
	rows := m.capabilityStatusRows()
	total, gaps := m.capabilityTitleCounts()
	writeSectionHeader(b, w, "CAPABILITY MATCH", "capabilities are explicit; platform is evidence, not ontology", "capability status")
	writeWrappedKV(b, "registry", fmt.Sprintf("rows:%d · gaps:%d · routes:%d · tools:%d · sources:%d",
		total, gaps, len(m.Capabilities.Routes), len(m.Capabilities.Tools), len(m.Capabilities.Sources)), countWarnToken(gaps), w)
	if rollups := capabilityClassRollups(rows); len(rollups) > 0 {
		writeWrappedKV(b, "class posture", renderCapabilityClassPosture(rollups, w), countWarnToken(gaps), w)
	}
	for _, row := range capabilitySplitPriorityRows(rows) {
		writeWrappedKV(b, row.Name, fmt.Sprintf("status=%s · authority=%s · evidence=%s", row.Status, row.Authority, row.Evidence), row.Token, w)
		if capabilityDetailVisible(row.Missing) {
			label, value, tok := capabilityDetailContext(row.Status, row.Missing)
			writeWrappedKV(b, label, value, tok, w)
		}
	}
}

func capabilitySplitPriorityRows(rows []capabilityStatusRow) []capabilityStatusRow {
	if len(rows) == 0 {
		return nil
	}
	want := []string{
		"route envelope",
		"admission pacing",
		"capability selection",
		"session resume",
		"methodology dispatch",
		"hkp support context",
	}
	out := make([]capabilityStatusRow, 0, len(want))
	used := map[int]bool{}
	for _, needle := range want {
		for i, row := range rows {
			if used[i] {
				continue
			}
			name := strings.ToLower(strings.ReplaceAll(row.Name, "_", " "))
			name = strings.ReplaceAll(name, "+", " ")
			if strings.Contains(name, needle) {
				out = append(out, row)
				used[i] = true
				break
			}
		}
	}
	for i, row := range rows {
		if len(out) >= 6 {
			break
		}
		if used[i] || !capabilityStatusNeedsAttention(row.Status) {
			continue
		}
		out = append(out, row)
		used[i] = true
	}
	if len(out) == 0 {
		limit := len(rows)
		if limit > 4 {
			limit = 4
		}
		out = append(out, rows[:limit]...)
	}
	return out
}

func (m Model) renderCapabilitySplitNextLegal(b *strings.Builder, w int) {
	writeSectionHeader(b, w, "NEXT LEGAL", "inspect, route-preview, or gather evidence; no dispatch from split context", "legal moves")
	writeWrappedKV(b, "authority", "no dispatch here; governed route required before claim/close/PTY/session launch", "yel", w)
	writeWrappedKV(b, "inspect", ":intent show-route · :intent open-trace · [Enter] source detail", "yel", w)
	if hkp, ok := m.capabilityRowByID("hkp_support_context"); ok {
		writeWrappedKV(b, "hkp", fmt.Sprintf("%s · %s · %s",
			capabilityRowFieldForAir(hkp, "status", m.AIR),
			capabilityRowFieldForAir(hkp, "authority", m.AIR),
			capabilityRowFieldForAir(hkp, "hkp_posture", m.AIR)), capabilityStatusToken(hkp.Status), w)
	} else {
		writeWrappedKV(b, "hkp", "support-only context; not source truth, dispatch authority, or calibration evidence", "yel", w)
	}
}

type capabilityPlatformRow struct {
	name                                string
	total, claim, live, stale, off, hot int
	hidden                              int
}

func capabilityPlatformToken(row capabilityPlatformRow) string {
	if row.hidden > 0 {
		return "org"
	}
	if row.off > 0 || row.stale > 0 {
		return "yel"
	}
	return "pri"
}

func (m Model) capabilityPlatformRows() []capabilityPlatformRow {
	rows := map[string]*capabilityPlatformRow{}
	hiddenPlatform := 0
	for _, s := range m.Sessions {
		if m.AIR && s.AIR["platform"] != "ok" {
			hiddenPlatform++
			continue
		}
		platform := strings.TrimSpace(s.Platform)
		if platform == "" {
			platform = "unknown"
		}
		row := rows[platform]
		if row == nil {
			row = &capabilityPlatformRow{name: platform}
			rows[platform] = row
		}
		row.total++
		if m.AIR && (s.AIR["readiness"] != "ok" || s.AIR["state"] != "ok" || s.AIR["blocker"] != "ok" || s.AIR["attention"] != "ok") {
			row.hidden++
			continue
		}
		switch s.Readiness {
		case "claim":
			row.claim++
		case "stale":
			row.stale++
		}
		switch s.State {
		case "active":
			row.live++
		}
		if s.Readiness == "off" || s.Readiness == "offline" || s.State == "offline" {
			row.off++
		}
		if s.Attention >= 0.50 || (s.Blocker != "" && s.Blocker != "none") {
			row.hot++
		}
	}
	out := make([]capabilityPlatformRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, *row)
	}
	if hiddenPlatform > 0 {
		out = append(out, capabilityPlatformRow{name: "▒▒▒", total: hiddenPlatform, hidden: hiddenPlatform})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].total != out[j].total {
			return out[i].total > out[j].total
		}
		return out[i].name < out[j].name
	})
	return out
}

type capabilityStatusRow struct {
	Name, Status, Token, Authority, Evidence, Missing string
	Class, Family, Spend, Egress, Receipt             string
	SourceRefs                                        string
	SourceRefLabels                                   []string
	Group                                             string
	RouteCount, OKCount, BlockedCount, EvidenceCount  int
}

func (m Model) capabilityTitleCounts() (total, blocked int) {
	rows := m.capabilityStatusRows()
	for _, row := range rows {
		if capabilityStatusNeedsAttention(row.Status) {
			blocked++
		}
	}
	return len(rows), blocked
}

func capabilityStatusNeedsAttention(status string) bool {
	switch status {
	case "observed", "preview-only", "support-only":
		return false
	}
	return true
}

func (m Model) capabilityStatusRows() []capabilityStatusRow {
	if len(m.Capabilities.Rows) > 0 {
		out := make([]capabilityStatusRow, 0, len(m.Capabilities.Rows))
		for _, row := range m.Capabilities.Rows {
			class := capabilityRowFieldForAir(row, "capability_class", m.AIR)
			family := capabilityRowFieldForAir(row, "surface_family", m.AIR)
			evidence := fmt.Sprintf("routes:%d ok:%d blocked:%d evidence:%d · %s/%s", row.RouteCount, row.OKCount, row.BlockedCount, row.EvidenceCount, class, family)
			statusRow := capabilityStatusRow{
				Name:            capabilityRowFieldForAir(row, "capability_id", m.AIR),
				Status:          capabilityRowFieldForAir(row, "status", m.AIR),
				Token:           capabilityStatusToken(row.Status),
				Authority:       capabilityRowFieldForAir(row, "authority", m.AIR),
				Evidence:        evidence,
				Missing:         capabilityRowFieldForAir(row, "blocker", m.AIR),
				Class:           class,
				Family:          family,
				Spend:           capabilityRowFieldForAir(row, "spend_model", m.AIR),
				Egress:          capabilityRowFieldForAir(row, "egress_class", m.AIR),
				Receipt:         capabilityRowFieldForAir(row, "receipt_requirement", m.AIR),
				SourceRefs:      capabilityRowFieldForAir(row, "source_refs", m.AIR),
				SourceRefLabels: capabilityRowSourceRefLabelsForAir(row, m.AIR),
				RouteCount:      row.RouteCount,
				OKCount:         row.OKCount,
				BlockedCount:    row.BlockedCount,
				EvidenceCount:   row.EvidenceCount,
			}
			statusRow.Group = capabilityStatusSourceGroup(statusRow)
			out = append(out, statusRow)
		}
		return out
	}
	platforms, hot := m.capabilityPlatformSummary()
	taskState := "no-subject"
	if len(m.Tasks) > 0 {
		taskState = "preview-only"
	}
	sessionState := "no-subject"
	if len(m.Sessions) > 0 {
		sessionState = "preview-only"
	}
	eventState := "no-subject"
	if len(m.Events) > 0 {
		eventState = "preview-only"
	}
	return []capabilityStatusRow{
		capRow("route envelope", "read-missing", "none", fmt.Sprintf("intents:%d", len(lookupIntentArgs())), "route_id/demand/eligibility/receipt"),
		capRow("admission + pacing", "admission-incomplete", "projection", fmt.Sprintf("tasks:%d sessions:%d", len(m.Tasks), len(m.Sessions)), "admission ledger/WIP policy"),
		capRow("WIP + fanout", "observed", "projection", fmt.Sprintf("lanes:%d hot:%d", len(m.Sessions), hot), "throughput model"),
		capRow("capability selection", "descriptor-missing", "projection", fmt.Sprintf("platforms:%d", platforms), "capability descriptor registry"),
		capRow("session resume", sessionState, "governed route", fmt.Sprintf("sessions:%d", len(m.Sessions)), "resume command receipt"),
		capRow("methodology dispatch", taskState, "governed route", fmt.Sprintf("tasks:%d", len(m.Tasks)), "dispatch route receipt"),
		capRow("claim/close/release", taskState, "governed route", fmt.Sprintf("tasks:%d", len(m.Tasks)), "cc-claim/cc-close receipts"),
		capRow("reviewer selection", "read-missing", "sub-router", "none", "review route receipts"),
		capRow("watcher/CCTV routing", "read-missing", "sub-router", "none", "watch cadence/evidence refs"),
		capRow("request hardening", "read-missing", "sub-router", "none", "intensity/axes/budget/receipt"),
		capRow("eval-plane choice", "read-missing", "sub-router", "none", "harness/oracle fit"),
		capRow("verifier/floor-check", "read-missing", "sub-router", "none", "gate/floor checker state"),
		capRow("learning eligibility", "read-missing", "calibration", "none", "independent accept/reject receipts"),
		capRow("quota/context economics", "read-missing", "admission", "none", "quota/context/fixed overhead"),
		capRow("HKP support context", "support-only", "authority-capped", "support doctrine", "promotion + cited-source verification"),
		capRow("provider gateway", "spend-forbidden", "spend forbidden", "none", "provider spend route receipt"),
		capRow("source acquisition", "read-missing", "sub-router", "none", "source egress/quota route receipt"),
		capRow("trace/open-route", eventState, "preview", fmt.Sprintf("events:%d", len(m.Events)), "trace/route evidence receipt"),
	}
}

func groupCapabilityStatusRows(rows []capabilityStatusRow) map[string][]capabilityStatusRow {
	grouped := map[string][]capabilityStatusRow{
		"class":   {},
		"surface": {},
		"core":    {},
		"score":   {},
	}
	for _, row := range rows {
		key := row.Group
		if key == "" {
			key = capabilityStatusGroup(row.Name)
		}
		grouped[key] = append(grouped[key], row)
	}
	return grouped
}

func (m Model) capabilityDisplayRows() []capabilityStatusRow {
	grouped := groupCapabilityStatusRows(m.capabilityStatusRows())
	out := make([]capabilityStatusRow, 0, len(m.capabilityStatusRows()))
	for _, key := range []string{"class", "surface", "core", "score"} {
		out = append(out, grouped[key]...)
	}
	return out
}

func (m Model) capabilityFocusScrollOffset(focus int) int {
	rows := m.capabilityDisplayRows()
	if len(rows) == 0 {
		return 0
	}
	focus = clamp(focus, 0, len(rows)-1)
	grouped := groupCapabilityStatusRows(m.capabilityStatusRows())
	line := 0
	// Page title, read-only line, source chips, and rule.
	line += 4
	rollups := capabilityClassRollups(m.capabilityStatusRows())
	if len(m.Capabilities.Rows) > 0 && len(rollups) > 0 {
		line++
	}
	displayIdx := 0
	if classRows := grouped["class"]; len(classRows) > 0 {
		line += 2 // section header + table header
		for range classRows {
			if displayIdx == focus {
				return clamp(line-4, 0, m.referenceScrollMax())
			}
			line++
			displayIdx++
		}
		line++ // rule
	}
	if m.sessionSplit() && !m.SuppressSplitPinned {
		line += 2 // selected lane fit + rule
	}
	if len(m.Capabilities.Sources) > 0 {
		line += 2 + len(m.Capabilities.Sources) + 1
	}
	// Fabric scope section: header + bullets + rule.
	line += 1 + 4 + 1
	// CAPABILITY STATUS header.
	line += 1
	for _, key := range []string{"surface", "core", "score"} {
		sectionRows := grouped[key]
		if len(sectionRows) == 0 {
			continue
		}
		line += 2 // section header + table header
		for _, row := range sectionRows {
			if displayIdx == focus {
				return clamp(line-4, 0, m.referenceScrollMax())
			}
			line++
			if capabilityDetailVisible(row.Missing) {
				line++
			}
			displayIdx++
		}
		line++ // rule
	}
	return clamp(line, 0, m.referenceScrollMax())
}

func capabilityStatusGroup(name string) string {
	switch name {
	case "route_envelope", "quota_context", "route_authority_receipts", "hkp_support_context":
		return "core"
	case "source_acquisition", "verifier_floor_checker", "publication_egress", "audio_avsdlc_tool", "provider_gateway", "subscription_tool_surface", "infrastructure_control":
		return "class"
	case "tavily_source_acquisition", "perplexity_source_acquisition", "context7_docs_currentness", "google_drive_docs_connector",
		"github_repo_ci", "codex_worker_reviewer_surface", "claude_worker_reviewer_surface", "antigravity_agy_tool_surface",
		"mistral_vibe_worker_surface", "glmcp_review_quota_admission", "gemini_agy_support_review", "glm_coding_plan_tool_surface",
		"huggingface_provider_gateway", "cohere_embed_rerank", "litellm_provider_gateway",
		"openrouter_break_glass", "semgrep_static_analysis", "codecov_coverage_signal", "codeql_status_floor",
		"deterministic_test_floor", "runtime_witness_floor", "worker_failure_witness", "langfuse_trace_eval",
		"elevenlabs_audio_generation", "picovoice_audio_verifier", "audio_fingerprint_source_id",
		"media_catalog_publication_support", "research_publication_deposit", "public_social_distribution",
		"google_workspace_youtube_connector", "research_storage_infra", "network_admin_tailscale", "local_inference_eval":
		return "surface"
	default:
		return "score"
	}
}

func capabilityStatusSourceGroup(row capabilityStatusRow) string {
	nameGroup := capabilityStatusGroup(row.Name)
	if nameGroup == "core" || nameGroup == "class" {
		return nameGroup
	}
	class := strings.TrimSpace(row.Class)
	family := strings.TrimSpace(row.Family)
	switch {
	case class == "routing_family":
		return "class"
	case class == "score_dimension" || class == "scoring_dimension" || class == "capability_score" || class == "registry_score" || family == "platform_capability_registry":
		return "score"
	case class != "" && class != "▒▒▒" && family != "" && family != "▒▒▒":
		return "surface"
	}
	return nameGroup
}

func renderCapabilityStatusRows(b *strings.Builder, w int, title, detail, label string, rows []capabilityStatusRow, selected int, displayIdx *int) {
	writeSectionHeader(b, w, title, detail, label)
	if w < 132 {
		for _, c := range rows {
			name := c.Name
			if displayIdx != nil && *displayIdx == selected {
				name = "▶ " + name
			}
			writeWrappedKV(b, name, fmt.Sprintf("class=%s · status=%s · authority=%s · evidence=%s · source=%s", c.Class, c.Status, c.Authority, c.Evidence, c.SourceRefs), c.Token, w)
			if capabilityDetailVisible(c.Missing) {
				label, detail, tok := capabilityDetailContext(c.Status, c.Missing)
				writeWrappedKV(b, label, detail, tok, w)
			}
			if displayIdx != nil {
				*displayIdx = *displayIdx + 1
			}
		}
		return
	}
	b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-24s %-19s %-18s %s", "CAPABILITY", "STATUS", "AUTHORITY", "EVIDENCE")) + "\n")
	for _, c := range rows {
		evW := maxVisible(12, w-68)
		line := " " +
			grammar.C("pri", fmt.Sprintf("%-24s", clipRunes(c.Name, 24))) +
			grammar.C(c.Token, fmt.Sprintf(" %-19s", clipRunes(c.Status, 19))) +
			grammar.C("2nd", fmt.Sprintf(" %-18s", clipRunes(c.Authority, 18))) +
			grammar.C("mut", " "+clipRunes(c.Evidence, evW))
		if displayIdx != nil && *displayIdx == selected {
			b.WriteString(focusBar(line, w) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
		if capabilityDetailVisible(c.Missing) {
			label, detail, tok := capabilityDetailContext(c.Status, c.Missing)
			writeWrappedKV(b, label, detail, tok, w)
		}
		if displayIdx != nil {
			*displayIdx = *displayIdx + 1
		}
	}
}

func renderCapabilityClassCockpit(b *strings.Builder, w int, classes, surfaces []capabilityStatusRow, selected int, displayIdx *int) {
	writeSectionHeader(b, w, "CAPABILITY CLASS COCKPIT", "class, status, authority, route readiness, surfaces, evidence, spend, and egress", "class/status/authority/evidence")
	if w < 132 {
		for _, c := range classes {
			surfaceCount, surfaceBlocked := capabilitySurfaceStats(c, surfaces)
			name := c.Name
			if displayIdx != nil && *displayIdx == selected {
				name = "▶ " + name
			}
			writeWrappedKV(b, name,
				fmt.Sprintf("status=%s · authority=%s · ready=%d/%d · route-block=%d · surfaces=%d/%d · evidence=%d · spend=%s · egress=%s",
					c.Status, c.Authority, c.OKCount, c.RouteCount, c.BlockedCount, surfaceCount-surfaceBlocked, surfaceCount, c.EvidenceCount, c.Spend, c.Egress),
				c.Token, w)
			if displayIdx != nil {
				*displayIdx = *displayIdx + 1
			}
		}
		return
	}
	b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-22s %-18s %-18s %8s %9s %8s %-18s %s", "CLASS", "STATUS", "AUTHORITY", "READY", "SURFACES", "EVID", "SPEND", "EGRESS")) + "\n")
	for _, c := range classes {
		surfaceCount, surfaceBlocked := capabilitySurfaceStats(c, surfaces)
		surf := fmt.Sprintf("%d/%d", surfaceCount-surfaceBlocked, surfaceCount)
		line := " " +
			grammar.C("pri", fmt.Sprintf("%-22s", clipRunes(c.Name, 22))) +
			grammar.C(c.Token, fmt.Sprintf(" %-18s", clipRunes(c.Status, 18))) +
			grammar.C("2nd", fmt.Sprintf(" %-18s", clipRunes(c.Authority, 18))) +
			grammar.C(countToken(c.OKCount), fmt.Sprintf(" %8s", fmt.Sprintf("%d/%d", c.OKCount, c.RouteCount))) +
			grammar.C(countWarnToken(surfaceBlocked), fmt.Sprintf(" %9s", surf)) +
			grammar.C(countToken(c.EvidenceCount), fmt.Sprintf(" %8d", c.EvidenceCount)) +
			grammar.C("2nd", fmt.Sprintf(" %-18s", clipRunes(c.Spend, 18))) +
			grammar.C("mut", " "+clipRunes(c.Egress, maxVisible(8, w-110)))
		if displayIdx != nil && *displayIdx == selected {
			b.WriteString(focusBar(line, w) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
		if displayIdx != nil {
			*displayIdx = *displayIdx + 1
		}
	}
}

func renderCapabilityTopology(b *strings.Builder, w int, classes, surfaces []capabilityStatusRow) {
	if w < 132 {
		for _, c := range classes {
			surfaceCount, surfaceBlocked := capabilitySurfaceStats(c, surfaces)
			writeWrappedKV(b, c.Name,
				fmt.Sprintf("status=%s · readiness ok:%d/%d blocked:%d · surfaces:%d blocked:%d · spend=%s · egress=%s",
					c.Status, c.OKCount, c.RouteCount, c.BlockedCount, surfaceCount, surfaceBlocked, c.Spend, c.Egress),
				c.Token, w)
		}
		return
	}
	b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-24s %-19s %8s %8s %8s %-22s %s", "FAMILY", "STATUS", "READY", "BLOCK", "SURF", "SPEND", "EGRESS")) + "\n")
	for _, c := range classes {
		surfaceCount, surfaceBlocked := capabilitySurfaceStats(c, surfaces)
		surf := fmt.Sprintf("%d/%d", surfaceCount-surfaceBlocked, surfaceCount)
		line := " " +
			grammar.C("pri", fmt.Sprintf("%-24s", clipRunes(c.Name, 24))) +
			grammar.C(c.Token, fmt.Sprintf(" %-19s", clipRunes(c.Status, 19))) +
			grammar.C(countToken(c.OKCount), fmt.Sprintf(" %8s", fmt.Sprintf("%d/%d", c.OKCount, c.RouteCount))) +
			grammar.C(countWarnToken(c.BlockedCount), fmt.Sprintf(" %8d", c.BlockedCount)) +
			grammar.C(countWarnToken(surfaceBlocked), fmt.Sprintf(" %8s", surf)) +
			grammar.C("2nd", fmt.Sprintf(" %-22s", clipRunes(c.Spend, 22))) +
			grammar.C("mut", " "+clipRunes(c.Egress, maxVisible(8, w-91)))
		b.WriteString(line + "\n")
		if capabilityDetailVisible(c.Missing) {
			label, detail, tok := capabilityDetailContext(c.Status, c.Missing)
			writeWrappedKV(b, label, detail, tok, w)
		}
	}
}

func capabilitySurfaceStats(class capabilityStatusRow, surfaces []capabilityStatusRow) (total, blocked int) {
	for _, surface := range surfaces {
		if surface.Class != class.Name && surface.Class != class.Family {
			continue
		}
		total++
		if capabilityStatusNeedsAttention(surface.Status) {
			blocked++
		}
	}
	return total, blocked
}

type capabilityClassRollup struct {
	Class                                     string
	Rows, Attention                           int
	Observed, Preview, Support                int
	OKRoutes, Routes, BlockedRoutes, Evidence int
	Spend, Egress                             string
}

func capabilityClassRollups(rows []capabilityStatusRow) []capabilityClassRollup {
	byClass := map[string]*capabilityClassRollup{}
	spendSets := map[string]map[string]bool{}
	egressSets := map[string]map[string]bool{}
	for _, row := range rows {
		class := strings.TrimSpace(row.Class)
		if class == "" || class == "▒▒▒" {
			class = "unclassified"
		}
		roll := byClass[class]
		if roll == nil {
			roll = &capabilityClassRollup{Class: class}
			byClass[class] = roll
			spendSets[class] = map[string]bool{}
			egressSets[class] = map[string]bool{}
		}
		roll.Rows++
		if capabilityStatusNeedsAttention(row.Status) {
			roll.Attention++
		}
		switch row.Status {
		case "observed":
			roll.Observed++
		case "preview-only":
			roll.Preview++
		case "support-only":
			roll.Support++
		}
		roll.OKRoutes += row.OKCount
		roll.Routes += row.RouteCount
		roll.BlockedRoutes += row.BlockedCount
		roll.Evidence += row.EvidenceCount
		if spend := strings.TrimSpace(row.Spend); spend != "" && spend != "▒▒▒" {
			spendSets[class][spend] = true
		}
		if egress := strings.TrimSpace(row.Egress); egress != "" && egress != "▒▒▒" {
			egressSets[class][egress] = true
		}
	}
	out := make([]capabilityClassRollup, 0, len(byClass))
	for class, roll := range byClass {
		roll.Spend = compactSet(spendSets[class], 2)
		roll.Egress = compactSet(egressSets[class], 2)
		out = append(out, *roll)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Attention != out[j].Attention {
			return out[i].Attention > out[j].Attention
		}
		if out[i].Rows != out[j].Rows {
			return out[i].Rows > out[j].Rows
		}
		return out[i].Class < out[j].Class
	})
	return out
}

func compactSet(set map[string]bool, limit int) string {
	if len(set) == 0 {
		return "unknown"
	}
	vals := make([]string, 0, len(set))
	for v := range set {
		vals = append(vals, v)
	}
	sort.Strings(vals)
	if limit > 0 && len(vals) > limit {
		return strings.Join(vals[:limit], ",") + fmt.Sprintf(",+%d", len(vals)-limit)
	}
	return strings.Join(vals, ",")
}

func renderCapabilityClassPosture(rows []capabilityClassRollup, w int) string {
	if len(rows) == 0 {
		return grammar.C("mut", "no source-backed classes")
	}
	totalRows, attention, okRoutes, routes := 0, 0, 0, 0
	for _, row := range rows {
		totalRows += row.Rows
		attention += row.Attention
		okRoutes += row.OKRoutes
		routes += row.Routes
	}
	budget := maxVisible(12, w-17)
	sep := grammar.C("mut", " · ")
	line := grammar.C(countWarnToken(attention), fmt.Sprintf("classes:%d rows:%d !%d ready:%d/%d", len(rows), totalRows, attention, okRoutes, routes))
	added := 0
	for i := range rows {
		row := rows[i]
		tok := "grn"
		if row.Attention > 0 {
			tok = "org"
		}
		label := fmt.Sprintf("%s %d/%d", clipRunes(row.Class, 18), row.OKRoutes, row.Routes)
		if row.Attention > 0 {
			label += fmt.Sprintf(" !%d", row.Attention)
		}
		candidate := sep + grammar.C(tok, label)
		remaining := len(rows) - i - 1
		reserve := 0
		if remaining > 0 {
			reserve = ansi.StringWidth(sep) + len(fmt.Sprintf("+%d", remaining))
		}
		if ansi.StringWidth(line)+ansi.StringWidth(candidate)+reserve > budget {
			break
		}
		line += candidate
		added++
	}
	if len(rows) > added {
		more := sep + grammar.C("mut", fmt.Sprintf("+%d classes", len(rows)-added))
		if ansi.StringWidth(line)+ansi.StringWidth(more) <= budget {
			line += more
		}
	}
	return ansi.Truncate(line, budget, "")
}

func renderCapabilityClassRollups(b *strings.Builder, w int, rows []capabilityClassRollup) {
	if w < 132 {
		for _, row := range rows {
			tok := "grn"
			if row.Attention > 0 {
				tok = "org"
			}
			writeWrappedKV(b, row.Class,
				fmt.Sprintf("rows:%d attention:%d ready:%d/%d blocked-routes:%d evidence:%d · spend=%s · egress=%s",
					row.Rows, row.Attention, row.OKRoutes, row.Routes, row.BlockedRoutes, row.Evidence, row.Spend, row.Egress),
				tok, w)
		}
		return
	}
	b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-24s %5s %9s %9s %8s %10s %-22s %s", "CLASS", "ROWS", "ATTENTION", "READY", "BLOCK", "EVIDENCE", "SPEND", "EGRESS")) + "\n")
	for _, row := range rows {
		tok := "grn"
		if row.Attention > 0 {
			tok = "org"
		}
		line := " " +
			grammar.C("pri", fmt.Sprintf("%-24s", clipRunes(row.Class, 24))) +
			grammar.C(countToken(row.Rows), fmt.Sprintf(" %5d", row.Rows)) +
			grammar.C(tok, fmt.Sprintf(" %9d", row.Attention)) +
			grammar.C(countToken(row.OKRoutes), fmt.Sprintf(" %9s", fmt.Sprintf("%d/%d", row.OKRoutes, row.Routes))) +
			grammar.C(countWarnToken(row.BlockedRoutes), fmt.Sprintf(" %8d", row.BlockedRoutes)) +
			grammar.C(countToken(row.Evidence), fmt.Sprintf(" %10d", row.Evidence)) +
			grammar.C("2nd", fmt.Sprintf(" %-22s", clipRunes(row.Spend, 22))) +
			grammar.C("mut", " "+clipRunes(row.Egress, maxVisible(8, w-96)))
		b.WriteString(line + "\n")
	}
}

func (m Model) capabilityRowByID(id string) (grammar.CapabilityRow, bool) {
	for _, row := range m.Capabilities.Rows {
		if row.CapabilityID == id {
			return row, true
		}
	}
	return grammar.CapabilityRow{}, false
}

func capabilitySourceFieldForAir(src grammar.CapabilitySource, field string, air bool) string {
	if air && src.AIR[field] != "ok" {
		return "▒▒▒"
	}
	switch field {
	case "id":
		return src.ID
	case "path":
		return src.Path
	case "exists":
		return fmt.Sprintf("%v", src.Exists)
	case "mtime":
		return src.MTime
	case "age_bucket":
		return src.AgeBucket
	case "status":
		return src.Status
	case "count":
		return fmt.Sprintf("%d", src.Count)
	case "detail":
		return src.Detail
	case "privacy":
		return src.Privacy
	case "raw_access":
		return fmt.Sprintf("%v", src.RawAccess)
	}
	return ""
}

func capabilityRowFieldForAir(row grammar.CapabilityRow, field string, air bool) string {
	if air && row.AIR[field] != "ok" {
		return "▒▒▒"
	}
	switch field {
	case "capability_id":
		return row.CapabilityID
	case "status":
		return row.Status
	case "authority":
		return row.Authority
	case "capability_class":
		return row.CapabilityClass
	case "surface_family":
		return row.SurfaceFamily
	case "spend_model":
		return row.SpendModel
	case "egress_class":
		return row.EgressClass
	case "receipt_requirement":
		return row.ReceiptRequirement
	case "route_count":
		return fmt.Sprintf("%d", row.RouteCount)
	case "ok_count":
		return fmt.Sprintf("%d", row.OKCount)
	case "blocked_count":
		return fmt.Sprintf("%d", row.BlockedCount)
	case "evidence_count":
		return fmt.Sprintf("%d", row.EvidenceCount)
	case "blocker":
		return row.Blocker
	case "hkp_posture":
		return row.HKPPosture
	case "source_refs":
		return row.SourceRefs
	}
	return ""
}

func capabilityRowSourceRefLabelsForAir(row grammar.CapabilityRow, air bool) []string {
	if air && row.AIR["source_ref_labels"] != "ok" {
		return []string{"▒▒▒"}
	}
	out := make([]string, 0, len(row.SourceRefLabels))
	for _, label := range row.SourceRefLabels {
		label = strings.TrimSpace(label)
		if label != "" {
			out = append(out, label)
		}
	}
	return out
}

func capabilityRouteFieldForAir(route grammar.CapabilityRoute, field string, air bool) string {
	if air && route.AIR[field] != "ok" {
		return "▒▒▒"
	}
	switch field {
	case "route_id":
		return route.RouteID
	case "capability_id":
		return route.CapabilityID
	case "platform":
		return route.Platform
	case "mode":
		return route.Mode
	case "profile":
		return route.Profile
	case "model_id":
		return route.ModelID
	case "effort":
		return route.Effort
	case "context_mode":
		return route.ContextMode
	case "fast_mode":
		return route.FastMode
	case "quantization":
		return route.Quantization
	case "capacity_pool":
		return route.CapacityPool
	case "demand_vector":
		return route.DemandVector
	case "hardening":
		return route.Hardening
	case "eval_plane":
		return route.EvalPlane
	case "review_obligation":
		return route.ReviewObligation
	case "learning_eligibility":
		return route.LearningEligibility
	case "benchmark_coverage":
		return route.BenchmarkCoverage
	case "fixed_overhead":
		return route.FixedOverhead
	case "route_state":
		return route.RouteState
	case "authority_ceiling":
		return route.AuthorityCeiling
	case "freshness_ok":
		return fmt.Sprintf("%v", route.FreshnessOK)
	case "quota_state":
		return route.QuotaState
	case "receipt_count":
		return fmt.Sprintf("%d", route.ReceiptCount)
	case "blockers":
		return strings.Join(route.Blockers, " · ")
	case "evidence_count":
		return fmt.Sprintf("%d", route.EvidenceCount)
	}
	return ""
}

func capabilityRouteAxisMissing(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "" || s == "missing" || s == "unknown" || s == "none"
}

func routeAxisToken(route grammar.CapabilityRoute) string {
	for _, v := range []string{
		route.Effort,
		route.ContextMode,
		route.FastMode,
		route.Quantization,
		route.CapacityPool,
		route.DemandVector,
		route.Hardening,
		route.EvalPlane,
		route.ReviewObligation,
		route.LearningEligibility,
		route.BenchmarkCoverage,
		route.FixedOverhead,
	} {
		if capabilityRouteAxisMissing(v) {
			return "yel"
		}
	}
	return "grn"
}

func capabilityRouteDescriptorSummary(route grammar.CapabilityRoute, air bool) string {
	parts := []string{
		"model=" + capabilityRouteAxisForDisplay(route, "model_id", air),
		"effort=" + capabilityRouteAxisForDisplay(route, "effort", air),
		"context=" + capabilityRouteAxisForDisplay(route, "context_mode", air),
		"fast=" + capabilityRouteAxisForDisplay(route, "fast_mode", air),
		"quant=" + capabilityRouteAxisForDisplay(route, "quantization", air),
		"pool=" + capabilityRouteAxisForDisplay(route, "capacity_pool", air),
	}
	return strings.Join(parts, " · ")
}

func capabilityRouteGovernanceSummary(route grammar.CapabilityRoute, air bool) string {
	parts := []string{
		"demand=" + capabilityRouteAxisForDisplay(route, "demand_vector", air),
		"hardening=" + capabilityRouteAxisForDisplay(route, "hardening", air),
		"eval=" + capabilityRouteAxisForDisplay(route, "eval_plane", air),
		"review=" + capabilityRouteAxisForDisplay(route, "review_obligation", air),
		"learning=" + capabilityRouteAxisForDisplay(route, "learning_eligibility", air),
		"bench=" + capabilityRouteAxisForDisplay(route, "benchmark_coverage", air),
		"overhead=" + capabilityRouteAxisForDisplay(route, "fixed_overhead", air),
	}
	return strings.Join(parts, " · ")
}

func capabilityRouteAxisForDisplay(route grammar.CapabilityRoute, field string, air bool) string {
	value := capabilityRouteFieldForAir(route, field, air)
	if strings.TrimSpace(value) == "" {
		return "missing"
	}
	return value
}

func capabilityToolFieldForAir(tool grammar.CapabilityTool, field string, air bool) string {
	if air && tool.AIR[field] != "ok" {
		return "▒▒▒"
	}
	switch field {
	case "route_id":
		return tool.RouteID
	case "platform":
		return tool.Platform
	case "tool_id":
		return tool.ToolID
	case "status":
		return tool.Status
	case "available":
		return fmt.Sprintf("%v", tool.Available)
	case "authority_use":
		return tool.AuthorityUse
	case "observed_at":
		return tool.ObservedAt
	case "stale_after":
		return tool.StaleAfter
	case "evidence_ref":
		return tool.EvidenceRef
	case "privacy":
		return tool.Privacy
	case "raw_access":
		return fmt.Sprintf("%v", tool.RawAccess)
	}
	return ""
}

func gateSourceFieldForAir(src grammar.GateSource, field string, air bool) string {
	if air && src.AIR[field] != "ok" {
		return "▒▒▒"
	}
	switch field {
	case "id":
		return src.ID
	case "status":
		return src.Status
	case "count":
		return fmt.Sprintf("%d", src.Count)
	case "detail":
		return src.Detail
	case "age_bucket":
		return src.AgeBucket
	case "path":
		return src.Path
	case "raw_access":
		return fmt.Sprintf("%v", src.RawAccess)
	}
	return ""
}

func gateRowFieldForAir(row grammar.GateRow, field string, air bool) string {
	if air && row.AIR[field] != "ok" {
		return "▒▒▒"
	}
	switch field {
	case "gate_id":
		return row.GateID
	case "domain":
		return row.Domain
	case "source":
		return row.Source
	case "subject":
		return row.Subject
	case "state":
		return row.State
	case "severity":
		return row.Severity
	case "authority":
		return row.Authority
	case "evidence":
		return row.Evidence
	case "missing":
		return row.Missing
	case "action":
		return row.Action
	}
	return ""
}

func (m Model) gateRowsByDomain(domain string) []grammar.GateRow {
	out := make([]grammar.GateRow, 0)
	for _, row := range m.Gates.Rows {
		if row.Domain == domain {
			out = append(out, row)
		}
	}
	return out
}

func (m Model) renderGateRows(b *strings.Builder, rows []grammar.GateRow, w, limit int) {
	if len(rows) == 0 {
		b.WriteString(" " + grammar.C("grn", "no gate rows") + "\n")
		return
	}
	if limit <= 0 || limit > len(rows) {
		limit = len(rows)
	}
	for i := 0; i < limit; i++ {
		b.WriteString(m.renderGateFactRow(rows[i], w) + "\n")
	}
	if len(rows) > limit {
		b.WriteString(" " + grammar.C("mut", fmt.Sprintf("… %d more gate rows", len(rows)-limit)) + "\n")
	}
}

func (m Model) renderGateFactRow(row grammar.GateRow, w int) string {
	tok := gateRowToken(row)
	gateID := gateRowFieldForAir(row, "gate_id", m.AIR)
	state := gateRowFieldForAir(row, "state", m.AIR)
	severity := gateRowFieldForAir(row, "severity", m.AIR)
	domain := gateRowFieldForAir(row, "domain", m.AIR)
	authority := gateRowFieldForAir(row, "authority", m.AIR)
	evidence := gateRowFieldForAir(row, "evidence", m.AIR)
	missing := gateRowFieldForAir(row, "missing", m.AIR)
	subject := gateRowFieldForAir(row, "subject", m.AIR)
	source := gateRowFieldForAir(row, "source", m.AIR)

	mark := gateRowMarker(row)
	prefix := " " +
		grammar.C(tok, mark+" ") +
		grammar.C(tok, fmt.Sprintf("%-12s", clipRunes(state, 12))) +
		grammar.C(tok, fmt.Sprintf(" %-5s", clipRunes(severity, 5))) +
		grammar.C("2nd", fmt.Sprintf(" %-7s", clipRunes(domain, 7)))
	facts := nonEmptyParts([]string{
		gateID,
		gateRowMissingFact(missing),
		evidence,
		authority,
		labeledPart("subj", subject),
		labeledPart("src", source),
	})
	if len(facts) == 0 {
		return prefix
	}
	return strings.Join(wrapSegmentedFactSegments(prefix, facts, w), "\n")
}

func gateRowMissingFact(missing string) string {
	missing = strings.TrimSpace(missing)
	if missing == "" || missing == "none" || missing == "·" {
		return ""
	}
	return labeledPart("missing", missing)
}

func gateRowMarker(row grammar.GateRow) string {
	state := strings.ToLower(strings.TrimSpace(row.State))
	severity := strings.ToLower(strings.TrimSpace(row.Severity))
	switch {
	case severity == "crit" || state == "blocked" || state == "missing" || state == "dark":
		return "✖"
	case severity == "major":
		return "!"
	case severity == "warn" || strings.Contains(state, "preview") || state == "stale":
		return "▸"
	case severity == "ok" || state == "pass" || state == "clear" || state == "observed":
		return "✓"
	default:
		return "·"
	}
}

func gateRowToken(row grammar.GateRow) string {
	switch row.Severity {
	case "crit":
		return "red"
	case "major":
		return "org"
	case "warn":
		return "yel"
	case "ok":
		return "grn"
	}
	switch row.State {
	case "blocked", "missing", "dark":
		return "red"
	case "preview-only", "preview_only", "stale":
		return "yel"
	case "pass", "clear", "observed":
		return "grn"
	}
	return "mut"
}

func sourceStatusToken(status string) string {
	switch status {
	case "observed", "live":
		return "grn"
	case "missing", "error", "dark":
		return "red"
	case "stale", "partial", "support-only", "preview-only":
		return "yel"
	}
	return "mut"
}

func capabilityRouteToken(route grammar.CapabilityRoute) string {
	if !route.FreshnessOK || len(route.Blockers) > 0 || route.RouteState == "blocked" {
		return "red"
	}
	if route.QuotaState == "unknown" {
		return "yel"
	}
	return "grn"
}

func capabilityToolToken(tool grammar.CapabilityTool) string {
	if !tool.Available || strings.EqualFold(tool.Status, "unavailable") {
		return "red"
	}
	switch tool.Status {
	case "observed":
		return "grn"
	case "read-missing", "missing":
		return "red"
	case "stale", "partial":
		return "yel"
	}
	return "mut"
}

func boolToken(v bool) string {
	if v {
		return "grn"
	}
	return "red"
}

func (m Model) capabilityGuardrailBullets() []string {
	if len(m.Capabilities.Rows) == 0 {
		return []string{
			"route envelope absent: route_id/model/effort/context/spend/quota/floor/veto chain",
			"capability descriptors are not yet registered per platform/profile/model/capacity-pool",
			"benchmark coverage, calibration provenance, HKP policy, and learning eligibility are not yet rendered as receipts",
			"role names such as fugu/fugu-ultra are not sufficient evidence of strengths or weaknesses",
			"dispatch remains behind methodology route evidence, authority, preflight, and receipt",
		}
	}
	return []string{
		"capability registry and receipts are visible, but route choice remains read-only until methodology dispatch supplies authority, preflight, and receipt",
		"platform/session labels remain evidence only; route descriptors, freshness, quota, and authority ceilings decide fit",
		"route-tool observations are candidate evidence; exact session tool status needs route_id",
		"HKP remains support-only until source verification, promotion route, redaction policy, consumer policy, and receipt are present",
		"route-decision ledger tail and evaluator/calibration receipts are still not rendered in this slice",
	}
}

func capRow(name, status, authority, evidence, missing string) capabilityStatusRow {
	return capabilityStatusRow{
		Name: name, Status: status, Authority: authority, Evidence: evidence, Missing: missing,
		Class: "fallback", Family: "local_projection", Spend: "unknown", Egress: "unknown", Receipt: missing,
		Token: capabilityStatusToken(status),
	}
}

func capabilityDetailVisible(detail string) bool {
	detail = strings.TrimSpace(detail)
	return detail != "" && detail != "none"
}

func capabilityDetailContext(status, detail string) (label, value, token string) {
	if !capabilityDetailVisible(detail) {
		return "ready", "none", "grn"
	}
	if capabilityStatusNeedsAttention(status) {
		return "missing", detail, "org"
	}
	return "guardrail", detail, "mut"
}

func capabilityStatusToken(status string) string {
	switch status {
	case "observed":
		return "grn"
	case "preview-only", "support-only":
		return "yel"
	case "read-missing", "spend-forbidden":
		return "red"
	case "admission-incomplete", "descriptor-missing", "no-subject", "manual-bakeoff", "raw-manual":
		return "org"
	}
	return "mut"
}

func (m Model) capabilityPlatformSummary() (platforms, hot int) {
	seen := map[string]bool{}
	for _, row := range m.capabilityPlatformRows() {
		if row.name != "▒▒▒" {
			seen[row.name] = true
		}
		hot += row.hot
	}
	return len(seen), hot
}

func capabilityPlatformContract(platform string) string {
	switch strings.ToLower(platform) {
	case "codex":
		return "coding CLI/session lane; route fit needs profile + task shape"
	case "claude":
		return "coding CLI/session lane; route fit needs profile + task shape"
	case "unknown", "":
		return "missing platform metadata; cannot route confidently"
	case "▒▒▒":
		return "platform hidden by AIR; aggregate only"
	}
	return "unprofiled platform; descriptor required before dispatch"
}

func capabilityIntentSubject(target string) string {
	switch target {
	case "resume":
		return "session"
	case "dispatch", "claim", "close", "approve", "deny":
		return "task"
	case "handoff":
		return "workstream"
	case "open-trace":
		return "event/trace"
	case "show-route":
		return "selection"
	}
	return "selection"
}

func (m Model) capabilityIntentState(target string) (string, string) {
	switch target {
	case "resume":
		for _, s := range m.Sessions {
			if s.Readiness == "claim" || s.Readiness == "live" {
				return "subject visible", "yel"
			}
		}
		return "no subject", "mut"
	case "dispatch", "claim", "close", "approve", "deny":
		if len(m.Tasks) > 0 {
			return "subject visible", "yel"
		}
		return "no subject", "mut"
	case "open-trace":
		if len(m.Events) > 0 {
			return "subject visible", "yel"
		}
		return "no subject", "mut"
	}
	return "preview only", "org"
}

func capabilitySessionFit(s grammar.Session, air bool) (string, string) {
	if air && (s.AIR["readiness"] != "ok" || s.AIR["blocker"] != "ok" || s.AIR["state"] != "ok") {
		return "fit hidden", "mut"
	}
	if s.Readiness == "claim" && (s.Blocker == "" || s.Blocker == "none") {
		return "resume-preview", "yel"
	}
	if s.Readiness == "live" {
		return "observe-only", "pri"
	}
	if s.Readiness == "stale" || s.Blocker == "stale_relay" {
		return "verify-first", "org"
	}
	if s.Readiness == "off" || s.State == "offline" {
		return "unavailable", "red"
	}
	if s.Blocker != "" && s.Blocker != "none" {
		return "blocked", "red"
	}
	return "unknown", "mut"
}

func attentionCountToken(n int) string {
	if n > 0 {
		return "yel"
	}
	return "mut"
}

func domainSourceFieldForAir(src grammar.DomainSource, field string, air bool) string {
	if air && src.AIR[field] != "ok" {
		return "▒▒▒"
	}
	switch field {
	case "id":
		return src.ID
	case "path":
		return src.Path
	case "exists":
		return fmt.Sprintf("%v", src.Exists)
	case "status":
		return src.Status
	case "count":
		return fmt.Sprintf("%d", src.Count)
	case "age_bucket":
		return src.AgeBucket
	case "authority":
		return src.Authority
	case "detail":
		return src.Detail
	case "privacy":
		return src.Privacy
	case "raw_access":
		return fmt.Sprintf("%v", src.RawAccess)
	}
	return ""
}

func domainRowFieldForAir(row grammar.DomainRow, field string, air bool) string {
	if air && row.AIR[field] != "ok" {
		return "▒▒▒"
	}
	switch field {
	case "domain_id":
		return row.DomainID
	case "label":
		return row.Label
	case "lifecycle":
		return row.Lifecycle
	case "terrain":
		return row.Terrain
	case "depth":
		return row.Depth
	case "scope":
		return row.Scope
	case "state":
		return row.State
	case "authority_ceiling":
		return row.AuthorityCeiling
	case "claim_ceiling":
		return row.ClaimCeiling
	case "windows":
		return row.Windows
	case "surfaces":
		return row.Surfaces
	case "parity":
		return row.Parity
	case "evidence_count":
		return fmt.Sprintf("%d", row.EvidenceCount)
	case "blocker":
		return row.Blocker
	case "source_refs":
		return row.SourceRefs
	}
	return ""
}

func domainRelationFieldForAir(rel grammar.DomainRelation, field string, air bool) string {
	if air && rel.AIR[field] != "ok" {
		return "▒▒▒"
	}
	switch field {
	case "source":
		return rel.Source
	case "target":
		return rel.Target
	case "relation":
		return rel.Relation
	case "authority_ceiling":
		return rel.AuthorityCeiling
	case "source_refs":
		return rel.SourceRefs
	}
	return ""
}

func lifecycleRowFieldForAir(row grammar.LifecycleRow, field string, air bool) string {
	if air && row.AIR[field] != "ok" {
		return "▒▒▒"
	}
	switch field {
	case "lifecycle_id":
		return row.LifecycleID
	case "label":
		return row.Label
	case "owner":
		return row.Owner
	case "scope":
		return row.Scope
	case "plant":
		return row.Plant
	case "posture":
		return row.Posture
	case "state":
		return row.State
	case "maturity":
		return row.Maturity
	case "adapter_id":
		return row.AdapterID
	case "authority_ceiling":
		return row.AuthorityCeiling
	case "claim_surface":
		return row.ClaimSurface
	case "mutation_surface":
		return row.MutationSurface
	case "dark_policy":
		return row.DarkPolicy
	case "freshness_policy":
		return row.FreshnessPolicy
	case "air_class":
		return row.AIRClass
	case "windows":
		return row.Windows
	case "surfaces":
		return row.Surfaces
	case "commands":
		return row.Commands
	case "receipt_contracts":
		return row.ReceiptContracts
	case "evidence_count":
		return fmt.Sprintf("%d", row.EvidenceCount)
	case "blocker":
		return row.Blocker
	case "next_evidence":
		return row.NextEvidence
	case "source_refs":
		return row.SourceRefs
	}
	return ""
}

func domainStateToken(state string) string {
	switch state {
	case "observed", "active", "source-backed", "source_backed":
		return "grn"
	case "candidate", "support-only", "preview-only", "dark_specified", "future-tenant", "declared-not-modeled":
		return "yel"
	case "missing", "blocked", "dark", "source_missing", "forced_dark":
		return "red"
	}
	return "pri"
}

func domainDepthToken(depth string) string {
	switch depth {
	case "core":
		return "org"
	case "stratum":
		return "blu"
	case "surface":
		return "pri"
	}
	return "mut"
}

type domainTopologyLine struct {
	label string
	value string
	token string
}

func (m Model) domainTopologyLines() []domainTopologyLine {
	if len(m.Domains.Rows) == 0 {
		return nil
	}
	type node struct {
		id        string
		lifecycle string
		terrain   string
		depth     string
		in        int
		out       int
	}
	nodes := map[string]*node{}
	for _, row := range m.Domains.Rows {
		id := domainRowFieldForAir(row, "domain_id", m.AIR)
		if strings.TrimSpace(id) == "" {
			continue
		}
		nodes[id] = &node{
			id:        id,
			lifecycle: domainRowFieldForAir(row, "lifecycle", m.AIR),
			terrain:   domainRowFieldForAir(row, "terrain", m.AIR),
			depth:     domainRowFieldForAir(row, "depth", m.AIR),
		}
	}
	relationKinds := map[string]int{}
	flows := map[string]int{}
	for _, rel := range m.Domains.Relations {
		src := domainRelationFieldForAir(rel, "source", m.AIR)
		tgt := domainRelationFieldForAir(rel, "target", m.AIR)
		kind := domainRelationFieldForAir(rel, "relation", m.AIR)
		if strings.TrimSpace(kind) == "" {
			kind = "related"
		}
		relationKinds[kind]++
		if n := nodes[src]; n != nil {
			n.out++
		}
		if n := nodes[tgt]; n != nil {
			n.in++
		}
		srcLife := "external"
		if n := nodes[src]; n != nil && strings.TrimSpace(n.lifecycle) != "" {
			srcLife = n.lifecycle
		}
		tgtLife := "external"
		if n := nodes[tgt]; n != nil && strings.TrimSpace(n.lifecycle) != "" {
			tgtLife = n.lifecycle
		} else if domainRegistryContains(tgt) {
			tgtLife = "compiled"
		}
		flows[srcLife+"→"+tgtLife]++
	}
	var ranked []*node
	var isolated []string
	for _, n := range nodes {
		ranked = append(ranked, n)
		if n.in+n.out == 0 {
			isolated = append(isolated, n.id)
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		di, dj := ranked[i].in+ranked[i].out, ranked[j].in+ranked[j].out
		if di != dj {
			return di > dj
		}
		return ranked[i].id < ranked[j].id
	})
	sort.Strings(isolated)
	out := []domainTopologyLine{}
	limit := len(ranked)
	if limit > 3 {
		limit = 3
	}
	for i := 0; i < limit; i++ {
		n := ranked[i]
		degree := n.in + n.out
		if degree == 0 {
			continue
		}
		out = append(out, domainTopologyLine{
			label: "central",
			value: fmt.Sprintf("%s degree=%d · in=%d out=%d · lifecycle=%s · terrain=%s/%s", n.id, degree, n.in, n.out, n.lifecycle, n.terrain, n.depth),
			token: "org",
		})
	}
	if len(flows) > 0 {
		out = append(out, domainTopologyLine{label: "flow", value: compactCountMap(flows, 4), token: "blu"})
	}
	if len(relationKinds) > 0 {
		out = append(out, domainTopologyLine{label: "relations", value: compactCountMap(relationKinds, 5), token: "pri"})
	}
	if len(isolated) > 0 {
		out = append(out, domainTopologyLine{label: "isolated", value: strings.Join(isolated, ", "), token: "yel"})
	}
	return out
}

func domainRegistryContains(id string) bool {
	for _, d := range registeredDomains() {
		if d.ID == id {
			return true
		}
	}
	return false
}

func compactCountMap(counts map[string]int, limit int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) > limit {
		keys = keys[:limit]
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", k, counts[k]))
	}
	return strings.Join(parts, " · ")
}

func surfaceKindGlyph(kind string) string {
	switch kind {
	case "door":
		return "◆"
	case "mode":
		return ":"
	case "lens":
		return "◌"
	case "layout":
		return "▣"
	case "projection":
		return "▤"
	case "feedback":
		return "✦"
	case "state":
		return "▒"
	case "selection":
		return "◇"
	case "granularity":
		return "·"
	case "review":
		return "?"
	case "composition":
		return "⟦⟧"
	case "command":
		return "!"
	default:
		return "•"
	}
}

func surfaceFactPrefix(mark, _ string, tok string) string {
	return " " + grammar.C(tok, mark)
}

func (m Model) renderSurfaceCatalog(w int) string {
	var b strings.Builder
	targetFocus := m.targetRowFocusActive()
	writeSectionHeader(&b, w, "SURFACES", surfacesSummary(), "named affordance registry")
	b.WriteString(" " + grammar.C("mut", "transient doors and modes are named affordances, not hidden command lore") + "\n")
	if targetFocus {
		b.WriteString(" " + grammar.C("mut", "[j/k] select surface · [←/→] cycle windows · door surfaces return with [Esc]/[Enter]") + "\n")
	} else {
		b.WriteString(" " + grammar.C("mut", "split reference: source lane owns [j/k]/[Enter]/[y]; [←/→] cycles windows") + "\n")
	}
	doors, modes, layouts, lenses := surfaceKindCounts()
	writeWrappedKV(&b, "shape", fmt.Sprintf("layout:%d · doors:%d modes:%d · lenses:%d · layout surfaces are promoted context, not decoration", layouts, doors, modes, lenses), "org", w)
	b.WriteString("\n")
	for i, surf := range registeredSurfaces() {
		token := "pri"
		switch surf.Kind {
		case "door":
			token = "yel"
		case "command":
			token = "org"
		case "granularity":
			token = "blu"
		}
		selected := targetFocus && i == m.SurfaceFocus
		mark := surfaceKindGlyph(surf.Kind)
		if selected {
			mark = "▶" + mark
		}
		writeSegmentedFactRow(&b, surfaceFactPrefix(mark, surf.ID, token), []string{
			"surface=" + surf.ID,
			"name=" + surf.Name,
			"open=" + surf.Open,
			"exit=" + surf.Exit,
			"scope=" + surf.Scope,
			"kind=" + surf.Kind,
			"AIR=" + surf.AIR,
			"contract=" + surf.Contract,
		}, w, selected)
	}
	b.WriteString("\n")
	b.WriteString(" " + grammar.C("brt", "COVERAGE") + grammar.C("mut", " — current registry plus parity projections; registry rows are not launchable controls") + "\n")
	b.WriteString(" " + grammar.C("2nd", "registered") + grammar.C("mut", "  command/mode · door · lens · layout · projection · review · selection · state · feedback") + "\n")
	b.WriteString(" " + grammar.C("2nd", "parity   ") + grammar.C("mut", "  trainyard · labrack · section-figure · intake observations · in-session coding-tool capabilities") + "\n")
	b.WriteString(" " + grammar.C("2nd", "contract  ") + grammar.C("mut", "  Reins projects and previews; governed COMMAND routes must supply authority, preflight, and receipt") + "\n")
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderDomainCatalog(w int) string {
	var b strings.Builder
	semantic := w < 180
	targetFocus := m.targetRowFocusActive()
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	sourceRows := len(m.Domains.Rows)
	sourceCount := len(m.Domains.Sources)
	lifecycleRows := len(m.Domains.Lifecycles)
	lifecycleSourceCount := len(m.Domains.LifecycleSources)
	missingSources := 0
	if m.Domains.Totals != nil {
		missingSources = m.Domains.Totals["missing_sources"]
	}
	lifecycleMissingSources := 0
	if m.Domains.LifecycleTotals != nil {
		lifecycleMissingSources = m.Domains.LifecycleTotals["missing_sources"]
	}
	writeSectionHeader(&b, w, "DOMAINS", "source-backed lifecycle/domain packs over compiled navigation fallback", "source packs + fallback")
	b.WriteString(" " + grammar.C("mut", "domain lenses group windows/surfaces without assuming one operator's n-DLC taxonomy") + "\n")
	b.WriteString(" " + grammar.C("2nd", "sources") + " " +
		m.readSourceChip("lifecycles", lifecycleRows, m.DomainsDark, m.DomainsSeq) + grammar.C("mut", " · ") +
		m.readSourceChip("domains", sourceRows, m.DomainsDark, m.DomainsSeq) + grammar.C("mut", " · ") +
		yardSourceChip("compiled", len(domainRegistry), false) + grammar.C("mut", " · ") +
		grammar.C(countWarnToken(missingSources+lifecycleMissingSources), fmt.Sprintf("missing:%d", missingSources+lifecycleMissingSources)) + grammar.C("mut", " · ") +
		grammar.C("yel", "read-only projection") + "\n")
	if m.DomainsDark {
		b.WriteString(darkHint(m.DomainsError, m.AIR) + "\n")
		b.WriteString(rule + "\n")
	}
	if lifecycleSourceCount > 0 || lifecycleRows > 0 {
		writeSectionHeader(&b, w, "LIFECYCLE REGISTRY", "authority-aware tenant contracts; domains and windows are projections", "SDLC/RDLC/LDLC/n-DLC")
		meta := []string{}
		if m.Domains.LifecycleAuthority != "" {
			meta = append(meta, "authority="+m.Domains.LifecycleAuthority)
		}
		if m.Domains.LifecycleGeneratedAt != "" {
			meta = append(meta, "generated="+m.Domains.LifecycleGeneratedAt)
		}
		if m.Domains.LifecycleDefaultLens != "" {
			meta = append(meta, "lens="+m.Domains.LifecycleDefaultLens)
		}
		if m.Domains.LifecyclePackageHash != "" {
			meta = append(meta, "hash="+clipRunes(m.Domains.LifecyclePackageHash, 24))
		}
		if len(meta) > 0 {
			writeWrappedKV(&b, "registry", strings.Join(meta, " · "), "mut", w)
		}
		for _, src := range m.Domains.LifecycleSources {
			writeWrappedKV(&b, "source "+domainSourceFieldForAir(src, "id", m.AIR),
				fmt.Sprintf("status=%s · rows=%s · age=%s · authority=%s · detail=%s",
					domainSourceFieldForAir(src, "status", m.AIR),
					domainSourceFieldForAir(src, "count", m.AIR),
					domainSourceFieldForAir(src, "age_bucket", m.AIR),
					domainSourceFieldForAir(src, "authority", m.AIR),
					domainSourceFieldForAir(src, "detail", m.AIR)), sourceStatusToken(src.Status), w)
		}
		if lifecycleRows == 0 {
			writeWrappedKV(&b, "effect", "no source-backed lifecycle contracts loaded; compiled domain/window registries remain navigation hints only", "yel", w)
			writeWrappedKV(&b, "next", "configure REINS_LIFECYCLE_REGISTRIES/lifecycle_registry_paths with tenant lifecycle contracts", "mut", w)
		}
		for _, row := range m.Domains.Lifecycles {
			id := lifecycleRowFieldForAir(row, "lifecycle_id", m.AIR)
			label := lifecycleRowFieldForAir(row, "label", m.AIR)
			labelPart := ""
			if strings.TrimSpace(label) != "" && label != "▒▒▒" {
				labelPart = " · label=" + label
			}
			writeWrappedKV(&b, id,
				fmt.Sprintf("state=%s · posture=%s · maturity=%s · plant=%s · owner=%s · scope=%s · authority=%s%s",
					lifecycleRowFieldForAir(row, "state", m.AIR),
					lifecycleRowFieldForAir(row, "posture", m.AIR),
					lifecycleRowFieldForAir(row, "maturity", m.AIR),
					lifecycleRowFieldForAir(row, "plant", m.AIR),
					lifecycleRowFieldForAir(row, "owner", m.AIR),
					lifecycleRowFieldForAir(row, "scope", m.AIR),
					lifecycleRowFieldForAir(row, "authority_ceiling", m.AIR),
					labelPart), domainStateToken(row.State), w)
			writeWrappedKV(&b, "surface", fmt.Sprintf("adapter=%s · claim=%s · mutate=%s · windows=%s · surfaces=%s",
				lifecycleRowFieldForAir(row, "adapter_id", m.AIR),
				lifecycleRowFieldForAir(row, "claim_surface", m.AIR),
				lifecycleRowFieldForAir(row, "mutation_surface", m.AIR),
				lifecycleRowFieldForAir(row, "windows", m.AIR),
				lifecycleRowFieldForAir(row, "surfaces", m.AIR)), "2nd", w)
			writeWrappedKV(&b, "policy", fmt.Sprintf("dark=%s · freshness=%s · air=%s · commands=%s · receipts=%s · refs=%s",
				lifecycleRowFieldForAir(row, "dark_policy", m.AIR),
				lifecycleRowFieldForAir(row, "freshness_policy", m.AIR),
				lifecycleRowFieldForAir(row, "air_class", m.AIR),
				lifecycleRowFieldForAir(row, "commands", m.AIR),
				lifecycleRowFieldForAir(row, "receipt_contracts", m.AIR),
				lifecycleRowFieldForAir(row, "source_refs", m.AIR)), "mut", w)
			if blocker := lifecycleRowFieldForAir(row, "blocker", m.AIR); strings.TrimSpace(blocker) != "" {
				writeWrappedKV(&b, "blocker", blocker, "yel", w)
			}
			if next := lifecycleRowFieldForAir(row, "next_evidence", m.AIR); strings.TrimSpace(next) != "" {
				writeWrappedKV(&b, "next", next, "pri", w)
			}
		}
		b.WriteString(rule + "\n")
	}
	if sourceCount > 0 {
		writeSectionHeader(&b, w, "PACK SOURCES", "metadata-only; no note bodies or raw source refs", "metadata-only")
		if semantic {
			for _, src := range m.Domains.Sources {
				writeWrappedKV(&b, domainSourceFieldForAir(src, "id", m.AIR),
					fmt.Sprintf("status=%s · count=%s · age=%s · authority=%s · detail=%s",
						domainSourceFieldForAir(src, "status", m.AIR),
						domainSourceFieldForAir(src, "count", m.AIR),
						domainSourceFieldForAir(src, "age_bucket", m.AIR),
						domainSourceFieldForAir(src, "authority", m.AIR),
						domainSourceFieldForAir(src, "detail", m.AIR)), sourceStatusToken(src.Status), w)
			}
		} else {
			b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-24s %-12s %6s %-8s %-20s %s", "SOURCE", "STATUS", "COUNT", "AGE", "AUTHORITY", "DETAIL")) + "\n")
			for _, src := range m.Domains.Sources {
				b.WriteString(" " +
					grammar.C("pri", fmt.Sprintf("%-24s", clipRunes(domainSourceFieldForAir(src, "id", m.AIR), 24))) +
					grammar.C(sourceStatusToken(src.Status), fmt.Sprintf(" %-12s", clipRunes(domainSourceFieldForAir(src, "status", m.AIR), 12))) +
					grammar.C(countWarnToken(src.Count), fmt.Sprintf(" %6s", domainSourceFieldForAir(src, "count", m.AIR))) +
					grammar.C("2nd", fmt.Sprintf(" %-8s", clipRunes(domainSourceFieldForAir(src, "age_bucket", m.AIR), 8))) +
					grammar.C("yel", fmt.Sprintf(" %-20s", clipRunes(domainSourceFieldForAir(src, "authority", m.AIR), 20))) +
					grammar.C("mut", " "+clipRunes(domainSourceFieldForAir(src, "detail", m.AIR), maxVisible(10, w-76))) + "\n")
			}
		}
		b.WriteString(rule + "\n")
	}
	if topo := m.domainTopologyLines(); len(topo) > 0 {
		writeSectionHeader(&b, w, "TOPOLOGY", "centrality, lifecycle flow, and relation kinds before row detail", "domain graph")
		for _, line := range topo {
			writeWrappedKV(&b, line.label, line.value, line.token, w)
		}
		b.WriteString(rule + "\n")
	}
	if sourceRows > 0 {
		writeSectionHeader(&b, w, "SOURCE-BACKED DOMAINS", "operator/tenant lifecycle rows; evidence, not authority", "domain pack rows")
		meta := []string{}
		if m.Domains.Authority != "" {
			meta = append(meta, "authority="+m.Domains.Authority)
		}
		if m.Domains.GeneratedAt != "" {
			meta = append(meta, "generated="+m.Domains.GeneratedAt)
		}
		if m.Domains.DefaultLens != "" {
			meta = append(meta, "lens="+m.Domains.DefaultLens)
		}
		if m.Domains.PackageHash != "" {
			meta = append(meta, "hash="+clipRunes(m.Domains.PackageHash, 24))
		}
		if len(meta) > 0 {
			writeWrappedKV(&b, "package", strings.Join(meta, " · "), "mut", w)
		}
		if semantic {
			for i, row := range m.Domains.Rows {
				label := domainRowFieldForAir(row, "domain_id", m.AIR)
				if targetFocus && i == m.DomainFocus {
					label = "▶ " + label
				}
				writeWrappedKV(&b, label,
					fmt.Sprintf("lifecycle=%s · state=%s · terrain=%s · depth=%s · scope=%s · authority=%s · windows=%s · surfaces=%s",
						domainRowFieldForAir(row, "lifecycle", m.AIR),
						domainRowFieldForAir(row, "state", m.AIR),
						domainRowFieldForAir(row, "terrain", m.AIR),
						domainRowFieldForAir(row, "depth", m.AIR),
						domainRowFieldForAir(row, "scope", m.AIR),
						domainRowFieldForAir(row, "authority_ceiling", m.AIR),
						domainRowFieldForAir(row, "windows", m.AIR),
						domainRowFieldForAir(row, "surfaces", m.AIR)), domainStateToken(row.State), w)
				if blocker := domainRowFieldForAir(row, "blocker", m.AIR); strings.TrimSpace(blocker) != "" {
					writeWrappedKV(&b, "blocker", blocker, "yel", w)
				}
			}
		} else {
			b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-24s %-9s %-10s %-8s %-12s %-18s %s", "DOMAIN", "LIFECYCLE", "TERRAIN", "DEPTH", "STATE", "AUTHORITY", "WINDOWS/SURFACES")) + "\n")
			for i, row := range m.Domains.Rows {
				ws := domainRowFieldForAir(row, "windows", m.AIR)
				ss := domainRowFieldForAir(row, "surfaces", m.AIR)
				if ss != "" {
					ws += " / " + ss
				}
				line := " " +
					grammar.C("brt", fmt.Sprintf("%-24s", clipRunes(domainRowFieldForAir(row, "domain_id", m.AIR), 24))) +
					grammar.C("pri", fmt.Sprintf(" %-9s", clipRunes(domainRowFieldForAir(row, "lifecycle", m.AIR), 9))) +
					grammar.C("2nd", fmt.Sprintf(" %-10s", clipRunes(domainRowFieldForAir(row, "terrain", m.AIR), 10))) +
					grammar.C(domainDepthToken(row.Depth), fmt.Sprintf(" %-8s", clipRunes(domainRowFieldForAir(row, "depth", m.AIR), 8))) +
					grammar.C(domainStateToken(row.State), fmt.Sprintf(" %-12s", clipRunes(domainRowFieldForAir(row, "state", m.AIR), 12))) +
					grammar.C("yel", fmt.Sprintf(" %-18s", clipRunes(domainRowFieldForAir(row, "authority_ceiling", m.AIR), 18))) +
					grammar.C("mut", " "+clipRunes(ws, maxVisible(10, w-90)))
				if targetFocus && i == m.DomainFocus {
					b.WriteString(focusBar(line, w) + "\n")
				} else {
					b.WriteString(line + "\n")
				}
			}
		}
		b.WriteString(rule + "\n")
	}
	if len(m.Domains.Relations) > 0 {
		writeSectionHeader(&b, w, "RELATIONS", "source-backed lifecycle adjacency and extension claims", "domain relations")
		limit := len(m.Domains.Relations)
		if limit > 8 {
			limit = 8
		}
		for _, rel := range m.Domains.Relations[:limit] {
			writeWrappedKV(&b,
				domainRelationFieldForAir(rel, "source", m.AIR),
				fmt.Sprintf("%s → %s · authority=%s · refs=%s",
					domainRelationFieldForAir(rel, "relation", m.AIR),
					domainRelationFieldForAir(rel, "target", m.AIR),
					domainRelationFieldForAir(rel, "authority_ceiling", m.AIR),
					domainRelationFieldForAir(rel, "source_refs", m.AIR)),
				"blu", w)
		}
		if len(m.Domains.Relations) > limit {
			writeWrappedKV(&b, "more", fmt.Sprintf("%d hidden relations", len(m.Domains.Relations)-limit), "mut", w)
		}
		b.WriteString(rule + "\n")
	}
	if sourceRows == 0 {
		writeSectionHeader(&b, w, "SOURCE STATUS", "no source-backed lifecycle/domain rows are active yet", "compiled fallback active")
		writeWrappedKV(&b, "effect", "compiled registry below remains navigation hints only; no operator taxonomy has been loaded", "yel", w)
		writeWrappedKV(&b, "next", "configure REINS_DOMAIN_PACKS/domain_pack_paths with source-backed SDLC/RDLC/n-DLC packs", "mut", w)
		b.WriteString(rule + "\n")
	}
	core, stratum, surface := domainDepthCounts()
	writeSectionHeader(&b, w, "COMPILED FALLBACK", "engine navigation hints; not source authority", "fallback registry")
	writeWrappedKV(&b, "shape",
		fmt.Sprintf("core:%d · stratum:%d · surface:%d · %s", core, stratum, surface, domainsSummary()),
		"mut", w)
	b.WriteString("\n")
	if !semantic {
		b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-19s %-10s %-8s %-9s %-21s %-26s %s", "DOMAIN", "TERRAIN", "DEPTH", "SCOPE", "WINDOWS", "SURFACES", "PARITY")) + "\n")
	}
	sourceRowsActive := len(m.Domains.Rows) > 0
	for i, d := range registeredDomains() {
		token := "pri"
		switch d.Depth {
		case "core":
			token = "org"
		case "stratum":
			token = "blu"
		}
		selected := targetFocus && !sourceRowsActive && i == m.DomainFocus
		if semantic {
			label := d.ID
			if selected {
				label = "▶ " + label
			}
			writeWrappedKV(&b, label, fmt.Sprintf("domain=%s · terrain=%s · depth=%s · scope=%s · windows=%s · surfaces=%s · parity=%s",
				d.ID, d.Terrain, d.Depth, d.Scope, d.Windows, d.Surfaces, d.Parity), token, w)
		} else {
			line := " " +
				grammar.C("brt", fmt.Sprintf("%-19s", clipRunes(d.ID, 19))) +
				grammar.C(token, fmt.Sprintf(" %-10s", clipRunes(d.Terrain, 10))) +
				grammar.C(token, fmt.Sprintf(" %-8s", clipRunes(d.Depth, 8))) +
				grammar.C("mut", fmt.Sprintf(" %-9s", clipRunes(d.Scope, 9))) +
				grammar.C("2nd", fmt.Sprintf(" %-21s", clipRunes(d.Windows, 21))) +
				grammar.C("2nd", fmt.Sprintf(" %-26s ", clipRunes(d.Surfaces, 26))) +
				grammar.C("mut", clipRunes(d.Parity, maxVisible(12, w-102)))
			if selected {
				b.WriteString(focusBar(line, w) + "\n")
			} else {
				b.WriteString(line + "\n")
			}
		}
	}
	b.WriteString("\n")
	b.WriteString(" " + grammar.C("brt", "FUNDAMENTALS") + grammar.C("mut", " — controls for each UI/UX checkpoint") + "\n")
	b.WriteString(" " + grammar.C("2nd", "ontology ") + grammar.C("mut", "domain/lens before screen; windows are projections, not the product ontology") + "\n")
	b.WriteString(" " + grammar.C("2nd", "action   ") + grammar.C("mut", "signifiers at point of action; no hidden command-only screens or modal traps") + "\n")
	b.WriteString(" " + grammar.C("2nd", "state    ") + grammar.C("mut", "freshness, authority, AIR policy, feedback, and receipt visible before trust") + "\n")
	b.WriteString(" " + grammar.C("2nd", "layout   ") + grammar.C("mut", "negative space must carry context, constraints, topology, or next legal moves") + "\n")
	b.WriteString(" " + grammar.C("2nd", "parity   ") + grammar.C("mut", "Trainyard/Labrack/Section-Figure/Logos/tool sessions become registered lenses before controls") + "\n")
	b.WriteString(" " + grammar.C("2nd", "forms    ") + grammar.C("mut", "choose stream, ladder, matrix, graph, rail, tree, heat, or figure-ground by data shape") + "\n")
	b.WriteString(" " + grammar.C("2nd", "motion   ") + grammar.C("mut", "animate only liveness, flow, arrival, decay, progress, and focus continuity") + "\n")
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderLifecycleCatalog(w int) string {
	var b strings.Builder
	semantic := w < 180
	targetFocus := m.targetRowFocusActive()
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	sourceRows := len(m.Domains.Lifecycles)
	sourceCount := len(m.Domains.LifecycleSources)
	missingSources := 0
	if m.Domains.LifecycleTotals != nil {
		missingSources = m.Domains.LifecycleTotals["missing_sources"]
	}
	writeSectionHeader(&b, w, "LIFECYCLES", "source-backed tenant contracts plus compiled navigation fallback", "SDLC/RDLC/LDLC/n-DLC")
	b.WriteString(" " + grammar.C("mut", "tenant-safe lifecycle contracts are tenant data; windows and domains are projections, not universal ontology") + "\n")
	b.WriteString(" " + grammar.C("2nd", "sources") + " " +
		m.readSourceChip("lifecycles", sourceRows, m.DomainsDark, m.DomainsSeq) + grammar.C("mut", " · ") +
		yardSourceChip("compiled", len(registeredLifecycleFallbacks()), false) + grammar.C("mut", " · ") +
		grammar.C(countWarnToken(missingSources), fmt.Sprintf("missing:%d", missingSources)) + grammar.C("mut", " · ") +
		grammar.C("yel", "read-only projection") + "\n")
	if m.DomainsDark {
		b.WriteString(darkHint(m.DomainsError, m.AIR) + "\n")
		b.WriteString(rule + "\n")
	}
	meta := []string{}
	if m.Domains.LifecycleAuthority != "" {
		meta = append(meta, "authority="+m.Domains.LifecycleAuthority)
	}
	if m.Domains.LifecycleGeneratedAt != "" {
		meta = append(meta, "generated="+m.Domains.LifecycleGeneratedAt)
	}
	if m.Domains.LifecycleDefaultLens != "" {
		meta = append(meta, "lens="+m.Domains.LifecycleDefaultLens)
	}
	if m.Domains.LifecyclePackageHash != "" {
		meta = append(meta, "hash="+clipRunes(m.Domains.LifecyclePackageHash, 24))
	}
	if len(meta) > 0 {
		writeWrappedKV(&b, "registry", strings.Join(meta, " · "), "mut", w)
	}
	if sourceCount > 0 {
		writeSectionHeader(&b, w, "PACK SOURCES", "metadata-only source contracts; no raw note bodies", "metadata-only")
		for _, src := range m.Domains.LifecycleSources {
			writeWrappedKV(&b, "source "+domainSourceFieldForAir(src, "id", m.AIR),
				fmt.Sprintf("status=%s · rows=%s · age=%s · authority=%s · detail=%s",
					domainSourceFieldForAir(src, "status", m.AIR),
					domainSourceFieldForAir(src, "count", m.AIR),
					domainSourceFieldForAir(src, "age_bucket", m.AIR),
					domainSourceFieldForAir(src, "authority", m.AIR),
					domainSourceFieldForAir(src, "detail", m.AIR)), sourceStatusToken(src.Status), w)
		}
		b.WriteString(rule + "\n")
	}
	if sourceRows > 0 {
		writeSectionHeader(&b, w, "SOURCE-BACKED LIFECYCLES", "authority ceilings, claim/mutation surfaces, and receipt contracts", "tenant contracts")
		if semantic {
			for i, row := range m.Domains.Lifecycles {
				label := lifecycleRowFieldForAir(row, "lifecycle_id", m.AIR)
				if targetFocus && i == m.LifecycleFocus {
					label = "▶ " + label
				}
				writeWrappedKV(&b, label,
					fmt.Sprintf("state=%s · posture=%s · maturity=%s · plant=%s · scope=%s · authority=%s",
						lifecycleRowFieldForAir(row, "state", m.AIR),
						lifecycleRowFieldForAir(row, "posture", m.AIR),
						lifecycleRowFieldForAir(row, "maturity", m.AIR),
						lifecycleRowFieldForAir(row, "plant", m.AIR),
						lifecycleRowFieldForAir(row, "scope", m.AIR),
						lifecycleRowFieldForAir(row, "authority_ceiling", m.AIR)), domainStateToken(row.State), w)
				writeWrappedKV(&b, "surfaces", fmt.Sprintf("claim=%s · mutate=%s · windows=%s · surfaces=%s",
					lifecycleRowFieldForAir(row, "claim_surface", m.AIR),
					lifecycleRowFieldForAir(row, "mutation_surface", m.AIR),
					lifecycleRowFieldForAir(row, "windows", m.AIR),
					lifecycleRowFieldForAir(row, "surfaces", m.AIR)), "2nd", w)
				writeWrappedKV(&b, "receipts", fmt.Sprintf("commands=%s · contracts=%s · source=%s",
					lifecycleRowFieldForAir(row, "commands", m.AIR),
					lifecycleRowFieldForAir(row, "receipt_contracts", m.AIR),
					lifecycleRowFieldForAir(row, "source_refs", m.AIR)), "yel", w)
				if blocker := lifecycleRowFieldForAir(row, "blocker", m.AIR); strings.TrimSpace(blocker) != "" {
					writeWrappedKV(&b, "blocker", blocker, "yel", w)
				}
				if next := lifecycleRowFieldForAir(row, "next_evidence", m.AIR); strings.TrimSpace(next) != "" {
					writeWrappedKV(&b, "next", next, "pri", w)
				}
			}
		} else {
			b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-18s %-13s %-17s %-20s %-12s %-20s %s", "LIFECYCLE", "STATE", "POSTURE", "PLANT", "SCOPE", "AUTHORITY", "WINDOWS/SURFACES")) + "\n")
			for i, row := range m.Domains.Lifecycles {
				ws := lifecycleRowFieldForAir(row, "windows", m.AIR)
				ss := lifecycleRowFieldForAir(row, "surfaces", m.AIR)
				if ss != "" {
					ws += " / " + ss
				}
				line := " " +
					grammar.C("brt", fmt.Sprintf("%-18s", clipRunes(lifecycleRowFieldForAir(row, "lifecycle_id", m.AIR), 18))) +
					grammar.C(domainStateToken(row.State), fmt.Sprintf(" %-13s", clipRunes(lifecycleRowFieldForAir(row, "state", m.AIR), 13))) +
					grammar.C("org", fmt.Sprintf(" %-17s", clipRunes(lifecycleRowFieldForAir(row, "posture", m.AIR), 17))) +
					grammar.C("2nd", fmt.Sprintf(" %-20s", clipRunes(lifecycleRowFieldForAir(row, "plant", m.AIR), 20))) +
					grammar.C("mut", fmt.Sprintf(" %-12s", clipRunes(lifecycleRowFieldForAir(row, "scope", m.AIR), 12))) +
					grammar.C("yel", fmt.Sprintf(" %-20s", clipRunes(lifecycleRowFieldForAir(row, "authority_ceiling", m.AIR), 20))) +
					grammar.C("mut", " "+clipRunes(ws, maxVisible(10, w-108)))
				if targetFocus && i == m.LifecycleFocus {
					b.WriteString(focusBar(line, w) + "\n")
				} else {
					b.WriteString(line + "\n")
				}
				writeWrappedKV(&b, "contracts", fmt.Sprintf("claim=%s · mutate=%s · commands=%s · receipts=%s · source=%s",
					lifecycleRowFieldForAir(row, "claim_surface", m.AIR),
					lifecycleRowFieldForAir(row, "mutation_surface", m.AIR),
					lifecycleRowFieldForAir(row, "commands", m.AIR),
					lifecycleRowFieldForAir(row, "receipt_contracts", m.AIR),
					lifecycleRowFieldForAir(row, "source_refs", m.AIR)), "2nd", w)
			}
		}
		b.WriteString(rule + "\n")
	} else {
		writeSectionHeader(&b, w, "SOURCE STATUS", "no source-backed lifecycle rows are active yet", "compiled fallback active")
		writeWrappedKV(&b, "effect", "compiled lifecycle labels below remain navigation hints only; no tenant lifecycle contract has authority", "yel", w)
		writeWrappedKV(&b, "next", "configure REINS_LIFECYCLE_REGISTRIES/lifecycle_registry_paths with source-backed SDLC/RDLC/LDLC/n-DLC rows", "mut", w)
		b.WriteString(rule + "\n")
	}
	writeSectionHeader(&b, w, "COMPILED FALLBACK", "window lifecycle labels; not tenant authority", "navigation hints")
	for i, row := range registeredLifecycleFallbacks() {
		label := row.ID
		if targetFocus && sourceRows == 0 && i == m.LifecycleFocus {
			label = "▶ " + label
		}
		writeWrappedKV(&b, label, fmt.Sprintf("scope=%s · windows=%s · %s", row.Scope, row.Windows, row.Contract), "2nd", w)
	}
	b.WriteString(rule + "\n")
	writeSectionHeader(&b, w, "EXTENSIBILITY", "one operator's lifecycle taxonomy is not the product ontology", "tenant-safe")
	writeWrappedKV(&b, "contract", "source packs define lifecycle tenants; compiled windows only project read models", "org", w)
	writeWrappedKV(&b, "LDLC", "life-management remains a tenant lifecycle with privacy/consent receipts before mutation", "yel", w)
	writeWrappedKV(&b, "n-DLC", "future lifecycles must declare owner, scope, authority ceiling, AIR class, surfaces, commands, and receipts", "pri", w)
	writeWrappedKV(&b, "boundary", "Reins may inspect and contextualize; governed mutation still requires explicit route authority and receipts", "mut", w)
	return strings.TrimRight(b.String(), "\n")
}

func intentTargetFactPrefix(mark, label, tok string) string {
	return " " + grammar.C(tok, mark+" ") + grammar.C(tok, fmt.Sprintf("%-12s", clipRunes(label, 12)))
}

// intentTargetsBody is the PRIMARY pane of the algebra-composed intent page (Inc 3 TRANSFORM): the
// review banner + the governed-route TARGETS list with the IntentFocus cursor. The selected target's
// full route review is the SECONDARY (renderSelectedIntentReview). Self-anchored — [j/k] moves the
// target natively (the page is no longer session-frozen).
func (m Model) intentTargetsBody(w, h int) string {
	target := strings.TrimSpace(m.IntentTarget)
	var b strings.Builder
	b.WriteString(" " + grammar.C("brt", "INTENT REVIEW") + grammar.C("mut", "  review-before-run, no effect emitted") + "\n")
	b.WriteString(" " + grammar.C("yel", "governed COMMAND route required") + grammar.C("mut", " · no dispatch · no claim · no close from this pane") + "\n")
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	b.WriteString(rule + "\n")
	if selected, ok := m.selectedIntentArg(); ok {
		writeWrappedKV(&b, "cursor", fmt.Sprintf("%d/%d · %s · [j/k] target · [Enter] preview selected target", m.IntentFocus+1, len(lookupIntentArgs()), selected.Label), "yel", w)
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "targets") + "\n")
	for i, a := range lookupIntentArgs() {
		mark, tok := " ", "mut"
		if i == m.IntentFocus {
			mark, tok = "▶", "yel"
		} else if a.Label == target {
			mark, tok = "◆", "yel"
		}
		writeSegmentedFactRow(&b, intentTargetFactPrefix(mark, a.Label, tok), []string{
			"target=" + a.Label,
			"detail=" + a.Detail,
		}, w, i == m.IntentFocus)
	}
	return b.String()
}

// renderSelectedIntentReview is the algebra SECONDARY for the intent page: the selected target's route
// review ladder + subject binding + contract. The subject is the AIR-safe subject captured before the
// page switch (m.intentReviewSubject()); no live session anchor — the page is self-anchored.
func (m Model) renderSelectedIntentReview(w int) string {
	target := strings.TrimSpace(m.IntentTarget)
	subject := m.intentReviewSubject()
	v, _ := lookupVerb("intent")
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	line := func(label, value, tok string) { writeWrappedKV(&b, label, value, tok, w) }
	writeSectionHeader(&b, w, "ROUTE REVIEW", "review-before-run; no effect emitted from this pane", firstNonEmpty(target, "choose a target"))
	if target == "" {
		line("target", "choose one in the list", "yel")
	} else if intentArg(target) == nil {
		line("target", "unknown: "+target, "red")
	} else {
		line("target", target, "yel")
	}
	line("subject", subject, "pri")
	line("authority", v.authority, "org")
	line("preflight", v.preflight, "2nd")
	line("receipt", v.receipt, "2nd")
	line("effect", "none emitted", "grn")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("brt", "ROUTE PREVIEW LADDER") + grammar.C("mut", " — subject binding before governed effect") + "\n")
	for _, row := range []contextRow{
		{"source", subject, "pri"},
		{"target", firstNonEmpty(target, "choose one in the list"), "yel"},
		{"authority", v.authority, "org"},
		{"preflight", v.preflight, "2nd"},
		{"receipt", v.receipt, "2nd"},
		{"effect", "none emitted from Reins until route receipt exists", "grn"},
	} {
		b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-10s ", row.label)) +
			grammar.C(row.token, clipRunes(row.value, maxVisible(12, w-14))) + "\n")
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("brt", "SUBJECT BINDING") + grammar.C("mut", " — templates make selection reusable without copy-paste") + "\n")
	for _, row := range []contextRow{
		{"{{focus}}", "current focused row identity", "yel"},
		{"{{sel.*}}", "selected/status/missing field through AIR", "yel"},
		{"{{ring.0}}", "last yanked AIR-safe value", "yel"},
		{"handoff", "future COMMAND route must attach evidence refs", "org"},
	} {
		b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-12s ", row.label)) +
			grammar.C(row.token, clipRunes(row.value, maxVisible(12, w-16))) + "\n")
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "contract") + "\n")
	b.WriteString(" " + grammar.C("mut", "This screen reviews target, subject, authority, preflight, and receipt only.") + "\n")
	b.WriteString(" " + grammar.C("mut", "A future COMMAND route must supply mutation surface, route evidence, and receipt refs.") + "\n")
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderIntentReview(w int) string {
	targetFocus := m.targetRowFocusActive()
	target := strings.TrimSpace(m.IntentTarget)
	subject := m.intentReviewSubject()
	v, _ := lookupVerb("intent")
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	line := func(label, value, tok string) {
		writeWrappedKV(&b, label, value, tok, w)
	}
	b.WriteString(" " + grammar.C("brt", "INTENT REVIEW") + grammar.C("mut", "  review-before-run, no effect emitted") + "\n")
	b.WriteString(" " + grammar.C("yel", "governed COMMAND route required") + grammar.C("mut", " · no dispatch · no claim · no close from this pane") + "\n")
	b.WriteString(rule + "\n")
	if target == "" {
		line("target", "choose one below", "yel")
	} else if intentArg(target) == nil {
		line("target", "unknown: "+target, "red")
	} else {
		line("target", target, "yel")
	}
	line("subject", subject, "pri")
	line("authority", v.authority, "org")
	line("preflight", v.preflight, "2nd")
	line("receipt", v.receipt, "2nd")
	line("effect", "none emitted", "grn")
	if selected, ok := m.selectedIntentArg(); ok {
		cursor := fmt.Sprintf("%d/%d · %s · [Enter] preview selected target", m.IntentFocus+1, len(lookupIntentArgs()), selected.Label)
		if !targetFocus {
			cursor = fmt.Sprintf("%d/%d · %s · split: source owns [j/k]/[Enter]; use :intent <target> or [|] unsplit", m.IntentFocus+1, len(lookupIntentArgs()), selected.Label)
		}
		line("cursor", cursor, "yel")
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "targets") + "\n")
	for i, a := range lookupIntentArgs() {
		mark := " "
		tok := "mut"
		if targetFocus && i == m.IntentFocus {
			mark, tok = "▶", "yel"
		} else if a.Label == target {
			mark, tok = "◆", "yel"
		}
		writeSegmentedFactRow(&b, intentTargetFactPrefix(mark, a.Label, tok), []string{
			"target=" + a.Label,
			"detail=" + a.Detail,
		}, w, targetFocus && i == m.IntentFocus)
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("brt", "ROUTE PREVIEW LADDER") + grammar.C("mut", " — subject binding before governed effect") + "\n")
	for _, row := range []contextRow{
		{"source", subject, "pri"},
		{"target", firstNonEmpty(target, "choose one above"), "yel"},
		{"authority", v.authority, "org"},
		{"preflight", v.preflight, "2nd"},
		{"receipt", v.receipt, "2nd"},
		{"effect", "none emitted from Reins until route receipt exists", "grn"},
	} {
		b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-10s ", row.label)) +
			grammar.C(row.token, clipRunes(row.value, maxVisible(12, w-14))) + "\n")
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("brt", "SUBJECT BINDING") + grammar.C("mut", " — templates make selection reusable without copy-paste") + "\n")
	for _, row := range []contextRow{
		{"{{focus}}", "current focused row identity", "yel"},
		{"{{sel.*}}", "selected/status/missing field through AIR", "yel"},
		{"{{ring.0}}", "last yanked AIR-safe value", "yel"},
		{"handoff", "future COMMAND route must attach evidence refs", "org"},
	} {
		b.WriteString(" " + grammar.C("2nd", fmt.Sprintf("%-12s ", row.label)) +
			grammar.C(row.token, clipRunes(row.value, maxVisible(12, w-16))) + "\n")
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "contract") + "\n")
	b.WriteString(" " + grammar.C("mut", "This screen reviews target, subject, authority, preflight, and receipt only.") + "\n")
	b.WriteString(" " + grammar.C("mut", "A future COMMAND route must supply mutation surface, route evidence, and receipt refs.") + "\n")
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) intentReviewSubject() string {
	if m.sessionSplit() {
		if s, ok := m.FocusedSession(); ok {
			role := sessionFieldValueForAir(s, "role", m.AIR)
			if strings.TrimSpace(role) == "" {
				role = "·"
			}
			return "session " + role
		}
		return "session source none"
	}
	subject := strings.TrimSpace(m.IntentSubject)
	if subject == "" {
		return "selection none"
	}
	return subject
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func intentArg(label string) *Candidate {
	for _, a := range lookupIntentArgs() {
		if a.Label == label {
			return &a
		}
	}
	return nil
}

func clipRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

func wrapRunes(s string, n int) []string {
	if n <= 0 {
		return []string{""}
	}
	r := []rune(s)
	if len(r) == 0 {
		return []string{""}
	}
	var out []string
	for len(r) > n {
		cut := n
		for i := n; i > n/2; i-- {
			if r[i-1] == ' ' || r[i-1] == '/' || r[i-1] == '-' || r[i-1] == '_' || r[i-1] == '·' {
				cut = i
				break
			}
		}
		part := strings.TrimSpace(string(r[:cut]))
		if part == "" {
			part = string(r[:n])
			cut = n
		}
		out = append(out, part)
		r = r[cut:]
		for len(r) > 0 && r[0] == ' ' {
			r = r[1:]
		}
	}
	out = append(out, strings.TrimSpace(string(r)))
	return out
}

func writeWrappedKV(b *strings.Builder, label, value, tok string, w int) {
	if strings.TrimSpace(value) == "" {
		value, tok = "·", "mut"
	}
	maxLabelW := 12
	switch {
	case w >= 132:
		maxLabelW = 18
	case w >= 96:
		maxLabelW = 16
	case w >= 72:
		maxLabelW = 14
	}
	labelW := 12
	labelLen := ansi.StringWidth(label)
	if labelLen > 12 {
		labelW = labelLen + 1
	}
	if labelW > maxLabelW && w-labelW-3 < 8 {
		labelW = maxLabelW
	}
	valueW := maxVisible(8, w-labelW-3)
	parts := wrapRunes(value, valueW)
	for i, part := range parts {
		lab := ""
		if i == 0 {
			lab = clipRunes(label, labelW-1)
		}
		b.WriteString(" " + grammar.C("mut", fmt.Sprintf("%-*s", labelW, lab)) + grammar.C(tok, part) + "\n")
	}
}

func writeSegmentedFactRow(b *strings.Builder, prefix string, facts []string, w int, focused bool) {
	writeSegmentedFactRowToken(b, prefix, facts, w, focused, "mut")
}

func writeSegmentedFactRowToken(b *strings.Builder, prefix string, facts []string, w int, focused bool, factTok string) {
	for _, line := range wrapSegmentedFactSegmentsToken(prefix, facts, w, factTok) {
		if focused {
			b.WriteString(focusBar(line, w) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
	}
}

func wrapSegmentedFactSegments(prefix string, facts []string, w int) []string {
	return wrapSegmentedFactSegmentsToken(prefix, facts, w, "mut")
}

func wrapSegmentedFactSegmentsToken(prefix string, facts []string, w int, factTok string) []string {
	lines := make([]string, 0, 2)
	linePrefix := prefix
	available := maxVisible(8, w-ansi.StringWidth(linePrefix)-1)
	current := ""
	flush := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		for _, part := range wrapRunes(text, available) {
			if strings.TrimSpace(part) == "" {
				continue
			}
			lines = append(lines, linePrefix+grammar.C(factTok, " "+part))
			linePrefix = segmentedFactContinuationPrefix()
			available = maxVisible(8, w-ansi.StringWidth(linePrefix)-1)
		}
	}
	for _, fact := range facts {
		fact = strings.TrimSpace(fact)
		if fact == "" {
			continue
		}
		sep := ""
		if current != "" {
			sep = " · "
		}
		candidate := current + sep + fact
		if current == "" || ansi.StringWidth(candidate) <= available {
			current = candidate
			continue
		}
		flush(current)
		current = ""
		if ansi.StringWidth(fact) <= available {
			current = fact
		} else {
			flush(fact)
		}
	}
	flush(current)
	if len(lines) == 0 {
		return []string{prefix}
	}
	return lines
}

func writeContextFactRow(b *strings.Builder, label, value, tok string, w int) {
	if strings.TrimSpace(value) == "" {
		value, tok = "·", "mut"
	}
	writeSegmentedFactRowToken(b, contextFactPrefix(label), []string{value}, w, false, tok)
}

func contextFactPrefix(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		label = "·"
	}
	if ansi.StringWidth(label) <= 12 {
		return " " + grammar.C("mut", fmt.Sprintf("%-11s", label))
	}
	return " " + grammar.C("mut", label)
}

func segmentedFactContinuationPrefix() string {
	return " " + grammar.C("mut", "  ↳")
}

func wrappedKVLines(label, value, tok string, w int) []string {
	var b strings.Builder
	writeWrappedKV(&b, label, value, tok, w)
	out := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(out) == 1 && out[0] == "" {
		return nil
	}
	return out
}

func writeWrappedBullet(b *strings.Builder, value, tok string, w int) {
	if strings.TrimSpace(value) == "" {
		return
	}
	parts := wrapRunes(value, maxVisible(8, w-4))
	for i, part := range parts {
		mark := "· "
		if i > 0 {
			mark = "  "
		}
		b.WriteString(" " + grammar.C("mut", mark) + grammar.C(tok, part) + "\n")
	}
}

func pageLabel(page int) string {
	switch page {
	case PageEvents:
		return "PageEvents"
	case PageTasks:
		return "PageTasks"
	case PageSessions:
		return "PageSessions"
	case PageTraces:
		return "PageTraces"
	case PageSessionTurns:
		return "PageSessionTurns"
	case PageDispatch:
		return "PageDispatch"
	case PageYard:
		return "PageYard"
	case PageReadiness:
		return "PageReadiness"
	case PageIntake:
		return "PageIntake"
	case PageCaps:
		return "PageCaps"
	case PageDynamics:
		return "PageDynamics"
	case PageLoops:
		return "PageLoops"
	case PageAxes:
		return "PageAxes"
	case PageIdentity:
		return "PageIdentity"
	case PageRelational:
		return "PageRelational"
	case PageEpistemics:
		return "PageEpistemics"
	case PageHelp:
		return "PageHelp"
	case PageLegend:
		return "PageLegend"
	case PageCommands:
		return "PageCommands"
	case PageWindows:
		return "PageWindows"
	case PageIntent:
		return "PageIntent"
	case PageSurfaces:
		return "PageSurfaces"
	case PageDomains:
		return "PageDomains"
	case PageLifecycles:
		return "PageLifecycles"
	}
	return fmt.Sprintf("Page(%d)", page)
}

func (m Model) referenceLines(w int) []string {
	s := strings.TrimRight(m.referenceContent(w), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func (m Model) referenceLayoutWidths(w int) (leftW, rightW int, split bool) {
	if w < 150 || m.Page == PageDynamics || m.Page == PageEpistemics {
		return w, 0, false
	}
	rightW = clamp(w/3, 50, 92)
	leftW = w - rightW - 1
	if leftW < 96 {
		return w, 0, false
	}
	return leftW, rightW, true
}

func dynamicsAlgebraPrimaryPaneDims(w, h int) (int, int) {
	if w < 40 || h < 12 {
		w, h = 120, 40
	}
	bodyH := h - 7
	if bodyH < 1 {
		bodyH = 1
	}
	if w < 3 {
		return w, bodyH
	}
	inner := w - 1 // composePage/layout.Render reserve one connector column.
	primaryW := clamp(int(float64(inner)*0.62), 1, inner-1)
	primaryH := bodyH
	if bodyH > 1 {
		primaryH = bodyH - 1 // connector relation header consumes the first body row.
	}
	return primaryW, primaryH
}

func (m Model) referenceBody(w, h int) string {
	leftW, rightW, split := m.referenceLayoutWidths(w)
	lines := m.referenceLines(leftW)
	if len(lines) == 0 {
		return ""
	}
	max := len(lines) - h
	if max < 0 {
		max = 0
	}
	off := clamp(m.RefScroll, 0, max)
	end := off + h
	if end > len(lines) {
		end = len(lines)
	}
	leftText := strings.Join(lines[off:end], "\n")
	if !split {
		slackRows := h - len(strings.Split(strings.TrimRight(leftText, "\n"), "\n"))
		if slackRows >= 3 {
			leftText = strings.TrimRight(leftText, "\n") + "\n" + strings.Join(m.slackRowsForPage(leftW, slackRows, slackSlotReferenceMain), "\n")
		}
		return strings.TrimRight(leftText, "\n")
	}
	left := fitBlockWithSlackFn(leftText, leftW, h, func(rows int) []string {
		return m.slackRowsForPage(leftW, rows, slackSlotReferenceMain)
	})
	right := fitBlockWithOverflowAndSlackFn(m.referenceContextPane(rightW), rightW, h, "context", func(rows int) []string {
		return m.slackRowsForPage(rightW, rows, slackSlotWideContext)
	})
	div := grammar.C("border", "│")
	out := make([]string, 0, h)
	for i := 0; i < h; i++ {
		out = append(out, left[i]+div+right[i])
	}
	return strings.Join(out, "\n")
}

func (m Model) referenceScrollMax() int {
	w, h := m.Width, m.Height
	if w < 40 || h < 12 {
		w, h = 120, 40
	}
	if m.Page == PageDynamics {
		primaryW, primaryH := dynamicsAlgebraPrimaryPaneDims(w, h)
		max := len(m.dynamicsMapDocumentLines(primaryW, primaryH)) - primaryH
		if max < 0 {
			return 0
		}
		return max
	}
	leftW, _, _ := m.referenceLayoutWidths(w)
	if m.sessionSplit() {
		if _, rightW, ok := splitContextWidths(w); ok {
			leftW = rightW
		}
	}
	midH := h - 7
	if midH < 1 {
		midH = 1
	}
	if m.sessionSplit() && m.Page == PageCaps {
		max := len(m.capabilitySplitPaneLines(leftW)) - m.splitContextBodyRows(midH)
		if max < 0 {
			return 0
		}
		return max
	}
	lines := m.referenceLines(leftW)
	if m.sessionSplit() {
		if pinned, ok := m.splitPinnedContextBlock(leftW); ok {
			midH -= len(strings.Split(strings.TrimRight(pinned, "\n"), "\n")) + 1
			if midH < 1 {
				midH = 1
			}
			bodyModel := m
			bodyModel.SuppressSplitPinned = true
			lines = bodyModel.referenceLines(leftW)
		}
	}
	max := len(lines) - midH
	if max < 0 {
		return 0
	}
	return max
}

func (m Model) scrollReference(delta int) Model {
	max := m.referenceScrollMax()
	if max == 0 {
		m.RefScroll = 0
		m.Status = "scroll: page fits"
		return m
	}
	m.RefScroll = clamp(m.RefScroll+delta, 0, max)
	switch {
	case m.RefScroll == 0:
		m.Status = "scroll: top"
	case m.RefScroll == max:
		m.Status = "scroll: bottom"
	default:
		m.Status = fmt.Sprintf("scroll %d/%d", m.RefScroll+1, max+1)
	}
	return m
}

func (m Model) referenceScrollLabel() string {
	w, h := m.Width, m.Height
	if w < 40 || h < 12 {
		w, h = 120, 40
	}
	if m.Page == PageDynamics {
		primaryW, primaryH := dynamicsAlgebraPrimaryPaneDims(w, h)
		lines := len(m.dynamicsMapDocumentLines(primaryW, primaryH))
		if lines == 0 {
			return "0/0"
		}
		max := m.referenceScrollMax()
		off := clamp(m.RefScroll, 0, max)
		end := off + primaryH
		if end > lines {
			end = lines
		}
		return fmt.Sprintf("%d-%d/%d", off+1, end, lines)
	}
	leftW, _, _ := m.referenceLayoutWidths(w)
	if m.sessionSplit() {
		if _, rightW, ok := splitContextWidths(w); ok {
			leftW = rightW
		}
	}
	lines := len(m.referenceLines(leftW))
	if lines == 0 {
		return "0/0"
	}
	max := m.referenceScrollMax()
	off := clamp(m.RefScroll, 0, max)
	visible := h - 7
	if visible < 1 {
		visible = 1
	}
	linesForCount := m.referenceLines(leftW)
	if m.sessionSplit() && m.Page == PageCaps {
		visible = m.splitContextBodyRows(visible)
		linesForCount = m.capabilitySplitPaneLines(leftW)
		lines = len(linesForCount)
		end := off + visible
		if end > lines {
			end = lines
		}
		return fmt.Sprintf("%d-%d/%d", off+1, end, lines)
	}
	if m.sessionSplit() {
		if pinned, ok := m.splitPinnedContextBlock(leftW); ok {
			visible -= len(strings.Split(strings.TrimRight(pinned, "\n"), "\n")) + 1
			if visible < 1 {
				visible = 1
			}
			bodyModel := m
			bodyModel.SuppressSplitPinned = true
			linesForCount = bodyModel.referenceLines(leftW)
		}
	}
	lines = len(linesForCount)
	end := off + visible
	if end > lines {
		end = lines
	}
	return fmt.Sprintf("%d-%d/%d", off+1, end, lines)
}

func (m Model) referenceContextPane(w int) string {
	name, count, dark := m.pageMeta()
	wnd, hasWindow := windowForPage(m.Page)
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	var b strings.Builder
	line := func(label, value, tok string) { writeContextFactRow(&b, label, value, tok, w) }
	b.WriteString(" " + grammar.C("brt", "CONTEXT") + grammar.C("mut", "  page contract, source state, composition") + "\n")
	b.WriteString(rule + "\n")
	line("window", ":"+name, "pri")
	if hasWindow {
		line("scope", wnd.Scope+" / "+wnd.Lifecycle, "2nd")
		line("kind", wnd.Kind, "2nd")
	}
	sigTok := "pri"
	if dark {
		sigTok = "red"
	}
	line("signal", fmt.Sprintf("%d", count), sigTok)
	b.WriteString(" " + grammar.C("mut", fmt.Sprintf("%-12s", "read")) + m.readReceipt() + "\n")
	b.WriteString(rule + "\n")
	if rows := m.referencePageRailRows(); len(rows) > 0 {
		b.WriteString(" " + grammar.C("2nd", "page shape") + "\n")
		for _, row := range rows {
			line(row.label, row.value, row.token)
		}
		b.WriteString(rule + "\n")
	}
	b.WriteString(" " + grammar.C("2nd", m.referenceContextHeading()) + "\n")
	for _, row := range m.referenceContextRows() {
		line(row.label, row.value, row.token)
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "signals") + "\n")
	for _, row := range m.referenceSignalRows() {
		line(row.label, row.value, row.token)
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "selection context") + "\n")
	line("{{focus}}", "current row identity", "yel")
	line("{{sel.*}}", "field/value refs", "yel")
	line("{{ring.0}}", "AIR-safe yank replay", "yel")
	line("split", "session │ context surface", "org")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "legal next") + "\n")
	for _, row := range m.referenceLegalRows() {
		line(row.label, row.value, row.token)
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "fundamentals") + "\n")
	for _, row := range referenceFundamentalRows() {
		line(row.label, row.value, row.token)
	}
	return strings.TrimRight(b.String(), "\n")
}

type contextRow struct {
	label, value, token string
}

func (m Model) referenceContextHeading() string {
	switch m.Page {
	case PageYard:
		return "trainyard context"
	case PageReadiness:
		return "readiness context"
	case PageIntake:
		return "observation context"
	case PageCaps:
		if m.sessionSplit() && !m.targetRowFocusActive() {
			return "lane capability fit"
		}
		if row, ok := m.FocusedCapabilityRow(); ok {
			group := row.Group
			if group == "" {
				group = capabilityStatusSourceGroup(row)
			}
			switch group {
			case "class":
				return "capability class"
			case "surface":
				return "surface capability"
			case "core":
				return "route contract"
			case "score":
				return "score dimension"
			}
		}
		return "capability ontology"
	case PageDynamics:
		return "map context"
	case PageLoops:
		return "feedback loops"
	case PageAxes:
		return "case-role axes"
	case PageIdentity:
		return "identity roster"
	case PageRelational:
		return "consent posture"
	case PageEpistemics:
		return "epistemic context"
	case PageHelp:
		return "help context"
	case PageLegend:
		return "legend context"
	case PageCommands:
		return "command grammar"
	case PageWindows:
		return "window topology"
	case PageIntent:
		return "intent target"
	case PageSurfaces:
		return "surface use"
	case PageDomains:
		return "domain lens"
	case PageLifecycles:
		return "lifecycle contracts"
	}
	return "selection context"
}

func capabilityRefLabelSummary(labels []string) string {
	parts := make([]string, 0, min(len(labels), 4))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		parts = append(parts, label)
		if len(parts) == 4 {
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	out := strings.Join(parts, " · ")
	if len(labels) > len(parts) {
		out += fmt.Sprintf(" · +%d", len(labels)-len(parts))
	}
	return out
}

func (m Model) referencePageRailRows() []contextRow {
	switch m.Page {
	case PageWindows:
		return []contextRow{
			{"first read", "which screens exist + why", "pri"},
			{"split pairs", splitPairSummary(), "org"},
			{"cursor", "▶ linked · ◆ source anchor", "yel"},
			{"rule", "relationships, not two-up", "2nd"},
		}
	case PageSurfaces:
		doors, modes, layouts, lenses := surfaceKindCounts()
		return []contextRow{
			{"layout", fmt.Sprintf("%d layout surfaces", layouts), "org"},
			{"doors", fmt.Sprintf("%d doors · %d modes", doors, modes), "yel"},
			{"lenses", fmt.Sprintf("%d lenses", lenses), "pri"},
			{"contract", "all named, exitable", "grn"},
		}
	case PageDomains:
		core, stratum, surface := domainDepthCounts()
		sourceRows := len(m.Domains.Rows)
		sourceState := "pack missing"
		sourceTok := "yel"
		if sourceRows > 0 {
			sourceState = fmt.Sprintf("%d source rows", sourceRows)
			sourceTok = "grn"
		} else if m.DomainsDark {
			sourceState = "read dark"
			sourceTok = "red"
		}
		return []contextRow{
			{"ontology", sourceState, sourceTok},
			{"depths", fmt.Sprintf("core:%d stratum:%d surface:%d", core, stratum, surface), "pri"},
			{"n-DLC", "pack-extensible", "yel"},
			{"risk", "SDLC renderers remain specific", "2nd"},
		}
	case PageEpistemics:
		rows := m.epistemicRows()
		gaps := 0
		for _, row := range rows {
			if row.Token == "red" {
				gaps++
			}
		}
		return []contextRow{
			{"rows", fmt.Sprintf("%d derived", len(rows)), "pri"},
			{"gaps", fmt.Sprintf("%d missing/dark/stale", gaps), countWarnToken(gaps)},
			{"authority", "evidence, not dispatch", "yel"},
			{"raw", "no bodies/transcripts", "grn"},
		}
	case PageHelp:
		// Inc-5: reference pages render catalog│context via referenceBody with ONE catalog scroll
		// ([j/k] doc). The legacy second "[J/K] ctx" scroll belonged to the abolished session-frozen
		// reference split; session context scroll now lives only on the session-anchored pages' footer.
		return []contextRow{
			{"split marks", "▶ linked · ◆ anchor", "org"},
			{"scroll", "[j/k] doc", "yel"},
			{"discover", "no buried screens", "grn"},
		}
	case PageLegend:
		return []contextRow{
			{"layout", "split:ctx · split:wide", "org"},
			{"cursor", "▶ source · ◆ anchor", "yel"},
			{"overflow", "… means hidden rows", "2nd"},
		}
	}
	return nil
}

func (m Model) referenceContextRows() []contextRow {
	switch m.Page {
	case PageYard:
		fleet := m.yardFleetCounts()
		return []contextRow{
			{"form", "ladder + rail + fleet + gates", "pri"},
			{"authority", "read-only projection", "yel"},
			{"fleet", fmt.Sprintf("%d live · %d stale/off", fleet.live, fleet.stale+fleet.off), countWarnToken(fleet.stale + fleet.off)},
			{"next", "drilldown + animation parity", "2nd"},
		}
	case PageReadiness:
		visibleBlocked, hiddenBlocked := m.yardBlockedIndices()
		fleet := m.yardFleetCounts()
		return []contextRow{
			{"contract", "gate stack projection", "pri"},
			{"holds", fmt.Sprintf("%d visible · %d hidden", len(visibleBlocked), hiddenBlocked), countWarnToken(len(visibleBlocked) + hiddenBlocked)},
			{"lanes", fmt.Sprintf("%d claim · %d stale/off", fleet.claim, fleet.stale+fleet.off), countWarnToken(fleet.stale + fleet.off)},
			{"authority", "receipt required", "yel"},
		}
	case PageIntake:
		return []contextRow{
			{"contract", "intake snapshot projection", "pri"},
			{"attention", fmt.Sprintf("%d demand units", m.intakeAttentionTotal()), countWarnToken(m.intakeAttentionTotal())},
			{"sources", fmt.Sprintf("%d snapshots", len(m.Intake.Sources)), countToken(len(m.Intake.Sources))},
			{"authority", "no drain/dismiss/write", "yel"},
		}
	case PageCaps:
		total, gaps := m.capabilityTitleCounts()
		if m.sessionSplit() && !m.targetRowFocusActive() {
			if s, ok := m.FocusedSession(); ok {
				fit, fitTok := m.selectedLaneCapabilityPosture(s)
				return []contextRow{
					{"lane", sessionFieldValueForAir(s, "role", m.AIR), airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR)},
					{"platform", sessionFieldValueForAir(s, "platform", m.AIR), "2nd"},
					{"readiness", sessionFieldValueForAir(s, "readiness", m.AIR), airHue(readinessPaneToken(s.Readiness), s.AIR, "readiness", m.AIR)},
					{"fit", fit, fitTok},
					{"claimed", sessionFieldValueForAir(s, "claimed_task", m.AIR), "mut"},
					{"authority", "source lane drives capability fit", "yel"},
					{"capability", fmt.Sprintf("%d rows · %d gaps", total, gaps), countWarnToken(gaps)},
				}
			}
		}
		if capRow, ok := m.FocusedCapabilityRow(); ok {
			detailLabel, detail, detailTok := capabilityDetailContext(capRow.Status, capRow.Missing)
			rows := []contextRow{
				{"capability", capRow.Name, "pri"},
				{"status", capRow.Status, capRow.Token},
				{"class", capRow.Class, "org"},
				{"authority", capRow.Authority, "2nd"},
				{"evidence", capRow.Evidence, countWarnToken(capRow.BlockedCount)},
				{"family", capRow.Family, "yel"},
				{"spend", capRow.Spend, "2nd"},
				{"egress", capRow.Egress, "2nd"},
				{"receipt", capRow.Receipt, "mut"},
				{"source", capRow.SourceRefs, "mut"},
			}
			if refs := capabilityRefLabelSummary(capRow.SourceRefLabels); refs != "" {
				rows = append(rows, contextRow{"refs", refs, "mut"})
			}
			rows = append(rows, contextRow{detailLabel, detail, detailTok})
			return rows
		}
		return []contextRow{
			{"ontology", "capability-first", "org"},
			{"status", fmt.Sprintf("%d capabilities · %d gaps", total, gaps), countWarnToken(gaps)},
			{"HKP", "support-only ceiling", "yel"},
			{"platforms", "evidence, not ontology", "2nd"},
		}
	case PageHelp:
		return []contextRow{
			{"contract", "self-documenting cockpit", "pri"},
			{"pages", fmt.Sprintf("%d registered", len(windowRegistry)), "pri"},
			{"screens", "cycleable, not buried", "grn"},
			{"templates", "{{sel.*}} visible refs", "yel"},
		}
	case PageLegend:
		return []contextRow{
			{"contract", "decode every mark", "pri"},
			{"channels", "hue · brightness · family", "yel"},
			{"AIR", "structure remains", "grn"},
			{"ground", "gray until meaningful", "2nd"},
		}
	case PageCommands:
		rows := []contextRow{
			{"verbs", fmt.Sprintf("%d registered", len(verbs)), "pri"},
			{"intent", fmt.Sprintf("%d governed targets", len(lookupIntentArgs())), "org"},
			{"templates", "{{focus}} / {{ring.0}}", "yel"},
			{"receipt", "preflight before effect", "2nd"},
		}
		if v, ok := m.FocusedCommand(); ok && !m.passiveReferenceSplitActive() {
			rows = append([]contextRow{
				{"selected", commandDisplayName(v), "brt"},
				{"kind", commandKindGroup(v), commandToken(v)},
				{"authority", firstNonEmpty(v.authority, "none"), "org"},
				{"preflight", firstNonEmpty(v.preflight, "none"), "2nd"},
				{"receipt", firstNonEmpty(v.receipt, "none"), "mut"},
				{"effect", v.uiDelta, "yel"},
			}, rows...)
		}
		return rows
	case PageWindows:
		rows := []contextRow{
			{"registry", "screens are objects", "pri"},
			{"cycle", "[←/→] every window", "yel"},
			{"split", "composition surface", "org"},
			{"count", fmt.Sprintf("%d windows", len(windowRegistry)), "pri"},
		}
		if wnd, ok := m.FocusedWindow(); ok && !m.passiveReferenceSplitActive() {
			pair, pairTok := splitPairCatalogCell(wnd.Page)
			signal, sigTok := m.windowSignal(wnd.Page)
			rows = append([]contextRow{
				{"selected", wnd.ID, "brt"},
				{"shape", wnd.Scope + "/" + wnd.Lifecycle + " · " + wnd.Kind, "pri"},
				{"signal/split", firstNonEmpty(signal, "quiet") + " · " + pair, strongerToken(sigTok, pairTok)},
				{"jump", "[" + wnd.Key + "] / :" + wnd.ID, "yel"},
			}, rows...)
		}
		return rows
	case PageSurfaces:
		rows := []contextRow{
			{"registry", fmt.Sprintf("%d surfaces", len(surfaceRegistry)), "pri"},
			{"template", "selection injection", "yel"},
		}
		if surf, ok := m.FocusedSurface(); ok && !m.passiveReferenceSplitActive() {
			rows = append(rows,
				contextRow{"selected", surf.ID, "brt"},
				contextRow{"glyph", surfaceKindGlyph(surf.Kind) + " " + surf.Kind, "org"},
				contextRow{"open", surf.Open, "yel"},
				contextRow{"exit", surf.Exit, "2nd"},
				contextRow{"contract", surf.Contract, "mut"},
			)
		}
		rows = append(rows,
			contextRow{"split", "session + context", "org"},
			contextRow{"modal", "all named, all exitable", "grn"},
		)
		return rows
	case PageDomains:
		sourceRows := len(m.Domains.Rows)
		sourceText := "compiled fallback"
		sourceTok := "yel"
		if sourceRows > 0 {
			sourceText = fmt.Sprintf("%d pack rows", sourceRows)
			sourceTok = "grn"
		} else if m.DomainsDark {
			sourceText = "read dark"
			sourceTok = "red"
		}
		rows := []contextRow{
			{"ontology", sourceText, sourceTok},
			{"n-DLC", "tenant-extensible map", "yel"},
			{"forms", "stream/rail/matrix/graph", "pri"},
			{"parity", "Trainyard/Labrack/etc", "2nd"},
		}
		if row, ok := m.FocusedDomainRow(); ok && !m.passiveReferenceSplitActive() {
			rows = append(rows,
				contextRow{"selected", domainRowFieldForAir(row, "domain_id", m.AIR), "brt"},
				contextRow{"state", domainRowFieldForAir(row, "state", m.AIR), domainStateToken(row.State)},
				contextRow{"terrain", domainRowFieldForAir(row, "terrain", m.AIR) + " / " + domainRowFieldForAir(row, "depth", m.AIR), domainDepthToken(row.Depth)},
				contextRow{"windows", domainRowFieldForAir(row, "windows", m.AIR), "yel"},
				contextRow{"surfaces", domainRowFieldForAir(row, "surfaces", m.AIR), "2nd"},
				contextRow{"authority", domainRowFieldForAir(row, "authority_ceiling", m.AIR), "mut"},
			)
		} else if d, ok := m.FocusedDomain(); ok && !m.passiveReferenceSplitActive() {
			rows = append(rows,
				contextRow{"selected", d.ID, "brt"},
				contextRow{"terrain", d.Terrain + " / " + d.Depth, domainDepthToken(d.Depth)},
				contextRow{"windows", d.Windows, "yel"},
				contextRow{"surfaces", d.Surfaces, "2nd"},
				contextRow{"parity", d.Parity, "mut"},
			)
		}
		return rows
	case PageLifecycles:
		sourceRows := len(m.Domains.Lifecycles)
		sourceText := "compiled fallback"
		sourceTok := "yel"
		if sourceRows > 0 {
			sourceText = fmt.Sprintf("%d lifecycle rows", sourceRows)
			sourceTok = "grn"
		} else if m.DomainsDark {
			sourceText = "read dark"
			sourceTok = "red"
		}
		rows := []contextRow{
			{"contracts", sourceText, sourceTok},
			{"n-DLC", "tenant lifecycle registry", "yel"},
			{"authority", firstNonEmpty(m.Domains.LifecycleAuthority, "support/non-authoritative"), "2nd"},
			{"boundary", "windows are projections", "mut"},
		}
		if row, ok := m.FocusedLifecycleRow(); ok && !m.passiveReferenceSplitActive() {
			rows = append(rows,
				contextRow{"selected", lifecycleRowFieldForAir(row, "lifecycle_id", m.AIR), "brt"},
				contextRow{"state", lifecycleRowFieldForAir(row, "state", m.AIR), domainStateToken(row.State)},
				contextRow{"posture", lifecycleRowFieldForAir(row, "posture", m.AIR), "org"},
				contextRow{"windows", lifecycleRowFieldForAir(row, "windows", m.AIR), "yel"},
				contextRow{"authority", lifecycleRowFieldForAir(row, "authority_ceiling", m.AIR), "mut"},
			)
		} else if fb, ok := m.FocusedLifecycleFallback(); ok && !m.passiveReferenceSplitActive() {
			rows = append(rows,
				contextRow{"selected", fb.ID, "brt"},
				contextRow{"scope", fb.Scope, "2nd"},
				contextRow{"windows", fb.Windows, "yel"},
				contextRow{"contract", fb.Contract, "mut"},
			)
		}
		return rows
	case PageEpistemics:
		rows := []contextRow{
			{"contract", "epistemic posture lens", "pri"},
			{"sources", "dynamics/intake/domains/caps", "2nd"},
			{"authority", "metadata-only support", "yel"},
			{"next", "map element -> evidence row", "org"},
		}
		if row, ok := m.FocusedEpistemicRow(); ok {
			rows = append([]contextRow{
				{"selected", row.Subject, "brt"},
				{"family", row.Family, "org"},
				{"status", row.Status, row.Token},
				{"authority", row.Authority, "yel"},
				{"evidence", row.Evidence, "2nd"},
				{"privacy", row.Privacy, "mut"},
			}, rows...)
		}
		return rows
	case PageIntent:
		target := m.IntentTarget
		if target == "" {
			target = "choose target"
		}
		if m.passiveReferenceSplitActive() {
			target = "source lane anchored"
		}
		return []contextRow{
			{"effect", "none emitted", "grn"},
			{"target", target, "yel"},
			{"authority", "governed route required", "org"},
			{"receipt", "preview only", "2nd"},
		}
	}
	return []contextRow{
		{"contract", "reference projection", "pri"},
		{"scroll", "document + context", "2nd"},
	}
}

func (m Model) referenceSignalRows() []contextRow {
	darkSources := 0
	for _, dark := range []bool{m.EventsDark, m.TasksDark, m.SessionsDark, m.IntakeDark, m.CapabilitiesDark, m.GatesDark, m.DynamicsDark} {
		if dark {
			darkSources++
		}
	}
	visibleBlocked, hiddenBlocked := m.yardBlockedIndices()
	capsTotal, capsGaps := m.capabilityTitleCounts()
	gateRows := len(m.Gates.Rows)
	gateBlocked := m.Gates.Totals["blocked"]
	gatePreview := m.Gates.Totals["preview"]
	if gateRows > 0 && gateBlocked == 0 && gatePreview == 0 {
		for _, row := range m.Gates.Rows {
			if strings.EqualFold(row.State, "preview-only") || strings.EqualFold(row.State, "preview_only") {
				gatePreview++
			} else if strings.EqualFold(row.State, "blocked") || strings.EqualFold(row.State, "missing") || gateRowToken(row) == "red" {
				gateBlocked++
			}
		}
	}
	gateSignal := fmt.Sprintf("%d rows · %d blocked", gateRows, gateBlocked)
	if gatePreview > 0 {
		gateSignal += fmt.Sprintf(" · %d preview", gatePreview)
	}
	return []contextRow{
		{"dark", fmt.Sprintf("%d sources", darkSources), countWarnToken(darkSources)},
		{"release", fmt.Sprintf("%d hold · %d hidden", len(visibleBlocked), hiddenBlocked), countWarnToken(len(visibleBlocked) + hiddenBlocked)},
		{"caps", fmt.Sprintf("%d total · %d gaps", capsTotal, capsGaps), countWarnToken(capsGaps)},
		{"gates", gateSignal, countWarnToken(gateBlocked)},
		{"intake", fmt.Sprintf("%d rows · %d demand", len(m.Intake.Rows), m.intakeAttentionTotal()), countWarnToken(m.intakeAttentionTotal())},
		{"sources", fmt.Sprintf("e%d t%d s%d i%d c%d g%d o%d d%d p%d", len(m.Events), len(m.Tasks), len(m.Sessions), len(m.Intake.Rows), len(m.Capabilities.Rows), len(m.Gates.Rows), len(m.Domains.Rows), len(m.Dynamics.AtResolution(m.DynScale).Nodes), len(m.epistemicRows())), "2nd"},
	}
}

func (m Model) referenceLegalRows() []contextRow {
	prev, next, ok := m.referenceNeighborWindows()
	prevID, nextID := "window", "window"
	if ok {
		prevID, nextID = ":"+prev.ID, ":"+next.ID
	}
	if m.sessionSplit() && !m.targetRowFocusActive() {
		rel := m.splitRelation()
		rows := []contextRow{
			{"[←/→]", prevID + " / " + nextID, "yel"},
			{"[:]", "command + template refs", "yel"},
			{"[?]", "legend", "yel"},
		}
		cues := m.splitControlCues(rel, splitCueTextOptions{IncludeScrollLabel: false, IncludeContext: false, IncludeSourceVerbs: false})
		for i := len(cues) - 1; i >= 0; i-- {
			rows = append([]contextRow{{cues[i].Key, cues[i].Long, "yel"}}, rows...)
		}
		return rows
	}
	if m.Page == PageIntake {
		rows := []contextRow{
			{"[j/k]", fmt.Sprintf("bucket %d/%d", m.IFocus+1, maxVisible(1, len(m.visibleIntakeRows()))), "yel"},
			{"[s/S]", "source filter", "yel"},
			{"[Enter]", "aggregate detail", "yel"},
			{"[E]", "evidence path", "yel"},
			{"[←/→]", prevID + " / " + nextID, "yel"},
			{"[:]", "command + template refs", "yel"},
			{"[?]", "legend", "yel"},
		}
		if m.referenceScrollMax() > 0 {
			rows = append([]contextRow{{"[J/K]", "page scroll", "yel"}}, rows...)
		}
		return rows
	}
	if m.Page == PageIntent {
		rows := []contextRow{
			{"[j/k]", fmt.Sprintf("intent target %d/%d", m.IntentFocus+1, maxVisible(1, len(lookupIntentArgs()))), "yel"},
			{"[Enter]", "preview selected target", "yel"},
			{"[←/→]", prevID + " / " + nextID, "yel"},
			{"[:]", "command + template refs", "yel"},
			{"[?]", "legend", "yel"},
		}
		if m.referenceScrollMax() > 0 {
			rows = append([]contextRow{{"[J/K]", "page scroll", "yel"}}, rows...)
		}
		return rows
	}
	if m.Page == PageCaps {
		rows := []contextRow{
			{"[j/k]", fmt.Sprintf("capability %d/%d", m.CFocus+1, maxVisible(1, len(m.capabilityDisplayRows()))), "yel"},
			{"[g/G]", "first/last capability", "yel"},
			{"[←/→]", prevID + " / " + nextID, "yel"},
			{"[:]", "command + template refs", "yel"},
			{"[?]", "legend", "yel"},
		}
		if m.sessionSplit() {
			rows = append([]contextRow{{"[J/K]", "scroll context", "yel"}}, rows...)
		}
		return rows
	}
	if m.Page == PageCommands {
		return []contextRow{
			{"[j/k]", fmt.Sprintf("command %d/%d", m.CommandFocus+1, maxVisible(1, len(verbs))), "yel"},
			{"[g/G]", "first/last command", "yel"},
			{"[←/→]", prevID + " / " + nextID, "yel"},
			{"[:]", "type or template command", "yel"},
			{"[?]", "legend", "yel"},
		}
	}
	if m.Page == PageWindows {
		return []contextRow{
			{"[j/k]", fmt.Sprintf("window %d/%d", m.WindowFocus+1, maxVisible(1, len(registeredWindows()))), "yel"},
			{"[g/G]", "first/last window", "yel"},
			{"[←/→]", prevID + " / " + nextID, "yel"},
			{"[:]", "command + template refs", "yel"},
			{"[?]", "legend", "yel"},
		}
	}
	if m.Page == PageSurfaces {
		return []contextRow{
			{"[j/k]", fmt.Sprintf("surface %d/%d", m.SurfaceFocus+1, maxVisible(1, len(registeredSurfaces()))), "yel"},
			{"[g/G]", "first/last surface", "yel"},
			{"[←/→]", prevID + " / " + nextID, "yel"},
			{"[:]", "command + template refs", "yel"},
			{"[?]", "legend", "yel"},
		}
	}
	if m.Page == PageDomains {
		return []contextRow{
			{"[j/k]", fmt.Sprintf("domain %d/%d", m.DomainFocus+1, maxVisible(1, m.domainRowCount())), "yel"},
			{"[g/G]", "first/last domain", "yel"},
			{"[←/→]", prevID + " / " + nextID, "yel"},
			{"[:]", "command + template refs", "yel"},
			{"[?]", "legend", "yel"},
		}
	}
	if m.Page == PageEpistemics {
		return []contextRow{
			{"[j/k]", fmt.Sprintf("evidence %d/%d", m.EpiFocus+1, maxVisible(1, len(m.epistemicRows()))), "yel"},
			{"[g/G]", "first/last evidence", "yel"},
			{"[←/→]", prevID + " / " + nextID, "yel"},
			{"[:]", "command + template refs", "yel"},
			{"[?]", "legend", "yel"},
		}
	}
	rows := []contextRow{
		{"[←/→]", prevID + " / " + nextID, "yel"},
		{"[:]", "command + template refs", "yel"},
		{"[?]", "legend", "yel"},
		{"[a]", "AIR lens", "yel"},
	}
	if m.referenceScrollMax() > 0 {
		rows = append([]contextRow{{"[j/k]", "scroll " + m.referenceScrollLabel(), "yel"}}, rows...)
	}
	return rows
}

func (m Model) referenceNeighborWindows() (WindowDef, WindowDef, bool) {
	windows := registeredWindows()
	if len(windows) == 0 {
		return WindowDef{}, WindowDef{}, false
	}
	idx := -1
	for i, wnd := range windows {
		if wnd.Page == m.Page {
			idx = i
			break
		}
	}
	if idx < 0 {
		return WindowDef{}, WindowDef{}, false
	}
	prev := windows[(idx-1+len(windows))%len(windows)]
	next := windows[(idx+1)%len(windows)]
	return prev, next, true
}

func referenceDoctrineRows() []contextRow {
	return []contextRow{
		{"ontology", "domain/lens before screen", "org"},
		{"affordance", "visible action at point", "yel"},
		{"state", "freshness/authority/AIR", "pri"},
		{"space", "context, constraints, topology", "grn"},
		{"motion", "liveness/flow/focus only", "2nd"},
	}
}

func referenceFormRows() []contextRow {
	return []contextRow{
		{"stream", "arrival order", "pri"},
		{"rail", "priority/attention", "yel"},
		{"ladder", "stage progression", "org"},
		{"matrix", "fit/comparison", "blu"},
		{"graph", "topology/centrality", "fch"},
		{"figure", "source-backed exposition", "2nd"},
	}
}

func referenceFundamentalRows() []contextRow {
	return []contextRow{
		{"doctrine", "affordance · state · space · motion", "yel"},
		{"forms", "stream · rail · ladder · matrix · graph", "pri"},
		{"split", splitPairSummary(), "org"},
		{"parity", "Trainyard · Labrack · SectionFig · tools", "2nd"},
	}
}

func referenceSplitRows() []contextRow {
	linked, sourceOnly := splitPairBuckets()
	return []contextRow{
		{"summary", splitPairSummary(), "pri"},
		{"linked", strings.Join(linked, ", "), "org"},
		{"source", strings.Join(sourceOnly, ", "), "2nd"},
		{"left", "sessions source", "yel"},
		{"right", "active window relation/context", "yel"},
	}
}

func splitPairBuckets() (linked, sourceOnly []string) {
	for _, pair := range registeredSplitPairs() {
		label := pageLabel(pair.Page)
		if wnd, ok := windowForPage(pair.Page); ok {
			label = wnd.ID
		}
		if pair.Reactive() {
			linked = append(linked, label)
		} else {
			sourceOnly = append(sourceOnly, label)
		}
	}
	return linked, sourceOnly
}

func referenceParityRows() []contextRow {
	return []contextRow{
		{"Trainyard", "SDLC cockpit", "pri"},
		{"Labrack", "RDLC custody/assay", "org"},
		{"SectionFig", "research figures", "blu"},
		{"intake", "observations/demand", "yel"},
		{"tools", "in-session capability", "fch"},
	}
}

// contextLine: the one-line "what am I looking at" for the active page (Norman conceptual model).
func (m Model) contextLine() string {
	switch m.Page {
	case PageTasks:
		f := "—"
		if t, ok := m.FocusedTask(); ok {
			f = grammar.Redact(t.AIR, "task_id", t.TaskID, m.AIR) // context line honors AIR too
		}
		shown := ""
		if strings.TrimSpace(m.Filter) != "" || m.CritFilter != "" {
			var parts []string
			if m.CritFilter != "" {
				parts = append(parts, "class "+m.CritFilter)
			}
			if strings.TrimSpace(m.Filter) != "" {
				parts = append(parts, fmt.Sprintf("%q", m.Filter))
			}
			shown = fmt.Sprintf(" · filter %s → %d shown [Esc clears]", strings.Join(parts, "+"), len(m.visibleTasks()))
		}
		if len(m.Tasks) == 0 {
			return grammar.C("2nd", " task registry · 0 tasks · no rows · focus: —")
		}
		return grammar.C("2nd", fmt.Sprintf(" task registry · %d tasks%s · [/] filter · focus: %s", len(m.Tasks), shown, f))
	case PageEvents:
		f := "—"
		if ev, ok := m.FocusedEvent(); ok {
			f = grammar.Redact(ev.AIR, "subject", ev.Subject, m.AIR)
			if r := []rune(f); len(r) > 28 {
				f = string(r[:28])
			}
		}
		if len(m.Events) == 0 {
			return grammar.C("2nd", " live coord events · no events · focus: —")
		}
		// "%d live" not "shown": under the DOI fold not every live event occupies a cell — the body's
		// "+N folded" marker is the honest count of what receded. Claiming "N shown" would contradict it.
		return grammar.C("2nd", fmt.Sprintf(" live coord events · newest at bottom · %d live · [j/k] select · [y]ank · focus: %s", len(m.Events), f))
	case PageTraces:
		f := "—"
		if tr, ok := m.FocusedTrace(); ok {
			f = grammar.Redact(tr.AIR, "trace_id", tr.TraceID, m.AIR)
			if r := []rune(f); len(r) > 28 {
				f = string(r[:28])
			}
		}
		if len(m.Traces) == 0 {
			return grammar.C("2nd", " LLM traces · no rows · focus: —")
		}
		return grammar.C("2nd", fmt.Sprintf(" LLM traces · newest at bottom · %d rows · [j/k] select · focus: %s", len(m.Traces), f))
	case PageSessionTurns:
		f := "—"
		if turn, ok := m.FocusedTurn(); ok {
			f = turnIDForAir(turn, m.AIR)
			if r := []rune(f); len(r) > 28 {
				f = string(r[:28])
			}
		}
		if len(m.TurnLadder) == 0 {
			return grammar.C("2nd", " session turns · fixture not loaded · focus: —")
		}
		return grammar.C("2nd", fmt.Sprintf(" SESSION TURN-LADDER · fixture-fed ahead of CapabilityIO · %d turns · [j/k] select [y]ank · focus: %s", len(m.TurnLadder), f))
	case PageSessions:
		f := "—"
		if s, ok := m.FocusedSession(); ok {
			role := grammar.Redact(s.AIR, "role", s.Role, m.AIR)
			rdy := grammar.Redact(s.AIR, "readiness", s.Readiness, m.AIR)
			f = role
			if strings.TrimSpace(rdy) != "" {
				f += " [" + rdy + "]"
			}
			if r := []rune(f); len(r) > 28 {
				f = string(r[:28])
			}
		}
		if len(m.Sessions) == 0 {
			return grammar.C("2nd", " live sessions · no lanes · focus: —")
		}
		return grammar.C("2nd", fmt.Sprintf(" session hotlist · %d lanes · [↵]detail [r]intent [y]ank · %s", len(m.Sessions), f))
	}
	return ""
}

// eventsBody: context line + header + a windowed slice of events that keeps the focused row in view
// (default = the newest tail). The events page is a first-class SELECTABLE surface — a focus cursor
// (▌), [j/k] to move, [y] to yank an event field — symmetric with :tasks.
func (m Model) eventsBody(w, h int) string {
	if m.EventsDark {
		return m.contextLine() + "\n" + darkHint(m.EventsError, m.AIR)
	}
	if w >= 150 {
		return m.eventsWideBody(w, h)
	}
	return m.eventsListBody(w, h)
}

// tracesBody: context line + header + a windowed slice of LLM-trace rows that keeps the focused
// row in view (default = the newest tail). A first-class SELECTABLE surface — a focus cursor +
// [j/k] to move — symmetric with :events. Field-yank is a tracked follow-up; the row still
// focuses + AIR-gates per field via grammar.RenderTraceRow.
func (m Model) tracesBody(w, h int) string {
	if m.TracesDark {
		return m.contextLine() + "\n" + darkHint(m.TracesError, m.AIR)
	}
	return m.tracesListBody(w, h)
}

// traceImportance scores a trace by SPEND and LATENCY — an expensive or slow call is interesting, a
// routine cheap/fast one recedes. Saturating (1−e^−x) so one outlier doesn't dwarf the rest. AIR: a
// denied cost/latency contributes nothing, so the visible set can't disclose the redacted magnitude.
func traceImportance(tr grammar.Trace, air bool) float64 {
	x := 0.0
	if !air || tr.AIR["cost"] == "ok" {
		x += tr.Cost / 0.05 // ~$0.05 ≈ a notable call
	}
	if !air || tr.AIR["latency_ms"] == "ok" {
		x += float64(tr.LatencyMs) / 2000.0 // ~2s ≈ a slow call
	}
	return 1 - math.Exp(-x)
}

// traceDoiSelection folds the traces feed through the shared doiVisible mechanism (same as events).
func (m Model) traceDoiSelection(budget int) (order []int, folded int) {
	imps := make([]float64, len(m.Traces))
	for i, tr := range m.Traces {
		imps[i] = traceImportance(tr, m.AIR)
	}
	return doiVisible(imps, m.TFocus, budget)
}

func (m Model) tracesListBody(w, h int) string {
	visible := h - 2 // context + header
	if visible < 1 {
		visible = 1
	}
	reserve := len(m.Traces) > visible && visible >= 2
	budget := visible
	if reserve {
		budget = visible - 1 // reserve a cell for the "+N folded" marker so it can't be clipped
	}
	order, folded := m.traceDoiSelection(budget)

	var b strings.Builder
	b.WriteString(m.contextLine() + "\n")
	b.WriteString("  " + grammar.RenderTraceHeader() + "\n")
	brushed := m.brushedTraces()
	for _, i := range order {
		switch {
		case i == m.TFocus:
			b.WriteString(grammar.C("yel", m.focusGlyph()) + focusBar(grammar.RenderTraceRow(m.Traces[i], m.AIR), w-1) + "\n")
		case brushed[traceEntity(m.Traces[i], m.AIR).ID]:
			// brushed: shares the focused trace's strongest emergent facet (├ decoded by the connector)
			b.WriteString(grammar.C("2nd", "├") + " " + grammar.RenderTraceRow(m.Traces[i], m.AIR) + "\n")
		default:
			b.WriteString("  " + grammar.RenderTraceRow(m.Traces[i], m.AIR) + "\n")
		}
	}
	if folded > 0 && reserve {
		// older traces no longer drop silently off the top — the DOI fold selects by spend/latency and
		// names the receded remainder honestly
		b.WriteString(grammar.C("mut", fmt.Sprintf("  +%d folded (lower interest) · [j/k] to summon", folded)) + "\n")
	}
	return b.String()
}

// renderSelectedTrace is the algebra secondary for :traces — the focused trace's full spend + latency
// breakdown, every field AIR-gated. The narrow primary list clips trailing columns (cost); this pane is
// where the operator reads the spend without clipping.
func (m Model) renderSelectedTrace(w int) string {
	tr, ok := m.FocusedTrace()
	if !ok {
		return grammar.C("mut", " trace detail\n\n no selected trace\n")
	}
	var b strings.Builder
	writeSectionHeader(&b, w, "TRACE DETAIL", "the focused LLM call — spend + latency, metadata-only", grammar.Redact(tr.AIR, "model", tr.Model, m.AIR))
	writeWrappedKV(&b, "trace", grammar.Redact(tr.AIR, "trace_id", tr.TraceID, m.AIR), "pri", w)
	writeWrappedKV(&b, "ts", grammar.Redact(tr.AIR, "ts", tr.TS, m.AIR), "2nd", w)
	writeWrappedKV(&b, "model", grammar.Redact(tr.AIR, "model", tr.Model, m.AIR), "org", w)
	writeWrappedKV(&b, "tokens", grammar.Redact(tr.AIR, "total_tok", fmt.Sprintf("prompt=%d completion=%d total=%d", tr.PromptTok, tr.CompletionTok, tr.TotalTok), m.AIR), "2nd", w)
	writeWrappedKV(&b, "cost", grammar.Redact(tr.AIR, "cost", fmt.Sprintf("$%.6f", tr.Cost), m.AIR), "yel", w)
	writeWrappedKV(&b, "latency", grammar.Redact(tr.AIR, "latency_ms", fmt.Sprintf("%dms", tr.LatencyMs), m.AIR), "2nd", w)
	writeWrappedKV(&b, "contract", "metadata-only LLM observability; no prompt/response body, no tool I/O — cost + latency are the held signals", "mut", w)
	return b.String()
}

// turnSourceLabel honestly names the chat-pane feed: live (freshly streaming a lane's turns), STALE
// (kept-but-old live rows after the feed went dark), or the demo FIXTURE (no live data) — so canned
// data is never read as live AND stale-live is never read as the fixture. The lane role redacts on air.
func (m Model) turnSourceLabel() string {
	role := grammar.Redact(nil, "label", strings.TrimSpace(m.TurnRole), m.AIR)
	if !m.TurnsDark {
		return "· live — streaming " + role + " session turns"
	}
	if m.TurnsFixture {
		if strings.TrimSpace(m.TurnRole) == "" {
			return "· demo fixture — no lane targeted (live session feed dark)"
		}
		return "· demo fixture — live turn feed for " + role + " is dark"
	}
	return "· stale — " + role + " live feed dark (showing the last live page)"
}

func (m Model) turnListBody(w, h int) string {
	topIdx, attnScore, _ := m.turnTopAttention()
	visible := h - 5 // two-row lane rail + source label + rule + header
	if attnScore > 0 {
		visible-- // reserve the attention pointer line (E4.9 synergy 2 — the one turn that needs you)
	}
	if visible < 1 {
		visible = 1
	}
	start := 0
	if len(m.TurnLadder) > visible {
		if m.TurnFocus >= visible {
			start = m.TurnFocus - visible + 1
		}
		if mx := len(m.TurnLadder) - visible; start > mx {
			start = mx
		}
	}
	var b strings.Builder
	b.WriteString(m.turnLaneRail(w) + "\n")
	if bk := m.turnBreakdownInbox(w); bk != "" {
		b.WriteString(bk + "\n")
	}
	if pos := m.turnSessionPosition(w); pos != "" {
		b.WriteString(pos + "\n")
	}
	srcTok := "2nd"
	if m.TurnsDark {
		srcTok = "mut"
	}
	b.WriteString(" " + grammar.C(srcTok, clipRunes(m.turnSourceLabel(), maxVisible(8, w-1))) + "\n")
	b.WriteString(" " + grammar.C("border", strings.Repeat("─", maxVisible(10, w-2))) + "\n")
	b.WriteString("  " + grammar.RenderTurnHeader() + "\n")
	if len(m.TurnLadder) == 0 {
		b.WriteString(" " + grammar.C("mut", "(no turn fixture loaded — CapabilityIO SESSION feed gated)") + "\n")
		return b.String()
	}
	if p := m.turnAttentionPointer(w, start, visible); p != "" {
		b.WriteString(p + "\n")
	}
	for i := start; i < start+visible && i < len(m.TurnLadder); i++ {
		row := grammar.RenderTurnRow(m.TurnLadder[i], m.AIR)
		switch {
		case i == m.TurnFocus:
			b.WriteString(grammar.C("yel", m.focusGlyph()) + focusBar(row, w-1) + "\n")
		case i == topIdx:
			b.WriteString(grammar.C("yel", "‼") + " " + row + "\n") // E4.9 synergy 2 — the attention turn
		default:
			b.WriteString("  " + row + "\n")
		}
	}
	return b.String()
}

// turnSessionPosition renders the C4 LIVE SESSION POSITION for the focused lane (m.TurnRole): WHERE it
// is in the n-DLC — its claimed task and that task's SDLC stage. This is the per-session variable part
// of the 06-28 coordination canon (C4 = active task · current stage), folded from the present read
// model. AIR-safe: role + claimed_task + stage redact per their own AIR maps; a denied field becomes
// ▒▒▒ in place. ("What's authorized" — the legal next transitions — awaits the FSM read endpoint.)
func (m Model) turnSessionPosition(w int) string {
	var s grammar.Session
	found := false
	for _, x := range m.Sessions {
		if x.Role == m.TurnRole {
			s, found = x, true
			break
		}
	}
	if !found {
		return ""
	}
	role := grammar.Redact(s.AIR, "role", s.Role, m.AIR)
	parts := []string{grammar.C("brt", "POSITION") + grammar.C("mut", "  lane ") + role}
	if strings.TrimSpace(s.ClaimedTask) == "" {
		parts = append(parts, grammar.C("mut", "no claimed task"))
		return fitWidth(" "+strings.Join(parts, grammar.C("mut", " · ")), w)
	}
	parts = append(parts, grammar.C("mut", "task ")+grammar.Redact(s.AIR, "claimed_task", s.ClaimedTask, m.AIR))
	stage, predicted := "", ""
	for _, t := range m.Tasks {
		if t.TaskID == s.ClaimedTask {
			stage = grammar.Redact(t.AIR, "stage", t.Stage, m.AIR)
			predicted = grammar.Redact(t.AIR, "predicted_stage", t.PredictedStage, m.AIR)
			break
		}
	}
	switch {
	case stage != "":
		seg := grammar.C("mut", "stage ") + stage
		if predicted != "" && predicted != stage {
			seg += grammar.C("mut", " → ") + predicted
		}
		parts = append(parts, seg)
	default:
		parts = append(parts, grammar.C("mut", "stage not in view"))
	}
	return fitWidth(" "+strings.Join(parts, grammar.C("mut", " · ")), w)
}

// turnBreakdownInbox is the aggregated decision surface (E4.5): it AUTO-SURFACES only at breakdown —
// the lanes that need the operator (gate-blocked or awaiting input) named with the reason. Steady state
// returns "" (the equipment recedes; conditions appear only when abnormal — the legibility law).
// AIR-safe: ranks/names off allowlisted role/state/readiness/blocker; a denied field degrades, no leak.
func (m Model) turnBreakdownInbox(w int) string {
	type need struct{ role, why string }
	needs := make([]need, 0, len(m.Sessions))
	for _, s := range m.Sessions {
		denied := func(f string) bool { return m.AIR && s.AIR[f] != "ok" }
		role := grammar.Redact(s.AIR, "role", s.Role, m.AIR)
		switch {
		case s.Stalled && !denied("stalled"),
			!denied("readiness") && (s.Readiness == "red" || s.Readiness == "stall"),
			!denied("blocker") && strings.TrimSpace(s.Blocker) != "" && s.Blocker != "none":
			needs = append(needs, need{role, "blocked"})
		case !denied("state") && s.State == "awaiting":
			needs = append(needs, need{role, "awaiting"})
		}
	}
	if len(needs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(needs))
	for _, n := range needs {
		tok := "red"
		if n.why == "awaiting" {
			tok = "yel"
		}
		parts = append(parts, grammar.C(tok, n.role+" ("+n.why+")"))
	}
	head := grammar.C("red", fmt.Sprintf(" ⚠ BREAKDOWN — %d lane(s) need you: ", len(needs)))
	return fitWidth(head+strings.Join(parts, grammar.C("mut", " · ")), w)
}

// turnLaneRail is the FLEET lane-rail above the turn split (E4.5 — the one-coordinating-session model):
// EVERY lane (session) in the fleet is an ambient pulse, severity-ranked (blocked > awaiting > done >
// streaming > idle), numbered for O(1) reference, with the focused lane (m.TurnRole, whose turns the
// split below shows) marked ▌. AIR-safe by construction: role/state/readiness/blocker/stalled all air
// per the allowlist; a DENIED field degrades that lane's rank to its safe default rather than letting
// the ordering disclose the denied value (the derived-channel discipline).
func (m Model) turnLaneRail(w int) string {
	type lane struct {
		role   string
		status string
		rank   int
		focus  bool
	}
	rankFor := func(s grammar.Session) (string, int) {
		denied := func(f string) bool { return m.AIR && s.AIR[f] != "ok" }
		switch {
		case s.Stalled && !denied("stalled"),
			!denied("readiness") && (s.Readiness == "red" || s.Readiness == "stall"),
			!denied("blocker") && strings.TrimSpace(s.Blocker) != "" && s.Blocker != "none":
			return "blocked", 5
		}
		state := ""
		if !denied("state") {
			state = s.State
		}
		switch state {
		case "awaiting":
			return "awaiting", 4
		case "streaming", "active":
			return "streaming", 2
		}
		if !denied("readiness") && s.Readiness == "green" && (denied("state") || s.State == "idle") {
			return "done", 3
		}
		if (s.Idle && !denied("idle")) || state == "idle" {
			return "idle", 1
		}
		return "active", 2
	}
	lanes := make([]lane, 0, len(m.Sessions))
	for _, s := range m.Sessions {
		role := grammar.Redact(s.AIR, "role", s.Role, m.AIR)
		if strings.TrimSpace(role) == "" {
			role = "—"
		}
		status, rk := rankFor(s)
		lanes = append(lanes, lane{role: role, status: status, rank: rk, focus: s.Role == m.TurnRole})
	}
	sort.SliceStable(lanes, func(i, j int) bool {
		if lanes[i].rank != lanes[j].rank {
			return lanes[i].rank > lanes[j].rank
		}
		return lanes[i].role < lanes[j].role
	})
	tok := map[string]string{"blocked": "red", "awaiting": "yel", "done": "grn", "streaming": "pri", "active": "pri", "idle": "mut"}
	chips := make([]string, 0, len(lanes))
	for i, l := range lanes {
		mark := " "
		if l.focus {
			mark = "▌"
		}
		chips = append(chips, grammar.C(tok[l.status], fmt.Sprintf("%s[%d %s %s]", mark, i+1, l.role, l.status)))
	}
	if len(chips) == 0 {
		chips = append(chips, grammar.C("mut", "[no lanes]"))
	}
	head := " " + grammar.C("brt", "LANE-RAIL") + grammar.C("mut", "  fleet · ranked blocked>awaiting>done>streaming>idle · ▌ = the lane the turn split shows")
	body := " " + strings.Join(chips, grammar.C("mut", " "))
	return fitWidth(head, w) + "\n" + fitWidth(body, w)
}

func (m Model) turnDetailBody(w int) string {
	t, ok := m.FocusedTurn()
	if !ok {
		return grammar.C("mut", " turn detail\n\n no selected turn\n")
	}
	// E4.6 two-frame broadcast (session-pane design §3.4 / forks B+D): an in-flight turn is NOT settled,
	// so it renders the broadcast frame — off air the present-at-hand cleartext partial; on air ONLY the
	// shape '▸ generating… [N tok]', never a live token. Settled turns fall through to the normal block
	// stream below. The hold-buffer/dump-key is the E4.8 activation layer (gated on CapabilityIO Phase 1).
	if t.Streaming {
		frame := grammar.BroadcastStreamFrame(t.Summary, t.Tokens, m.AIR, maxVisible(8, w-4))
		return " " + frame + "\n" + m.turnImpingeAffordances(t)
	}
	var detail string
	blocks := m.TurnBlocks[TurnID(t)]
	if len(blocks) == 0 {
		// honest: a LIVE turn with no blocks is detail-fetch-pending, NOT a fixture (never-false-green).
		note := "(no expanded blocks for this live turn — the per-turn detail fetch is pending)"
		if m.TurnsFixture {
			note = "(no expanded blocks for this fixture turn)"
		}
		detail = grammar.RenderTurnDetail(t, nil, m.AIR) + "   " + grammar.C("mut", note) + "\n"
	} else {
		detail = grammar.RenderTurnDetail(t, blocks, m.AIR)
	}
	return detail + m.turnImpingeAffordances(t)
}

// turnImpingeAffordances previews the governed ACT-ON-THIS-TURN decisions for the focused turn (E4.4
// impinge, PREVIEW-ONLY): the HITL decision set contextualized by the turn KIND, routed through the
// governed COMMAND surface — NOT WIRED (never-mint; the egress stays gated). It turns the read-only
// turn into a LEGIBLE control surface (the operator's "tells-me-little / lets-me-do-nothing" veto fix)
// without yet wiring the AIR-critical send. AIR-safe: the verbs are structural, no turn body.
// impingeDecisionsFor is the HITL decision set offered for a turn KIND (LangChain impinge enum, §4):
// an approval gate offers accept/deny/edit, a tool_call approve/edit/deny, everything else respond/ignore.
func impingeDecisionsFor(kind string) []string {
	switch kind {
	case "approval":
		return []string{"accept", "deny", "edit"}
	case "tool_call":
		return []string{"approve", "edit", "deny"}
	default: // assistant/reasoning/refusal/user/result/etc. — answer or dismiss
		return []string{"respond", "ignore"}
	}
}

func (m Model) turnImpingeAffordances(t grammar.Turn) string {
	decisions := impingeDecisionsFor(t.Kind)
	verbs := make([]string, len(decisions))
	for i, d := range decisions {
		verbs[i] = grammar.C("brt", "["+d+"]")
	}
	primary := "respond"
	if len(decisions) > 0 {
		primary = decisions[0]
	}
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, 42)))
	// name the governed EVENT each decision maps to — the §4 legibility "x → gate_output" that the bare
	// verb does not show — kept short so the honesty floor (NOT wired) survives the narrow detail pane.
	// The FULL envelope (target/authority/preflight/receipt/Δ) is the :intent review pane's job.
	gate := grammar.C("mut", "gate_output ▸ ") + grammar.C("2nd", "impinge("+primary+")") +
		grammar.C("mut", " · ") + grammar.C("red", "NOT wired")
	return "\n " + rule + "\n " + grammar.C("brt", "IMPINGE") + grammar.C("mut", " — act on this turn: ") +
		strings.Join(verbs, " ") + grammar.C("mut", "  · governed COMMAND route → :intent") + "\n " + gate + "\n"
}

func (m Model) eventsWideBody(w, h int) string {
	leftW := 102
	if w < 210 {
		leftW = 88
	}
	if leftW > w-44 {
		leftW = w - 44
	}
	if leftW < 72 {
		return m.eventsListBody(w, h)
	}
	rightW := w - leftW - 1
	left := fitBlock(m.eventsListBody(leftW, h), leftW, h)
	right := fitBlockWithSlackFn(m.eventContextPane(rightW), rightW, h, func(rows int) []string {
		return m.slackRowsForPage(rightW, rows, slackSlotWideContext)
	})
	div := grammar.C("border", "│")
	out := make([]string, 0, h)
	for i := 0; i < h; i++ {
		out = append(out, left[i]+div+right[i])
	}
	return strings.Join(out, "\n")
}

// eventDeniedImportance is the importance assigned to an on-air event whose Score is DENIED: a uniform
// constant carrying NO signal, so the visible set's membership/order can never disclose the redacted
// score (the derived-channel discipline). 0 ranks denied events on proximity-to-focus alone.
const eventDeniedImportance = 0.0

// doiVisible is the framework's ONE scaling mechanism, pane-agnostic: given a per-item importance and a
// finite cell `budget`, it chooses WHICH items occupy the budget by DOI = importance − normalized
// distance-from-focus (Furnas), PINS the focused item visible, and returns the items to render in
// CHRONOLOGICAL order (selection decides membership; reading order is never reordered) plus the count
// folded into the "+N" tail. Every FEED pane (events, traces, …) folds through this same helper, so the
// "scale by allocation, recede the rest" contract is structural, not copy-pasted. AIR-safety lives in the
// CALLER's importance (a denied field must contribute a uniform value so ordering can't disclose it).
func doiVisible(importances []float64, focus, budget int) (order []int, folded int) {
	n := len(importances)
	if n == 0 {
		return nil, 0
	}
	if n <= budget {
		order = make([]int, n)
		for i := range order {
			order[i] = i
		}
		return order, 0
	}
	span := float64(n - 1)
	if span < 1 {
		span = 1
	}
	scored := make([]doi.Scored, n)
	for i := range importances {
		var d float64
		if i == focus {
			d = math.Inf(1) // the focal row is always retained, whatever its interest
		} else {
			dist := math.Abs(float64(i-focus)) / span // normalize so importance genuinely competes
			d = doi.DOI(importances[i], dist)
		}
		scored[i] = doi.Scored{ID: strconv.Itoa(i), DOI: d}
	}
	placements, aggregated := doi.Fold(scored, budget, 0)
	keep := make(map[int]bool, len(placements))
	for _, p := range placements {
		if idx, err := strconv.Atoi(p.ID); err == nil {
			keep[idx] = true
		}
	}
	for i := 0; i < n; i++ {
		if keep[i] {
			order = append(order, i)
		}
	}
	return order, aggregated
}

// eventDoiSelection folds the events feed through doiVisible. Importance is the served Score; a denied
// score on air contributes a uniform constant so the visible set's membership can't leak it.
func (m Model) eventDoiSelection(budget int) (order []int, folded int) {
	imps := make([]float64, len(m.Events))
	for i, ev := range m.Events {
		imp := ev.Score
		if m.AIR && ev.AIR["score"] != "ok" {
			imp = eventDeniedImportance
		}
		imps[i] = imp
	}
	return doiVisible(imps, m.EFocus, budget)
}

func (m Model) eventsListBody(w, h int) string {
	visible := h - 2 // context + header
	if visible < 1 {
		visible = 1
	}
	// Reserve a cell for the "+N folded" marker ONLY when there's room for both a row AND the marker
	// (visible ≥ 2). At visible == 1 we forfeit the marker rather than overflow the height and clip it;
	// the context line still discloses the live count. (glm review B1.)
	reserve := len(m.Events) > visible && visible >= 2
	budget := visible
	if reserve {
		budget = visible - 1
	}
	order, folded := m.eventDoiSelection(budget)

	var b strings.Builder
	b.WriteString(m.contextLine() + "\n")
	b.WriteString("  " + grammar.RenderEventHeader() + "\n") // 2-col gutter aligns under the cursor
	brushed := m.brushedEvents()
	for _, i := range order {
		switch {
		case i == m.EFocus && m.Mode == ModeYank:
			b.WriteString(fitWidth(eventPickRow(m.Events[i], m.AIR, m.Sel.Field), w) + "\n")
		case i == m.EFocus:
			b.WriteString(grammar.C("yel", m.focusGlyph()) + focusBar(grammar.RenderEventRow(m.Events[i], m.AIR), w-1) + "\n")
		case brushed[eventEntity(m.Events[i], m.AIR).ID]:
			// brushed: shares the focused event's strongest emergent facet (├ decoded by the connector)
			b.WriteString(grammar.C("2nd", "├") + " " + grammar.RenderEventRow(m.Events[i], m.AIR) + "\n")
		default:
			b.WriteString("  " + grammar.RenderEventRow(m.Events[i], m.AIR) + "\n")
		}
	}
	if folded > 0 && reserve {
		// the dropped tail, named honestly — the cure for today's silent older-event drop (only when a
		// cell was reserved for it; at visible==1 the context-line count carries the disclosure instead)
		b.WriteString(grammar.C("mut", fmt.Sprintf("  +%d folded (lower interest) · [j/k] to summon", folded)) + "\n")
	}
	return b.String()
}

func (m Model) eventContextPane(w int) string {
	if m.sessionSplit() {
		return m.sessionEventContextPane(w)
	}
	ev, ok := m.FocusedEvent()
	if !ok {
		return grammar.C("mut", " event context\n\n no selected event\n")
	}
	subj := grammar.Redact(ev.AIR, "subject", ev.Subject, m.AIR)
	if strings.TrimSpace(subj) == "" {
		subj = "·"
	}
	actor := grammar.Redact(ev.AIR, "actor", ev.Actor, m.AIR)
	kind := grammar.Redact(ev.AIR, "kind", ev.Kind, m.AIR)
	summary := grammar.Redact(ev.AIR, "summary", ev.Summary, m.AIR)
	if strings.TrimSpace(summary) == "" {
		summary = "·"
	}
	sameSubject, sameActor, failures, successes := 0, 0, 0, 0
	kinds := map[string]int{}
	subjAnchorOK := !m.AIR || ev.AIR["subject"] == "ok"
	actorAnchorOK := !m.AIR || ev.AIR["actor"] == "ok"
	for _, x := range m.Events {
		// neighborhood-by-subject/actor must not COUNT denied items (the cardinality discloses the
		// redacted field) — gate on BOTH the anchor's and the compared event's per-field AIR.
		if subjAnchorOK && (!m.AIR || x.AIR["subject"] == "ok") && ev.Subject != "" && x.Subject == ev.Subject {
			sameSubject++
		}
		if actorAnchorOK && (!m.AIR || x.AIR["actor"] == "ok") && ev.Actor != "" && x.Actor == ev.Actor {
			sameActor++
		}
		if (!m.AIR || x.AIR["kind"] == "ok") && strings.Contains(strings.ToLower(x.Kind), "fail") {
			failures++ // on air a denied kind must not be classified — the count discloses it
		}
		if (!m.AIR || x.AIR["kind"] == "ok") && strings.Contains(strings.ToLower(x.Kind), "succeed") {
			successes++
		}
		kinds[grammar.Redact(x.AIR, "kind", shortKindForPanel(x.Kind), m.AIR)]++ // per-event AIR: a denied kind aggregates as ▒▒▒, never leaks
	}
	// the state is a function of (kind, score); assert it only when BOTH inputs air — else "routine"
	// would leak "kind!=fail && score<0.45" through the back door. Both denied → honest unknown.
	stateTok := "grn"
	state := "routine"
	if m.AIR && (ev.AIR["kind"] != "ok" || ev.AIR["score"] != "ok") {
		stateTok, state = "mut", "▒▒▒"
	} else if strings.Contains(strings.ToLower(ev.Kind), "fail") || ev.Score >= 0.70 {
		stateTok, state = "red", "breakdown"
	} else if ev.Score >= 0.45 {
		stateTok, state = "yel", "watch"
	}
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	line := func(label, value, tok string) {
		writeWrappedKV(&b, label, value, tok, w)
	}
	b.WriteString(" " + grammar.C("brt", "EVENT CONTEXT") + grammar.C("mut", "  selected row, constraints, next legal moves") + "\n")
	b.WriteString(rule + "\n")
	line("state", state, stateTok)
	// show-WHY legibility rail (classification→affordance seed): the live signals DRIVING this row's
	// treatment, made legible — importance (the DOI/score allocator) → the state classification, plus the
	// emergent relation (brushed peers). The signal is shown, not yet acted on. AIR-safe: score redacts
	// per its field gate; the related count is derived from AIR-aware facets (never leaks a denied facet).
	signalScore := grammar.Redact(ev.AIR, "score", fmt.Sprintf("%.2f", ev.Score), m.AIR)
	line("signal", fmt.Sprintf("importance %s · %d related", signalScore, len(m.brushedEvents())), "mut")
	line("subject", subj, "pri")
	line("kind", kind, "blu")
	line("actor", actor, airHue(grammar.LaneToken(ev.Actor), ev.AIR, "actor", m.AIR))
	line("summary", summary, "2nd")
	line("score", grammar.Redact(ev.AIR, "score", fmt.Sprintf("%.2f", ev.Score), m.AIR), "pri")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "neighborhood") + "\n")
	line("same subj", fmt.Sprintf("%d events", sameSubject), "pri")
	line("same actor", fmt.Sprintf("%d events", sameActor), "pri")
	line("failures", fmt.Sprintf("%d recent", failures), severityCountToken(failures))
	line("successes", fmt.Sprintf("%d recent", successes), "grn")
	line("kinds", compactKindCounts(kinds, w-14), "2nd")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "constraints") + "\n")
	if m.AIR && ev.AIR["subject"] != "ok" {
		b.WriteString(" " + grammar.C("mut", "AIR denies subject; keep topology, hide value") + "\n")
	} else {
		b.WriteString(" " + grammar.C("mut", "read-only projection; source event is not authority") + "\n")
	}
	b.WriteString(" " + grammar.C("mut", "no dispatch · no transcript · no raw payload by default") + "\n")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "legal next") + "\n")
	b.WriteString(" " + grammar.C("yel", ":intent open-trace") + grammar.C("mut", " preview trace focus, no effect") + "\n")
	b.WriteString(" " + grammar.C("yel", ":intent show-route") + grammar.C("mut", " preview route/veto context") + "\n")
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) sessionEventContextPane(w int) string {
	s, ok := m.FocusedSession()
	if !ok {
		return grammar.C("mut", " event neighborhood\n\n no selected session\n")
	}
	role := sessionFieldValueForAir(s, "role", m.AIR)
	task := sessionFieldValueForAir(s, "claimed_task", m.AIR)
	related := m.sessionRelatedEvents(s)
	failures, successes := 0, 0
	kinds := map[string]int{}
	for _, ev := range related {
		classOK := !m.AIR || ev.AIR["kind"] == "ok" // on air a denied kind must not be classified — the fail/succeed tally + kinds breakdown disclose it
		if classOK && strings.Contains(strings.ToLower(ev.Kind), "fail") {
			failures++
		}
		if classOK && strings.Contains(strings.ToLower(ev.Kind), "succeed") {
			successes++
		}
		kinds[grammar.Redact(ev.AIR, "kind", shortKindForPanel(ev.Kind), m.AIR)]++ // per-event AIR: a denied kind aggregates as ▒▒▒, never leaks
	}
	var latest grammar.Event
	if len(related) > 0 {
		latest = related[len(related)-1]
	}
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	line := func(label, value, tok string) {
		writeWrappedKV(&b, label, value, tok, w)
	}
	b.WriteString(" " + grammar.C("brt", "EVENT NEIGHBORHOOD") + grammar.C("mut", "  selected session, actor/task events") + "\n")
	b.WriteString(rule + "\n")
	line("source", role, airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR))
	line("claimed", task, "pri")
	line("matched", fmt.Sprintf("%d events", len(related)), countToken(len(related)))
	line("failures", fmt.Sprintf("%d recent", failures), severityCountToken(failures))
	line("successes", fmt.Sprintf("%d recent", successes), "grn")
	line("kinds", compactKindCounts(kinds, w-14), "2nd")
	if len(related) > 0 {
		line("latest", eventFieldValueForAir(latest, "kind", latest.Kind, m.AIR), "blu")
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "related events") + "\n")
	if len(related) == 0 {
		b.WriteString(" " + grammar.C("mut", "no actor/task event in the current read window") + "\n")
	} else {
		start := 0
		if len(related) > 8 {
			start = len(related) - 8
		}
		for _, ev := range related[start:] {
			if w < 96 {
				writeWrappedKV(&b, "event", eventFieldValueForAir(ev, "subject", ev.Subject, m.AIR), "pri", w)
				writeWrappedKV(&b, "kind", eventFieldValueForAir(ev, "kind", ev.Kind, m.AIR), "blu", w)
				what := ev.Summary
				if strings.TrimSpace(what) == "" {
					what = shortKindForPanel(ev.Kind)
				}
				writeWrappedKV(&b, "what", eventFieldValueForAir(ev, "summary", what, m.AIR), "mut", w)
			} else {
				row := grammar.RenderEventRow(ev, m.AIR)
				b.WriteString(" " + fitWidth(row, maxVisible(8, w-2)) + "\n")
			}
		}
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "constraints") + "\n")
	b.WriteString(" " + grammar.C("mut", "read-only projection; selected session is not authority") + "\n")
	b.WriteString(" " + grammar.C("mut", "join = event.actor==role OR event.subject==claimed_task") + "\n")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "legal next") + "\n")
	b.WriteString(" " + grammar.C("yel", ":intent open-trace") + grammar.C("mut", " preview lane trace, no effect") + "\n")
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) sessionRelatedEvents(s grammar.Session) []grammar.Event {
	var out []grammar.Event
	role := strings.TrimSpace(s.Role)
	task := strings.TrimSpace(s.ClaimedTask)
	// AIR-aware join: on air, a join field denied on EITHER side (the session's role/claimed_task or
	// the event's actor/subject) must not contribute — else the related-event CARDINALITY rendered by
	// the yard/intake/caps cards + topology discloses the hidden role/claim relationship (fugu review).
	roleOK := !m.AIR || s.AIR["role"] == "ok"
	taskOK := !m.AIR || s.AIR["claimed_task"] == "ok"
	for _, ev := range m.Events {
		if role != "" && roleOK && ev.Actor == role && (!m.AIR || ev.AIR["actor"] == "ok") {
			out = append(out, ev)
			continue
		}
		if task != "" && taskOK && ev.Subject == task && (!m.AIR || ev.AIR["subject"] == "ok") {
			out = append(out, ev)
		}
	}
	return out
}

func shortKindForPanel(kind string) string {
	if strings.Contains(kind, "failed") {
		return "failed"
	}
	if strings.Contains(kind, "succeeded") || strings.Contains(kind, "succeed") {
		return "succeed"
	}
	if strings.Contains(kind, "started") {
		return "started"
	}
	return shortStage2(kind)
}

func compactKindCounts(kinds map[string]int, width int) string {
	if len(kinds) == 0 {
		return "·"
	}
	order := []string{"failed", "succeed", "started"}
	var parts []string
	seen := map[string]bool{}
	for _, k := range order {
		if n := kinds[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", k, n))
			seen[k] = true
		}
	}
	for k, n := range kinds {
		if !seen[k] && n > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", k, n))
		}
	}
	return clipRunes(strings.Join(parts, " · "), maxVisible(8, width))
}

func severityCountToken(n int) string {
	if n > 0 {
		return "red"
	}
	return "grn"
}

func maxVisible(lo, v int) int {
	if v < lo {
		return lo
	}
	return v
}

// eventPickRow: the :events analogue of yankPickRow — in ModeYank the focused event becomes a labeled
// field-picker IN PLACE. AIR-denied content fields (subject/actor/summary) redact + drop their key;
// ts/kind are structural (always pickable).
func eventPickRow(ev grammar.Event, air bool, cur string) string {
	clip := func(s string, n int) string {
		if r := []rune(s); len(r) > n {
			return string(r[:n])
		}
		return s
	}
	what := ev.Summary
	if strings.TrimSpace(what) == "" {
		what = ev.Kind
	}
	fields := []struct{ key, field, val string }{
		{"t", "ts", ev.TS}, {"K", "kind", clip(ev.Kind, 16)},
		{"s", "subject", clip(ev.Subject, 24)}, {"a", "actor", clip(ev.Actor, 12)},
		{"m", "summary", clip(what, 28)},
	}
	out := grammar.C("brt", "▶ yank ")
	for _, f := range fields {
		v := f.val
		denied := air && ev.AIR[f.field] != "ok"
		out += yankCell(f.key, f.field, v, cur, denied) + " "
	}
	return strings.TrimRight(out, " ")
}

func yankCell(key, field, val, cur string, denied bool) string {
	if denied {
		return grammar.C("mut", "["+key+"]▒▒▒")
	}
	if strings.TrimSpace(val) == "" {
		val = "·"
	}
	if field == cur {
		return grammar.SelLabel("["+key+"]") + grammar.C("brt", val)
	}
	return grammar.C("mut", "["+key+"]") + grammar.C("pri", val)
}

// sessionsBody renders the live lane/session roster. It intentionally shows roster and health only:
// no raw transcript, no PTY, no dispatch.
func (m Model) sessionsBody(w, h int) string {
	if m.SessionsDark {
		return m.contextLine() + "\n" + darkHint(m.SessionsError, m.AIR)
	}
	if w >= 150 {
		return m.sessionsWideBody(w, h)
	}
	return m.sessionsListBody(w, h)
}

func (m Model) sessionsWideBody(w, h int) string {
	leftW := 112
	if w < 220 {
		leftW = 96
	}
	if leftW > w-50 {
		leftW = w - 50
	}
	if leftW < 76 {
		return m.sessionsListBody(w, h)
	}
	rightW := w - leftW - 1
	left := fitBlock(m.sessionsListBody(leftW, h), leftW, h)
	right := fitBlockWithSlackFn(m.sessionConstraintPane(rightW), rightW, h, func(rows int) []string {
		return m.slackRowsForPage(rightW, rows, slackSlotWideContext)
	})
	div := grammar.C("border", "│")
	out := make([]string, 0, h)
	for i := 0; i < h; i++ {
		out = append(out, left[i]+div+right[i])
	}
	return strings.Join(out, "\n")
}

func (m Model) sessionsListBody(w, h int) string {
	visible := h - 2 // context + header
	if visible < 1 {
		visible = 1
	}
	if len(m.Sessions) == 0 {
		return m.contextLine() + "\n" + grammar.C("mut", " sessions empty — waiting for /read/sessions\n")
	}
	start := 0
	if len(m.Sessions) > visible {
		if m.SFocus >= visible {
			start = m.SFocus - visible + 1
		}
		if mx := len(m.Sessions) - visible; start > mx {
			start = mx
		}
	}
	var b strings.Builder
	b.WriteString(m.contextLine() + "\n")
	b.WriteString("  " + grammar.RenderSessionHeader() + "\n")
	for i := start; i < start+visible && i < len(m.Sessions); i++ {
		switch {
		case i == m.SFocus && m.Mode == ModeYank:
			b.WriteString(fitWidth(sessionPickRow(m.Sessions[i], m.AIR, m.Sel.Field), w) + "\n")
		case i == m.SFocus:
			b.WriteString(grammar.C("yel", m.focusGlyph()) + focusBar(grammar.RenderSessionRow(m.Sessions[i], m.AIR), w-1) + "\n")
		default:
			b.WriteString(m.sessionLiveGutter(m.Sessions[i]) + grammar.RenderSessionRow(m.Sessions[i], m.AIR) + "\n")
		}
	}
	return b.String()
}

func (m Model) sessionConstraintPane(w int) string {
	s, ok := m.FocusedSession()
	if !ok {
		return grammar.C("mut", " lane context\n\n no selected session\n")
	}
	role := sessionFieldValueForAir(s, "role", m.AIR)
	if strings.TrimSpace(role) == "" {
		role = "·"
	}
	stateTok := airHue(sessionStateToken(s.State), s.AIR, "state", m.AIR)
	rdyTok := airHue(readinessPaneToken(s.Readiness), s.AIR, "readiness", m.AIR)
	blockTok := airHue(blockerToken(s.Blocker), s.AIR, "blocker", m.AIR)
	claim, stale, off, stalled := 0, 0, 0, 0
	for _, x := range m.Sessions {
		// On air a denied readiness/state/stalled must NOT be classified into the fleet tally — the
		// per-class count discloses the field (same policy as blockedBreakdown / coordinatorThroughputLine).
		rdyOK := !m.AIR || x.AIR["readiness"] == "ok"
		stateOK := !m.AIR || x.AIR["state"] == "ok"
		stalledOK := !m.AIR || x.AIR["stalled"] == "ok"
		if rdyOK && x.Readiness == "claim" {
			claim++
		}
		if rdyOK && x.Readiness == "stale" {
			stale++
		}
		if (rdyOK && (x.Readiness == "off" || x.Readiness == "offline")) || (stateOK && x.State == "offline") {
			off++
		}
		if (stalledOK && x.Stalled) || (rdyOK && x.Readiness == "stall") {
			stalled++
		}
	}
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	line := func(label, value, tok string) {
		writeWrappedKV(&b, label, value, tok, w)
	}
	b.WriteString(" " + grammar.C("brt", "LANE READINESS") + grammar.C("mut", "  selected lane, blockers, safe next moves") + "\n")
	b.WriteString(" " + grammar.C("yel", ":intent resume") + grammar.C("mut", " · governed COMMAND route required") + "\n")
	b.WriteString(" " + grammar.C("mut", "no transcript · no PTY · no stdin bridge") + "\n")
	b.WriteString(rule + "\n")
	line("lane", role, airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR))
	line("ready", sessionReadinessLabel(s, m.AIR), rdyTok)
	attnVal, attnTok := fmt.Sprintf("%.2f", s.Attention), attentionToken(s.Attention)
	if m.AIR && s.AIR["attention"] != "ok" {
		attnVal, attnTok = "▒▒▒", "mut" // the value AND the heat hue disclose attention
	}
	line("attention", attnVal, attnTok)
	line("blocker", sessionFieldValueForAir(s, "blocker", m.AIR), blockTok)
	line("state", sessionFieldValueForAir(s, "state", m.AIR), stateTok)
	line("platform", sessionFieldValueForAir(s, "platform", m.AIR), "2nd")
	line("task", sessionFieldValueForAir(s, "claimed_task", m.AIR), "2nd")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "route binding") + "\n")
	routeID := sessionFieldValueForAir(s, "route_id", m.AIR)
	if strings.TrimSpace(routeID) == "" {
		routeID = "none"
	}
	mode := strings.TrimSpace(sessionFieldValueForAir(s, "mode", m.AIR))
	profile := strings.TrimSpace(sessionFieldValueForAir(s, "profile", m.AIR))
	modeProfile := strings.Trim(strings.TrimSpace(mode+"/"+profile), "/")
	if modeProfile == "" {
		modeProfile = "none"
	}
	evidenceRef := sessionFieldValueForAir(s, "route_evidence_ref", m.AIR)
	if strings.TrimSpace(evidenceRef) == "" {
		evidenceRef = "none"
	}
	binding, bindTok := routeBindingLabel(s.RouteBindingState), routeBindingToken(s.RouteBindingState)
	if m.AIR && s.AIR["route_binding_state"] != "ok" {
		binding, bindTok = "▒▒▒", "mut" // the binding VALUE and its hue disclose route_binding_state
	}
	line("route", routeID, bindTok)
	line("binding", binding, bindTok)
	line("mode/profile", modeProfile, "2nd")
	line("evidence", evidenceRef, "2nd")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "freshness") + "\n")
	outAge, outTok := fmt.Sprintf("%.1fs", s.OutputAgeS), ageToken(s.OutputAgeS)
	if m.AIR && s.AIR["output_age_s"] != "ok" {
		outAge, outTok = "▒▒▒", "mut"
	}
	relAge, relTok := fmt.Sprintf("%.1fs", s.RelayAgeS), ageToken(s.RelayAgeS)
	if m.AIR && s.AIR["relay_age_s"] != "ok" {
		relAge, relTok = "▒▒▒", "mut"
	}
	line("output age", outAge, outTok)
	line("relay age", relAge, relTok)
	line("tmux", sessionFieldValueForAir(s, "session", m.AIR), "pri")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "fleet context") + "\n")
	line("lanes", fmt.Sprintf("%d total", len(m.Sessions)), "pri")
	line("claim", fmt.Sprintf("%d ready", claim), countToken(claim))
	line("stale", fmt.Sprintf("%d stale", stale), countWarnToken(stale))
	line("stalled/off", fmt.Sprintf("%d stalled · %d off", stalled, off), countWarnToken(stalled+off))
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "session work surface") + "\n")
	line("templates", "{{sel.role}} · {{sel.claimed_task}} · {{focus}} · {{ring.0}}", "pri")
	line("preview verbs", ":intent resume · :intent handoff · :intent show-route · :intent open-trace", "yel")
	if m.SessionDetail.Role == s.Role && !m.SessionDetailDark {
		detail := m.SessionDetail
		taskID := sessionDetailFieldForAir(detail, "task_id", detail.Task.TaskID, m.AIR)
		status := sessionDetailFieldForAir(detail, "status", detail.Task.Status, m.AIR)
		mutation := sessionDetailFieldForAir(detail, "mutation_surface", detail.Task.MutationSurface, m.AIR)
		authority := sessionDetailFieldForAir(detail, "authority_case", detail.Task.AuthorityCase, m.AIR)
		parent := sessionDetailFieldForAir(detail, "parent_spec", detail.Task.ParentSpec, m.AIR)
		line("detail", fmt.Sprintf("task=%s · status=%s · mutation=%s · evidence=%d",
			taskID, status, mutation, sessionEvidenceTotal(detail)), "2nd")
		line("refs", sessionEvidenceKindSummary(detail), "2nd")
		line("evidence", sessionEvidencePosture(detail), "2nd")
		line("authority", fmt.Sprintf("case=%s · parent=%s · resume_ready=%t · blocked=%s",
			authority, parent, detail.Resume.Ready, strings.Join(detail.Resume.BlockedReasons, ",")), "yel")
	} else if m.SessionDetailDark {
		reason := "dark detail; no fabricated resume context"
		if strings.TrimSpace(m.SessionDetailError) != "" && !m.AIR {
			reason += " · " + m.SessionDetailError
		}
		line("detail", reason, "red")
	} else {
		line("detail", "press [Enter] for /session frontmatter/evidence refs; transcript candidates stay metadata-only", "mut")
	}
	line("drafts", "summarize session · prepare resume · draft handoff · explain blockers; all preview-only until coordinator/chat contract", "mut")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "constraints") + "\n")
	for _, c := range sessionConstraints(s, m.AIR) {
		b.WriteString(" " + c + "\n")
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "legal next") + "\n")
	b.WriteString(" " + grammar.C("yel", ":intent resume") + grammar.C("mut", " · handoff · show-route · governed COMMAND route required") + "\n")
	b.WriteString(" " + grammar.C("mut", "no transcript · no PTY · no stdin bridge") + "\n")
	return strings.TrimRight(b.String(), "\n")
}

func sessionStateToken(state string) string {
	switch state {
	case "active":
		return "grn"
	case "idle":
		return "yel"
	case "offline", "stalled":
		return "red"
	}
	return "mut"
}

func readinessPaneToken(readiness string) string {
	switch readiness {
	case "claim", "live":
		return "grn"
	case "idle", "stale":
		return "yel"
	case "stall", "off", "offline":
		return "red"
	}
	return "mut"
}

func sessionReadinessLabel(s grammar.Session, air bool) string {
	ready := sessionFieldValueForAir(s, "readiness", air)
	if s.Readiness == "claim" && ready == "claim" {
		return "claim-ready"
	}
	return ready
}

func sessionDetailFieldForAir(d grammar.SessionDetail, field, raw string, air bool) string {
	value := grammar.Redact(d.AIR, field, raw, air)
	if strings.TrimSpace(value) == "" {
		return "·"
	}
	return value
}

func sessionEvidenceTotal(d grammar.SessionDetail) int {
	if d.EvidenceSummary.Total > 0 {
		return d.EvidenceSummary.Total
	}
	return len(d.EvidenceRefs)
}

func sessionEvidenceKindSummary(d grammar.SessionDetail) string {
	if len(d.EvidenceSummary.ByKind) > 0 {
		keys := make([]string, 0, len(d.EvidenceSummary.ByKind))
		for key := range d.EvidenceSummary.ByKind {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s:%d", sessionEvidenceKindLabel(key), d.EvidenceSummary.ByKind[key]))
		}
		return strings.Join(parts, ",")
	}
	if len(d.EvidenceRefs) == 0 {
		return "none"
	}
	counts := map[string]int{}
	for _, ref := range d.EvidenceRefs {
		kind := strings.TrimSpace(ref.Kind)
		if kind == "" {
			kind = "unknown"
		}
		counts[kind]++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", sessionEvidenceKindLabel(key), counts[key]))
	}
	return strings.Join(parts, ",")
}

func sessionEvidenceKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case "cc_task_note":
		return "task-note"
	case "transcript_candidate":
		return "transcript"
	case "":
		return "unknown"
	default:
		return kind
	}
}

func sessionEvidencePosture(d grammar.SessionDetail) string {
	summary := d.EvidenceSummary
	if summary.Total == 0 && len(d.EvidenceRefs) > 0 {
		summary.Total = len(d.EvidenceRefs)
		summary.Privacy = "metadata-only"
	}
	privacy := strings.TrimSpace(summary.Privacy)
	if privacy == "" {
		privacy = "metadata-only"
	}
	return fmt.Sprintf("roots observed=%d missing=%d · truncated=%t · privacy=%s · raw_access=%t",
		summary.TranscriptRootsObserved, summary.TranscriptRootsMissing, summary.Truncated, privacy, summary.RawAccess)
}

func sessionRouteChip(s grammar.Session, air bool) string {
	route := strings.TrimSpace(sessionFieldValueForAir(s, "route_id", air))
	binding := routeBindingLabel(s.RouteBindingState)
	if route == "" {
		switch binding {
		case "", "unbound", "no-claim":
			return ""
		default:
			return binding
		}
	}
	if binding == "" || binding == "unbound" {
		return route
	}
	return binding + " " + route
}

func (m Model) sessionRouteBindingSummary() string {
	if len(m.Sessions) == 0 {
		return ""
	}
	counts := map[string]int{}
	routed := 0
	for _, s := range m.Sessions {
		if strings.TrimSpace(s.RouteID) == "" && strings.TrimSpace(s.RouteBindingState) == "" {
			continue
		}
		label := routeBindingLabel(s.RouteBindingState)
		if label == "" {
			label = "unbound"
		}
		if !m.AIR || s.AIR["route_binding_state"] == "ok" {
			counts[label]++ // a denied binding state must not be tallied on air (class membership leak)
		}
		if strings.TrimSpace(s.RouteID) != "" && (!m.AIR || s.AIR["route_id"] == "ok") {
			routed++ // route_id presence is a disclosure — gate it on air
		}
	}
	if len(counts) == 0 {
		return ""
	}
	return fmt.Sprintf("bound evidence:%d · %s", routed, compactCountMap(counts, 5))
}

func attentionToken(v float64) string {
	if v >= 0.75 {
		return "red"
	}
	if v >= 0.50 {
		return "yel"
	}
	return "pri"
}

func blockerToken(blocker string) string {
	blocker = strings.TrimSpace(blocker)
	if blocker == "" || blocker == "none" {
		return "grn"
	}
	return "red"
}

func ageToken(age float64) string {
	if age <= 0 {
		return "mut"
	}
	if age > 3600 {
		return "red"
	}
	if age > 900 {
		return "yel"
	}
	return "pri"
}

func countToken(n int) string {
	if n > 0 {
		return "grn"
	}
	return "mut"
}

func countWarnToken(n int) string {
	if n > 0 {
		return "red"
	}
	return "grn"
}

func sessionConstraints(s grammar.Session, air bool) []string {
	var out []string
	// A constraint DERIVED from a denied field must not air — its mere PRESENCE would disclose the
	// redacted readiness/state/blocker/stalled value through the back door (derived-channel leak).
	denied := func(field string) bool { return air && s.AIR[field] != "ok" }
	if strings.TrimSpace(s.Blocker) != "" && s.Blocker != "none" && !denied("blocker") {
		out = append(out, grammar.C("red", "blocked")+
			grammar.C("mut", " · "+sessionFieldValueForAir(s, "blocker", air)))
	}
	if s.Readiness == "claim" && !denied("readiness") {
		out = append(out, grammar.C("grn", "claim-ready")+
			grammar.C("mut", " · needs governed resume/claim route"))
	}
	if (s.Readiness == "stale" && !denied("readiness")) || (s.RelayAgeS > 3600 && !denied("relay_age_s")) {
		out = append(out, grammar.C("yel", "stale relay")+
			grammar.C("mut", " · verify before resume"))
	}
	if (s.State == "offline" && !denied("state")) ||
		((s.Readiness == "off" || s.Readiness == "offline") && !denied("readiness")) {
		out = append(out, grammar.C("red", "offline")+
			grammar.C("mut", " · no live session surface"))
	}
	if (s.Stalled && !denied("stalled")) || (s.Readiness == "stall" && !denied("readiness")) {
		out = append(out, grammar.C("red", "stalled")+
			grammar.C("mut", " · needs operator attention"))
	}
	if strings.TrimSpace(s.ClaimedTask) == "" && !denied("claimed_task") {
		out = append(out, grammar.C("mut", "no claimed task in current read model"))
	}
	if len(out) == 0 {
		out = append(out, grammar.C("grn", "no visible lane constraint in current read model"))
	}
	return out
}

func sessionPickRow(s grammar.Session, air bool, cur string) string {
	clip := func(v string, n int) string {
		if r := []rune(v); len(r) > n {
			return string(r[:n])
		}
		return v
	}
	fields := []struct{ key, field, val string }{
		{"r", "role", clip(s.Role, 14)}, {"p", "platform", s.Platform},
		{"d", "readiness", s.Readiness}, {"t", "state", s.State},
		{"b", "blocker", s.Blocker}, {"a", "attention", fmt.Sprintf("%.2f", s.Attention)},
		{"s", "session", clip(s.Session, 18)},
		{"c", "claimed_task", clip(s.ClaimedTask, 24)},
		{"u", "route_id", clip(s.RouteID, 24)},
		{"m", "mode", s.RouteMode},
		{"f", "profile", s.RouteProfile},
		{"g", "route_binding_state", s.RouteBindingState},
		{"e", "route_evidence_ref", clip(s.RouteEvidenceRef, 24)},
		{"o", "output_age_s", fmt.Sprintf("%.1f", s.OutputAgeS)},
		{"l", "relay_age_s", fmt.Sprintf("%.1f", s.RelayAgeS)},
	}
	out := grammar.C("brt", "▶ yank ")
	for _, f := range fields {
		v := f.val
		denied := air && s.AIR[f.field] != "ok"
		out += yankCell(f.key, f.field, v, cur, denied) + " "
	}
	return strings.TrimRight(out, " ")
}

func darkHint(reason string, air bool) string {
	line := "(spine dark — no fabricated data; check READ API/config)"
	if strings.TrimSpace(reason) != "" {
		if air {
			line += " · reason hidden on AIR"
		} else {
			line += " · " + reason
		}
	}
	return grammar.C("mut", line+"\n")
}

// taskBody windows the registry to the visible height, keeping the focused row in view, with a
// 1-col focus gutter (▌ marks the focused row).
func (m Model) taskBody(w, h int) string {
	if m.TasksDark {
		return m.contextLine() + "\n" + darkHint(m.TasksError, m.AIR)
	}
	if w >= 150 {
		return m.tasksWideBody(w, h)
	}
	return m.tasksListBody(w, h)
}

func (m Model) tasksWideBody(w, h int) string {
	leftW := 110
	if w < 220 {
		leftW = 94
	}
	if leftW > w-48 {
		leftW = w - 48
	}
	if leftW < 74 {
		return m.tasksListBody(w, h)
	}
	rightW := w - leftW - 1
	left := fitBlock(m.tasksListBody(leftW, h), leftW, h)
	right := fitBlockWithSlackFn(m.taskWorkDomainPane(rightW), rightW, h, func(rows int) []string {
		return m.slackRowsForPage(rightW, rows, slackSlotWideContext)
	})
	div := grammar.C("border", "│")
	out := make([]string, 0, h)
	for i := 0; i < h; i++ {
		out = append(out, left[i]+div+right[i])
	}
	return strings.Join(out, "\n")
}

func (m Model) tasksListBody(w, h int) string {
	visible := h - 2 // context line + header
	if visible < 1 {
		visible = 1
	}
	vt := m.visibleTasks()
	if len(vt) == 0 {
		return m.contextLine() + "\n" + m.emptyTasksBody()
	}
	off := m.scrollOffset(visible)
	memberSet := map[int]bool{} // class-selected rows (granularity g2) get a ▏ left-rail
	for _, mi := range m.Sel.Members {
		memberSet[mi] = true
	}
	var b strings.Builder
	b.WriteString(m.contextLine() + "\n")
	b.WriteString(" " + grammar.RenderTaskHeader() + "\n")
	for i := off; i < off+visible && i < len(vt); i++ {
		if m.Mode == ModeHint { // every visible row carries its teleport label in the gutter
			label := " "
			if li := i - off; li < len(hintAlphabet) {
				label = string(hintAlphabet[li])
			}
			b.WriteString(grammar.SelLabel(label) + grammar.RenderTaskRow(vt[i], m.AIR) + "\n")
			continue
		}
		switch {
		case i == m.Focus && m.Mode == ModeYank:
			// yank pick: the selectable FIELDS show their pick-keys ON the row — choose by LOOKING.
			b.WriteString(fitWidth(yankPickRow(vt[i], m.AIR, m.Sel.Field), w) + "\n")
		case i == m.Focus && m.Sel.Rank == RankField:
			// field cursor: the SELECTED field carries the sel swatch ON the row — steer with h/l.
			b.WriteString(fitWidth(fieldRow(vt[i], m.Sel.Field, m.AIR), w) + "\n")
		case i == m.Focus:
			// the SELECTED row — a bright full-width highlight bar (always visible)
			b.WriteString(grammar.C("yel", m.focusGlyph()) + focusBar(grammar.RenderTaskRow(vt[i], m.AIR), w-1) + "\n")
		default:
			gut := " "
			if memberSet[i] { // class member — left-rail marker (▏)
				gut = grammar.C("brt", "▏")
			}
			b.WriteString(gut + grammar.RenderTaskRow(vt[i], m.AIR) + "\n")
		}
	}
	return b.String()
}

func (m Model) taskWorkDomainPane(w int) string {
	t, ok := m.FocusedTask()
	source := "selected task, constraints, legal next moves"
	if m.sessionSplit() {
		s, hasSession := m.FocusedSession()
		if !hasSession {
			return grammar.C("mut", " work domain\n\n no selected session\n")
		}
		if claim := strings.TrimSpace(s.ClaimedTask); claim != "" {
			if linked, found := m.taskByID(claim); found {
				t, ok = linked, true
				source = "selected session claimed task, constraints, legal next moves"
			} else {
				return m.sessionTaskGapPane(w, s)
			}
		} else {
			return m.sessionTaskGapPane(w, s)
		}
	}
	if !ok {
		return grammar.C("mut", " work domain\n\n no selected task\n")
	}
	id := taskFieldValueForAir(t, "task_id", m.AIR)
	if strings.TrimSpace(id) == "" {
		id = "·"
	}
	crit := t.Criticality
	if crit == "" {
		crit = "ok"
	}
	ctok := airSeverityToken(crit, t.AIR, m.AIR)
	next := taskFieldValueForAir(t, "predicted_stage", m.AIR)
	if strings.TrimSpace(next) == "" {
		next = "·"
	}
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	line := func(label, value, tok string) {
		writeWrappedKV(&b, label, value, tok, w)
	}
	b.WriteString(" " + grammar.C("brt", "WORK DOMAIN") + grammar.C("mut", "  "+source) + "\n")
	b.WriteString(rule + "\n")
	line("task", id, "pri")
	if m.sessionSplit() {
		if s, ok := m.FocusedSession(); ok {
			line("source", sessionFieldValueForAir(s, "role", m.AIR), airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR))
		}
	}
	line("state", taskFieldValueForAir(t, "stage", m.AIR), ctok)
	line("was", taskFieldValueForAir(t, "prior_stage", m.AIR), "mut")
	line("next", next, airHue(nextToken(t.PredictedStage), t.AIR, "predicted_stage", m.AIR))
	line("crit", taskFieldValueForAir(t, "criticality", m.AIR), ctok)
	line("owner", taskFieldValueForAir(t, "owner", m.AIR), airHue(grammar.LaneToken(t.Owner), t.AIR, "owner", m.AIR))
	line("freshness", grammar.Redact(t.AIR, "freshness", fmt.Sprintf("%.2f", t.Freshness), m.AIR), airHue(freshnessToken(t.Freshness), t.AIR, "freshness", m.AIR))
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "authority") + "\n")
	line("case", grammar.Redact(t.AIR, "authority_case", t.AuthorityCase, m.AIR), "2nd")
	if m.AIR && t.AIR["no_go"] != "ok" {
		line("granted", "▒▒▒", "mut")
	} else if strings.TrimSpace(t.NoGo) == "" {
		line("granted", "none recorded", "mut")
	} else {
		line("granted", strings.Join(splitAuthPlain(t.NoGo), " · "), "grn")
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "constraints") + "\n")
	for _, c := range taskConstraints(t, m.AIR) {
		b.WriteString(" " + c + "\n")
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "relationships") + "\n")
	line("task edges", grammar.Redact(t.AIR, "rel_count", fmt.Sprintf("●%d", t.RelCount), m.AIR), relToken(t.RelCount))
	if t.RelCount == 0 && !(m.AIR && t.AIR["rel_count"] != "ok") {
		// derived-channel (class-c): on air, a denied rel_count must not disclose rel_count==0 through
		// the mere PRESENCE of this source-absent advisory — gate it on the field's AIR.
		b.WriteString(" " + grammar.C("mut", "task-edge source absent; do not infer graph neighborhood") + "\n")
	}
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "legal next") + "\n")
	b.WriteString(" " + grammar.C("yel", ":intent show-route") + grammar.C("mut", " · claim · close · governed COMMAND route required") + "\n")
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) taskByID(id string) (grammar.Task, bool) {
	for _, t := range m.Tasks {
		if t.TaskID == id {
			return t, true
		}
	}
	return grammar.Task{}, false
}

func (m Model) sessionTaskGapPane(w int, s grammar.Session) string {
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	line := func(label, value, tok string) {
		writeContextFactRow(&b, label, value, tok, w)
	}
	b.WriteString(" " + grammar.C("brt", "WORK DOMAIN") + grammar.C("mut", "  selected session claimed-task join") + "\n")
	b.WriteString(rule + "\n")
	line("source", sessionFieldValueForAir(s, "role", m.AIR), airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR))
	line("claimed", sessionFieldValueForAir(s, "claimed_task", m.AIR), "pri")
	line("matched", "0 tasks", "red")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "gap") + "\n")
	if strings.TrimSpace(s.ClaimedTask) == "" {
		b.WriteString(" " + grammar.C("mut", "selected session has no claimed task in current read model") + "\n")
	} else {
		b.WriteString(" " + grammar.C("mut", "claimed task is absent from the current task registry window") + "\n")
	}
	b.WriteString(" " + grammar.C("mut", "do not infer task state from the lane alone") + "\n")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("2nd", "legal next") + "\n")
	b.WriteString(" " + grammar.C("yel", ":intent show-route") + grammar.C("mut", " preview route/veto context") + "\n")
	return strings.TrimRight(b.String(), "\n")
}

func splitAuthPlain(noGo string) []string {
	var out []string
	for _, a := range strings.Split(noGo, ",") {
		a = strings.TrimSuffix(strings.TrimSpace(a), "_authorized")
		if a != "" {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return []string{"none recorded"}
	}
	return out
}

func taskConstraints(t grammar.Task, airOn bool) []string {
	var out []string
	// A constraint DERIVED from a denied field must not air — it would disclose the redacted value
	// (criticality / predicted_stage / freshness) through the back door (the derived-channel leak class).
	denied := func(field string) bool { return airOn && t.AIR[field] != "ok" }
	if t.PredictedStage == "hold" && !denied("predicted_stage") {
		out = append(out, grammar.C("red", "release blocked")+
			grammar.C("mut", " · predicted next stage is hold"))
	}
	if !denied("criticality") {
		switch t.Criticality {
		case "crit":
			out = append(out, grammar.C("red", "critical exception")+
				grammar.C("mut", " · inspect evidence before acting"))
		case "major":
			out = append(out, grammar.C("org", "major issue")+
				grammar.C("mut", " · prioritize review"))
		case "warn":
			out = append(out, grammar.C("yel", "watch")+
				grammar.C("mut", " · non-calm task state"))
		}
	}
	if t.Freshness <= 0.20 && !denied("freshness") {
		out = append(out, grammar.C("mut", "stale/absent event freshness"))
	}
	if strings.TrimSpace(t.AuthorityCase) == "" && !denied("authority_case") {
		out = append(out, grammar.C("mut", "authority case absent in task row"))
	}
	if len(out) == 0 {
		out = append(out, grammar.C("grn", "no visible constraint in current read model"))
	}
	return out
}

func nextToken(stage string) string {
	if stage == "hold" {
		return "red"
	}
	if strings.TrimSpace(stage) == "" {
		return "mut"
	}
	return "grn"
}

func freshnessToken(f float64) string {
	if f > 0.60 {
		return "grn"
	}
	if f > 0.20 {
		return "pri"
	}
	return "mut"
}

func relToken(n int) string {
	if n > 0 {
		return "blu"
	}
	return "mut"
}

func (m Model) emptyTasksBody() string {
	if strings.TrimSpace(m.Filter) != "" || m.CritFilter != "" {
		return grammar.C("mut", " 0 matching tasks — [Esc] clears filters\n")
	}
	return grammar.C("mut", " registry empty — waiting for /read/tasks\n")
}

func (m Model) scrollOffset(visible int) int {
	n := len(m.visibleTasks())
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

// fieldRow: at L3 the focused row shows its field VALUES with the SELECTED field in the sel swatch,
// others dimmed — the field cursor is visible ON the data; [h/l] steers it (navigate by looking).
func fieldRow(t grammar.Task, cur string, air bool) string {
	clip := func(s string, n int) string {
		if r := []rune(s); len(r) > n {
			return string(r[:n])
		}
		return s
	}
	fields := []struct{ field, val string }{
		{"task_id", clip(t.TaskID, 20)}, {"stage", shortStage2(t.Stage)}, {"owner", t.Owner},
		{"prior_stage", shortStage2(t.PriorStage)}, {"predicted_stage", t.PredictedStage},
		{"criticality", t.Criticality}, {"authority_case", clip(t.AuthorityCase, 14)},
	}
	out := grammar.C("brt", "▶ ")
	for _, f := range fields {
		v := f.val
		if air && t.AIR[f.field] != "ok" {
			v = "▒▒▒"
		} else if strings.TrimSpace(v) == "" {
			v = "·"
		}
		if f.field == cur {
			out += grammar.SelLabel(" "+v+" ") + " "
		} else {
			out += grammar.C("mut", v) + " "
		}
	}
	return strings.TrimRight(out, " ")
}

// yankPickRow: in ModeYank the focused row becomes a labeled field-picker IN PLACE — each yankable
// field shows its pick-key [i]/[s]/… next to its actual value, so the operator chooses by LOOKING at
// the data (not a separate menu). AIR-denied fields dim + redact (un-yankable, visible as such).
func yankPickRow(t grammar.Task, air bool, cur string) string {
	clip := func(s string, n int) string {
		if r := []rune(s); len(r) > n {
			return string(r[:n])
		}
		return s
	}
	fields := []struct{ key, field, val string }{
		{"i", "task_id", clip(t.TaskID, 18)}, {"s", "stage", shortStage2(t.Stage)},
		{"o", "owner", t.Owner}, {"w", "prior_stage", shortStage2(t.PriorStage)},
		{"n", "predicted_stage", t.PredictedStage}, {"c", "criticality", t.Criticality},
		{"a", "authority_case", clip(t.AuthorityCase, 14)},
	}
	out := grammar.C("brt", "▶ yank ")
	for _, f := range fields {
		v := f.val
		denied := air && t.AIR[f.field] != "ok"
		out += yankCell(f.key, f.field, v, cur, denied) + " "
	}
	return strings.TrimRight(out, " ")
}

// completionStrip: fish-style autocomplete — the candidate list is REVEALED explicitly and is
// NAVIGABLE ([Tab]/[↓] next, [⇧Tab]/[↑] prev), the current candidate carried in the sel swatch;
// [Enter] accepts it. Dynamic on the active selection (a `paste <value>` candidate leads when a
// field is selected). The full multi-row grid + sub-menus are the next step (see handoff §9).
// completionStrip — the fish-style navigable candidate line (the 2nd floor row in command mode).
// Candidates are revealed EXPLICITLY (chips), the highlighted one rides the sel swatch, a SUB-MENU
// node is marked `▸` (its accept descends), and the highlighted candidate's Detail trails as the
// description column. The input string carries the descent path, so this stays stateless.
func (m Model) completionStrip(w int) string {
	cands := m.completionTree()
	if len(cands) == 0 {
		return fitWidth(grammar.C("mut", "   (no match — keep typing or [Esc] cancel)"), w)
	}
	cur := m.CompIdx % len(cands)
	labelFor := func(c Candidate) string {
		label := c.Label
		if len(c.Sub) > 0 {
			label += "▸" // signifier: accepting this DESCENDS into a sub-menu
		}
		return label
	}
	var b strings.Builder
	b.WriteString(grammar.C("mut", "  "))
	for i, c := range cands {
		label := labelFor(c)
		if i == cur {
			b.WriteString(grammar.SelLabel(" "+label+" ") + " ")
		} else {
			b.WriteString(grammar.C("mut", label) + " ")
		}
	}
	if d := cands[cur].Detail; d != "" {
		b.WriteString(grammar.C("2nd", "  — "+d))
	}
	full := b.String()
	if ansi.StringWidth(full) <= w {
		return fitWidth(full, w)
	}

	leftHidden := cur
	rightHidden := len(cands) - cur - 1
	prefix := grammar.C("mut", fmt.Sprintf("  %d/%d ", cur+1, len(cands)))
	left := ""
	if leftHidden > 0 {
		left = grammar.C("mut", fmt.Sprintf("‹%d ", leftHidden))
	}
	right := ""
	if rightHidden > 0 {
		right = grammar.C("mut", fmt.Sprintf(" ›%d", rightHidden))
	}
	detail := ""
	if d := cands[cur].Detail; d != "" {
		detail = grammar.C("2nd", " — "+d)
	}
	staticW := ansi.StringWidth(prefix) + ansi.StringWidth(left) + ansi.StringWidth(right)
	detailBudget := 0
	if w-staticW > 28 && detail != "" {
		detailBudget = maxVisible(8, (w-staticW)/3)
		if detailBudget > ansi.StringWidth(detail) {
			detailBudget = ansi.StringWidth(detail)
		}
		staticW += detailBudget
	}
	labelBudget := maxVisible(6, w-staticW-2)
	label := clipRunes(labelFor(cands[cur]), labelBudget)
	row := prefix + left + grammar.SelLabel(" "+label+" ") + right
	if detailBudget > 0 {
		row += ansi.Truncate(detail, detailBudget, "…")
	}
	return fitWidth(row, w)
}

// Z2b — context rail: the focused row unfolded. Page-aware — a task's seven dimensions on :tasks, a
// coord event's anatomy on :events — so the rail always describes what the cursor is actually on.
func (m Model) viewRail(w int) string {
	switch m.Page {
	case PageEvents:
		return m.eventRail(w)
	case PageTasks:
		return m.taskRail(w)
	case PageSessions:
		return m.sessionRail(w)
	default:
		return grammar.C("mut", " (this page has no row selection)")
	}
}

// eventRail: the focused coord event, unfolded + AIR-gated — the :events analogue of taskRail.
func (m Model) eventRail(w int) string {
	ev, ok := m.FocusedEvent()
	if !ok {
		return grammar.C("mut", " (no events — spine quiet)")
	}
	rule := grammar.C("border", " "+strings.Repeat("─", w-2))
	line := func(label, field, val, tok string) string {
		if m.AIR && ev.AIR[field] != "ok" {
			val, tok = "▒▒▒", "mut"
		}
		if strings.TrimSpace(val) == "" {
			val, tok = "·", "mut"
		}
		return " " + grammar.C("mut", fmt.Sprintf("%-6s", label)) + grammar.C(tok, val)
	}
	subj := grammar.Redact(ev.AIR, "subject", ev.Subject, m.AIR)
	if r := []rune(subj); len(r) > w-4 {
		subj = string(r[:w-4])
	}
	var b strings.Builder
	b.WriteString(" " + grammar.C("brt", "▶ ") + grammar.C("2nd", fmt.Sprintf("Z2▸:events▸row %d/%d", m.EFocus+1, len(m.Events))) + "\n")
	b.WriteString(" " + grammar.C("mut", "the selected event, unfolded ↓") + "\n")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("brt", "◆ "+subj) + "\n")
	b.WriteString(rule + "\n")
	b.WriteString(line("when", "ts", ev.TS, "pri") + "\n")
	b.WriteString(line("kind", "kind", ev.Kind, "blu") + "\n")
	b.WriteString(line("who", "actor", ev.Actor, grammar.LaneToken(ev.Actor)) + "\n")
	b.WriteString(line("what", "summary", ev.Summary, "2nd") + "\n")
	b.WriteString(line("score", "score", fmt.Sprintf("%.2f", ev.Score), "pri") + "\n")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("mut", "[y]ank a field · [j/k] move") + "\n")
	return b.String()
}

// taskRail — the focused registry task unfolded into its seven dimensions, plus the relationship web
// and mini :dynamics (structured-silence until their data sources land).
func (m Model) taskRail(w int) string {
	t, ok := m.FocusedTask()
	if !ok {
		return grammar.C("mut", " (no tasks — registry empty)")
	}
	id := grammar.Redact(t.AIR, "task_id", t.TaskID, m.AIR) // the rail honors the on-air lens too (PII)
	if r := []rune(id); len(r) > w-3 {
		id = string(r[:w-3])
	}
	ctok := airSeverityToken(t.Criticality, t.AIR, m.AIR)
	ntok := "grn"
	if t.PredictedStage == "hold" {
		ntok = "red"
	}
	// line gates each unfolded dimension through the AIR lens by its field key — a denied field
	// blanks to ▒▒▒ (default-deny), so the rail can never leak a private value on the stream.
	line := func(label, field, val, tok string) string {
		if m.AIR && t.AIR[field] != "ok" {
			val, tok = "▒▒▒", "mut"
		}
		return " " + grammar.C("mut", fmt.Sprintf("%-6s", label)) + grammar.C(tok, val)
	}
	rule := grammar.C("border", " "+strings.Repeat("─", w-2))
	var b strings.Builder
	// self-context = the lattice BREADCRUMB: WHAT this panel is + the part's place in the whole. The
	// address descends as the cursor descends (row → field), so you always know where you are.
	crumb := fmt.Sprintf("Z2▸:tasks▸row %d/%d", m.Focus+1, len(m.visibleTasks()))
	if m.Sel.Rank == RankField {
		crumb += "▸field " + m.Sel.Field
	}
	b.WriteString(" " + grammar.C("brt", "▶ ") + grammar.C("2nd", crumb) + "\n")
	b.WriteString(" " + grammar.C("mut", "the selected row, unfolded ↓") + "\n")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("brt", "◆ "+id) + "\n")
	b.WriteString(rule + "\n")
	b.WriteString(line("state", "stage", orDash(shortStage2(t.Stage)), ctok) + "\n")
	b.WriteString(line("was", "prior_stage", orDash(shortStage2(t.PriorStage)), "mut") + "\n")
	b.WriteString(line("next", "predicted_stage", orDash(t.PredictedStage), ntok) + "\n")
	b.WriteString(line("crit", "criticality", orDash(t.Criticality), ctok) + "\n")
	b.WriteString(line("who", "owner", orDash(t.Owner), grammar.LaneToken(t.Owner)) + "\n")
	b.WriteString(line("fresh", "freshness", fmt.Sprintf("%.2f", t.Freshness), "pri") + "\n")
	b.WriteString(line("rel", "rel_count", fmt.Sprintf("●%d", t.RelCount), "blu") + "\n")
	b.WriteString(rule + "\n")
	// authorizations granted so far (situates WHY a task is or isn't release-ready)
	b.WriteString(" " + grammar.C("mut", "granted") + "\n")
	if m.AIR && t.AIR["no_go"] != "ok" {
		b.WriteString("  " + grammar.C("mut", "▒▒▒") + "\n") // the grant set is private on-air
	} else if strings.TrimSpace(t.NoGo) == "" {
		b.WriteString("  " + grammar.C("mut", "(none granted yet)") + "\n")
	} else {
		for _, g := range splitAuth(t.NoGo) {
			b.WriteString("  " + grammar.C("grn", "✓ ") + grammar.C("mut", g) + "\n")
		}
	}
	if t.AuthorityCase != "" {
		b.WriteString(" " + grammar.C("mut", "case ") +
			grammar.C("2nd", grammar.Redact(t.AIR, "authority_case", t.AuthorityCase, m.AIR)) + "\n")
	}
	b.WriteString(rule + "\n")
	b.WriteString(grammar.C("2nd", " relationships") + "\n")
	b.WriteString(grammar.C("mut", " (no task-edge source yet)") + "\n")
	b.WriteString(rule + "\n")
	b.WriteString(grammar.C("2nd", " :dynamics neighborhood") + "\n")
	b.WriteString(grammar.C("mut", " (INC-6 mini-map)") + "\n")
	return b.String()
}

func (m Model) sessionRail(w int) string {
	s, ok := m.FocusedSession()
	if !ok {
		return grammar.C("mut", " (no sessions — roster empty)")
	}
	line := func(label, field, val, tok string) string {
		if m.AIR && s.AIR[field] != "ok" {
			val, tok = "▒▒▒", "mut"
		}
		if strings.TrimSpace(val) == "" {
			val, tok = "·", "mut"
		}
		return " " + grammar.C("mut", fmt.Sprintf("%-9s", label)) + grammar.C(tok, val)
	}
	role := grammar.Redact(s.AIR, "role", s.Role, m.AIR)
	if r := []rune(role); len(r) > w-4 {
		role = string(r[:w-4])
	}
	stateTok := "mut"
	switch s.State {
	case "active":
		stateTok = "grn"
	case "idle":
		stateTok = "yel"
	case "offline", "stalled":
		stateTok = "red"
	}
	rdyTok := "mut"
	switch s.Readiness {
	case "claim", "live":
		rdyTok = "grn"
	case "idle", "stale":
		rdyTok = "yel"
	case "stall", "off", "offline":
		rdyTok = "red"
	}
	blockTok := "grn"
	if strings.TrimSpace(s.Blocker) != "" && s.Blocker != "none" {
		blockTok = "red"
	}
	rule := grammar.C("border", " "+strings.Repeat("─", w-2))
	var b strings.Builder
	b.WriteString(" " + grammar.C("brt", "▶ ") + grammar.C("2nd", fmt.Sprintf("Z2▸:sessions▸row %d/%d", m.SFocus+1, len(m.Sessions))) + "\n")
	b.WriteString(" " + grammar.C("mut", "selected lane, cutover hotlist ↓") + "\n")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("brt", "◆ "+role) + "\n")
	b.WriteString(rule + "\n")
	b.WriteString(line("ready", "readiness", s.Readiness, rdyTok) + "\n")
	b.WriteString(line("attn", "attention", fmt.Sprintf("%.2f", s.Attention), "pri") + "\n")
	b.WriteString(line("blocker", "blocker", s.Blocker, blockTok) + "\n")
	b.WriteString(line("state", "state", s.State, stateTok) + "\n")
	b.WriteString(line("plat", "platform", s.Platform, "2nd") + "\n")
	b.WriteString(line("tmux", "session", s.Session, "pri") + "\n")
	b.WriteString(line("output", "output_age_s", fmt.Sprintf("%.1fs", s.OutputAgeS), "mut") + "\n")
	b.WriteString(line("relay", "relay_age_s", fmt.Sprintf("%.1fs", s.RelayAgeS), "mut") + "\n")
	b.WriteString(rule + "\n")
	b.WriteString(line("task", "claimed_task", s.ClaimedTask, "2nd") + "\n")
	b.WriteString(rule + "\n")
	b.WriteString(" " + grammar.C("mut", "[Enter] detail · [r] resume-intent · [y]ank · [j/k] move") + "\n")
	b.WriteString(" " + grammar.C("mut", "no transcript · no PTY · no dispatch") + "\n")
	return b.String()
}

// Z3 — command/status floor: row1 = bezel + keys + lens; row2 = the command-as-effect line.
// In command mode the floor is given over to the completion experience: row1 = the prompt + input
// (with the navigation legend), row2 = the navigable candidate strip.
// verbMenuFloor renders the object-verb menu (ModeVerbMenu): the focused task's verbs — legal ones lit
// + pickable, illegal ones dimmed. Verbs attach to the OBJECT (state-legal only), never to memory; a
// pick pre-seeds the governed COMMAND preview (the cockpit never mints authority).
func (m Model) verbMenuFloor(w int) string {
	t, ok := m.FocusedTask()
	if !ok {
		return grammar.C("mut", " no focused task") + "\n"
	}
	id := grammar.Redact(t.AIR, "task_id", t.TaskID, m.AIR)
	var b strings.Builder
	b.WriteString(" " + grammar.C("brt", "VERBS") + grammar.C("mut", " on ") + grammar.C("pri", clipRunes(id, 28)) +
		grammar.C("mut", "  (legal only · pick to preview · [Esc])") + "  ")
	for _, vb := range grammar.TaskVerbs(t) {
		if vb.Legal {
			b.WriteString(grammar.C("yel", "["+vb.Key+"]") + grammar.C("pri", " "+vb.Name+"  "))
		} else {
			b.WriteString(grammar.C("mut", " "+vb.Name+"  "))
		}
	}
	return clipRunes(b.String(), w) + "\n" + grammar.C("mut", " verbs route through the governed COMMAND surface — the cockpit never mints authority")
}

func (m Model) viewFloor(w int) string {
	if m.Mode == ModeCommand {
		prompt := inputPromptLine(":", m.commandInputDisplay(), "[Tab/↓]next · [→]fill · [↵]accept · [Esc]cancel", w)
		return prompt + "\n" + m.completionStrip(w)
	}
	if m.Mode == ModeFilter { // same fish-style candidate strip as the command line
		prompt := inputPromptLine("/", m.Filter, fmt.Sprintf("%d match · [Tab/↓]ids · [→]fill · [↵]keep · [Esc]clear", len(m.visibleTasks())), w)
		return prompt + "\n" + m.completionStrip(w)
	}
	if m.Mode == ModeVerbMenu { // the object-verb menu — state-legal governed verbs on the focused task
		return m.verbMenuFloor(w)
	}
	lens := grammar.C("pri", "● LOCAL")
	if m.AIR {
		lens = grammar.C("fch", "▮ ON-AIR · allowlist")
	}
	focus := grammar.C("mut", "—")
	if m.sessionSplit() {
		if s, ok := m.FocusedSession(); ok {
			ref := grammar.Redact(s.AIR, "role", s.Role, m.AIR)
			if r := []rune(ref); len(r) > 24 {
				ref = string(r[:24])
			}
			rel := m.splitRelation()
			if rel.Reactive() || m.Mode == ModeYank {
				op := " -> "
				if m.Mode == ModeYank && !rel.Reactive() {
					op = " · "
				}
				focus = grammar.C("brt", ref) + grammar.C("mut", op+clipRunes(rel.Target, 18))
			} else {
				name := "context"
				if wnd, ok := windowForPage(m.Page); ok {
					name = ":" + wnd.ID
				}
				focus = grammar.C("brt", name) + grammar.C("mut", " · "+rel.Join+" · anchor "+clipRunes(ref, 18))
			}
		}
	} else {
		switch m.Page { // the focus line reflects the CURRENT page's cursor, not always a task
		case PageEvents:
			if ev, ok := m.FocusedEvent(); ok {
				s := grammar.Redact(ev.AIR, "subject", ev.Subject, m.AIR)
				if r := []rune(s); len(r) > 24 {
					s = string(r[:24])
				}
				focus = grammar.C("brt", s)
			}
		case PageTasks:
			if t, ok := m.FocusedTask(); ok {
				fid := grammar.Redact(t.AIR, "task_id", t.TaskID, m.AIR) // the focus line honors AIR too
				if r := []rune(fid); len(r) > 24 {
					fid = string(r[:24])
				}
				focus = grammar.C("brt", fid)
			}
		case PageSessions:
			if s, ok := m.FocusedSession(); ok {
				ref := grammar.Redact(s.AIR, "role", s.Role, m.AIR)
				if r := []rune(ref); len(r) > 24 {
					ref = string(r[:24])
				}
				focus = grammar.C("brt", ref)
			}
		case PageCaps:
			if c, ok := m.FocusedCapabilityRow(); ok {
				ref := c.Name
				if r := []rune(ref); len(r) > 24 {
					ref = string(r[:24])
				}
				focus = grammar.C("brt", ref)
			}
		case PageCommands:
			if v, ok := m.FocusedCommand(); ok {
				focus = grammar.C("brt", clipRunes(v.name, 24))
			}
		case PageWindows:
			if wnd, ok := m.FocusedWindow(); ok {
				focus = grammar.C("brt", clipRunes(wnd.ID, 24))
			}
		case PageSurfaces:
			if surf, ok := m.FocusedSurface(); ok {
				focus = grammar.C("brt", clipRunes(surf.ID, 24))
			}
		case PageDomains:
			if row, ok := m.FocusedDomainRow(); ok {
				focus = grammar.C("brt", clipRunes(domainRowFieldForAir(row, "domain_id", m.AIR), 24))
			} else if d, ok := m.FocusedDomain(); ok {
				focus = grammar.C("brt", clipRunes(d.ID, 24))
			}
		case PageLifecycles:
			if row, ok := m.FocusedLifecycleRow(); ok {
				focus = grammar.C("brt", clipRunes(lifecycleRowFieldForAir(row, "lifecycle_id", m.AIR), 24))
			} else if fb, ok := m.FocusedLifecycleFallback(); ok {
				focus = grammar.C("brt", clipRunes(fb.ID, 24))
			}
		case PageEpistemics:
			if row, ok := m.FocusedEpistemicRow(); ok {
				focus = grammar.C("brt", clipRunes(row.Subject, 24))
			}
		default:
			name, _, _ := m.pageMeta()
			focus = grammar.C("brt", ":"+name)
		}
	}
	globals := grammar.C("yel", "[:]") + "cmd "
	if m.Mode == ModeYank {
		globals += grammar.C("yel", "[[/]]") + "win "
	} else if m.sessionSplit() {
		globals += grammar.C("yel", "[←/→]") + "ctx "
	} else {
		globals += grammar.C("yel", "[←/→]") + "win "
	}
	globals += grammar.C("yel", "[|]") + "split " +
		grammar.C("yel", "[?]") + "legend " + grammar.C("yel", "[a]") + "AIR " +
		grammar.C("yel", "[q]") + "quit"
	prefix := " " + globals + grammar.C("mut", " │ focus ") + focus + grammar.C("mut", " │ ")
	suffix := grammar.C("mut", " │ ") + lens
	actionBudget := w - ansi.StringWidth(prefix) - ansi.StringWidth(suffix)
	if actionBudget < 0 {
		actionBudget = 0
	}
	r1 := prefix + m.floorActions(actionBudget) + suffix
	var r2 string
	switch {
	case m.Mode == ModeHint:
		r2 = grammar.C("brt", " ▶ jump/select") + grammar.C("mut", " — a row letter (gutter) teleports · O/W/M/C (counts) filters by class · [Esc]")
	case m.Mode == ModeYank:
		r2 = grammar.C("brt", " ▶ yank field ") + grammar.SelLabel(" "+m.Sel.Field+" ") +
			grammar.C("mut", "  · [j/k] rows · [Tab/←/→] fields · [Enter/y] yank · letters jump+yank · [Esc]")
	case m.Sel.Rank == RankField:
		t, _ := m.FocusedTask()
		r2 = grammar.C("brt", " ▶ field ") + grammar.SelLabel(" "+m.Sel.Field+" ") +
			grammar.C("pri", "  = "+taskFieldValueForAir(t, m.Sel.Field, m.AIR)) +
			grammar.C("mut", "  · [h/l] move · [y] yank this · [Tab] back to rows")
	case m.Status != "":
		r2 = grammar.C("blu", ":") + " " + grammar.C("mut", clipRunes(m.Status, maxVisible(8, w-36))) +
			grammar.C("mut", " · press [:] command line")
	default:
		r2 = grammar.C("blu", ":") + grammar.C("mut", " press [:] to open the command line — type a verb, [Tab] completes")
	}
	return fitWidth(r1, w) + "\n" + fitWidth(r2, w)
}

func inputPromptLine(mark, input, hint string, w int) string {
	prefix := grammar.C("blu", " "+mark) + " "
	cursor := grammar.C("brt", "█")
	suffix := grammar.C("mut", "   "+hint)
	prefixW := ansi.StringWidth(prefix)
	cursorW := ansi.StringWidth(cursor)
	minInput := 16
	if room := w - prefixW - cursorW; room < minInput {
		minInput = room
	}
	if minInput < 0 {
		minInput = 0
	}
	suffixW := w - prefixW - cursorW - minInput
	if suffixW < 0 {
		suffixW = 0
	}
	if ansi.StringWidth(suffix) > suffixW {
		if suffixW == 0 {
			suffix = ""
		} else {
			suffix = ansi.Truncate(suffix, suffixW, "…")
		}
	}
	reserved := prefixW + cursorW + ansi.StringWidth(suffix)
	inputBudget := w - reserved
	if inputBudget < 0 {
		inputBudget = 0
	}
	display := input
	if ansi.StringWidth(display) > inputBudget {
		display = tailRunes(display, inputBudget)
	}
	return fitWidth(prefix+display+cursor+suffix, w)
}

func tailRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return "…" + string(r[len(r)-n+1:])
}

func (m Model) commandInputDisplay() string {
	if !m.AIR {
		return m.Input
	}
	fields := strings.Fields(m.Input)
	if len(fields) == 0 {
		return m.Input
	}
	verb := fields[0]
	// note = free text; a governed object-verb (arm/rework/refute/close/resume) carries a sensitive
	// TARGET id as its argument. Redact the argument on air, keep the verb (structural). m.Input (the
	// executable buffer) is unchanged — only its on-air RENDERING is redacted.
	_, governed := governedVerbSpecs[verb]
	if (verb == "note" || verb == "n" || governed) && len(m.Input) > len(verb) {
		return verb + " ▒▒▒"
	}
	return m.Input
}

func (m Model) floorActions(w int) string {
	var parts []string
	if m.sessionSplit() {
		if len(m.Sessions) > 0 {
			compact := w <= 120
			rel := m.splitRelation()
			parts = append(parts, m.splitControlCueTexts(rel, splitCueTextOptions{IncludeScroll: true, Compact: compact, Tight: true, IncludeScrollLabel: false, IncludeContext: false, Color: true, IncludeSourceVerbs: true})...)
			if rel.TargetScrollable && m.referenceScrollMax() > 0 && m.Mode != ModeYank && !compact {
				parts = append(parts, grammar.C("mut", m.referenceScrollLabel()))
			}
		}
		return fitFloorActionParts(parts, w)
	}
	switch m.Page {
	case PageTasks:
		if m.pageRows() > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+"select",
				grammar.C("yel", "[↵]")+"inspect",
				grammar.C("yel", "[y]")+"ank",
			)
		}
	case PageEvents:
		if m.pageRows() > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+"select",
				grammar.C("yel", "[y]")+"ank",
			)
		}
	case PageSessions:
		if m.pageRows() > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+"select",
				grammar.C("yel", "[↵]")+"detail",
				grammar.C("yel", "[r]")+"resume-intent",
				grammar.C("yel", "[y]")+"ank",
			)
		}
	case PageSessionTurns:
		if m.pageRows() > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+fmt.Sprintf("turn %d/%d", m.TurnFocus+1, len(m.TurnLadder)),
				grammar.C("yel", "[g/G]")+"first/last",
				grammar.C("yel", "[y]")+"ank",
			)
		}
	case PageIntake:
		if n := len(m.visibleIntakeRows()); n > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+fmt.Sprintf("bucket %d/%d", m.IFocus+1, n),
				grammar.C("yel", "[s/S]")+"source",
				grammar.C("yel", "[↵]")+"detail",
				grammar.C("yel", "[E]")+"evidence",
			)
		}
		if m.referenceScrollMax() > 0 {
			parts = append(parts, grammar.C("mut", m.referenceScrollLabel()))
		}
	case PageIntent:
		if n := len(lookupIntentArgs()); n > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+fmt.Sprintf("intent target %d/%d", m.IntentFocus+1, n),
				grammar.C("yel", "[↵]")+"preview",
			)
		}
		if m.referenceScrollMax() > 0 {
			parts = append(parts, grammar.C("mut", m.referenceScrollLabel()))
		}
	case PageCaps:
		if m.pageRows() > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+"capability",
				grammar.C("yel", "[g/G]")+"first/last",
			)
			if m.referenceScrollMax() > 0 {
				parts = append(parts, grammar.C("mut", m.referenceScrollLabel()))
			}
		}
	case PageCommands:
		if m.pageRows() > 0 {
			selection := fmt.Sprintf("command %d/%d", m.CommandFocus+1, len(verbs))
			parts = append(parts,
				grammar.C("yel", "[j/k]")+selection,
				grammar.C("yel", "[g/G]")+"first/last",
			)
			if m.referenceScrollMax() > 0 {
				parts = append(parts, grammar.C("mut", m.referenceScrollLabel()))
			}
		}
	case PageWindows:
		if m.pageRows() > 0 {
			selection := fmt.Sprintf("window %d/%d", m.WindowFocus+1, len(registeredWindows()))
			parts = append(parts,
				grammar.C("yel", "[j/k]")+selection,
				grammar.C("yel", "[g/G]")+"first/last",
			)
			if m.referenceScrollMax() > 0 {
				parts = append(parts, grammar.C("mut", m.referenceScrollLabel()))
			}
		}
	case PageSurfaces:
		if m.pageRows() > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+"surface",
				grammar.C("yel", "[g/G]")+"first/last",
			)
		}
	case PageDomains:
		if m.pageRows() > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+"domain",
				grammar.C("yel", "[g/G]")+"first/last",
			)
		}
	case PageLifecycles:
		if m.pageRows() > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+"lifecycle",
				grammar.C("yel", "[g/G]")+"first/last",
			)
		}
	case PageEpistemics:
		if m.pageRows() > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+"evidence",
				grammar.C("yel", "[g/G]")+"first/last",
			)
			if m.referenceScrollMax() > 0 {
				parts = append(parts, grammar.C("mut", m.referenceScrollLabel()))
			}
		}
	case PageLoops:
		if n := len(m.loopRows()); n > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+fmt.Sprintf("loop %d/%d", m.LoopFocus+1, n),
				grammar.C("yel", "[g/G]")+"first/last",
				grammar.C("mut", "computed no-sim"),
			)
		} else {
			parts = append(parts, grammar.C("mut", "no causal loops"))
		}
	case PageAxes:
		if n := len(grammar.Axes()); n > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+fmt.Sprintf("axis %d/%d", m.AxisFocus+1, n),
				grammar.C("yel", "[g/G]")+"first/last",
				grammar.C("mut", "five-tuple contracts"),
			)
		} else {
			parts = append(parts, grammar.C("mut", "no case-role axes"))
		}
	case PageIdentity:
		if n := len(m.identityRoster()); n > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+fmt.Sprintf("identity %d/%d", m.IdentityFocus+1, n),
				grammar.C("yel", "[g/G]")+"first/last",
				grammar.C("mut", "derived roster"),
			)
		} else {
			parts = append(parts, grammar.C("mut", "no principals"))
		}
	case PageRelational:
		if n := len(m.consentFacets()); n > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+fmt.Sprintf("consent facet %d/%d", m.RelationalFocus+1, n),
				grammar.C("yel", "[g/G]")+"first/last",
				grammar.C("mut", "access-control posture"),
			)
		} else {
			parts = append(parts, grammar.C("mut", "no consent facets"))
		}
	default:
		if m.Page == PageDynamics {
			if m.pageRows() > 0 {
				selection := fmt.Sprintf("map %d/%d", m.DynFocus+1, len(m.dynamicsFocusRows()))
				parts = append(parts,
					grammar.C("yel", "[j/k]")+selection,
					grammar.C("yel", "[g/G]")+"first/last",
					grammar.C("yel", "[E]")+"epistemics",
				)
			}
			parts = append(parts,
				grammar.C("yel", "[,/.]")+"scale",
				grammar.C("mut", dynScaleName(m.DynScale)),
			)
			if m.referenceScrollMax() > 0 {
				parts = append(parts,
					grammar.C("yel", "[J/K]")+"scroll",
					grammar.C("mut", m.referenceScrollLabel()),
				)
			}
		} else if m.Page == PageIntake {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+"bucket",
				grammar.C("yel", "[s/S]")+"source",
				grammar.C("yel", "[↵]")+"detail",
			)
		} else if m.isReferencePage() && m.referenceScrollMax() > 0 {
			parts = append(parts,
				grammar.C("yel", "[j/k]")+"scroll",
				grammar.C("mut", m.referenceScrollLabel()),
				grammar.C("yel", "[g/G]")+"top/bottom",
			)
		} else {
			parts = append(parts, grammar.C("mut", "reference page"))
		}
	}
	if len(parts) == 0 {
		return grammar.C("mut", "no rows")
	}
	return fitFloorActionParts(parts, w)
}

func fitFloorActionParts(parts []string, w int) string {
	if w <= 0 || len(parts) == 0 {
		return ""
	}
	out := ""
	for _, part := range parts {
		if strings.TrimSpace(ansi.Strip(part)) == "" {
			continue
		}
		candidate := part
		if out != "" {
			candidate = out + " " + part
		}
		if ansi.StringWidth(candidate) <= w {
			out = candidate
		}
	}
	return out
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

type slackRowsFn func(maxRows int) []string

func fitBlockWithSlackFn(s string, w, h int, rowsFor slackRowsFn) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		lines = nil
	}
	slackRows := h - len(lines)
	if slackRows >= 3 && rowsFor != nil {
		slack := rowsFor(slackRows)
		lines = append(lines, firstLines(strings.Join(slack, "\n"), slackRows)...)
	}
	return fitBlock(strings.Join(lines, "\n"), w, h)
}

func fitBlockWithSlack(s string, w, h int, slack []string) []string {
	return fitBlockWithSlackFn(s, w, h, func(int) []string { return slack })
}

func fitBlockWithOverflow(s string, w, h int, label string) []string {
	lines := strings.Split(s, "\n")
	out := fitBlock(s, w, h)
	if h > 0 && len(lines) > h {
		hidden := len(lines) - h + 1
		out[h-1] = fitWidth(" "+grammar.C("mut", fmt.Sprintf("… %d %s rows hidden; taller frame", hidden, label)), w)
	}
	return out
}

func fitBlockWithOverflowAndSlackFn(s string, w, h int, label string, rowsFor slackRowsFn) []string {
	lines := strings.Split(s, "\n")
	if h > 0 && len(lines) > h {
		return fitBlockWithOverflow(s, w, h, label)
	}
	return fitBlockWithSlackFn(s, w, h, rowsFor)
}

func fitBlockWithOverflowAndSlack(s string, w, h int, label string, slack []string) []string {
	return fitBlockWithOverflowAndSlackFn(s, w, h, label, func(int) []string { return slack })
}

type slackSlot string

const (
	slackSlotWideContext    slackSlot = "wide-context"
	slackSlotSessionContext slackSlot = "split-context"
	slackSlotReferenceMain  slackSlot = "reference-main"
)

type trustSignal struct {
	Authority string
	Freshness string
	Support   string
	Recency   string
	Token     string
}

func (t trustSignal) render() string {
	return fmt.Sprintf("auth=%s · fresh=%s · support=%s · recent=%s",
		firstNonEmpty(t.Authority, "unknown"),
		firstNonEmpty(t.Freshness, "unknown"),
		firstNonEmpty(t.Support, "unknown"),
		firstNonEmpty(t.Recency, "unknown"))
}

func trustToken(t trustSignal) string {
	text := strings.ToLower(t.render())
	switch {
	case strings.Contains(text, "dark"), strings.Contains(text, "missing"), strings.Contains(text, "failed"), strings.Contains(text, "forbidden"):
		return "red"
	case strings.Contains(text, "support-only"), strings.Contains(text, "preview"), strings.Contains(text, "observation-only"), strings.Contains(text, "metadata-only"), strings.Contains(text, "non-authoritative"):
		return "yel"
	case strings.Contains(text, "authoritative"), strings.Contains(text, "observed"), strings.Contains(text, "live"), strings.Contains(text, "<5m"), strings.Contains(text, "<1h"):
		return "grn"
	default:
		return "2nd"
	}
}

func freshnessScoreBucket(score float64) string {
	switch {
	case score >= 0.85:
		return "live"
	case score >= 0.55:
		return "recent"
	case score > 0:
		return "stale"
	default:
		return "unknown"
	}
}

func ageBucketFromSeconds(age float64) string {
	switch {
	case age <= 0:
		return "unknown"
	case age < 300:
		return "<5m"
	case age < 3600:
		return "<1h"
	case age < 21600:
		return "<6h"
	case age < 86400:
		return "<1d"
	default:
		return ">1d"
	}
}

func bestAgeBucket(values ...string) string {
	order := []string{"<5m", "<1h", "<6h", "<1d", ">1d", "stale", "missing", "dark"}
	for _, want := range order {
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), strings.ToLower(want)) {
				return want
			}
		}
	}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "unknown"
}

func eventRecencyBucket(events []grammar.Event) string {
	var newest time.Time
	for _, ev := range events {
		ts := strings.TrimSpace(ev.TS)
		if ts == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			continue
		}
		if newest.IsZero() || parsed.After(newest) {
			newest = parsed
		}
	}
	if newest.IsZero() {
		return "unknown"
	}
	return ageBucketFromSeconds(time.Since(newest).Seconds())
}

func capabilitySourceAge(sources []grammar.CapabilitySource) string {
	values := make([]string, 0, len(sources))
	for _, src := range sources {
		values = append(values, src.AgeBucket)
	}
	return bestAgeBucket(values...)
}

func dynamicsSourceAge(sources []grammar.DynamicsSource) string {
	values := make([]string, 0, len(sources))
	for _, src := range sources {
		values = append(values, src.AgeBucket)
	}
	return bestAgeBucket(values...)
}

func intakeSourceAge(sources []grammar.IntakeSource) string {
	values := make([]string, 0, len(sources))
	for _, src := range sources {
		values = append(values, src.AgeBucket)
	}
	return bestAgeBucket(values...)
}

func gateSourceAge(sources []grammar.GateSource) string {
	values := make([]string, 0, len(sources))
	for _, src := range sources {
		values = append(values, src.AgeBucket)
	}
	return bestAgeBucket(values...)
}

func domainSourceAge(sources ...[]grammar.DomainSource) string {
	values := []string{}
	for _, group := range sources {
		for _, src := range group {
			values = append(values, src.AgeBucket)
		}
	}
	return bestAgeBucket(values...)
}

func (m Model) trustSignalForPage(page int) trustSignal {
	out := trustSignal{Authority: "local-read", Freshness: "static", Support: "metadata-only", Recency: "not temporal", Token: "2nd"}
	switch page {
	case PageEvents:
		recency := eventRecencyBucket(m.Events)
		out = trustSignal{Authority: "observation-only", Freshness: recency, Support: "metadata-only", Recency: recency}
	case PageTasks:
		fresh := "unknown"
		auth := "registry"
		if t, ok := m.FocusedTask(); ok {
			fresh = freshnessScoreBucket(t.Freshness)
			if strings.TrimSpace(t.AuthorityCase) != "" {
				auth = "governed-required"
			}
		}
		out = trustSignal{Authority: auth, Freshness: fresh, Support: "task-registry", Recency: fresh}
	case PageSessions, PageYard:
		fresh, recent := "unknown", "unknown"
		if s, ok := m.FocusedSession(); ok {
			fresh = ageBucketFromSeconds(s.RelayAgeS)
			recent = ageBucketFromSeconds(s.OutputAgeS)
		}
		out = trustSignal{Authority: "resume-preview", Freshness: fresh, Support: "metadata-only", Recency: recent}
	case PageReadiness:
		fresh := gateSourceAge(m.Gates.Sources)
		out = trustSignal{Authority: "governed-readiness", Freshness: fresh, Support: "metadata-only", Recency: fresh}
	case PageIntake:
		fresh := intakeSourceAge(m.Intake.Sources)
		if row, ok := m.FocusedIntakeRow(); ok && strings.TrimSpace(row.AgeBucket) != "" {
			fresh = row.AgeBucket
		}
		out = trustSignal{Authority: "observation-only", Freshness: fresh, Support: "metadata-only", Recency: fresh}
	case PageCaps:
		auth := "projection"
		support := "capability-source"
		if row, ok := m.FocusedCapabilityRow(); ok {
			auth = firstNonEmpty(row.Authority, auth)
			support = firstNonEmpty(row.Status, support)
		}
		fresh := capabilitySourceAge(m.Capabilities.Sources)
		out = trustSignal{Authority: auth, Freshness: fresh, Support: support, Recency: fresh}
	case PageDynamics:
		fresh := dynamicsSourceAge(m.Dynamics.Package.Sources)
		out = trustSignal{Authority: firstNonEmpty(m.Dynamics.Package.Authority, "map-package"), Freshness: fresh, Support: "metadata-only", Recency: fresh}
	case PageLoops:
		fresh := dynamicsSourceAge(m.Dynamics.Package.Sources)
		out = trustSignal{Authority: firstNonEmpty(m.Dynamics.Package.Authority, "map-package"), Freshness: fresh, Support: "computed-structure", Recency: fresh}
	case PageAxes:
		out = trustSignal{Authority: "framework", Freshness: "static", Support: "five-tuple-contract", Recency: "static"}
	case PageIdentity:
		out = trustSignal{Authority: "projection", Freshness: "folded", Support: "role/actor/owner", Recency: "current-fold"}
	case PageRelational:
		out = trustSignal{Authority: "projection", Freshness: "folded", Support: "consent-posture", Recency: "current-fold"}
	case PageDomains:
		fresh := domainSourceAge(m.Domains.LifecycleSources, m.Domains.Sources)
		auth := "domain-pack"
		if row, ok := m.FocusedDomainRow(); ok {
			auth = firstNonEmpty(row.AuthorityCeiling, auth)
		}
		out = trustSignal{Authority: auth, Freshness: fresh, Support: "metadata-only", Recency: fresh}
	case PageLifecycles:
		fresh := domainSourceAge(m.Domains.LifecycleSources)
		auth := firstNonEmpty(m.Domains.LifecycleAuthority, "lifecycle-pack")
		if row, ok := m.FocusedLifecycleRow(); ok {
			auth = firstNonEmpty(row.AuthorityCeiling, auth)
		}
		out = trustSignal{Authority: auth, Freshness: fresh, Support: "metadata-only", Recency: fresh}
	case PageEpistemics:
		if row, ok := m.FocusedEpistemicRow(); ok {
			out = trustSignal{Authority: row.Authority, Freshness: row.Freshness, Support: row.Privacy, Recency: row.Freshness}
		} else {
			out = trustSignal{Authority: "evidence-posture", Freshness: "derived", Support: "metadata-only", Recency: "derived"}
		}
	case PageCommands, PageIntent:
		out = trustSignal{Authority: "preview-only", Freshness: "static", Support: "local-command", Recency: "not temporal"}
	case PageWindows, PageSurfaces, PageHelp, PageLegend:
		out = trustSignal{Authority: "local-reference", Freshness: "static", Support: "metadata-only", Recency: "not temporal"}
	}
	out.Token = trustToken(out)
	return out
}

func (m Model) slackRowsForPage(w, maxRows int, slot slackSlot) []string {
	if maxRows <= 0 || w < 34 {
		return nil
	}
	var b strings.Builder
	rule := grammar.C("border", strings.Repeat("─", maxVisible(10, w-2)))
	b.WriteString(rule + "\n")
	if slot == slackSlotReferenceMain {
		writeSectionHeader(&b, w, "SCREEN SLACK", "relationships, source status, and available controls", "relations + controls")
	} else if slot == slackSlotSessionContext {
		writeSectionHeader(&b, w, "CONTEXT SLACK", "relationship, source status, and lifecycle totals", "relationship + totals")
		rel := m.splitRelation()
		writeWrappedKV(&b, "pair", rel.RelationLabel(), "org", w)
		writeWrappedKV(&b, "contract", rel.Contract, "mut", w)
	} else {
		writeSectionHeader(&b, w, "CONTEXT SLACK", "source status and nearby lifecycle totals", "source + totals")
	}
	if slot == slackSlotReferenceMain {
		for _, row := range m.referenceMainSlackRows(w) {
			b.WriteString(row + "\n")
		}
	} else {
		for _, row := range m.genericSlackRows(w) {
			b.WriteString(row + "\n")
		}
	}
	switch m.Page {
	case PageEvents:
		for _, row := range m.eventSlackRows(w) {
			b.WriteString(row + "\n")
		}
	case PageTasks:
		for _, row := range m.taskSlackRows(w) {
			b.WriteString(row + "\n")
		}
	case PageSessions:
		for _, row := range m.sessionSlackRows(w) {
			b.WriteString(row + "\n")
		}
	case PageYard:
		for _, row := range m.yardSlackRows(w) {
			b.WriteString(row + "\n")
		}
	case PageReadiness:
		for _, row := range m.readinessSlackRows(w) {
			b.WriteString(row + "\n")
		}
	case PageDynamics:
		for _, row := range m.dynamicsSlackRows(w) {
			b.WriteString(row + "\n")
		}
	case PageHelp, PageLegend:
		for _, row := range m.referenceSlackRows(w) {
			b.WriteString(row + "\n")
		}
	case PageCommands:
		for _, row := range m.commandSlackRows(w) {
			b.WriteString(row + "\n")
		}
	case PageWindows:
		for _, row := range m.windowSlackRows(w) {
			b.WriteString(row + "\n")
		}
	case PageIntent:
		for _, row := range m.intentSlackRows(w) {
			b.WriteString(row + "\n")
		}
	case PageSurfaces:
		for _, row := range m.surfaceSlackRows(w) {
			b.WriteString(row + "\n")
		}
	case PageCaps:
		for _, row := range m.capabilitySlackRows(w) {
			b.WriteString(row + "\n")
		}
	case PageDomains:
		for _, row := range m.domainSlackRows(w) {
			b.WriteString(row + "\n")
		}
	case PageLifecycles:
		for _, row := range m.lifecycleSlackRows(w) {
			b.WriteString(row + "\n")
		}
	case PageEpistemics:
		for _, row := range m.epistemicsSlackRows(w) {
			b.WriteString(row + "\n")
		}
	case PageIntake:
		for _, row := range m.intakeSlackRows(w) {
			b.WriteString(row + "\n")
		}
	}
	return firstLines(b.String(), maxRows)
}

func (m Model) referenceMainSlackRows(w int) []string {
	var b strings.Builder
	heading := m.referenceContextHeading()
	if heading == "" {
		heading = "screen context"
	}
	writeWrappedKV(&b, "screen", heading+" · main pane owns the readable registry; context panes own authority/source/provenance", "org", w)
	writeWrappedKV(&b, "cycle", "every registered screen is reachable by title key, :command, and [←/→]; no command-only buried pages", "yel", w)
	if actions := ansi.Strip(m.floorActions(maxVisible(24, w-16))); strings.TrimSpace(actions) != "" {
		writeWrappedKV(&b, "moves", actions, "pri", w)
	}
	writeWrappedKV(&b, "pairing", splitPairSummary()+" · split is a relationship surface, not a second unrelated viewport", "2nd", w)
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) genericSlackRows(w int) []string {
	var b strings.Builder
	trust := m.trustSignalForPage(m.Page)
	writeWrappedKV(&b, "trust", trust.render(), trust.Token, w)
	dynScale := m.dynamicsRenderScaleValue(w)
	writeWrappedKV(&b, "sources", fmt.Sprintf("events:%d r%d · tasks:%d r%d · sessions:%d r%d · caps:%d r%d · domains:%d r%d · dyn:%d r%d",
		len(m.Events), m.EventsSeq, len(m.Tasks), m.TasksSeq, len(m.Sessions), m.SessionsSeq,
		len(m.Capabilities.Rows), m.CapabilitiesSeq, len(m.Domains.Rows), m.DomainsSeq,
		len(m.Dynamics.AtResolution(dynScale).Nodes), m.DynamicsSeq), "2nd", w)
	showLaneContext := m.sessionSplit() || m.Page == PageSessions || m.Page == PageYard
	if s, ok := m.FocusedSession(); ok && showLaneContext {
		writeWrappedKV(&b, "lane", fmt.Sprintf("%s · events:%d · cap-routes:%d",
			strings.Join(nonEmptyParts([]string{sessionFieldValueForAir(s, "role", m.AIR), sessionAnchorSignal(s, m.AIR), m.sessionLivePulse(s)}), " · "),
			len(m.sessionRelatedEvents(s)), len(m.capabilityRoutesForPlatform(strings.TrimSpace(s.Platform)))), airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR), w)
	}
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) eventSlackRows(w int) []string {
	failures, successes, shown := 0, 0, 0
	for _, ev := range m.Events {
		if m.AIR && ev.AIR["kind"] != "ok" {
			continue // a denied kind must not be classified into the mix OR its "other" denominator
		}
		shown++
		kind := strings.ToLower(ev.Kind)
		if strings.Contains(kind, "fail") {
			failures++
		}
		if strings.Contains(kind, "succeed") {
			successes++
		}
	}
	var b strings.Builder
	writeWrappedKV(&b, "event mix", fmt.Sprintf("fail:%d · succeed:%d · other:%d", failures, successes, maxVisible(0, shown-failures-successes)), "2nd", w)
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) taskSlackRows(w int) []string {
	crit := map[string]int{}
	hold := 0
	for _, t := range m.Tasks {
		if !m.AIR || t.AIR["criticality"] == "ok" {
			crit[strings.TrimSpace(t.Criticality)]++ // a denied criticality must not be tallied into the mix
		}
		predHold := strings.Contains(strings.ToLower(t.PredictedStage), "hold") && (!m.AIR || t.AIR["predicted_stage"] == "ok")
		stageRelease := strings.Contains(strings.ToLower(t.Stage), "release") && (!m.AIR || t.AIR["stage"] == "ok")
		if predHold || stageRelease {
			hold++
		}
	}
	var b strings.Builder
	writeWrappedKV(&b, "task mix", fmt.Sprintf("ok:%d · warn:%d · major:%d · crit:%d · release/hold:%d",
		crit["ok"], crit["warn"], crit["major"], crit["crit"], hold), "2nd", w)
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) sessionSlackRows(w int) []string {
	claim, stale, off := 0, 0, 0
	for _, s := range m.Sessions {
		// On air a denied readiness/state must not be classified into the fleet tally — the count
		// discloses the field's class membership (same leak class as the throughput held tally).
		readyOK := !m.AIR || s.AIR["readiness"] == "ok"
		stateOK := !m.AIR || s.AIR["state"] == "ok"
		switch {
		case s.Readiness == "claim" && readyOK:
			claim++
		case s.Readiness == "stale" && readyOK:
			stale++
		case (readyOK && (s.Readiness == "off" || s.Readiness == "offline")) || (stateOK && s.State == "offline"):
			off++
		}
	}
	var b strings.Builder
	writeWrappedKV(&b, "fleet", fmt.Sprintf("claim:%d · stale:%d · off:%d · total:%d", claim, stale, off, len(m.Sessions)), "2nd", w)
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) yardSlackRows(w int) []string {
	stageCounts, hiddenStages, unstaged := m.yardStageCounts()
	fleet := m.yardFleetCounts()
	visibleBlocked, hiddenBlocked := m.yardBlockedIndices()
	var b strings.Builder
	writeWrappedKV(&b, "yard ladder", fmt.Sprintf("S0:%d S3:%d S5:%d S7:%d · hidden:%d unstaged:%d",
		stageCounts[0], stageCounts[3], stageCounts[5], stageCounts[7], hiddenStages, unstaged), "2nd", w)
	writeWrappedKV(&b, "yard attention", fmt.Sprintf("holds:%d+%d hidden · live:%d stale/off:%d",
		len(visibleBlocked), hiddenBlocked, fleet.live, fleet.stale+fleet.off), countWarnToken(len(visibleBlocked)+hiddenBlocked+fleet.stale+fleet.off), w)
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) readinessSlackRows(w int) []string {
	fleet := m.yardFleetCounts()
	visibleBlocked, hiddenBlocked := m.yardBlockedIndices()
	var b strings.Builder
	writeWrappedKV(&b, "gates", fmt.Sprintf("rows:%d · sources:%d · blocked:%d visible + %d hidden",
		len(m.Gates.Rows), len(m.Gates.Sources), len(visibleBlocked), hiddenBlocked), countWarnToken(len(visibleBlocked)+hiddenBlocked), w)
	writeWrappedKV(&b, "lane gates", fmt.Sprintf("claim:%d · stale:%d · off:%d · live:%d · stalled:%d",
		fleet.claim, fleet.stale, fleet.off, fleet.live, fleet.stalled), countWarnToken(fleet.stale+fleet.off+fleet.stalled), w)
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) dynamicsSlackRows(w int) []string {
	scale, scaleLabel := m.dynamicsRenderScale(w)
	g := m.Dynamics.AtResolution(scale)
	var b strings.Builder
	writeWrappedKV(&b, "graph", fmt.Sprintf("nodes:%d · edges:%d · layers:%d · scale:%s", len(g.Nodes), len(g.Edges), len(g.Layers), scaleLabel), "2nd", w)
	if len(m.Dynamics.Package.Totals) > 0 {
		writeWrappedKV(&b, "package", fmt.Sprintf("sources:%d · relations:%d · obs:%d · missing:%d",
			m.Dynamics.Package.Totals["sources"], m.Dynamics.Package.Totals["relations"],
			m.Dynamics.Package.Totals["observations"], m.Dynamics.Package.Totals["missing_sources"]), "mut", w)
	}
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) epistemicsSlackRows(w int) []string {
	rows := m.epistemicRows()
	summary := summarizeEpistemicRows(rows)
	var b strings.Builder
	writeWrappedKV(&b, "epistemics", fmt.Sprintf("rows:%d · observed:%d · support:%d · gaps:%d · neutral:%d", len(rows), summary.Observed, summary.Support, summary.Gaps, summary.Neutral), countWarnToken(summary.Gaps), w)
	if len(summary.Families) > 0 {
		writeWrappedKV(&b, "families", compactCountMap(summary.Families, 5), "2nd", w)
	}
	writeWrappedKV(&b, "boundary", "metadata-only; no raw bodies/transcripts; no authority minted", "yel", w)
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) referenceSlackRows(w int) []string {
	var b strings.Builder
	for _, row := range append(m.referencePageRailRows(), m.referenceContextRows()...) {
		writeWrappedKV(&b, row.label, row.value, row.token, w)
	}
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) commandSlackRows(w int) []string {
	kinds := map[commandKind]int{}
	groups := map[string]int{}
	argVerbs := 0
	governed := 0
	for _, v := range verbs {
		kinds[v.kind]++
		if strings.TrimSpace(v.group) != "" {
			groups[v.group]++
		}
		if len(v.args) > 0 {
			argVerbs++
		}
		if strings.Contains(v.authority, "governed") || v.kind == commandIntent {
			governed++
		}
	}
	var b strings.Builder
	writeWrappedKV(&b, "command classes", fmt.Sprintf("read:%d · lens:%d · local:%d · intent:%d · governed:%d",
		kinds[commandRead], kinds[commandLens], kinds[commandLocal], kinds[commandIntent], governed), "2nd", w)
	writeWrappedKV(&b, "completion", fmt.Sprintf("verbs:%d · arg verbs:%d · window group:%d · template refs:%d",
		len(verbs), argVerbs, groups["window"], len(m.templateInjectionRows())), "yel", w)
	if v, ok := m.FocusedCommand(); ok && !m.passiveReferenceSplitActive() {
		writeWrappedKV(&b, "selected command", fmt.Sprintf("%s · %s · auth=%s · receipt=%s",
			commandDisplayName(v), commandKindGroup(v), firstNonEmpty(v.authority, "none"), firstNonEmpty(v.receipt, "none")), commandToken(v), w)
		if len(v.args) > 0 {
			labels := make([]string, 0, len(v.args))
			for _, arg := range v.args {
				labels = append(labels, arg.Label)
			}
			writeWrappedKV(&b, "legal args", strings.Join(labels, " · "), "pri", w)
		}
	}
	writeWrappedKV(&b, "route contract", "preview target/preflight/receipt before any mutation surface", "org", w)
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) windowSlackRows(w int) []string {
	scopeCounts := map[string]int{}
	lifecycleCounts := map[string]int{}
	linked, sourceOnly := 0, 0
	for _, wnd := range registeredWindows() {
		scopeCounts[wnd.Scope]++
		lifecycleCounts[wnd.Lifecycle]++
	}
	for _, pair := range registeredSplitPairs() {
		if pair.Reactive() {
			linked++
		} else {
			sourceOnly++
		}
	}
	var b strings.Builder
	writeWrappedKV(&b, "window topology", fmt.Sprintf("registered:%d · engine:%d · instance:%d · sdlc:%d · routing:%d",
		len(registeredWindows()), scopeCounts["engine"], scopeCounts["instance"], lifecycleCounts["sdlc"], lifecycleCounts["routing"]), "2nd", w)
	writeWrappedKV(&b, "split topology", fmt.Sprintf("linked:%d · source-only:%d · cycle:[←/→] · jump:title keys", linked, sourceOnly), "org", w)
	if wnd, ok := m.FocusedWindow(); ok && !m.passiveReferenceSplitActive() {
		pair, pairTok := splitPairCatalogCell(wnd.Page)
		signal, sigTok := m.windowSignal(wnd.Page)
		writeWrappedKV(&b, "selected window", fmt.Sprintf("%s · key=%s · %s/%s · %s", wnd.ID, wnd.Key, wnd.Scope, wnd.Lifecycle, wnd.Kind), "pri", w)
		writeWrappedKV(&b, "window signal", firstNonEmpty(signal, "quiet"), sigTok, w)
		writeWrappedKV(&b, "window split", pair, pairTok, w)
	}
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) intentSlackRows(w int) []string {
	target := strings.TrimSpace(m.IntentTarget)
	if m.passiveReferenceSplitActive() {
		target = "source lane anchored"
	} else if target == "" {
		if selected, ok := m.selectedIntentArg(); ok {
			target = selected.Label
		}
	}
	v, _ := lookupVerb("intent")
	var b strings.Builder
	writeWrappedKV(&b, "intent target", fmt.Sprintf("%s · %d targets · focus:%d/%d",
		firstNonEmpty(target, "choose"), len(lookupIntentArgs()), m.IntentFocus+1, len(lookupIntentArgs())), "yel", w)
	writeWrappedKV(&b, "route preview", fmt.Sprintf("authority=%s · receipt=%s", v.authority, v.receipt), "org", w)
	writeWrappedKV(&b, "templates", "{{focus}} · {{sel.*}} · {{ring.0}} bind subject without copy/paste", "2nd", w)
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) surfaceSlackRows(w int) []string {
	kindCounts := map[string]int{}
	scopeCounts := map[string]int{}
	for _, surf := range surfaceRegistry {
		kindCounts[surf.Kind]++
		scopeCounts[surf.Scope]++
	}
	var b strings.Builder
	writeWrappedKV(&b, "surface classes", fmt.Sprintf("mode:%d · door:%d · lens:%d · projection:%d · layout:%d",
		kindCounts["mode"], kindCounts["door"], kindCounts["lens"], kindCounts["projection"], kindCounts["layout"]), "2nd", w)
	writeWrappedKV(&b, "surface scope", fmt.Sprintf("engine:%d · instance:%d · all named/exitable", scopeCounts["engine"], scopeCounts["instance"]), "grn", w)
	if surf, ok := m.FocusedSurface(); ok && !m.passiveReferenceSplitActive() {
		writeWrappedKV(&b, "selected surface", fmt.Sprintf("%s · %s · open=%s · exit=%s", surf.ID, surf.Kind, surf.Open, surf.Exit), "org", w)
	}
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) capabilitySlackRows(w int) []string {
	total, gaps := m.capabilityTitleCounts()
	grouped := groupCapabilityStatusRows(m.capabilityStatusRows())
	var b strings.Builder
	writeWrappedKV(&b, "capability", fmt.Sprintf("rows:%d · gaps:%d · routes:%d · tools:%d", total, gaps, len(m.Capabilities.Routes), len(m.Capabilities.Tools)), "2nd", w)
	if classRows := grouped["class"]; len(classRows) > 0 {
		limit := len(classRows)
		if limit > 3 {
			limit = 3
		}
		parts := make([]string, 0, limit)
		for i := 0; i < limit; i++ {
			row := classRows[i]
			_, blocked := capabilitySurfaceStats(row, grouped["surface"])
			parts = append(parts, fmt.Sprintf("%s %s r%d/%d s!%d",
				clipRunes(row.Name, 16), row.Status, row.OKCount, row.RouteCount, blocked))
		}
		writeWrappedKV(&b, "class gaps", strings.Join(parts, " · "), countWarnToken(gaps), w)
	}
	if m.sessionSplit() && !m.targetRowFocusActive() {
		if s, ok := m.FocusedSession(); ok {
			fit, fitTok := m.selectedLaneCapabilityPosture(s)
			writeWrappedKV(&b, "selected lane", fmt.Sprintf("%s · %s · readiness=%s", sessionFieldValueForAir(s, "role", m.AIR), sessionFieldValueForAir(s, "platform", m.AIR), sessionFieldValueForAir(s, "readiness", m.AIR)), airHue(grammar.LaneToken(s.Role), s.AIR, "role", m.AIR), w)
			writeWrappedKV(&b, "lane fit", fit, fitTok, w)
		}
		return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	}
	if capRow, ok := m.FocusedCapabilityRow(); ok {
		writeWrappedKV(&b, "selected cap", fmt.Sprintf("%s · status=%s · authority=%s", capRow.Name, capRow.Status, capRow.Authority), capRow.Token, w)
		writeWrappedKV(&b, "selected ev", fmt.Sprintf("%s · source=%s", capRow.Evidence, firstNonEmpty(capRow.SourceRefs, "none")), countWarnToken(capRow.BlockedCount), w)
	}
	if len(m.Capabilities.Sources) > 0 {
		src := m.Capabilities.Sources[len(m.Capabilities.Sources)-1]
		writeWrappedKV(&b, "source pack", fmt.Sprintf("%s · %s · count:%d · age:%s",
			capabilitySourceFieldForAir(src, "id", m.AIR),
			capabilitySourceFieldForAir(src, "status", m.AIR),
			src.Count,
			capabilitySourceFieldForAir(src, "age_bucket", m.AIR)), sourceStatusToken(src.Status), w)
	}
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) domainSlackRows(w int) []string {
	var b strings.Builder
	writeWrappedKV(&b, "domains", fmt.Sprintf("lifecycles:%d · rows:%d · catalog:%d · relations:%d · packs:%d/%d · lifecycle-extensible", len(m.Domains.Lifecycles), len(m.Domains.Rows), m.domainRowCount(), len(m.Domains.Relations), len(m.Domains.LifecycleSources), len(m.Domains.Sources)), "2nd", w)
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) lifecycleSlackRows(w int) []string {
	var b strings.Builder
	writeWrappedKV(&b, "lifecycles", fmt.Sprintf("rows:%d · fallback:%d · sources:%d · authority:%s", len(m.Domains.Lifecycles), len(registeredLifecycleFallbacks()), len(m.Domains.LifecycleSources), firstNonEmpty(m.Domains.LifecycleAuthority, "support-only")), "2nd", w)
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m Model) intakeSlackRows(w int) []string {
	var b strings.Builder
	writeWrappedKV(&b, "intake", fmt.Sprintf("buckets:%d/%d · sources:%d · attention:%d", len(m.visibleIntakeRows()), len(m.Intake.Rows), len(m.Intake.Sources), m.intakeAttentionTotal()), "2nd", w)
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}
