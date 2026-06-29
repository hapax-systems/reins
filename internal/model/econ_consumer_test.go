package model

import (
	"testing"

	"github.com/hapax-systems/reins/internal/grammar"
)

// Consumer-readiness for the STEP-7 v̂ producer (dev2 #4338, lands first): a record carrying served
// value_hat/value_status/fit flows into the econ cell as MEASURED; an unserved record (value_status "")
// reads ABSENT so the frontier stays honestly UNDEFINED — and lights up the moment the producer serves.
func TestEconCellsConsumeServedValueHatElseAbsent(t *testing.T) {
	v, fit := 42.0, 0.8
	m := New("R")
	m.DispatchRecords = []grammar.DispatchRecord{
		{Capability: "glm", CCTask: "t1", ValueHat: &v, ValueStatus: "measured", Fit: &fit, Launched: true},
		{Capability: "codex", CCTask: "t2", Launched: true}, // producer hasn't served → absent
	}
	cells := m.econCells()
	if len(cells) != 2 {
		t.Fatalf("expected 2 cells, got %d", len(cells))
	}
	if cells[0].ValueStatus != "measured" || cells[0].ValueHat == nil || *cells[0].ValueHat != 42 || cells[0].Fit == nil {
		t.Fatalf("served v̂/fit must flow through to the cell: %+v", cells[0])
	}
	if cells[1].ValueStatus != "absent" || cells[1].ValueHat != nil {
		t.Fatalf("an unserved record must read ABSENT (frontier undefined): %+v", cells[1])
	}
}
