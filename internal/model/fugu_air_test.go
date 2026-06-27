package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// AIR derived-channel leaks found by an independent codex-fugu review (panes the comprehensive audit
// missed). Each test pins the off-air-includes / on-air-excludes contract.

// On air, [V] class-select must not disclose a denied criticality class (the status named it, and the
// migration unblocked this native control). It is withheld on air for a denied-criticality task.
func TestClassSelectWithheldOnAirForDeniedCrit(t *testing.T) {
	task := grammar.Task{TaskID: "x", Criticality: "crit", AIR: map[string]string{"task_id": "ok", "criticality": "deny"}}
	m := New("R").FoldTasks([]grammar.Task{task}, false)
	m.Page = PageTasks

	m.AIR = true
	on := step(m, "V")
	if len(on.Sel.Members) != 0 {
		t.Fatalf("on air [V] must not select the denied class: %v", on.Sel.Members)
	}
	if strings.Contains(on.Status, "'crit'") || !strings.Contains(on.Status, "withheld") {
		t.Fatalf("on air [V] must withhold without naming the class, got %q", on.Status)
	}
	m.AIR = false
	if off := step(m, "V"); len(off.Sel.Members) == 0 {
		t.Fatalf("off air [V] selects the criticality class")
	}
}

// sessionConstraintPane is the migrated PageSessions secondary; it renders the lane readiness, route
// binding, freshness, AND the inline session work surface (SessionDetail). Every value + derived hue
// must honor AIR (regression for the codex-fugu tasks/sessions review: attention/route-binding/ages
// rendered raw, and the work-surface AIR coverage was lost when the legacy wide-body test was deleted).
func TestSessionConstraintPaneIsAirSafe(t *testing.T) {
	s := grammar.Session{
		Role: "cx-session", Session: "tmux-secret", Platform: "codex", State: "active",
		Readiness: "claim", RouteID: "r1", RouteBindingState: "policy_only", Attention: 0.88,
		OutputAgeS: 5, RelayAgeS: 9999,
		AIR: map[string]string{
			"role": "ok", "session": "deny", "platform": "ok", "state": "ok", "readiness": "ok",
			"attention": "deny", "route_id": "ok", "route_binding_state": "deny",
			"output_age_s": "deny", "relay_age_s": "deny", "blocker": "ok", "claimed_task": "ok",
		},
	}
	m := New("R").FoldSessions([]grammar.Session{s}, false).
		FoldSessionDetail(grammar.SessionDetail{
			Role:         "cx-session",
			Task:         grammar.SessionTaskDetail{TaskID: "task-1", Status: "claimed", AuthorityCase: "SECRET-CASE", ParentSpec: "SECRET-PARENT", MutationSurface: "source"},
			EvidenceRefs: []grammar.EvidenceRef{{Kind: "cc_task_note", Path: "/secret/task.md", Size: 12}},
			AIR:          map[string]string{"task_id": "ok", "status": "ok", "mutation_surface": "ok", "authority_case": "deny", "parent_spec": "deny", "path": "deny"},
		}, false)
	m.AIR = true
	out := ansi.Strip(m.sessionConstraintPane(120))
	for _, leak := range []string{"SECRET-CASE", "SECRET-PARENT", "tmux-secret", "/secret/task.md", "0.88", "9999"} {
		if strings.Contains(out, leak) {
			t.Fatalf("sessionConstraintPane leaked %q on air:\n%s", leak, out)
		}
	}
	if !strings.Contains(out, "LANE READINESS") || !strings.Contains(out, "▒▒▒") {
		t.Fatalf("the pane should keep structure with redaction:\n%s", out)
	}
}

// The emergent connector relation must not derive over a DENIED facet: even though airRelationLabel
// withholds the value, the facet-CHOICE + count ("shares crit (N)") itself discloses that a denied
// field is shared. AIR-aware entity builders omit denied facets before relate.Derive, so the
// connector can never pick one.
func TestEmergentRelationOmitsDeniedFacetOnAir(t *testing.T) {
	a := grammar.Task{TaskID: "a", Criticality: "crit", Stage: "S7", AIR: map[string]string{"task_id": "ok", "criticality": "deny", "stage": "ok"}}
	b := grammar.Task{TaskID: "b", Criticality: "crit", Stage: "S6", AIR: map[string]string{"task_id": "ok", "criticality": "deny", "stage": "ok"}}
	m := New("R").FoldTasks([]grammar.Task{a, b}, false)
	m.Focus = 0

	m.AIR = false
	if rel := ansi.Strip(m.tasksEmergentRelation()); !strings.Contains(rel, "crit") {
		t.Fatalf("off air the shared (denied-on-air) criticality IS the strongest relation: %q", rel)
	}
	m.AIR = true
	if rel := ansi.Strip(m.tasksEmergentRelation()); strings.Contains(rel, "crit") {
		t.Fatalf("on air a denied criticality must not appear in the emergent connector relation: %q", rel)
	}
}

// eventSlackRows classifies events by kind into fail/succeed/other — a denied kind must not classify,
// nor inflate the "other" denominator.
func TestEventSlackRowsIsAirSafe(t *testing.T) {
	denied := grammar.Event{Kind: "deploy.fail", AIR: map[string]string{"kind": "deny"}}
	allowed := grammar.Event{Kind: "deploy.succeed", AIR: map[string]string{"kind": "ok"}}
	m := New("R").Fold([]grammar.Event{denied, allowed}, false)

	m.AIR = true
	out := strings.Join(m.eventSlackRows(120), " ")
	if !strings.Contains(out, "fail:0") {
		t.Fatalf("on air a denied-kind fail must not count (want fail:0): %s", out)
	}
	if strings.Contains(out, "other:1") {
		t.Fatalf("on air a denied event must not leak into the 'other' bucket: %s", out)
	}
	m.AIR = false
	if !strings.Contains(strings.Join(m.eventSlackRows(120), " "), "fail:1") {
		t.Fatalf("off air the fail event counts")
	}
}

// taskSlackRows tallies criticality buckets — a denied criticality must not be counted.
func TestTaskSlackRowsIsAirSafe(t *testing.T) {
	denied := grammar.Task{Criticality: "crit", AIR: map[string]string{"criticality": "deny"}}
	allowed := grammar.Task{Criticality: "crit", AIR: map[string]string{"criticality": "ok"}}
	m := New("R").FoldTasks([]grammar.Task{denied, allowed}, false)

	m.AIR = true
	out := strings.Join(m.taskSlackRows(120), " ")
	if !strings.Contains(out, "crit:1") || strings.Contains(out, "crit:2") {
		t.Fatalf("on air a denied criticality must not be tallied (want crit:1): %s", out)
	}
	m.AIR = false
	if !strings.Contains(strings.Join(m.taskSlackRows(120), " "), "crit:2") {
		t.Fatalf("off air both count (crit:2)")
	}
}

// eventContextPane: the same-subject/same-actor neighborhood COUNTS, the derived state (from
// kind/score), and the raw score all leaked the denied field. On air the counts must be 0 (no
// cardinality leak), the state must not assert breakdown, and the score must redact.
func TestEventContextPaneNeighborhoodStateScoreAreAirSafe(t *testing.T) {
	focus := grammar.Event{Kind: "deploy.fail", Subject: "svc", Actor: "a", Score: 0.9,
		AIR: map[string]string{"subject": "deny", "actor": "deny", "kind": "deny", "score": "deny"}}
	sib := grammar.Event{Kind: "deploy.start", Subject: "svc", Actor: "a", Score: 0.1,
		AIR: map[string]string{"subject": "ok", "actor": "ok", "kind": "ok", "score": "ok"}}
	m := New("R").Fold([]grammar.Event{focus, sib}, false)

	m.AIR = false
	off := ansi.Strip(m.eventContextPane(120))
	if !strings.Contains(off, "2 events") { // off air the focus+sibling both share subject → count 2
		t.Fatalf("off air the same-subject neighborhood counts (want 2 events):\n%s", off)
	}

	m.AIR = true
	on := ansi.Strip(m.eventContextPane(120))
	if strings.Contains(on, "2 events") {
		t.Fatalf("on air a denied-subject/actor anchor must not leak the neighborhood cardinality:\n%s", on)
	}
	if strings.Contains(on, "breakdown") {
		t.Fatalf("on air a denied kind/score must not leak the derived breakdown state:\n%s", on)
	}
	if strings.Contains(on, "0.90") {
		t.Fatalf("on air a denied score must redact:\n%s", on)
	}
}
