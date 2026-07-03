package model

// PaneContract (invariant I7 — the forcing-function cure applied to the cockpit): every navigable pane
// declares its five-tuple contract ⟨question · state(incl ∅) · controls(the guard) · fold-provenance ·
// named-blind-spot⟩ — the contract IS the acceptance gate. Mirrors grammar.Axis (the six interrogatives).
// The cockpit-vision design-of-record (~/Documents/Personal/30-areas/hapax/reins-cockpit-vision-design-
// 2026-07-03.md) reconceives the ~30 accreted pages into ~9 projections + doors; this registry contracts
// the KEY projection pages first. Pages not yet contracted are tracked in undeclaredPanes (debt that
// shrinks as each is contracted) — pane_contract_test.go fails if a page is in NEITHER (regression).
type PaneContract struct {
	Question   string // the case-role interrogative the pane resolves
	State      string // what state it shows, incl. the explicit ∅
	Controls   string // the guard — what the operator can do here
	Provenance string // fold-provenance — which events/sources produced it
	BlindSpot  string // what it structurally CANNOT see (named, honest)
}

// PageContracts: the registered page → five-tuple contract map. A page here has a COMPLETE contract
// (asserted by pane_contract_test.go). Add a page by contracting it (fill the five-tuple from the
// design-of-record's screen-rethink) AND removing it from undeclaredPanes.
var PageContracts = map[int]PaneContract{
	PageCoordinator: {
		Question:   "what am I doing right now — the steering conversation I am in",
		State:      "the active lanes + their state + the turn-ladder",
		Controls:   "steer DIRECTION (priority · hold · scope · accept-reject); /verb governed dispatch",
		Provenance: "the live lanes + the governed rail + the witnessed ledger",
		BlindSpot:  "speed/provider/fanout (derived by routing, never steered); live turn-stream until CapabilityIO",
	},
	PageEvents: {
		Question:   "what needs me across all n-DLC right now",
		State:      "DOI-ranked attention/triage items",
		Controls:   "triage / drill an item",
		Provenance: "the reactive event feed + the DOI fold",
		BlindSpot:  "proactive priority (that's A3 Purpose); items below the DOI threshold",
	},
	PageAxes: {
		Question:   "the six case-role interrogatives (A1–A6)",
		State:      "the axes + their fold status",
		Controls:   "cycle the axes / drill an axis contract",
		Provenance: "the representational framework",
		BlindSpot:  "axes not yet folded (A1/A3/A5/A6 projection-pending)",
	},
	PageIdentity: {
		Question:   "who/what is acting — the principal roster",
		State:      "lanes/agents/sessions/capabilities-as-actors",
		Controls:   "inspect a principal",
		Provenance: "the role/actor/owner derivation",
		BlindSpot:  "role (A3 structural membership) ≠ agency; authority (A4) ≠ identity",
	},
	PageReadiness: {
		Question:   "what stage is this work in / what is legally next",
		State:      "the FSM/lifecycle/gate-stack",
		Controls:   "observe the gate-stack (observation, NOT authorization)",
		Provenance: "the lifecycle registry + the gate projections",
		BlindSpot:  "authorization (that's A4); capability (A4)",
	},
	PageDispatch: {
		Question:   "what should I be working on / the shape of my demand",
		State:      "the Pareto partial-order frontier over ⟨v̂, ĉ, fit, u, μ⟩ + an INCOMPARABLE/HOLD band",
		Controls:   "navigate the frontier (NO scalar rank)",
		Provenance: "the demand vector + the DOI",
		BlindSpot:  "a legible priority scalar (deliberately excised); capability supply (A4)",
	},
	PageCaps: {
		Question:   "what can do this / what route will it take",
		State:      "capability-status (NOT platform) + route evidence + cost",
		Controls:   "inspect a capability / route",
		Provenance: "the capability registry + route receipts",
		BlindSpot:  "the routing DECISION (spine-minted, never reins); demand (A3)",
	},
	PageObserve: {
		Question:   "how is the broader system behaving",
		State:      "fleet/health/drift/agents/governance/cost (read-only)",
		Controls:   "observe (read-only)",
		Provenance: "verified-host signals only",
		BlindSpot:  "unverified-host signals (the M4 boutique-liveness lesson); action (that's the Helm)",
	},
	PageRelational: {
		Question:   "what is my consent posture / who has access / who must consent",
		State:      "the consent/access-control ledger per principal",
		Controls:   "inspect a consent boundary",
		Provenance: "the consent ledger + the tri-audience egress policy",
		BlindSpot:  "agency (A1); authority (A4) — consent is neither",
	},
	PageVault: {
		Question:   "my memory / context / life-DLC",
		State:      "vault titles + links + the open-n-DLC set",
		Controls:   "navigate the vault (bodies default-deny)",
		Provenance: "the Obsidian vault + intake + deck + presence + rdlc",
		BlindSpot:  "vault BODIES (default-deny); dark lifecycles (rdlc/ldlc/presence) until each model M exists",
	},
	PageCommands: {
		Question:   "did my action land / is it verified",
		State:      "the witnessed command ledger (demand+verdict+effect) + the integrity state",
		Controls:   "inspect a receipt",
		Provenance: "the signed hash-chain ledger + the out-of-band anchor",
		BlindSpot:  "tail-truncation/whole-replacement/key-substitution (need the out-of-band anchor)",
	},
	PageLifecycles: {
		Question:   "what lifecycle contracts are declared (the DLC taxonomy + per-lifecycle schema)",
		State:      "the registered lifecycles + their posture/maturity",
		Controls:   "inspect a lifecycle contract",
		Provenance: "the lifecycle registry",
		BlindSpot:  "the lifecycle MODEL M (posture/maturity are declared, not source-backed, until M exists)",
	},
	PageDynamics: {
		Question:   "how is the system causally structured (the system-dynamics graph)",
		State:      "the dynamics graph (nodes/edges/layers)",
		Controls:   "navigate the graph / drill a node",
		Provenance: "the system-dynamics-map (Tier-1, honestly hand-seeded-labeled)",
		BlindSpot:  "Tier-2 live-dominance (pending the relate producer); simulated/inferred signs",
	},
	PageLoops: {
		Question:   "what are the causal feedback loops",
		State:      "the computed causal loops (Tarjan SCC + sign parity)",
		Controls:   "inspect a loop",
		Provenance: "the dynamics graph + the loop computation",
		BlindSpot:  "loops over unbuilt edges; the live-dominance Tier-2",
	},
	PageTraces: {
		Question:   "what did the LLM calls cost / how long did they take",
		State:      "the LLM trace rows (model/tokens/cost/latency)",
		Controls:   "inspect a trace",
		Provenance: "the Langfuse trace fold",
		BlindSpot:  "the trace BODIES (input/output = operator content, never enter the row); the routing decision (A4)",
	},
	PageEpistemics: {
		Question:   "what is the evidence/provenance posture",
		State:      "the epistemic rows (provenance/freshness/authority per subject)",
		Controls:   "inspect an epistemic row",
		Provenance: "the epistemic engine over the sources",
		BlindSpot:  "raw source bodies; the DOI ranking (that's the fold, not a row)",
	},
	PageIntake: {
		Question:   "what is the intake (requests / P0 / security signals)",
		State:      "the intake summaries (bounded metadata/counts only)",
		Controls:   "triage an intake item",
		Provenance: "the request/p0/security intake snapshots",
		BlindSpot:  "intake BODIES (notification/message text = default-deny); the DOI priority",
	},
}

// doorPanes: self-describing registries DEMOTED TO DOORS (the decoder-in-band — Latour immutable-mobile).
// These earn NO steady-state projection contract: they are full-screen drill-ins on demand (Gate-13
// cold-read recovery depends on the decoder traveling WITH the artifact). They are neither contracted nor
// debt — a door is a deliberate non-projection.
var doorPanes = map[int]bool{
	PageHelp: true, PageLegend: true, PageWindows: true, PageSurfaces: true,
}

// undeclaredPanes: projection pages not yet contracted (tracked debt). A page here MUST be contracted
// (moved to PageContracts) before it earns a steady-state screen in the rethought cockpit.
// pane_contract_test.go fails if a navigable page is in NONE of PageContracts / doorPanes / undeclaredPanes.
var undeclaredPanes = map[int]bool{
	PageTasks: true, PageSessions: true, PageIntent: true, PageDomains: true, PageYard: true,
	PageSessionTurns: true, PageRdlc: true, PagePresence: true, PageDeck: true,
}
