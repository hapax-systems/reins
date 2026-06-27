package grammar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func ptrS(s string) *string   { return &s }
func ptrF(f float64) *float64 { return &f }

// A fresh dispatch record has null cost/quality/outcome (the measurement-completion follow-on has
// not run yet). The surface must render those as flagged GAPS — never a fabricated $0.00, fake
// score, or "done". This is the never-false-green discipline made visible.
func TestDispatchRowRendersUnmeasuredHonestly(t *testing.T) {
	r := DispatchRecord{
		Capability: "glm-via-cc", RouteID: "claude.full",
		Platform: "claude", Mode: "fast", Profile: "full",
		CCTask: "cc-task-X", SliceKind: "impl", AdmissionAction: "admitted",
		Launched: true, DispatchLatencyMs: 1200, SessionRole: "dev2",
	}
	out := ansi.Strip(RenderDispatchRow(r, false))

	if strings.Contains(out, "$0.00") || strings.Contains(out, "$0.0000") {
		t.Fatalf("a null cost must NOT render as $0.00 (never-false-green):\n%s", out)
	}
	if !strings.Contains(out, "UNMEASURED") {
		t.Fatalf("a null cost must render UNMEASURED:\n%s", out)
	}
	if !strings.Contains(out, "asserted") {
		t.Fatalf("a null quality must render as unverified/asserted:\n%s", out)
	}
	if !strings.Contains(out, "in-flight") {
		t.Fatalf("a null outcome must render in-flight:\n%s", out)
	}
}

// Once the measurement-completion follow-on fills the slots, the real values render (and the gap
// tokens disappear) — proving the pointer-nil distinction actually drives the output.
func TestDispatchRowRendersMeasuredValues(t *testing.T) {
	r := DispatchRecord{
		Capability: "glm-via-cc", RouteID: "claude.full", Platform: "claude",
		Mode: "fast", Profile: "full", CCTask: "cc-task-X", SliceKind: "impl",
		AdmissionAction: "admitted", Launched: true, DispatchLatencyMs: 1200,
		CostUSD: ptrF(0.0123), QualitySignal: ptrS("pass"), Outcome: ptrS("succeeded"),
	}
	out := ansi.Strip(RenderDispatchRow(r, false))

	if !strings.Contains(out, "0.0123") {
		t.Fatalf("a measured cost must render the real value:\n%s", out)
	}
	if strings.Contains(out, "UNMEASURED") || strings.Contains(out, "in-flight") {
		t.Fatalf("measured slots must drop the gap tokens:\n%s", out)
	}
	if !strings.Contains(out, "pass") || !strings.Contains(out, "succeeded") {
		t.Fatalf("measured quality/outcome must render:\n%s", out)
	}
}

// The cc_task id and session role/lane are sensitive on a livestream; the routing + measurement
// fields are structural and survive. On air the sensitive fields redact.
func TestDispatchRowRedactsSensitiveOnAir(t *testing.T) {
	cost := 0.0123
	r := DispatchRecord{
		Capability: "glm-via-cc", RouteID: "claude.full", Platform: "claude",
		Mode: "fast", Profile: "full", CCTask: "cc-task-SECRET", SliceKind: "impl",
		AdmissionAction: "admitted", Launched: true, DispatchLatencyMs: 1200,
		CostUSD: &cost, SessionRole: "dev2",
	}
	out := ansi.Strip(RenderDispatchRow(r, true))

	if strings.Contains(out, "cc-task-SECRET") {
		t.Fatalf("the cc_task id must redact on air:\n%s", out)
	}
	// structural routing + measurement still air (no false confidentiality)
	for _, keep := range []string{"glm-via-cc", "claude.full", "0.0123"} {
		if !strings.Contains(out, keep) {
			t.Fatalf("structural field %q must survive on air:\n%s", keep, out)
		}
	}
}

// The utilization rollup is the "latent resource" signal: routable capabilities with zero
// dispatches must surface as LATENT, dispatched ones as ACTIVE with their tally.
func TestUtilizationSplitsActiveFromLatent(t *testing.T) {
	records := []DispatchRecord{
		{Capability: "glm-via-cc"}, {Capability: "glm-via-cc"}, {Capability: "codex.full"},
	}
	routable := []string{"glm-via-cc", "codex.full", "fugu", "sakana"}
	u := Utilization(records, routable)

	if u.Active["glm-via-cc"] != 2 || u.Active["codex.full"] != 1 {
		t.Fatalf("active tallies wrong: %v", u.Active)
	}
	if len(u.Latent) != 2 {
		t.Fatalf("fugu + sakana are routable-but-never-dispatched (latent): %v", u.Latent)
	}
	out := ansi.Strip(RenderUtilization(u))
	if !strings.Contains(out, "2 active · 2 latent (of 4 routable)") {
		t.Fatalf("utilization header wrong:\n%s", out)
	}
	for _, lat := range []string{"fugu", "sakana"} {
		if !strings.Contains(out, lat) {
			t.Fatalf("latent capability %q must be named:\n%s", lat, out)
		}
	}
}

// An empty ledger must say so plainly — never fake activity — and still name the blind-spots.
func TestEmptyLedgerIsHonest(t *testing.T) {
	out := ansi.Strip(RenderDispatchLedger(nil, false))
	if !strings.Contains(out, "ledger empty") {
		t.Fatalf("empty ledger must say so:\n%s", out)
	}
	if !strings.Contains(out, "blind-spots") {
		t.Fatalf("even an empty ledger names what it cannot measure:\n%s", out)
	}
}

// A held dispatch (admission denied → not launched) must read as held, not launched.
func TestDispatchRowShowsHeld(t *testing.T) {
	r := DispatchRecord{Capability: "fugu", AdmissionAction: "fail_closed", Launched: false}
	out := ansi.Strip(RenderDispatchRow(r, false))
	if !strings.Contains(out, "held") {
		t.Fatalf("a non-launched dispatch must read held:\n%s", out)
	}
	if strings.Contains(out, "launched") {
		t.Fatalf("a held dispatch must not read launched:\n%s", out)
	}
}
