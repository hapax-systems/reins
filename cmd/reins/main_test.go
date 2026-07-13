package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestFetchContextOncePresentFoldsToSafeDark(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(
			`{"schema":"hapax.reins-context-read.v1","state":"present","audience":"operator_private","reason_codes":[],"projection":{"sentinel":"UNVERIFIED-PRIVATE"},"compatibility":null}`,
		))
	}))
	defer server.Close()

	const epoch uint64 = 7
	msg, ok := fetchContextOnce(server.URL, epoch).(model.ContextMsg)
	if !ok {
		t.Fatal("context fetch did not return ContextMsg")
	}
	if msg.Error == "" ||
		msg.Epoch != epoch ||
		msg.Readout.State != grammar.ContextReadDark ||
		msg.Readout.Projection != nil ||
		msg.Readout.Compatibility != nil ||
		len(msg.Readout.RawEnvelope) != 0 ||
		len(msg.Readout.ReasonCodes) != 1 ||
		msg.Readout.ReasonCodes[0] != grammar.ContextReadReasonCanonUnverified {
		t.Fatalf("unverified PRESENT was not reduced to safe DARK: %+v", msg)
	}

	m := model.New("REINS")
	m.ContextEpoch = epoch
	updated, _ := m.Update(msg)
	m = updated.(model.Model)
	if m.ContextReadout.State != grammar.ContextReadDark ||
		len(m.ContextReadout.ReasonCodes) != 1 ||
		m.ContextReadout.ReasonCodes[0] != grammar.ContextReadReasonCanonUnverified {
		t.Fatalf("safe verifier-unavailable signal was lost in the model fold: %+v", m.ContextReadout)
	}
}

func TestRootAIROffSchedulesFreshCurrentEpochContextFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(
			`{"schema":"hapax.reins-context-read.v1","state":"dark","audience":"operator_private","reason_codes":["producer_absent"],"projection":null,"compatibility":null}`,
		))
	}))
	defer server.Close()

	m := model.New("REINS").SetAIR(true)
	priorEpoch := m.ContextEpoch
	updated, cmd := (root{m: m, url: server.URL}).Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{'a'},
	})
	next := updated.(root)
	if next.m.AIR || next.m.ContextEpoch != priorEpoch+1 {
		t.Fatalf(
			"AIR off did not advance the context epoch: air=%v epoch=%d",
			next.m.AIR,
			next.m.ContextEpoch,
		)
	}
	if cmd == nil {
		t.Fatal("AIR off did not schedule a fresh context read")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("AIR off command is %T, want tea.BatchMsg", cmd())
	}
	foundCurrentEpochFetch := false
	for _, batched := range batch {
		if batched == nil {
			continue
		}
		if msg, ok := batched().(model.ContextMsg); ok {
			if msg.Epoch != next.m.ContextEpoch {
				t.Fatalf("fresh context fetch used epoch %d, want %d", msg.Epoch, next.m.ContextEpoch)
			}
			foundCurrentEpochFetch = true
		}
	}
	if !foundCurrentEpochFetch {
		t.Fatal("AIR off batch omitted the immediate current-epoch context fetch")
	}
}

func TestRootDoesNotRearmStaleContextEpoch(t *testing.T) {
	m := model.New("REINS").SetAIR(true).SetAIR(false)
	stale := model.ContextMsg{
		Epoch: m.ContextEpoch - 2,
		Readout: grammar.ContextReadout{
			Schema:      grammar.ContextReadSchema,
			State:       grammar.ContextReadDark,
			Audience:    grammar.ContextReadAudience,
			ReasonCodes: []string{"producer_absent"},
		},
	}
	_, cmd := (root{m: m, url: "http://127.0.0.1:0"}).Update(stale)
	if cmd != nil {
		t.Fatal("stale context response rearmed a polling loop")
	}
}
