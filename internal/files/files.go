// Package files is the Reins filebrowser's data + render layer (filebrowser research brief
// Increment 1 foundation). It lists a directory and renders it as a Miller-column listing whose
// state is carried by glyph (dir ▸ / file ·) so it reads in grayscale. The cwd PATH is sensitive
// and redacts on air through grammar.Redact — the SAME interlock the task rows and the injection
// composer use, never a parallel raw-value path (the leak class GLM-via-CC caught in the composer).
//
// This layer is pure of any tea/model state: ListDir does the I/O, RenderList is a pure function.
// The L0 zone-switch + descent that mount this into the coordinator lens land as a follow-up.
package files

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hapax-systems/reins/internal/grammar"
)

// Entry is one directory child.
type Entry struct {
	Name  string
	IsDir bool
	Size  int64
	Ext   string // lowercase extension without the dot ("" for none / directories)
}

// ListDir reads a directory and returns its children sorted dirs-first then case-insensitive alpha.
func ListDir(path string) ([]Entry, error) {
	des, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(des))
	for _, de := range des {
		e := Entry{Name: de.Name(), IsDir: de.IsDir()}
		if !e.IsDir {
			if info, err := de.Info(); err == nil {
				e.Size = info.Size()
			} else {
				e.Size = -1 // stat failed after readdir — render "?" honestly, not a bogus 0B
			}
			// filepath.Ext(".bashrc") == ".bashrc": a dotfile has a leading dot, not an extension.
			if ext := filepath.Ext(e.Name); ext != "" && ext != e.Name {
				e.Ext = strings.ToLower(strings.TrimPrefix(ext, "."))
			}
		}
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir // directories first
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

func humanSize(n int64) string {
	switch {
	case n < 0:
		return "?" // unknown (stat failed) — honest, not a fake 0B
	case n >= 1<<40:
		return fmt.Sprintf("%.1fTB", float64(n)/(1<<40))
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func clipName(s string, n int) string {
	if n < 4 {
		n = 4
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// RenderList renders a directory listing: a path-redacted breadcrumb, then one row per entry. The
// cursor row is marked ▶, directories carry ▸ and a trailing slash, files carry · plus size+type.
// The cwd is SENSITIVE — it redacts on air (default-deny). Out-of-range cursor and empty listings
// render honestly and never panic.
func RenderList(entries []Entry, cwd string, cursor int, airOn bool, width int) string {
	if width <= 0 {
		width = 60
	}
	var b strings.Builder
	crumb := grammar.Redact(nil, "path", cwd, airOn) // filesystem path is sensitive on air
	// Breadcrumb uses a path glyph «»» distinct from the three state glyphs (dir ▸ / file · /
	// cursor ▶) so the listing reads in grayscale — the breadcrumb is never mistaken for the cursor.
	b.WriteString(grammar.C("2nd", "» ") + grammar.C("mut", crumb) + "\n")
	if len(entries) == 0 {
		b.WriteString(grammar.C("mut", "  (empty directory)"))
		return b.String()
	}
	nameBudget := width - 20
	for i, e := range entries {
		mark := "  "
		if i == cursor {
			mark = grammar.C("yel", "▶ ")
		}
		glyph := grammar.C("2nd", "· ")
		name := clipName(e.Name, nameBudget)
		var meta string
		if e.IsDir {
			glyph = grammar.C("pri", "▸ ")
			name += "/"
		} else {
			meta = "  " + grammar.C("mut", humanSize(e.Size))
			if e.Ext != "" {
				meta += "  " + grammar.C("2nd", e.Ext)
			}
		}
		b.WriteString(mark + glyph + name + meta)
		if i < len(entries)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
