package model

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

func tasksN(ids ...string) []grammar.Task {
	out := make([]grammar.Task, 0, len(ids))
	for _, id := range ids {
		out = append(out, grammar.Task{TaskID: id})
	}
	return out
}

// A1.1 — restore is identity-anchored: after a snapshot at focus task "t3", a restore against a
// REORDERED task set re-homes focus to "t3" by identity, not to the old index.
func TestRestoreReanchorsByIdentityNotIndex(t *testing.T) {
	orig := Model{Tasks: tasksN("t0", "t1", "t2", "t3", "t4"), Focus: 3}
	orig.Sel.Members = []int{1, 4} // t1, t4
	snap := orig.SnapshotPosture()
	if snap.Anchors.FocusedTaskID != "t3" {
		t.Fatalf("snapshot did not capture focused id: %q", snap.Anchors.FocusedTaskID)
	}
	if strings.Join(snap.Anchors.SelMemberIDs, ",") != "t1,t4" {
		t.Fatalf("snapshot member ids wrong: %v", snap.Anchors.SelMemberIDs)
	}

	// fresh process: restore, then the first live fold arrives with t3 at a DIFFERENT index.
	fresh := Model{}.RestorePosture(snap)
	if fresh.pending == nil {
		t.Fatal("restore must leave a pending-reanchor set until the first live fold")
	}
	fresh = fresh.FoldTasks(tasksN("x", "t4", "t3", "t1"), false) // t3 now at index 2
	if got, _ := fresh.FocusedTask(); got.TaskID != "t3" {
		t.Fatalf("focus did not reanchor to t3 by identity: got %q (Focus=%d)", got.TaskID, fresh.Focus)
	}
	if !reflect.DeepEqual(fresh.Sel.Members, []int{1, 3}) { // t4@1, t1@3
		t.Fatalf("members did not reanchor by identity: %v", fresh.Sel.Members)
	}
	if fresh.pending != nil {
		t.Fatal("pending set should be spent after a successful live reanchor")
	}
}

// A1.1 — a DARK first fold must NOT consume the pending set (the bug class: index-restore
// destroyed on the first dark poll). The anchor survives until a live fold arrives.
func TestDarkFirstFoldDoesNotDestroyPending(t *testing.T) {
	snap := Model{Tasks: tasksN("a", "b", "c"), Focus: 2}.SnapshotPosture()
	m := Model{}.RestorePosture(snap)
	m = m.FoldTasks(nil, true) // DARK first fold
	if m.pending == nil {
		t.Fatal("dark fold consumed the pending set — the fold-clamp bug class")
	}
	m = m.FoldTasks(tasksN("z", "c", "a", "b"), false) // live fold: c at index 1
	if got, _ := m.FocusedTask(); got.TaskID != "c" {
		t.Fatalf("focus did not reanchor after a dark-then-live sequence: %q", got.TaskID)
	}
}

// A1.1 — an anchor whose identity is ABSENT after a live fold is dropped honestly (focus falls to
// a valid row; the pending set is spent), never silently pointed at a different row.
func TestAbsentAnchorIsDroppedNotMisassigned(t *testing.T) {
	snap := Model{Tasks: tasksN("gone", "keep"), Focus: 0}.SnapshotPosture()
	m := Model{}.RestorePosture(snap)
	m = m.FoldTasks(tasksN("keep", "other"), false) // "gone" is absent
	if got, _ := m.FocusedTask(); got.TaskID == "gone" {
		t.Fatal("resolved a task that no longer exists")
	}
	if m.pending != nil {
		t.Fatal("pending set must be spent after a live fold even when the anchor is absent")
	}
}

// Zero-loss round trip through disk: snapshot -> write -> read -> restore preserves page, chat, and
// (after a live fold) focus identity.
func TestPostureRoundTripIsNoLoss(t *testing.T) {
	path := filepath.Join(t.TempDir(), "posture.json")
	orig := Model{Tasks: tasksN("t0", "t1"), Focus: 1, Page: PageTasks, TurnRole: "lane-x"}
	orig = orig.AppendOperatorText("remember this")
	if err := WritePosture(path, orig.SnapshotPosture()); err != nil {
		t.Fatal(err)
	}
	snap, ok, err := ReadPosture(path)
	if err != nil || !ok {
		t.Fatalf("read posture: ok=%v err=%v", ok, err)
	}
	m := Model{}.RestorePosture(snap)
	if m.Page != PageTasks {
		t.Fatalf("page not restored: %d", m.Page)
	}
	if m.TurnRole != "lane-x" {
		t.Fatalf("turn role not restored: %q", m.TurnRole)
	}
	if len(m.CoordChatLog) != 1 || len(m.CoordChatLog[0].Parts) != 1 ||
		m.CoordChatLog[0].Parts[0].Text != "remember this" {
		t.Fatalf("coordchat log not restored no-loss: %+v", m.CoordChatLog)
	}
	m = m.FoldTasks(tasksN("t1", "t0"), false)
	if got, _ := m.FocusedTask(); got.TaskID != "t1" {
		t.Fatalf("focus identity lost across disk round trip: %q", got.TaskID)
	}
}

// Forbidden-fields schema guard: the PostureSnapshot must carry ONLY posture + the operator's own
// chat — NEVER task/gate/route/capability FACTS (those are the read model's to fetch fresh).
// Structural pin: no field type may be a read-model fact type.
func TestPostureSnapshotCarriesNoReadModelFacts(t *testing.T) {
	forbidden := map[string]bool{
		"Task": true, "Session": true, "Event": true, "Trace": true, "Node": true,
		"Edge": true, "GateSummary": true, "CapabilitySummary": true, "IntakeSummary": true,
		"DomainSummary": true, "ObserveDimension": true, "VaultNote": true, "Graph": true,
	}
	var walk func(rt reflect.Type, path string)
	walk = func(rt reflect.Type, path string) {
		switch rt.Kind() {
		case reflect.Slice, reflect.Array, reflect.Ptr, reflect.Map:
			walk(rt.Elem(), path)
		case reflect.Struct:
			if forbidden[rt.Name()] {
				t.Fatalf("PostureSnapshot reaches a read-model fact type %q at %s", rt.Name(), path)
			}
			for i := 0; i < rt.NumField(); i++ {
				f := rt.Field(i)
				walk(f.Type, path+"."+f.Name)
			}
		}
	}
	walk(reflect.TypeOf(PostureSnapshot{}), "PostureSnapshot")
}

// Followup fixes (U2 review): the Deck's no-loss history survives the swap, and a focused session
// re-anchors by role on the first live :sessions fold.
func TestDeckAndSessionFocusSurviveRestart(t *testing.T) {
	orig := Model{
		Sessions: []grammar.Session{{Role: "alpha"}, {Role: "beta"}, {Role: "gamma"}},
		SFocus:   2, // gamma
		Deck:     []string{"line one", "line two"},
	}
	snap := orig.SnapshotPosture()
	if snap.Anchors.FocusedSessionRole != "gamma" {
		t.Fatalf("session role not captured: %q", snap.Anchors.FocusedSessionRole)
	}
	if len(snap.Deck) != 2 {
		t.Fatalf("deck not captured no-loss: %v", snap.Deck)
	}

	m := Model{}.RestorePosture(snap)
	if len(m.Deck) != 2 || m.Deck[0] != "line one" {
		t.Fatalf("deck not restored no-loss: %v", m.Deck)
	}
	// sessions arrive REORDERED on the first live fold — gamma must re-home by role, not index.
	m = m.FoldSessions([]grammar.Session{{Role: "gamma"}, {Role: "alpha"}}, false)
	if s, _ := m.FocusedSession(); s.Role != "gamma" {
		t.Fatalf("session focus did not reanchor by role: %q (SFocus=%d)", s.Role, m.SFocus)
	}
}

// A1.4 — forward-tolerant: a snapshot with a non-matching schema major is IGNORED, never
// mis-applied (an old binary restoring a newer snapshot must not corrupt posture).
func TestForwardToleranceIgnoresUnmatchedMajor(t *testing.T) {
	snap := Model{Tasks: tasksN("a"), Page: PageTasks}.SnapshotPosture()
	snap.SchemaMajor = postureSchemaMajor + 1
	m := Model{Page: PageEvents}.RestorePosture(snap)
	if m.Page != PageEvents {
		t.Fatal("an unmatched schema major must be ignored, not applied")
	}
	if m.pending != nil {
		t.Fatal("no pending set should be created for an unmatched major")
	}
}
