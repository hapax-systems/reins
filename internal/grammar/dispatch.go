package grammar

import (
	"fmt"
	"sort"
	"strings"
)

// DispatchRecord mirrors shared/dispatch_record.py — the dispatch ledger at
// ~/.cache/hapax/sdlc-routing/dispatch-events.jsonl (schema-aligned with gate-events). It is the
// READ-projection Reins surfaces (the cc-task-capdispatch-surface-20260627 lane emits the records;
// Reins renders them, built ahead of the feed like the session-pane Turn grammar).
//
// cost/quality/outcome are POINTERS so "not yet measured" (null) is distinguishable from a real
// value. That distinction is the whole point: a null cost renders UNMEASURED, never "$0.00" — the
// never-false-green discipline ("observe the improvement" must be MEASURED, not asserted).
type DispatchRecord struct {
	TS                string   `json:"ts"`
	Capability        string   `json:"capability"`
	RouteID           string   `json:"route_id"`
	Platform          string   `json:"platform"`
	Mode              string   `json:"mode"`
	Profile           string   `json:"profile"`
	CCTask            string   `json:"cc_task"`
	SliceKind         string   `json:"slice_kind"`
	AdmissionAction   string   `json:"admission_action"`
	Launched          bool     `json:"launched"`
	DispatchLatencyMs int      `json:"dispatch_latency_ms"`
	Outcome           *string  `json:"outcome"`        // null = in-flight (not done)
	CostUSD           *float64 `json:"cost_usd"`       // null = UNMEASURED (never $0.00)
	QualitySignal     *string  `json:"quality_signal"` // null = asserted/unverified (never a fake score)
	SessionRole       string   `json:"session_role"`
}

// dispatchDash renders an empty routing field as an em-dash so a blank platform/mode reads honestly.
func dispatchDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// dispatchSecret redacts a sensitive dispatch field (the cc_task id, the session role/lane) on air;
// the routing/measurement fields are structural and survive.
func dispatchSecret(v string, airOn bool) string {
	if airOn {
		return "▒▒▒"
	}
	if strings.TrimSpace(v) == "" {
		return "—"
	}
	return v
}

// RenderDispatchRow renders one dispatch-ledger row in the cell grammar. The measurement slots are
// HONEST: a null cost/quality/outcome shows UNMEASURED / asserted? / in-flight (a flagged gap),
// NEVER a fabricated $0.00, fake score, or "done" — measurement-first, not asserted.
func RenderDispatchRow(r DispatchRecord, airOn bool) string {
	launched := C("grn", "▸launched")
	if !r.Launched {
		launched = C("red", "✖held")
	}
	cost := C("yel", "$ UNMEASURED")
	if r.CostUSD != nil {
		cost = C("pri", fmt.Sprintf("$%.4f", *r.CostUSD))
	}
	quality := C("yel", "q asserted?")
	if r.QualitySignal != nil && strings.TrimSpace(*r.QualitySignal) != "" {
		quality = C("grn", "q:"+*r.QualitySignal)
	}
	outcome := C("mut", "▸in-flight")
	if r.Outcome != nil && strings.TrimSpace(*r.Outcome) != "" {
		outcome = C("2nd", *r.Outcome)
	}
	return strings.Join([]string{
		C("brt", "⇉ "+r.Capability),
		C("2nd", "route:"+r.RouteID),
		dispatchDash(r.Platform) + "/" + dispatchDash(r.Mode) + "/" + dispatchDash(r.Profile),
		C("mut", "task:") + dispatchSecret(r.CCTask, airOn),
		C("mut", r.SliceKind),
		C("2nd", r.AdmissionAction),
		launched,
		C("mut", fmt.Sprintf("%dms", r.DispatchLatencyMs)),
		cost, quality, outcome,
	}, "  ")
}

// RenderDispatchRowCompact renders the ACTIVITY half of a dispatch (for the ledger list in a split
// pane): routing + admission + launch + latency. The MEASUREMENT half (cost/quality/outcome) lives in
// the measurement pane, where its honest gaps have room to be named rather than clipped off the row.
func RenderDispatchRowCompact(r DispatchRecord, airOn bool) string {
	launched := C("grn", "▸")
	if !r.Launched {
		launched = C("red", "✖")
	}
	return strings.Join([]string{
		C("brt", "⇉ "+r.Capability),
		C("2nd", r.RouteID),
		C("mut", r.SliceKind),
		C("2nd", r.AdmissionAction),
		launched + C("mut", fmt.Sprintf("%dms", r.DispatchLatencyMs)),
		C("mut", "task:") + dispatchSecret(r.CCTask, airOn),
	}, "  ")
}

// MeasurementSummary aggregates the measurement-completion state across the ledger: how many
// dispatches have a real cost / quality / terminal outcome vs how many are still unmeasured.
type MeasurementSummary struct {
	Total, Priced, Verified, Settled int
}

// SummarizeMeasurement counts the measured-vs-not split (nil = not yet measured by the follow-on).
func SummarizeMeasurement(records []DispatchRecord) MeasurementSummary {
	s := MeasurementSummary{Total: len(records)}
	for _, r := range records {
		if r.CostUSD != nil {
			s.Priced++
		}
		if r.QualitySignal != nil && strings.TrimSpace(*r.QualitySignal) != "" {
			s.Verified++
		}
		if r.Outcome != nil && strings.TrimSpace(*r.Outcome) != "" {
			s.Settled++
		}
	}
	return s
}

// RenderMeasurementSummary is the measurement-first readout: the cost/quality/outcome gaps are COUNTED
// and named (priced N/total, the rest UNMEASURED; verified N/total, the rest asserted), never hidden.
func RenderMeasurementSummary(s MeasurementSummary) string {
	cost := C("yel", fmt.Sprintf("cost     %d/%d priced · rest UNMEASURED", s.Priced, s.Total))
	if s.Total > 0 && s.Priced == s.Total {
		cost = C("grn", fmt.Sprintf("cost     %d/%d priced", s.Priced, s.Total))
	}
	qual := C("yel", fmt.Sprintf("quality  %d/%d verified · rest asserted", s.Verified, s.Total))
	if s.Total > 0 && s.Verified == s.Total {
		qual = C("grn", fmt.Sprintf("quality  %d/%d verified", s.Verified, s.Total))
	}
	out := C("mut", fmt.Sprintf("outcome  %d/%d settled · %d in-flight", s.Settled, s.Total, s.Total-s.Settled))
	return strings.Join([]string{cost, qual, out}, "\n")
}

// RenderDispatchHeader labels the dispatch-ledger columns + names the two open measurement gaps so a
// reader knows the surface is honest about what it cannot yet measure.
func RenderDispatchHeader() string {
	return C("mut", "⇉ capability · route · platform/mode/profile · task · slice · admission · launched · latency · "+
		C("yel", "cost(0.0 today)")+" · "+C("yel", "quality(asserted)")+" · outcome")
}

// RenderDispatchBlindSpots names — on the surface itself — the dimensions the dispatch ledger cannot
// yet measure, so the surface never reads as complete. Measurement-first means the gaps are SHOWN,
// not hidden: a reader sees exactly which signals are real and which are still asserted/absent.
func RenderDispatchBlindSpots() string {
	return C("yel", "⚠ blind-spots: ") + C("mut",
		"cost=0.0 (unwired from LiteLLM _response_cost) · quality=asserted (EDT loop open) · "+
			"A/B shadow-pairs absent · workstream-adherence untracked")
}

// RenderDispatchLedger renders the full ledger: header, one honest row per record (newest first is
// the caller's job), and the blind-spots footer. An empty ledger says so plainly rather than faking
// activity.
func RenderDispatchLedger(records []DispatchRecord, airOn bool) string {
	if len(records) == 0 {
		return RenderDispatchHeader() + "\n" + C("mut", "  (no dispatches recorded — ledger empty)") +
			"\n" + RenderDispatchBlindSpots()
	}
	rows := make([]string, 0, len(records)+2)
	rows = append(rows, RenderDispatchHeader())
	for _, r := range records {
		rows = append(rows, "  "+RenderDispatchRow(r, airOn))
	}
	rows = append(rows, RenderDispatchBlindSpots())
	return strings.Join(rows, "\n")
}

// DispatchUtilization is the "latent resource" rollup: of the routable capability set, which are
// ACTIVE (≥1 dispatch in the ledger) vs LATENT (routable but never dispatched — capacity we are
// paying to route but not using).
type DispatchUtilization struct {
	Active   map[string]int // capability → dispatch count (count ≥ 1)
	Latent   []string       // routable capabilities with zero dispatches
	Routable int
}

// Utilization derives the active-vs-latent split from the ledger against the canonical routable set.
func Utilization(records []DispatchRecord, routable []string) DispatchUtilization {
	counts := map[string]int{}
	for _, r := range records {
		if c := strings.TrimSpace(r.Capability); c != "" {
			counts[c]++
		}
	}
	var latent []string
	for _, c := range routable {
		if counts[c] == 0 {
			latent = append(latent, c)
		}
	}
	return DispatchUtilization{Active: counts, Latent: latent, Routable: len(routable)}
}

// RenderUtilization renders the latent-resource signal: how much of the routable capability surface
// is actually exercised, and which capabilities sit idle.
func RenderUtilization(u DispatchUtilization) string {
	head := C("brt", fmt.Sprintf("UTILIZATION  %d active · %d latent (of %d routable)",
		len(u.Active), len(u.Latent), u.Routable))
	var active []string
	for cap, n := range u.Active {
		active = append(active, C("grn", cap)+C("mut", fmt.Sprintf(" ▣%d", n)))
	}
	sort.Strings(active)
	lines := []string{head, C("2nd", "  active:  ") + strings.Join(active, "  ")}
	if len(u.Latent) > 0 {
		latent := make([]string, len(u.Latent))
		copy(latent, u.Latent)
		sort.Strings(latent)
		lines = append(lines, C("yel", "  latent:  ")+C("mut", strings.Join(latent, " · ")))
	}
	return strings.Join(lines, "\n")
}
