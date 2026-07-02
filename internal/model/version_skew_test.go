package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestVersionSkewLogic(t *testing.T) {
	m := New("REINS")
	if m.versionSkew() {
		t.Fatal("no versions known -> no skew")
	}
	m.CockpitVersion, m.APIVersion = "0.1.0", "0.1.0"
	if m.versionSkew() {
		t.Fatal("matching versions -> no skew")
	}
	m.APIVersion = "0.2.0"
	if !m.versionSkew() {
		t.Fatal("mismatched semvers -> skew")
	}
	m.CockpitVersion = "dev" // a bare source run must never trip skew
	if m.versionSkew() {
		t.Fatal("'dev' must not trip skew")
	}
}

func TestGenSkewRendersOnTitleBar(t *testing.T) {
	m := New("REINS")
	m.Width, m.Height = 160, 44
	m.Page = PageCoordinator
	m.CockpitVersion, m.APIVersion = "0.1.0", "0.2.0"
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "GEN-SKEW") {
		t.Fatalf("a binary↔API version mismatch must render GEN-SKEW on the title bar, got:\n%s", firstLine(out))
	}
	// a matched pair must NOT render the skew banner
	m.APIVersion = "0.1.0"
	if strings.Contains(ansi.Strip(m.View()), "GEN-SKEW") {
		t.Fatal("matched versions must not render GEN-SKEW")
	}
}
