package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/grammar"
)

// E11.5b :vault renders LIVE Obsidian metadata off-air; AIR-seals on air (operator-private life-planning,
// count airs / titles do not); honest-dark when the endpoint is unreachable.
func TestRenderVaultLiveAndSealsOnAir(t *testing.T) {
	m := New("R").FoldVault([]grammar.VaultNote{
		{Title: "deep-goals", Folder: "10-projects", ObsidianURI: "obsidian://x"},
		{Title: "sprint-measures", Folder: "20-areas"},
	}, false)

	off := ansi.Strip(m.renderVault(90))
	if !strings.Contains(off, "deep-goals") || !strings.Contains(off, "10-projects") {
		t.Fatalf("off-air vault must list note titles + folders:\n%s", off)
	}

	m.AIR = true
	on := ansi.Strip(m.renderVault(90))
	if strings.Contains(on, "deep-goals") || !strings.Contains(on, "SEALED on air") {
		t.Fatalf("on air the vault must SEAL (operator-private):\n%s", on)
	}

	dark := ansi.Strip(New("R").FoldVault(nil, true).renderVault(90))
	if !strings.Contains(dark, "dark") {
		t.Fatalf("an unreachable vault must render honest-dark:\n%s", dark)
	}
}
