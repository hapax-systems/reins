package grammar

import "strings"

// CommandEnvelope is the governed shape of a cockpit COMMAND verb: WHAT it would emit, against what
// TARGET, under what AUTHORITY, with what PREFLIGHT + RECEIPT contract + UI delta. The cockpit never
// mints authority — when !Wired the envelope is explicitly a never-mint PREVIEW, the honesty floor.
type CommandEnvelope struct {
	Verb      string
	Target    string // the object id (task/session) — sensitive, redacts on air
	Payload   string // the governed event it WOULD emit, e.g. sdlc.authorization_flip(release_authorized=true)
	Authority string // the authority/route it requires
	Preflight string // the checks that must pass first
	Receipt   string // the receipt contract (what it would write to the spine)
	UIDelta   string // the resulting UI change
	Wired     bool   // false → preview only (NOT wired)
	// TargetAirOK is the caller's assertion that Target is ALREADY air-resolved (it respected the
	// item's per-field AIR map / allowlist). Default false → the renderer default-denies the target
	// on air (defense in depth — a caller that forgets to resolve still cannot leak).
	TargetAirOK bool
}

// RenderCommandEnvelope renders the governed preview one line. AIR: the TARGET (which object) is
// sensitive and redacts; the verb/payload/authority/contract are the governed SHAPE (structural) and
// survive — the operator (and the stream) can see WHAT a verb would do without WHICH object on air.
func RenderCommandEnvelope(e CommandEnvelope, airOn bool) string {
	target := e.Target
	if airOn && !e.TargetAirOK {
		target = Redact(nil, "label", e.Target, true)
	}
	parts := []string{C("brt", e.Verb)}
	if strings.TrimSpace(e.Target) != "" {
		parts = append(parts, C("pri", target))
	}
	parts = append(parts, "would emit "+C("2nd", e.Payload), "auth "+e.Authority)
	if strings.TrimSpace(e.Preflight) != "" {
		parts = append(parts, "preflight "+e.Preflight)
	}
	if strings.TrimSpace(e.Receipt) != "" {
		parts = append(parts, "receipt "+e.Receipt)
	}
	if strings.TrimSpace(e.UIDelta) != "" {
		parts = append(parts, "Δ "+e.UIDelta)
	}
	tail := C("red", "NOT wired (the cockpit never mints authority)")
	if e.Wired {
		tail = C("grn", "wired")
	}
	return strings.Join(parts, " · ") + " · " + tail
}
