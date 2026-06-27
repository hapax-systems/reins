package grammar

import (
	"strings"
	"testing"
)

// The trainyard metro-map encodes SDLC honesty in shape+position; color is a redundant
// amplifier. These tests pin the four hard rules: WITNESS terminus (not deploy), the DARK
// gate ✖ (never green), blocked→siding, and grayscale-legibility (state in the glyphs).

func TestTrainyardStationsTerminateAtWitness(t *testing.T) {
	last := trainyardStations[len(trainyardStations)-1]
	if last != "WIT" {
		t.Fatalf("WITNESS must be the terminus station; got %q", last)
	}
	// Deploy is never the terminus: a shipped-but-unwitnessed task (S8) maps to DEP,
	// strictly left of WITNESS — "merged-unwitnessed sits penultimate".
	if got := trainyardStations[stationIndex("S8_SHIP")]; got != "DEP" {
		t.Fatalf("S8 must map to DEP, not the terminus; got %q", got)
	}
	if stationIndex("S8_SHIP") >= len(trainyardStations)-1 {
		t.Fatalf("DEP must sit strictly left of the WITNESS terminus")
	}
	// Closeout/archive collapse onto the witnessed terminus.
	if got := stationIndex("S11_ARCHIVE"); got != len(trainyardStations)-1 {
		t.Fatalf("S11 collapses onto the WITNESS terminus idx %d; got %d", len(trainyardStations)-1, got)
	}
}

func TestSignalGlyphHonestGates(t *testing.T) {
	// MEASURED-stale (low-but-positive freshness) is DARK ✖ — heavier than red, uncrossable.
	if g := signalGlyph("ok", 0.1); g != "✖" {
		t.Fatalf("measured-stale gate is DARK ✖; got %q", g)
	}
	if signalGlyph("ok", 0.1) == "►" {
		t.Fatalf("a dark gate must never render green ► even when criticality is ok")
	}
	// ABSENT freshness (==0) is honest UNKNOWN ◌ — NOT dark, NOT clear. Painting universal ✖
	// when the API simply doesn't report freshness would be a lie.
	if g := signalGlyph("ok", 0); g != "◌" {
		t.Fatalf("absent freshness is UNKNOWN ◌, not dark; got %q", g)
	}
	if g := signalGlyph("crit", 0.9); g != "■" {
		t.Fatalf("crit is a red ■ hard stop; got %q", g)
	}
	if g := signalGlyph("warn", 0.9); g != "▷" {
		t.Fatalf("warn is amber ▷; got %q", g)
	}
	if g := signalGlyph("ok", 0.9); g != "►" {
		t.Fatalf("ok+fresh is green ►; got %q", g)
	}
}

func TestTrainCapsByVelocity(t *testing.T) {
	for v, want := range map[string][2]string{
		"fast":    {">", ">"},
		"normal":  {"(", ")"},
		"slow":    {"<", "<"},
		"stalled": {"[", "]"},
	} {
		if l, r := trainCaps(v); l != want[0] || r != want[1] {
			t.Fatalf("trainCaps(%s) = %q%q, want %q%q", v, l, r, want[0], want[1])
		}
	}
}

func TestTaskBlockedRoutesToSiding(t *testing.T) {
	if !taskBlocked(Task{Stage: "S5", Criticality: "crit", Freshness: 0.5}) {
		t.Fatal("a crit task is blocked -> siding")
	}
	if !taskBlocked(Task{Stage: "S5", PredictedStage: "hold", Freshness: 0.5}) {
		t.Fatal("a held task (predicted=hold) is blocked -> siding")
	}
	if taskBlocked(Task{Stage: "S5", Criticality: "ok", Freshness: 0.8}) {
		t.Fatal("a healthy task stays on the mainline")
	}
}

func TestRenderTrainyardEncodesStateInGlyphs(t *testing.T) {
	y := Trainyard{Tasks: []Task{
		{TaskID: "a", Stage: "S5_IMPL", Owner: "claude", Criticality: "ok", Freshness: 0.9, RelCount: 2},
		{TaskID: "b", Stage: "S6_VERIFY", Owner: "codex", Criticality: "crit", Freshness: 0.4, RelCount: 1},
		{TaskID: "c", Stage: "S2_SCOPE", Owner: "operator", Criticality: "ok", Freshness: 0.08, RelCount: 1}, // measured-stale -> DARK ✖
		{TaskID: "d", Stage: "S1_TRIAGE", Owner: "claude", Criticality: "ok", Freshness: 0.0, RelCount: 1},   // absent freshness -> ◌ unknown
	}}
	out := RenderTrainyard(y, 100)
	if strings.TrimSpace(out) == "" {
		t.Fatal("render must not be empty")
	}
	for _, want := range []string{"○", "WIT", "✖", "◌"} { // stations, terminus, a dark gate, an unknown gate
		if !strings.Contains(out, want) {
			t.Fatalf("render must contain %q; got:\n%s", want, out)
		}
	}
	// The blocked crit task terminates on a siding bumper.
	if !strings.Contains(out, "┤") && !strings.Contains(out, "╣") {
		t.Fatalf("a blocked task must end on a siding bumper ┤/╣; got:\n%s", out)
	}
}

func TestRenderTrainyardEmptyDoesNotPanic(t *testing.T) {
	if strings.TrimSpace(RenderTrainyard(Trainyard{}, 80)) == "" {
		t.Fatal("an empty yard should still render an honest empty-state line")
	}
}
