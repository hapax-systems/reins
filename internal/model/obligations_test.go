package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

func TestRenderObligationsLiveSealsOnAirAndHonestDark(t *testing.T) {
	rep := grammar.ObligationsReport{
		Dark: false,
		Summary: grammar.ObligationsSummary{
			TotalOwes:                 2,
			LanebusOwesCount:          1,
			CustodyOwesCount:          1,
			LocusAnomalyCount:         1,
			LocalLociOccupiedInMemory: 1,
			LocalLociTotal:            1,
			RoutesOccupiedInMemory:    1,
			RoutesTotal:               2,
		},
		LocalSubstrateLoci: []grammar.SubstrateLocusRow{
			{
				ID:                  "host:node-a:8000/model-x",
				Host:                "node-a",
				Port:                "8000",
				State:               "lit",
				LoadedModels:        []string{"model-x"},
				ResidentModelsCount: 1,
				Topology:            "tp=2,nnodes=2",
			},
		},
		LocusAnomalies: []grammar.LocusAnomalyRow{
			{Kind: "host.dark", ID: "host:node-dark", State: "dark", Subject: "node-dark", Attention: "warning"},
		},
		LanebusOwes: []grammar.ObligationRow{
			{Kind: "lanebus.unacked", Relation: "owes", Creditor: "lanebus", Debtor: "lane-alpha", Subject: "3 unacked message(s)", Attention: "warning"},
		},
	}

	m := New("R").FoldObligations(rep, false)
	off := ansi.Strip(m.renderObligations(120))
	if !strings.Contains(off, "OBLIGATIONS") || !strings.Contains(off, "node-a:8000") || !strings.Contains(off, "lane-alpha") {
		t.Fatalf("off-air obligations must show locus and debtor details:\n%s", off)
	}

	m.AIR = true
	on := ansi.Strip(m.renderObligations(120))
	if strings.Contains(on, "node-a:8000") || strings.Contains(on, "lane-alpha") || !strings.Contains(on, "sealed on air") {
		t.Fatalf("on-air obligations must seal host/debtor identities:\n%s", on)
	}
	if !strings.Contains(on, "owes=2") {
		t.Fatalf("on-air obligations must still air structural counts:\n%s", on)
	}

	dark := ansi.Strip(New("R").FoldObligations(grammar.ObligationsReport{Dark: true}, true).renderObligations(120))
	if !strings.Contains(dark, "obligations dark") {
		t.Fatalf("dark obligations report must render honest-dark:\n%s", dark)
	}

	nav := New("R").Exec("obligations")
	if nav.Page != PageObligations {
		t.Fatalf(":obligations did not switch to PageObligations (got %d)", nav.Page)
	}
}
