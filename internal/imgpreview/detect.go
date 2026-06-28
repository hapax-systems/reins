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

import (
	"fmt"
	"regexp"
	"strings"
)

// Protocol is the chosen image-preview rendering strategy.
type Protocol int

const (
	// ProtoMetadataOnly renders no pixels — only a textual descriptor (the safe floor of the floor).
	ProtoMetadataOnly Protocol = iota
	// ProtoHalfBlock is the universal string-native floor: a truecolor ANSI half-block (▀) grid.
	ProtoHalfBlock
	// ProtoKitty is the kitty/ghostty Unicode-placeholder tier (highest fidelity). The actual pixels
	// are transmitted once out-of-band; Bubble Tea render strings contain only placeholders.
	ProtoKitty
	// ProtoSixel is intentionally a later-increment stub. This increment does not claim Sixel.
	ProtoSixel
)

func (p Protocol) String() string {
	switch p {
	case ProtoKitty:
		return "kitty"
	case ProtoHalfBlock:
		return "halfblock"
	case ProtoSixel:
		return "sixel-stub"
	default:
		return "metadata"
	}
}

// CapabilityTier is the preview-fidelity ladder. It intentionally separates terminal capability
// from a particular renderer so AIR can clamp fidelity without lying about terminal support.
type CapabilityTier int

const (
	Tier0Metadata CapabilityTier = iota
	Tier1HalfBlock
	Tier2Braille
	Tier3TruePixel
)

func (t CapabilityTier) String() string {
	switch t {
	case Tier3TruePixel:
		return "tier3:true-pixel"
	case Tier2Braille:
		return "tier2:braille"
	case Tier1HalfBlock:
		return "tier1:half-block"
	default:
		return "tier0:metadata"
	}
}

// Detection records both the legacy protocol choice and the fidelity tier that should drive new
// media containers. Authoritative means an in-band Kitty graphics query won the DA race; otherwise
// the result is an environment pre-guess/fallback.
type Detection struct {
	Protocol       Protocol
	CapabilityTier CapabilityTier
	Authoritative  bool
	Source         string
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
	return false
}

func colorCapable(env func(string) string) bool {
	return truecolor(env) || strings.Contains(strings.ToLower(env("TERM")), "256color")
}

// KittyTmuxPassthroughSupported is an honest later-increment stub: Tier-3 in tmux requires an
// attempt-then-verify passthrough path, not an environment guess.
func KittyTmuxPassthroughSupported(func(string) string) bool { return false }

// SixelSupported is an honest later-increment stub. Sixel is not detected or rendered in this Tier-3
// Kitty-placeholder increment.
func SixelSupported(func(string) string) bool { return false }

func protocolForTier(t CapabilityTier) Protocol {
	switch {
	case t >= Tier3TruePixel:
		return ProtoKitty
	case t >= Tier1HalfBlock:
		return ProtoHalfBlock
	default:
		return ProtoMetadataOnly
	}
}

func envDetection(env func(string) string) Detection {
	tier := Tier0Metadata
	source := "env:metadata"
	if kittyCapable(env) && !InTmux(env) {
		tier = Tier3TruePixel
		source = "env:kitty-off-tmux"
	} else if truecolor(env) {
		tier = Tier2Braille
		source = "env:truecolor"
	} else if colorCapable(env) {
		tier = Tier1HalfBlock
		source = "env:256color"
	}
	return Detection{Protocol: protocolForTier(tier), CapabilityTier: tier, Source: source}
}

// DetectTerminal returns the conservative environment pre-guess. Use DetectTerminalFromResponse
// after sending KittyGraphicsQuery(id) + CSI c to upgrade this to an authoritative in-band result.
func DetectTerminal(env func(string) string) Detection { return envDetection(env) }

// DetectTerminalFromResponse promotes an env pre-guess when the terminal answered the Kitty graphics
// query before the device-attributes reply. Tier-3 is intentionally off-tmux only in this increment;
// tmux passthrough is a later attempt-then-verify path, so an OK inside tmux remains a fallback.
func DetectTerminalFromResponse(env func(string) string, queryID uint32, response string) Detection {
	if KittyOKBeforeDeviceAttributes(response, queryID) && !InTmux(env) {
		return Detection{Protocol: ProtoKitty, CapabilityTier: Tier3TruePixel, Authoritative: true, Source: "kgp-query"}
	}
	d := envDetection(env)
	if KittyOKBeforeDeviceAttributes(response, queryID) && InTmux(env) {
		d.Source += "+kgp-tmux-stubbed"
	}
	return d
}

// DetectProtocol keeps the legacy API for existing call-sites while the richer Detection carries
// capability_tier for the new media path.
func DetectProtocol(env func(string) string) Protocol { return DetectTerminal(env).Protocol }

// KittyGraphicsQuery is the authoritative support probe: a 1×1 direct RGB query followed by a
// primary device-attributes request. If the _Gi=<id>;OK response arrives first, KGP is supported.
func KittyGraphicsQuery(id uint32) string {
	if id == 0 {
		id = 1
	}
	return fmt.Sprintf("\x1b_Gi=%d,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\\x1b[c", id)
}

// KittyOKBeforeDeviceAttributes parses the response stream for the KGP-vs-DA race. It accepts only
// an OK for the same image id, and only if that OK appears before a CSI ... c DA response.
func KittyOKBeforeDeviceAttributes(response string, id uint32) bool {
	if id == 0 {
		return false
	}
	kgp := strings.Index(response, fmt.Sprintf("\x1b_Gi=%d;OK\x1b\\", id))
	if kgp < 0 {
		return false
	}
	da := deviceAttributesRe.FindStringIndex(response)
	return da == nil || kgp < da[0]
}

var deviceAttributesRe = regexp.MustCompile(`\x1b\[[?0-9;]*c`)

// TmuxWrap wraps terminal-graphics bytes in the tmux DCS passthrough envelope, doubling every ESC
// as tmux requires. Needed when a kitty/sixel blit must traverse a tmux pane with passthrough on.
func TmuxWrap(payload string) string {
	return "\x1bPtmux;" + strings.ReplaceAll(payload, "\x1b", "\x1b\x1b") + "\x1b\\"
}
