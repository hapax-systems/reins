package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// E8.3: operator readouts accumulate in the non-evicting DECK; AIR-safe — the deck is operator-private
// history (rendered off-air, possibly cleartext) so it SEALS on air (count airs, content does not).
func TestDeckAccumulatesReadoutsAndSealsOnAir(t *testing.T) {
	m := New("R")
	m = m.appendStatusNotice("first readout")
	m = m.appendStatusNotice("second readout")
	if len(m.Deck) != 2 {
		t.Fatalf("deck must accumulate readouts non-evicting: %v", m.Deck)
	}
	off := ansi.Strip(m.renderDeck(80))
	if !strings.Contains(off, "first readout") || !strings.Contains(off, "second readout") {
		t.Fatalf("off-air deck must show its readouts:\n%s", off)
	}
	m.AIR = true
	on := ansi.Strip(m.renderDeck(80))
	if strings.Contains(on, "first readout") || !strings.Contains(on, "SEALED on air") {
		t.Fatalf("on air the deck must SEAL (operator-private, not for the wire):\n%s", on)
	}
}
