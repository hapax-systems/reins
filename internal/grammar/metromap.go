package grammar

import (
	"strconv"
	"strings"
)

// The trainyard metro-map: an octolinear SDLC rail map where every state is carried by SHAPE
// and POSITION (Bertin), and color (via C) is only a redundant amplifier — the whole map stays
// legible in grayscale. It renders lanes (by actor) across the 11 SDLC stations, the gate
// signals at each station, the work-item "trains", blocked work pulled onto sidings, and the
// WITNESS terminus (merged-but-unwitnessed sits penultimate; deploy is never the end).
//
// Design provenance: reins-trainyard-metromap-design-agy-2026-06-27 (the agy lane), implemented
// directly here for the honesty invariants (the GLM impl of the full design timed out).

// trainyardStations is the closed, ordered station list. WIT (witnessed-in-production) is the
// terminus — observe/closeout/archive collapse onto it, and deploy (DEP) sits strictly left.
var trainyardStations = []string{"REQ", "TRI", "SCO", "PLN", "DSN", "IMP", "VRF", "REL", "DEP", "OBS", "WIT"}

// Trainyard is the metro-map input: the live tasks, grouped into actor lanes at render time.
type Trainyard struct {
	Tasks []Task
}

// stationIndex maps an SDLC stage ("S8_SHIP") onto its station column. whoisStageIndex is bounded
// to 0..11 (or -1), so the clamps are honest: an UNRESOLVED stage falls to intake (REQ, never the
// terminus), and only S11 (archive) reaches the WIT terminus via the upper clamp — a merged-but-
// UNWITNESSED task (S8) maps to DEP (penultimate), never to WIT. (Pinned by
// TestTrainyardStationsTerminateAtWitness; the GLM-via-CC review's "unwitnessed could land on WIT"
// concern does not occur because the bad case is unreachable given that range.)
func stationIndex(stage string) int {
	n := whoisStageIndex(stage)
	if n < 0 {
		return 0
	}
	if n >= len(trainyardStations) {
		return len(trainyardStations) - 1
	}
	return n
}

// signalGlyph is the gate at a station, honest about what is and isn't known:
//   - ✖ DARK   — a MEASURED-stale gate (low-but-positive freshness): uncrossable, worse than red.
//   - ■ red    — a crit hard-stop.
//   - ▷ amber  — warn/major, proceed with caution.
//   - ◌ unknown— no freshness signal at all (freshness==0): we don't know — NOT clear, NOT dark.
//   - ► clear  — fresh and ok.
//
// The ◌/✖ split is the key honesty rule: an ABSENT freshness field must read "unknown", never
// "dark" — painting universal ✖ when the API simply doesn't report freshness is a lie. A gate is
// NEVER green when stale or unknown.
const darkFreshnessFloor = 0.15

func signalGlyph(criticality string, freshness float64) string {
	if freshness > 0 && freshness < darkFreshnessFloor {
		return "✖" // DARK — measured stale/unverified, worse than red
	}
	switch criticality {
	case "crit":
		return "■" // red — hard stop
	case "major", "warn":
		return "▷" // amber — proceed with warnings
	}
	if freshness <= 0 {
		return "◌" // unknown — no freshness signal (honest: neither clear nor dark)
	}
	return "►" // green — clear
}

// tyGateSeverity ranks gates so the worst one wins at a shared station. Derived FROM signalGlyph
// so the two can never drift: DARK > red > amber > unknown > clear.
func tyGateSeverity(criticality string, freshness float64) int {
	switch signalGlyph(criticality, freshness) {
	case "✖":
		return 5
	case "■":
		return 4
	case "▷":
		return 3
	case "◌":
		return 2
	default: // ►
		return 1
	}
}

// trainCaps encodes velocity (SLA pace) in the train's end-caps: fast >..>, normal (..),
// slow <..<, stalled [..].
func trainCaps(velocity string) (string, string) {
	switch velocity {
	case "fast":
		return ">", ">"
	case "slow":
		return "<", "<"
	case "stalled":
		return "[", "]"
	default: // normal
		return "(", ")"
	}
}

// trainVelocity proxies SLA pace from freshness; blocked or signal-less work is stalled.
func trainVelocity(t Task) string {
	if taskBlocked(t) || t.Freshness <= 0 {
		return "stalled"
	}
	switch {
	case t.Freshness < 0.34:
		return "slow"
	case t.Freshness < 0.67:
		return "normal"
	default:
		return "fast"
	}
}

// trainCore is the body: mass runes (scope) shaded by age (new █ · mid ▓ · old ▒ · ancient ░).
func trainCore(freshness float64, mass int) string {
	shade := "█" // new
	switch {
	case freshness <= 0:
		shade = "░" // ancient / no signal
	case freshness < 0.34:
		shade = "▒" // old
	case freshness < 0.67:
		shade = "▓" // mid
	}
	if mass < 1 {
		mass = 1
	}
	return strings.Repeat(shade, mass)
}

// taskBlocked: work that must leave the mainline for a siding — a hard-stop criticality or an
// explicit hold predicate. (NoGo descriptors are intentionally NOT a block signal: every SDLC
// packet carries no-go fields, so keying on their presence would side-track every task.)
func taskBlocked(t Task) bool {
	return t.Criticality == "crit" || strings.EqualFold(strings.TrimSpace(t.PredictedStage), "hold")
}

// laneClass is the actor lane; each renders in a distinct, colorblind-safe track style.
type laneClass int

const (
	laneClaude   laneClass = iota // solid ─
	laneCodex                     // dashed ╌
	laneOperator                  // double ═
)

func laneClassOf(owner string) laneClass {
	o := strings.ToLower(strings.TrimSpace(owner))
	switch {
	case strings.Contains(o, "codex") || strings.HasPrefix(o, "cx"):
		return laneCodex
	case strings.Contains(o, "operator") || strings.Contains(o, "hapax") || o == "op":
		return laneOperator
	default:
		return laneClaude
	}
}

func laneTrackRune(c laneClass) rune {
	switch c {
	case laneCodex:
		return '╌'
	case laneOperator:
		return '═'
	default:
		return '─'
	}
}

// sidingBumper terminates a blocked siding in the lane's own weight.
func sidingBumper(c laneClass) string {
	if c == laneOperator {
		return "╣"
	}
	return "┤"
}

type tyLane struct {
	class laneClass
	label string
	tasks []Task
}

// groupLanes buckets tasks into actor lanes in a stable order, dropping empty lanes.
func groupLanes(tasks []Task) []tyLane {
	order := []laneClass{laneClaude, laneCodex, laneOperator}
	labels := map[laneClass]string{laneClaude: "Cl", laneCodex: "Co", laneOperator: "Op"}
	byClass := map[laneClass][]Task{}
	for _, t := range tasks {
		c := laneClassOf(t.Owner)
		byClass[c] = append(byClass[c], t)
	}
	var lanes []tyLane
	for _, c := range order {
		if len(byClass[c]) > 0 {
			lanes = append(lanes, tyLane{class: c, label: labels[c], tasks: byClass[c]})
		}
	}
	return lanes
}

// --- rune-grid helpers (the map is a 2D rune canvas; placement clamps, never panics) ---

func tyBlank(w int) []rune {
	r := make([]rune, w)
	for i := range r {
		r[i] = ' '
	}
	return r
}

func tyPlace(row []rune, x int, s []rune) {
	for i, c := range s {
		if p := x + i; p >= 0 && p < len(row) {
			row[p] = c
		}
	}
}

func tyRow(row []rune) string { return strings.TrimRight(string(row), " ") }

func tyHasContent(row []rune) bool {
	for _, r := range row {
		if r != ' ' {
			return true
		}
	}
	return false
}

// tyBucket holds one (lane, station) cell's tasks, split by mainline vs siding.
type tyBucket struct{ main, side []Task }

// worstTask returns the task with the most severe gate (it drives the station signal).
func worstTask(ts []Task) Task {
	w := ts[0]
	for _, t := range ts[1:] {
		if tyGateSeverity(t.Criticality, t.Freshness) > tyGateSeverity(w.Criticality, w.Freshness) {
			w = t
		}
	}
	return w
}

// stationGlyph renders the work in one (lane, station) cell: a single train when one task is
// present, else an honest "▣N" count chip standing for N collapsed trains — multiple trains are
// never silently overdrawn into a single misleading glyph.
func stationGlyph(ts []Task, maxMass int) string {
	if len(ts) == 1 {
		return renderTrain(ts[0], maxMass)
	}
	return "▣" + strconv.Itoa(len(ts))
}

func renderTrain(t Task, maxMass int) string {
	l, r := trainCaps(trainVelocity(t))
	mass := t.RelCount
	if mass < 1 {
		mass = 1
	}
	if mass > maxMass {
		mass = maxMass
	}
	return l + trainCore(t.Freshness, mass) + r
}

// RenderTrainyard draws the metro-map for the live tasks across the SDLC stations. Width drives
// responsive station spacing. The output is grayscale-complete: signals (►▷■✖), train physics
// (caps=pace, shade=age, length=scope), and sidings carry every state without any color.
func RenderTrainyard(y Trainyard, width int) string {
	if width <= 0 {
		width = 100
	}
	lanes := groupLanes(y.Tasks)
	if len(lanes) == 0 {
		return C("mut", "▭ trainyard · no tasks in view · stations REQ→WIT (WIT = witnessed terminus)")
	}

	spacing := 8
	switch {
	case width >= 120:
		spacing = 10
	case width < 90:
		spacing = 6
	}
	const labelW = 6
	nSt := len(trainyardStations)
	stX := func(i int) int { return labelW + i*spacing }
	gridW := stX(nSt-1) + 6 // tail past the terminus
	maxMass := spacing - 4
	if maxMass < 1 {
		maxMass = 1
	}
	if maxMass > 4 {
		maxMass = 4
	}

	var b strings.Builder

	// Station-label header.
	hdr := tyBlank(gridW)
	for i, name := range trainyardStations {
		tyPlace(hdr, stX(i), []rune(name))
	}
	b.WriteString(C("mut", tyRow(hdr)))
	b.WriteByte('\n')

	for _, ln := range lanes {
		main := tyBlank(gridW)
		siding := tyBlank(gridW)
		tyPlace(main, 0, []rune(pad(ln.label, labelW-1)))

		// Base track from the first to the last station — it stops AT witness (terminus).
		track := laneTrackRune(ln.class)
		for x := labelW; x <= stX(nSt-1); x++ {
			main[x] = track
		}

		// Group this lane's tasks by station, splitting healthy (mainline) from blocked (siding).
		byStation := map[int]*tyBucket{}
		for _, t := range ln.tasks {
			i := stationIndex(t.Stage)
			b := byStation[i]
			if b == nil {
				b = &tyBucket{}
				byStation[i] = b
			}
			if taskBlocked(t) {
				b.side = append(b.side, t)
			} else {
				b.main = append(b.main, t)
			}
		}

		for i := 0; i < nSt; i++ {
			x := stX(i)
			main[x] = '○'
			b := byStation[i]
			if b == nil {
				continue
			}
			// Worst gate among all tasks at this station drives the signal.
			w := worstTask(append(append([]Task{}, b.main...), b.side...))
			tyPlace(main, x+1, []rune(signalGlyph(w.Criticality, w.Freshness)))
			// Mainline: one train, or an honest "▣N" count chip when several share the cell.
			if len(b.main) > 0 {
				tyPlace(main, x+2, []rune(stationGlyph(b.main, maxMass)))
			}
			// Siding: blocked work, collapsed the same way, terminated by a bumper.
			if len(b.side) > 0 {
				cx := x + 2
				tyPlace(siding, cx, []rune("╰─"))
				chip := []rune(stationGlyph(b.side, maxMass))
				tyPlace(siding, cx+2, chip)
				tyPlace(siding, cx+2+len(chip), []rune(sidingBumper(ln.class)))
			}
		}

		b.WriteString(tyRow(main))
		b.WriteByte('\n')
		if tyHasContent(siding) {
			b.WriteString(tyRow(siding))
			b.WriteByte('\n')
		}
	}

	b.WriteString(trainyardLegend())
	return strings.TrimRight(b.String(), "\n")
}

func trainyardLegend() string {
	return C("mut", "  ○ station · ► clear ▷ warn ■ stop ✖ DARK ◌ unknown · train caps=pace [stalled <slow (norm) >fast, shade=age · ▣N=N trains · ┤ siding=blocked · WIT=witnessed terminus")
}
