package grammar

import (
	"fmt"
	"sort"
	"strings"
)

// EconCell is the (task×capability) economics READ-PROJECTION the :dispatch partial-order surface folds
// (design wm4nd9pkx). DERIVED fields (Task/Capability/CostUSD/Held) come from the DispatchRecord ledger
// Reins already reads; v̂/Fit/Conf/ValueStatus come from dev2's STEP-7 producer (gate_outcome →
// route_posteriors → SpendReceipt), which is UNBUILT — so ValueHat/CostUSD/Fit are POINTERS and
// ValueStatus is an explicit enum: nil/"absent" renders ABSENT, never 0 (the never-false-green
// discipline already governing DispatchRecord.CostUSD).
type EconCell struct {
	Task        string   // cc_task — AIR-sensitive (redacts)
	Capability  string   // routing capability — structural, airs
	ValueHat    *float64 // v̂ posterior value (nil ⇔ ValueStatus "absent")
	ValueStatus string   // "measured" | "projected" | "absent" — SSOT for v̂ trust / Pareto-testability
	CostUSD     *float64 // ĉ measured spend (nil = UNMEASURED)
	Fit         *float64 // candidate-set predicate input
	Conf        string   // μ as a confidence-ladder WORD (observed/inferred/…), never a scalar
	Held        bool     // gate-state — ORTHOGONAL to dominance
	HeldReason  string
}

// EconBand is the partition region — a NAMED UNORDERED set, never a tier/rank.
type EconBand int

const (
	BandFrontier     EconBand = iota // non-dominated on ⟨v̂↑, ĉ↓⟩
	BandDominated                    // beaten by ≥1 frontier cell (carries the cover relation)
	BandIncomparable                 // cannot be Pareto-tested (value/cost absent/projected/withheld)
)

// EconPlacement is one cell's band assignment + (for dominated) the honest cover relation.
type EconPlacement struct {
	Cell        EconCell
	Band        EconBand
	DominatedBy []string // frontier capabilities that cover this cell (BandDominated)
	BindingAxis []string // per dominator: "v̂" | "ĉ" | "v̂,ĉ"
	BlockReason string   // BandIncomparable: why it can't be tested
}

// EconPartition is the full classified surface — band-ordered, name-sorted within band (a stable,
// NON-ORDINAL layout: vertical position carries no rank).
type EconPartition struct {
	Placements            []EconPlacement
	NFront, NDom, NIncomp int
	Sealed                bool   // an economic axis denied on air ⇒ frontier NOT computed
	Basis                 string // header declaration
}

// econComparable: a cell is Pareto-testable on ⟨v̂,ĉ⟩ only when BOTH axes are measured.
func econComparable(c EconCell) bool {
	return c.ValueStatus == "measured" && c.ValueHat != nil && c.CostUSD != nil
}

// econDominates: a covers b iff a is ≥ on value AND ≤ on cost AND strictly better on at least one.
// Equal-on-both cells do NOT dominate each other → both stay on the frontier (epistemic peers).
func econDominates(a, b EconCell) bool {
	return *a.ValueHat >= *b.ValueHat && *a.CostUSD <= *b.CostUSD &&
		(*a.ValueHat > *b.ValueHat || *a.CostUSD < *b.CostUSD)
}

func econBindingAxis(a, b EconCell) string {
	betterV := *a.ValueHat > *b.ValueHat
	cheaper := *a.CostUSD < *b.CostUSD
	switch {
	case betterV && cheaper:
		return "v̂,ĉ"
	case betterV:
		return "v̂"
	default:
		return "ĉ"
	}
}

func econBlockReason(c EconCell) string {
	switch {
	case c.ValueStatus == "projected":
		return "value projected"
	case c.ValueStatus != "measured" || c.ValueHat == nil:
		return "value absent"
	case c.CostUSD == nil:
		return "cost UNMEASURED"
	default:
		return "incomparable"
	}
}

// econValueStatusGlyph maps value_status to the EXISTING confidence-ladder alphabet (closed set).
func econValueStatusGlyph(status string) string {
	switch status {
	case "measured":
		return "◉"
	case "projected":
		return "◍"
	default:
		return "·"
	}
}

// econRow renders one cell: gate-glyph + capability (structural, airs) + value (status glyph + raw v̂
// only when measured) + cost token + μ glyph + redactable task. NO normalized bar — raw data only, so
// vertical position never reads as rank (the scalar-via-gestalt leak). On air, ĉ and the task redact.
func econRow(pl EconPlacement, airOn bool, costAirs bool) string {
	c := pl.Cell
	gate := C("grn", "▸")
	held := ""
	if c.Held {
		gate = C("red", "✖")
		held = C("red", " ✖held")
	}
	val := C("mut", "val· absent ")
	switch {
	case c.ValueStatus == "measured" && c.ValueHat != nil:
		// v̂ is an economic estimate, NOT a structural-skeleton field → it DENIES on air (not in the AIR
		// allowlist, same posture as cost). Redact the magnitude so the SEALED partition is truly
		// byte-identical regardless of the withheld v̂ (the derived-channel/composition-leak floor — the
		// leak STEP-7's measured cells would otherwise arm: the seal claim covered cost/task, not value).
		if airOn {
			val = C("mut", "val◉ ▒▒▒  ")
		} else {
			val = C("eme", fmt.Sprintf("val◉ %-6.1f", *c.ValueHat))
		}
	case c.ValueStatus == "projected":
		val = C("mut", "val◍ proj  ")
	}
	cost := C("mut", "$UNMEAS")
	switch {
	case airOn && !costAirs:
		cost = C("mut", "$ ▒▒▒")
	case c.CostUSD != nil:
		cost = C("mut", fmt.Sprintf("$%.4f", *c.CostUSD))
	}
	mu := "μ" + econValueStatusGlyph(c.Conf)
	if g, ok := statusGlyphs[c.Conf]; ok {
		mu = "μ" + g
	}
	task := ""
	if strings.TrimSpace(c.Task) != "" {
		t := c.Task
		if airOn {
			t = "▒▒▒"
		}
		task = C("mut", "  "+t)
	}
	return gate + " " + C(LaneToken(c.Capability), pad(c.Capability, 20)) + " " + val + "  " + cost + "  " + mu + task + held
}

// RenderEconPartition is the three-band SCAN surface (design wm4nd9pkx §1): named UNORDERED bands, rows
// name-sorted within band. When the frontier is empty off-air it reads UNDEFINED (v̂ producer unbuilt —
// honest, never a fabricated rank); on air with ĉ denied the partition SEALS (all incomparable, byte-
// identical regardless of the withheld magnitudes). costAirs reflects the live AIR policy (deny $cost).
func RenderEconPartition(p EconPartition, airOn bool, costAirs bool, w int) string {
	rule := strings.Repeat("─", clampW(w-2, 10, 120))
	var b strings.Builder
	head := "⟨v̂,ĉ⟩ partial order"
	if p.Sealed {
		head += " · frontier SEALED — " + p.Basis
	}
	b.WriteString(" " + C("brt", head) + C("mut", "   (sets — not ranked)") + "\n")
	b.WriteString(" " + C("border", rule) + "\n")

	emit := func(label, gloss string, band EconBand, undefined string) {
		n := 0
		for _, pl := range p.Placements {
			if pl.Band == band {
				n++
			}
		}
		if band == BandFrontier && n == 0 && !p.Sealed {
			b.WriteString(" " + C("yel", "FRONTIER UNDEFINED") + C("mut", " — "+undefined) + "\n")
			return
		}
		if n == 0 {
			return
		}
		b.WriteString(" " + C("2nd", label) + C("mut", "  "+gloss) + "\n")
		for _, pl := range p.Placements {
			if pl.Band == band {
				b.WriteString("   " + econRow(pl, airOn, costAirs) + "\n")
			}
		}
	}
	emit("FRONTIER", "non-dominated on ⟨v̂,ĉ⟩", BandFrontier, "v̂ absent (STEP-7 producer unbuilt)")
	emit("DOMINATED", "beaten by a frontier cell (focus → which · which axis)", BandDominated, "")
	emit("INCOMPARABLE", "cannot be Pareto-tested", BandIncomparable, "")
	return strings.TrimRight(b.String(), "\n")
}

func clampW(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ClassifyEcon is the PURE, AIR-aware partitioner. airAllow reports whether an economic axis ("value"
// or "cost") airs. CRITICAL AIR discipline: if EITHER economic axis is denied on air, it SEALS — the
// partition is NOT computed over the denied axis (every cell → BandIncomparable "axis withheld"), so the
// denied magnitude can NEVER steer band membership (the rendering would otherwise leak it). Within-band
// order is sort.Strings(capability) — the anti-rank key (same device RenderUtilization uses). No DOI.
func ClassifyEcon(cells []EconCell, airOn bool, airAllow func(axis string) bool) EconPartition {
	valueAirs := !airOn || airAllow("value")
	costAirs := !airOn || airAllow("cost")

	sorted := append([]EconCell(nil), cells...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Capability < sorted[j].Capability })

	if airOn && (!valueAirs || !costAirs) {
		p := EconPartition{Sealed: true, Basis: "ĉ WITHHELD on air"}
		if valueAirs && !costAirs {
			p.Basis = "ĉ WITHHELD on air"
		} else if !valueAirs && costAirs {
			p.Basis = "v̂ WITHHELD on air"
		} else {
			p.Basis = "⟨v̂,ĉ⟩ WITHHELD on air"
		}
		for _, c := range sorted {
			p.Placements = append(p.Placements, EconPlacement{Cell: c, Band: BandIncomparable, BlockReason: "axis withheld on air"})
		}
		p.NIncomp = len(sorted)
		return p
	}

	var comp, incomp []EconCell
	for _, c := range sorted {
		if econComparable(c) {
			comp = append(comp, c)
		} else {
			incomp = append(incomp, c)
		}
	}

	frontier := make([]bool, len(comp))
	for i := range comp {
		dominated := false
		for j := range comp {
			if i != j && econDominates(comp[j], comp[i]) {
				dominated = true
				break
			}
		}
		frontier[i] = !dominated
	}

	p := EconPartition{Basis: "⟨v̂,ĉ⟩"}
	for i, c := range comp { // frontier band (name-sorted, since comp is)
		if frontier[i] {
			p.Placements = append(p.Placements, EconPlacement{Cell: c, Band: BandFrontier})
			p.NFront++
		}
	}
	for i, b := range comp { // dominated band, with the cover relation
		if frontier[i] {
			continue
		}
		var by, axes []string
		for j, a := range comp {
			if frontier[j] && econDominates(a, b) {
				by = append(by, a.Capability)
				axes = append(axes, econBindingAxis(a, b))
			}
		}
		p.Placements = append(p.Placements, EconPlacement{Cell: b, Band: BandDominated, DominatedBy: by, BindingAxis: axes})
		p.NDom++
	}
	for _, c := range incomp { // incomparable band
		p.Placements = append(p.Placements, EconPlacement{Cell: c, Band: BandIncomparable, BlockReason: econBlockReason(c)})
		p.NIncomp++
	}
	return p
}
