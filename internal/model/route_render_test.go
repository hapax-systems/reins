package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

func sampleRoutePosture() grammar.RoutePosture {
	return grammar.RoutePosture{
		Decision: "NO SPINE DECISION ON FILE",
		Keyspace: grammar.RouteKeyspace{
			Pinned: make([]string, 11), PinnedCount: 11, ObservedCount: 5, UnknownObserved: nil,
		},
		Reqvec:  grammar.RouteReqvec{Dims: make([]string, 8), Min: 0, Max: 5, RangeSource: "producer-contract"},
		Sources: []grammar.RouteSource{{Name: "gate_events", State: "live", Events: 153}, {Name: "edt", State: "dark"}},
	}
}

// :route renders the spine's routing evidence honestly — NO SPINE DECISION, keyspace coverage, the
// reqvec contract, source states; and per class the measured demand OR the word ABSENT.
func TestRenderRouteHonest(t *testing.T) {
	m := Model{Width: 120}.FoldRoute(sampleRoutePosture(), []grammar.RouteCandidate{
		{RoutingClass: "source_python", InKeyspace: true, MeasuredEvents: 4, ReqvecMeasured: true,
			DispatchReqvec: map[string]int{"quality_floor": 5, "information_scope": 1, "context_length": 1,
				"mutation_risk": 3, "verification_demand": 3, "ambiguity_novelty": 3,
				"composition_coupling": 4, "governance_sensitivity": 1}},
		{RoutingClass: "verification", InKeyspace: true, MeasuredEvents: 1, ReqvecMeasured: false},
	}, false)
	out := ansi.Strip(m.renderRoute(120))
	if !strings.Contains(out, "NO SPINE DECISION ON FILE") {
		t.Fatalf("route must render the honest no-decision posture:\n%s", out)
	}
	if !strings.Contains(out, "5/11 observed") || !strings.Contains(out, "0..5") || !strings.Contains(out, "producer-contract") {
		t.Fatalf("keyspace coverage / reqvec contract not rendered:\n%s", out)
	}
	if !strings.Contains(out, "source_python") || !strings.Contains(out, "q5") {
		t.Fatalf("measured demand row not rendered:\n%s", out)
	}
	// the verification class has no complete vector -> ABSENT, never a q0/fabricated-zero vector.
	verLine := ""
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "verification") {
			verLine = ln
		}
	}
	if !strings.Contains(verLine, "ABSENT") || strings.Contains(verLine, "q0") {
		t.Fatalf("absent-vector class must render ABSENT, not fabricated zeros: %q", verLine)
	}
}

func TestRenderRouteHonestDark(t *testing.T) {
	dark := Model{Width: 120}.FoldRoute(grammar.RoutePosture{Dark: true, Error: "feed unreachable"}, nil, true)
	out := ansi.Strip(dark.renderRoute(120))
	if !strings.Contains(out, "route dark") || strings.Contains(out, "NO SPINE DECISION ON FILE") {
		t.Fatalf("dark route must disclose dark, not render posture:\n%s", out)
	}
}

// no-display-scalar: the route pane must not mint an aggregate ranking/goodness scalar.
func TestRenderRouteNoScalar(t *testing.T) {
	m := Model{Width: 120}.FoldRoute(sampleRoutePosture(), []grammar.RouteCandidate{
		{RoutingClass: "coordination", InKeyspace: true, MeasuredEvents: 2, ReqvecMeasured: false},
	}, false)
	low := strings.ToLower(ansi.Strip(m.renderRoute(120)))
	for _, scalar := range []string{"aggregate", "score", "posterior", "p_correct", "rank "} {
		if strings.Contains(low, scalar) {
			t.Fatalf("route pane leaks a display scalar (%q):\n%s", scalar, low)
		}
	}
}

// width-determinism: every rendered line fits exactly within the target width at 80 and 120.
func TestRenderRouteWidthDeterministic(t *testing.T) {
	m := Model{Width: 120}.FoldRoute(sampleRoutePosture(), []grammar.RouteCandidate{
		{RoutingClass: "a-very-long-routing-class-name-that-overflows", InKeyspace: false, MeasuredEvents: 999,
			ReqvecMeasured: true, DispatchReqvec: map[string]int{"quality_floor": 5, "information_scope": 5,
				"context_length": 5, "mutation_risk": 5, "verification_demand": 5, "ambiguity_novelty": 5,
				"composition_coupling": 5, "governance_sensitivity": 5}},
	}, false)
	for _, w := range []int{80, 120} {
		for _, ln := range strings.Split(m.renderRoute(w), "\n") {
			if vw := ansi.StringWidth(ln); vw > w {
				t.Fatalf("route line exceeds width %d (got %d): %q", w, vw, ansi.Strip(ln))
			}
		}
	}
}
