package model

import (
	"strings"
	"testing"
)

// TestPaneContractRegistryIsComplete (invariant I7 — the pane-contract forcing function): every navigable
// pane declares its five-tuple contract ⟨question·state·controls·provenance·blind-spot⟩, OR is tracked as
// undeclared debt. The gate FAILS THE BUILD if (a) a registered contract is incomplete, (b) a page is in
// BOTH the registry + the debt (contracted twice), or (c) a page is in NEITHER (a new page that was added
// without being contracted or declared debt). The debt may only shrink; the registry may only grow — toward
// the hard gate (every pane contracted). Enforce-not-exhort.
func TestPaneContractRegistryIsComplete(t *testing.T) {
	// 1. every REGISTERED contract is COMPLETE (all five fields non-empty).
	for page, c := range PageContracts {
		if strings.TrimSpace(c.Question) == "" {
			t.Errorf("page %d: PaneContract.Question is empty", page)
		}
		if strings.TrimSpace(c.State) == "" {
			t.Errorf("page %d: PaneContract.State is empty", page)
		}
		if strings.TrimSpace(c.Controls) == "" {
			t.Errorf("page %d: PaneContract.Controls is empty", page)
		}
		if strings.TrimSpace(c.Provenance) == "" {
			t.Errorf("page %d: PaneContract.Provenance is empty", page)
		}
		if strings.TrimSpace(c.BlindSpot) == "" {
			t.Errorf("page %d: PaneContract.BlindSpot is empty", page)
		}
	}

	// 2. NO page in BOTH (overlap is an error — contract a page exactly once).
	for page := range PageContracts {
		if undeclaredPanes[page] {
			t.Errorf(
				"page %d is in BOTH PageContracts and undeclaredPanes — contract it once (remove it from "+
					"undeclaredPanes when you add it to PageContracts)",
				page,
			)
		}
	}

	// 3. EVERY navigable page (0..PageDeck) is accounted for (registered OR declared debt) — regression
	//    prevention: a new page added to the iota is auto-caught here (it must be contracted or declared
	//    debt; it cannot silently accrete without a contract).
	for page := 0; page <= PageDeck; page++ {
		_, registered := PageContracts[page]
		if !registered && !undeclaredPanes[page] {
			t.Errorf(
				"page %d is in NEITHER PageContracts nor undeclaredPanes — a navigable page must declare its "+
					"five-tuple contract (add it to PageContracts) or be declared debt (undeclaredPanes)",
				page,
			)
		}
	}

	t.Logf(
		"pane-contract registry: %d contracted / %d undeclared debt / %d total — the registry may only grow, "+
			"the debt only shrink",
		len(PageContracts), len(undeclaredPanes), PageDeck+1,
	)
}
