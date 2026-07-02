package model

import "testing"

// U6b-deploy cockpit half: :swap must request the exec-handover AND release the terminal (Quitting), so
// main.go can syscall.Exec into the current generation after the TUI exits. The actual exec is a thin
// wrapper over the tested internal/swap.ResolveExecPlan; here we pin the interactive trigger.
func TestSwapCommandRequestsHandoverAndQuits(t *testing.T) {
	m := New("REINS").Exec("swap")
	if !m.SwapRequested {
		t.Fatal(":swap must set SwapRequested (main.go exec-handovers on it)")
	}
	if !m.Quitting {
		t.Fatal(":swap must set Quitting (release the terminal before the re-exec)")
	}
	// the verb is registered (so command completion + the catalog list it).
	if _, ok := lookupVerb("swap"); !ok {
		t.Fatal("swap verb not registered")
	}
}

// A cold model does not accidentally request a swap.
func TestNoSwapByDefault(t *testing.T) {
	if New("REINS").SwapRequested {
		t.Fatal("a fresh model must not request a swap")
	}
}
