package files

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListDirSortsDirsFirstThenAlpha(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"zsub", "asub"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package x"), 0o644); err != nil {
		t.Fatal(err)
	}
	es, err := ListDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"asub", "zsub", "a.go", "b.txt"} // dirs first (alpha), then files (alpha)
	if len(es) != len(want) {
		t.Fatalf("want %d entries, got %d", len(want), len(es))
	}
	for i, w := range want {
		if es[i].Name != w {
			t.Fatalf("entry %d = %q, want %q", i, es[i].Name, w)
		}
	}
	if !es[0].IsDir || es[2].IsDir {
		t.Fatal("dir/file flags wrong")
	}
	if es[2].Ext != "go" {
		t.Fatalf("a.go ext = %q, want go", es[2].Ext)
	}
	if es[3].Size != 2 {
		t.Fatalf("b.txt size = %d, want 2", es[3].Size)
	}
}

func TestRenderListCursorAndGlyphs(t *testing.T) {
	es := []Entry{
		{Name: "src", IsDir: true},
		{Name: "main.go", Size: 1500, Ext: "go"},
	}
	out := RenderList(es, "/proj/x", 1, false, 60)
	for _, want := range []string{"src", "main.go", "▸", "/proj/x"} { // entries, dir glyph, cwd
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
	// The cursor (row 1 = main.go) carries the ▶ marker.
	var cursorLine string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "main.go") {
			cursorLine = l
		}
	}
	if !strings.Contains(cursorLine, "▶") {
		t.Fatalf("cursor row should carry ▶:\n%s", out)
	}
}

func TestRenderListRedactsCwdOnAir(t *testing.T) {
	out := RenderList([]Entry{{Name: "f", Size: 1}}, "/srv/secret/proj", 0, true, 60)
	if strings.Contains(out, "/srv/secret/proj") || strings.Contains(out, "secret") {
		t.Fatalf("cwd path is SENSITIVE — must redact on air:\n%s", out)
	}
	if !strings.Contains(out, "▒▒▒") {
		t.Fatalf("redaction token expected on air:\n%s", out)
	}
}

func TestDotfileHasNoExtension(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".bashrc"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	es, err := ListDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if es[0].Name != ".bashrc" || es[0].Ext != "" {
		t.Fatalf("a dotfile has no extension; got name=%q ext=%q", es[0].Name, es[0].Ext)
	}
}

func TestRenderListLargeSizeAndGrayscaleBreadcrumb(t *testing.T) {
	out := RenderList([]Entry{{Name: "huge.bin", Size: 2 << 30, Ext: "bin"}}, "/p", 0, false, 60)
	if !strings.Contains(out, "GB") {
		t.Fatalf("a 2GiB file must render in GB, not overflow as MB:\n%s", out)
	}
	// Grayscale invariant: the cursor glyph ▶ must appear exactly once (the cursor row), not also
	// on the breadcrumb — state is carried by glyph, not by color.
	if n := strings.Count(out, "▶"); n != 1 {
		t.Fatalf("cursor glyph ▶ must be unique to the cursor row, found %d:\n%s", n, out)
	}
}

func TestRenderListEmptyAndOOBNoPanic(t *testing.T) {
	if strings.TrimSpace(RenderList(nil, "/x", 0, false, 40)) == "" {
		t.Fatal("an empty directory should still render an honest line")
	}
	_ = RenderList([]Entry{{Name: "a"}}, "/x", 99, false, 40) // out-of-bounds cursor must not panic
}
