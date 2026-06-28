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
	"github.com/hapax-systems/reins/internal/dispatch"
	"github.com/hapax-systems/reins/internal/grammar"
	"github.com/hapax-systems/reins/internal/graph"
	"github.com/hapax-systems/reins/internal/imgpreview"
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

// fetchLastlogPageOnce: one backward-page fetch (PgUp in /lastlog) -> a LastlogPageMsg.
func fetchLastlogPageOnce(url, before string) tea.Msg {
	evs, dark, err := api.FetchEventsBefore(url, before)
	msg := model.LastlogPageMsg{Events: evs, Dark: dark}
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

// fetchTracesOnce: one LLM-observability fetch -> a TracesMsg.
func fetchTracesOnce(url string) tea.Msg {
	tr, dark, err := api.FetchTraces(url)
	msg := model.TracesMsg{Traces: tr, Dark: dark}
	if err != nil {
		msg.Error = err.Error()
	}
	return msg
}

// fetchTurnsOnce: one session turn-receipt page for a lane role -> a TurnsMsg. An empty role (no
// lane targeted yet) is dark by construction — the chat pane keeps its demo fixture (labeled).
func fetchTurnsOnce(role string) tea.Msg {
	if strings.TrimSpace(role) == "" {
		return model.TurnsMsg{Dark: true}
	}
	turns, err := api.FetchTurns(role, "")
	return model.TurnsMsg{Turns: turns, Dark: err != nil, Error: errorText(err)}
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
func turnsTick(role string) tea.Cmd { // chat-pane live feed — polls the targeted lane's turn receipts
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return fetchTurnsOnce(role) })
}
func tracesTick(url string) tea.Cmd { // LLM-spend obs — a cadence between sessions(4s) and intake(8s)
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return fetchTracesOnce(url) })
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
		func() tea.Msg { return fetchTracesOnce(r.url) },
		func() tea.Msg { return fetchTurnsOnce(r.m.TurnRole) },
		eventsTick(r.url), tasksTick(r.url), dynamicsTick(r.url), epistemicsTick(r.url), sessionsTick(r.url), intakeTick(r.url), capabilitiesTick(r.url), gatesTick(r.url), domainsTick(r.url), tracesTick(r.url), turnsTick(r.m.TurnRole), beatTick(),
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
	case model.TracesMsg:
		return r, tracesTick(r.url) // re-arm the LLM-observability poll/re-fold loop
	case model.TurnsMsg:
		return r, turnsTick(r.m.TurnRole) // re-arm the chat-pane turn feed for the currently-targeted lane
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
	case model.LastlogPageRequest:
		return r, func() tea.Msg { return fetchLastlogPageOnce(r.url, msg.(model.LastlogPageRequest).Before) }
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
	case "coordinator", "coord":
		return model.PageCoordinator, true
	case "readiness", "ready", "gates", "gate":
		return model.PageReadiness, true
	case "capabilities", "caps", "cap":
		return model.PageCaps, true
	case "intake", "obs", "observations":
		return model.PageIntake, true
	case "dynamics", "dyn":
		return model.PageDynamics, true
	case "loops-page", "causal-page", "causal":
		return model.PageLoops, true
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
	case "traces", "trace":
		return model.PageTraces, true
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
		// --probe facets: fetch the facet-registry SSOT (/read/facets) and render the cold-read facet
		// legend (A6: the decoder travels in-band; the Go side consumes the registry, never re-derives).
		for _, a := range os.Args[2:] {
			if a == "facets" {
				reg, dark, err := api.FetchFacets(cfg.APIURL)
				if dark || err != nil {
					fmt.Printf("facets: dark/unreachable (%v)\n", err)
					return
				}
				fmt.Print(grammar.RenderFacetLegend(reg))
				fmt.Printf("\n%d facets · %d fields air on-stream (registry SSOT).\n",
					len(reg.Facets), len(reg.AirAllowlist))
				return
			}
		}
		// --probe loops: run the A5 Tier-1 graph primitive over the LIVE :dynamics map (read-only
		// fetch) — detect feedback loops + classify Reinforcing/Balancing by negative-sign parity,
		// NO simulation. Demonstrates the qualitative systems-dynamics layer on real data.
		for _, a := range os.Args[2:] {
			if a == "loops" || a == "matrix" {
				dg, dark, err := api.FetchDynamics(cfg.APIURL)
				if err != nil || dark {
					fmt.Printf("loops: :dynamics dark/unreachable (%v)\n", err)
					return
				}
				tg := graph.New()
				for _, e := range dg.Edges {
					tg.Add(graph.Relation{Src: e.Source, Dst: e.Target, Type: e.Relation})
				}
				if a == "matrix" {
					for _, ln := range tg.AdjacencyMatrix() {
						fmt.Println(ln)
					}
					return
				}
				loops := tg.CausalLoops()
				fmt.Printf("CAUSAL LOOPS over :dynamics (%d nodes · %d edges) — A5 Tier-1, computed no-sim\n",
					len(tg.Nodes()), len(dg.Edges))
				badge := map[graph.LoopKind]string{graph.Reinforcing: "⟲R", graph.Balancing: "⟳B", graph.Indeterminate: "⟲?"}
				if len(loops) == 0 {
					fmt.Println("  (no feedback loops in the current map — it is largely a DAG)")
				}
				for _, lp := range loops {
					d := ""
					if lp.HasDelay {
						d = " ‖delay"
					}
					fmt.Printf("  %s%s  %s\n", badge[lp.Kind], d, strings.Join(lp.Nodes, " → "))
				}
				return
			}
		}
		// --probe turns/session: OFFLINE fixture render of the session-pane turn grammar (§9
		// read-projection; CapabilityIO SESSION-kind is gated, so no live backend yet). Demonstrates
		// the turn-ladder + its livestream-safe AIR collapse for the operator vetting loop.
		for _, a := range os.Args[2:] {
			if a == "turns" || a == "session-turns" || a == "session" {
				air := false
				for _, b := range os.Args[2:] {
					if b == "--air" {
						air = true
					}
				}
				detail := false
				for _, b := range os.Args[2:] {
					if b == "--detail" {
						detail = true
					}
				}
				turns, blocks := model.SessionTurnFixture()
				if detail { // expanded single-turn tree (progressive-disclosure summon view)
					idx := 3
					if idx >= len(turns) {
						idx = 0
					}
					if len(turns) == 0 {
						return
					}
					hdr := turns[idx]
					fmt.Print(grammar.RenderTurnDetail(hdr, blocks[model.TurnID(hdr)], air))
					return
				}
				fmt.Println("· demo fixture — offline turn-grammar render (live session feed not wired in --probe)")
				fmt.Println(grammar.RenderTurnHeader())
				for _, t := range turns {
					fmt.Println(grammar.RenderTurnRow(t, air))
				}
				return
			}
		}
		// --probe dispatch: OFFLINE fixture render of the dispatch-observability surface (the
		// cc-task-capdispatch-surface-20260627 ledger — records emitted by the dev2 lane, SURFACED here).
		// Read-projection AHEAD of the feed (mirrors --probe turns): proves the measurement-first honesty —
		// a null cost renders UNMEASURED (never $0.00), quality renders asserted (never a fake score),
		// outcome renders in-flight — plus the latent-resource utilization rollup. [--air] redacts the
		// cc_task id + session role; routing + measurement stay (no false confidentiality).
		for _, a := range os.Args[2:] {
			if a == "dispatch" {
				air := false
				for _, b := range os.Args[2:] {
					if b == "--air" {
						air = true
					}
				}
				routable := []string{"glm-via-cc", "codex.full", "claude.fast", "claude.interactive", "agy", "api.provider_gateway", "fugu", "fugu-ultra", "glmcp-worker", "sakana"}
				// LIVE first: read the real ledger the dev2 lane (cc-dispatch) appends to. Empty →
				// fall back to the fixture (the read-projection-ahead) with an honest note.
				records, _ := dispatch.Read(dispatch.LedgerPath(), 50)
				if len(records) == 0 {
					cost := 0.0123
					pass := "pass"
					done := "succeeded"
					records = []grammar.DispatchRecord{
						{TS: "2026-06-27T20:50:01Z", Capability: "glm-via-cc", RouteID: "claude.full", Platform: "claude", Mode: "fast", Profile: "full", CCTask: "cc-task-capdispatch-surface-20260627", SliceKind: "impl", AdmissionAction: "admitted", Launched: true, DispatchLatencyMs: 1180, SessionRole: "dev2"},
						{TS: "2026-06-27T20:42:14Z", Capability: "codex.full", RouteID: "codex.spark.full", Platform: "codex", Mode: "spark", Profile: "full", CCTask: "cc-task-edt-scorer-20260627", SliceKind: "review", AdmissionAction: "admitted", Launched: true, DispatchLatencyMs: 940, CostUSD: &cost, QualitySignal: &pass, Outcome: &done, SessionRole: "dev3"},
						{TS: "2026-06-27T20:31:09Z", Capability: "fugu", RouteID: "—", Platform: "—", Mode: "—", Profile: "—", CCTask: "cc-task-routedef", SliceKind: "impl", AdmissionAction: "fail_closed", Launched: false, DispatchLatencyMs: 12, SessionRole: "dev2"},
					}
					fmt.Println(grammar.C("yel", "(fixture — no live ledger at ~/.cache/hapax/sdlc-routing/dispatch-events.jsonl yet)"))
				}
				fmt.Println(grammar.RenderDispatchLedger(records, air))
				fmt.Println()
				fmt.Println(grammar.RenderUtilization(grammar.Utilization(records, routable)))
				return
			}
		}
		// --probe encode: render the cell-grammar channel-typing table (framework §1 Layer-2). Each
		// facet binds to ONE cell channel (Bertin-for-monospace), shown with a sample-encoded cell.
		// Uses the live /read/facets registry as the SSOT binding when reachable, else the built-in
		// default table. [--air] shows the bimodal on-air picture: skeleton facets air; the PII-bearing
		// facets (identity = label/title/subject, place = path/session) redact. Proves color is a
		// redundant amplifier — the table still reads with hue stripped (grayscale / freeze-frame).
		for _, a := range os.Args[2:] {
			if a == "encode" {
				air := false
				for _, b := range os.Args[2:] {
					if b == "--air" {
						air = true
					}
				}
				reg, dark, _ := api.FetchFacets(cfg.APIURL)
				src := "registry SSOT (/read/facets)"
				if dark || len(reg.Facets) == 0 {
					src = "built-in default table (API dark)"
				}
				order := reg.CitationOrder
				if len(order) == 0 { // offline citation order (decreasing concreteness)
					order = []string{"identity", "ownership", "place", "action", "posture", "variant", "measure", "time", "provenance"}
				}
				samples := map[string]grammar.CellValue{
					"identity":   {Text: "task-4284", Width: 12},
					"ownership":  {Text: "alpha", Width: 8},
					"place":      {Text: "podium", Width: 8},
					"action":     {Text: "implement", Width: 10},
					"posture":    {Text: "crit", Width: 6},
					"variant":    {Text: "opus·fast", Width: 10},
					"measure":    {Magnitude: 0.72, Text: "0.72", Width: 5},
					"time":       {Magnitude: 0.85, Text: "2m", Width: 4},
					"provenance": {Text: "inferred", Width: 9},
				}
				denyOnAir := map[string]bool{"identity": true, "place": true} // PII-bearing facets
				if !dark && len(reg.Facets) > 0 {                             // surface SSOT drift at runtime (re-worded prose the parser can't bind)
					var drift []string
					for f := range reg.Facets {
						if grammar.ChannelFromProse(reg.Facets[f].Channel) == grammar.ChannelUnknown {
							drift = append(drift, f)
						}
					}
					if len(drift) > 0 {
						fmt.Printf("  ⚠ WARN: registry channel prose unrecognized for %s — fell back to the name default (possible SSOT drift)\n\n", strings.Join(drift, ", "))
					}
				}
				fmt.Println("CELL GRAMMAR ENCODER — Bertin-for-monospace (framework §1 Layer-2)")
				fmt.Println("each facet binds to ONE cell channel; color is a redundant amplifier (reads in grayscale)")
				fmt.Println()
				fmt.Printf("  %-11s %-16s %s\n", "FACET", "CHANNEL", "SAMPLE CELL")
				var rowCells []grammar.FacetCell
				for _, f := range order {
					v, ok := samples[f]
					if !ok {
						continue
					}
					if air && denyOnAir[f] {
						v.Denied = true
					}
					cell := grammar.EncodeCell(reg, f, v, air)
					fmt.Printf("  %-11s %-16s %s\n", f, cell.Channel.String(), cell.Rendered)
					rowCells = append(rowCells, grammar.FacetCell{Facet: f, Value: v})
				}
				// the same cells COMPOSED into one row — the generalization of RenderTaskRow to any
				// faceted entity ("every pane renders the same way", framework §1 Layer-2).
				fmt.Println()
				fmt.Println("  SAMPLE ROW (one faceted entity, composed via the encoder):")
				fmt.Println("  " + grammar.RenderFacetRow(reg, rowCells, air))
				airNote := ""
				if air {
					airNote = " · ON AIR: identity/place redact (PII); skeleton facets air"
				}
				fmt.Printf("\n%d facets · binding from %s%s\n", len(samples), src, airNote)
				return
			}
			if a == "trainyard" {
				// --probe trainyard: render the octolinear SDLC metro-map. Uses live /read/tasks when
				// reachable, else a fixture exercising every honesty rule: a clear lane, an amber gate,
				// a crit task pulled to a siding, a DARK (✖) gate, and the WITNESS terminus. Color is a
				// redundant amplifier — the map reads in grayscale.
				ts, dark, _ := api.FetchTasks(cfg.APIURL)
				src := "live /read/tasks"
				if dark || len(ts) == 0 {
					src = "fixture (API dark)"
					ts = []grammar.Task{
						{TaskID: "alpha", Stage: "S5_IMPL", Owner: "claude", Criticality: "ok", Freshness: 0.92, RelCount: 2},
						{TaskID: "beta", Stage: "S8_SHIP", Owner: "claude", Criticality: "warn", Freshness: 0.4, RelCount: 1},
						{TaskID: "gamma", Stage: "S6_VERIFY", Owner: "codex", Criticality: "crit", Freshness: 0.5, RelCount: 2},
						{TaskID: "delta", Stage: "S3_PLAN", Owner: "codex", Criticality: "ok", Freshness: 0.0, RelCount: 1},
						{TaskID: "epsilon", Stage: "S7_RELEASE", Owner: "claude", Criticality: "ok", Freshness: 0.1, RelCount: 1},
						{TaskID: "omega", Stage: "S2_SCOPE", Owner: "operator", Criticality: "ok", Freshness: 0.7, RelCount: 1},
					}
				}
				w := 100
				for _, b := range os.Args[2:] {
					if pw, _, ok := parseProbeSize(b); ok {
						w = pw
					}
				}
				fmt.Println("TRAINYARD — octolinear SDLC metro-map (WITNESS terminus; state in shape+position)")
				fmt.Println()
				fmt.Println(grammar.RenderTrainyard(grammar.Trainyard{Tasks: ts}, w))
				fmt.Printf("\n%d tasks · binding from %s\n", len(ts), src)
				return
			}
			if a == "image" {
				// --probe image <path> [size:WxH]: render an image through the preview substrate (the
				// operator's off-air present-at-hand frame) so the filebrowser's image preview can be
				// eyeballed headlessly. Run in a real terminal to SEE the picture.
				path := ""
				braille := false
				w, h := 80, 40
				for _, b := range os.Args[2:] {
					if pw, ph, ok := parseProbeSize(b); ok {
						w, h = pw, ph
						continue
					}
					if b == "braille" {
						braille = true
						continue
					}
					if b != "image" && !strings.HasPrefix(b, "--") {
						path = b
					}
				}
				if path == "" {
					fmt.Println("usage: reins --probe image <path> [size:WxH] [braille]")
					return
				}
				var out string
				var err error
				if braille {
					fmt.Printf("IMAGE PREVIEW — %s · braille dot-matrix 2×4 dots/cell (%dx%d cells)\n\n", path, w, h)
					out, err = imgpreview.RenderFileBraille(path, w, h)
				} else {
					proto := imgpreview.DetectProtocol(os.Getenv)
					fmt.Printf("IMAGE PREVIEW — %s · half-block %s (%dx%d cells)\n\n", path, proto, w, h)
					out, err = imgpreview.RenderFile(path, w, h, proto)
				}
				if err != nil {
					fmt.Println("error:", err)
				}
				fmt.Println(out)
				return
			}
		}
		evs, ed, evErr := api.FetchEvents(cfg.APIURL)
		ts, td, taskErr := api.FetchTasks(cfg.APIURL)
		dg, dd, dynErr := api.FetchDynamics(cfg.APIURL)
		ep, epd, epiErr := api.FetchEpistemics(cfg.APIURL)
		ss, sd, sessErr := api.FetchSessions(cfg.APIURL)
		in, id, intakeErr := api.FetchIntake(cfg.APIURL)
		caps, cd, capsErr := api.FetchCapabilities(cfg.APIURL)
		gates, gd, gatesErr := api.FetchGates(cfg.APIURL)
		doms, domd, domErr := api.FetchDomains(cfg.APIURL)
		tr, trd, traceErr := api.FetchTraces(cfg.APIURL)
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
		m = updateProbeModel(m, model.TracesMsg{Traces: tr, Dark: trd, Error: errorText(traceErr)})
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
	launch := model.New("REINS")
	launch.Page = model.PageCoordinator // land on the Yard Coordinator (the new framework gestalt), not the legacy :events
	r := root{m: launch, url: cfg.APIURL}
	if _, err := tea.NewProgram(r, tea.WithAltScreen()).Run(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
