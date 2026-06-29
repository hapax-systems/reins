package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// E11.7 :observe renders LIVE per-dimension whole-system signals; AIR seals the VALUES (operator-private
// system state) while the dimension key + live/dark status air (structural); honest-dark when the
// endpoint is down. Per-dimension honest-dark (a dark source shows ·, no fabricated count).
func TestRenderObserveLiveAndSealsValuesOnAir(t *testing.T) {
	n := 97
	m := New("R").FoldObserve([]grammar.ObserveDimension{
		{Key: "health", Status: "live", Summary: "101/124 failed"},
		{Key: "drift", Status: "live", Summary: "items", Count: &n},
		{Key: "cost", Status: "dark"},
	}, false)

	off := ansi.Strip(m.renderObserve(100))
	if !strings.Contains(off, "health") || !strings.Contains(off, "101/124") || !strings.Contains(off, "97") {
		t.Fatalf("off-air observe must show dimension values:\n%s", off)
	}

	m.AIR = true
	on := ansi.Strip(m.renderObserve(100))
	if strings.Contains(on, "101/124") || strings.Contains(on, "97") || !strings.Contains(on, "sealed on air") {
		t.Fatalf("on air observe must SEAL the values:\n%s", on)
	}
	if !strings.Contains(on, "health") {
		t.Fatalf("on air the dimension key/status (structural) must still air:\n%s", on)
	}

	dark := ansi.Strip(New("R").FoldObserve(nil, true).renderObserve(100))
	if !strings.Contains(dark, "dark") {
		t.Fatalf("an unreachable observe must render honest-dark:\n%s", dark)
	}
}
