package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hapax-systems/reins/internal/config"
	"github.com/hapax-systems/reins/internal/grammar"
	"github.com/hapax-systems/reins/internal/model"
)

func TestTickCmdProducesEventsMsg(t *testing.T) {
	// against an unreachable url, the tick must still yield an EventsMsg (dark=true), never panic.
	msg := fetchOnce("http://127.0.0.1:0")
	em, ok := msg.(model.EventsMsg)
	if !ok {
		t.Fatalf("tick must yield model.EventsMsg, got %T", msg)
	}
	if !em.Dark {
		t.Fatal("unreachable api must fold to dark (honest), not empty-success")
	}
}

func TestParseProbeSize(t *testing.T) {
	w, h, ok := parseProbeSize("size:170x46")
	if !ok || w != 170 || h != 46 {
		t.Fatalf("size:170x46 parsed as w=%d h=%d ok=%v", w, h, ok)
	}
	for _, bad := range []string{"size:", "size:170", "size:0x46", "size:170x0", "size:widextall"} {
		if w, h, ok := parseProbeSize(bad); ok {
			t.Fatalf("%q should be rejected, got w=%d h=%d", bad, w, h)
		}
	}
}

func TestProbePageTokenCoversRegisteredPagesAndAliases(t *testing.T) {
	for _, tt := range []struct {
		arg  string
		page int
	}{
		{"events", model.PageEvents},
		{"tasks", model.PageTasks},
		{"sessions", model.PageSessions},
		{"yard", model.PageYard},
		{"readiness", model.PageReadiness},
		{"ready", model.PageReadiness},
		{"gates", model.PageReadiness},
		{"gate", model.PageReadiness},
		{"capabilities", model.PageCaps},
		{"caps", model.PageCaps},
		{"cap", model.PageCaps},
		{"intake", model.PageIntake},
		{"obs", model.PageIntake},
		{"dynamics", model.PageDynamics},
		{"dyn", model.PageDynamics},
		{"epistemics", model.PageEpistemics},
		{"epi", model.PageEpistemics},
		{"help", model.PageHelp},
		{"commands", model.PageCommands},
		{"cmds", model.PageCommands},
		{"windows", model.PageWindows},
		{"wins", model.PageWindows},
		{"surfaces", model.PageSurfaces},
		{"surf", model.PageSurfaces},
		{"domains", model.PageDomains},
		{"terrain", model.PageDomains},
		{"lifecycles", model.PageLifecycles},
		{"life", model.PageLifecycles},
		{"lifecycle", model.PageLifecycles},
		{"ndlc", model.PageLifecycles},
		{"n-dlc", model.PageLifecycles},
		{"intent", model.PageIntent},
		{"legend", model.PageLegend},
		{"traces", model.PageTraces},
		{"trace", model.PageTraces},
		{"axes", model.PageAxes},
		{"framework", model.PageAxes},
		{"identity", model.PageIdentity},
		{"who", model.PageIdentity},
		{"a1", model.PageIdentity},
		{"relational", model.PageRelational},
		{"consent", model.PageRelational},
		{"a6", model.PageRelational},
	} {
		t.Run(tt.arg, func(t *testing.T) {
			page, ok := probePageToken(tt.arg)
			if !ok || page != tt.page {
				t.Fatalf("probePageToken(%q) = page %d ok %v, want page %d", tt.arg, page, ok, tt.page)
			}
		})
	}
	if page, ok := probePageToken("unknown"); ok {
		t.Fatalf("unknown probe token should not route, got page %d", page)
	}
}

func TestUpdateProbeModelAdvancesReadSourceState(t *testing.T) {
	m := updateProbeModel(model.New("REINS"), model.EventsMsg{})
	if m.EventsSeq != 1 || m.LastFold != "events" {
		t.Fatalf("probe update should mirror live read folds, seq=%d last=%q", m.EventsSeq, m.LastFold)
	}
}

func TestTickCmdProducesTracesMsg(t *testing.T) {
	// against an unreachable url, the traces fetch must still yield a TracesMsg (dark=true), never panic.
	msg := fetchTracesOnce("http://127.0.0.1:0")
	tm, ok := msg.(model.TracesMsg)
	if !ok {
		t.Fatalf("fetchTracesOnce must yield model.TracesMsg, got %T", msg)
	}
	if !tm.Dark {
		t.Fatal("unreachable traces api must fold honest-dark, not empty-success")
	}
}

// dispatchSlot: inflection verbs key per-dispatch (last-wins); governed verbs share the 30s window.
func TestDispatchSlotInflectionVsGoverned(t *testing.T) {
	// two focus dispatches in the SAME 30s window get DISTINCT slots (no false dedup of A->B->A refocus)
	if dispatchSlot("inflection", 1, 1000) == dispatchSlot("inflection", 2, 1000) {
		t.Fatal("inflection dispatches must not share a slot (the A->B->A refocus-dedup bug)")
	}
	// a governed verb in the same window SHARES the slot (an accidental double-confirm dedups)
	if dispatchSlot("governed", 1, 1000) != dispatchSlot("governed", 2, 1000) {
		t.Fatal("governed verbs in one 30s window must share a slot (dedup double-confirm)")
	}
	// inflection slots are negative so they never collide with a positive window bucket
	if dispatchSlot("inflection", 5, 999999) >= 0 {
		t.Fatal("inflection slot must be negative (never collide with a window bucket)")
	}
}

func TestObserveConfigStampAbsentIsFirstClass(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope.toml")
	if st := observeConfigStamp(p); st != "absent" {
		t.Fatalf("missing file must stamp 'absent', got %q", st)
	}
}

func TestPollConfigOnceUnchangedStampDoesNotLoad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte("palette = \"gruvbox\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := observeConfigStamp(p)
	pm := pollConfigOnce(p, st)
	if pm.changed || pm.cfg != nil || pm.err != nil {
		t.Fatalf("unchanged stamp must not re-load: %+v", pm)
	}
}

func TestPollConfigOnceAppearingFileLoads(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	pm0 := pollConfigOnce(p, "absent")
	if pm0.changed || pm0.err != nil {
		t.Fatalf("still-absent file must be a no-op: %+v", pm0)
	}
	if err := os.WriteFile(p, []byte("palette = \"solarized\"\napi_url = \"http://127.0.0.1:9999\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pm := pollConfigOnce(p, pm0.stamp)
	if !pm.changed || pm.err != nil || pm.cfg == nil {
		t.Fatalf("appearing file must load: %+v", pm)
	}
	if pm.cfg.Palette != "solarized" || pm.cfg.APIURL != "http://127.0.0.1:9999" {
		t.Fatalf("loaded values wrong: %+v", pm.cfg)
	}
}

func TestPollConfigOnceMalformedFailsClosedWithLastGood(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte("palette = \"gruvbox\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	good := pollConfigOnce(p, "absent")
	if !good.changed || good.err != nil {
		t.Fatalf("initial load must succeed: %+v", good)
	}
	// a malformed edit: stamp moves, load fails — the caller keeps serving the last-good config
	if err := os.WriteFile(p, []byte("palette = [unterminated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := pollConfigOnce(p, good.stamp)
	if !bad.changed || bad.err == nil || bad.cfg != nil {
		t.Fatalf("malformed edit must surface an error and no config: %+v", bad)
	}
}

func TestPollConfigOnceRemovedFileFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte("palette = \"solarized\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	live := pollConfigOnce(p, "absent")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	gone := pollConfigOnce(p, live.stamp)
	if !gone.changed || gone.err != nil || gone.cfg == nil {
		t.Fatalf("removal must re-load to defaults, not error: %+v", gone)
	}
	if gone.cfg.Palette != "gruvbox" { // Defaults()
		t.Fatalf("removal must restore defaults, got palette %q", gone.cfg.Palette)
	}
}

func TestRootUpdateConfigReloadAppliesAndKeepsLastGood(t *testing.T) {
	defer grammar.SetPalette("gruvbox") // restore the global palette — no cross-test leak
	m := model.New("REINS")
	r := root{m: m, url: "http://127.0.0.1:8799", cfgPath: "/tmp/x", palette: "gruvbox"}

	// malformed edit: last-good keeps serving, the notice persists and leads with the corrective action
	rm, _ := r.Update(configPollMsg{stamp: "2", changed: true, err: os.ErrInvalid})
	r = rm.(root)
	if r.m.ConfigNotice == "" {
		t.Fatal("a failed reload must leave a persistent honest notice")
	}
	if !strings.HasPrefix(r.m.ConfigNotice, "config kept last-good — fix the TOML or remove the file; ") {
		t.Fatalf("the notice must lead with the corrective action, got %q", r.m.ConfigNotice)
	}
	if !strings.Contains(r.m.ConfigNotice, os.ErrInvalid.Error()) {
		t.Fatalf("the notice must carry the underlying error, got %q", r.m.ConfigNotice)
	}
	if r.url != "http://127.0.0.1:8799" || r.palette != "gruvbox" {
		t.Fatalf("failed reload must not move url/palette: %q %q", r.url, r.palette)
	}

	// a later good load clears the notice and applies palette + url live, through the real path
	good := configPollMsg{stamp: "3", changed: true, cfg: &config.Config{APIURL: "http://127.0.0.1:9999", Palette: "solarized"}}
	rm, _ = r.Update(good)
	r = rm.(root)
	if r.m.ConfigNotice != "" {
		t.Fatalf("successful reload must clear the notice, got %q", r.m.ConfigNotice)
	}
	if r.url != "http://127.0.0.1:9999" || r.palette != "solarized" {
		t.Fatalf("successful reload must apply url+palette: %q %q", r.url, r.palette)
	}
	if r.m.Flash == "" {
		t.Fatal("successful reload must flash a transient confirmation")
	}
	if mode := grammar.PaletteMode(); mode != "solarized" {
		t.Fatalf("SetPalette through Update must switch the live grammar, got mode %q", mode)
	}
}

// The unchanged-stamp branch: no reload, state untouched, the poll re-arms.
func TestRootUpdateConfigUnchangedRearms(t *testing.T) {
	m := model.New("REINS")
	r := root{m: m, url: "http://127.0.0.1:8799", cfgPath: "/tmp/x", palette: "gruvbox"}
	rm, cmd := r.Update(configPollMsg{stamp: "9", changed: false})
	r = rm.(root)
	if cmd == nil {
		t.Fatal("an unchanged poll must re-arm the reload tick")
	}
	if r.m.ConfigNotice != "" || r.url != "http://127.0.0.1:8799" {
		t.Fatalf("an unchanged poll must not touch state: %q %q", r.m.ConfigNotice, r.url)
	}
}

// A present-but-unreadable config is NOT the absent case: stat fails with ENOTDIR here, the poll
// must surface an error (last-good keeps serving) rather than silently fall back to defaults.
func TestPollConfigOnceStatErrorIsNotAbsent(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(blocker, "config.toml") // a path component is a file -> ENOTDIR
	if st := observeConfigStamp(p); st == "absent" {
		t.Fatalf("a stat error must not masquerade as absent, got %q", st)
	}
	pm := pollConfigOnce(p, "absent")
	if !pm.changed || pm.err == nil || pm.cfg != nil {
		t.Fatalf("a stat error must fail closed with the error, got %+v", pm)
	}
}
