package model

import (
	"path/filepath"
	"strings"

	"github.com/hapax-systems/reins/internal/files"
	"github.com/hapax-systems/reins/internal/grammar"
)

// The injection basket stages files marked in the filebrowser zone ([space]) for the {{basket}}
// composer transclusion. It holds absolute paths only; the structured CoordChatPart parts (and any
// provider egress) are assembled later, behind the send-gate — staging mints nothing.

func (m Model) inBasket(path string) bool {
	for _, p := range m.Basket {
		if p == path {
			return true
		}
	}
	return false
}

// toggleBasket stages path if absent, unstages it if present (idempotent set membership).
func (m Model) toggleBasket(path string) Model {
	for i, p := range m.Basket {
		if p == path {
			m.Basket = append(append([]string{}, m.Basket[:i]...), m.Basket[i+1:]...)
			return m
		}
	}
	m.Basket = append(append([]string{}, m.Basket...), path)
	return m
}

// basketMarks returns a parallel-to-entries bool slice for the files-zone listing: marks[i] is true
// when entries[i] (joined onto cwd) is staged. Used to draw the ▣ stage glyph.
func (m Model) basketMarks(entries []files.Entry) []bool {
	marks := make([]bool, len(entries))
	for i, e := range entries {
		marks[i] = !e.IsDir && m.inBasket(filepath.Join(m.FilesCwd, e.Name))
	}
	return marks
}

// basketManifest renders the {{basket}} transclusion. AIR-safe: a staged FILENAME is sensitive
// (a name like "secret-draft.png" leaks regardless of the path), so the whole manifest denies on
// air; off air it lists the staged basenames as ▤ chips for the composer.
func (m Model) basketManifest() string {
	if len(m.Basket) == 0 {
		return "(basket empty)"
	}
	chips := make([]string, 0, len(m.Basket))
	for _, p := range m.Basket {
		chips = append(chips, "▤ "+filepath.Base(p))
	}
	return grammar.Redact(map[string]string{}, "basket", strings.Join(chips, " "), m.AIR)
}
