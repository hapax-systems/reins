// Package smoke is the headless navigation driver for the Reins cockpit — it feeds key sequences
// through the live Model.Update loop and captures the rendered frame after each step, so navigation
// can be smoke-tested and visually inspected WITHOUT a human at a terminal. The captured frames are
// the AVSDLC visual witness (render them to PNG via `freeze --language ansi` and inspect / eval).
package smoke

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/hapax-systems/reins/internal/model"
)

// Frame is one captured step: the keys that produced it, the rendered View (raw ANSI), and a panic
// message if Update/View crashed on this step (recovered so the run continues — a crash is the
// finding, not an abort).
type Frame struct {
	Step  string
	View  string
	Panic string
}

// Plain is the View with ANSI stripped — what a text reader / metric witness inspects.
func (f Frame) Plain() string { return ansi.Strip(f.View) }

// namedKeys maps the named (non-rune) key tokens to their tea key type.
var namedKeys = map[string]tea.KeyType{
	"enter": tea.KeyEnter, "ret": tea.KeyEnter,
	"esc": tea.KeyEsc, "escape": tea.KeyEsc,
	"tab": tea.KeyTab,
	"bs":  tea.KeyBackspace, "backspace": tea.KeyBackspace,
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
}

// Expand turns one step token into its tea.KeyMsg sequence:
//   - ":word"      → type the command (':' then each rune then Enter)
//   - "enter"/"esc"/"tab"/"space"/"up"/… → that one named key
//   - any other    → each rune is one keypress (so "jjk" = j,j,k; "a" = a)
func Expand(tok string) []tea.KeyMsg {
	if tok == "" {
		return nil
	}
	if strings.HasPrefix(tok, ":") {
		keys := []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune(":")}}
		for _, r := range tok[1:] {
			keys = append(keys, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		return append(keys, tea.KeyMsg{Type: tea.KeyEnter})
	}
	if tok == "space" || tok == "spc" {
		return []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune(" ")}}
	}
	if kt, ok := namedKeys[tok]; ok {
		return []tea.KeyMsg{{Type: kt}}
	}
	keys := make([]tea.KeyMsg, 0, len(tok))
	for _, r := range tok {
		keys = append(keys, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return keys
}

// Drive feeds each step (a space-separated list of key tokens) through m.Update and captures the
// rendered View after the step. Panics in Update/View are RECOVERED and recorded on the frame so one
// bad page never aborts the smoke — the panic is the finding. m must have Width/Height set.
func Drive(m model.Model, steps []string) []Frame {
	frames := make([]Frame, 0, len(steps))
	for _, step := range steps {
		f := Frame{Step: step}
		func() {
			defer func() {
				if r := recover(); r != nil {
					f.Panic = fmt.Sprintf("%v", r)
				}
			}()
			for _, tok := range strings.Fields(step) {
				for _, k := range Expand(tok) {
					nm, _ := m.Update(k)
					m = nm.(model.Model)
				}
			}
			f.View = m.View()
		}()
		frames = append(frames, f)
	}
	return frames
}
