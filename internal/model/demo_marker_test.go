package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// reins --demo: a FIXTURE instance must be UNMISSABLE as not-live on EVERY page (persistent title-bar
// chrome, set from frame 1) — never a single scroll-off status line, never a fabricated-live frame.
func TestDemoMarkerPersistsAcrossPages(t *testing.T) {
	m := New("REINS")
	m.Demo = true
	m.Width, m.Height = 160, 44
	for _, p := range []int{PageCoordinator, PageTasks, PageSessions, PageEvents, PageCommands, PageCaps, PageObserve} {
		m.Page = p
		out := ansi.Strip(m.View())
		if !strings.Contains(out, "DEMO") || !strings.Contains(out, "not live") {
			t.Fatalf("DEMO provenance marker must render on page %d (persistent chrome), got:\n%s",
				p, firstLine(out))
		}
	}
}

func TestLiveModelHasNoDemoMarker(t *testing.T) {
	m := New("REINS")
	m.Width, m.Height = 160, 44
	m.Page = PageCoordinator
	if strings.Contains(ansi.Strip(m.View()), "DEMO") {
		t.Fatal("a live (non-demo) model must NEVER show the DEMO marker (it would be a false provenance)")
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
