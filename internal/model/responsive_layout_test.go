package model

import (
	"strings"
	"testing"
)

func TestDeviceClassSelection(t *testing.T) {
	for s, want := range map[string]DeviceClass{
		"handheld": DeviceHandheld, "deck": DeviceHandheld, "steamdeck": DeviceHandheld,
		"compact": DeviceCompact, "mobile": DeviceCompact, "portrait": DeviceCompact,
		"monitor": DeviceMonitor, "desktop": DeviceMonitor, "": DeviceAuto, "garbage": DeviceAuto,
	} {
		if got := parseDeviceClass(s); got != want {
			t.Fatalf("parseDeviceClass(%q)=%v want %v", s, got, want)
		}
	}
	// dimension fallback (no override): narrow -> compact, wide -> monitor
	m := New("REINS")
	m.DeviceOverride = DeviceAuto
	m.Width = 60
	if m.deviceClass() != DeviceCompact {
		t.Fatal("a 60-col viewport should infer Compact")
	}
	m.Width = 200
	if m.deviceClass() != DeviceMonitor {
		t.Fatal("a 200-col viewport should infer Monitor")
	}
	// an explicit override wins over dimensions (a 200-col deck stays handheld)
	m.DeviceOverride = DeviceHandheld
	if m.deviceClass() != DeviceHandheld || m.paneCap() != 2 {
		t.Fatal("REINS_DEVICE=handheld must override the width inference (cap 2)")
	}
	// pane caps
	m.DeviceOverride = DeviceCompact
	if m.paneCap() != 1 {
		t.Fatal("compact cap must be 1")
	}
	m.DeviceOverride = DeviceMonitor
	if m.paneCap() != 3 {
		t.Fatal("monitor cap must be 3")
	}
}

// The coordinator page collapses its pane count to the device — the operator's cramping fix.
func TestResponsiveCoordinatorCollapse(t *testing.T) {
	view := func(dev DeviceClass, w int) string {
		m := New("REINS")
		m.Page, m.Width, m.Height, m.DeviceOverride = PageCoordinator, w, 44, dev
		return m.View()
	}
	// MONITOR @220 -> full 3-pane (the middle CROW context pane is present)
	if mon := view(DeviceMonitor, 220); !strings.Contains(mon, "CROW") {
		t.Fatal("monitor must keep the 3-pane coordinator (CROW context pane)")
	}
	// HANDHELD @164 -> 2-pane LENS|CHAT; the middle CROW pane DROPS (was the cramp), chat survives
	hh := view(DeviceHandheld, 164)
	if !strings.Contains(hh, "LENS") || !strings.Contains(hh, "CHAT") {
		t.Fatalf("handheld coordinator must keep LENS + CHAT")
	}
	if strings.Contains(hh, "CROW") {
		t.Fatal("handheld must COLLAPSE the 3rd coordinator pane (the cramping) — got the middle pane")
	}
	// COMPACT @80 -> single column (chat only): no LENS/CROW pane headers, chat content present
	c := view(DeviceCompact, 80)
	if strings.Contains(c, "CROW") || strings.Contains(c, "LENS ·") {
		t.Fatal("compact must be a single column (no lens/coordinator pane headers)")
	}
	if !strings.Contains(c, "steer") && !strings.Contains(c, "hapax") {
		t.Fatal("compact must still render the chat (steer) column")
	}
}

// A list page collapses its 2-pane list|context to a single column on a compact viewport (paneCap drives
// specListContext, exercised end-to-end by the coordinator render test above).
func TestResponsiveListContextCap(t *testing.T) {
	mon := New("REINS")
	mon.Page, mon.Width, mon.DeviceOverride = PageTasks, 200, DeviceMonitor
	if mon.paneCap() < 2 {
		t.Fatal("monitor tasks should allow a 2-pane list|context")
	}
	cmp := New("REINS")
	cmp.Page, cmp.Width, cmp.DeviceOverride = PageTasks, 70, DeviceCompact
	if cmp.paneCap() != 1 {
		t.Fatal("compact tasks must be single-column (cap 1)")
	}
}
