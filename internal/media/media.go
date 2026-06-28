// Package media is the preview-container layer above imgpreview. It keeps terminal graphics bytes
// out of layout render strings: true-pixel images are transmitted once by tea.Cmd, while Pane.Render
// returns only Unicode placeholders that compose with the layout pure fold and Bubble Tea diffing.
package media

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/imgpreview"
	"github.com/hapax-systems/reins/internal/layout"
)

// Rect records the current intended placement rectangle in terminal cells. Placement is still driven
// by the pane's Render(w,h) budget; Rect is metadata for owners that diff live media across frames.
type Rect struct {
	X, Y       int
	Cols, Rows int
}

// Container describes one previewable media item and its effective fidelity. Fidelity should be the
// result of ClampFidelity(capabilityTier, air): AIR is a hard ceiling at Tier1.
type Container struct {
	Source   string
	Fidelity imgpreview.CapabilityTier
	ID       uint32
	Rect     Rect
}

// New constructs a Container with the doctrine-level fidelity clamp applied.
func New(source string, id uint32, capability imgpreview.CapabilityTier, air bool, rect Rect) Container {
	return Container{Source: source, ID: id, Fidelity: ClampFidelity(capability, air), Rect: rect}
}

// ClampFidelity implements Fidelity = min(capability_tier, AIR ? Tier1 : Tier3). AIR is a hard
// ceiling: even a Kitty-capable terminal renders only the coarse half-block tier on air.
func ClampFidelity(capability imgpreview.CapabilityTier, air bool) imgpreview.CapabilityTier {
	if capability < imgpreview.Tier0Metadata {
		capability = imgpreview.Tier0Metadata
	}
	if capability > imgpreview.Tier3TruePixel {
		capability = imgpreview.Tier3TruePixel
	}
	ceiling := imgpreview.Tier3TruePixel
	if air {
		ceiling = imgpreview.Tier1HalfBlock
	}
	if capability > ceiling {
		return ceiling
	}
	return capability
}

// Pane returns a layout leaf whose render function chooses exactly one representation for the
// current cell budget: Kitty placeholders, braille, half-block, or pixel-free metadata.
func (c Container) Pane() *layout.Pane {
	return &layout.Pane{MinW: 8, Render: func(w, h int) string {
		switch {
		case c.Fidelity >= imgpreview.Tier3TruePixel:
			return imgpreview.EmitKittyPlaceholder(c.ID, w, h)
		case c.Fidelity >= imgpreview.Tier2Braille:
			out, _ := imgpreview.RenderFileBraille(c.Source, w, h)
			return out
		case c.Fidelity >= imgpreview.Tier1HalfBlock:
			out, _ := imgpreview.RenderFile(c.Source, w, h, imgpreview.ProtoHalfBlock)
			return out
		default:
			return imgpreview.Metadata(c.Source)
		}
	}}
}

// LiveMediaIDs is the first-class live-set used to diff terminal-side images between frames. Deleted
// ids MUST emit a KGP a=d command so terminal image data does not leak across teardown/navigation.
type LiveMediaIDs map[uint32]struct{}

func NewLiveMediaIDs(ids ...uint32) LiveMediaIDs {
	out := LiveMediaIDs{}
	for _, id := range ids {
		if id != 0 {
			out[id] = struct{}{}
		}
	}
	return out
}

func (s LiveMediaIDs) Has(id uint32) bool {
	_, ok := s[id]
	return ok
}

func (s LiveMediaIDs) Add(id uint32) bool {
	if id == 0 || s.Has(id) {
		return false
	}
	s[id] = struct{}{}
	return true
}

func (s LiveMediaIDs) Clone() LiveMediaIDs {
	out := LiveMediaIDs{}
	for id := range s {
		out[id] = struct{}{}
	}
	return out
}

// DiffDeleted returns ids present in previous but absent from next, sorted for deterministic command
// payloads/tests.
func DiffDeleted(previous, next LiveMediaIDs) []uint32 {
	var deleted []uint32
	for id := range previous {
		if _, ok := next[id]; !ok {
			deleted = append(deleted, id)
		}
	}
	sort.Slice(deleted, func(i, j int) bool { return deleted[i] < deleted[j] })
	return deleted
}

// TerminalGraphicsMsg carries terminal-graphics escape bytes produced by a tea.Cmd. Integrators can
// write Payload to the terminal/output path appropriate for their Bubble Tea renderer.
type TerminalGraphicsMsg struct {
	Op      string
	IDs     []uint32
	Payload string
	Err     error
}

// TransmitOnce returns a command that transmits the image once for Tier-3 containers. The caller's
// live set is updated eagerly to make repeat calls no-ops; teardown deletion is handled separately by
// DeleteTeardownCmd(DiffDeleted(...)).
func (c Container) TransmitOnce(live LiveMediaIDs) tea.Cmd {
	if c.Fidelity < imgpreview.Tier3TruePixel || c.ID == 0 {
		return nil
	}
	if live == nil {
		live = LiveMediaIDs{}
	}
	if !live.Add(c.ID) {
		return nil
	}
	return func() tea.Msg {
		payload, err := c.kittyTransmitPayload()
		return TerminalGraphicsMsg{Op: "transmit", IDs: []uint32{c.ID}, Payload: payload, Err: err}
	}
}

// DeleteTeardownCmd emits a KGP a=d delete for every id that left the live set. This is the teardown
// correctness path: image IDs are diffed, sorted, and deleted explicitly rather than relying on view
// disappearance to clean up terminal-side state.
func DeleteTeardownCmd(previous, next LiveMediaIDs) tea.Cmd {
	deleted := DiffDeleted(previous, next)
	if len(deleted) == 0 {
		return nil
	}
	return func() tea.Msg {
		var sb strings.Builder
		for _, id := range deleted {
			sb.WriteString(KittyDeletePayload(id))
		}
		return TerminalGraphicsMsg{Op: "delete", IDs: deleted, Payload: sb.String()}
	}
}

func KittyDeletePayload(id uint32) string {
	if id == 0 {
		return ""
	}
	// d=I deletes placements for this image id and frees the image data when no placements remain.
	return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d\x1b\\", id)
}

func (c Container) kittyTransmitPayload() (string, error) {
	pngBytes, err := pngBytesFromSource(c.Source)
	if err != nil {
		return "", err
	}
	cols, rows := c.Rect.Cols, c.Rect.Rows
	if cols <= 0 {
		cols = 1
	}
	if rows <= 0 {
		rows = 1
	}
	return KittyTransmitPayload(c.ID, cols, rows, pngBytes), nil
}

func pngBytesFromSource(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// KittyTransmitPayload creates a chunked Kitty graphics transmission with a virtual placement. The
// payload is out-of-band terminal graphics data; it must not be returned from Pane.Render.
func KittyTransmitPayload(id uint32, cols, rows int, pngBytes []byte) string {
	if id == 0 || len(pngBytes) == 0 {
		return ""
	}
	if cols <= 0 {
		cols = 1
	}
	if rows <= 0 {
		rows = 1
	}
	encoded := base64.StdEncoding.EncodeToString(pngBytes)
	const chunk = 4096
	var sb strings.Builder
	for offset := 0; offset < len(encoded); offset += chunk {
		end := offset + chunk
		if end > len(encoded) {
			end = len(encoded)
		}
		more := 0
		if end < len(encoded) {
			more = 1
		}
		if offset == 0 {
			fmt.Fprintf(&sb, "\x1b_Ga=T,f=100,t=d,i=%d,q=2,U=1,c=%d,r=%d,m=%d;%s\x1b\\", id, cols, rows, more, encoded[offset:end])
		} else {
			fmt.Fprintf(&sb, "\x1b_Gm=%d;%s\x1b\\", more, encoded[offset:end])
		}
	}
	return sb.String()
}
