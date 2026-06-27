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
	"github.com/hapax-systems/reins/internal/imgpreview"
)

// The filebrowser opens in the operator's screenshots dir by default — pickFilesDir returns the first
// existing candidate (screenshots → home → cwd), else the fallback.
func TestPickFilesDirPrefersFirstExisting(t *testing.T) {
	real := t.TempDir()
	if got := pickFilesDir([]string{"/no/such/dir", real, "/another"}, "."); got != real {
		t.Fatalf("want the first existing dir %q, got %q", real, got)
	}
	if got := pickFilesDir([]string{"/no/such/dir", ""}, "fallback"); got != "fallback" {
		t.Fatalf("want the fallback when no candidate exists, got %q", got)
	}
}

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

	// On air (operator ruling 2026-06-27): the AIR version of the image is the coarse BLOCK-PIXEL
	// (half-block) rendering — confidentiality-by-resolution. Pixels ARE shown, but clamped to a hard
	// coarseness ceiling so fine detail (text, faces) is destroyed below legibility; the filename is
	// still redacted.
	m.AIR = true
	on := m.coordinatorFilePreview(200, 80) // a large pane must NOT sharpen the on-air image
	pixRows := 0
	for _, ln := range strings.Split(on, "\n") {
		if strings.Contains(ln, "▀") {
			pixRows++
		}
	}
	if pixRows == 0 {
		t.Fatalf("on-air must render the coarse block-pixel image (half-block ▀):\n%s", on)
	}
	if pixRows > imgpreview.AIRMaxRows {
		t.Fatalf("on-air pixels must clamp to the AIR ceiling (≤%d rows), got %d:\n%s", imgpreview.AIRMaxRows, pixRows, on)
	}
	if strings.Contains(on, "pic.png") {
		t.Fatalf("on-air must still redact the filename:\n%s", on)
	}
}
