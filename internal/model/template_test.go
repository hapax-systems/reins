package model

import (
	"strings"
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

func selModel() Model {
	m := New("REINS").FoldTasks([]grammar.Task{
		{TaskID: "task-9", Stage: "S5", Owner: "alpha", Criticality: "crit",
			AIR: map[string]string{"task_id": "ok", "stage": "ok", "owner": "deny", "criticality": "ok"}},
	}, false)
	m.Width, m.Height = 120, 40
	m.Page = PageTasks
	m.Focus = 0
	return m
}

func TestTemplateResolvesSelection(t *testing.T) {
	m := selModel()
	m.Sel.Rank, m.Sel.Field = RankField, "stage"
	cases := map[string]string{
		"{{sel}}":        "S5",     // selected field's value
		"{{sel.id}}":     "task-9", // focused row id
		"{{focus}}":      "task-9",
		"{{sel.field}}":  "stage", // the field NAME
		"{{sel.crit}}":   "crit",
		"{{sel.owner}}":  "alpha", // arbitrary field (LOCAL — not redacted)
		"a {{sel.id}} b": "a task-9 b",
		"{{nope}}":       "{{nope}}", // unknown token stays literal
	}
	for in, want := range cases {
		if got := m.resolveTemplate(in); got != want {
			t.Errorf("resolveTemplate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTemplateRedactsOnAir(t *testing.T) {
	m := selModel()
	m.AIR = true // ON AIR — a denied field must NOT leak its value
	if got := m.resolveTemplate("{{sel.owner}}"); got != "▒▒▒" {
		t.Fatalf("on-air, a denied field must redact, got %q", got)
	}
	if got := m.resolveTemplate("{{sel.stage}}"); got != "S5" { // allowlisted: still resolves
		t.Fatalf("on-air, an allowlisted field should resolve, got %q", got)
	}
}

func TestRingTemplateRedactsDeniedEntryWhenAirEnabledLater(t *testing.T) {
	m := selModel()
	m = step(m, "y")
	m = step(m, "o") // local owner yank is allowed, but provenance is deny
	if len(m.Ring) != 1 || m.Ring[0].Value != "alpha" || m.Ring[0].AIR != "deny" {
		t.Fatalf("local yank should preserve denied AIR provenance: %+v", m.Ring)
	}
	m.AIR = true
	out := m.Exec("note {{ring.0}}")
	if out.Status == "note ▸ alpha" {
		t.Fatalf("ring template replay leaked a denied value after AIR enabled: %q", out.Status)
	}
	if out.Status != "note ▸ ▒▒▒ (free text hidden on AIR)" {
		t.Fatalf("ring template should preserve structure with redaction, got %q", out.Status)
	}
}

func TestNoteVerbExpandsTemplate(t *testing.T) {
	m := selModel()
	out := m.Exec("note owner is {{sel.owner}}")
	if out.Status != "note ▸ owner is alpha" {
		t.Fatalf("note should expand the ref, got %q", out.Status)
	}
}

func TestNoteVerbHidesFreeTextOnAir(t *testing.T) {
	m := selModel()
	m.AIR = true
	out := m.Exec("note AIR_FREE_TEXT_SENTINEL")
	if strings.Contains(out.Status, "AIR_FREE_TEXT_SENTINEL") || out.Status != "note ▸ ▒▒▒ (free text hidden on AIR)" {
		t.Fatalf("note must not echo arbitrary free text on AIR, got %q", out.Status)
	}
}

func TestTemplateResolutionIsPageAware(t *testing.T) {
	m := New("REINS").
		FoldTasks([]grammar.Task{{TaskID: "hidden-task", AIR: map[string]string{"task_id": "ok"}}}, false).
		Fold(evFixture(), false)
	m.Width, m.Height = 120, 40
	m.Page = PageEvents
	if out := m.Exec("note {{focus}}"); out.Status != "note ▸ third" {
		t.Fatalf("event page focus should resolve to the visible event, got %q", out.Status)
	}
	m.Page = PageHelp
	if out := m.Exec("note {{focus}}"); strings.Contains(out.Status, "hidden-task") {
		t.Fatalf("reference pages must not resolve hidden task focus, got %q", out.Status)
	}
}

func TestEventTemplateHonorsDeniedMetadataAir(t *testing.T) {
	m := New("REINS").Fold([]grammar.Event{{
		TS: "SECRET-TS", Kind: "secret.kind", Subject: "visible-subject",
		AIR: map[string]string{"ts": "deny", "kind": "deny", "subject": "ok"},
	}}, false)
	m.Width, m.Height = 120, 40
	m.Page = PageEvents
	m.AIR = true
	for _, expr := range []string{"{{sel.ts}}", "{{sel.kind}}"} {
		if got := m.resolveTemplate(expr); got != "▒▒▒" {
			t.Fatalf("%s should redact denied event metadata, got %q", expr, got)
		}
	}
}

func TestSessionTemplateHonorsAir(t *testing.T) {
	m := New("REINS").FoldSessions([]grammar.Session{{
		Role: "cx-p0", Session: "SECRET-TMUX", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88, ClaimedTask: "SECRET-TASK",
		AIR: map[string]string{
			"role": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok",
			"session": "deny", "claimed_task": "deny",
		},
	}}, false)
	m.Width, m.Height = 120, 40
	m.Page = PageSessions
	m.AIR = true
	if got := m.resolveTemplate("{{focus}}"); got != "cx-p0" {
		t.Fatalf("session focus should resolve to the AIR-safe role, got %q", got)
	}
	if got := m.resolveTemplate("{{sel.readiness}}"); got != "claim" {
		t.Fatalf("session readiness should be template-addressable, got %q", got)
	}
	for _, expr := range []string{"{{sel.session}}", "{{sel.claimed_task}}"} {
		if got := m.resolveTemplate(expr); got != "▒▒▒" {
			t.Fatalf("%s should redact denied session metadata, got %q", expr, got)
		}
	}
}

func TestSplitContextTemplatesBindToSessionSource(t *testing.T) {
	m := New("REINS").
		FoldTasks([]grammar.Task{{TaskID: "hidden-task", AIR: map[string]string{"task_id": "ok"}}}, false).
		Fold([]grammar.Event{{
			TS: "10:00", Kind: "coord_dispatch.launch_started", Subject: "hidden-event", Actor: "cx-source",
			AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok"},
		}}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-source", Session: "tmux-source", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88, ClaimedTask: "claimed-task",
			AIR: map[string]string{
				"role": "ok", "session": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok", "claimed_task": "ok",
			},
		}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 170, 40, PageEvents, true
	if !m.splitContextActive() {
		t.Fatal("test requires active split context")
	}

	if got := m.resolveTemplate("{{focus}}"); got != "cx-source" {
		t.Fatalf("split {{focus}} should bind to visible session source, got %q", got)
	}
	if got := m.resolveTemplate("{{sel.platform}}"); got != "codex" {
		t.Fatalf("split {{sel.platform}} should resolve from visible session source, got %q", got)
	}
	if out := m.Exec("intent show-route"); out.IntentSubject != "session cx-source" {
		t.Fatalf("split intent subject should bind to visible session source, got %q", out.IntentSubject)
	}
}

func TestSplitContextSelectedFieldTemplatesAndPasteUseSessionSource(t *testing.T) {
	m := New("REINS").
		Fold([]grammar.Event{{
			TS: "10:00", Kind: "coord_dispatch.launch_started", Subject: "hidden-event", Actor: "cx-source",
			AIR: map[string]string{"ts": "ok", "kind": "ok", "subject": "ok", "actor": "ok"},
		}}, false).
		FoldSessions([]grammar.Session{{
			Role: "cx-source", Session: "tmux-source", Platform: "codex", State: "active", Readiness: "claim", Blocker: "none", Attention: 0.88,
			AIR: map[string]string{"role": "ok", "session": "ok", "platform": "ok", "state": "ok", "readiness": "ok", "blocker": "ok", "attention": "ok"},
		}}, false)
	m.Width, m.Height, m.Page, m.SplitContext = 170, 40, PageEvents, true
	m.Sel.Rank, m.Sel.Field = RankField, "platform"

	if got := m.resolveTemplate("{{sel.field}}={{sel.value}}"); got != "platform=codex" {
		t.Fatalf("split selected field template should use session field, got %q", got)
	}
	m.Mode, m.Input = ModeCommand, "paste"
	cands := m.completionTree()
	if len(cands) == 0 || cands[0].Label != pastePrefix+"platform" || cands[0].Value != "codex" {
		t.Fatalf("split paste candidate should lead with selected session field, got %+v", cands)
	}
}

func TestCapabilityTemplateResolutionUsesCapabilityCursor(t *testing.T) {
	m := New("REINS").FoldCapabilities(grammar.CapabilitySummary{
		Rows: []grammar.CapabilityRow{
			{CapabilityID: "source_acquisition", Status: "admission-incomplete", Authority: "sub-router", RouteCount: 5, OKCount: 3, BlockedCount: 2, EvidenceCount: 5, Blocker: "Tavily usage telemetry schema",
				AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "blocker": "ok", "hkp_posture": "ok"}},
			{CapabilityID: "tavily_source_acquisition", Status: "admission-incomplete", Authority: "source-acquisition", CapabilityClass: "source_acquisition", SurfaceFamily: "tavily", SpendModel: "api_spend_budgeted", EgressClass: "source_query", ReceiptRequirement: "usage + budget + route receipt", RouteCount: 1, OKCount: 0, BlockedCount: 1, EvidenceCount: 4, Blocker: "route on refreshed usage receipt",
				AIR: map[string]string{"capability_id": "ok", "status": "ok", "authority": "ok", "capability_class": "ok", "surface_family": "ok", "spend_model": "ok", "egress_class": "ok", "receipt_requirement": "ok", "route_count": "ok", "ok_count": "ok", "blocked_count": "ok", "evidence_count": "ok", "blocker": "ok", "hkp_posture": "ok"}},
		},
	}, false)
	m.Width, m.Height, m.Page = 120, 40, PageCaps
	m = m.capabilityFocusTo(1)

	for expr, want := range map[string]string{
		"{{focus}}":         "tavily_source_acquisition",
		"{{sel.id}}":        "tavily_source_acquisition",
		"{{sel.field}}":     "capability",
		"{{sel.status}}":    "admission-incomplete",
		"{{sel.authority}}": "source-acquisition",
		"{{sel.family}}":    "tavily",
		"{{sel.receipt}}":   "usage + budget + route receipt",
		"{{sel.missing}}":   "route on refreshed usage receipt",
	} {
		if got := m.resolveTemplate(expr); got != want {
			t.Fatalf("%s resolved to %q, want %q", expr, got, want)
		}
	}
}

func TestRegistryTemplateResolutionUsesSurfaceAndDomainCursors(t *testing.T) {
	m := New("REINS")
	m.Width, m.Height, m.Page = 120, 40, PageSurfaces
	m = m.surfaceFocusTo(1)
	for expr, want := range map[string]string{
		"{{focus}}":        "filter",
		"{{sel.id}}":       "filter",
		"{{sel.field}}":    "surface",
		"{{sel.kind}}":     "mode",
		"{{sel.glyph}}":    ":",
		"{{sel.contract}}": "narrows task rows; completion offers visible ids",
	} {
		if got := m.resolveTemplate(expr); got != want {
			t.Fatalf("surface %s resolved to %q, want %q", expr, got, want)
		}
	}

	m.Page = PageDomains
	m = m.domainFocusTo(len(registeredDomains()) - 1)
	for expr, want := range map[string]string{
		"{{focus}}":        "future-n-dlc",
		"{{sel.id}}":       "future-n-dlc",
		"{{sel.field}}":    "domain",
		"{{sel.terrain}}":  "bedrock",
		"{{sel.windows}}":  "domains,windows",
		"{{sel.surfaces}}": "surfaces",
	} {
		if got := m.resolveTemplate(expr); got != want {
			t.Fatalf("domain %s resolved to %q, want %q", expr, got, want)
		}
	}

	m = New("REINS").FoldDomains(grammar.DomainSummary{
		Rows: []grammar.DomainRow{
			{
				DomainID: "sdlc-trainyard", Lifecycle: "SDLC", Terrain: "delivery", Depth: "surface", Scope: "operator", State: "observed",
				AuthorityCeiling: "projection", Windows: "yard,readiness", Surfaces: "yard,caps", Parity: "trainyard", EvidenceCount: 8, Blocker: "none",
				AIR: map[string]string{"domain_id": "ok", "lifecycle": "ok", "terrain": "ok", "depth": "ok", "scope": "ok", "state": "ok", "authority_ceiling": "ok", "windows": "ok", "surfaces": "ok", "parity": "ok", "evidence_count": "ok", "blocker": "ok"},
			},
			{
				DomainID: "rdlc-labrack", Lifecycle: "RDLC", Terrain: "research", Depth: "stratum", Scope: "tenant", State: "candidate",
				AuthorityCeiling: "support", Windows: "domains,dynamics", Surfaces: "labrack,figure", Parity: "labrack", EvidenceCount: 3, Blocker: "source review",
				AIR: map[string]string{"domain_id": "ok", "lifecycle": "ok", "terrain": "ok", "depth": "ok", "scope": "ok", "state": "ok", "authority_ceiling": "ok", "windows": "ok", "surfaces": "ok", "parity": "ok", "evidence_count": "ok", "blocker": "ok"},
			},
		},
	}, false)
	m.Width, m.Height, m.Page = 120, 40, PageDomains
	m = m.domainFocusTo(1)
	for expr, want := range map[string]string{
		"{{focus}}":         "rdlc-labrack",
		"{{sel.id}}":        "rdlc-labrack",
		"{{sel.field}}":     "domain",
		"{{sel.lifecycle}}": "RDLC",
		"{{sel.state}}":     "candidate",
		"{{sel.authority}}": "support",
		"{{sel.missing}}":   "source review",
	} {
		if got := m.resolveTemplate(expr); got != want {
			t.Fatalf("source-backed domain %s resolved to %q, want %q", expr, got, want)
		}
	}
}

func TestTemplateCandidatesWhenOpen(t *testing.T) {
	m := selModel()
	m.Mode = ModeCommand
	m.Input = "note {{sel."
	cands := m.completionTree()
	if len(cands) == 0 {
		t.Fatal("an open {{sel. should offer template refs")
	}
	for _, c := range cands {
		if c.Label[:2] != "{{" {
			t.Fatalf("expected template candidates, got %q", c.Label)
		}
	}
	// accepting fills the open fragment, stays in command mode (composing)
	m.CompIdx = 0
	m = m.acceptCompletion()
	if m.Mode != ModeCommand {
		t.Fatal("accepting a template ref should keep composing")
	}
	if got := m.Input; got[:5] != "note " || got[len(got)-2:] != "}}" {
		t.Fatalf("fill should replace the open fragment with a full ref, got %q", got)
	}
}
