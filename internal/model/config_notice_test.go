package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The ConfigNotice status-bar branch: off-air the notice renders verbatim; on air the detail is
// withheld (config parse errors can embed paths/content) and only the label airs.
func TestConfigNoticeRenderAirRedaction(t *testing.T) {
	m := New("REINS")
	m.ConfigNotice = "config kept last-good — fix the TOML or remove the file; secret /home/ops/token.toml detail"

	off := ansi.Strip(m.viewVital(160))
	if !strings.Contains(off, "fix the TOML") {
		t.Fatalf("off-air notice must render, got %q", off)
	}

	m.AIR = true
	on := ansi.Strip(m.viewVital(160))
	if !strings.Contains(on, "withheld on air") {
		t.Fatalf("on-air notice must render the withheld label, got %q", on)
	}
	if strings.Contains(on, "token.toml") || strings.Contains(on, "fix the TOML") {
		t.Fatalf("on-air notice must never carry config detail, got %q", on)
	}
}
