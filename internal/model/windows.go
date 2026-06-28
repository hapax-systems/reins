package model

import (
	"fmt"
	"sort"
	"strings"
)

// WindowDef is the seed of the lifecycle/window registry. The current definitions are still
// compiled in, but they already separate engine reference windows from instance lifecycle windows
// so Reins does not treat one operator's n-DLC taxonomy as the product ontology.
type WindowDef struct {
	Key       string
	ID        string
	Name      string
	Short     string
	Page      int
	Scope     string // engine | instance
	Lifecycle string // engine | substrate | sdlc | system
	Kind      string // stream | registry | fleet | map | reference
}

// SplitPairDef declares the wide-terminal composition contract for a window. The source pane is
// intentionally explicit: split panes are relationship operators, not arbitrary two-up layouts.
type SplitPairDef struct {
	Page             int
	Source           string
	Target           string
	Join             string
	Mode             string // linked | reference
	SourceCursor     string // session-row | session-anchor
	TargetReactivity string // source-linked | independent
	TargetCursor     string // context-scroll | none | intake-bucket | map-element | epistemic-row
	TargetScrollable bool
	SourceOwnedVerbs []string
	Contract         string
}

type PaneRelationship string

const (
	PaneRelationshipLinked   PaneRelationship = "linked"
	PaneRelationshipAnchored PaneRelationship = "anchored"
)

type PaneProfile string

const (
	PaneLinkedCompact      PaneProfile = "linked-compact"
	PaneLinkedScrollable   PaneProfile = "linked-scrollable"
	PaneLinkedTargetCursor PaneProfile = "linked-target-cursor"
	PaneAnchoredTarget     PaneProfile = "anchored-target"
	PaneAnchoredReference  PaneProfile = "anchored-reference"
)

const (
	splitModeLinked        = "linked"
	splitModeReference     = "reference"
	splitSourceSessionRow  = "session-row"
	splitSourceAnchor      = "session-anchor"
	splitTargetLinked      = "source-linked"
	splitTargetIndependent = "independent"
	splitTargetScroll      = "context-scroll"
	splitTargetNone        = "none"
	splitTargetIntake      = "intake-bucket"
	splitTargetMapElement  = "map-element"
	splitTargetEpistemic   = "epistemic-row"
)

func (s SplitPairDef) Reactive() bool {
	if s.TargetReactivity != "" {
		return s.TargetReactivity == splitTargetLinked
	}
	return s.Mode == splitModeLinked
}

func (s SplitPairDef) Relationship() PaneRelationship {
	if s.Reactive() {
		return PaneRelationshipLinked
	}
	return PaneRelationshipAnchored
}

func (s SplitPairDef) PaneProfile() PaneProfile {
	if s.Reactive() {
		if s.TargetUsesNP() {
			return PaneLinkedTargetCursor
		}
		if s.TargetScrollable || s.TargetCursor == splitTargetScroll {
			return PaneLinkedScrollable
		}
		return PaneLinkedCompact
	}
	if s.TargetUsesNP() {
		return PaneAnchoredTarget
	}
	return PaneAnchoredReference
}

func (s SplitPairDef) PaneProfileLabel() string {
	switch s.PaneProfile() {
	case PaneLinkedCompact:
		return "link:compact"
	case PaneLinkedScrollable:
		return "link:scroll"
	case PaneLinkedTargetCursor:
		return "link:target"
	case PaneAnchoredTarget:
		return "anchor:target"
	case PaneAnchoredReference:
		return "anchor:ref"
	default:
		return string(s.PaneProfile())
	}
}

func (s SplitPairDef) RelationLabel() string {
	if s.Reactive() {
		return s.Source + " -> " + s.Target + " by " + s.Join
	}
	return s.Source + " + " + s.Target + " as " + s.Join
}

func (s SplitPairDef) SourceOwns(verb string) bool {
	for _, candidate := range s.SourceOwnedVerbs {
		if candidate == verb {
			return true
		}
	}
	return false
}

func (s SplitPairDef) SourceNavLabel() string {
	if s.Reactive() {
		return "source->ctx"
	}
	return "lane anchor"
}

func (s SplitPairDef) TargetUsesNP() bool {
	switch s.TargetCursor {
	case splitTargetIntake, splitTargetMapElement, splitTargetEpistemic:
		return true
	default:
		return false
	}
}

func (s SplitPairDef) TargetNPLabels() (string, string, bool) {
	switch s.TargetCursor {
	case splitTargetIntake:
		return "intake target", "intake", true
	case splitTargetMapElement:
		return "map target", "map", true
	case splitTargetEpistemic:
		return "evidence target", "evidence", true
	default:
		return "", "", false
	}
}

func (s SplitPairDef) TargetUsesPageJK() bool {
	return s.TargetCursor == splitTargetIntake
}

func (s SplitPairDef) TargetIsPassiveScroll() bool {
	return s.TargetCursor == splitTargetScroll
}

func splitSessionVerbs() []string {
	return []string{"detail", "resume", "yank"}
}

var windowRegistry = []WindowDef{
	{Key: "1", ID: "events", Name: "events", Short: "events", Page: PageEvents, Scope: "instance", Lifecycle: "substrate", Kind: "stream"},
	{Key: "2", ID: "tasks", Name: "tasks", Short: "tasks", Page: PageTasks, Scope: "instance", Lifecycle: "sdlc", Kind: "registry"},
	{Key: "3", ID: "sessions", Name: "sessions", Short: "sess", Page: PageSessions, Scope: "instance", Lifecycle: "sdlc", Kind: "fleet"},
	{Key: "Y", ID: "yard", Name: "yard", Short: "yard", Page: PageYard, Scope: "instance", Lifecycle: "sdlc", Kind: "cockpit"},
	{Key: "Z", ID: "coordinator", Name: "yard coordinator", Short: "coord", Page: PageCoordinator, Scope: "instance", Lifecycle: "sdlc", Kind: "cockpit"},
	{Key: "R", ID: "readiness", Name: "readiness", Short: "ready", Page: PageReadiness, Scope: "instance", Lifecycle: "sdlc", Kind: "gate"},
	{Key: "I", ID: "intake", Name: "intake observations", Short: "obs", Page: PageIntake, Scope: "instance", Lifecycle: "intake", Kind: "triage"},
	{Key: "C", ID: "capabilities", Name: "capabilities", Short: "caps", Page: PageCaps, Scope: "instance", Lifecycle: "routing", Kind: "matrix"},
	{Key: "4", ID: "dynamics", Name: "dynamics", Short: "dyn", Page: PageDynamics, Scope: "instance", Lifecycle: "system", Kind: "map"},
	{Key: "A", ID: "loops", Name: "causal loops", Short: "loops", Page: PageLoops, Scope: "instance", Lifecycle: "system", Kind: "graph"},
	{Key: "X", ID: "axes", Name: "case-role axes", Short: "axes", Page: PageAxes, Scope: "instance", Lifecycle: "system", Kind: "reference"},
	{Key: "E", ID: "epistemics", Name: "epistemics", Short: "epi", Page: PageEpistemics, Scope: "instance", Lifecycle: "system", Kind: "lens"},
	{Key: "5", ID: "help", Name: "help", Short: "help", Page: PageHelp, Scope: "engine", Lifecycle: "engine", Kind: "reference"},
	{Key: "6", ID: "commands", Name: "commands", Short: "cmds", Page: PageCommands, Scope: "engine", Lifecycle: "engine", Kind: "registry"},
	{Key: "7", ID: "windows", Name: "windows", Short: "wins", Page: PageWindows, Scope: "engine", Lifecycle: "engine", Kind: "registry"},
	{Key: "8", ID: "intent", Name: "intent", Short: "intent", Page: PageIntent, Scope: "engine", Lifecycle: "engine", Kind: "review"},
	{Key: "9", ID: "surfaces", Name: "surfaces", Short: "surf", Page: PageSurfaces, Scope: "engine", Lifecycle: "engine", Kind: "registry"},
	{Key: "0", ID: "domains", Name: "domains", Short: "doms", Page: PageDomains, Scope: "engine", Lifecycle: "engine", Kind: "registry"},
	{Key: "L", ID: "lifecycles", Name: "lifecycles", Short: "life", Page: PageLifecycles, Scope: "engine", Lifecycle: "engine", Kind: "registry"},
	{Key: "T", ID: "traces", Name: "traces", Short: "traces", Page: PageTraces, Scope: "instance", Lifecycle: "substrate", Kind: "stream"},
	{Key: "D", ID: "dispatch", Name: "dispatch", Short: "disp", Page: PageDispatch, Scope: "instance", Lifecycle: "sdlc", Kind: "stream"},
	{Key: "U", ID: "turns", Name: "session turns", Short: "turns", Page: PageSessionTurns, Scope: "instance", Lifecycle: "substrate", Kind: "stream"},
	{Key: "?", ID: "legend", Name: "legend", Short: "legend", Page: PageLegend, Scope: "engine", Lifecycle: "engine", Kind: "reference"},
}

var splitPairRegistry = []SplitPairDef{
	{Page: PageEvents, Source: "sessions", Target: "events", Join: "actor/task", Mode: splitModeLinked, SourceCursor: splitSourceSessionRow, TargetReactivity: splitTargetLinked, TargetCursor: splitTargetNone, SourceOwnedVerbs: splitSessionVerbs(), Contract: "lane focus brushes event neighborhood"},
	{Page: PageTasks, Source: "sessions", Target: "task work", Join: "claimed_task", Mode: splitModeLinked, SourceCursor: splitSourceSessionRow, TargetReactivity: splitTargetLinked, TargetCursor: splitTargetNone, SourceOwnedVerbs: splitSessionVerbs(), Contract: "lane focus resolves claimed task"},
	{Page: PageSessions, Source: "sessions", Target: "lane readiness", Join: "role", Mode: splitModeLinked, SourceCursor: splitSourceSessionRow, TargetReactivity: splitTargetLinked, TargetCursor: splitTargetNone, SourceOwnedVerbs: splitSessionVerbs(), Contract: "lane focus explains readiness constraints"},
	{Page: PageYard, Source: "sessions", Target: "yard drilldown", Join: "lane/task", Mode: splitModeLinked, SourceCursor: splitSourceSessionRow, TargetReactivity: splitTargetLinked, TargetCursor: splitTargetScroll, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "lane focus drives trainyard gate/task/event drilldown"},
	{Page: PageCoordinator, Source: "lattice lens", Target: "coordinator", Join: "selection/lattice-descent", Mode: splitModeLinked, SourceCursor: splitSourceSessionRow, TargetReactivity: splitTargetLinked, TargetCursor: splitTargetScroll, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "the miller-column lens selection drives the coordinator context (page-composed via HConcat, not session-frozen)"},
	{Page: PageReadiness, Source: "sessions", Target: "gate stack", Join: "role/claimed_task", Mode: splitModeLinked, SourceCursor: splitSourceSessionRow, TargetReactivity: splitTargetLinked, TargetCursor: splitTargetScroll, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "lane focus explains source freshness, blockers, claimed-task gate, and legal route"},
	{Page: PageIntake, Source: "sessions", Target: "intake observations", Join: "actor/claimed_task", Mode: splitModeLinked, SourceCursor: splitSourceSessionRow, TargetReactivity: splitTargetLinked, TargetCursor: splitTargetIntake, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "lane focus brushes intake demand by actor, claimed task, and ambient unassigned counts"},
	{Page: PageCaps, Source: "sessions", Target: "capability fit", Join: "role/platform", Mode: splitModeLinked, SourceCursor: splitSourceSessionRow, TargetReactivity: splitTargetLinked, TargetCursor: splitTargetScroll, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "lane focus derives fit evidence"},
	{Page: PageDynamics, Source: "sessions", Target: "dynamics map", Join: "system topology", Mode: splitModeReference, SourceCursor: splitSourceAnchor, TargetReactivity: splitTargetIndependent, TargetCursor: splitTargetMapElement, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "map scale explains system context while lane source stays actionable"},
	{Page: PageLoops, Source: "sessions", Target: "causal loops", Join: "feedback structure", Mode: splitModeReference, SourceCursor: splitSourceAnchor, TargetReactivity: splitTargetIndependent, TargetCursor: splitTargetScroll, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "computed loop structure explains feedback without simulation while lane source stays actionable"},
	{Page: PageAxes, Source: "sessions", Target: "case-role axes", Join: "framework", Mode: splitModeReference, SourceCursor: splitSourceAnchor, TargetReactivity: splitTargetIndependent, TargetCursor: splitTargetScroll, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "the six axes + five-tuple contracts; self-anchored"},
	{Page: PageEpistemics, Source: "sessions", Target: "epistemic posture", Join: "evidence/provenance", Mode: splitModeReference, SourceCursor: splitSourceAnchor, TargetReactivity: splitTargetIndependent, TargetCursor: splitTargetEpistemic, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "evidence/provenance lens contextualizes the held lane without minting authority"},
	{Page: PageHelp, Source: "sessions", Target: "help reference", Join: "operator orientation", Mode: splitModeReference, SourceCursor: splitSourceAnchor, TargetReactivity: splitTargetIndependent, TargetCursor: splitTargetScroll, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "help explains controls for the held lane/context relation"},
	{Page: PageLegend, Source: "sessions", Target: "legend reference", Join: "glyph decoding", Mode: splitModeReference, SourceCursor: splitSourceAnchor, TargetReactivity: splitTargetIndependent, TargetCursor: splitTargetScroll, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "legend decodes marks and color channels for current source/context"},
	{Page: PageCommands, Source: "sessions", Target: "command catalog", Join: "command grammar", Mode: splitModeReference, SourceCursor: splitSourceAnchor, TargetReactivity: splitTargetIndependent, TargetCursor: splitTargetScroll, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "catalog exposes legal verbs/templates without changing selected source"},
	{Page: PageWindows, Source: "sessions", Target: "window registry", Join: "window topology", Mode: splitModeReference, SourceCursor: splitSourceAnchor, TargetReactivity: splitTargetIndependent, TargetCursor: splitTargetScroll, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "registry shows cycle/split topology while lane source stays anchored"},
	{Page: PageIntent, Source: "sessions", Target: "intent review", Join: "explicit target", Mode: splitModeReference, SourceCursor: splitSourceAnchor, TargetReactivity: splitTargetIndependent, TargetCursor: splitTargetScroll, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "intent target, not lane focus, owns review"},
	{Page: PageSurfaces, Source: "sessions", Target: "surface registry", Join: "affordance registry", Mode: splitModeReference, SourceCursor: splitSourceAnchor, TargetReactivity: splitTargetIndependent, TargetCursor: splitTargetScroll, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "surface registry names available modes/doors for the held lane"},
	{Page: PageDomains, Source: "sessions", Target: "domain registry", Join: "domain lens", Mode: splitModeReference, SourceCursor: splitSourceAnchor, TargetReactivity: splitTargetIndependent, TargetCursor: splitTargetScroll, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "domain lens situates selected lane inside SDLC/RDLC/n-DLC maps"},
	{Page: PageLifecycles, Source: "sessions", Target: "lifecycle registry", Join: "tenant lifecycle", Mode: splitModeReference, SourceCursor: splitSourceAnchor, TargetReactivity: splitTargetIndependent, TargetCursor: splitTargetScroll, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "lifecycle contracts expose SDLC/RDLC/LDLC/n-DLC tenants without assuming one operator taxonomy"},
	{Page: PageTraces, Source: "sessions", Target: "trace feed", Join: "model/latency", Mode: splitModeReference, SourceCursor: splitSourceAnchor, TargetReactivity: splitTargetIndependent, TargetCursor: splitTargetScroll, TargetScrollable: true, SourceOwnedVerbs: splitSessionVerbs(), Contract: "trace feed contextualizes LLM spend without minting authority"},
}

// DomainDef is the extensibility layer above windows: Reins can project SDLC, RDLC, capability
// routing, intake, tool sessions, or future n-DLCs without treating any one operator taxonomy as the
// engine ontology. The Logos terrain terms are used here as grouping hints, not as control routes.
type DomainDef struct {
	ID       string
	Terrain  string
	Depth    string
	Scope    string
	Windows  string
	Surfaces string
	Parity   string
}

var domainRegistry = []DomainDef{
	{ID: "substrate-events", Terrain: "horizon", Depth: "surface", Scope: "instance", Windows: "events", Surfaces: "read-dark,flash", Parity: "event spine, intake/eventlog"},
	{ID: "sdlc-work", Terrain: "field", Depth: "surface", Scope: "instance", Windows: "yard,tasks,sessions", Surfaces: "trainyard-cockpit,whois,session-detail,field-rank,class-select", Parity: "cc-task,claims,session lanes"},
	{ID: "system-dynamics", Terrain: "watershed", Depth: "stratum", Scope: "instance", Windows: "dynamics", Surfaces: "dynamics-map,dyn-scale,wide-context", Parity: "topology,staleness,active flow"},
	{ID: "epistemic-inspection", Terrain: "bedrock", Depth: "stratum", Scope: "instance", Windows: "epistemics,dynamics,intake,domains", Surfaces: "epistemic-posture,observation-feed,dynamics-map,read-dark", Parity: "claims,observations,validation,provenance,authority ceilings"},
	{ID: "command-intent", Terrain: "ground", Depth: "surface", Scope: "engine", Windows: "commands,intent", Surfaces: "command,intent-target,compose", Parity: "query/list/subscribe/sequences"},
	{ID: "capability-routing", Terrain: "watershed", Depth: "core", Scope: "instance", Windows: "capabilities,surfaces,domains", Surfaces: "air-lens,read-dark", Parity: "route descriptors,admission,calibration"},
	{ID: "source-acquisition-routing", Terrain: "horizon", Depth: "stratum", Scope: "instance", Windows: "capabilities,domains,intake", Surfaces: "source-acquisition-router,tool-capability-inventory,read-dark", Parity: "Tavily,Perplexity,Context7,Drive/GitHub source acquisition"},
	{ID: "verifier-floor-routing", Terrain: "bedrock", Depth: "core", Scope: "instance", Windows: "capabilities,readiness,domains", Surfaces: "verifier-floor,gate-stack,read-dark", Parity: "Semgrep,Codecov,CodeQL,tests,runtime witnesses"},
	{ID: "provider-gateway-routing", Terrain: "watershed", Depth: "core", Scope: "instance", Windows: "capabilities,domains,intent", Surfaces: "provider-gateway,tool-capability-inventory,read-dark", Parity: "LiteLLM,OpenRouter,HuggingFace,local inference spend/control"},
	{ID: "publication-media-egress", Terrain: "horizon", Depth: "core", Scope: "instance", Windows: "capabilities,domains,intent", Surfaces: "publication-egress,audio-avsdlc,air-lens", Parity: "research deposit, public social, YouTube, media/audio tools"},
	{ID: "infrastructure-control", Terrain: "bedrock", Depth: "core", Scope: "instance", Windows: "capabilities,domains,intent", Surfaces: "infra-control,read-dark,air-lens", Parity: "backup/storage/network admin surfaces stay receipt-gated"},
	{ID: "governance-readiness", Terrain: "bedrock", Depth: "core", Scope: "instance", Windows: "readiness,yard,capabilities,intent", Surfaces: "gate-stack,read-dark,air-lens,split-context", Parity: "authority_case,parent_spec,route evidence,preflight,receipt"},
	{ID: "intake-observations", Terrain: "horizon", Depth: "stratum", Scope: "instance", Windows: "intake,events,tasks", Surfaces: "observation-feed,observation-detail,filter,hint,yank,split-context,read-dark", Parity: "Obsidian nav,eventlog intake,request/P0/security snapshots"},
	{ID: "research-rdlc", Terrain: "bedrock", Depth: "stratum", Scope: "instance", Windows: "domains,dynamics", Surfaces: "labrack-lens,section-figure-lens,wide-context", Parity: "labrack,section-figure,research corpus"},
	{ID: "tool-sessions", Terrain: "ground", Depth: "surface", Scope: "instance", Windows: "capabilities,sessions", Surfaces: "tool-capability-inventory,session-detail,yank", Parity: "codex/claude/vibe/agy capabilities"},
	{ID: "governance-safety", Terrain: "bedrock", Depth: "core", Scope: "engine", Windows: "commands,intent,legend", Surfaces: "air-lens,read-dark", Parity: "authority,consent,refusals,hygiene"},
	{ID: "future-n-dlc", Terrain: "bedrock", Depth: "core", Scope: "engine", Windows: "domains,windows", Surfaces: "surfaces", Parity: "domain packs,tenant taxonomies"},
}

// SurfaceDef describes transient screens and modes that are not always their own lifecycle window
// but must still be visible, nameable, and governed by the same recognition-over-recall contract.
type SurfaceDef struct {
	ID       string
	Name     string
	Open     string
	Exit     string
	Scope    string
	Kind     string
	AIR      string
	Contract string
}

var surfaceRegistry = []SurfaceDef{
	{ID: "command", Name: "command line", Open: "[:]", Exit: "Enter/Esc", Scope: "engine", Kind: "mode", AIR: "templates safe", Contract: "completion + typed verbs; pure command fold"},
	{ID: "filter", Name: "task filter", Open: "[/] on :tasks", Exit: "Enter/Esc", Scope: "instance", Kind: "mode", AIR: "ids redact", Contract: "narrows task rows; completion offers visible ids"},
	{ID: "hint", Name: "hint/select", Open: "[f] on :tasks", Exit: "pick/Esc", Scope: "instance", Kind: "mode", AIR: "labels only", Contract: "row labels, class filters, blocker jumps"},
	{ID: "field-rank", Name: "task field rank", Open: "[Tab] on :tasks", Exit: "Tab/Esc", Scope: "instance", Kind: "granularity", AIR: "deny blocks yank", Contract: "cell-level cursor over focused row"},
	{ID: "yank", Name: "field yank", Open: "[y] on rows", Exit: "Enter/Esc", Scope: "instance", Kind: "mode", AIR: "default deny", Contract: "row + field navigation; AIR-safe kill-ring"},
	{ID: "class-select", Name: "criticality class", Open: "[V] on :tasks", Exit: "yank/Esc", Scope: "instance", Kind: "selection", AIR: "all ids must pass", Contract: "select visible siblings by criticality"},
	{ID: "whois", Name: "/whois task door", Open: "[Enter] :tasks", Exit: "Esc/Enter/q", Scope: "instance", Kind: "door", AIR: "structure remains", Contract: "SDLC ladder + governed verb stubs"},
	{ID: "session-detail", Name: "/session lane door", Open: "[Enter] :sessions", Exit: "Esc/Enter/q", Scope: "instance", Kind: "door", AIR: "metadata only", Contract: "roster facts + resume-intent stub; no PTY"},
	{ID: "dyn-scale", Name: "dynamics scale", Open: "[,/.] :dynamics", Exit: "cycle/page", Scope: "instance", Kind: "lens", AIR: "nodes redact", Contract: "all/overview/domain/artifact/runtime/evidence"},
	{ID: "intent-target", Name: "intent target", Open: ":intent <target>", Exit: "page cycle", Scope: "engine", Kind: "review", AIR: "subject safe", Contract: "resume/dispatch/claim/close/approve/deny/handoff/trace/route"},
	{ID: "wide-context", Name: "context panes", Open: "wide terminals", Exit: "responsive", Scope: "instance", Kind: "layout", AIR: "pane values redact", Contract: "event/work/lane/reference context fills negative space"},
	{ID: "split-context", Name: "split context", Open: "[|] wide terminals", Exit: "[|] / responsive", Scope: "instance", Kind: "layout", AIR: "pane values redact", Contract: "all splits: [j/k] source, [J/K] context; Enter/y act on source"},
	{ID: "selection-template", Name: "selection templates", Open: "{{sel.*}} in [:]", Exit: "complete/run", Scope: "engine", Kind: "composition", AIR: "resolver safe", Contract: "inject focus/field/ring refs into commands without copy-paste"},
	{ID: "gate-stack", Name: "gate stack", Open: "[R] / :readiness", Exit: "page cycle", Scope: "instance", Kind: "projection", AIR: "gate values redact", Contract: "source freshness, lane readiness, task holds, authority/preflight/receipt before action"},
	{ID: "trainyard-cockpit", Name: "Trainyard cockpit", Open: "[Y] / :yard", Exit: "page cycle", Scope: "instance", Kind: "projection", AIR: "metadata only", Contract: "SDLC ladder, attention rail, fleet matrix, gates, and lane drilldown"},
	{ID: "dynamics-map", Name: "system dynamics map", Open: "[4] / :dynamics", Exit: "page cycle", Scope: "instance", Kind: "projection", AIR: "nodes redact", Contract: "layered topology, source status, centrality, relations, and resolution scales"},
	{ID: "epistemic-posture", Name: "epistemic posture", Open: "[E] / :epistemics", Exit: "page cycle", Scope: "instance", Kind: "projection", AIR: "metadata only", Contract: "derived evidence/provenance rows; no raw transcript, note body, or dispatch authority"},
	{ID: "observation-feed", Name: "intake observation feed", Open: "[I] / :intake", Exit: "page cycle", Scope: "instance", Kind: "projection", AIR: "metadata only", Contract: "source-backed intake snapshots; counts/coverage visible, raw note/notification bodies absent"},
	{ID: "observation-detail", Name: "intake aggregate detail", Open: "[Enter] on :intake", Exit: "Esc/Enter/q", Scope: "instance", Kind: "door", AIR: "metadata only", Contract: "full-screen provenance and top buckets; drain/dismiss/write/raw-body unavailable"},
	{ID: "labrack-lens", Name: "Labrack / RDLC lens", Open: "[0] :domains / [4] :dynamics", Exit: "page cycle", Scope: "instance", Kind: "lens", AIR: "metadata only", Contract: "research custody/assay domain projection; support/non-authoritative until source pack receipts land"},
	{ID: "section-figure-lens", Name: "Section-Figure lens", Open: "[0] :domains / [4] :dynamics", Exit: "page cycle", Scope: "instance", Kind: "lens", AIR: "metadata only", Contract: "figure-ground exposition plane over lifecycle/domain relations"},
	{ID: "tool-capability-inventory", Name: "tool capability inventory", Open: "[C] / :capabilities", Exit: "page cycle", Scope: "instance", Kind: "projection", AIR: "metadata only", Contract: "in-session coding tool capabilities, route tools, and per-surface admission gaps"},
	{ID: "source-acquisition-router", Name: "source acquisition router", Open: "[C] / :capabilities", Exit: "page cycle", Scope: "instance", Kind: "projection", AIR: "metadata only", Contract: "source tools are support capabilities; egress/quota/source receipts before routing"},
	{ID: "verifier-floor", Name: "verifier floor", Open: "[C] / :capabilities", Exit: "page cycle", Scope: "instance", Kind: "projection", AIR: "metadata only", Contract: "security, coverage, tests, and runtime witnesses are floor signals, not worker lanes"},
	{ID: "provider-gateway", Name: "provider gateway", Open: "[C] / :capabilities", Exit: "page cycle", Scope: "instance", Kind: "projection", AIR: "metadata only", Contract: "provider spend and fallback require explicit route envelope and spend receipt"},
	{ID: "publication-egress", Name: "publication egress", Open: "[C] / :capabilities", Exit: "page cycle", Scope: "instance", Kind: "projection", AIR: "metadata only", Contract: "public/research/social publication requires redaction, authority, and operator receipt"},
	{ID: "audio-avsdlc", Name: "audio AVSDLC", Open: "[C] / :capabilities", Exit: "page cycle", Scope: "instance", Kind: "projection", AIR: "metadata only", Contract: "audio generation/recognition needs consent, media privacy, quota, and egress receipts"},
	{ID: "infra-control", Name: "infrastructure control", Open: "[C] / :capabilities", Exit: "page cycle", Scope: "instance", Kind: "projection", AIR: "metadata only", Contract: "storage/network/admin surfaces require target, destructive preflight, rollback, and receipt"},
	{ID: "signal-pips", Name: "signal pips", Open: "[0] :domains / [?] :legend", Exit: "page cycle", Scope: "instance", Kind: "feedback", AIR: "metadata only", Contract: "compact presence/status marks salvaged from Logos; no authority minted"},
	{ID: "terrain-depth", Name: "terrain-depth lens", Open: "[0] / :domains", Exit: "page cycle", Scope: "instance", Kind: "lens", AIR: "metadata only", Contract: "terrain/depth grouping over domain packs and compiled lifecycle fallback"},
	{ID: "severity-freshness", Name: "severity/freshness lens", Open: "[?] :legend / active pages", Exit: "page cycle", Scope: "instance", Kind: "lens", AIR: "metadata only", Contract: "separate criticality hue from freshness brightness and ownership family"},
	{ID: "air-lens", Name: "AIR lens", Open: "[a] / :air", Exit: "[a] / :air", Scope: "engine", Kind: "lens", AIR: "default deny", Contract: "broadcast-safe renderer, structure preserved"},
	{ID: "read-dark", Name: "read dark/error", Open: "fetch failure", Exit: "next fold", Scope: "instance", Kind: "state", AIR: "reason may hide", Contract: "unknown is visible, never green"},
	{ID: "flash", Name: "effect feedback", Open: "local effect", Exit: "timer", Scope: "engine", Kind: "feedback", AIR: "no raw data", Contract: "short confirmation chip for local-only actions"},
	{ID: "compose", Name: "note compose", Open: ":note / :n", Exit: "status", Scope: "engine", Kind: "command", AIR: "free text hidden", Contract: "local status sink; no authority minted"},
}

func registeredWindows() []WindowDef {
	out := make([]WindowDef, len(windowRegistry))
	copy(out, windowRegistry)
	return out
}

func registeredSplitPairs() []SplitPairDef {
	out := make([]SplitPairDef, len(splitPairRegistry))
	copy(out, splitPairRegistry)
	return out
}

func splitPairForPage(page int) (SplitPairDef, bool) {
	for _, s := range splitPairRegistry {
		if s.Page == page {
			return s, true
		}
	}
	return SplitPairDef{}, false
}

func splitPairSummary() string {
	linked, reference := 0, 0
	for _, s := range splitPairRegistry {
		if s.Reactive() {
			linked++
		} else {
			reference++
		}
	}
	return fmt.Sprintf("split pairs: %d linked · %d source-only", linked, reference)
}

func windowForPage(page int) (WindowDef, bool) {
	for _, w := range windowRegistry {
		if w.Page == page {
			return w, true
		}
	}
	return WindowDef{}, false
}

func windowForKey(key string) (WindowDef, bool) {
	for _, w := range windowRegistry {
		if w.Key == key {
			return w, true
		}
	}
	return WindowDef{}, false
}

func (m Model) cycleWindow(delta int) Model {
	windows := registeredWindows()
	if len(windows) == 0 {
		return m
	}
	idx := 0
	for i, w := range windows {
		if w.Page == m.Page {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(windows)) % len(windows)
	next := windows[idx]
	m = m.switchPage(next.Page)
	m.Status = ":" + next.ID
	return m
}

func windowsSummary() string {
	scopes := map[string]int{}
	lifecycles := map[string]bool{}
	for _, w := range windowRegistry {
		scopes[w.Scope]++
		lifecycles[w.Lifecycle] = true
	}
	ls := make([]string, 0, len(lifecycles))
	for l := range lifecycles {
		ls = append(ls, l)
	}
	sort.Strings(ls)
	return fmt.Sprintf("windows: %d registered · engine %d · instance %d · lifecycles %s",
		len(windowRegistry), scopes["engine"], scopes["instance"], strings.Join(ls, ","))
}

func registeredSurfaces() []SurfaceDef {
	out := make([]SurfaceDef, len(surfaceRegistry))
	copy(out, surfaceRegistry)
	return out
}

func surfacesSummary() string {
	scopes := map[string]int{}
	kinds := map[string]bool{}
	for _, s := range surfaceRegistry {
		scopes[s.Scope]++
		kinds[s.Kind] = true
	}
	ks := make([]string, 0, len(kinds))
	for k := range kinds {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return fmt.Sprintf("surfaces: %d registered · engine %d · instance %d · kinds %s",
		len(surfaceRegistry), scopes["engine"], scopes["instance"], strings.Join(ks, ","))
}

func surfaceKindCounts() (doors, modes, layouts, lenses int) {
	for _, s := range surfaceRegistry {
		switch s.Kind {
		case "door":
			doors++
		case "mode":
			modes++
		case "layout":
			layouts++
		case "lens":
			lenses++
		}
	}
	return doors, modes, layouts, lenses
}

func registeredDomains() []DomainDef {
	out := make([]DomainDef, len(domainRegistry))
	copy(out, domainRegistry)
	return out
}

type LifecycleFallbackDef struct {
	ID, Scope, Windows, Contract string
}

func registeredLifecycleFallbacks() []LifecycleFallbackDef {
	byID := map[string]*LifecycleFallbackDef{}
	for _, wnd := range windowRegistry {
		id := strings.TrimSpace(wnd.Lifecycle)
		if id == "" {
			id = "unknown"
		}
		row := byID[id]
		if row == nil {
			row = &LifecycleFallbackDef{
				ID:       id,
				Scope:    wnd.Scope,
				Contract: "compiled navigation hint; source-backed lifecycle pack required for authority",
			}
			byID[id] = row
		}
		if !strings.Contains(","+row.Windows+",", ","+wnd.ID+",") {
			if row.Windows != "" {
				row.Windows += ","
			}
			row.Windows += wnd.ID
		}
		if row.Scope != wnd.Scope {
			row.Scope = "mixed"
		}
	}
	keys := make([]string, 0, len(byID))
	for id := range byID {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	out := make([]LifecycleFallbackDef, 0, len(keys))
	for _, id := range keys {
		out = append(out, *byID[id])
	}
	return out
}

func domainsSummary() string {
	scopes := map[string]int{}
	terrains := map[string]bool{}
	depths := map[string]bool{}
	for _, d := range domainRegistry {
		scopes[d.Scope]++
		terrains[d.Terrain] = true
		depths[d.Depth] = true
	}
	ts := make([]string, 0, len(terrains))
	for t := range terrains {
		ts = append(ts, t)
	}
	ds := make([]string, 0, len(depths))
	for d := range depths {
		ds = append(ds, d)
	}
	sort.Strings(ts)
	sort.Strings(ds)
	return fmt.Sprintf("domains: %d registered · engine %d · instance %d · terrain %s · depth %s",
		len(domainRegistry), scopes["engine"], scopes["instance"], strings.Join(ts, ","), strings.Join(ds, ","))
}

func lifecyclesSummary(sourceRows int) string {
	source := "source rows absent"
	if sourceRows > 0 {
		source = fmt.Sprintf("%d source rows", sourceRows)
	}
	return fmt.Sprintf("lifecycles: %s · compiled fallback %d · tenant-extensible",
		source, len(registeredLifecycleFallbacks()))
}

func domainDepthCounts() (core, stratum, surface int) {
	for _, d := range domainRegistry {
		switch d.Depth {
		case "core":
			core++
		case "stratum":
			stratum++
		case "surface":
			surface++
		}
	}
	return core, stratum, surface
}
