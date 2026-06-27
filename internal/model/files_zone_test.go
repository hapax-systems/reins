package model

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/files"
)

// The coordinator [z] toggle enters/leaves the filebrowser zone; while active, j/k/l/h drive the
// FILES cursor (not the task focus), and the lens pane renders the listing. (Runs against the
// package directory, which `go test` makes the cwd — so the listing is non-empty.)
func TestCoordinatorFilesZoneToggleAndNav(t *testing.T) {
	send := func(m Model, r rune) Model {
		nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		return nm.(Model)
	}
	m := New("R")
	m.Page = PageCoordinator

	m = send(m, 'z') // enter files zone
	if m.LensZone != "files" {
		t.Fatalf("[z] should enter the files zone, got %q", m.LensZone)
	}
	if len(m.FilesEntries) == 0 {
		t.Fatal("entering the files zone should load the directory listing")
	}

	if len(m.FilesEntries) > 1 { // [j] advances the FILES cursor, not the task focus
		before := m.FilesCursor
		m = send(m, 'j')
		if m.FilesCursor != before+1 {
			t.Fatalf("[j] should advance the files cursor: %d -> %d", before, m.FilesCursor)
		}
	}

	if !strings.Contains(m.coordinatorLensPane(70, 40), "▸files") {
		t.Fatal("the lens pane should mark the files zone active")
	}

	m = send(m, 'z') // leave files zone
	if m.LensZone != "tasks" {
		t.Fatalf("[z] should leave the files zone, got %q", m.LensZone)
	}
}

// The image browser shows the actual image off air (the operator's present-at-hand frame), and is
// shape-only on air (pixels withheld, filename redacted) — egress-safe by construction.
func TestCoordinatorFilePreviewImageOffAirOnAir(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 180
	}
	f, err := os.Create(filepath.Join(dir, "pic.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	m := New("R")
	m.Page = PageCoordinator
	m.LensZone = "files"
	m.FilesCwd = dir
	m.FilesEntries = []files.Entry{{Name: "pic.png", Size: 200, Ext: "png"}}

	// Off air: the operator sees the real image, rendered as higher-res braille dot-matrix.
	m.AIR = false
	off := m.coordinatorFilePreview(60, 24)
	if strings.TrimSpace(off) == "" {
		t.Fatal("off-air image preview must not be empty")
	}
	braille := false
	for _, r := range off {
		if r >= 0x2800 && r <= 0x28FF {
			braille = true
			break
		}
	}
	if !braille {
		t.Fatalf("off-air preview should render the image as braille dot-matrix:\n%s", off)
	}

	// On air: shape-only — no pixels, filename redacted, the withheld note shown.
	m.AIR = true
	on := m.coordinatorFilePreview(60, 24)
	if strings.Contains(on, "▀") {
		t.Fatalf("on-air must NOT egress image pixels:\n%s", on)
	}
	if strings.Contains(on, "pic.png") {
		t.Fatalf("on-air must redact the filename:\n%s", on)
	}
	if !strings.Contains(on, "shape-only") {
		t.Fatalf("on-air must state pixels are withheld:\n%s", on)
	}
}
