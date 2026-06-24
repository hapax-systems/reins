// Package palette is the cockpit's color grammar: ONE semantic token table per working mode
// (Gruvbox Hard Dark = R&D, Solarized Dark = Research). Color is a REDUNDANT AMPLIFIER layered on
// the glyph grammar — gray is the ground; color blooms only where it means something. Render code
// never hardcodes hex: it names a token (a meaning), so a mode flip recolors everything by name and
// a greyscale terminal still reads the glyph. Criticality = hue; ownership = its own channel.
package palette

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// token -> hex, per mode. Token names are MEANINGS shared across both palettes so render code is
// mode-agnostic. Severity hues (grn/yel/org/red), lane channels (blu/fch), ground greys, canvas.
var gruvbox = map[string]string{
	"yel": "#fabd2f", "grn": "#b8bb26", "red": "#fb4934", "org": "#fe8019",
	"blu": "#83a598", "fch": "#d3869b", "eme": "#8ec07c",
	"mut": "#928374", "2nd": "#bdae93", "pri": "#ebdbb2", "brt": "#fdf4c9",
	"bg": "#1d2021", "surface": "#282828", "focus": "#3c3836", "border": "#504945",
}
var solarized = map[string]string{
	"yel": "#b58900", "grn": "#859900", "red": "#dc322f", "org": "#cb4b16",
	"blu": "#268bd2", "fch": "#d33682", "eme": "#2aa198",
	"mut": "#586e75", "2nd": "#657b83", "pri": "#839496", "brt": "#93a1a1",
	"bg": "#002b36", "surface": "#073642", "focus": "#0a4856", "border": "#586e75",
}

type Palette struct {
	mode   string
	tokens map[string]string
}

// For returns the palette for a working mode. "solarized"/"research" -> Solarized; else Gruvbox.
func For(mode string) *Palette {
	t, m := gruvbox, "gruvbox"
	switch strings.ToLower(mode) {
	case "solarized", "research":
		t, m = solarized, "solarized"
	}
	return &Palette{mode: m, tokens: t}
}

func (p *Palette) Mode() string         { return p.mode }
func (p *Palette) Hex(token string) string { return p.tokens[token] }

// Colorize wraps text in the token's foreground color. Unknown token or empty text -> unchanged
// (graceful). In a non-color terminal lipgloss renders plain, so the glyph still carries meaning.
func (p *Palette) Colorize(token, text string) string {
	hex, ok := p.tokens[token]
	if !ok || text == "" {
		return text
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render(text)
}

// SeverityToken maps a severity word to its heat-axis color token (the primary criticality channel).
func SeverityToken(sev string) string {
	switch strings.ToLower(sev) {
	case "ok", "healthy", "done", "ship", "success", "merged", "live":
		return "grn"
	case "warn", "actionable", "review", "maj", "started", "progress", "in_progress":
		return "yel"
	case "urgent", "degraded", "major":
		return "org"
	case "crit", "critical", "failed", "fail", "blocked", "held":
		return "red"
	}
	return "mut"
}

// LaneToken maps an owner/lane to its ownership channel color (orthogonal to severity).
func LaneToken(owner string) string {
	switch {
	case owner == "alpha":
		return "org"
	case owner == "gov" || owner == "GOV" || owner == "governance":
		return "fch"
	case strings.HasPrefix(owner, "cc-") || strings.HasPrefix(owner, "cx-"):
		return "blu"
	}
	return "mut"
}

// ProvToken maps a dynamics provenance status to its confidence hue.
func ProvToken(status string) string {
	switch status {
	case "asserted", "observed":
		return "eme"
	case "simulated", "candidate":
		return "blu"
	case "inferred":
		return "2nd"
	}
	return "mut"
}
