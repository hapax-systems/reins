package model

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hapax-systems/reins/internal/files"
)

// The injection BASKET: the operator marks files in the filebrowser zone ([space]); the staged set
// feeds the {{basket}} composer transclusion. AIR: the STAGE mark is the operator's own structural
// action (survives on air); the file names/paths it points at stay sensitive and redact.

func TestBasketToggleStagesAndUnstages(t *testing.T) {
	m := New("REINS")
	p := "/srv/x/a.png"
	if m.inBasket(p) {
		t.Fatal("fresh basket must not contain the path")
	}
	m = m.toggleBasket(p)
	if !m.inBasket(p) || len(m.Basket) != 1 {
		t.Fatalf("toggle should stage the path; got %v", m.Basket)
	}
	m = m.toggleBasket(p)
	if m.inBasket(p) || len(m.Basket) != 0 {
		t.Fatalf("re-toggle should unstage the path; got %v", m.Basket)
	}
}

func TestFilesZoneSpaceStagesFocusedFileNotDir(t *testing.T) {
	m := New("REINS")
	m.LensZone = "files"
	m.FilesCwd = "/srv/x"
	m.FilesEntries = []files.Entry{{Name: "a.png", Ext: "png"}, {Name: "sub", IsDir: true}}
	m.FilesCursor = 0
	space := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}
	next, _, handled := m.updateFilesZone(space)
	if !handled {
		t.Fatal("[space] must be owned by the files zone")
	}
	m = next.(Model)
	if !m.inBasket("/srv/x/a.png") {
		t.Fatalf("[space] on a file must stage it; got %v", m.Basket)
	}
	// move to the directory and try to stage it — directories are not stageable
	m.FilesCursor = 1
	next, _, _ = m.updateFilesZone(space)
	m = next.(Model)
	if m.inBasket("/srv/x/sub") || len(m.Basket) != 1 {
		t.Fatalf("[space] on a directory must not stage it; got %v", m.Basket)
	}
}

func TestBasketTemplateExpandsOffAirRedactsOnAir(t *testing.T) {
	m := New("REINS")
	m.Basket = []string{"/srv/x/secret-a.png", "/srv/x/secret-b.pdf"}
	off, ok := m.templateValue("basket")
	if !ok {
		t.Fatal("{{basket}} must resolve")
	}
	if !strings.Contains(off, "secret-a.png") || !strings.Contains(off, "secret-b.pdf") {
		t.Fatalf("off-air {{basket}} must list the staged basenames:\n%s", off)
	}
	m.AIR = true
	on, _ := m.templateValue("basket")
	if strings.Contains(on, "secret-a.png") || strings.Contains(on, "secret-b.pdf") {
		t.Fatalf("on-air {{basket}} must NOT leak basenames:\n%s", on)
	}
	if !strings.Contains(on, "▒▒▒") {
		t.Fatalf("on-air {{basket}} must redact:\n%s", on)
	}
}

func TestRenderListMarkedShowsStageGlyph(t *testing.T) {
	es := []files.Entry{{Name: "a.png", Ext: "png"}, {Name: "b.png", Ext: "png"}}
	out := files.RenderListMarked(es, "/x", 0, []bool{true, false}, false, 60)
	if strings.Count(out, "▣") != 1 {
		t.Fatalf("exactly the marked entry must show the stage glyph ▣:\n%s", out)
	}
	// back-compat: the unmarked RenderList draws no stage glyph
	if strings.Contains(files.RenderList(es, "/x", 0, false, 60), "▣") {
		t.Fatalf("plain RenderList must not draw stage glyphs")
	}
}
