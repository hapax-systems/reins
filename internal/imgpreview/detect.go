// Package imgpreview is the Reins filebrowser image-preview substrate (research brief
// reins-design-ref, Increment 0). It selects a terminal
// image-rendering protocol from the environment and renders an image into a string that composes
// safely with a Bubble Tea View() — the universal floor is a pure-stdlib ANSI half-block grid, so
// it needs no external binary (chafa) and works in tmux, Konsole, and any truecolor terminal.
//
// Egress note: rendering pixels is a PREVIEW concern, separate from PROVIDER/ON-AIR egress. The
// on-air / default-deny path uses Metadata() (pixel-free); pixels are only ever drawn in the
// operator's present-at-hand frame, never aired by construction.
package imgpreview

import "strings"

// Protocol is the chosen image-preview rendering strategy.
type Protocol int

const (
	// ProtoMetadataOnly renders no pixels — only a textual descriptor (the safe floor of the floor).
	ProtoMetadataOnly Protocol = iota
	// ProtoHalfBlock is the universal string-native floor: a truecolor ANSI half-block (▀) grid.
	ProtoHalfBlock
	// ProtoKitty is the kitty/ghostty Unicode-placeholder tier (highest fidelity). Placeholder
	// emission is a noted follow-up; today the caller floors ProtoKitty to the half-block render.
	ProtoKitty
)

func (p Protocol) String() string {
	switch p {
	case ProtoKitty:
		return "kitty"
	case ProtoHalfBlock:
		return "halfblock"
	default:
		return "metadata"
	}
}

// InTmux reports whether the session is inside a tmux pane.
func InTmux(env func(string) string) bool { return env("TMUX") != "" }

func kittyCapable(env func(string) string) bool {
	if env("KITTY_WINDOW_ID") != "" || env("GHOSTTY_RESOURCES_DIR") != "" || env("GHOSTTY_BIN_DIR") != "" {
		return true
	}
	term := strings.ToLower(env("TERM"))
	tp := strings.ToLower(env("TERM_PROGRAM"))
	return term == "xterm-kitty" || tp == "ghostty" || tp == "kitty"
}

func truecolor(env func(string) string) bool {
	switch strings.ToLower(env("COLORTERM")) {
	case "truecolor", "24bit":
		return true
	}
	return strings.Contains(env("TERM"), "256color")
}

// DetectProtocol picks an image-preview protocol from environment hints, conservatively. Kitty
// placeholders are claimed ONLY outside tmux: inside tmux they require `allow-passthrough on`,
// which the environment cannot confirm, so claiming kitty there would risk a broken blit — the
// universal half-block floor is chosen instead. Any truecolor/256-color terminal gets the floor;
// with no color signal at all, only metadata is shown (never paint pixels we cannot render).
func DetectProtocol(env func(string) string) Protocol {
	if kittyCapable(env) && !InTmux(env) {
		return ProtoKitty
	}
	if truecolor(env) {
		return ProtoHalfBlock
	}
	return ProtoMetadataOnly
}

// TmuxWrap wraps terminal-graphics bytes in the tmux DCS passthrough envelope, doubling every ESC
// as tmux requires. Needed when a kitty/sixel blit must traverse a tmux pane with passthrough on.
func TmuxWrap(payload string) string {
	return "\x1bPtmux;" + strings.ReplaceAll(payload, "\x1b", "\x1b\x1b") + "\x1b\\"
}
