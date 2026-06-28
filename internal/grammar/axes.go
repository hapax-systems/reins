package grammar

import (
	"fmt"
	"strings"
)

// The six CASE-ROLE AXES (the representational framework, E7.2). Each axis is a pane answering ONE
// case-role question, carrying the five-tuple contract ⟨question · state(incl. explicit ∅) · controls
// (the guard) · fold-provenance · named-blind-spot⟩ — the contract IS the acceptance gate. A2+A4 fold
// live today; A1/A3/A5/A6 are PROJECTION-PENDING (or PARTIAL) and badged honestly so a thin surface is
// never read as authoritative (anti-false-green / Gate-13). Sources: the representational-
// framework + split-layout-lens-scope design notes; the per-axis fold code where built.
type Axis struct {
	ID         string // A1..A6
	Name       string // Identity, Process, Purpose, Authority+Capability, World, Relational
	Question   string // the case-role interrogative the pane resolves
	Status     string // "built" | "partial" | "pending"
	Maps       string // the command the axis folds into today ("" when no dedicated pane yet)
	State      string // five-tuple: the state it shows, incl. the explicit ∅
	Controls   string // five-tuple: the guard
	Provenance string // five-tuple: fold-provenance (which events/sources produced it)
	BlindSpot  string // five-tuple: the named blind-spot (what it structurally CANNOT see)
}

// Axes returns the six case-role axes in framework order.
func Axes() []Axis {
	return []Axis{
		{
			ID: "A1", Name: "Identity", Status: "pending", Maps: ":sessions",
			Question:   "who is acting — what is this entity's role?",
			State:      "named role string; ∅ = unidentified / ambient (anonymous or system events)",
			Controls:   "role-visibility AIR policy + identity-verification scope",
			Provenance: "turn role · session role · event actor field",
			BlindSpot:  "does not distinguish INTENT from CAPABILITY — that is A4 (role is structural membership, not agency)",
		},
		{
			ID: "A2", Name: "Process", Status: "built", Maps: ":readiness",
			Question:   "where in the process is this entity — which lifecycle stage / checkpoint?",
			State:      "stage + predicted_stage + blocker + readiness posture; ∅ = no task claimed / completed",
			Controls:   "authority gates stage transitions (only some roles may advance); AIR guards stage + gate detail",
			Provenance: "coord_event_log stage-transition events; on-air: skeleton only (stage name, blocker glyph, readiness)",
			BlindSpot:  "does not model AUTHORITY to change stage — that is A4 (A2 is observation-only: readiness, not authorization)",
		},
		{
			ID: "A3", Name: "Purpose", Status: "pending", Maps: ":domains",
			Question:   "what is the purview / scope / mandate — what problem space?",
			State:      "domain name + topic classification; ∅ = unclassified / cross-domain",
			Controls:   "editorial — domain taxonomy is instance-configured, operator-overridable (not engine-hardcoded)",
			Provenance: "domain registry (frozen per instance) · topic_interest_engine score · reclassification events",
			BlindSpot:  "does not distinguish ACTUAL IMPACT from assigned scope (A5); nor authority over the domain (A4/A6). The A3 BRIDGE = the DOI fold (producer-gated, E1)",
		},
		{
			ID: "A4", Name: "Authority+Capability", Status: "built", Maps: ":capabilities",
			Question:   "what authority does it hold to act; what capabilities can it exercise; what binds it?",
			State:      "capability status {ok | blocked | preview-only | support-only} + authority signature; ∅ = no authority asserted / no capability mapped",
			Controls:   "AUTHORITY — the hardest gate: never-mint; route only through the governed COMMAND surface; AIR withholds authority detail on air",
			Provenance: "capability-routing ledger · route-envelope authority packets · spend receipts (on-air: skeleton; bodies default-deny)",
			BlindSpot:  "does not model whether the authority is APPROPRIATE (A6 — accountable to whom); nor runtime correctness (A5 — did it work)",
		},
		{
			ID: "A5", Name: "World", Status: "partial", Maps: ":dynamics",
			Question:   "how is the whole system behaving — what are the live effects + feedback?",
			State:      "graph (V,E); node state {active|blocked|idle}; edge polarity ± delay; loop type R/B; ∅ = acyclic / no current feedback",
			Controls:   "OBSERVATION ONLY — read-only, never authorizes action; inferred signs/archetypes carry visible provenance, operator-correctable",
			Provenance: "relation-derivation (Qdrant cosine-kNN · topic facets · FCA concept-lattice · LLM-KG); inferred ≠ asserted; time-series sign for rates",
			BlindSpot:  "Tier-2 LIVE DOMINANCE blocked pending the rate-substrate decision; whether a causal direction is DESIRABLE is A6",
		},
		{
			ID: "A6", Name: "Relational", Status: "pending", Maps: "",
			Question:   "who is affected — what consent / accountability lines exist; who can contest it?",
			State:      "stakeholder roster + consent status {consented | withheld | no-input | deferred}; ∅ = no stakeholders / UNILATERAL authority (dangerous flag)",
			Controls:   "CONSENT — who authorized this frame's visibility; bimodal AIR (present-at-hand ⟵ operator only · air ⟵ default-deny, N-sec hold, dump/kill)",
			Provenance: "consent ledger (NOT yet landed) · the per-turn provenance-glyph ladder (who authored each block) · AIR-safe redaction",
			BlindSpot:  "does not model whether the CONSENT MECHANISM ITSELF is legitimate — that is meta/governance level; A6 is object-level",
		},
	}
}

// axisStatusGlyph carries the build status monochrome-safe: ● built (folds live) · ◐ partial · ○
// projection-pending (badged — a thin/unaudited surface, never read as authoritative).
func axisStatusGlyph(status string) string {
	switch status {
	case "built":
		return C("grn", "●")
	case "partial":
		return C("yel", "◐")
	default:
		return C("mut", "○")
	}
}

func axisStatusWord(status string) string {
	switch status {
	case "built":
		return "folds live"
	case "partial":
		return "partial — Tier-1 only"
	default:
		return "projection-pending — badged"
	}
}

// RenderAxisHeader situates the axis list (the case-role lattice).
func RenderAxisHeader() string {
	return C("mut", fmt.Sprintf(" %-3s %-1s %-20s %s", "AX", "·", "CASE-ROLE", "QUESTION"))
}

// RenderAxisRow is one axis row: id · status-glyph · name · question (clipped). Axis content is design
// metadata (non-sensitive) — it is the same on or off air.
func RenderAxisRow(a Axis, w int) string {
	if w < 24 {
		w = 24
	}
	head := fmt.Sprintf(" %-3s %s %-20s ", C("brt", a.ID), axisStatusGlyph(a.Status), C("pri", a.Name))
	q := C("2nd", a.Question)
	row := head + q
	return clipRunes(row, w)
}

// RenderAxisDetail renders the focused axis's FULL five-tuple contract — the acceptance gate made
// legible (a pane lacking a complete contract is projection-pending).
func RenderAxisDetail(a Axis, w int) string {
	if w < 28 {
		w = 28
	}
	var b strings.Builder
	maps := a.Maps
	if maps == "" {
		maps = C("mut", "no dedicated pane yet")
	} else {
		maps = C("2nd", maps)
	}
	b.WriteString(" " + C("brt", a.ID+" · "+a.Name) + "  " + axisStatusGlyph(a.Status) + C("mut", " "+axisStatusWord(a.Status)) + C("mut", "  → ") + maps + "\n")
	bw := w - 2
	if bw < 10 {
		bw = 10
	}
	b.WriteString(" " + C("border", strings.Repeat("─", bw)) + "\n")
	field := func(label, val string) {
		b.WriteString(" " + C("mut", fmt.Sprintf("%-12s", label)) + wrapInto(val, w-14, 14) + "\n")
	}
	field("question", a.Question)
	field("state ∅", a.State)
	field("controls", a.Controls)
	field("provenance", a.Provenance)
	field("blind-spot", a.BlindSpot)
	return strings.TrimRight(b.String(), "\n")
}

// wrapInto soft-wraps val to width cols, indenting continuation lines by indent spaces.
func wrapInto(val string, width, indent int) string {
	if width < 8 {
		width = 8
	}
	words := strings.Fields(val)
	var lines []string
	cur := ""
	for _, wd := range words {
		if cur == "" {
			cur = wd
		} else if len(cur)+1+len(wd) <= width {
			cur += " " + wd
		} else {
			lines = append(lines, cur)
			cur = wd
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	pad := strings.Repeat(" ", indent)
	for i := 1; i < len(lines); i++ {
		lines[i] = pad + lines[i]
	}
	return strings.Join(lines, "\n")
}
