package grammar

import "testing"

// PaletteMode witnesses SetPalette: the observability seam the live config reload relies on
// (off-TTY renders strip color, so only the mode itself can testify the switch happened).
func TestPaletteModeWitnessesSetPalette(t *testing.T) {
	defer SetPalette(PaletteMode()) // restore
	SetPalette("solarized")
	if got := PaletteMode(); got != "solarized" {
		t.Fatalf("after SetPalette(solarized) PaletteMode must report solarized, got %q", got)
	}
	SetPalette("gruvbox")
	if got := PaletteMode(); got != "gruvbox" {
		t.Fatalf("after SetPalette(gruvbox) PaletteMode must report gruvbox, got %q", got)
	}
}
