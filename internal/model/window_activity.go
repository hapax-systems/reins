package model

// windowSig is the signature of a window's current visible state (the count/hot/dark the
// hotlist shows) — the value compared across visits to detect "what changed."
func (m Model) windowSig(page int) string {
	sig, _ := m.windowSignal(page)
	return sig
}

// windowActive reports whether a window's state changed since the operator last visited
// it — the activity ladder (audit §2 #1: the hotlist must flag "what changed," not just
// "what is"). A window is snapshotted when left (switchPage); the current page is never
// active; a never-visited window has no baseline (not active).
func (m Model) windowActive(page int) bool {
	if page == m.Page {
		return false
	}
	seen, ok := m.WindowSeen[page]
	if !ok {
		return false
	}
	return m.windowSig(page) != seen
}
