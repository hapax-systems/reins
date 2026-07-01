package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// PostureSnapshot v2 — the externalized session posture that lets the cockpit die and restore
// (hot-plug, <req-id>). The load-bearing invariant (A1.1, CONFIRMED-bug fix): the
// snapshot carries STABLE IDENTITIES, never raw row indices. Folds clamp cursors and reset
// selection on every poll (FoldTasks), so an index restored cold is destroyed within one cycle —
// identities are re-resolved on the first non-dark fold instead (pending-reanchor).
//
// Forbidden-fields invariant: this struct carries ONLY posture (which page, which identities are
// focused/selected, window-visit state) and the operator's own local chat log. It NEVER carries
// task / gate / route / capability FACTS — those are the read model's to fetch fresh. The
// forbidden-fields schema test pins this structurally.
type PostureSnapshot struct {
	SchemaMajor int `json:"schema_major"` // reader accepts any matching major; unknown fields ignored (A1.4)
	SchemaMinor int `json:"schema_minor"`

	PageWindowID string `json:"page_window_id"` // stable window ID, never page index (A1.12)

	Anchors PostureAnchors `json:"anchors"` // identity-anchored restore (A1.1)

	WindowSeen map[string]string `json:"window_seen"` // windowID -> last-visit signature (A1.12)

	CoordChatLog []CoordChatMessage `json:"coordchat_log"` // the operator's local chat (datom-shaped; ChatPart carries AIRProv)

	Deck []string `json:"deck"` // E8.3 non-evicting operator-readout history — its "no-loss" property must survive the swap

	IntentTarget  string `json:"intent_target"`  // a swap intent staged during the swap survives it (A1.8)
	IntentSubject string `json:"intent_subject"`
}

// PostureAnchors holds the stable identities the restore re-resolves against the live fold.
type PostureAnchors struct {
	FocusedTaskID     string   `json:"focused_task_id"`
	FocusedSessionRole string  `json:"focused_session_role"`
	TurnRole          string   `json:"turn_role"`
	SelMemberIDs      []string `json:"sel_member_ids"` // task ids, NOT indices
}

const postureSchemaMajor = 2

// pendingReanchor holds anchors awaiting the first non-dark fold of their page. Nil once applied
// or when there is nothing to restore.
type pendingReanchor struct {
	anchors PostureAnchors
}

// SnapshotPosture extracts the current session posture as identities.
func (m Model) SnapshotPosture() PostureSnapshot {
	snap := PostureSnapshot{
		SchemaMajor:   postureSchemaMajor,
		SchemaMinor:   0,
		PageWindowID:  pageToWindowID(m.Page),
		WindowSeen:    windowSeenByID(m.WindowSeen),
		CoordChatLog:  m.CoordChatLog,
		Deck:          m.Deck,
		IntentTarget:  m.IntentTarget,
		IntentSubject: m.IntentSubject,
	}
	if t, ok := m.FocusedTask(); ok {
		snap.Anchors.FocusedTaskID = t.TaskID
	}
	if s, ok := m.FocusedSession(); ok {
		snap.Anchors.FocusedSessionRole = s.Role
	}
	snap.Anchors.TurnRole = m.TurnRole
	// selection members (row indices into visibleTasks) -> task ids
	vt := m.visibleTasks()
	for _, mi := range m.Sel.Members {
		if mi >= 0 && mi < len(vt) {
			snap.Anchors.SelMemberIDs = append(snap.Anchors.SelMemberIDs, vt[mi].TaskID)
		}
	}
	return snap
}

// RestorePosture folds a snapshot into the model as PAGE + PENDING anchors + local chat/intent.
// The page and window-visit state apply immediately (no live data needed); the focus/selection
// identities go into a pending-reanchor set applied on the first non-dark fold (A1.1). A dark
// first fold must NOT destroy the pending set — ApplyPendingAnchors is a no-op until live rows
// arrive.
func (m Model) RestorePosture(snap PostureSnapshot) Model {
	if snap.SchemaMajor != postureSchemaMajor {
		return m // forward-tolerant: an unmatched major is ignored, not mis-applied (A1.4)
	}
	if p, ok := windowIDToPage(snap.PageWindowID); ok {
		m.Page = p
	}
	if snap.WindowSeen != nil {
		m.WindowSeen = windowSeenByPage(snap.WindowSeen)
	}
	m.CoordChatLog = snap.CoordChatLog
	m.Deck = snap.Deck
	m.TurnRole = snap.Anchors.TurnRole
	m.IntentTarget = snap.IntentTarget
	m.IntentSubject = snap.IntentSubject
	m.pending = &pendingReanchor{anchors: snap.Anchors}
	return m
}

// ApplyPendingAnchors re-resolves pending identity anchors against the CURRENT (post-fold) row
// sets. Called after every fold; a no-op when there is no pending set or the page's rows are
// still dark/empty (so a dark first fold cannot consume the pending set with a wrong index).
// An anchor whose identity is absent after a live fold is dropped (the render surfaces the
// absence honestly — never a silently different row).
func (m Model) ApplyPendingAnchors() Model {
	if m.pending == nil {
		return m
	}
	a := m.pending.anchors

	if a.FocusedTaskID != "" && !m.TasksDark && len(m.visibleTasks()) > 0 {
		vt := m.visibleTasks()
		for i, t := range vt {
			if t.TaskID == a.FocusedTaskID {
				m.Focus = i
				break
			}
		}
		// members re-resolve in the same live fold
		if len(a.SelMemberIDs) > 0 {
			want := map[string]bool{}
			for _, id := range a.SelMemberIDs {
				want[id] = true
			}
			var members []int
			for i, t := range vt {
				if want[t.TaskID] {
					members = append(members, i)
				}
			}
			m.Sel.Members = members
		}
		a.FocusedTaskID = "" // consumed
		a.SelMemberIDs = nil
	}

	// the session-focus anchor re-resolves against a live :sessions fold (separate page, separate
	// fold — so it clears independently of the task anchor).
	if a.FocusedSessionRole != "" && !m.SessionsDark && len(m.Sessions) > 0 {
		for i, s := range m.Sessions {
			if s.Role == a.FocusedSessionRole {
				m.SFocus = i
				break
			}
		}
		a.FocusedSessionRole = "" // consumed (resolved or honestly absent — SFocus stays valid either way)
	}

	// the pending set is spent once every currently-restorable anchor has resolved-or-dropped.
	if a.FocusedTaskID == "" && a.FocusedSessionRole == "" {
		m.pending = nil
	} else {
		m.pending = &pendingReanchor{anchors: a}
	}
	return m
}

// --- persistence -----------------------------------------------------------

func posturePath() string {
	if v := strings.TrimSpace(os.Getenv("REINS_POSTURE_PATH")); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "hapax", "reins", "posture.json")
}

// WritePosture persists the snapshot atomically (temp + rename). Callers debounce; this does no
// throttling of its own.
func WritePosture(path string, snap PostureSnapshot) error {
	if path == "" {
		path = posturePath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadPosture loads a snapshot; a missing file is (zero, false, nil) — cold boot is legal.
func ReadPosture(path string) (PostureSnapshot, bool, error) {
	if path == "" {
		path = posturePath()
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return PostureSnapshot{}, false, nil
	}
	if err != nil {
		return PostureSnapshot{}, false, err
	}
	var snap PostureSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return PostureSnapshot{}, false, err
	}
	return snap, true, nil
}

// --- window-ID <-> page helpers (never serialize the page index — A1.12) ---

func pageToWindowID(page int) string {
	for _, w := range registeredWindows() {
		if w.Page == page {
			return w.ID
		}
	}
	return ""
}

func windowIDToPage(id string) (int, bool) {
	for _, w := range registeredWindows() {
		if w.ID == id {
			return w.Page, true
		}
	}
	return 0, false
}

func windowSeenByID(byPage map[int]string) map[string]string {
	out := map[string]string{}
	for page, sig := range byPage {
		if id := pageToWindowID(page); id != "" {
			out[id] = sig
		}
	}
	return out
}

func windowSeenByPage(byID map[string]string) map[int]string {
	out := map[int]string{}
	for id, sig := range byID {
		if p, ok := windowIDToPage(id); ok {
			out[p] = sig
		}
	}
	return out
}
