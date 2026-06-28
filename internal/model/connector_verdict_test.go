package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/layout"
)

// E0.2 — the connector header states the page's typed-join VERDICT honestly (pageVerdict, the 19-page
// decision). A STANDING page (real join — the secondary is keyed to the operator's selection) asserts
// its join key (⋈); a DOOR (no real join) asserts NONE — the never-mint honesty floor.
func TestConnectorStatesPageJoinVerdict(t *testing.T) {
	// PageAxes is a STANDING page with a constant emergent relation (no external data needed), so the
	// connector header always renders — the cleanest place to assert the live verdict clause.
	m := New("REINS")
	m.Page = PageAxes
	if pageVerdict(PageAxes) != VerdictStanding {
		t.Fatal("precondition: axes is a STANDING page")
	}
	spec := m.composePage(200, 16)
	if spec == nil {
		t.Fatal("axes must compose via the view-algebra")
	}
	header := strings.SplitN(ansi.Strip(layout.Render(spec, 200, 16)), "\n", 2)[0]
	if !strings.Contains(header, "⋈") || !strings.Contains(header, "axis -> five-tuple contract") {
		t.Fatalf("a STANDING page must assert its join key alongside the relation in the connector header:\n%s", header)
	}

	// A DOOR page renders via referenceBody (not composePage), so assert the helper directly: the
	// verdict is Door and the asserted key is dropped (the honesty floor — never mint a join).
	d := New("REINS")
	d.Page = PageHelp
	c := d.verdictConnector("ambient context")
	if c.Verdict != "Door" || c.JoinKey != "" {
		t.Fatalf("a DOOR must carry Verdict=Door and NO join key, got verdict=%q key=%q", c.Verdict, c.JoinKey)
	}
}
